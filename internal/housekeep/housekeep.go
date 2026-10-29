// Package housekeep keeps the panel's disk usage in check: retention of artifacts, audit entries and
// resolved alerts, expired sessions and uploads, and on-demand removal of unused container images on the
// nodes (PLAN.md §7 "Disk housekeeping").
package housekeep

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/artifacts"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// Settings are the retention options (settings key "housekeeping").
type Settings struct {
	KeepArtifacts int `json:"keep_artifacts"` // newest uploads kept per app (artifacts of live releases are always kept)
	AuditDays     int `json:"audit_days"`
	AlertDays     int `json:"alert_days"` // resolved alerts
	// K3sDataDir is the k3s data directory on the nodes (for image cleanup), default /var/lib/rancher/k3s.
	K3sDataDir string `json:"k3s_data_dir"`
}

// SettingsKey is the key of Settings in the settings table.
const SettingsKey = "housekeeping"

// Defaults are used for unset values.
var Defaults = Settings{KeepArtifacts: 10, AuditDays: 365, AlertDays: 90, K3sDataDir: "/var/lib/rancher/k3s"}

// Normalize fills in defaults.
func (s Settings) Normalize() Settings {
	if s.KeepArtifacts <= 0 {
		s.KeepArtifacts = Defaults.KeepArtifacts
	}
	if s.AuditDays <= 0 {
		s.AuditDays = Defaults.AuditDays
	}
	if s.AlertDays <= 0 {
		s.AlertDays = Defaults.AlertDays
	}
	if s.K3sDataDir == "" {
		s.K3sDataDir = Defaults.K3sDataDir
	}
	return s
}

// Validate checks user input.
func (s Settings) Validate() error {
	switch {
	case s.KeepArtifacts < 1 || s.KeepArtifacts > 100:
		return errors.New("keep_artifacts must be between 1 and 100")
	case s.AuditDays < 30 || s.AuditDays > 3650:
		return errors.New("audit_days must be between 30 and 3650")
	case s.AlertDays < 7 || s.AlertDays > 3650:
		return errors.New("alert_days must be between 7 and 3650")
	case !filepath.IsAbs(s.K3sDataDir) || strings.Contains(s.K3sDataDir, ".."):
		return errors.New("k3s_data_dir must be an absolute path")
	}
	return nil
}

// Load returns the stored settings with defaults filled in.
func Load(ctx context.Context, st *store.Store) Settings {
	var s Settings
	_ = st.GetSetting(ctx, SettingsKey, &s)
	return s.Normalize()
}

// Kube is the part of the Kubernetes client housekeeping needs.
type Kube interface {
	ListNodes(ctx context.Context) ([]kube.Node, error)
	Get(ctx context.Context, path string, out any) error
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error)
}

// Keeper runs the retention jobs.
type Keeper struct {
	Store     *store.Store
	Artifacts *artifacts.Store
	Kube      Kube // nil without Kubernetes
	DataDir   string
	DBPath    string
	Namespace string // system namespace for image cleanup jobs
	Log       *slog.Logger
	Now       func() time.Time

	mu sync.Mutex // one cleanup at a time
}

func (k *Keeper) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}
	return time.Now()
}

// Result reports what a cleanup removed.
type Result struct {
	Sessions       bool  `json:"-"`
	Artifacts      int   `json:"artifacts"`       // artifact records removed
	Files          int   `json:"files"`           // artifact files removed
	BytesFreed     int64 `json:"bytes_freed"`     // by removed artifact files
	Uploads        int   `json:"uploads"`         // expired unfinished uploads
	AuditEntries   int64 `json:"audit_entries"`   // old audit entries
	ResolvedAlerts int64 `json:"resolved_alerts"` // old resolved alerts
}

// Run cleans up every hour until ctx ends.
func (k *Keeper) Run(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if res, err := k.Clean(ctx); err != nil {
			k.Log.Warn("housekeeping", "err", err)
		} else if res.Files > 0 || res.Uploads > 0 || res.AuditEntries > 0 || res.ResolvedAlerts > 0 {
			k.Log.Info("housekeeping", "artifacts", res.Artifacts, "files", res.Files, "freed", res.BytesFreed,
				"uploads", res.Uploads, "audit", res.AuditEntries, "alerts", res.ResolvedAlerts)
		}
	}
}

// Clean applies the retention settings once.
func (k *Keeper) Clean(ctx context.Context) (Result, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	set := Load(ctx, k.Store)
	now := k.now()
	var res Result
	var errs []error
	if err := k.Store.PurgeExpiredSessions(ctx); err != nil {
		errs = append(errs, fmt.Errorf("sessions: %w", err))
	}
	before, _ := k.Store.CountArtifacts(ctx)
	orphans, err := k.Store.PruneArtifacts(ctx, set.KeepArtifacts)
	if err != nil {
		errs = append(errs, fmt.Errorf("artifacts: %w", err))
	} else {
		after, _ := k.Store.CountArtifacts(ctx)
		res.Artifacts = before - after
		for _, sha := range orphans {
			size := k.Artifacts.Size(sha)
			if err := k.Artifacts.Remove(sha); err == nil {
				res.Files++
				res.BytesFreed += size
			}
		}
	}
	if ids, err := k.Store.ExpiredUploads(ctx); err != nil {
		errs = append(errs, fmt.Errorf("uploads: %w", err))
	} else {
		for _, id := range ids {
			_ = k.Artifacts.RemoveUpload(id)
		}
		res.Uploads = len(ids)
	}
	if n, err := k.Store.PruneAudit(ctx, now.AddDate(0, 0, -set.AuditDays)); err != nil {
		errs = append(errs, fmt.Errorf("audit: %w", err))
	} else {
		res.AuditEntries = n
	}
	if n, err := k.Store.PruneAlerts(ctx, now.AddDate(0, 0, -set.AlertDays)); err != nil {
		errs = append(errs, fmt.Errorf("alerts: %w", err))
	} else {
		res.ResolvedAlerts = n
	}
	return res, errors.Join(errs...)
}

// ------------------------------------------------------------------- report --

// NodeDisk is the disk usage of one node.
type NodeDisk struct {
	Name        string `json:"name"`
	FsUsed      int64  `json:"fs_used"`
	FsTotal     int64  `json:"fs_total"`
	ImagesUsed  int64  `json:"images_used"`  // container image filesystem (k3s data dir)
	ImagesTotal int64  `json:"images_total"` // equals FsTotal when images share the root filesystem
	Error       string `json:"error,omitempty"`
}

// PanelDisk is the usage of the panel's data volume.
type PanelDisk struct {
	VolumeTotal   int64 `json:"volume_total,omitempty"`
	VolumeFree    int64 `json:"volume_free,omitempty"`
	DatabaseBytes int64 `json:"database_bytes"`
	ArtifactBytes int64 `json:"artifact_bytes"`
	ArtifactFiles int   `json:"artifact_files"`
	UploadBytes   int64 `json:"upload_bytes"`
}

// PruneJob is a recent image cleanup on a node.
type PruneJob struct {
	Name     string     `json:"name"`
	Node     string     `json:"node"`
	Status   string     `json:"status"` // running | succeeded | failed
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	Removed  int        `json:"removed"` // images deleted
	Message  string     `json:"message,omitempty"`
}

// Report is the disk overview of the Server page.
type Report struct {
	Nodes    []NodeDisk `json:"nodes"`
	Panel    PanelDisk  `json:"panel"`
	Prunes   []PruneJob `json:"image_cleanups"`
	Settings Settings   `json:"settings"`
}

// Disk builds the disk report (node numbers come live from the kubelet summary API).
func (k *Keeper) Disk(ctx context.Context) Report {
	r := Report{Nodes: []NodeDisk{}, Prunes: []PruneJob{}, Settings: Load(ctx, k.Store)}
	if k.Kube != nil {
		if nodes, err := k.Kube.ListNodes(ctx); err == nil {
			for _, n := range nodes {
				r.Nodes = append(r.Nodes, k.nodeDisk(ctx, n.Name))
			}
		}
		r.Prunes = k.pruneJobs(ctx)
	}
	if total, free, ok := volume(k.DataDir); ok {
		r.Panel.VolumeTotal, r.Panel.VolumeFree = total, free
	}
	for _, f := range []string{k.DBPath, k.DBPath + "-wal", k.DBPath + "-shm"} {
		if info, err := os.Stat(f); err == nil {
			r.Panel.DatabaseBytes += info.Size()
		}
	}
	r.Panel.ArtifactBytes, r.Panel.ArtifactFiles, r.Panel.UploadBytes = k.Artifacts.Usage()
	return r
}

func (k *Keeper) nodeDisk(ctx context.Context, name string) NodeDisk {
	d := NodeDisk{Name: name}
	var sum struct {
		Node struct {
			Fs struct {
				CapacityBytes *int64 `json:"capacityBytes"`
				UsedBytes     *int64 `json:"usedBytes"`
			} `json:"fs"`
			Runtime struct {
				ImageFs struct {
					CapacityBytes *int64 `json:"capacityBytes"`
					UsedBytes     *int64 `json:"usedBytes"`
				} `json:"imageFs"`
			} `json:"runtime"`
		} `json:"node"`
	}
	if err := k.Kube.Get(ctx, "/api/v1/nodes/"+name+"/proxy/stats/summary", &sum); err != nil {
		d.Error = "the node's disk statistics are unavailable: " + err.Error()
		return d
	}
	val := func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	}
	d.FsUsed, d.FsTotal = val(sum.Node.Fs.UsedBytes), val(sum.Node.Fs.CapacityBytes)
	d.ImagesUsed, d.ImagesTotal = val(sum.Node.Runtime.ImageFs.UsedBytes), val(sum.Node.Runtime.ImageFs.CapacityBytes)
	return d
}

// ----------------------------------------------------------- image cleanup --

const pruneLabel = "nectar.dev/housekeeping"

// fallbackPause is used when a node does not list a pause image (it is always present on k3s nodes).
const fallbackPause = "rancher/mirrored-pause:3.6"

// PruneImages starts a Job on each node (or only on node) that removes the container images no container
// uses ("crictl rmi --prune" with the crictl that ships with k3s). It returns the started jobs.
//
// The job runs the pause image (always present on k3s nodes, so nothing has to be pulled) with the k3s data
// directory and the containerd socket mounted from the host.
func (k *Keeper) PruneImages(ctx context.Context, node string) ([]PruneJob, error) {
	if k.Kube == nil {
		return nil, errors.New("kubernetes is not configured")
	}
	nodes, err := k.Kube.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	set := Load(ctx, k.Store)
	running := map[string]bool{}
	for _, j := range k.pruneJobs(ctx) {
		if j.Status == "running" {
			running[j.Node] = true
		}
	}
	now := k.now()
	var out []PruneJob
	for _, n := range nodes {
		if node != "" && n.Name != node {
			continue
		}
		if running[n.Name] {
			continue
		}
		short := jobSafe(n.Name)
		if len(short) > 30 {
			short = strings.TrimRight(short[:30], "-")
		}
		name := fmt.Sprintf("nectar-image-cleanup-%s-%d", short, now.Unix()) // ≤ 63 characters
		if _, err := k.Kube.Apply(ctx, pruneJob(k.Namespace, name, n.Name, k.pauseImage(ctx, n.Name), set.K3sDataDir)); err != nil {
			return out, fmt.Errorf("node %s: %w", n.Name, err)
		}
		out = append(out, PruneJob{Name: name, Node: n.Name, Status: "running", Started: now})
	}
	if node != "" && len(out) == 0 && !running[node] {
		return nil, fmt.Errorf("node %q not found", node)
	}
	return out, nil
}

func jobSafe(s string) string {
	s = strings.ToLower(s)
	b := []byte(s)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			b[i] = '-'
		}
	}
	return strings.Trim(string(b), "-")
}

func (k *Keeper) pauseImage(ctx context.Context, node string) string {
	var n struct {
		Status struct {
			Images []struct {
				Names []string `json:"names"`
			} `json:"images"`
		} `json:"status"`
	}
	if err := k.Kube.GetObject(ctx, "Node", "", node, &n); err != nil {
		return fallbackPause
	}
	for _, img := range n.Status.Images {
		for _, name := range img.Names {
			if strings.Contains(name, "/mirrored-pause:") && !strings.Contains(name, "@") {
				return name
			}
		}
	}
	return fallbackPause
}

func pruneJob(ns, name, node, image, dataDir string) kube.Object {
	dataDir = strings.TrimRight(dataDir, "/")
	return kube.Object{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name": name, "namespace": ns,
			"labels":      map[string]any{pruneLabel: "image-cleanup", "nectar.dev/node": jobSafe(node), "app.kubernetes.io/managed-by": "nectar-apps"},
			"annotations": map[string]any{"nectar.dev/node": node},
		},
		"spec": map[string]any{
			"backoffLimit":            0,
			"activeDeadlineSeconds":   600,
			"ttlSecondsAfterFinished": 86400,
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{pruneLabel: "image-cleanup"}},
				"spec": map[string]any{
					"nodeName":                     node,
					"restartPolicy":                "Never",
					"automountServiceAccountToken": false,
					"tolerations":                  []any{map[string]any{"operator": "Exists"}},
					"containers": []any{map[string]any{
						"name":            "cleanup",
						"image":           image,
						"imagePullPolicy": "IfNotPresent",
						"command":         []any{dataDir + "/data/current/bin/crictl", "rmi", "--prune"},
						"env": []any{
							map[string]any{"name": "CONTAINER_RUNTIME_ENDPOINT", "value": "unix:///run/k3s/containerd/containerd.sock"},
							map[string]any{"name": "IMAGE_SERVICE_ENDPOINT", "value": "unix:///run/k3s/containerd/containerd.sock"},
						},
						"securityContext": map[string]any{"runAsUser": 0, "runAsGroup": 0},
						"resources": map[string]any{
							"requests": map[string]any{"cpu": "10m", "memory": "32Mi"},
							"limits":   map[string]any{"memory": "256Mi"},
						},
						"volumeMounts": []any{
							map[string]any{"name": "k3s-data", "mountPath": dataDir + "/data", "readOnly": true},
							map[string]any{"name": "containerd", "mountPath": "/run/k3s/containerd"},
						},
					}},
					"volumes": []any{
						map[string]any{"name": "k3s-data", "hostPath": map[string]any{"path": dataDir + "/data", "type": "Directory"}},
						map[string]any{"name": "containerd", "hostPath": map[string]any{"path": "/run/k3s/containerd", "type": "Directory"}},
					},
				},
			},
		},
	}
}

// pruneJobs returns the recent image cleanups, newest first.
func (k *Keeper) pruneJobs(ctx context.Context) []PruneJob {
	jobs, err := k.Kube.List(ctx, "Job", k.Namespace, pruneLabel+"=image-cleanup")
	if err != nil {
		return []PruneJob{}
	}
	var pods []kube.Object
	out := []PruneJob{}
	for _, j := range jobs {
		pj := PruneJob{Name: kube.Str(j, "metadata", "name"), Node: kube.Str(j, "metadata", "annotations", "nectar.dev/node"), Status: "running"}
		if t, err := time.Parse(time.RFC3339, kube.Str(j, "metadata", "creationTimestamp")); err == nil {
			pj.Started = t
		}
		switch {
		case kube.Int(j, "status", "succeeded") > 0:
			pj.Status = "succeeded"
		case kube.Int(j, "status", "failed") > 0:
			pj.Status = "failed"
		}
		for _, c := range kube.Slice(j, "status", "conditions") {
			if kube.Str(c, "type") == "Failed" && kube.Str(c, "status") == "True" {
				pj.Status, pj.Message = "failed", kube.Str(c, "message")
			}
		}
		if t, err := time.Parse(time.RFC3339, kube.Str(j, "status", "completionTime")); err == nil {
			pj.Finished = &t
		}
		if pj.Status != "running" {
			if pods == nil {
				pods, _ = k.Kube.List(ctx, "Pod", k.Namespace, pruneLabel+"=image-cleanup")
				if pods == nil {
					pods = []kube.Object{}
				}
			}
			for _, p := range pods {
				if kube.Str(p, "metadata", "labels", "job-name") != pj.Name {
					continue
				}
				logs, err := k.Kube.PodLogs(ctx, k.Namespace, kube.Str(p, "metadata", "name"), "cleanup", 2000, false)
				if err != nil {
					break
				}
				for _, l := range strings.Split(logs, "\n") {
					if strings.HasPrefix(l, "Deleted:") {
						pj.Removed++
					}
				}
				if pj.Status == "failed" && pj.Message == "" {
					pj.Message = lastLine(logs)
				}
				break
			}
		}
		out = append(out, pj)
	}
	slices.SortFunc(out, func(a, b PruneJob) int { return b.Started.Compare(a.Started) })
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
