package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/store"
)

func TestAlertChannelsRulesAndDisk(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	_, v, _ := e.do(c, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password", "client": "mobile"})
	auth := []string{"Authorization", "Bearer " + v["access_token"].(string)}
	plain := &http.Client{}

	var mu sync.Mutex
	var got []string
	ntfy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.URL.Path+"|"+r.Header.Get("Authorization")+"|"+string(b))
		mu.Unlock()
	}))
	defer ntfy.Close()

	// channels: validation, masked secrets, kept secrets on update
	if code, _, _ := e.do(plain, "POST", "/api/alerts/channels", map[string]any{"kind": "sms", "name": "x"}, auth...); code != 400 {
		t.Fatalf("unknown kind: %d", code)
	}
	if code, _, _ := e.do(plain, "POST", "/api/alerts/channels", map[string]any{"kind": "telegram", "name": "tg", "config": map[string]any{"bot_token": "x"}}, auth...); code != 400 {
		t.Fatalf("bad telegram: %d", code)
	}
	code, ch, _ := e.do(plain, "POST", "/api/alerts/channels", map[string]any{"kind": "ntfy", "name": "Phone",
		"config": map[string]any{"url": ntfy.URL, "topic": "ops", "token": "tk_secret"}}, auth...)
	if code != 201 || ch["config"].(map[string]any)["token"] != "••••••••" || ch["enabled"] != true {
		t.Fatalf("create: %d %v", code, ch)
	}
	id := itoa(ch["id"].(float64))
	code, ch, _ = e.do(plain, "PUT", "/api/alerts/channels/"+id, map[string]any{"name": "Phone 2",
		"config": map[string]any{"url": ntfy.URL, "topic": "ops2", "token": "••••••••"}}, auth...)
	if code != 200 || ch["name"] != "Phone 2" {
		t.Fatalf("update: %d %v", code, ch)
	}
	if code, _, _ := e.do(plain, "POST", "/api/alerts/channels/"+id+"/test", nil, auth...); code != 200 {
		t.Fatalf("test: %d", code)
	}
	mu.Lock()
	if len(got) != 1 || !strings.HasPrefix(got[0], "/ops2|Bearer tk_secret|Alerts will arrive") {
		t.Fatalf("test message: %q", got)
	}
	mu.Unlock()
	if code, v, _ := e.do(plain, "GET", "/api/alerts/channels", nil, auth...); code != 200 || len(v["channels"].([]any)) != 1 || len(v["kinds"].([]any)) != 5 {
		t.Fatalf("list: %d %v", code, v)
	}

	// rules
	code, v, _ = e.do(plain, "GET", "/api/alerts/rules", nil, auth...)
	if code != 200 || len(v["catalog"].([]any)) != len(v["settings"].(map[string]any)["rules"].([]any)) {
		t.Fatalf("rules: %d %v", code, v)
	}
	if code, _, _ := e.do(plain, "PUT", "/api/alerts/rules", map[string]any{"rules": []any{map[string]any{"kind": "node_cpu", "enabled": true, "threshold": 300}}}, auth...); code != 400 {
		t.Fatalf("bad threshold: %d", code)
	}
	code, v, _ = e.do(plain, "PUT", "/api/alerts/rules", map[string]any{"panel_url": "https://panel.example.com/", "repeat_hours": 6,
		"rules": []any{map[string]any{"kind": "node_cpu", "enabled": false, "threshold": 80, "for": 300}}}, auth...)
	if code != 200 || v["settings"].(map[string]any)["panel_url"] != "https://panel.example.com" {
		t.Fatalf("put rules: %d %v", code, v)
	}
	if code, _, list := e.do(plain, "GET", "/api/alerts?state=firing", nil, auth...); code != 200 || list == nil {
		t.Fatalf("alerts: %d %v", code, list)
	}

	// read marks: per user, "all" covers firing incidents, unread brings one back
	a1, err := e.api.store.OpenAlert(t.Context(), store.Alert{Rule: "app_down", Key: "app_down:1", Subject: "web", Severity: "critical", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := e.api.store.OpenAlert(t.Context(), store.Alert{Rule: "node_disk", Key: "node_disk:n1", Subject: "n1", Severity: "warning", StartedAt: time.Now()})
	readOf := func() map[float64]bool {
		_, _, list := e.do(plain, "GET", "/api/alerts?state=firing", nil, auth...)
		out := map[float64]bool{}
		for _, x := range list {
			m := x.(map[string]any)
			out[m["id"].(float64)] = m["read"].(bool)
		}
		return out
	}
	if code, _, _ := e.do(plain, "POST", "/api/alerts/read", map[string]any{"ids": []int64{a1.ID, 999}}, auth...); code != 204 {
		t.Fatalf("read one: %d", code)
	}
	if r := readOf(); !r[float64(a1.ID)] || r[float64(a2.ID)] {
		t.Fatalf("after read one: %v", r)
	}
	if code, _, _ := e.do(plain, "POST", "/api/alerts/read", nil, auth...); code != 204 {
		t.Fatalf("read all: %d", code)
	}
	if r := readOf(); !r[float64(a1.ID)] || !r[float64(a2.ID)] {
		t.Fatalf("after read all: %v", r)
	}
	if code, _, _ := e.do(plain, "DELETE", "/api/alerts/read/"+itoa(float64(a2.ID)), nil, auth...); code != 204 {
		t.Fatalf("unread: %d", code)
	}
	if r := readOf(); !r[float64(a1.ID)] || r[float64(a2.ID)] {
		t.Fatalf("after unread: %v", r)
	}

	if code, _, _ := e.do(plain, "DELETE", "/api/alerts/channels/"+id, nil, auth...); code != 204 {
		t.Fatalf("delete: %d", code)
	}

	// housekeeping
	code, v, _ = e.do(plain, "GET", "/api/server/disk", nil, auth...)
	if code != 200 || v["settings"].(map[string]any)["keep_artifacts"].(float64) != 10 || len(v["nodes"].([]any)) != 1 {
		t.Fatalf("disk: %d %v", code, v)
	}
	if code, _, _ := e.do(plain, "PUT", "/api/settings/housekeeping", map[string]any{"keep_artifacts": 0, "audit_days": 365, "alert_days": 90}, auth...); code != 400 {
		t.Fatalf("bad housekeeping: %d", code)
	}
	if code, _, _ := e.do(plain, "PUT", "/api/settings/housekeeping", map[string]any{"keep_artifacts": 3, "audit_days": 90, "alert_days": 30}, auth...); code != 200 {
		t.Fatalf("housekeeping: %d", code)
	}
	if code, v, _ := e.do(plain, "POST", "/api/server/cleanup", nil, auth...); code != 200 || v["result"] == nil {
		t.Fatalf("cleanup: %d %v", code, v)
	}
}
