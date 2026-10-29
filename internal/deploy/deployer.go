package deploy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/detect"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

// App statuses (apps.status).
const (
	StatusNew       = "new"
	StatusDeploying = "deploying"
	StatusRunning   = "running"
	StatusStopped   = "stopped"
	StatusFailed    = "failed"
	StatusDegraded  = "degraded" // a deploy failed but the previous release is still serving
	StatusScheduled = "scheduled"
	StatusReady     = "ready" // task apps: deployed, run on demand
)

// Kube is the subset of the Kubernetes client the deployer uses (a fake implements it in tests).
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Delete(ctx context.Context, kind, namespace, name string) error
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	ListNodes(ctx context.Context) ([]kube.Node, error)
	PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error)
	Patch(ctx context.Context, kind, namespace, name string, patch any) (kube.Object, error)
}

// Options configure the deployer.
type Options struct {
	SystemNamespace string        // "nectar-system"
	InternalURL     string        // panel URL reachable from pods, e.g. http://nectar-apps.nectar-system.svc
	SigningKey      []byte        // HMAC key for artifact download URLs
	RolloutTimeout  time.Duration // default 10m
	JobTimeout      time.Duration // release command, default 30m
	PollInterval    time.Duration // default 2s
	// OnDone is called after every release finishes (live or failed), e.g. to resync gateway routes.
	OnDone func(appID int64)
	// PullSecret returns a Docker config.json with private registry logins for app pods (nil = none).
	PullSecret func(ctx context.Context) ([]byte, error)
	// OnResult reports the outcome of every release (deploy notifications); err is nil when it went live.
	OnResult func(a store.App, rel store.Release, err error)
}

// Deployer drives releases.
type Deployer struct {
	store   *store.Store
	kube    Kube
	catalog *runtimes.Catalog
	envBox  *secretbox.Box
	log     *slog.Logger
	opt     Options

	mu      sync.Mutex
	running map[int64]bool // app id → deploy in progress
	wg      sync.WaitGroup
}

// New creates a deployer.
func New(st *store.Store, k Kube, cat *runtimes.Catalog, envBox *secretbox.Box, log *slog.Logger, opt Options) *Deployer {
	if opt.RolloutTimeout == 0 {
		opt.RolloutTimeout = 10 * time.Minute
	}
	if opt.JobTimeout == 0 {
		opt.JobTimeout = 30 * time.Minute
	}
	if opt.PollInterval == 0 {
		opt.PollInterval = 2 * time.Second
	}
	if opt.SystemNamespace == "" {
		opt.SystemNamespace = "nectar-system"
	}
	return &Deployer{store: st, kube: k, catalog: cat, envBox: envBox, log: log, opt: opt, running: map[int64]bool{}}
}

// Wait blocks until background deploys finish (tests, shutdown).
func (d *Deployer) Wait() { d.wg.Wait() }

// ---------------------------------------------------------------- projects --

// EnsureProject creates or updates a project's namespace and guard rails.
func (d *Deployer) EnsureProject(ctx context.Context, p store.Project) error {
	var ips []string
	if nodes, err := d.kube.ListNodes(ctx); err == nil {
		for _, n := range nodes {
			if n.InternalIP != "" {
				ips = append(ips, n.InternalIP)
			}
		}
	}
	var pull []byte
	if d.opt.PullSecret != nil {
		var err error
		if pull, err = d.opt.PullSecret(ctx); err != nil {
			return fmt.Errorf("registry logins: %w", err)
		}
	}
	for _, o := range ProjectObjects(ProjectSpec{Project: p, NodeIPs: ips, SystemNamespace: d.opt.SystemNamespace, PullSecret: pull}) {
		if _, err := d.kube.Apply(ctx, o); err != nil {
			return err
		}
	}
	if len(pull) == 0 {
		if err := d.kube.Delete(ctx, "Secret", p.Namespace, PullSecretName); err != nil {
			return err
		}
	}
	if p.Quota == nil || p.Quota.IsZero() {
		if err := d.kube.Delete(ctx, "ResourceQuota", p.Namespace, "quota"); err != nil {
			return err
		}
	}
	return nil
}

// DeleteProject removes a project's namespace (and with it everything inside). For an adopted
// namespace only the objects of Nectar apps are removed; the namespace and everything else stay.
func (d *Deployer) DeleteProject(ctx context.Context, p store.Project) error {
	if !p.Adopted {
		return d.kube.Delete(ctx, "Namespace", "", p.Namespace)
	}
	apps, err := d.store.ListApps(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, a := range apps {
		if err := d.DeleteApp(ctx, a, p); err != nil {
			return err
		}
	}
	for _, o := range []struct{ kind, name string }{{"ServiceAccount", AppServiceAccount}, {"ResourceQuota", "quota"}} {
		var cur kube.Object
		if err := d.kube.GetObject(ctx, o.kind, p.Namespace, o.name, &cur); err != nil {
			continue
		}
		if kube.Str(cur, "metadata", "labels", LabelManagedBy) == ManagedBy {
			if err := d.kube.Delete(ctx, o.kind, p.Namespace, o.name); err != nil {
				return err
			}
		}
	}
	_, err = d.kube.Patch(ctx, "Namespace", "", p.Namespace, map[string]any{"metadata": map[string]any{
		"labels": map[string]any{LabelProject: nil, LabelAdopted: nil}}})
	if kube.IsNotFound(err) {
		return nil
	}
	return err
}

// -------------------------------------------------------------------- apps --

// ErrBusy is returned when a deploy is already running for the app.
var ErrBusy = errors.New("a deploy is already in progress for this app")

// DeployRequest starts a release.
type DeployRequest struct {
	App        store.App
	ArtifactID *int64 // nil = reuse the artifact of the current live release (config-only deploy)
	By         *int64
	Force      bool // skip the capacity check
	// Rollback: redeploy exactly this config and image (instead of the app's current ones).
	Config *store.AppConfig
	Image  string
}

// Image returns the container image for an app (runtime version image, or the user image).
func (d *Deployer) Image(a store.App) (string, error) {
	rt, ok := d.catalog.Get(a.RuntimeID)
	if !ok {
		return "", fmt.Errorf("unknown runtime %q", a.RuntimeID)
	}
	if rt.Artifact.Kind == runtimes.ArtifactNone {
		if a.Config.Image == "" {
			return "", errors.New("this app type needs an image reference")
		}
		return a.Config.Image, nil
	}
	v, ok := rt.Version(a.RuntimeVersion)
	if !ok {
		return "", fmt.Errorf("runtime %s has no version %q", rt.ID, a.RuntimeVersion)
	}
	return v.Image, nil
}

// Deploy validates the request, creates a release and runs it in the background.
func (d *Deployer) Deploy(ctx context.Context, req DeployRequest) (store.Release, error) {
	a := req.App
	if req.Config != nil {
		a.Config = *req.Config
	}
	rt, ok := d.catalog.Get(a.RuntimeID)
	if !ok {
		return store.Release{}, fmt.Errorf("unknown runtime %q", a.RuntimeID)
	}
	image := req.Image
	if image == "" {
		var err error
		if image, err = d.Image(a); err != nil {
			return store.Release{}, err
		}
	}
	artID := req.ArtifactID
	if rt.Artifact.Kind != runtimes.ArtifactNone {
		if artID == nil { // reuse what is live, otherwise take the newest upload
			if live, err := d.store.LiveRelease(ctx, a.ID); err == nil && live.ArtifactID != nil {
				artID = live.ArtifactID
			} else if arts, err := d.store.ListArtifacts(ctx, a.ID, 1); err == nil && len(arts) > 0 {
				artID = &arts[0].ID
			}
		}
		if artID == nil {
			return store.Release{}, errors.New("upload an artifact first")
		}
		art, err := d.store.GetArtifact(ctx, *artID)
		if err != nil || art.AppID != a.ID {
			return store.Release{}, errors.New("artifact not found for this app")
		}
	} else {
		artID = nil
	}
	if a.Kind == runtimes.KindCron && strings.TrimSpace(a.Config.Schedule) == "" {
		return store.Release{}, errors.New("cron apps need a schedule")
	}
	if !req.Force {
		if err := d.CheckCapacity(ctx, a); err != nil {
			return store.Release{}, err
		}
	}

	d.mu.Lock()
	if d.running[a.ID] {
		d.mu.Unlock()
		return store.Release{}, ErrBusy
	}
	d.running[a.ID] = true
	d.mu.Unlock()

	rel, err := d.store.CreateRelease(ctx, a.ID, artID, image, a.Config, req.By)
	if err != nil {
		d.done(a.ID)
		return store.Release{}, err
	}
	_ = d.store.SetAppStatus(ctx, a.ID, StatusDeploying, fmt.Sprintf("deploying release #%d", rel.Number))
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer d.done(a.ID)
		bg, cancel := context.WithTimeout(context.Background(), d.opt.JobTimeout+d.opt.RolloutTimeout+5*time.Minute)
		defer cancel()
		d.run(bg, a, rel)
	}()
	return rel, nil
}

func (d *Deployer) done(appID int64) {
	d.mu.Lock()
	delete(d.running, appID)
	d.mu.Unlock()
}

// Busy reports whether a deploy is running for the app.
func (d *Deployer) Busy(appID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running[appID]
}

func (d *Deployer) run(ctx context.Context, a store.App, rel store.Release) {
	log := d.log.With("app", a.Slug, "release", rel.Number)
	if d.opt.OnDone != nil {
		defer d.opt.OnDone(a.ID)
	}
	_ = d.store.SetReleaseStatus(ctx, rel.ID, store.ReleaseDeploying, "")
	err := d.execute(ctx, a, rel, log)
	if d.opt.OnResult != nil {
		defer d.opt.OnResult(a, rel, err)
	}
	if err == nil {
		status := StatusRunning
		switch {
		case a.Kind == runtimes.KindTask:
			status = StatusReady
		case a.Kind == runtimes.KindCron && rel.Config.Replicas > 0:
			status = StatusScheduled
		case rel.Config.Replicas == 0:
			status = StatusStopped
		}
		_ = d.store.SetReleaseStatus(ctx, rel.ID, store.ReleaseLive, "")
		_ = d.store.SetAppStatus(ctx, a.ID, status, "")
		log.Info("release live")
		return
	}
	log.Warn("release failed", "err", err)
	_ = d.store.SetReleaseStatus(ctx, rel.ID, store.ReleaseFailed, err.Error())

	// Roll back to the previous live release so the app keeps serving the last good version.
	prev, perr := d.store.LiveRelease(ctx, a.ID)
	if perr != nil {
		_ = d.store.SetAppStatus(ctx, a.ID, StatusFailed, err.Error())
		return
	}
	if rerr := d.applyRelease(ctx, a, prev); rerr != nil {
		log.Error("rollback failed", "err", rerr)
		_ = d.store.SetAppStatus(ctx, a.ID, StatusFailed, fmt.Sprintf("deploy #%d failed: %v; rollback failed: %v", rel.Number, err, rerr))
		return
	}
	_ = d.store.SetAppStatus(ctx, a.ID, StatusDegraded, fmt.Sprintf("deploy #%d failed, still serving #%d: %v", rel.Number, prev.Number, err))
}

func (d *Deployer) execute(ctx context.Context, a store.App, rel store.Release, log *slog.Logger) error {
	spec, err := d.spec(ctx, a, rel)
	if err != nil {
		return err
	}
	if err := d.EnsureProject(ctx, spec.Project); err != nil {
		return fmt.Errorf("prepare project: %w", err)
	}
	objs, err := AppObjects(spec)
	if err != nil {
		return err
	}
	// config first (secrets/configmaps/services), workloads after the release command
	var workloads []kube.Object
	for _, o := range objs {
		kind, _, _ := kube.Meta(o)
		if kind == "Deployment" || kind == "CronJob" {
			workloads = append(workloads, o)
			continue
		}
		if err := d.apply(ctx, o); err != nil {
			return err
		}
	}
	if strings.TrimSpace(rel.Config.ReleaseCommand) != "" {
		log.Info("running release command")
		job, err := ReleaseJob(spec)
		if err != nil {
			return err
		}
		if _, err := d.kube.Apply(ctx, job); err != nil {
			return err
		}
		_, ns, name := kube.Meta(job)
		if err := d.waitJob(ctx, ns, name); err != nil {
			return fmt.Errorf("release command failed: %w", err)
		}
	}
	for _, o := range workloads {
		if _, err := d.kube.Apply(ctx, o); err != nil {
			return err
		}
	}
	if err := d.prune(ctx, spec, objs); err != nil {
		log.Warn("prune stale objects", "err", err)
	}
	if (a.Kind == runtimes.KindWeb || a.Kind == runtimes.KindWorker) && rel.Config.Replicas > 0 {
		return d.waitRollout(ctx, spec)
	}
	return nil
}

// applyRelease re-applies the objects of an existing release (rollback) without waiting.
func (d *Deployer) applyRelease(ctx context.Context, a store.App, rel store.Release) error {
	spec, err := d.spec(ctx, a, rel)
	if err != nil {
		return err
	}
	objs, err := AppObjects(spec)
	if err != nil {
		return err
	}
	for _, o := range objs {
		if err := d.apply(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

// apply applies an object. Volumes (PVCs) are only created: their spec is immutable except the size,
// which ResizeVolume changes explicitly.
func (d *Deployer) apply(ctx context.Context, o kube.Object) error {
	kind, ns, name := kube.Meta(o)
	if kind == "PersistentVolumeClaim" {
		var cur kube.Object
		err := d.kube.GetObject(ctx, kind, ns, name, &cur)
		if err == nil {
			return nil
		}
		if !kube.IsNotFound(err) {
			return err
		}
	}
	_, err := d.kube.Apply(ctx, o)
	return err
}

// spec assembles the AppSpec for a release.
func (d *Deployer) spec(ctx context.Context, a store.App, rel store.Release) (AppSpec, error) {
	rt, ok := d.catalog.Get(a.RuntimeID)
	if !ok {
		return AppSpec{}, fmt.Errorf("unknown runtime %q", a.RuntimeID)
	}
	p, err := d.store.GetProject(ctx, a.ProjectID)
	if err != nil {
		return AppSpec{}, err
	}
	env, err := d.LinkedEnv(ctx, a.ID) // add-on connection settings first: the app's own variables win
	if err != nil {
		return AppSpec{}, err
	}
	own, err := d.DecryptEnv(ctx, a.ID)
	if err != nil {
		return AppSpec{}, err
	}
	for k, v := range own {
		env[k] = v
	}
	files, err := d.DecryptFiles(ctx, a.ID)
	if err != nil {
		return AppSpec{}, err
	}
	s := AppSpec{App: a, Project: p, Runtime: rt, Image: rel.Image, Config: rel.Config, Release: rel.Number, Env: env, Files: files}
	if rel.ArtifactID != nil {
		art, err := d.store.GetArtifact(ctx, *rel.ArtifactID)
		if err != nil {
			return AppSpec{}, fmt.Errorf("artifact of release #%d: %w", rel.Number, err)
		}
		s.Artifact = &art
		s.ArtifactURL = d.ArtifactURL(art.SHA256)
		var det detect.Result
		if json.Unmarshal(art.Detect, &det) == nil && det.Archive != nil {
			s.ArchiveFormat, s.StripPrefix = det.Archive.Format, det.Archive.StripPrefix
		}
		if s.ArchiveFormat == "" {
			s.ArchiveFormat = "zip"
		}
	}
	return s, nil
}

// DecryptEnv returns the plain env vars of an app.
func (d *Deployer) DecryptEnv(ctx context.Context, appID int64) (map[string]string, error) {
	vars, err := d.store.ListEnv(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		pt, err := d.envBox.Open(v.ValueEnc)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", v.Key, err)
		}
		out[v.Key] = pt
	}
	return out, nil
}

// DecryptFiles returns the config files of an app with plain contents.
func (d *Deployer) DecryptFiles(ctx context.Context, appID int64) ([]UserFile, error) {
	files, err := d.store.ListFiles(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := make([]UserFile, 0, len(files))
	for _, f := range files {
		pt, err := d.envBox.Open(f.ContentEnc)
		if err != nil {
			return nil, fmt.Errorf("file %s: %w", f.Path, err)
		}
		out = append(out, UserFile{Path: f.Path, Content: pt})
	}
	return out, nil
}

// prune deletes objects of the app that are no longer desired (e.g. expose ports removed, kind changed).
func (d *Deployer) prune(ctx context.Context, s AppSpec, desired []kube.Object) error {
	want := map[string]bool{}
	for _, o := range desired {
		k, _, n := kube.Meta(o)
		want[k+"/"+n] = true
	}
	sel := LabelApp + "=" + s.App.Slug + "," + LabelManagedBy + "=" + ManagedBy
	for _, kind := range []string{"Deployment", "CronJob", "Service", "ConfigMap", "Secret"} {
		items, err := d.kube.List(ctx, kind, s.Project.Namespace, sel)
		if err != nil {
			return err
		}
		for _, it := range items {
			_, _, name := kube.Meta(it)
			if !want[kind+"/"+name] {
				if err := d.kube.Delete(ctx, kind, s.Project.Namespace, name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// DeleteApp removes every object of an app from the cluster.
func (d *Deployer) DeleteApp(ctx context.Context, a store.App, p store.Project) error {
	sel := LabelApp + "=" + a.Slug + "," + LabelManagedBy + "=" + ManagedBy
	// volumes go too: deleting an app deletes its data
	for _, kind := range []string{"Deployment", "CronJob", "Job", "Service", "ConfigMap", "Secret", "PersistentVolumeClaim"} {
		items, err := d.kube.List(ctx, kind, p.Namespace, sel)
		if kube.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, it := range items {
			_, _, name := kube.Meta(it)
			if kind == "PersistentVolumeClaim" {
				if err := d.releaseClaim(ctx, p.Namespace, name); err != nil {
					return err
				}
			}
			if err := d.kube.Delete(ctx, kind, p.Namespace, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunTask starts a one-off Job from the app's live release. command overrides the app command.
func (d *Deployer) RunTask(ctx context.Context, a store.App, command []string) (string, error) {
	rel, err := d.store.LiveRelease(ctx, a.ID)
	if err != nil {
		return "", errors.New("deploy the app first")
	}
	spec, err := d.spec(ctx, a, rel)
	if err != nil {
		return "", err
	}
	job, err := TaskJob(spec, strconv.FormatInt(time.Now().Unix(), 36), command)
	if err != nil {
		return "", err
	}
	if _, err := d.kube.Apply(ctx, job); err != nil {
		return "", err
	}
	_, _, name := kube.Meta(job)
	return name, nil
}

// ------------------------------------------------------------------ waiting --

func (d *Deployer) waitJob(ctx context.Context, ns, name string) error {
	deadline := time.Now().Add(d.opt.JobTimeout)
	for {
		var job kube.Object
		if err := d.kube.GetObject(ctx, "Job", ns, name, &job); err != nil {
			return err
		}
		if kube.Int(job, "status", "succeeded") > 0 {
			return nil
		}
		if kube.Int(job, "status", "failed") > 0 {
			return errors.New(d.jobFailure(ctx, ns, name))
		}
		if time.Now().After(deadline) {
			return errors.New("timed out")
		}
		if err := sleep(ctx, d.opt.PollInterval); err != nil {
			return err
		}
	}
}

func (d *Deployer) jobFailure(ctx context.Context, ns, job string) string {
	pods, err := d.kube.List(ctx, "Pod", ns, "job-name="+job)
	if err != nil || len(pods) == 0 {
		return "job failed"
	}
	_, _, pod := kube.Meta(pods[0])
	msg := "job failed"
	if r := podProblem(pods[0]); r != "" {
		msg = r
	}
	if logs, err := d.kube.PodLogs(ctx, ns, pod, "app", 15, false); err == nil && strings.TrimSpace(logs) != "" {
		msg += "\n" + strings.TrimSpace(logs)
	}
	return msg
}

// waitRollout waits until the Deployment runs the new release on all replicas.
func (d *Deployer) waitRollout(ctx context.Context, s AppSpec) error {
	deadline := time.Now().Add(d.opt.RolloutTimeout)
	ns, name := s.Project.Namespace, s.name()
	problemSince := map[string]time.Time{}
	for {
		var dep kube.Object
		if err := d.kube.GetObject(ctx, "Deployment", ns, name, &dep); err != nil {
			return err
		}
		want := kube.Int(dep, "spec", "replicas")
		if kube.Int(dep, "status", "observedGeneration") >= kube.Int(dep, "metadata", "generation") &&
			kube.Int(dep, "status", "updatedReplicas") >= want &&
			kube.Int(dep, "status", "availableReplicas") >= want &&
			kube.Int(dep, "status", "replicas") == kube.Int(dep, "status", "updatedReplicas") {
			return nil
		}
		for _, c := range kube.Slice(dep, "status", "conditions") {
			if kube.Str(c, "type") == "Progressing" && kube.Str(c, "reason") == "ProgressDeadlineExceeded" {
				return errors.New("rollout did not finish in time: " + kube.Str(c, "message"))
			}
		}
		// fail fast on problems that will not fix themselves
		pods, _ := d.kube.List(ctx, "Pod", ns, LabelApp+"="+s.App.Slug+","+LabelRelease+"="+strconv.Itoa(s.Release))
		for _, p := range pods {
			reason := podProblem(p)
			if reason == "" {
				continue
			}
			since, ok := problemSince[reason]
			if !ok {
				problemSince[reason] = time.Now()
				continue
			}
			limit := 90 * time.Second
			if strings.Contains(reason, "CrashLoopBackOff") || strings.Contains(reason, "CreateContainerConfigError") {
				limit = 30 * time.Second
			}
			if time.Since(since) >= limit {
				_, _, pod := kube.Meta(p)
				msg := reason
				if logs, err := d.kube.PodLogs(ctx, ns, pod, "app", 20, true); err == nil && strings.TrimSpace(logs) != "" {
					msg += "\nlast log lines:\n" + strings.TrimSpace(logs)
				} else if logs, err := d.kube.PodLogs(ctx, ns, pod, "fetch-artifact", 10, false); err == nil && strings.Contains(reason, "fetch-artifact") {
					msg += "\n" + strings.TrimSpace(logs)
				}
				return errors.New(msg)
			}
		}
		if time.Now().After(deadline) {
			return errors.New("rollout timed out")
		}
		if err := sleep(ctx, d.opt.PollInterval); err != nil {
			return err
		}
	}
}

// podProblem returns a human-readable reason if a pod is stuck (image pull, crash loop, …).
func podProblem(p kube.Object) string {
	check := func(statuses []kube.Object, init bool) string {
		for _, cs := range statuses {
			name := kube.Str(cs, "name")
			w := kube.Str(cs, "state", "waiting", "reason")
			switch w {
			case "ImagePullBackOff", "ErrImagePull", "InvalidImageName", "CreateContainerConfigError", "CreateContainerError":
				return fmt.Sprintf("%s: %s %s", name, w, kube.Str(cs, "state", "waiting", "message"))
			case "CrashLoopBackOff":
				last := kube.Str(cs, "lastState", "terminated", "reason")
				code := kube.Int(cs, "lastState", "terminated", "exitCode")
				return fmt.Sprintf("%s: CrashLoopBackOff (last exit: %s, code %d)", name, last, code)
			}
			if init {
				if t := kube.Str(cs, "state", "terminated", "reason"); t == "Error" {
					return fmt.Sprintf("%s: failed (exit code %d)", name, kube.Int(cs, "state", "terminated", "exitCode"))
				}
			}
			if kube.Str(cs, "state", "terminated", "reason") == "OOMKilled" || kube.Str(cs, "lastState", "terminated", "reason") == "OOMKilled" {
				return name + ": OOMKilled — the app needs more memory"
			}
		}
		return ""
	}
	if r := check(kube.Slice(p, "status", "initContainerStatuses"), true); r != "" {
		return r
	}
	if r := check(kube.Slice(p, "status", "containerStatuses"), false); r != "" {
		return r
	}
	for _, c := range kube.Slice(p, "status", "conditions") {
		if kube.Str(c, "type") == "PodScheduled" && kube.Str(c, "status") == "False" {
			return "unschedulable: " + kube.Str(c, "message")
		}
	}
	return ""
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RecoverInterrupted marks releases that were running when the panel stopped as failed.
func (d *Deployer) RecoverInterrupted(ctx context.Context) {
	for _, status := range []string{store.ReleasePending, store.ReleaseDeploying} {
		rels, err := d.store.ReleasesInStatus(ctx, status)
		if err != nil {
			d.log.Warn("recover releases", "err", err)
			return
		}
		for _, r := range rels {
			_ = d.store.SetReleaseStatus(ctx, r.ID, store.ReleaseFailed, "interrupted: the panel restarted during the deploy — deploy again")
			if _, err := d.store.LiveRelease(ctx, r.AppID); err == nil {
				_ = d.store.SetAppStatus(ctx, r.AppID, StatusDegraded, fmt.Sprintf("deploy #%d was interrupted", r.Number))
			} else {
				_ = d.store.SetAppStatus(ctx, r.AppID, StatusFailed, fmt.Sprintf("deploy #%d was interrupted", r.Number))
			}
		}
	}
}

// ---------------------------------------------------------- artifact URLs --

// ArtifactURL returns the in-cluster download URL of an artifact. The signature is not time-limited
// because pods must be able to re-fetch the artifact whenever they restart.
func (d *Deployer) ArtifactURL(sha string) string {
	return strings.TrimRight(d.opt.InternalURL, "/") + "/internal/artifacts/" + sha + "?sig=" + SignArtifact(d.opt.SigningKey, sha)
}

// SignArtifact returns the HMAC signature for an artifact hash.
func SignArtifact(key []byte, sha string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("artifact:" + sha))
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyArtifact checks a signature from ArtifactURL.
func VerifyArtifact(key []byte, sha, sig string) bool {
	want := SignArtifact(key, sha)
	return hmac.Equal([]byte(want), []byte(sig))
}
