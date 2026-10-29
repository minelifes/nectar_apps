package housekeep

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/artifacts"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

type fakeKube struct {
	applied []kube.Object
	jobs    []kube.Object
	pods    []kube.Object
}

func (f *fakeKube) ListNodes(context.Context) ([]kube.Node, error) {
	return []kube.Node{{Name: "node-1"}, {Name: "Node_2.example"}}, nil
}

func (f *fakeKube) Get(_ context.Context, path string, out any) error {
	js := `{"node":{"fs":{"capacityBytes":1000,"usedBytes":400},"runtime":{"imageFs":{"capacityBytes":1000,"usedBytes":300}}}}`
	return json.Unmarshal([]byte(js), out)
}

func (f *fakeKube) GetObject(_ context.Context, kind, _, name string, out any) error {
	js := `{"status":{"images":[{"names":["docker.io/rancher/mirrored-pause@sha256:abc","docker.io/rancher/mirrored-pause:3.6"]}]}}`
	if name != "node-1" {
		js = `{"status":{"images":[]}}`
	}
	return json.Unmarshal([]byte(js), out)
}

func (f *fakeKube) Apply(_ context.Context, o kube.Object) (kube.Object, error) {
	f.applied = append(f.applied, o)
	return o, nil
}

func (f *fakeKube) List(_ context.Context, kind, _, _ string) ([]kube.Object, error) {
	if kind == "Job" {
		return f.jobs, nil
	}
	return f.pods, nil
}

func (f *fakeKube) PodLogs(context.Context, string, string, string, int, bool) (string, error) {
	return "Deleted: docker.io/library/a:1\nDeleted: docker.io/library/b:2\n", nil
}

func obj(js string) kube.Object {
	var o kube.Object
	if err := json.Unmarshal([]byte(js), &o); err != nil {
		panic(err)
	}
	return o
}

func TestCleanAndDisk(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(dir, "nectar.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	arts, err := artifacts.New(filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	app, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "image"})
	for i := 0; i < 3; i++ {
		tmp, sha, size, err := arts.Receive(strings.NewReader(strings.Repeat("x", 100+i)), 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		if err := arts.Commit(tmp, sha); err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateArtifact(ctx, app.ID, sha, "a.jar", size, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.SetSetting(ctx, SettingsKey, Settings{KeepArtifacts: 1})
	_ = st.Audit(ctx, nil, "x", "old.action", "t", "", nil)

	k := &Keeper{Store: st, Artifacts: arts, DataDir: dir, DBPath: filepath.Join(dir, "nectar.db"), Namespace: "nectar-system",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return time.Now().AddDate(2, 0, 0) }}
	res, err := k.Clean(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Artifacts != 2 || res.Files != 2 || res.BytesFreed != 201 || res.AuditEntries != 1 {
		t.Fatalf("result %+v", res)
	}
	if n, _ := st.CountArtifacts(ctx); n != 1 {
		t.Fatalf("artifacts left: %d", n)
	}

	fk := &fakeKube{}
	k.Kube = fk
	r := k.Disk(ctx)
	if r.Panel.ArtifactFiles != 1 || r.Panel.ArtifactBytes != 102 || r.Panel.DatabaseBytes == 0 || len(r.Nodes) != 2 || r.Nodes[0].ImagesUsed != 300 {
		t.Fatalf("report %+v", r)
	}
	if r.Settings.KeepArtifacts != 1 || r.Settings.AuditDays != 365 {
		t.Fatalf("settings %+v", r.Settings)
	}
}

func TestPruneImages(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "n.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fk := &fakeKube{jobs: []kube.Object{obj(`{"metadata":{"name":"nectar-image-cleanup-node-1-1","creationTimestamp":"2026-10-28T10:00:00Z",
		"annotations":{"nectar.dev/node":"node-1"}},"status":{"active":1}}`)}}
	k := &Keeper{Store: st, Kube: fk, Namespace: "nectar-system", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	jobs, err := k.PruneImages(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	// node-1 already has a running cleanup
	if len(jobs) != 1 || jobs[0].Node != "Node_2.example" || len(fk.applied) != 1 {
		t.Fatalf("jobs %+v", jobs)
	}
	b, _ := json.Marshal(fk.applied[0])
	s := string(b)
	for _, want := range []string{`"nodeName":"Node_2.example"`, `"image":"rancher/mirrored-pause:3.6"`, `/var/lib/rancher/k3s/data/current/bin/crictl`,
		`"name":"nectar-image-cleanup-node-2-example-`} {
		if !strings.Contains(s, want) {
			t.Errorf("job lacks %s: %s", want, s)
		}
	}

	// finished job: removed images counted from the logs
	fk.jobs[0]["status"] = map[string]any{"succeeded": float64(1), "completionTime": "2026-10-28T10:01:00Z"}
	fk.pods = []kube.Object{obj(`{"metadata":{"name":"p","labels":{"job-name":"nectar-image-cleanup-node-1-1"}}}`)}
	list := k.pruneJobs(ctx)
	if len(list) != 1 || list[0].Status != "succeeded" || list[0].Removed != 2 || list[0].Finished == nil {
		t.Fatalf("jobs %+v", list)
	}
	fk.applied = nil
	if _, err := k.PruneImages(ctx, "node-1"); err != nil || len(fk.applied) != 1 {
		t.Fatalf("node-1: %v %d", err, len(fk.applied))
	}
	if b, _ := json.Marshal(fk.applied[0]); !strings.Contains(string(b), `"image":"docker.io/rancher/mirrored-pause:3.6"`) {
		t.Errorf("pause image from node status: %s", b)
	}
	if _, err := k.PruneImages(ctx, "missing"); err == nil {
		t.Error("unknown node accepted")
	}
}
