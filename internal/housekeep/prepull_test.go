package housekeep

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/store"
)

// fakeAPI hides kubefake's test helper Get, which has another signature than Kube.Get.
type fakeAPI struct{ *kubefake.Fake }

func (fakeAPI) Get(context.Context, string, any) error { return nil }

func TestPrepull(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "p.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	k := kubefake.New()
	used := []string{"node:24-slim", "eclipse-temurin:21-jre", "node:24-slim"}
	kp := &Keeper{Store: st, Kube: fakeAPI{k}, Namespace: "nectar-system", InitImage: "busybox:1.37",
		Images: func(context.Context) []string { return used }}

	// off by default: nothing is created
	if err := kp.SyncPrepull(ctx); err != nil || k.Get("DaemonSet", "nectar-system", PrepullName) != nil {
		t.Fatalf("disabled: %v", err)
	}
	st.SetSetting(ctx, SettingsKey, Settings{Prepull: true, PrepullImages: []string{"redis:7.4", "busybox:1.37"}})
	if err := kp.SyncPrepull(ctx); err != nil {
		t.Fatal(err)
	}
	ds := k.Get("DaemonSet", "nectar-system", PrepullName)
	if ds == nil {
		t.Fatal("no DaemonSet")
	}
	if got := kube.Str(ds, "metadata", "annotations", "nectar.dev/images"); got != "busybox:1.37,eclipse-temurin:21-jre,node:24-slim,redis:7.4" {
		t.Fatalf("images %s", got)
	}
	cs := kube.Slice(ds, "spec", "template", "spec", "containers")
	if len(cs) != 4 || kube.Str(cs[1], "image") != "eclipse-temurin:21-jre" || kube.Field(cs[1], "command").([]any)[0] != "/nectar/busybox" {
		t.Fatalf("containers %v", cs)
	}
	if kube.Str(kube.Slice(ds, "spec", "template", "spec", "initContainers")[0], "image") != "busybox:1.37" {
		t.Fatal("init container")
	}
	// switching it off removes the DaemonSet
	st.SetSetting(ctx, SettingsKey, Settings{})
	if err := kp.SyncPrepull(ctx); err != nil || k.Get("DaemonSet", "nectar-system", PrepullName) != nil {
		t.Fatalf("not removed: %v", err)
	}
	if err := kp.SyncPrepull(ctx); err != nil {
		t.Fatalf("second removal: %v", err)
	}
	// validation
	bad := Defaults
	bad.PrepullImages = []string{"a b"}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "prepull_images") {
		t.Fatalf("validate: %v", err)
	}
}
