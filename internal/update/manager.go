package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// Settings rows.
const (
	StateKey = "updates_state" // last check (read by the alerts engine and the UI)
	BadKey   = "updates_bad"   // component@version → why it is not installed automatically again
)

// ComponentState is what the last check found for a component.
type ComponentState struct {
	Installed Installed `json:"installed"`
	Available []Release `json:"available"`         // newer releases in the channel, newest first
	Latest    *Release  `json:"latest,omitempty"`  // newest release in the channel (may be the installed one)
	Current   *Release  `json:"current,omitempty"` // release entry of the installed version
	Auto      *Release  `json:"auto,omitempty"`    // what the auto-update policy would install
	Waiting   string    `json:"waiting,omitempty"` // why an available update is not installed automatically now
	Source    string    `json:"source,omitempty"`  // feed | github | k3s channel
	Error     string    `json:"error,omitempty"`   // last check failed
	Extra     any       `json:"extra,omitempty"`   // k3s: upgrade controller state
	CheckedAt time.Time `json:"checked_at"`
}

// State is the result of the last check.
type State struct {
	CheckedAt  time.Time                  `json:"checked_at"`
	Components map[string]*ComponentState `json:"components"`
}

// GatewayVersioner reports the running gateway version (the gateway client adapted in main).
type GatewayVersioner interface {
	Version(ctx context.Context) (version string, api int, err error)
}

// Manager checks for updates, plans them and starts and tracks updater Jobs.
type Manager struct {
	Store     *store.Store
	Kube      Kube             // nil without Kubernetes
	Gateway   GatewayVersioner // nil without a gateway
	Fetcher   *Fetcher
	Log       *slog.Logger
	Namespace string

	PanelVersion    string
	PanelAPI        int
	PanelRequires   Requires // what this build needs from the gateway
	PanelURL        string   // http://nectar-apps.<ns>.svc (health gate)
	GatewayAdminURL string
	ServiceAccount  string // of the updater Job, default nectar-apps
	SecretName      string // holds gateway-api-token, default nectar-secrets

	// Notify announces finished runs (alerts channels); may be nil.
	Notify func(ctx context.Context, title, message string, ok bool)
	Now    func() time.Time
	Poll   time.Duration // run tracking, default 5 s

	mu       sync.Mutex
	checking bool
	tracking map[int64]bool
	lastAuto time.Time
	running  sync.WaitGroup
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) poll() time.Duration {
	if m.Poll > 0 {
		return m.Poll
	}
	return 5 * time.Second
}

// Wait waits for tracking goroutines (tests).
func (m *Manager) Wait() { m.running.Wait() }

// Settings loads the configuration.
func (m *Manager) Settings(ctx context.Context) (Settings, error) {
	var s Settings
	if err := m.Store.GetSetting(ctx, SettingsKey, &s); err != nil && !errors.Is(err, store.ErrNotFound) {
		return Defaults, err
	}
	return s.Normalize(), nil
}

// SaveSettings stores the configuration (and re-applies the k3s plans).
func (m *Manager) SaveSettings(ctx context.Context, s Settings) (Settings, error) {
	s = s.Normalize()
	if err := s.Validate(); err != nil {
		return s, err
	}
	if err := m.Store.SetSetting(ctx, SettingsKey, s); err != nil {
		return s, err
	}
	if m.Kube != nil {
		if err := m.SyncK3s(ctx, s, ""); err != nil {
			return s, fmt.Errorf("saved, but the k3s upgrade plans could not be updated: %w", err)
		}
	}
	return s, nil
}

// State returns the last check.
func (m *Manager) State(ctx context.Context) State {
	var st State
	if err := m.Store.GetSetting(ctx, StateKey, &st); err != nil || st.Components == nil {
		st.Components = map[string]*ComponentState{}
	}
	return st
}

// Bad returns the releases that failed (component@version → reason).
func (m *Manager) Bad(ctx context.Context) map[string]string {
	bad := map[string]string{}
	_ = m.Store.GetSetting(ctx, BadKey, &bad)
	return bad
}

// ClearBad allows a release to be installed automatically again.
func (m *Manager) ClearBad(ctx context.Context, key string) error {
	bad := m.Bad(ctx)
	delete(bad, key)
	return m.Store.SetSetting(ctx, BadKey, bad)
}

func (m *Manager) markBad(ctx context.Context, keys []string, why string) {
	if len(keys) == 0 {
		return
	}
	bad := m.Bad(ctx)
	for _, k := range keys {
		bad[k] = why
	}
	_ = m.Store.SetSetting(ctx, BadKey, bad)
}

func (m *Manager) source(set Settings, component string) Source {
	src := DefaultSources[component]
	if u := set.Component(component).FeedURL; u != "" {
		src.FeedURL = u
	}
	return src
}

// deploymentImage returns the image of a component's container.
func (m *Manager) deploymentImage(ctx context.Context, component string) string {
	if m.Kube == nil {
		return ""
	}
	d := deployments[component]
	var dep kube.Object
	if err := m.Kube.GetObject(ctx, "Deployment", m.Namespace, d.name, &dep); err != nil {
		return ""
	}
	for _, c := range kube.Slice(dep, "spec", "template", "spec", "containers") {
		if kube.Str(c, "name") == d.container {
			return kube.Str(c, "image")
		}
	}
	return ""
}

// Check refreshes the state of every component (it is cheap: two small JSON files and the k3s channel).
func (m *Manager) Check(ctx context.Context) (State, error) {
	m.mu.Lock()
	if m.checking {
		m.mu.Unlock()
		return m.State(ctx), errors.New("a check is already running")
	}
	m.checking = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.checking = false; m.mu.Unlock() }()

	set, err := m.Settings(ctx)
	if err != nil {
		return State{}, err
	}
	now := m.now().UTC()
	st := State{CheckedAt: now, Components: map[string]*ComponentState{}}
	prev := m.State(ctx)
	bad := m.Bad(ctx)

	// panel
	pc := &ComponentState{CheckedAt: now, Installed: Installed{Version: m.PanelVersion, Image: m.deploymentImage(ctx, Panel),
		APIVersion: m.PanelAPI, Requires: m.PanelRequires}}
	st.Components[Panel] = pc
	// gateway
	gc := &ComponentState{CheckedAt: now}
	st.Components[Gateway] = gc
	if m.Gateway != nil {
		vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		v, api, err := m.Gateway.Version(vctx)
		cancel()
		if err != nil {
			gc.Error = "the gateway did not answer: " + err.Error()
		}
		gc.Installed = Installed{Version: v, APIVersion: api, Image: m.deploymentImage(ctx, Gateway)}
	} else {
		gc.Error = "no gateway is configured"
	}

	for _, comp := range []string{Panel, Gateway} {
		cs := st.Components[comp]
		cfg := set.Component(comp)
		if cfg.Policy == PolicyOff {
			cs.Waiting = "update checks are off"
			continue
		}
		if comp == Gateway && cs.Installed.Version == "" {
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		list, src, err := m.Fetcher.Releases(fctx, comp, cfg.Channel, m.source(set, comp))
		cancel()
		cs.Source = src
		if err != nil {
			cs.Error = strings.TrimSpace(cs.Error + " " + err.Error())
			if p := prev.Components[comp]; p != nil { // keep what we knew
				cs.Available, cs.Latest, cs.Current = p.Available, p.Latest, p.Current
			}
			continue
		}
		cur, curOK := ParseSemver(cs.Installed.Version)
		for i := range list {
			r := list[i]
			if i == 0 {
				cs.Latest = &r
			}
			if r.Version == cs.Installed.Version || (curOK && r.Semver().Compare(cur) == 0) {
				cs.Current = &r
			}
			if !curOK || cur.Less(r.Semver()) {
				if len(cs.Available) < 20 {
					cs.Available = append(cs.Available, r)
				}
			}
		}
		if !curOK {
			cs.Waiting = "this is a development build (" + cs.Installed.Version + "): install a release to get updates"
			cs.Available = nil
		}
		if comp == Gateway && cs.Current != nil {
			cs.Installed.Requires = cs.Current.Requires
		}
		// what auto-update would pick
		if curOK && (cfg.Policy == PolicyPatch || cfg.Policy == PolicyMinor) {
			for i := range cs.Available {
				r := cs.Available[i]
				if bad[comp+"@"+r.Version] != "" {
					continue
				}
				if AutoInstall(cfg.Policy, cur, r) {
					cs.Auto = &r
					break
				}
			}
			if cs.Auto == nil && len(cs.Available) > 0 {
				top := cs.Available[0]
				switch {
				case bad[comp+"@"+top.Version] != "":
					cs.Waiting = top.Version + " failed before: " + bad[comp+"@"+top.Version]
				case top.Breaking || cur.Bump(top.Semver()) == "major":
					cs.Waiting = top.Version + " is a major or breaking release: install it by hand after reading the notes"
				default:
					cs.Waiting = top.Version + " is not covered by the policy \"" + cfg.Policy + "\""
				}
			}
		}
	}

	// k3s
	st.Components[K3s] = m.checkK3s(ctx, set, now)

	if err := m.Store.SetSetting(ctx, StateKey, st); err != nil {
		return st, err
	}
	return st, nil
}

// PlanRequest asks for versions ("" = keep the component as it is).
type PlanRequest struct {
	Panel   string `json:"panel,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}

func findRelease(cs *ComponentState, version string) (*Release, error) {
	if cs == nil {
		return nil, errors.New("check for updates first")
	}
	for i := range cs.Available {
		if cs.Available[i].Version == version {
			r := cs.Available[i]
			return &r, nil
		}
	}
	if cs.Latest != nil && cs.Latest.Version == version {
		r := *cs.Latest
		return &r, nil
	}
	return nil, fmt.Errorf("version %s is not in the release list (check for updates, or switch the channel)", version)
}

// Plan builds a plan from the last check; Complete adds the other component when needed.
func (m *Manager) Plan(ctx context.Context, req PlanRequest, complete bool) (Plan, error) {
	st := m.State(ctx)
	pc, gc := st.Components[Panel], st.Components[Gateway]
	if pc == nil || gc == nil {
		return Plan{}, errors.New("check for updates first")
	}
	in := PlanInput{Panel: pc.Installed, Gateway: gc.Installed, PanelAPI: pc.Installed.APIVersion, GatewayAPI: gc.Installed.APIVersion,
		PanelReleases: pc.Available, GatewayReleases: gc.Available, Complete: complete, Skip: map[string]bool{}}
	in.Panel.Requires = m.PanelRequires
	if gc.Current != nil {
		in.Gateway.Requires = gc.Current.Requires
	}
	for k := range m.Bad(ctx) {
		in.Skip[k] = true
	}
	var err error
	if req.Panel != "" {
		if in.PanelTo, err = findRelease(pc, req.Panel); err != nil {
			return Plan{}, fmt.Errorf("panel: %w", err)
		}
	}
	if req.Gateway != "" {
		if gc.Installed.Version == "" {
			return Plan{}, errors.New("gateway: its version is unknown (is it reachable?)")
		}
		if in.GatewayTo, err = findRelease(gc, req.Gateway); err != nil {
			return Plan{}, fmt.Errorf("gateway: %w", err)
		}
	}
	return MakePlan(in)
}

// Summary is a one-line description of a plan.
func Summary(p Plan) string {
	var parts []string
	for _, s := range p.Steps {
		parts = append(parts, fmt.Sprintf("%s %s → %s", s.Component, s.From, s.To))
	}
	return strings.Join(parts, ", ")
}

// ErrBusy means another update is running.
var ErrBusy = errors.New("an update is already running")

// Start creates a run and its updater Job.
func (m *Manager) Start(ctx context.Context, plan Plan, trigger string, restore map[string]string, userID *int64) (store.UpdateRun, error) {
	if m.Kube == nil {
		return store.UpdateRun{}, errors.New("updates need the panel to run in Kubernetes")
	}
	if len(plan.Steps) == 0 {
		return store.UpdateRun{}, errors.New("nothing to update")
	}
	if active, err := m.Store.ActiveUpdateRuns(ctx); err != nil {
		return store.UpdateRun{}, err
	} else if len(active) > 0 {
		return store.UpdateRun{}, ErrBusy
	}
	jobImage := m.deploymentImage(ctx, Panel)
	if jobImage == "" {
		return store.UpdateRun{}, fmt.Errorf("deployment nectar-apps not found in %s", m.Namespace)
	}
	for _, s := range plan.Steps {
		// a newer panel runs the update with its own (newer) updater
		if s.Component == Panel && trigger != "rollback" {
			jobImage = s.ToImage
		}
	}
	pj, _ := json.Marshal(plan)
	run, err := m.Store.CreateUpdateRun(ctx, store.UpdateRun{Trigger: trigger, State: RunPending, Summary: Summary(plan), Plan: pj, UserID: userID})
	if err != nil {
		return run, err
	}
	spec := RunSpec{ID: run.ID, Namespace: m.Namespace, Steps: plan.Steps, Restore: restore, HelperImage: jobImage,
		PanelURL: m.PanelURL, GatewayAdminURL: m.GatewayAdminURL, Trigger: trigger}
	if err := m.createJob(ctx, spec, jobImage); err != nil {
		_ = m.Store.SaveUpdateRun(ctx, run.ID, RunFailed, json.RawMessage("{}"), "could not start the updater: "+err.Error(), true)
		return run, err
	}
	run.State = RunRunning
	_ = m.Store.SaveUpdateRun(ctx, run.ID, RunRunning, json.RawMessage("{}"), "", false)
	m.track(run.ID)
	return run, nil
}

func (m *Manager) createJob(ctx context.Context, spec RunSpec, image string) error {
	sa, secret := m.ServiceAccount, m.SecretName
	if sa == "" {
		sa = "nectar-apps"
	}
	if secret == "" {
		secret = "nectar-secrets"
	}
	sj, _ := json.Marshal(spec)
	id := strconv.FormatInt(spec.ID, 10)
	job := kube.Object{"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": JobName(spec.ID), "namespace": m.Namespace,
			"labels": map[string]any{LabelRun: id, "app.kubernetes.io/name": "nectar-updater", "app.kubernetes.io/part-of": "nectar"}},
		"spec": map[string]any{
			"backoffLimit":            0,
			"activeDeadlineSeconds":   3600,
			"ttlSecondsAfterFinished": 7 * 24 * 3600,
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{LabelRun: id, "app.kubernetes.io/name": "nectar-updater"}},
				"spec": map[string]any{
					"serviceAccountName": sa,
					"restartPolicy":      "Never",
					"priorityClassName":  "system-cluster-critical",
					"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532, "runAsGroup": 65532,
						"seccompProfile": map[string]any{"type": "RuntimeDefault"}},
					"containers": []any{map[string]any{
						"name": "updater", "image": image, "imagePullPolicy": "IfNotPresent",
						"args": []any{"update", "run"},
						"env": []any{
							map[string]any{"name": "NECTAR_UPDATE_RUN", "value": string(sj)},
							map[string]any{"name": "NECTAR_GATEWAY_TOKEN", "valueFrom": map[string]any{"secretKeyRef": map[string]any{
								"name": secret, "key": "gateway-api-token", "optional": true}}},
						},
						"resources": map[string]any{"requests": map[string]any{"cpu": "50m", "memory": "32Mi"}, "limits": map[string]any{"memory": "256Mi"}},
						"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true,
							"capabilities": map[string]any{"drop": []any{"ALL"}}},
					}},
				},
			},
		}}
	_, err := m.Kube.Apply(ctx, job)
	return err
}

// Rollback returns the components of the newest successful run to the versions they had before (restoring
// the databases of steps that ran migrations; changes made since are lost for those).
func (m *Manager) Rollback(ctx context.Context, runID int64, userID *int64) (store.UpdateRun, error) {
	var src store.UpdateRun
	var err error
	if runID > 0 {
		src, err = m.Store.GetUpdateRun(ctx, runID)
		if err != nil {
			return src, err
		}
	} else {
		runs, err := m.Store.ListUpdateRuns(ctx, 50)
		if err != nil {
			return src, err
		}
		for _, r := range runs {
			if r.State == RunSucceeded && r.Trigger != "rollback" {
				src = r
				break
			}
		}
		if src.ID == 0 {
			return src, errors.New("there is no update to roll back")
		}
	}
	if src.State != RunSucceeded {
		return src, errors.New("only a successful update can be rolled back")
	}
	var status RunStatus
	if err := json.Unmarshal(src.Status, &status); err != nil || len(status.Steps) == 0 {
		return src, errors.New("this run has no recorded steps")
	}
	var plan Plan
	restore := map[string]string{}
	for i := len(status.Steps) - 1; i >= 0; i-- {
		s := status.Steps[i]
		if s.State != StepDone {
			continue
		}
		if cur := m.deploymentImage(ctx, s.Component); cur != "" && cur != s.ToImage {
			return src, fmt.Errorf("%s was changed after this update (it runs %s now)", s.Component, cur)
		}
		plan.Steps = append(plan.Steps, Step{Component: s.Component, From: s.To, To: s.From, FromImage: s.ToImage, ToImage: s.FromImage})
		if s.Migrations && s.Snapshot != "" {
			restore[s.Component] = s.Snapshot
			plan.Notes = append(plan.Notes, fmt.Sprintf("The %s database goes back to its state before the update (%s); changes made since are lost.", s.Component, src.StartedAt.Format(time.RFC3339)))
		}
	}
	if len(plan.Steps) == 0 {
		return src, errors.New("this run changed nothing")
	}
	return m.Start(ctx, plan, "rollback", restore, userID)
}

// track follows a run's ConfigMap until it is final.
func (m *Manager) track(id int64) {
	m.mu.Lock()
	if m.tracking == nil {
		m.tracking = map[int64]bool{}
	}
	if m.tracking[id] {
		m.mu.Unlock()
		return
	}
	m.tracking[id] = true
	m.mu.Unlock()
	m.running.Add(1)
	go func() {
		defer m.running.Done()
		defer func() { m.mu.Lock(); delete(m.tracking, id); m.mu.Unlock() }()
		ctx := context.Background()
		missing := 0
		for {
			done, gone := m.sync(ctx, id)
			if done {
				return
			}
			if gone {
				missing++
			} else {
				missing = 0
			}
			if missing > 24 { // two minutes without Job and status
				_ = m.Store.SaveUpdateRun(ctx, id, RunFailed, json.RawMessage("{}"), "the updater Job disappeared", true)
				return
			}
			time.Sleep(m.poll())
		}
	}()
}

// sync copies the status of a run from its ConfigMap; it reports whether the run is final and whether
// neither the Job nor the status exist.
func (m *Manager) sync(ctx context.Context, id int64) (done, gone bool) {
	run, err := m.Store.GetUpdateRun(ctx, id)
	if err != nil || run.FinishedAt != nil {
		return true, false
	}
	var cm kube.Object
	cmErr := m.Kube.GetObject(ctx, "ConfigMap", m.Namespace, StatusName(id), &cm)
	var job kube.Object
	jobErr := m.Kube.GetObject(ctx, "Job", m.Namespace, JobName(id), &job)
	jobFailed := jobErr == nil && (kube.Int(job, "status", "failed") > 0 || condition(job, "Failed"))
	if cmErr != nil {
		if jobFailed {
			logs := m.jobLogs(ctx, id)
			_ = m.Store.SaveUpdateRun(ctx, id, RunFailed, json.RawMessage("{}"), "the updater failed to start: "+logs, true)
			m.finished(ctx, run, RunStatus{State: RunFailed, Error: logs})
			return true, false
		}
		return false, kube.IsNotFound(cmErr) && kube.IsNotFound(jobErr)
	}
	raw := kube.Str(cm, "data", "status")
	var status RunStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return false, false
	}
	if !status.Done() && jobFailed {
		status.State, status.Error = RunRollbackFailed, "the updater Job ended in the middle of the update: "+m.jobLogs(ctx, id)
		if !anyApplied(status) {
			status.State = RunFailed
		}
		b, _ := json.Marshal(status)
		raw = string(b)
	}
	if err := m.Store.SaveUpdateRun(ctx, id, status.State, json.RawMessage(raw), status.Error, status.Done()); err != nil {
		return false, false
	}
	if status.Done() {
		m.finished(ctx, run, status)
		return true, false
	}
	return false, false
}

func anyApplied(s RunStatus) bool {
	for _, st := range s.Steps {
		if st.Applied {
			return true
		}
	}
	return false
}

func condition(o kube.Object, typ string) bool {
	for _, c := range kube.Slice(o, "status", "conditions") {
		if kube.Str(c, "type") == typ && kube.Str(c, "status") == "True" {
			return true
		}
	}
	return false
}

func (m *Manager) jobLogs(ctx context.Context, id int64) string {
	pods, err := m.Kube.List(ctx, "Pod", m.Namespace, "job-name="+JobName(id))
	if err != nil || len(pods) == 0 {
		return "no logs"
	}
	_, _, name := kube.Meta(pods[0])
	logs, err := m.Kube.PodLogs(ctx, m.Namespace, name, "updater", 20, false)
	if err != nil {
		return err.Error()
	}
	return lastLine(logs, "no logs")
}

func (m *Manager) finished(ctx context.Context, run store.UpdateRun, st RunStatus) {
	if len(st.Bad) > 0 {
		m.markBad(ctx, st.Bad, st.Error)
	}
	ok := st.State == RunSucceeded
	action := "update." + st.State
	_ = m.Store.Audit(ctx, run.UserID, "", action, run.Summary, "", map[string]any{"run": run.ID, "trigger": run.Trigger, "error": st.Error})
	if m.Log != nil {
		m.Log.Info("update finished", "run", run.ID, "state", st.State, "summary", run.Summary, "err", st.Error)
	}
	if m.Notify != nil {
		title, msg := "Update finished: "+run.Summary, "Nectar Apps updated "+run.Summary+"."
		switch st.State {
		case RunSucceeded:
			if run.Trigger == "rollback" {
				title, msg = "Rolled back: "+run.Summary, "The rollback finished: "+run.Summary+"."
			}
		case RunFailed:
			title, msg = "Update not started: "+run.Summary, "Nothing was changed: "+st.Error
		case RunRolledBack:
			title, msg = "Update rolled back: "+run.Summary, "The update failed its health checks and was rolled back: "+st.Error
		default:
			title, msg = "Update failed, needs attention: "+run.Summary, "The update failed and the rollback did not finish: "+st.Error
		}
		m.Notify(ctx, title, msg, ok)
	}
	// refresh the state so the UI shows the new versions
	m.running.Add(1)
	go func() {
		defer m.running.Done()
		time.Sleep(m.poll())
		_, _ = m.Check(context.Background())
	}()
}

// Run checks on the configured interval, installs automatic updates in the maintenance window and resumes
// tracking runs that were in progress when the panel started (e.g. its own update).
func (m *Manager) Run(ctx context.Context) {
	if active, err := m.Store.ActiveUpdateRuns(ctx); err == nil && m.Kube != nil {
		for _, r := range active {
			m.track(r.ID)
		}
	}
	// first check soon after start (also confirms a self-update)
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		m.Tick(ctx)
		timer.Reset(time.Minute)
	}
}

// Tick runs a check when it is due and starts automatic updates.
func (m *Manager) Tick(ctx context.Context) {
	set, err := m.Settings(ctx)
	if err != nil {
		return
	}
	st := m.State(ctx)
	if m.now().Sub(st.CheckedAt) >= time.Duration(set.IntervalHours)*time.Hour {
		if st, err = m.Check(ctx); err != nil && m.Log != nil {
			m.Log.Warn("update check", "err", err)
		}
	}
	if m.Kube == nil {
		return
	}
	if err := m.autoUpdate(ctx, set, st); err != nil && m.Log != nil {
		m.Log.Info("auto-update", "err", err)
	}
}

// autoUpdate installs what the policies allow, inside the window.
func (m *Manager) autoUpdate(ctx context.Context, set Settings, st State) error {
	pc, gc := st.Components[Panel], st.Components[Gateway]
	if pc == nil || gc == nil || (pc.Auto == nil && gc.Auto == nil) {
		return nil
	}
	now := m.now()
	m.mu.Lock()
	recent := now.Sub(m.lastAuto) < time.Hour
	m.mu.Unlock()
	if recent {
		return nil
	}
	if !set.Window.Allows(now) {
		next := set.Window.Next(now)
		m.setWaiting(ctx, "waiting for the maintenance window (next: "+next.Format("Mon 15:04 MST")+")", pc, gc)
		return nil
	}
	if active, _ := m.Store.ActiveUpdateRuns(ctx); len(active) > 0 {
		return nil
	}
	req := PlanRequest{}
	if pc.Auto != nil {
		req.Panel = pc.Auto.Version
	}
	if gc.Auto != nil {
		req.Gateway = gc.Auto.Version
	}
	plan, err := m.Plan(ctx, req, false)
	if err != nil {
		// the other component may need an update too: allowed only when its own policy covers it
		full, err2 := m.Plan(ctx, req, true)
		if err2 != nil || !m.allowedAuto(set, st, full) {
			why := err.Error()
			if err2 != nil {
				why = err2.Error()
			}
			m.setWaiting(ctx, why, pc, gc)
			return nil
		}
		plan = full
	}
	m.mu.Lock()
	m.lastAuto = now
	m.mu.Unlock()
	_, err = m.Start(ctx, plan, "auto", nil, nil)
	return err
}

func (m *Manager) allowedAuto(set Settings, st State, p Plan) bool {
	for _, s := range p.Steps {
		cs := st.Components[s.Component]
		cur, ok := ParseSemver(s.From)
		r, err := findRelease(cs, s.To)
		if !ok || err != nil || !AutoInstall(set.Component(s.Component).Policy, cur, *r) {
			return false
		}
	}
	return true
}

func (m *Manager) setWaiting(ctx context.Context, why string, cs ...*ComponentState) {
	st := m.State(ctx)
	changed := false
	for _, c := range cs {
		if c == nil || c.Auto == nil {
			continue
		}
		for name, sc := range st.Components {
			if sc.Installed.Version == c.Installed.Version && sc.Auto != nil && sc.Auto.Version == c.Auto.Version && name != K3s && sc.Waiting != why {
				sc.Waiting = why
				changed = true
			}
		}
	}
	if changed {
		_ = m.Store.SetSetting(ctx, StateKey, st)
	}
}
