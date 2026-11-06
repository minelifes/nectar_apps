package api

import (
	"bytes"
	"encoding/base64"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/backup"
	"nectar_apps/internal/cli"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/secretbox"
)

// An app marked as a database: detection, credentials, the CLI's list, links (${pg.DATABASE_URL} resolved
// on the linked app's deploy), unlink, and dump backups.
func TestAppDatabase(t *testing.T) {
	e := newEnv(t)
	e.api.cluster = e.kube
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	_, pg, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "pg", "name": "PG", "runtime": "image", "kind": "worker",
		"config": map[string]any{"image": "postgres:17-alpine", "port": 5432}})
	_, api, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image",
		"config": map[string]any{"image": "traefik/whoami:v1.10", "port": 80}})
	pgID, apiID := itoa(pg["id"].(float64)), itoa(api["id"].(float64))
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+pgID+"/env", []map[string]any{{"key": "POSTGRES_USER", "value": "shop"},
		{"key": "POSTGRES_PASSWORD", "value": "s3cret", "secret": true}, {"key": "POSTGRES_DB", "value": "orders"}}); code != 204 {
		t.Fatalf("env: %d", code)
	}

	code, det, _ := e.do(o, "GET", "/api/apps/"+pgID+"/database/detect", nil)
	sug, _ := det["suggestion"].(map[string]any)
	if code != 200 || sug["engine"] != "postgres" || sug["user_var"] != "POSTGRES_USER" || sug["password_var"] != "POSTGRES_PASSWORD" || sug["database_var"] != "POSTGRES_DB" {
		t.Fatalf("detect: %d %v", code, det)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+pgID+"/database", map[string]any{"engine": "postgres", "password_var": "NOPE"}); code != 400 {
		t.Fatalf("missing variable: %d", code)
	}
	if code, _, _ := e.do(o, "PUT", "/api/apps/"+apiID+"/database", map[string]any{"engine": "oracle"}); code != 400 {
		t.Fatalf("unknown engine: %d", code)
	}
	if code, v, _ := e.do(o, "PUT", "/api/apps/"+pgID+"/database", sug); code != 200 || v["database"].(map[string]any)["engine"] != "postgres" {
		t.Fatalf("mark: %d %v", code, v)
	}
	if code, cr, _ := e.do(o, "GET", "/api/apps/"+pgID+"/database/credentials", nil); code != 200 || cr["user"] != "shop" || cr["password"] != "s3cret" ||
		cr["database"] != "orders" || cr["port"].(float64) != 5432 || !strings.HasPrefix(cr["env"].(map[string]any)["DATABASE_URL"].(string), "postgres://shop:s3cret@pg:5432/orders") {
		t.Fatalf("credentials: %d %v", code, cr)
	}
	if code, _, list := e.do(o, "GET", "/api/projects/"+pid+"/app-databases", nil); code != 200 || len(list) != 1 || list[0].(map[string]any)["slug"] != "pg" || list[0].(map[string]any)["app"] != true {
		t.Fatalf("app-databases: %d %v", code, list)
	}

	// link: the api app gets references, resolved when it deploys
	if code, _, links := e.do(o, "POST", "/api/apps/"+pgID+"/database/links", map[string]any{"app_id": api["id"], "prefix": ""}); code != 200 ||
		len(links) != 1 || links[0].(map[string]any)["slug"] != "api" {
		t.Fatalf("link: %d %v", code, links)
	}
	_, _, envs := e.do(o, "GET", "/api/apps/"+apiID+"/env", nil)
	if !strings.Contains(jsonOf(envs), `"PGHOST"`) || !strings.Contains(jsonOf(envs), `"DATABASE_URL"`) {
		t.Fatalf("linked env: %v", envs)
	}
	if code, v, _ := e.do(o, "POST", "/api/apps/"+apiID+"/deploy", nil); code != 202 && code != 200 {
		t.Fatalf("deploy api: %d %v", code, v)
	}
	e.dep.Wait()
	sec := e.kube.Get("Secret", "prj-shop", "api-env")
	url, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "DATABASE_URL"))
	if !strings.HasPrefix(string(url), "postgres://shop:s3cret@pg:5432/orders") {
		t.Fatalf("resolved DATABASE_URL = %q (%v)", url, sec["data"])
	}
	// a second link without prefix conflicts with the existing variables of another database? no: same refs → fine
	if code, _, _ := e.do(o, "POST", "/api/apps/"+pgID+"/database/links", map[string]any{"app_id": api["id"], "prefix": "lower_"}); code != 400 {
		t.Fatalf("bad prefix: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/apps/"+pgID+"/database/links/"+apiID, nil); code != 204 {
		t.Fatalf("unlink: %d", code)
	}
	if _, _, envs := e.do(o, "GET", "/api/apps/"+apiID+"/env", nil); strings.Contains(jsonOf(envs), `"PGHOST"`) {
		t.Fatalf("unlinked env still has PGHOST: %v", envs)
	}

	// backups of a database app are dumps (no volumes needed)
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32), "backups")
	e.api.backups = &backup.Manager{Store: e.api.store, Kube: e.kube, Deployer: e.dep, Box: box, Namespace: "nectar-system",
		DataDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Poll: time.Millisecond}
	_, tg, _ := e.do(o, "POST", "/api/backup/targets", map[string]any{"name": "S3", "kind": "s3",
		"config": map[string]any{"endpoint": "https://s3.example.com", "bucket": "b", "access_key": "AK", "secret_key": "SK"}})
	code, plan, _ := e.do(o, "POST", "/api/backup/plans", map[string]any{"target_id": tg["id"], "scope": "app", "subject_id": pg["id"], "schedule": "@daily"})
	if code != 201 || plan["subject"] != "shop/pg (postgres)" {
		t.Fatalf("plan: %d %v", code, plan)
	}
	if code, run, _ := e.do(o, "POST", "/api/backup/plans/"+itoa(plan["id"].(float64))+"/run", nil); code != 202 {
		t.Fatalf("run: %d %v", code, run)
	}
	jobs, _ := e.kube.List(t.Context(), "Job", "prj-shop", "")
	var dump string
	for _, j := range jobs {
		for _, c := range kube.Slice(j, "spec", "template", "spec", "initContainers") {
			dump = jsonOf(c)
		}
	}
	if !strings.Contains(dump, "pg_dump -h pg -p 5432") || !strings.Contains(dump, `"postgres:17-alpine"`) || !strings.Contains(dump, `"key":"POSTGRES_PASSWORD"`) ||
		strings.Contains(dump, "s3cret") {
		t.Fatalf("dump container: %s", dump)
	}
	e.api.backups.Wait()

	// the CLI lists it with the add-ons
	_, tok, _ := e.do(o, "POST", "/api/me/tokens", map[string]any{"name": "cli"})
	var out, errb bytes.Buffer
	if code := cli.Run(t.Context(), []string{"db", "list", "--project", "shop"}, cli.Env{Stdout: &out, Stderr: &errb, Version: "test",
		Client: &cli.Client{URL: e.srv.URL, Token: tok["token"].(string)}}); code != 0 || !strings.Contains(out.String(), "shop/pg") || !strings.Contains(out.String(), "(app)") {
		t.Fatalf("db list: %d %s %s", code, out.String(), errb.String())
	}

	// unmark
	if code, _, _ := e.do(o, "DELETE", "/api/apps/"+pgID+"/database", nil); code != 204 {
		t.Fatalf("unmark: %d", code)
	}
	if _, _, list := e.do(o, "GET", "/api/projects/"+pid+"/app-databases", nil); len(list) != 0 {
		t.Fatalf("still listed: %v", list)
	}
}
