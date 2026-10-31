package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// ErrNotDeployed is returned by live operations on an app that has nothing in the cluster yet.
var ErrNotDeployed = errors.New("the app is not deployed yet")

// Scale changes the replicas of a deployed app right away, without a new release. For cron apps 0 pauses
// the schedule and anything else resumes it. The app's config is updated so the next deploy keeps it.
func (d *Deployer) Scale(ctx context.Context, a store.App, p store.Project, replicas int) error {
	if d.Busy(a.ID) {
		return ErrBusy
	}
	var err error
	switch a.Kind {
	case runtimes.KindWeb, runtimes.KindWorker:
		_, err = d.kube.Patch(ctx, "Deployment", p.Namespace, a.Slug, map[string]any{"spec": map[string]any{"replicas": replicas}})
	case runtimes.KindCron:
		_, err = d.kube.Patch(ctx, "CronJob", p.Namespace, a.Slug, map[string]any{"spec": map[string]any{"suspend": replicas == 0}})
		if replicas > 1 {
			replicas = 1
		}
	default:
		return errors.New("task apps run on demand: there is nothing to scale")
	}
	if kube.IsNotFound(err) {
		return ErrNotDeployed
	}
	if err != nil {
		return err
	}
	resume := a.Config.ResumeReplicas
	if replicas == 0 && a.Config.Replicas > 0 {
		resume = a.Config.Replicas
	}
	if err := d.store.SetAppReplicas(ctx, a.ID, replicas, resume); err != nil {
		return err
	}
	status := StatusRunning
	switch {
	case replicas == 0:
		status = StatusStopped
	case a.Kind == runtimes.KindCron:
		status = StatusScheduled
	}
	return d.store.SetAppStatus(ctx, a.ID, status, "")
}

// Restart replaces the pods of a web app or worker one by one (a rolling restart, no new release).
func (d *Deployer) Restart(ctx context.Context, a store.App, p store.Project) error {
	if a.Kind != runtimes.KindWeb && a.Kind != runtimes.KindWorker {
		return errors.New("only web apps and workers can be restarted")
	}
	if d.Busy(a.ID) {
		return ErrBusy
	}
	if a.Config.Replicas == 0 {
		return errors.New("the app is stopped: start it instead")
	}
	_, err := d.kube.Patch(ctx, "Deployment", p.Namespace, a.Slug, map[string]any{"spec": map[string]any{"template": map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{"nectar.dev/restarted-at": time.Now().UTC().Format(time.RFC3339)}},
	}}})
	if kube.IsNotFound(err) {
		return ErrNotDeployed
	}
	return err
}

// RestartAddon restarts a database's pod (rolling restart of its StatefulSet).
func (d *Deployer) RestartAddon(ctx context.Context, a store.Addon, p store.Project) error {
	if a.Stopped {
		return errors.New("the database is stopped: start it instead")
	}
	_, err := d.kube.Patch(ctx, "StatefulSet", p.Namespace, a.Slug, map[string]any{"spec": map[string]any{"template": map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{"nectar.dev/restarted-at": time.Now().UTC().Format(time.RFC3339)}},
	}}})
	if kube.IsNotFound(err) {
		return ErrNotDeployed
	}
	return err
}

// Pending lists what changed since the live release and needs a deploy to take effect: "settings"
// (config), "image" (runtime version or image) and "environment" (env vars or config files).
func (d *Deployer) Pending(ctx context.Context, a store.App) ([]string, error) {
	live, err := d.store.LiveRelease(ctx, a.ID)
	if errors.Is(err, store.ErrNotFound) {
		return []string{}, nil // never deployed: the UI says so
	}
	if err != nil {
		return nil, err
	}
	out := []string{}
	norm := func(c store.AppConfig) string {
		c.Replicas, c.ResumeReplicas = 0, 0 // scaling is applied live
		b, _ := json.Marshal(c)
		return string(b)
	}
	if norm(a.Config) != norm(live.Config) {
		out = append(out, "settings")
	}
	if img, err := d.Image(a); err == nil && img != live.Image {
		out = append(out, "image")
	}
	if t, err := d.store.EnvChangedAt(ctx, a.ID); err == nil && t.After(live.CreatedAt) {
		out = append(out, "environment")
	}
	return out, nil
}

// VolumeInfo is a volume of an app as it exists in the cluster.
type VolumeInfo struct {
	Name         string `json:"name"`
	Claim        string `json:"claim"`
	MountPath    string `json:"mount_path,omitempty"`
	RequestedMB  int64  `json:"requested_mb"`
	CapacityB    int64  `json:"capacity_bytes,omitempty"`
	UsedB        int64  `json:"used_bytes,omitempty"` // from the kubelet, while a pod mounts it
	Phase        string `json:"phase"`                // Pending | Bound | Lost | "" (not created yet)
	StorageClass string `json:"storage_class,omitempty"`
	InConfig     bool   `json:"in_config"` // part of the app settings
	Mounted      bool   `json:"mounted"`   // used by the live release
}

// rawGetter is implemented by the real Kubernetes client (kubelet summaries for volume usage).
type rawGetter interface {
	Get(ctx context.Context, path string, out any) error
}

// Volumes lists the configured volumes and the app's PVCs (including ones removed from the settings).
func (d *Deployer) Volumes(ctx context.Context, a store.App, p store.Project) ([]VolumeInfo, error) {
	byName := map[string]*VolumeInfo{}
	var order []string
	for _, v := range a.Config.Volumes {
		byName[v.Name] = &VolumeInfo{Name: v.Name, Claim: VolumeClaimName(a.Slug, v.Name), MountPath: v.MountPath, RequestedMB: v.SizeMB, InConfig: true}
		order = append(order, v.Name)
	}
	if live, err := d.store.LiveRelease(ctx, a.ID); err == nil {
		for _, v := range live.Config.Volumes {
			if vi, ok := byName[v.Name]; ok {
				vi.Mounted = true
			} else {
				byName[v.Name] = &VolumeInfo{Name: v.Name, Claim: VolumeClaimName(a.Slug, v.Name), MountPath: v.MountPath, RequestedMB: v.SizeMB, Mounted: true}
				order = append(order, v.Name)
			}
		}
	}
	pvcs, err := d.kube.List(ctx, "PersistentVolumeClaim", p.Namespace, LabelApp+"="+a.Slug+","+LabelManagedBy+"="+ManagedBy)
	if err != nil && !kube.IsNotFound(err) {
		return nil, err
	}
	claims := map[string]*VolumeInfo{}
	for _, pvc := range pvcs {
		name := kube.Str(pvc, "metadata", "labels", LabelVolume)
		if name == "" {
			continue
		}
		vi, ok := byName[name]
		if !ok {
			vi = &VolumeInfo{Name: name, Claim: kube.Str(pvc, "metadata", "name")}
			byName[name] = vi
			order = append(order, name)
		}
		vi.Phase = kube.Str(pvc, "status", "phase")
		vi.StorageClass = kube.Str(pvc, "spec", "storageClassName")
		if q, err := kube.ParseQuantity(kube.Str(pvc, "status", "capacity", "storage")); err == nil {
			vi.CapacityB = int64(q)
		}
		if !vi.InConfig {
			if q, err := kube.ParseQuantity(kube.Str(pvc, "spec", "resources", "requests", "storage")); err == nil {
				vi.RequestedMB = int64(q) / (1 << 20)
			}
		}
		claims[vi.Claim] = vi
	}
	d.volumeUsage(ctx, a, p, claims)
	out := make([]VolumeInfo, 0, len(order))
	for _, n := range order {
		out = append(out, *byName[n])
	}
	return out, nil
}

// volumeUsage fills UsedB from the kubelet summaries of the nodes running the app's pods (best effort).
func (d *Deployer) volumeUsage(ctx context.Context, a store.App, p store.Project, claims map[string]*VolumeInfo) {
	// node-local folders (k3s local-path): sizes measured by the volume meter (the kubelet cannot tell them)
	defer func() {
		for name, vi := range claims {
			if vi.UsedB <= 0 {
				if b, ok := d.measuredUsage(p.Namespace + "/" + name); ok {
					vi.UsedB = b
				}
			}
		}
	}()
	rg, ok := d.kube.(rawGetter)
	if !ok || len(claims) == 0 {
		return
	}
	pods, err := d.kube.List(ctx, "Pod", p.Namespace, LabelApp+"="+a.Slug)
	if err != nil {
		return
	}
	nodes := map[string]bool{}
	for _, pod := range pods {
		if n := kube.Str(pod, "spec", "nodeName"); n != "" {
			nodes[n] = true
		}
	}
	for node := range nodes {
		for key, st := range nodePVCStats(ctx, rg, node) {
			if vi, ok := claims[strings.TrimPrefix(key, p.Namespace+"/")]; ok && strings.HasPrefix(key, p.Namespace+"/") && st.UsedB > vi.UsedB {
				vi.UsedB = st.UsedB
			}
		}
	}
}

// ResizeVolume grows a volume. The storage class must support expansion (k3s local-path does not: there
// the size is not enforced anyway, so the new size is only recorded).
func (d *Deployer) ResizeVolume(ctx context.Context, a store.App, p store.Project, name string, sizeMB int64) (applied bool, err error) {
	return d.ResizeClaim(ctx, p.Namespace, VolumeClaimName(a.Slug, name), sizeMB)
}

// ResizeClaim grows a PVC. applied is false when the claim does not exist yet or the storage class cannot
// expand it (the caller then only records the size).
func (d *Deployer) ResizeClaim(ctx context.Context, namespace, claim string, sizeMB int64) (applied bool, err error) {
	var pvc kube.Object
	if err := d.kube.GetObject(ctx, "PersistentVolumeClaim", namespace, claim, &pvc); kube.IsNotFound(err) {
		return false, nil // not created yet: it is created with the new size
	} else if err != nil {
		return false, err
	}
	cur, _ := kube.ParseQuantity(kube.Str(pvc, "spec", "resources", "requests", "storage"))
	if float64(sizeMB)*(1<<20) < cur {
		return false, fmt.Errorf("volumes can only grow (now %d MiB)", int64(cur)/(1<<20))
	}
	_, err = d.kube.Patch(ctx, "PersistentVolumeClaim", namespace, claim, map[string]any{
		"spec": map[string]any{"resources": map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dMi", sizeMB)}}},
	})
	if err != nil {
		// most often the storage class cannot expand volumes: the caller records the size only
		d.log.Info("volume resize not applied", "claim", claim, "err", err)
		return false, nil
	}
	return true, nil
}

// DeleteVolume deletes the PVC of a volume that is neither configured nor used by the live release.
func (d *Deployer) DeleteVolume(ctx context.Context, a store.App, p store.Project, name string) error {
	inUse := func(vs []store.Volume) bool {
		return slices.ContainsFunc(vs, func(v store.Volume) bool { return v.Name == name })
	}
	if inUse(a.Config.Volumes) {
		return errors.New("remove the volume from the app settings and redeploy first")
	}
	if live, err := d.store.LiveRelease(ctx, a.ID); err == nil && inUse(live.Config.Volumes) {
		return errors.New("the running release still mounts this volume: redeploy first")
	}
	claim := VolumeClaimName(a.Slug, name)
	if err := d.releaseClaim(ctx, p.Namespace, claim); err != nil {
		return err
	}
	err := d.kube.Delete(ctx, "PersistentVolumeClaim", p.Namespace, claim)
	if kube.IsNotFound(err) {
		return nil
	}
	return err
}

// releaseClaim deletes the finished pods (backup, restore, release and task jobs) that still reference
// a PersistentVolumeClaim: Kubernetes keeps a claim "in use" while such a pod exists, so the claim would
// stay Terminating until the job's TTL (a week for backups) and block a new app of the same name.
func (d *Deployer) releaseClaim(ctx context.Context, namespace, claim string) error {
	pods, err := d.kube.List(ctx, "Pod", namespace, "")
	if kube.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, pod := range pods {
		// finished pods (backup/restore jobs) and the volume file browser still hold the claim
		browser := kube.Str(pod, "metadata", "labels", "nectar.dev/files") != ""
		if phase := kube.Str(pod, "status", "phase"); phase != "Succeeded" && phase != "Failed" && !browser {
			continue
		}
		for _, v := range kube.Slice(pod, "spec", "volumes") {
			if kube.Str(v, "persistentVolumeClaim", "claimName") == claim {
				_, _, name := kube.Meta(pod)
				if err := d.kube.Delete(ctx, "Pod", namespace, name); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}
