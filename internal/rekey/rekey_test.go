package rekey

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

func box(t *testing.T, key []byte, purpose string) *secretbox.Box {
	t.Helper()
	b, err := secretbox.New(key, purpose)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func seal(t *testing.T, key []byte, purpose, v string) string {
	t.Helper()
	s, err := box(t, key, purpose).Seal(v)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func open(t *testing.T, key []byte, purpose, v string) string {
	t.Helper()
	pt, err := box(t, key, purpose).Open(v)
	if err != nil {
		t.Fatalf("open %s with the new key: %v", purpose, err)
	}
	return pt
}

func secret(old, next []byte) kube.Object {
	d := map[string]any{KeyCurrent: base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(old)))}
	if next != nil {
		d[KeyNext] = base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(next)))
	}
	return kube.Object{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": SecretName, "namespace": "nectar-system"}, "data": d}
}

func TestRotation(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "p.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	oldKey, newKey := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// one sealed value in every column and setting
	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	a, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "image"})
	u, _ := st.CreateUser(ctx, "o@x.io", "O", "hash", true)
	db := st.DB()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO env_vars (app_id, key, value_enc) VALUES (?, 'K', ?)`, a.ID, seal(t, oldKey, "env", "env-value"))
	exec(`INSERT INTO app_files (app_id, path, content_enc) VALUES (?, '/etc/x', ?)`, a.ID, seal(t, oldKey, "env", "file"))
	exec(`INSERT INTO addons (project_id, slug, name, kind, version, size_mb, cpu_millicores, memory_mb, credentials, created_at, updated_at)
		VALUES (?, 'db', 'DB', 'postgres', '16', 1, 1, 1, ?, 0, 0)`, p.ID, seal(t, oldKey, "env", `{"password":"pw"}`))
	exec(`UPDATE users SET totp_secret = ? WHERE id = ?`, seal(t, oldKey, "totp", "TOTP"), u.ID)
	exec(`INSERT INTO alert_channels (kind, name, config, created_at, updated_at) VALUES ('ntfy', 'n', ?, 0, 0)`, seal(t, oldKey, "alerts", `{"topic":"t"}`))
	exec(`INSERT INTO backup_targets (name, kind, config, created_at, updated_at) VALUES ('t', 'local', ?, 0, 0)`, seal(t, oldKey, "backups", `{"password":"r"}`))
	exec(`INSERT INTO registry_credentials (host, username, password, created_at, updated_at) VALUES ('ghcr.io', 'me', ?, 0, 0)`, seal(t, oldKey, "registries", "ghp"))
	exec(`INSERT INTO app_sources (app_id, repo_url, token, webhook_secret, updated_at) VALUES (?, 'https://g/x.git', ?, ?, 0)`, a.ID,
		seal(t, oldKey, "builds", "tok"), seal(t, oldKey, "builds", "hook"))
	if err := st.SetSetting(ctx, "smtp", map[string]any{"host": "mail", "password": seal(t, oldKey, "smtp", "smtp-pw")}); err != nil {
		t.Fatal(err)
	}
	akey, err := ArtifactKey(ctx, st, box(t, oldKey, "env"))
	if err != nil {
		t.Fatal(err)
	}

	k := kubefake.New()
	k.Apply(ctx, secret(oldKey, nil))
	// first start: records the key id
	if got, err := Startup(ctx, Options{Store: st, Kube: k, Namespace: "nectar-system", Current: oldKey, Log: log}); err != nil || !bytes.Equal(got, oldKey) {
		t.Fatalf("plain start: %v", err)
	}
	// the wrong key is refused before anything is touched
	if _, err := Startup(ctx, Options{Store: st, Kube: k, Namespace: "nectar-system", Current: bytes.Repeat([]byte{9}, 32), Log: log}); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("wrong key: %v", err)
	}

	// a workload with an artifact URL signed with the master key (before the artifact key existed)
	sha := strings.Repeat("ab", 32)
	legacy := "http://panel/internal/artifacts/" + sha + "?sig=" + deploy.SignArtifact(oldKey, sha)
	k.Apply(ctx, kube.Object{"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "api", "namespace": "prj-shop", "labels": map[string]any{deploy.LabelManagedBy: deploy.ManagedBy}},
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"initContainers": []any{
			map[string]any{"name": "fetch-artifact", "env": []any{map[string]any{"name": "ARTIFACT_URL", "value": legacy}}}}}}}})

	// rotation: master-key-next is set, the next start re-seals
	k.Apply(ctx, secret(oldKey, newKey))
	got, err := Startup(ctx, Options{Store: st, Kube: k, Namespace: "nectar-system", Current: oldKey, Log: log})
	if err != nil || !bytes.Equal(got, newKey) {
		t.Fatalf("rotation: %v", err)
	}
	str := func(q string, args ...any) string {
		var v string
		if err := db.QueryRowContext(ctx, q, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return v
	}
	for _, c := range []struct{ purpose, q, want string }{
		{"env", `SELECT value_enc FROM env_vars`, "env-value"},
		{"env", `SELECT content_enc FROM app_files`, "file"},
		{"env", `SELECT credentials FROM addons`, `{"password":"pw"}`},
		{"totp", `SELECT totp_secret FROM users WHERE totp_secret != ''`, "TOTP"},
		{"alerts", `SELECT config FROM alert_channels`, `{"topic":"t"}`},
		{"backups", `SELECT config FROM backup_targets`, `{"password":"r"}`},
		{"registries", `SELECT password FROM registry_credentials`, "ghp"},
		{"builds", `SELECT token FROM app_sources`, "tok"},
		{"builds", `SELECT webhook_secret FROM app_sources`, "hook"},
	} {
		if v := open(t, newKey, c.purpose, str(c.q)); v != c.want {
			t.Errorf("%s: %q", c.q, v)
		}
	}
	var smtp map[string]any
	st.GetSetting(ctx, "smtp", &smtp)
	if open(t, newKey, "smtp", smtp["password"].(string)) != "smtp-pw" || smtp["host"] != "mail" {
		t.Errorf("smtp %v", smtp)
	}
	// the artifact key itself is the same (re-sealed), so new-style URLs stay valid
	if a2, err := ArtifactKey(ctx, st, box(t, newKey, "env")); err != nil || !bytes.Equal(a2, akey) {
		t.Fatalf("artifact key after rotation: %v", err)
	}
	// the Secret now holds the new key only
	var sec kube.Object
	k.GetObject(ctx, "Secret", "nectar-system", SecretName, &sec)
	if kube.Str(sec, "data", KeyNext) != "" || kube.Str(sec, "data", KeyCurrent) != secret(newKey, nil)["data"].(map[string]any)[KeyCurrent] {
		t.Fatalf("secret after rotation: %v", sec["data"])
	}
	// the legacy URL was re-signed with the artifact key
	dep := k.Get("Deployment", "prj-shop", "api")
	url := kube.Str(kube.Slice(kube.Slice(dep, "spec", "template", "spec", "initContainers")[0], "env")[0], "value")
	if !strings.HasSuffix(url, "sig="+deploy.SignArtifact(akey, sha)) {
		t.Fatalf("artifact url not re-signed: %s", url)
	}
	// starting with the old key now fails; with the new one it works
	if _, err := Startup(ctx, Options{Store: st, Kube: k, Namespace: "nectar-system", Current: oldKey, Log: log}); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("old key after rotation: %v", err)
	}
	if got, err := Startup(ctx, Options{Store: st, Kube: k, Namespace: "nectar-system", Current: newKey, Log: log}); err != nil || !bytes.Equal(got, newKey) {
		t.Fatalf("new key: %v", err)
	}
}

// A crash after the database was re-sealed but before the Secret switch is finished at the next start.
func TestRotationResumesAfterCrash(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "p.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	oldKey, newKey := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	if _, err := Reseal(ctx, st, oldKey, newKey); err != nil { // the "crashed" start
		t.Fatal(err)
	}
	k := kubefake.New()
	k.Apply(ctx, secret(oldKey, newKey))
	got, err := Startup(ctx, Options{Store: st, Kube: k, Namespace: "nectar-system", Current: oldKey, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil || !bytes.Equal(got, newKey) {
		t.Fatalf("resume: %v", err)
	}
	var sec kube.Object
	k.GetObject(ctx, "Secret", "nectar-system", SecretName, &sec)
	if kube.Str(sec, "data", KeyNext) != "" {
		t.Fatal("next key left in the Secret")
	}
}
