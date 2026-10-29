package api

import "testing"

func TestOnboardingAndProjectActivity(t *testing.T) {
	e := newEnv(t)
	o := e.owner()

	if code, v, _ := e.do(o, "GET", "/api/onboarding", nil); code != 200 || v["done"] != false {
		t.Fatalf("onboarding: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "PUT", "/api/onboarding", map[string]bool{"done": true}); code != 200 {
		t.Fatalf("finish: %d", code)
	}
	if _, v, _ := e.do(o, "GET", "/api/onboarding", nil); v["done"] != true || v["done_at"] == nil {
		t.Fatalf("after finish: %v", v)
	}

	if code, _, _ := e.do(o, "POST", "/api/settings/dns-check", map[string]string{"base_domain": "*.bad"}); code != 400 {
		t.Fatalf("bad base domain: %d", code)
	}
	code, v, _ := e.do(o, "POST", "/api/settings/dns-check", map[string]string{"base_domain": "apps.example.invalid"})
	if code != 200 || v["ok"] != false || v["host"] != "*.apps.example.invalid" {
		t.Fatalf("dns check: %d %v", code, v)
	}

	// project activity: only this project's entries (shop2 shares the prefix "shop")
	e.do(o, "POST", "/api/projects", map[string]string{"slug": "shop", "name": "Shop"})
	e.do(o, "POST", "/api/projects", map[string]string{"slug": "shop2", "name": "Shop 2"})
	e.do(o, "POST", "/api/projects/1/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image", "config": map[string]any{"image": "nginx:1.29"}})
	e.do(o, "POST", "/api/projects/2/apps", map[string]any{"slug": "web", "name": "Web", "runtime": "image", "config": map[string]any{"image": "nginx:1.29"}})
	code, _, list := e.do(o, "GET", "/api/projects/1/activity", nil)
	if code != 200 || len(list) != 2 {
		t.Fatalf("activity: %d %v", code, list)
	}
	for _, it := range list {
		m := it.(map[string]any)
		if m["target"] != "shop" && m["target"] != "shop/api" {
			t.Fatalf("foreign entry: %v", m)
		}
		if m["ip"] != "" {
			t.Fatalf("ip leaked: %v", m)
		}
	}
}
