package deploy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// Volume sizes on node-local storage. A k3s local-path volume is a folder on a node's disk: the kubelet
// reports the whole filesystem for it (see pvcStats), so the panel measures the folders itself. Every few
// minutes a short-lived pod per node mounts the folders read-only and runs du; the results are kept in memory
// and used wherever volume usage is shown (app volumes, server volumes, the "volume full" alert).

// LabelVolumeMeter marks the measuring pods.
const LabelVolumeMeter = "nectar.dev/volume-meter"

// meterBatch is the most folders one pod mounts (pods with very many volumes are slow to start).
const meterBatch = 60

type duEntry struct {
	UsedB int64
	At    time.Time
}

// localDir is a node-local volume folder: the PVC it belongs to ("namespace/name"), its node and its path.
type localDir struct {
	Key, Node, Path string
}

// localDirs finds the node-local folders of the given PVCs from their PersistentVolumes (local-path writes
// hostPath or local volumes with a node affinity).
func localDirs(pvs []kube.Object, claims map[string]bool) []localDir {
	var out []localDir
	for _, pv := range pvs {
		key := kube.Str(pv, "spec", "claimRef", "namespace") + "/" + kube.Str(pv, "spec", "claimRef", "name")
		if !claims[key] {
			continue
		}
		path := kube.Str(pv, "spec", "hostPath", "path")
		if path == "" {
			path = kube.Str(pv, "spec", "local", "path")
		}
		if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\n\r\x00") {
			continue // network storage (Longhorn, NFS, …): the kubelet reports those correctly
		}
		node := ""
		for _, t := range kube.Slice(pv, "spec", "nodeAffinity", "required", "nodeSelectorTerms") {
			for _, e := range kube.Slice(t, "matchExpressions") {
				if kube.Str(e, "key") == "kubernetes.io/hostname" && kube.Str(e, "operator") == "In" {
					if vals, _ := kube.Field(e, "values").([]any); len(vals) > 0 {
						node, _ = vals[0].(string)
					}
				}
			}
		}
		if node == "" {
			node = kube.Str(pv, "metadata", "annotations", "local.path.provisioner/selected-node")
		}
		if node == "" {
			continue
		}
		out = append(out, localDir{Key: key, Node: node, Path: path})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// meterPod is the measuring pod for some folders of one node: folder i is mounted at /v/i.
func meterPod(ns, name, node string, dirs []localDir) kube.Object {
	var volumes, mounts []any
	for i, d := range dirs {
		v := "v" + strconv.Itoa(i)
		volumes = append(volumes, map[string]any{"name": v, "hostPath": map[string]any{"path": d.Path, "type": "Directory"}})
		mounts = append(mounts, map[string]any{"name": v, "mountPath": "/v/" + strconv.Itoa(i), "readOnly": true})
	}
	return kube.Object{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": name, "namespace": ns,
			"labels": map[string]any{LabelVolumeMeter: "1", LabelManagedBy: ManagedBy}},
		"spec": map[string]any{
			"nodeName":                     node,
			"restartPolicy":                "Never",
			"automountServiceAccountToken": false,
			"enableServiceLinks":           false,
			"activeDeadlineSeconds":        300,
			"tolerations":                  []any{map[string]any{"operator": "Exists"}},
			"containers": []any{map[string]any{
				"name": "du", "image": InitImage, "imagePullPolicy": "IfNotPresent",
				// "<KiB>\t/v/<i>" per folder; -x stays on the volume's filesystem
				"command": []any{"sh", "-c", `for d in /v/*; do du -skx "$d" 2>/dev/null; done; true`},
				"securityContext": map[string]any{
					"runAsUser": 0, "runAsGroup": 0, "allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true,
					// read any folder whatever its owner and mode; nothing else
					"capabilities": map[string]any{"drop": []any{"ALL"}, "add": []any{"DAC_READ_SEARCH"}},
				},
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "10m", "memory": "16Mi"},
					"limits":   map[string]any{"cpu": "200m", "memory": "64Mi"},
				},
				"volumeMounts": mounts,
			}},
			"volumes": volumes,
		},
	}
}

// parseDu reads the output of the measuring pod: folder index → bytes.
func parseDu(out string) map[int]int64 {
	res := map[int]int64{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) != 2 || !strings.HasPrefix(f[1], "/v/") {
			continue
		}
		kib, err1 := strconv.ParseInt(f[0], 10, 64)
		i, err2 := strconv.Atoi(strings.TrimPrefix(f[1], "/v/"))
		if err1 == nil && err2 == nil {
			res[i] = kib * 1024
		}
	}
	return res
}

// MeasureLocalVolumes measures every node-local Nectar volume (one pod per node and batch).
func (d *Deployer) MeasureLocalVolumes(ctx context.Context) error {
	pvcs, err := d.kube.List(ctx, "PersistentVolumeClaim", "", LabelManagedBy+"="+ManagedBy)
	if err != nil {
		return err
	}
	claims := map[string]bool{}
	for _, p := range pvcs {
		claims[kube.Str(p, "metadata", "namespace")+"/"+kube.Str(p, "metadata", "name")] = true
	}
	if len(claims) == 0 {
		return nil
	}
	pvs, err := d.kube.List(ctx, "PersistentVolume", "", "")
	if err != nil {
		return err
	}
	byNode := map[string][]localDir{}
	for _, dir := range localDirs(pvs, claims) {
		byNode[dir.Node] = append(byNode[dir.Node], dir)
	}
	var errs []error
	for node, dirs := range byNode {
		for start := 0; start < len(dirs); start += meterBatch {
			batch := dirs[start:min(start+meterBatch, len(dirs))]
			name := fmt.Sprintf("nectar-du-%s-%d", jobSafeName(node), start/meterBatch)
			sizes, err := d.runMeter(ctx, name, node, batch)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", node, err))
				continue
			}
			now := time.Now()
			d.duMu.Lock()
			if d.du == nil {
				d.du = map[string]duEntry{}
			}
			for i, b := range sizes {
				if i < len(batch) {
					d.du[batch[i].Key] = duEntry{UsedB: b, At: now}
				}
			}
			d.duMu.Unlock()
		}
	}
	return errors.Join(errs...)
}

// runMeter runs one measuring pod and returns its sizes.
func (d *Deployer) runMeter(ctx context.Context, name, node string, dirs []localDir) (map[int]int64, error) {
	ns := d.opt.SystemNamespace
	_ = d.kube.Delete(ctx, "Pod", ns, name) // a leftover of an interrupted run
	defer func() { _ = d.kube.Delete(context.WithoutCancel(ctx), "Pod", ns, name) }()
	// the delete above may still be in progress: retry the create for a moment
	var err error
	for i := 0; i < 10; i++ {
		if _, err = d.kube.Apply(ctx, meterPod(ns, name, node, dirs)); err == nil {
			break
		}
		if err := sleep(ctx, time.Second); err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		var pod kube.Object
		if err := d.kube.GetObject(ctx, "Pod", ns, name, &pod); err != nil {
			return nil, err
		}
		switch kube.Str(pod, "status", "phase") {
		case "Succeeded":
			out, err := d.kube.PodLogs(ctx, ns, name, "du", 10000, false)
			if err != nil {
				return nil, err
			}
			return parseDu(out), nil
		case "Failed":
			return nil, errors.New("the measuring pod failed: " + kube.Str(pod, "status", "message"))
		}
		if time.Now().After(deadline) {
			return nil, errors.New("the measuring pod did not finish in time")
		}
		if err := sleep(ctx, d.opt.PollInterval); err != nil {
			return nil, err
		}
	}
}

// measuredUsage returns the last measured size of a PVC ("namespace/name") if it is recent.
func (d *Deployer) measuredUsage(key string) (int64, bool) {
	d.duMu.Lock()
	defer d.duMu.Unlock()
	e, ok := d.du[key]
	if !ok || time.Since(e.At) > 2*time.Hour {
		return 0, false
	}
	return e.UsedB, true
}

// RunVolumeMeter measures node-local volumes now and then every interval until ctx ends.
func (d *Deployer) RunVolumeMeter(ctx context.Context, every time.Duration) {
	for {
		if err := d.MeasureLocalVolumes(ctx); err != nil && ctx.Err() == nil {
			d.log.Warn("volume sizes", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// jobSafeName turns a node name into a DNS label part.
func jobSafeName(s string) string {
	b := []byte(strings.ToLower(s))
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			b[i] = '-'
		}
	}
	out := strings.Trim(string(b), "-")
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}
