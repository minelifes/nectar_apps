package api

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/backup"
	"nectar_apps/internal/secretbox"
)

func TestBackupTargetsAndPlans(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	if code, _, _ := e.do(o, "GET", "/api/backup/targets", nil); code != 503 {
		t.Fatalf("without kubernetes: %d", code)
	}
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32), "backups")
	e.api.backups = &backup.Manager{Store: e.api.store, Kube: e.kube, Deployer: e.dep, Box: box, Namespace: "nectar-system",
		DataDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Poll: time.Millisecond}

	if code, _, _ := e.do(o, "POST", "/api/backup/targets", map[string]any{"name": "S3", "kind": "s3", "config": map[string]any{"bucket": "b"}}); code != 400 {
		t.Fatalf("missing keys: %d", code)
	}
	code, tg, _ := e.do(o, "POST", "/api/backup/targets", map[string]any{"name": "S3", "kind": "s3",
		"config": map[string]any{"endpoint": "https://s3.us-west-004.backblazeb2.com", "bucket": "nectar", "access_key": "AK", "secret_key": "SK"}})
	if code != 201 || tg["repository"] != "s3:https://s3.us-west-004.backblazeb2.com/nectar" || tg["config"].(map[string]any)["secret_key"] != backup.Masked ||
		tg["config"].(map[string]any)["password"] != backup.Masked {
		t.Fatalf("create target: %d %v", code, tg)
	}
	tid := itoa(tg["id"].(float64))
	if code, v, _ := e.do(o, "PUT", "/api/backup/targets/"+tid, map[string]any{"name": "B2", "config": map[string]any{
		"endpoint": "https://s3.us-west-004.backblazeb2.com", "bucket": "nectar", "prefix": "srv1", "access_key": "AK", "secret_key": backup.Masked}}); code != 200 || v["name"] != "B2" {
		t.Fatalf("update target: %d %v", code, v)
	}
	code, kit, _ := e.do(o, "GET", "/api/backup/targets/"+tid+"/recovery-kit", nil)
	env, _ := kit["env"].(map[string]any)
	if code != 200 || kit["repository"] != "s3:https://s3.us-west-004.backblazeb2.com/nectar/srv1" || env["AWS_SECRET_ACCESS_KEY"] != "SK" || len(env["RESTIC_PASSWORD"].(string)) < 20 {
		t.Fatalf("kit: %d %v", code, kit)
	}

	// plans: an app without volumes cannot be backed up; the panel can
	e.do(o, "POST", "/api/projects", map[string]any{"slug": "web", "name": "Web"})
	e.do(o, "POST", "/api/projects/1/apps", map[string]any{"slug": "img", "name": "Img", "runtime": "image", "config": map[string]any{"image": "x:1", "port": 80}})
	if code, _, _ := e.do(o, "POST", "/api/backup/plans", map[string]any{"target_id": tg["id"], "scope": "app", "subject_id": 1, "schedule": "@daily"}); code != 400 {
		t.Fatalf("app without volumes: %d", code)
	}
	if code, _, _ := e.do(o, "POST", "/api/backup/plans", map[string]any{"target_id": tg["id"], "scope": "panel", "schedule": "61 * * * *"}); code != 400 {
		t.Fatalf("bad schedule: %d", code)
	}
	code, plan, _ := e.do(o, "POST", "/api/backup/plans", map[string]any{"target_id": tg["id"], "scope": "panel", "schedule": "30 2 * * *"})
	if code != 201 || plan["subject"] != "Panel data" || plan["next_run"] == nil || plan["keep_daily"].(float64) != 7 {
		t.Fatalf("create plan: %d %v", code, plan)
	}
	pid := itoa(plan["id"].(float64))
	if code, _, _ := e.do(o, "DELETE", "/api/backup/targets/"+tid, nil); code != 409 {
		t.Fatalf("delete a target in use: %d", code)
	}
	if code, run, _ := e.do(o, "POST", "/api/backup/plans/"+pid+"/run", nil); code != 202 || run["status"] != "running" {
		t.Fatalf("run: %d %v", code, run)
	}
	e.api.backups.Wait()
	if code, _, runs := e.do(o, "GET", "/api/backup/runs?plan="+pid, nil); code != 200 || len(runs) != 1 || runs[0].(map[string]any)["status"] != "succeeded" {
		t.Fatalf("runs: %d %v", code, runs)
	}
	if code, v, _ := e.do(o, "POST", "/api/backup/plans/"+pid+"/restore", map[string]any{"snapshot": "latest"}); code != 400 || !strings.Contains(v["error"].(string), "install.sh --restore") {
		t.Fatalf("panel restore: %d %v", code, v)
	}
	if code, v, _ := e.do(o, "PUT", "/api/backup/plans/"+pid, map[string]any{"schedule": "", "enabled": false}); code != 200 || v["next_run"] != nil {
		t.Fatalf("update plan: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/backup/plans/"+pid, nil); code != 204 {
		t.Fatalf("delete plan: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", "/api/backup/targets/"+tid, nil); code != 204 {
		t.Fatalf("delete target: %d", code)
	}
}
