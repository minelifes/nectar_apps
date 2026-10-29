package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// Run states.
const (
	RunPending        = "pending"
	RunRunning        = "running"
	RunSucceeded      = "succeeded"
	RunFailed         = "failed"          // stopped before anything changed
	RunRolledBack     = "rolled_back"     // changed, failed the health gate, restored
	RunRollbackFailed = "rollback_failed" // needs a person
)

// Step states.
const (
	StepPending    = "pending"
	StepRunning    = "running"
	StepDone       = "done"
	StepSkipped    = "skipped"
	StepFailed     = "failed"
	StepRolledBack = "rolled_back"
)

// Kube is the part of the Kubernetes client the updater needs.
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Patch(ctx context.Context, kind, namespace, name string, patch any) (kube.Object, error)
	Delete(ctx context.Context, kind, namespace, name string) error
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error)
}

// RunSpec is what the updater Job does (passed as JSON in NECTAR_UPDATE_RUN).
type RunSpec struct {
	ID        int64  `json:"id"`
	Namespace string `json:"namespace"`
	Steps     []Step `json:"steps"`
	// Restore maps a component to a pre-update snapshot that replaces its database before the switch
	// (rolling back over database migrations).
	Restore         map[string]string `json:"restore,omitempty"`
	HelperImage     string            `json:"helper_image"`
	PanelURL        string            `json:"panel_url"`         // http://nectar-apps.<ns>.svc
	GatewayAdminURL string            `json:"gateway_admin_url"` // http://nectar-gateway-admin.<ns>.svc:8081
	HealthTimeout   int               `json:"health_timeout"`    // seconds per step, default 180
	Trigger         string            `json:"trigger"`           // manual | auto | rollback
}

// StepStatus is the progress of a step.
type StepStatus struct {
	Step
	State    string `json:"state"`
	Snapshot string `json:"snapshot,omitempty"`
	Message  string `json:"message,omitempty"`
	Applied  bool   `json:"applied,omitempty"`
}

// LogEntry is one line of the run log.
type LogEntry struct {
	Time time.Time `json:"t"`
	Msg  string    `json:"m"`
}

// RunStatus is written by the Job into ConfigMap nectar-update-<id> and read by the panel.
type RunStatus struct {
	ID         int64        `json:"id"`
	State      string       `json:"state"`
	Steps      []StepStatus `json:"steps"`
	Log        []LogEntry   `json:"log"`
	Error      string       `json:"error,omitempty"`
	Bad        []string     `json:"bad,omitempty"` // component@version that failed the health gate
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
}

// Done reports a final state.
func (s RunStatus) Done() bool {
	return s.State != RunPending && s.State != RunRunning
}

// StatusName is the ConfigMap with the status of a run.
func StatusName(id int64) string { return "nectar-update-" + strconv.FormatInt(id, 10) }

// JobName is the updater Job of a run.
func JobName(id int64) string { return "nectar-updater-" + strconv.FormatInt(id, 10) }

// LabelRun marks the objects of a run.
const LabelRun = "nectar.dev/update-run"

// target is a component's Deployment as the updater sees it.
type target struct {
	deployment string
	container  string
	selector   string
	replicas   int64
	image      string
	dbPath     string // inside the container
	claim      string // PVC holding the database
	mountPath  string
	uid        int64
	proxyPort  int // gateway: port of PROXY_ADDR
}

// Deployments of the components.
var deployments = map[string]struct{ name, container string }{
	Panel:   {"nectar-apps", "panel"},
	Gateway: {"nectar-gateway", "gateway"},
}

// Runner executes a RunSpec inside the updater Job.
type Runner struct {
	Kube         Kube
	HTTP         *http.Client
	Spec         RunSpec
	GatewayToken string
	Log          *slog.Logger
	Poll         time.Duration // default 3 s
	Now          func() time.Time

	status RunStatus
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) poll() time.Duration {
	if r.Poll > 0 {
		return r.Poll
	}
	return 3 * time.Second
}

func (r *Runner) httpc() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (r *Runner) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if r.Log != nil {
		r.Log.Info(msg)
	}
	r.status.Log = append(r.status.Log, LogEntry{Time: r.now().UTC(), Msg: msg})
}

// save writes the status ConfigMap (best effort: the Job logs keep everything too).
func (r *Runner) save(ctx context.Context) {
	b, _ := json.Marshal(r.status)
	cm := kube.Object{"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": StatusName(r.Spec.ID), "namespace": r.Spec.Namespace,
			"labels": map[string]any{LabelRun: strconv.FormatInt(r.Spec.ID, 10), "app.kubernetes.io/part-of": "nectar"}},
		"data": map[string]any{"status": string(b)}}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	if _, err := r.Kube.Apply(sctx, cm); err != nil && r.Log != nil {
		r.Log.Warn("update: save status", "err", err)
	}
}

// Run executes the plan and returns the final status.
func (r *Runner) Run(ctx context.Context) RunStatus {
	r.status = RunStatus{ID: r.Spec.ID, State: RunRunning, StartedAt: r.now().UTC()}
	for _, s := range r.Spec.Steps {
		r.status.Steps = append(r.status.Steps, StepStatus{Step: s, State: StepPending})
	}
	r.save(ctx)
	failedAt := -1
	var failure error
	for i := range r.status.Steps {
		if err := r.step(ctx, i); err != nil {
			failedAt, failure = i, err
			break
		}
		r.save(ctx)
	}
	if failedAt < 0 {
		r.status.State = RunSucceeded
		r.logf("update finished")
	} else {
		st := &r.status.Steps[failedAt]
		st.State, st.Message = StepFailed, failure.Error()
		r.status.Error = fmt.Sprintf("%s %s: %v", st.Component, st.To, failure)
		r.logf("failed: %s", r.status.Error)
		applied := false
		for _, s := range r.status.Steps {
			applied = applied || s.Applied
		}
		if !applied {
			r.status.State = RunFailed
		} else {
			r.status.State = RunRolledBack
			if r.Spec.Trigger != "rollback" {
				r.status.Bad = append(r.status.Bad, st.Component+"@"+st.To)
			}
			for j := failedAt; j >= 0; j-- {
				if err := r.rollback(ctx, j); err != nil {
					r.status.State = RunRollbackFailed
					r.status.Error += fmt.Sprintf("; rolling back %s failed: %v", r.status.Steps[j].Component, err)
					r.logf("rollback of %s failed: %v", r.status.Steps[j].Component, err)
				}
			}
		}
	}
	t := r.now().UTC()
	r.status.FinishedAt = &t
	r.save(ctx)
	return r.status
}

func (r *Runner) timeout() time.Duration {
	if r.Spec.HealthTimeout > 0 {
		return time.Duration(r.Spec.HealthTimeout) * time.Second
	}
	return 3 * time.Minute
}

func (r *Runner) step(ctx context.Context, i int) error {
	st := &r.status.Steps[i]
	st.State = StepRunning
	r.logf("%s: %s → %s", st.Component, st.From, st.To)
	r.save(ctx)

	// pre-flight
	tg, err := r.target(ctx, st.Component)
	if err != nil {
		return err
	}
	if tg.image == st.ToImage {
		st.State, st.Message = StepSkipped, "already running "+st.To
		r.logf("%s already runs %s", st.Component, st.To)
		return nil
	}
	if st.FromImage != "" && tg.image != st.FromImage {
		r.logf("%s runs %s (expected %s); rolling back would return to the running image", st.Component, tg.image, st.FromImage)
	}
	st.FromImage = tg.image
	routes := -1
	if st.Component == Gateway {
		if n, err := r.routeCount(ctx); err == nil {
			routes = n
		} else {
			r.logf("could not list the gateway routes before the update: %v", err)
		}
	}

	// backup
	snap, err := r.helper(ctx, tg, st.Component, "snapshot", []string{"update", "snapshot", "--db", tg.dbPath,
		"--dir", path.Join(tg.mountPath, "update-backups"), "--name", fmt.Sprintf("%d-%s-%s", r.Spec.ID, st.Component, st.From), "--keep", "3"})
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	st.Snapshot = snap
	r.logf("%s database saved to %s", st.Component, snap)
	r.save(ctx)

	// apply
	st.Applied = true
	if from := r.Spec.Restore[st.Component]; from != "" {
		if err := r.swapWithRestore(ctx, tg, st.ToImage, from); err != nil {
			return err
		}
	} else if err := r.setImage(ctx, tg, st.ToImage, nil); err != nil {
		return err
	}
	r.logf("%s image set to %s", st.Component, st.ToImage)
	r.save(ctx)

	// health gate
	if err := r.health(ctx, tg, st.Component, st.To, routes); err != nil {
		return err
	}
	st.State = StepDone
	r.logf("%s %s is healthy", st.Component, st.To)
	return nil
}

func (r *Runner) rollback(ctx context.Context, j int) error {
	st := &r.status.Steps[j]
	if !st.Applied || st.State == StepRolledBack {
		return nil
	}
	tg, err := r.target(ctx, st.Component)
	if err != nil {
		return err
	}
	r.logf("rolling back %s to %s", st.Component, st.From)
	restore := ""
	if st.Snapshot != "" && (st.Migrations || r.Spec.Restore[st.Component] != "") {
		restore = st.Snapshot
	}
	if restore != "" {
		err = r.swapWithRestore(ctx, tg, st.FromImage, restore)
	} else {
		err = r.setImage(ctx, tg, st.FromImage, nil)
	}
	if err != nil {
		return err
	}
	if err := r.health(ctx, tg, st.Component, st.From, -1); err != nil {
		return err
	}
	st.State = StepRolledBack
	r.logf("%s is back on %s", st.Component, st.From)
	r.save(ctx)
	return nil
}

// target reads a component's Deployment.
func (r *Runner) target(ctx context.Context, component string) (target, error) {
	d, ok := deployments[component]
	if !ok {
		return target{}, fmt.Errorf("unknown component %q", component)
	}
	var dep kube.Object
	if err := r.Kube.GetObject(ctx, "Deployment", r.Spec.Namespace, d.name, &dep); err != nil {
		return target{}, fmt.Errorf("deployment %s: %w", d.name, err)
	}
	tg := target{deployment: d.name, container: d.container, replicas: kube.Int(dep, "spec", "replicas"), uid: 65532}
	if tg.replicas < 1 {
		tg.replicas = 1
	}
	if sel, ok := kube.Field(dep, "spec", "selector", "matchLabels").(map[string]any); ok {
		var parts []string
		for k, v := range sel {
			parts = append(parts, k+"="+fmt.Sprint(v))
		}
		sort.Strings(parts)
		tg.selector = strings.Join(parts, ",")
	}
	var mounts []kube.Object
	env := map[string]string{}
	for _, c := range kube.Slice(dep, "spec", "template", "spec", "containers") {
		if kube.Str(c, "name") != d.container {
			continue
		}
		tg.image = kube.Str(c, "image")
		mounts = kube.Slice(c, "volumeMounts")
		for _, e := range kube.Slice(c, "env") {
			env[kube.Str(e, "name")] = kube.Str(e, "value")
		}
	}
	if tg.image == "" {
		return target{}, fmt.Errorf("deployment %s has no container %q", d.name, d.container)
	}
	switch component {
	case Panel:
		dir := env["NECTAR_DATA_DIR"]
		if dir == "" {
			dir = "/data"
		}
		tg.dbPath = path.Join(dir, "nectar.db")
	case Gateway:
		tg.uid = 0 // the gateway runs as root (it binds 80/443) and owns its files
		tg.dbPath = env["DB_PATH"]
		if tg.dbPath == "" {
			tg.dbPath = "/data/nectar.db"
		}
		tg.proxyPort = 80
		if a := env["PROXY_ADDR"]; a != "" {
			if i := strings.LastIndexByte(a, ':'); i >= 0 {
				if p, err := strconv.Atoi(a[i+1:]); err == nil {
					tg.proxyPort = p
				}
			}
		}
	}
	if sc, ok := kube.Field(dep, "spec", "template", "spec", "securityContext", "runAsUser").(float64); ok {
		tg.uid = int64(sc)
	}
	volumes := map[string]string{}
	for _, v := range kube.Slice(dep, "spec", "template", "spec", "volumes") {
		if c := kube.Str(v, "persistentVolumeClaim", "claimName"); c != "" {
			volumes[kube.Str(v, "name")] = c
		}
	}
	for _, m := range mounts {
		mp := kube.Str(m, "mountPath")
		if claim := volumes[kube.Str(m, "name")]; claim != "" && strings.HasPrefix(tg.dbPath, strings.TrimRight(mp, "/")+"/") {
			tg.claim, tg.mountPath = claim, mp
		}
	}
	if tg.claim == "" {
		return target{}, fmt.Errorf("deployment %s: no volume holds %s", d.name, tg.dbPath)
	}
	return tg, nil
}

func (r *Runner) setImage(ctx context.Context, tg target, image string, replicas *int64) error {
	spec := map[string]any{"template": map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{"nectar.dev/update-run": strconv.FormatInt(r.Spec.ID, 10)}},
		"spec":     map[string]any{"containers": []any{map[string]any{"name": tg.container, "image": image}}}}}
	if replicas != nil {
		spec["replicas"] = *replicas
	}
	// strategic merge on containers by name: the client sends a merge patch, so send the full container list
	var dep kube.Object
	if err := r.Kube.GetObject(ctx, "Deployment", r.Spec.Namespace, tg.deployment, &dep); err != nil {
		return err
	}
	var containers []any
	for _, c := range kube.Slice(dep, "spec", "template", "spec", "containers") {
		if kube.Str(c, "name") == tg.container {
			c["image"] = image
		}
		containers = append(containers, c)
	}
	spec["template"].(map[string]any)["spec"] = map[string]any{"containers": containers}
	_, err := r.Kube.Patch(ctx, "Deployment", r.Spec.Namespace, tg.deployment, map[string]any{"spec": spec})
	return err
}

func (r *Runner) scale(ctx context.Context, tg target, n int64) error {
	_, err := r.Kube.Patch(ctx, "Deployment", r.Spec.Namespace, tg.deployment, map[string]any{"spec": map[string]any{"replicas": n}})
	return err
}

// swapWithRestore stops the component, puts a snapshot back and starts it with image.
func (r *Runner) swapWithRestore(ctx context.Context, tg target, image, snapshot string) error {
	r.logf("stopping %s to restore %s", tg.deployment, snapshot)
	if err := r.scale(ctx, tg, 0); err != nil {
		return err
	}
	if err := r.waitPodsGone(ctx, tg); err != nil {
		return err
	}
	if _, err := r.helper(ctx, tg, componentOf(tg), "restore", []string{"update", "restore", "--db", tg.dbPath, "--from", snapshot}); err != nil {
		// start it again as it was, the database is untouched when the copy failed
		_ = r.scale(ctx, tg, tg.replicas)
		return fmt.Errorf("restore: %w", err)
	}
	n := tg.replicas
	return r.setImage(ctx, tg, image, &n)
}

func componentOf(tg target) string {
	for c, d := range deployments {
		if d.name == tg.deployment {
			return c
		}
	}
	return tg.deployment
}

func (r *Runner) waitPodsGone(ctx context.Context, tg target) error {
	deadline := r.now().Add(3 * time.Minute)
	for {
		pods, err := r.Kube.List(ctx, "Pod", r.Spec.Namespace, tg.selector)
		if err != nil {
			return err
		}
		if len(pods) == 0 {
			return nil
		}
		if r.now().After(deadline) {
			return fmt.Errorf("%s did not stop within 3 minutes", tg.deployment)
		}
		if err := r.sleep(ctx); err != nil {
			return err
		}
	}
}

func (r *Runner) sleep(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(r.poll()):
		return nil
	}
}

// helper runs `nectar-apps <args>` in a pod that mounts the component's volume (it lands on the volume's node)
// and returns the value after "result: " in its output.
func (r *Runner) helper(ctx context.Context, tg target, component, op string, args []string) (string, error) {
	name := fmt.Sprintf("nectar-update-%d-%s-%s", r.Spec.ID, component, op)
	_ = r.Kube.Delete(ctx, "Pod", r.Spec.Namespace, name)
	sc := map[string]any{"runAsUser": tg.uid, "runAsGroup": tg.uid, "seccompProfile": map[string]any{"type": "RuntimeDefault"}}
	if tg.uid != 0 {
		sc["runAsNonRoot"] = true
	}
	pod := kube.Object{"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": name, "namespace": r.Spec.Namespace,
			"labels": map[string]any{LabelRun: strconv.FormatInt(r.Spec.ID, 10), "app.kubernetes.io/part-of": "nectar"}},
		"spec": map[string]any{
			"restartPolicy":                "Never",
			"automountServiceAccountToken": false,
			"securityContext":              sc,
			"containers": []any{map[string]any{"name": "helper", "image": r.Spec.HelperImage, "imagePullPolicy": "IfNotPresent", "args": args,
				"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []any{"ALL"}}},
				"volumeMounts":    []any{map[string]any{"name": "data", "mountPath": tg.mountPath}}}},
			"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": tg.claim}}},
		}}
	if _, err := r.Kube.Apply(ctx, pod); err != nil {
		return "", err
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		_ = r.Kube.Delete(dctx, "Pod", r.Spec.Namespace, name)
		cancel()
	}()
	deadline := r.now().Add(5 * time.Minute)
	for {
		var p kube.Object
		if err := r.Kube.GetObject(ctx, "Pod", r.Spec.Namespace, name, &p); err != nil {
			return "", err
		}
		phase := kube.Str(p, "status", "phase")
		if phase == "Succeeded" || phase == "Failed" {
			logs, _ := r.Kube.PodLogs(ctx, r.Spec.Namespace, name, "helper", 50, false)
			if phase == "Failed" {
				return "", fmt.Errorf("%s", lastLine(logs, "the helper pod failed"))
			}
			for _, l := range strings.Split(logs, "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(l), "result: "); ok {
					return v, nil
				}
			}
			return "", nil
		}
		for _, cs := range kube.Slice(p, "status", "containerStatuses") {
			if reason := kube.Str(cs, "state", "waiting", "reason"); reason == "ErrImagePull" || reason == "ImagePullBackOff" || reason == "CreateContainerConfigError" {
				return "", fmt.Errorf("helper pod: %s: %s", reason, kube.Str(cs, "state", "waiting", "message"))
			}
		}
		if r.now().After(deadline) {
			return "", errors.New("the helper pod did not finish within 5 minutes (is the volume attachable?)")
		}
		if err := r.sleep(ctx); err != nil {
			return "", err
		}
	}
}

func lastLine(s, def string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return def
}

// health waits until the rollout finished and the component answers as the expected version.
func (r *Runner) health(ctx context.Context, tg target, component, want string, routesBefore int) error {
	deadline := r.now().Add(r.timeout())
	var last error
	for {
		last = r.check(ctx, tg, component, want, routesBefore)
		if last == nil {
			return nil
		}
		if r.now().After(deadline) {
			return fmt.Errorf("health check: %w", last)
		}
		if err := r.sleep(ctx); err != nil {
			return err
		}
	}
}

func (r *Runner) check(ctx context.Context, tg target, component, want string, routesBefore int) error {
	var dep kube.Object
	if err := r.Kube.GetObject(ctx, "Deployment", r.Spec.Namespace, tg.deployment, &dep); err != nil {
		return err
	}
	replicas := kube.Int(dep, "spec", "replicas")
	if kube.Int(dep, "status", "observedGeneration") < kube.Int(dep, "metadata", "generation") {
		return errors.New("the rollout has not started")
	}
	if up := kube.Int(dep, "status", "updatedReplicas"); up < replicas {
		return fmt.Errorf("%d of %d pods updated", up, replicas)
	}
	if av := kube.Int(dep, "status", "availableReplicas"); av < replicas {
		return fmt.Errorf("%d of %d pods ready", av, replicas)
	}
	if total := kube.Int(dep, "status", "replicas"); total > replicas {
		return errors.New("old pods are still running")
	}
	// pods stuck pulling the image never become ready: fail fast with the reason
	pods, _ := r.Kube.List(ctx, "Pod", r.Spec.Namespace, tg.selector)
	for _, p := range pods {
		for _, cs := range kube.Slice(p, "status", "containerStatuses") {
			if reason := kube.Str(cs, "state", "waiting", "reason"); reason == "ErrImagePull" || reason == "ImagePullBackOff" || reason == "CrashLoopBackOff" {
				return fmt.Errorf("%s: %s", reason, kube.Str(cs, "state", "waiting", "message"))
			}
		}
	}
	base := r.Spec.PanelURL
	if component == Gateway {
		base = r.Spec.GatewayAdminURL
	}
	if base != "" {
		var v struct {
			Version string `json:"version"`
		}
		if err := r.getJSON(ctx, strings.TrimRight(base, "/")+"/api/version", "", &v); err != nil {
			return fmt.Errorf("version: %w", err)
		}
		if a, ok := ParseSemver(v.Version); ok {
			if b, ok := ParseSemver(want); ok && a.Compare(b) != 0 {
				return fmt.Errorf("it reports version %s, not %s", v.Version, want)
			}
		}
	}
	if component == Gateway {
		if err := r.canary(ctx, tg); err != nil {
			return fmt.Errorf("canary: %w", err)
		}
		if routesBefore >= 0 {
			n, err := r.routeCount(ctx)
			if err != nil {
				return fmt.Errorf("routes: %w", err)
			}
			if n < routesBefore {
				return fmt.Errorf("the gateway has %d routes, before the update it had %d", n, routesBefore)
			}
		}
	}
	return nil
}

func (r *Runner) canary(ctx context.Context, tg target) error {
	pods, err := r.Kube.List(ctx, "Pod", r.Spec.Namespace, tg.selector)
	if err != nil {
		return err
	}
	tried := 0
	for _, p := range pods {
		ip := kube.Str(p, "status", "hostIP")
		if ip == "" || kube.Str(p, "metadata", "deletionTimestamp") != "" {
			continue
		}
		tried++
		u := "http://" + ip + ":" + strconv.Itoa(tg.proxyPort) + "/__nectar/canary"
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := r.httpc().Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s answered %d", u, resp.StatusCode)
		}
		return nil
	}
	if tried == 0 {
		return errors.New("no gateway pod with a node address")
	}
	return nil
}

func (r *Runner) routeCount(ctx context.Context) (int, error) {
	if r.Spec.GatewayAdminURL == "" {
		return -1, errors.New("no gateway admin URL")
	}
	var routes []json.RawMessage
	if err := r.getJSON(ctx, strings.TrimRight(r.Spec.GatewayAdminURL, "/")+"/api/routes", r.GatewayToken, &routes); err != nil {
		return 0, err
	}
	return len(routes), nil
}

func (r *Runner) getJSON(ctx context.Context, url, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := r.httpc().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}
