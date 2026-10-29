package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

// Plan scopes.
const (
	ScopeApp   = "app"   // the app's volumes
	ScopeAddon = "addon" // a dump of a database add-on
	ScopePanel = "panel" // the panel database and uploaded files (disaster recovery)
)

// Kube is the part of the Kubernetes client backups need (kube.Client and the test fake implement it).
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Delete(ctx context.Context, kind, namespace, name string) error
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error)
}

// Manager starts and tracks backup and restore Jobs and runs the schedules.
type Manager struct {
	Store    *store.Store
	Kube     Kube
	Deployer *deploy.Deployer
	Box      *secretbox.Box // target configs
	Log      *slog.Logger

	Namespace  string // system namespace (panel backups, snapshot listings)
	DataDir    string // the panel's data directory
	PanelClaim string // PVC of the panel data, default nectar-apps-data
	PanelUID   int64  // user the panel runs as (owns its files), default 65532

	Poll time.Duration // Job polling, default 3 s
	Now  func() time.Time

	mu      sync.Mutex
	busy    map[string]bool // scope:subject → a backup or restore is running
	snaps   map[int64]snapCache
	running sync.WaitGroup
}

type snapCache struct {
	at   time.Time
	list []Snapshot
}

// Snapshot is a restic snapshot.
type Snapshot struct {
	ID      string    `json:"id"`
	ShortID string    `json:"short_id"`
	Time    time.Time `json:"time"`
	Paths   []string  `json:"paths"`
	Tags    []string  `json:"tags"`
	Summary *struct {
		TotalBytesProcessed int64 `json:"total_bytes_processed"`
		DataAdded           int64 `json:"data_added"`
	} `json:"summary,omitempty"`
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
	return 3 * time.Second
}

func (m *Manager) claim() string {
	if m.PanelClaim != "" {
		return m.PanelClaim
	}
	return "nectar-apps-data"
}

func (m *Manager) uid() int64 {
	if m.PanelUID != 0 {
		return m.PanelUID
	}
	return 65532
}

// Wait blocks until running Jobs are tracked to the end (tests).
func (m *Manager) Wait() { m.running.Wait() }

// ErrBusy means a backup or restore of the same subject is running.
var ErrBusy = errors.New("a backup or restore of this is already running")

func (m *Manager) lock(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy == nil {
		m.busy = map[string]bool{}
	}
	if m.busy[key] {
		return false
	}
	m.busy[key] = true
	return true
}

func (m *Manager) unlock(key string) {
	m.mu.Lock()
	delete(m.busy, key)
	m.mu.Unlock()
}

// Tag identifies the snapshots of a plan's subject in the repository.
func Tag(scope string, subjectID int64) string {
	if scope == ScopePanel {
		return "panel"
	}
	return scope + ":" + strconv.FormatInt(subjectID, 10)
}

// target loads and decrypts a plan's target.
func (m *Manager) target(ctx context.Context, id int64) (store.BackupTarget, TargetConfig, error) {
	t, err := m.Store.GetBackupTarget(ctx, id)
	if err != nil {
		return t, TargetConfig{}, err
	}
	c, err := Open(m.Box, t.Config)
	return t, c, err
}

// subject describes what a plan backs up.
type subject struct {
	Name      string // shown in the history
	Namespace string
	App       *store.App
	Addon     *store.Addon
	Project   store.Project
	Volumes   []string  // app volumes that exist as PVCs
	CreatedAt time.Time // apps and add-ons: snapshots before this belong to an earlier subject with the same id
}

// Subject resolves a plan's scope and subject (exported for the API).
func (m *Manager) Subject(ctx context.Context, scope string, id int64) (string, error) {
	s, err := m.subject(ctx, scope, id)
	return s.Name, err
}

func (m *Manager) subject(ctx context.Context, scope string, id int64) (subject, error) {
	switch scope {
	case ScopePanel:
		return subject{Name: "Panel data", Namespace: m.Namespace}, nil
	case ScopeApp:
		a, err := m.Store.GetApp(ctx, id)
		if err != nil {
			return subject{}, fmt.Errorf("app %d: %w", id, err)
		}
		p, err := m.Store.GetProject(ctx, a.ProjectID)
		if err != nil {
			return subject{}, err
		}
		s := subject{Name: p.Slug + "/" + a.Slug, Namespace: p.Namespace, App: &a, Project: p, CreatedAt: a.CreatedAt}
		for _, v := range a.Config.Volumes {
			s.Volumes = append(s.Volumes, v.Name)
		}
		return s, nil
	case ScopeAddon:
		a, err := m.Store.GetAddon(ctx, id)
		if err != nil {
			return subject{}, fmt.Errorf("add-on %d: %w", id, err)
		}
		p, err := m.Store.GetProject(ctx, a.ProjectID)
		if err != nil {
			return subject{}, err
		}
		return subject{Name: p.Slug + "/" + a.Slug + " (" + a.Kind + ")", Namespace: p.Namespace, Addon: &a, Project: p, CreatedAt: a.CreatedAt}, nil
	}
	return subject{}, fmt.Errorf("unknown scope %q", scope)
}

func (m *Manager) jobName(kind string, plan store.BackupPlan) string {
	return fmt.Sprintf("nectar-%s-%s-%d-%d", kind, plan.Scope, plan.SubjectID, m.now().Unix())
}

// ------------------------------------------------------------- repositories --

// restServer is the in-cluster address of a local target's rest-server.
func (m *Manager) restServer(t store.BackupTarget) string {
	if t.Kind != KindLocal {
		return ""
	}
	return RestServerName(t.ID) + "." + m.Namespace + ".svc:8000"
}

// prepare puts the target credentials into the Job namespace and, for local targets, makes sure the
// rest-server runs.
func (m *Manager) prepare(ctx context.Context, ns string, t store.BackupTarget, cfg TargetConfig) error {
	if t.Kind == KindLocal {
		if err := m.ensureRestServer(ctx, t, cfg); err != nil {
			return fmt.Errorf("backup repository server: %w", err)
		}
	}
	_, err := m.Kube.Apply(ctx, targetSecret(ns, t.ID, t.Kind, cfg, m.restServer(t)))
	return err
}

func (m *Manager) ensureRestServer(ctx context.Context, t store.BackupTarget, cfg TargetConfig) error {
	htpasswd, err := auth.Htpasswd([]auth.BasicAuthUser{{User: cfg.RestUser, Password: cfg.RestPassword}}, "")
	if err != nil {
		return err
	}
	var cur kube.Object
	if err := m.Kube.GetObject(ctx, "Deployment", m.Namespace, RestServerName(t.ID), &cur); err == nil && kube.Int(cur, "status", "readyReplicas") > 0 {
		return nil // running (the htpasswd only changes with the target, which re-creates it)
	}
	for _, o := range restServerObjects(m.Namespace, t.ID, cfg, htpasswd) {
		if _, err := m.Kube.Apply(ctx, o); err != nil {
			return err
		}
	}
	deadline := m.now().Add(2 * time.Minute)
	for {
		var dep kube.Object
		if err := m.Kube.GetObject(ctx, "Deployment", m.Namespace, RestServerName(t.ID), &dep); err == nil && kube.Int(dep, "status", "readyReplicas") > 0 {
			return nil
		}
		if m.now().After(deadline) {
			return errors.New("the rest-server did not start within 2 minutes")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.poll()):
		}
	}
}

// RemoveRestServer deletes the rest-server of a deleted local target (the backups stay in the folder).
func (m *Manager) RemoveRestServer(ctx context.Context, t store.BackupTarget) {
	if t.Kind != KindLocal {
		return
	}
	for _, kind := range []string{"Deployment", "Service", "Secret"} {
		_ = m.Kube.Delete(ctx, kind, m.Namespace, RestServerName(t.ID))
	}
}

// ---------------------------------------------------------------- backups --

// StartBackup starts a backup Job for a plan and tracks it in the background.
func (m *Manager) StartBackup(ctx context.Context, plan store.BackupPlan) (store.BackupRun, error) {
	key := Tag(plan.Scope, plan.SubjectID)
	if !m.lock(key) {
		return store.BackupRun{}, ErrBusy
	}
	run, err := m.startBackup(ctx, plan)
	if err != nil {
		m.unlock(key)
		return run, err
	}
	m.track(run, func(bool) { m.unlock(key) })
	return run, nil
}

func (m *Manager) startBackup(ctx context.Context, plan store.BackupPlan) (store.BackupRun, error) {
	t, cfg, err := m.target(ctx, plan.TargetID)
	if err != nil {
		return store.BackupRun{}, err
	}
	sub, err := m.subject(ctx, plan.Scope, plan.SubjectID)
	if err != nil {
		return store.BackupRun{}, err
	}
	tag := Tag(plan.Scope, plan.SubjectID)
	j := jobSpec{Namespace: sub.Namespace, Name: m.jobName("backup", plan), TargetID: t.ID, Kind: t.Kind, Config: cfg,
		Env: map[string]string{"TAG": tag}}
	forget := fmt.Sprintf("restic forget --host nectar --tag %s --keep-daily %d --keep-weekly %d --keep-monthly %d --prune --quiet || echo 'warning: removing old snapshots failed'",
		shellQuote(tag), max(plan.KeepDaily, 1), max(plan.KeepWeekly, 0), max(plan.KeepMonthly, 0))
	backup := func(paths string) string {
		return resticPreamble + "restic backup --json --host nectar --tag nectar --tag \"$TAG\" " + paths + " | tail -n 1\n" + forget + "\n"
	}
	switch plan.Scope {
	case ScopeApp:
		if len(sub.Volumes) == 0 {
			return store.BackupRun{}, errors.New("the app has no volumes to back up")
		}
		for _, v := range sub.Volumes {
			j.Volumes = append(j.Volumes, map[string]any{"name": "v-" + v, "persistentVolumeClaim": map[string]any{"claimName": deploy.VolumeClaimName(sub.App.Slug, v), "readOnly": true}})
			j.Mounts = append(j.Mounts, map[string]any{"name": "v-" + v, "mountPath": "/data/" + v, "readOnly": true})
		}
		j.Script = backup("/data")
	case ScopeAddon:
		init, file, err := dumpContainer(*sub.Addon)
		if err != nil {
			return store.BackupRun{}, err
		}
		j.Init = []map[string]any{init}
		j.Script = backup("/work/" + file)
	case ScopePanel:
		dir := filepath.Join(m.DataDir, "backup")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return store.BackupRun{}, err
		}
		dst := filepath.Join(dir, "nectar.db")
		_ = os.Remove(dst)
		if err := m.Store.SnapshotDB(ctx, dst); err != nil {
			return store.BackupRun{}, fmt.Errorf("snapshot the database: %w", err)
		}
		uid := m.uid()
		j.RunAsUser = &uid
		j.Volumes = []map[string]any{{"name": "panel", "persistentVolumeClaim": map[string]any{"claimName": m.claim(), "readOnly": true}}}
		j.Mounts = []map[string]any{{"name": "panel", "mountPath": "/data", "readOnly": true}}
		j.Script = backup("/data/backup /data/artifacts/blobs")
	}
	if err := m.prepare(ctx, j.Namespace, t, cfg); err != nil {
		return store.BackupRun{}, err
	}
	if _, err := m.Kube.Apply(ctx, j.object()); err != nil {
		return store.BackupRun{}, err
	}
	pid := plan.ID
	return m.Store.CreateBackupRun(ctx, store.BackupRun{PlanID: &pid, Kind: "backup", Scope: plan.Scope, SubjectID: plan.SubjectID,
		Subject: sub.Name, Namespace: j.Namespace, Job: j.Name, StartedAt: m.now()})
}

// dumpContainer dumps an add-on into /work/<file> (the image of the add-on: client and server versions match).
func dumpContainer(a store.Addon) (map[string]any, string, error) {
	k, ok := deploy.AddonKindByID(a.Kind)
	if !ok {
		return nil, "", fmt.Errorf("unknown add-on kind %q", a.Kind)
	}
	v, ok := k.Version(a.Version)
	if !ok {
		return nil, "", fmt.Errorf("unknown version %q", a.Version)
	}
	host := a.Slug
	var script, file string
	switch a.Kind {
	case "postgres":
		file = "db.dump"
		script = `PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -h ` + host + ` -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /work/db.dump`
	case "mysql", "mariadb":
		file = "db.sql"
		dump := "mysqldump"
		if a.Kind == "mariadb" {
			dump = "mariadb-dump"
		}
		script = dump + ` -h ` + host + ` -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines --triggers --databases "$MYSQL_DATABASE" > /work/db.sql`
	case "redis":
		file = "dump.rdb"
		script = `redis-cli -h ` + host + ` -a "$REDIS_PASSWORD" --no-auth-warning --rdb /work/dump.rdb`
	}
	return map[string]any{
		"name":            "dump",
		"image":           v.Image,
		"imagePullPolicy": "IfNotPresent",
		"command":         []any{"sh", "-c", "set -eu\n" + script + "\nls -l /work"},
		"envFrom":         []any{map[string]any{"secretRef": map[string]any{"name": a.Slug + "-credentials"}}},
		"volumeMounts":    []any{map[string]any{"name": "work", "mountPath": "/work"}},
	}, file, nil
}

// ----------------------------------------------------------------- restore --

// StartRestore restores a snapshot of a plan's subject. Apps and Redis are stopped meanwhile and started
// again afterwards; PostgreSQL and MySQL dumps are loaded into the running database.
func (m *Manager) StartRestore(ctx context.Context, plan store.BackupPlan, snapshot string) (store.BackupRun, error) {
	if plan.Scope == ScopePanel {
		return store.BackupRun{}, errors.New("the panel itself is restored with install.sh --restore on a new server")
	}
	if !snapshotRe(snapshot) {
		return store.BackupRun{}, errors.New("invalid snapshot id")
	}
	// only a snapshot of this subject: restic restores any id (and "latest" across all tags) of the
	// repository, which other apps and projects share
	snaps, err := m.Snapshots(ctx, plan)
	if err != nil {
		return store.BackupRun{}, err
	}
	full, ok := pickSnapshot(snaps, snapshot)
	if !ok {
		return store.BackupRun{}, fmt.Errorf("%w: %s", ErrNoSnapshot, snapshot)
	}
	snapshot = full
	key := Tag(plan.Scope, plan.SubjectID)
	if !m.lock(key) {
		return store.BackupRun{}, ErrBusy
	}
	t, cfg, err := m.target(ctx, plan.TargetID)
	if err != nil {
		m.unlock(key)
		return store.BackupRun{}, err
	}
	sub, err := m.subject(ctx, plan.Scope, plan.SubjectID)
	if err != nil {
		m.unlock(key)
		return store.BackupRun{}, err
	}
	pid := plan.ID
	run, err := m.Store.CreateBackupRun(ctx, store.BackupRun{PlanID: &pid, Kind: "restore", Scope: plan.Scope, SubjectID: plan.SubjectID,
		Subject: sub.Name, Namespace: sub.Namespace, Job: m.jobName("restore", plan), Snapshot: snapshot, StartedAt: m.now()})
	if err != nil {
		m.unlock(key)
		return run, err
	}
	m.running.Add(1)
	go func() {
		defer m.running.Done()
		defer m.unlock(key)
		bg, cancel := context.WithTimeout(context.Background(), 7*time.Hour)
		defer cancel()
		err := m.restore(bg, run, sub, t, cfg, snapshot)
		status, msg := store.RunSucceeded, "restored snapshot "+shortID(snapshot)
		if err != nil {
			status, msg = store.RunFailed, err.Error()
			m.Log.Warn("restore failed", "subject", sub.Name, "err", err)
		}
		_ = m.Store.FinishBackupRun(bg, run.ID, status, msg, "", 0, m.now())
	}()
	return run, nil
}

// ErrNoSnapshot: the snapshot is not one of the plan subject's backups.
var ErrNoSnapshot = errors.New("no such backup of this app or database")

// pickSnapshot resolves "latest" or a (short) id among the subject's snapshots (newest first).
func pickSnapshot(snaps []Snapshot, want string) (string, bool) {
	if want == "latest" {
		if len(snaps) == 0 {
			return "", false
		}
		return snaps[0].ID, true
	}
	for _, s := range snaps {
		if strings.HasPrefix(s.ID, want) {
			return s.ID, true
		}
	}
	return "", false
}

func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func snapshotRe(s string) bool {
	if s == "latest" {
		return true
	}
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (m *Manager) restore(ctx context.Context, run store.BackupRun, sub subject, t store.BackupTarget, cfg TargetConfig, snapshot string) error {
	j := jobSpec{Namespace: sub.Namespace, Name: run.Job, TargetID: t.ID, Kind: t.Kind, Config: cfg, Env: map[string]string{"SNAP": snapshot}}
	open := strings.Replace(resticPreamble, "|| restic init", "|| { echo 'the backup repository does not exist'; exit 1; }", 1)
	var after func(context.Context) error
	switch {
	case sub.App != nil:
		app := *sub.App
		if len(sub.Volumes) == 0 {
			return errors.New("the app has no volumes")
		}
		includes := ""
		for _, v := range sub.Volumes {
			j.Volumes = append(j.Volumes, map[string]any{"name": "v-" + v, "persistentVolumeClaim": map[string]any{"claimName": deploy.VolumeClaimName(app.Slug, v)}})
			j.Mounts = append(j.Mounts, map[string]any{"name": "v-" + v, "mountPath": "/data/" + v})
			includes += " --include /data/" + v
		}
		j.Script = open + `restic restore "$SNAP" --host nectar --target /` + includes + " --delete\necho restored\n"
		replicas := app.Config.Replicas
		if replicas > 0 {
			if err := m.Deployer.Scale(ctx, app, sub.Project, 0); err != nil && !errors.Is(err, deploy.ErrNotDeployed) {
				return fmt.Errorf("stop the app: %w", err)
			}
			if err := m.waitPodsGone(ctx, sub.Namespace, deploy.LabelApp+"="+app.Slug+","+deploy.LabelRole+"=workload"); err != nil {
				return err
			}
			after = func(ctx context.Context) error {
				app, err := m.Store.GetApp(ctx, app.ID)
				if err != nil {
					return err
				}
				return m.Deployer.Scale(ctx, app, sub.Project, replicas)
			}
		}
	case sub.Addon != nil:
		a := *sub.Addon
		k, _ := deploy.AddonKindByID(a.Kind)
		v, _ := k.Version(a.Version)
		if a.Kind == "redis" {
			// replace the RDB file while Redis is stopped; it loads it on start (and rewrites its AOF)
			if !a.Stopped {
				a.Stopped = true
				if _, err := m.Store.SaveAddon(ctx, a); err != nil {
					return err
				}
				if err := m.Deployer.ApplyAddon(ctx, a); err != nil {
					return err
				}
				if err := m.waitPodsGone(ctx, sub.Namespace, deploy.LabelAddon+"="+a.Slug); err != nil {
					return err
				}
				after = func(ctx context.Context) error {
					a.Stopped = false
					a, err := m.Store.SaveAddon(ctx, a)
					if err != nil {
						return err
					}
					return m.Deployer.ApplyAddon(ctx, a)
				}
			}
			j.Volumes = []map[string]any{{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data-" + a.Slug + "-0"}}}
			j.Mounts = []map[string]any{{"name": "data", "mountPath": "/data"}}
			j.Script = open + `restic dump "$SNAP" /work/dump.rdb > /data/dump.rdb.new
mv /data/dump.rdb.new /data/dump.rdb
rm -rf /data/appendonlydir /data/appendonly.aof
echo restored`
			break
		}
		file := "db.dump"
		load := `PGPASSWORD="$POSTGRES_PASSWORD" pg_restore --clean --if-exists --no-owner -h ` + a.Slug + ` -U "$POSTGRES_USER" -d "$POSTGRES_DB" /work/db.dump`
		if a.Kind != "postgres" {
			file = "db.sql"
			client := "mysql"
			if a.Kind == "mariadb" {
				client = "mariadb"
			}
			load = client + ` -h ` + a.Slug + ` -uroot -p"$MYSQL_ROOT_PASSWORD" < /work/db.sql`
		}
		j.Script = open + `restic dump "$SNAP" /work/` + file + ` > /work/restore.tmp
mv /work/restore.tmp /work/` + file + "\n"
		j.After = []map[string]any{{
			"name":            "load",
			"image":           v.Image,
			"imagePullPolicy": "IfNotPresent",
			"command":         []any{"sh", "-c", "set -eu\n" + load + "\necho restored"},
			"envFrom":         []any{map[string]any{"secretRef": map[string]any{"name": a.Slug + "-credentials"}}},
			"volumeMounts":    []any{map[string]any{"name": "work", "mountPath": "/work"}},
		}}
	}
	jobErr := func() error {
		if err := m.prepare(ctx, j.Namespace, t, cfg); err != nil {
			return err
		}
		if _, err := m.Kube.Apply(ctx, j.object()); err != nil {
			return err
		}
		_, logs, err := m.waitJob(ctx, j.Namespace, j.Name)
		if err != nil {
			return fmt.Errorf("%w\n%s", err, lastLines(logs, 8))
		}
		return nil
	}()
	if after != nil {
		if err := after(ctx); err != nil && jobErr == nil {
			return fmt.Errorf("restored, but starting again failed: %w", err)
		}
	}
	return jobErr
}

// waitPodsGone waits until no pod matches the selector (up to 3 minutes).
func (m *Manager) waitPodsGone(ctx context.Context, ns, selector string) error {
	deadline := m.now().Add(3 * time.Minute)
	for {
		pods, err := m.Kube.List(ctx, "Pod", ns, selector)
		if err != nil {
			return err
		}
		if len(pods) == 0 {
			return nil
		}
		if m.now().After(deadline) {
			return errors.New("the pods did not stop within 3 minutes")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.poll()):
		}
	}
}

// --------------------------------------------------------------- tracking --

// track waits for a run's Job in the background and records the outcome.
func (m *Manager) track(run store.BackupRun, done func(ok bool)) {
	m.running.Add(1)
	go func() {
		defer m.running.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Hour)
		defer cancel()
		ok, logs, err := m.waitJob(ctx, run.Namespace, run.Job)
		status, msg := store.RunSucceeded, ""
		snap, added := "", int64(0)
		if sum := backupSummary(logs); sum != nil {
			snap, added = sum.SnapshotID, sum.DataAdded
			msg = fmt.Sprintf("snapshot %s: %d files, %s added", shortID(snap), sum.TotalFilesProcessed, humanBytes(sum.DataAdded))
		}
		if !ok {
			status, msg = store.RunFailed, strings.TrimSpace(fmt.Sprintf("%v\n%s", err, lastLines(logs, 8)))
			m.Log.Warn("backup failed", "subject", run.Subject, "err", err)
		}
		_ = m.Store.FinishBackupRun(ctx, run.ID, status, msg, snap, added, m.now())
		m.mu.Lock()
		if run.PlanID != nil {
			delete(m.snaps, *run.PlanID)
		}
		m.mu.Unlock()
		if done != nil {
			done(ok)
		}
	}()
}

type summary struct {
	SnapshotID          string `json:"snapshot_id"`
	DataAdded           int64  `json:"data_added"`
	TotalFilesProcessed int64  `json:"total_files_processed"`
}

func backupSummary(logs string) *summary {
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if !strings.Contains(l, `"message_type":"summary"`) {
			continue
		}
		var s summary
		if json.Unmarshal([]byte(l), &s) == nil && s.SnapshotID != "" {
			return &s
		}
	}
	return nil
}

// waitJob polls a Job until it ends and returns the logs of its containers.
func (m *Manager) waitJob(ctx context.Context, ns, name string) (bool, string, error) {
	for {
		var job kube.Object
		err := m.Kube.GetObject(ctx, "Job", ns, name, &job)
		if kube.IsNotFound(err) {
			return false, "", errors.New("the job disappeared (was the panel restarted while it ran?)")
		}
		if err != nil {
			return false, "", err
		}
		succeeded, failed := kube.Int(job, "status", "succeeded") > 0, kube.Int(job, "status", "failed") > 0
		for _, c := range kube.Slice(job, "status", "conditions") {
			if kube.Str(c, "type") == "Failed" && kube.Str(c, "status") == "True" {
				failed = true
			}
		}
		if succeeded || failed {
			logs, problem := m.jobLogs(ctx, ns, name)
			if succeeded {
				return true, logs, nil
			}
			if problem == "" {
				problem = "the job failed"
			}
			return false, logs, errors.New(problem)
		}
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		case <-time.After(m.poll()):
		}
	}
}

// jobLogs collects the logs of every container of a Job's pod (init containers first).
func (m *Manager) jobLogs(ctx context.Context, ns, job string) (string, string) {
	pods, err := m.Kube.List(ctx, "Pod", ns, "job-name="+job)
	if err != nil || len(pods) == 0 {
		return "", ""
	}
	pod := pods[len(pods)-1]
	name := kube.Str(pod, "metadata", "name")
	var b strings.Builder
	problem := ""
	for _, part := range []string{"initContainers", "containers"} {
		for _, c := range kube.Slice(pod, "spec", part) {
			cn := kube.Str(c, "name")
			if logs, err := m.Kube.PodLogs(ctx, ns, name, cn, 400, false); err == nil && strings.TrimSpace(logs) != "" {
				b.WriteString(strings.TrimRight(logs, "\n") + "\n")
			}
		}
	}
	for _, part := range []string{"initContainerStatuses", "containerStatuses"} {
		for _, cs := range kube.Slice(pod, "status", part) {
			if code := kube.Int(cs, "state", "terminated", "exitCode"); code != 0 && problem == "" {
				problem = fmt.Sprintf("%s exited with code %d", kube.Str(cs, "name"), code)
			}
			if r := kube.Str(cs, "state", "waiting", "reason"); r != "" && problem == "" {
				problem = kube.Str(cs, "name") + ": " + r + " " + kube.Str(cs, "state", "waiting", "message")
			}
		}
	}
	return b.String(), problem
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	var keep []string
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), `{"message_type":"status"`) {
			continue // progress lines
		}
		keep = append(keep, l)
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return strings.Join(keep, "\n")
}

func humanBytes(b int64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(u), 0
	for n := b / u; n >= u && exp < 4; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTP"[exp])
}

// --------------------------------------------------------------- snapshots --

// Snapshots lists the snapshots of a plan's subject, newest first (cached for 30 s; a short Job reads them).
func (m *Manager) Snapshots(ctx context.Context, plan store.BackupPlan) ([]Snapshot, error) {
	m.mu.Lock()
	if c, ok := m.snaps[plan.ID]; ok && m.now().Sub(c.at) < 30*time.Second {
		m.mu.Unlock()
		return c.list, nil
	}
	m.mu.Unlock()
	t, cfg, err := m.target(ctx, plan.TargetID)
	if err != nil {
		return nil, err
	}
	out, err := m.resticOutput(ctx, t, cfg, fmt.Sprintf("nectar-snapshots-%d-%d", plan.ID, m.now().UnixNano()%1e9),
		`restic snapshots --json --host nectar --tag "$TAG"`, map[string]string{"TAG": Tag(plan.Scope, plan.SubjectID)}, false)
	if err != nil {
		return nil, err
	}
	list, err := parseSnapshots(out)
	if err != nil {
		return nil, err
	}
	if plan.Scope != ScopePanel {
		// ids of deleted apps and add-ons are reused (SQLite rowids): the repository may still hold the
		// tag of an earlier subject, possibly of another project. Its snapshots are older than this one.
		sub, err := m.subject(ctx, plan.Scope, plan.SubjectID)
		if err != nil {
			return nil, err
		}
		list = since(list, sub.CreatedAt.Add(-snapshotSkew))
	}
	m.mu.Lock()
	if m.snaps == nil {
		m.snaps = map[int64]snapCache{}
	}
	m.snaps[plan.ID] = snapCache{at: m.now(), list: list}
	m.mu.Unlock()
	return list, nil
}

// snapshotSkew allows for clock differences between the panel and the node that ran restic. Ids are
// not reused since migration 0015; this guards repositories written before it.
const snapshotSkew = 10 * time.Second

func since(list []Snapshot, t time.Time) []Snapshot {
	out := list[:0:0]
	for _, s := range list {
		if !s.Time.Before(t) {
			out = append(out, s)
		}
	}
	return out
}

func parseSnapshots(out string) ([]Snapshot, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(l, "[") {
			continue
		}
		var list []Snapshot
		if err := json.Unmarshal([]byte(l), &list); err != nil {
			return nil, fmt.Errorf("unexpected restic output: %w", err)
		}
		sort.Slice(list, func(a, b int) bool { return list[a].Time.After(list[b].Time) })
		return list, nil
	}
	return nil, errors.New("restic printed no snapshot list")
}

// CheckTarget opens (or creates) the repository of a target and reports its snapshot count.
func (m *Manager) CheckTarget(ctx context.Context, t store.BackupTarget) (int, error) {
	cfg, err := Open(m.Box, t.Config)
	if err != nil {
		return 0, err
	}
	out, err := m.resticOutput(ctx, t, cfg, fmt.Sprintf("nectar-check-t%d-%d", t.ID, m.now().UnixNano()%1e9), `restic snapshots --json`, nil, true)
	if err != nil {
		return 0, err
	}
	list, err := parseSnapshots(out)
	return len(list), err
}

// resticOutput runs a short restic command in the system namespace and returns its output.
func (m *Manager) resticOutput(ctx context.Context, t store.BackupTarget, cfg TargetConfig, name, cmd string, env map[string]string, init bool) (string, error) {
	pre := resticPreamble
	if !init {
		pre = strings.Replace(pre, "|| restic init", "|| { echo '[]'; exit 0; }", 1) // no repository yet: no snapshots
	}
	j := jobSpec{Namespace: m.Namespace, Name: name, TargetID: t.ID, Kind: t.Kind, Config: cfg, Script: pre + cmd + "\n", Env: env, Deadline: 300}
	if err := m.prepare(ctx, m.Namespace, t, cfg); err != nil {
		return "", err
	}
	if _, err := m.Kube.Apply(ctx, j.object()); err != nil {
		return "", err
	}
	defer func() { _ = m.Kube.Delete(context.Background(), "Job", m.Namespace, name) }()
	wctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ok, logs, err := m.waitJob(wctx, m.Namespace, name)
	if !ok {
		if err == nil {
			err = errors.New("restic failed")
		}
		return "", fmt.Errorf("%w\n%s", err, lastLines(logs, 6))
	}
	return logs, nil
}

// ---------------------------------------------------------------- schedule --

// Run starts scheduled backups every minute until ctx ends and resumes tracking runs left from a restart.
func (m *Manager) Run(ctx context.Context) {
	m.recover(ctx)
	for {
		now := m.now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		select {
		case <-ctx.Done():
			return
		case <-time.After(next.Sub(now) + time.Second):
		}
		m.Tick(ctx, next)
	}
}

// Tick starts the backups due at minute t.
func (m *Manager) Tick(ctx context.Context, t time.Time) {
	plans, err := m.Store.ListBackupPlans(ctx)
	if err != nil {
		m.Log.Warn("backups: plans", "err", err)
		return
	}
	for _, p := range plans {
		if !p.Enabled || p.Schedule == "" {
			continue
		}
		s, err := ParseSchedule(p.Schedule)
		if err != nil || !s.Match(t) {
			continue
		}
		if _, err := m.StartBackup(ctx, p); err != nil && !errors.Is(err, ErrBusy) {
			m.Log.Warn("scheduled backup could not start", "plan", p.ID, "err", err)
			pid := p.ID
			name, _ := m.Subject(ctx, p.Scope, p.SubjectID)
			if run, cerr := m.Store.CreateBackupRun(ctx, store.BackupRun{PlanID: &pid, Kind: "backup", Scope: p.Scope, SubjectID: p.SubjectID,
				Subject: name, StartedAt: m.now()}); cerr == nil {
				_ = m.Store.FinishBackupRun(ctx, run.ID, store.RunFailed, err.Error(), "", 0, m.now())
			}
		}
	}
}

// recover tracks Jobs that were running when the panel stopped.
func (m *Manager) recover(ctx context.Context) {
	runs, err := m.Store.ListBackupRuns(ctx, 0, store.RunRunning, 100)
	if err != nil {
		return
	}
	for _, r := range runs {
		if r.Kind != "backup" || r.Job == "" {
			_ = m.Store.FinishBackupRun(ctx, r.ID, store.RunFailed, "interrupted by a panel restart", "", 0, m.now())
			continue
		}
		key := Tag(r.Scope, r.SubjectID)
		if m.lock(key) {
			m.track(r, func(bool) { m.unlock(key) })
		}
	}
}
