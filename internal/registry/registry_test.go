package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
)

func TestEnsureAndRefs(t *testing.T) {
	ctx := context.Background()
	f := kubefake.New()
	r := &Registry{Kube: f, Namespace: "nectar-system", Password: "secret-password-123"}
	if !r.Enabled() || (&Registry{}).Enabled() || (*Registry)(nil).Enabled() {
		t.Fatal("Enabled")
	}
	if err := r.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"PersistentVolumeClaim/nectar-registry-data", "Secret/nectar-registry-auth", "Deployment/nectar-registry", "Service/nectar-registry", "CronJob/nectar-registry-gc"} {
		kind, name, _ := strings.Cut(k, "/")
		if !f.Has(kind, "nectar-system", name) {
			t.Errorf("missing %s", k)
		}
	}
	sec := f.Get("Secret", "nectar-system", "nectar-registry-auth")
	line := kube.Str(sec, "stringData", "htpasswd")
	u, h, _ := strings.Cut(line, ":")
	if u != User || bcrypt.CompareHashAndPassword([]byte(h), []byte("secret-password-123")) != nil {
		t.Fatalf("htpasswd %q", line)
	}
	if p := kube.Slice(f.Get("Service", "nectar-system", "nectar-registry"), "spec", "ports"); len(p) != 1 || kube.Int(p[0], "nodePort") != 30500 {
		t.Errorf("node port %v", p)
	}
	if got := r.PullRef("prj-shop/api", "build-3"); got != "registry.nectar.local/prj-shop/api:build-3" || !r.Owns(got) || r.Owns("ghcr.io/x/y:1") {
		t.Errorf("pull ref %s", got)
	}
	if got := r.PushRef("prj-shop/api", "build-3"); got != "nectar-registry.nectar-system.svc:5000/prj-shop/api:build-3" {
		t.Errorf("push ref %s", got)
	}
	var cfg struct {
		Auths map[string]map[string]string `json:"auths"`
	}
	json.Unmarshal(DockerConfig([]Credential{r.BuildCredential(), {Host: "docker.io", Username: "me", Password: "pw"}}), &cfg)
	if a := cfg.Auths["https://index.docker.io/v1/"]["auth"]; a != base64.StdEncoding.EncodeToString([]byte("me:pw")) {
		t.Errorf("docker hub auth %v", cfg.Auths)
	}
	if cfg.Auths[r.PushHost()]["username"] != User {
		t.Error("registry auth")
	}
	if buildNum("build-12") != 12 || buildNum("cache") != 0 {
		t.Error("buildNum")
	}
	if err := (&Registry{}).Ensure(ctx); err != ErrDisabled {
		t.Error("disabled")
	}
}
