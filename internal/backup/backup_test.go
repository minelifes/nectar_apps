package backup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

func TestSchedule(t *testing.T) {
	s, err := ParseSchedule("30 2 * * 1-5")
	if err != nil {
		t.Fatal(err)
	}
	mon := time.Date(2026, 10, 26, 2, 30, 0, 0, time.UTC) // a Monday
	if !s.Match(mon) || s.Match(mon.Add(time.Minute)) || s.Match(mon.AddDate(0, 0, 6)) {
		t.Error("weekday schedule")
	}
	if n := s.Next(mon); !n.Equal(mon.AddDate(0, 0, 1)) {
		t.Errorf("next %v", n)
	}
	e, _ := ParseSchedule("*/15 * * * *")
	if !e.Match(time.Date(2026, 1, 1, 5, 45, 0, 0, time.UTC)) || e.Match(time.Date(2026, 1, 1, 5, 46, 0, 0, time.UTC)) {
		t.Error("step")
	}
	d, _ := ParseSchedule("@daily")
	if !d.Match(time.Date(2026, 3, 4, 3, 0, 0, 0, time.UTC)) {
		t.Error("@daily")
	}
	sun, _ := ParseSchedule("0 0 * * 7")
	if !sun.Match(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Error("7 = Sunday")
	}
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* * * * 8", "*/0 * * * *", "a * * * *"} {
		if _, err := ParseSchedule(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestTargetConfig(t *testing.T) {
	c := TargetConfig{Bucket: "bk", Prefix: "/nectar/", AccessKey: "AK", SecretKey: "SK"}
	if err := c.Validate(KindS3); err != nil {
		t.Fatal(err)
	}
	if c.Repository(KindS3, "") != "s3:https://s3.amazonaws.com/bk/nectar" || len(c.Password) < 20 {
		t.Errorf("repo %q", c.Repository(KindS3, ""))
	}
	env := c.Env(KindS3, "")
	if env["AWS_ACCESS_KEY_ID"] != "AK" || env["RESTIC_PASSWORD"] != c.Password {
		t.Errorf("env %v", env)
	}
	m := c.Mask()
	if m.SecretKey != Masked || m.Password != Masked || m.AccessKey != "AK" {
		t.Errorf("mask %+v", m)
	}
	edited := m
	edited.Password = "changed"
	if k := edited.KeepSecrets(c); k.SecretKey != "SK" || k.Password != c.Password {
		t.Errorf("keep %+v", k)
	}
	if err := (&TargetConfig{Path: "/var/lib/rancher/x"}).Validate(KindLocal); err == nil {
		t.Error("k3s data dir accepted")
	}
	if err := (&TargetConfig{Host: "h", User: "u", Path: "/b", PrivateKey: "nope"}).Validate(KindSFTP); err == nil {
		t.Error("bad key accepted")
	}
	if u, h, ok := splitSFTP("sftp:bak@nas.lan:/srv/restic"); !ok || u != "bak" || h != "nas.lan" {
		t.Errorf("split %q %q", u, h)
	}
}

// fake extends kubefake with Job pods and programmable logs.
type fake struct {
	*kubefake.Fake
	mu   sync.Mutex
	logs map[string]string // container → output
}

func (f *fake) List(ctx context.Context, kind, ns, sel string) ([]kube.Object, error) {
	if kind == "Pod" && strings.HasPrefix(sel, "job-name=") {
		var job kube.Object
		if f.Fake.GetObject(ctx, "Job", ns, strings.TrimPrefix(sel, "job-name="), &job) != nil {
			return nil, nil
		}
		return []kube.Object{{"metadata": map[string]any{"name": "p"}, "spec": kube.Field(job, "spec", "template", "spec")}}, nil
	}
	items, err := f.Fake.List(ctx, kind, ns, sel)
	if kind != "Pod" {
		return items, err
	}
	// the base fake synthesizes a pod per Deployment: none for a Deployment scaled to 0
	var out []kube.Object
	for _, it := range items {
		var dep kube.Object
		if f.Fake.GetObject(ctx, "Deployment", ns, strings.TrimSuffix(kube.Str(it, "metadata", "name"), "-pod"), &dep) != nil || kube.Int(dep, "spec", "replicas") > 0 {
			out = append(out, it)
		}
	}
	return out, err
}

func (f *fake) PodLogs(_ context.Context, _, _, container string, _ int, _ bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs[container], nil
}

func (f *fake) setLogs(c, s string) {
	f.mu.Lock()
	f.logs[c] = s
	f.mu.Unlock()
}

type env struct {
	st  *store.Store
	k   *fake
	m   *Manager
	d   *deploy.Deployer
	box *secretbox.Box
	t   store.BackupTarget
	dir string
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(dir, "nectar.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, _ := secretbox.New(bytes.Repeat([]byte{7}, 32), "env")
	k := &fake{Fake: kubefake.New(), logs: map[string]string{}}
	cat, _ := runtimes.LoadBuiltin()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := deploy.New(st, k, cat, box, log, deploy.Options{InternalURL: "http://panel", SigningKey: []byte("k"), PollInterval: time.Millisecond})
	cfg := TargetConfig{Path: "/var/backups/nectar"}
	if err := cfg.Validate(KindLocal); err != nil {
		t.Fatal(err)
	}
	sealed, _ := Seal(box, cfg)
	tg, err := st.SaveBackupTarget(ctx, store.BackupTarget{Name: "disk", Kind: KindLocal, Config: sealed})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: st, Kube: k, Deployer: d, Box: box, Log: log, Namespace: "nectar-system", DataDir: dir, Poll: time.Millisecond}
	return &env{st: st, k: k, m: m, d: d, box: box, t: tg, dir: dir}
}

const summaryLine = `{"message_type":"summary","files_new":3,"total_files_processed":3,"data_added":2048,"snapshot_id":"0123456789abcdef0123456789abcdef"}`

func TestAddonBackupAndRestore(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p, _ := e.st.CreateProject(ctx, "shop", "Shop", nil)
	creds, _ := e.d.SealCredentials(deploy.NewAddonCredentials("postgres", "db"))
	a, err := e.st.CreateAddon(ctx, store.Addon{ProjectID: p.ID, Slug: "db", Name: "DB", Kind: "postgres", Version: "16", SizeMB: 1024, CPUMillicores: 250, MemoryMB: 256, Credentials: creds})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.st.SaveBackupPlan(ctx, store.BackupPlan{TargetID: e.t.ID, Scope: ScopeAddon, SubjectID: a.ID, Schedule: "0 3 * * *", KeepDaily: 7, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	e.k.setLogs("restic", `{"message_type":"status","percent_done":0.5}`+"\n"+summaryLine)
	run, err := e.m.StartBackup(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	e.m.Wait()
	if !e.m.lock("x") || e.m.lock("x") {
		t.Fatal("one backup per subject at a time")
	}
	job := e.k.Get("Job", p.Namespace, run.Job)
	b, _ := json.Marshal(job)
	for _, want := range []string{`pg_dump -h db`, `"image":"postgres:16-alpine"`, `restic backup --json --host nectar --tag nectar --tag \"$TAG\" /work/db.dump`,
		`"value":"addon:` + itoa(a.ID) + `"`, `--keep-daily 7`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("job lacks %s\n%s", want, b)
		}
	}
	sec := e.k.Get("Secret", p.Namespace, "nectar-backup-t"+itoa(e.t.ID))
	if v, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "RESTIC_REPOSITORY")); !strings.HasPrefix(string(v), "rest:http://nectar:") ||
		!strings.HasSuffix(string(v), "@nectar-backup-repo-"+itoa(e.t.ID)+".nectar-system.svc:8000/") {
		t.Errorf("repository %q", v)
	}
	rs, _ := json.Marshal(e.k.Get("Deployment", "nectar-system", RestServerName(e.t.ID)))
	if !strings.Contains(string(rs), `"hostPath":{"path":"/var/backups/nectar","type":"DirectoryOrCreate"}`) || strings.Contains(string(b), "hostPath") {
		t.Errorf("rest-server %s", rs)
	}
	e.m.Wait()
	runs, _ := e.st.ListBackupRuns(ctx, plan.ID, "", 10)
	if len(runs) != 1 || runs[0].Status != store.RunSucceeded || runs[0].Snapshot != "0123456789abcdef0123456789abcdef" || runs[0].BytesAdded != 2048 {
		t.Fatalf("run %+v", runs)
	}

	// snapshots are read by a short Job
	// (the older one has the same tag: a deleted add-on whose id this one reuses)
	later := a.CreatedAt.Add(time.Hour).Format(time.RFC3339)
	e.k.setLogs("restic", `[{"id":"0123456789abcdef0123456789abcdef","short_id":"01234567","time":"`+later+`","paths":["/work/db.dump"],"tags":["nectar","addon:1"]},`+
		`{"id":"fedcba9876543210fedcba9876543210","short_id":"fedcba98","time":"2020-01-01T03:00:00Z","paths":["/work/db.dump"],"tags":["nectar","addon:1"]}]`)
	snaps, err := e.m.Snapshots(ctx, plan)
	if err != nil || len(snaps) != 1 || snaps[0].ShortID != "01234567" {
		t.Fatalf("snapshots (only this add-on's) %v %v", snaps, err)
	}

	// restore: restic fetches the dump first, pg_restore loads it
	if _, err := e.m.StartRestore(ctx, plan, "bad id!"); err == nil {
		t.Fatal("bad snapshot id accepted")
	}
	// a snapshot of another subject (or the earlier owner of the id) is refused
	for _, other := range []string{"fedcba98", "abcdef0123456789"} {
		if _, err := e.m.StartRestore(ctx, plan, other); !errors.Is(err, ErrNoSnapshot) {
			t.Fatalf("restore of a foreign snapshot %s: %v", other, err)
		}
	}
	rr, err := e.m.StartRestore(ctx, plan, snaps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	e.m.Wait()
	b, _ = json.Marshal(e.k.Get("Job", p.Namespace, rr.Job))
	if !strings.Contains(string(b), `restic dump \"$SNAP\" /work/db.dump`) || !strings.Contains(string(b), `pg_restore --clean --if-exists --no-owner -h db`) {
		t.Errorf("restore job: %s", b)
	}
	if r, _ := e.st.ListBackupRuns(ctx, plan.ID, "", 1); r[0].Kind != "restore" || r[0].Status != store.RunSucceeded {
		t.Fatalf("restore run %+v", r[0])
	}

	// a failing job records the reason
	e.k.JobOK = false
	e.k.setLogs("restic", "Fatal: unable to open repository")
	if _, err := e.m.StartBackup(ctx, plan); err != nil {
		t.Fatal(err)
	}
	e.m.Wait()
	last, _ := e.st.LastBackupRuns(ctx)
	if last[plan.ID].Status != store.RunFailed || !strings.Contains(last[plan.ID].Message, "unable to open repository") {
		t.Fatalf("failed run %+v", last[plan.ID])
	}
}

func TestAppVolumesAndPanelBackup(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p, _ := e.st.CreateProject(ctx, "shop", "Shop", nil)
	a, _ := e.st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "image",
		Config: store.AppConfig{Image: "x:1", Port: 80, Replicas: 2, Health: store.HealthConfig{Type: "tcp"},
			Resources: store.Resources{CPUMillicores: 100, MemoryMB: 64}, Volumes: []store.Volume{{Name: "data", MountPath: "/data", SizeMB: 512}}}})
	rel, err := e.d.Deploy(ctx, deploy.DeployRequest{App: a})
	if err != nil {
		t.Fatal(err)
	}
	e.d.Wait()
	if r, _ := e.st.GetRelease(ctx, rel.ID); r.Status != store.ReleaseLive {
		t.Fatalf("deploy: %s %s", r.Status, r.Message)
	}
	plan, _ := e.st.SaveBackupPlan(ctx, store.BackupPlan{TargetID: e.t.ID, Scope: ScopeApp, SubjectID: a.ID, Schedule: "*/5 * * * *", KeepDaily: 3, Enabled: true})

	// the schedule starts it
	e.k.setLogs("restic", summaryLine)
	e.m.Tick(ctx, time.Date(2026, 10, 28, 12, 10, 0, 0, time.UTC))
	e.m.Wait()
	runs, _ := e.st.ListBackupRuns(ctx, plan.ID, "", 1)
	if len(runs) != 1 || runs[0].Status != store.RunSucceeded {
		t.Fatalf("scheduled run %+v", runs)
	}
	b, _ := json.Marshal(e.k.Get("Job", p.Namespace, runs[0].Job))
	if !strings.Contains(string(b), `"claimName":"api-data","readOnly":true`) || !strings.Contains(string(b), `"mountPath":"/data/data"`) {
		t.Errorf("app backup job: %s", b)
	}

	// restore stops the app, restores with --delete and starts it again ("latest" = the app's newest)
	e.k.setLogs("restic", `[{"id":"0123456789abcdef0123456789abcdef","short_id":"01234567","time":"`+a.CreatedAt.Add(time.Minute).Format(time.RFC3339)+`","paths":["/data/data"]}]`)
	if snaps, err := e.m.Snapshots(ctx, plan); err != nil || len(snaps) != 1 {
		t.Fatalf("app snapshots: %v %v", snaps, err)
	}
	e.k.setLogs("restic", "restored")
	rr, err := e.m.StartRestore(ctx, plan, "latest")
	if err != nil {
		t.Fatal(err)
	}
	e.m.Wait()
	runs, _ = e.st.ListBackupRuns(ctx, plan.ID, "", 1)
	if runs[0].ID != rr.ID || runs[0].Status != store.RunSucceeded {
		t.Fatalf("restore run: %+v", runs[0])
	}
	b, _ = json.Marshal(e.k.Get("Job", p.Namespace, rr.Job))
	if !strings.Contains(string(b), `restic restore \"$SNAP\" --host nectar --target / --include /data/data --delete`) || strings.Contains(string(b), `"readOnly":true`) {
		t.Errorf("restore job: %s", b)
	}
	if a2, _ := e.st.GetApp(ctx, a.ID); a2.Config.Replicas != 2 || kube.Int(e.k.Get("Deployment", p.Namespace, "api"), "spec", "replicas") != 2 {
		t.Errorf("replicas after the restore: %d", a2.Config.Replicas)
	}

	// panel backup snapshots the database first
	pp, _ := e.st.SaveBackupPlan(ctx, store.BackupPlan{TargetID: e.t.ID, Scope: ScopePanel, Schedule: "", KeepDaily: 7, Enabled: true})
	e.k.JobOK = true
	e.k.setLogs("restic", summaryLine)
	run, err := e.m.StartBackup(ctx, pp)
	if err != nil {
		t.Fatal(err)
	}
	e.m.Wait()
	if _, err := os.Stat(filepath.Join(e.dir, "backup", "nectar.db")); err != nil {
		t.Fatal("no database snapshot")
	}
	b, _ = json.Marshal(e.k.Get("Job", "nectar-system", run.Job))
	if !strings.Contains(string(b), "/data/backup /data/artifacts/blobs") || !strings.Contains(string(b), `"runAsUser":65532`) {
		t.Errorf("panel job: %s", b)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
