package api

import (
	"strings"
	"testing"
	"time"
)

func (f *fakeGateway) record(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dns[name]
}

func (f *fakeGateway) setFeatures(feats ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.features = feats
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", what)
}

func TestWebOptionsAndAutoDNS(t *testing.T) {
	e, fg := newEnvGW(t)
	fg.setFeatures("certificates", "rate-limit-429", "web-options", "dns-records")
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	pid := itoa(proj["id"].(float64))
	_, app, _ := e.do(o, "POST", "/api/projects/"+pid+"/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image",
		"config": map[string]any{"image": "nginx:1", "port": 80}})
	aid := itoa(app["id"].(float64))
	if code, _, _ := e.do(o, "POST", "/api/apps/"+aid+"/deploy", nil); code != 202 {
		t.Fatalf("deploy: %d", code)
	}
	e.dep.Wait()

	// web options: validated, stored, sent to the gateway
	for _, bad := range []map[string]any{
		{"redirects": []map[string]any{{"from": "old", "to": "/new"}}},
		{"allow_ips": []string{"nope"}},
		{"response_headers": map[string]string{"Content-Length": "1"}},
		{"cors": map[string]any{"origins": []string{"*"}, "credentials": true}},
	} {
		if code, v, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "bad.example.com", "web": bad}); code != 400 {
			t.Errorf("accepted web %v: %d %v", bad, code, v)
		}
	}
	web := map[string]any{"redirects": []map[string]any{{"from": "/old", "to": "/new"}}, "allow_ips": []string{"203.0.113.0/24"},
		"response_headers": map[string]string{"X-Frame-Options": "DENY"}, "compress": true, "hsts": 31536000}
	code, d, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "shop.example.com", "web": web})
	if code != 201 || d["web"] == nil || d["web"].(map[string]any)["hsts"].(float64) != 31536000 {
		t.Fatalf("create: %d %v", code, d)
	}
	did := itoa(d["id"].(float64))
	rt := fg.byDomain("shop.example.com")
	if w, _ := rt["web"].(map[string]any); w == nil || w["compress"] != true || len(w["redirects"].([]any)) != 1 {
		t.Fatalf("gateway route web: %v", rt["web"])
	}
	// clearing: {} removes the options on the gateway too
	code, d, _ = e.do(o, "PATCH", "/api/apps/"+aid+"/domains/"+did, map[string]any{"web": map[string]any{}})
	if code != 200 || d["web"] != nil {
		t.Fatalf("clear web: %d %v", code, d)
	}
	if w, _ := fg.byDomain("shop.example.com")["web"].(map[string]any); len(w) != 0 {
		t.Fatalf("gateway web not cleared: %v", w)
	}
	// an older gateway: the route is served without the options and the domain says why
	fg.setFeatures("certificates")
	e.do(o, "PATCH", "/api/apps/"+aid+"/domains/"+did, map[string]any{"web": map[string]any{"compress": true}})
	_, _, list := e.do(o, "GET", "/api/apps/"+aid+"/domains", nil)
	if len(list) == 0 {
		t.Fatal("no domains")
	}
	found := false
	for _, x := range list {
		m := x.(map[string]any)
		if m["host"] == "shop.example.com" {
			found = strings.Contains(m["status_message"].(string), "the latest nectar-gateway")
		}
	}
	if !found {
		t.Fatalf("no update hint on an old gateway: %v", list)
	}
	if w, _ := fg.byDomain("shop.example.com")["web"].(map[string]any); len(w) != 0 {
		t.Fatal("web sent to a gateway without web-options")
	}
	fg.setFeatures("certificates", "web-options", "dns-records")

	// automatic DNS: refused without the public IP? (no nodes in the fake cluster → set public_ips)
	if code, v, _ := e.do(o, "PUT", "/api/settings", map[string]any{"base_domain": "apps.example.com", "public_ips": []string{"203.0.113.10"}, "dns_auto": true}); code != 200 ||
		v["dns"].(map[string]any)["name"] != "*.apps.example.com" {
		t.Fatalf("settings dns_auto: %d %v", code, v)
	}
	if got := fg.record("*.apps.example.com"); len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("base wildcard record: %v", got)
	}
	// a custom domain gets its own record; a host under the base is covered by the wildcard
	_, cd, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "blog.example.org"})
	eventually(t, "record for blog.example.org", func() bool { return len(fg.record("blog.example.org")) == 1 })
	_, sub, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "extra.apps.example.com"})
	eventually(t, "covered state", func() bool {
		_, v, _ := e.do(o, "GET", "/api/domains", nil)
		for _, x := range v["domains"].([]any) {
			m := x.(map[string]any)
			if m["host"] == "extra.apps.example.com" {
				return m["dns_state"] == "covered"
			}
		}
		return false
	})
	if fg.record("extra.apps.example.com") != nil {
		t.Fatal("a record for a host covered by the wildcard")
	}
	// a name with someone else's records: conflict, nothing changed
	fg.mu.Lock()
	fg.foreign = map[string]bool{"taken.example.net": true}
	fg.mu.Unlock()
	_, td, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains", map[string]any{"host": "taken.example.net"})
	tid := itoa(td["id"].(float64))
	code, v, _ := e.do(o, "POST", "/api/apps/"+aid+"/domains/"+tid+"/dns", nil)
	if code != 200 || v["state"] != "conflict" {
		t.Fatalf("conflict: %d %v", code, v)
	}

	// overview
	_, all, _ := e.do(o, "GET", "/api/domains", nil)
	rows := all["domains"].([]any)
	if len(rows) < 5 || all["dns_available"] != true || all["dns_auto"] != true || all["server_ips"].([]any)[0] != "203.0.113.10" {
		t.Fatalf("overview: %v", all)
	}
	first := rows[0].(map[string]any)
	if first["project_slug"] != "shop" || first["app_slug"] != "web" || first["can_manage"] != true {
		t.Fatalf("row: %v", first)
	}
	// a user without access sees nothing
	e.do(o, "POST", "/api/users", map[string]string{"email": "viewer@x.io", "name": "V", "password": "viewer-password1"})
	viewer := e.client()
	if code, _, _ := e.do(viewer, "POST", "/api/auth/login", map[string]string{"email": "viewer@x.io", "password": "viewer-password1"}); code != 200 {
		t.Fatalf("viewer login: %d", code)
	}
	if _, v, _ := e.do(viewer, "GET", "/api/domains", nil); len(v["domains"].([]any)) != 0 {
		t.Fatalf("viewer without projects sees domains: %v", v["domains"])
	}

	// deleting the domain removes the record the gateway created
	e.do(o, "DELETE", "/api/apps/"+aid+"/domains/"+itoa(cd["id"].(float64)), nil)
	eventually(t, "record deleted", func() bool { return fg.record("blog.example.org") == nil })
	_ = sub
}
