package api

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/cli"
)

// The nectar CLI against the real API (fake cluster): token, deploy of a file and of an image, rollback.
func TestCLIDeploy(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	roles := roleIDs(t, e, o)
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := proj["id"].(float64)
	e.do(o, "POST", "/api/projects/"+itoa(pid)+"/apps", map[string]any{"slug": "site", "name": "Site", "runtime": "static"})
	e.do(o, "POST", "/api/projects/"+itoa(pid)+"/apps", map[string]any{"slug": "img", "name": "Img", "runtime": "image",
		"config": map[string]any{"image": "nginx:1.27", "port": 80}})
	_, tok, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "CI", "role_id": roles["deployer"], "project_id": pid})

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := cli.Run(context.Background(), args, cli.Env{Stdout: &out, Stderr: &errb, Version: "test", Poll: 5 * time.Millisecond,
			Client: &cli.Client{URL: e.srv.URL, Token: tok["token"].(string)}})
		return code, out.String(), errb.String()
	}
	if code, out, errs := run("apps"); code != 0 || !strings.Contains(out, "shop/site") || !strings.Contains(out, "shop/img") {
		t.Fatalf("apps: %d %s %s", code, out, errs)
	}
	dir := t.TempDir()
	zip := filepath.Join(dir, "site.zip")
	os.WriteFile(zip, zipBytes("index.html"), 0o644)
	code, out, errs := run("deploy", "shop/site", "--file", zip)
	if code != 0 || !strings.Contains(out, "Release #1 of Site is live") {
		t.Fatalf("deploy file: %d\n%s\n%s", code, out, errs)
	}
	e.dep.Wait()
	// image apps: --image switches the tag (allowed with app.deploy)
	if code, out, errs = run("deploy", "shop/img", "--image", "nginx:1.28"); code != 0 {
		t.Fatalf("deploy image: %d\n%s\n%s", code, out, errs)
	}
	e.dep.Wait()
	run("deploy", "shop/img", "--image", "nginx:1.29")
	e.dep.Wait()
	_, app, _ := e.do(o, "GET", "/api/apps/2", nil)
	if app["config"].(map[string]any)["image"] != "nginx:1.29" {
		t.Fatalf("image not switched: %v", app["config"])
	}
	if code, _, errs = run("deploy", "shop/site", "--image", "x:1"); code != 1 || !strings.Contains(errs, "image is only for") {
		t.Fatalf("image on a static app: %d %s", code, errs)
	}
	if code, out, errs = run("rollback", "shop/img"); code != 0 || !strings.Contains(out, "to release #1") {
		t.Fatalf("rollback: %d\n%s\n%s", code, out, errs)
	}
	e.dep.Wait()
	if code, out, _ = run("releases", "2"); code != 0 || !strings.Contains(out, "live") {
		t.Fatalf("releases: %d %s", code, out)
	}
	if code, _, errs = run("deploy", "shop/nope"); code != 1 || !strings.Contains(errs, "no app") {
		t.Fatalf("unknown app: %d %s", code, errs)
	}
	// login saves the config (token from stdin)
	var out2 bytes.Buffer
	code = cli.Run(context.Background(), []string{"login", "--url", e.srv.URL}, cli.Env{Stdout: &out2, Stderr: &out2,
		Stdin: strings.NewReader(tok["token"].(string) + "\n")})
	if code != 0 || !strings.Contains(out2.String(), "Logged in") {
		t.Fatalf("login: %d %s", code, out2.String())
	}
	cfg, err := cli.LoadConfig()
	if err != nil || cfg.URL != e.srv.URL || !strings.HasPrefix(cfg.Token, "nat_") {
		t.Fatalf("config %+v %v", cfg, err)
	}
	if p, _ := cli.ConfigPath(); func() bool { st, err := os.Stat(p); return err != nil || st.Mode().Perm() != 0o600 }() {
		t.Error("config must be 0600")
	}
}
