package api

import (
	"testing"
	"time"
)

// The gateway allowlist is set through the panel; the addresses of signed-in people are reported to the
// gateway (once per address and TrustEvery) so its abuse detection doesn't lock them out.
func TestAllowlistAndTrustedClients(t *testing.T) {
	e, fg := newEnvGW(t)
	fg.setFeatures("web-options")
	o := e.owner()
	if code, _, _ := e.do(o, "PUT", "/api/gateway/settings", map[string]any{"allow_ips": []string{"203.0.113.7"}}); code != 409 {
		t.Fatalf("allowlist on an old gateway: %d", code)
	}
	trusted := func() []string {
		fg.mu.Lock()
		defer fg.mu.Unlock()
		return append([]string(nil), fg.trusted...)
	}
	time.Sleep(50 * time.Millisecond)
	if got := trusted(); len(got) != 0 {
		t.Fatalf("reported to a gateway without the feature, or a local address: %v", got)
	}

	fg.setFeatures("web-options", "allow-ips")
	code, v, _ := e.do(o, "PUT", "/api/gateway/settings", map[string]any{"allow_ips": []string{" 203.0.113.7 ", "198.51.100.0/24", ""}})
	if list, _ := v["allow_ips"].([]any); code != 200 || len(list) != 2 || list[0] != "203.0.113.7" {
		t.Fatalf("allowlist: %d %v", code, v)
	}
	if code, v, _ := e.do(o, "PUT", "/api/gateway/settings", map[string]any{"allow_ips": []string{}}); code != 200 || len(v["allow_ips"].([]any)) != 0 {
		t.Fatalf("clear: %d %v", code, v)
	}

	// a signed-in person behind the gateway (X-Forwarded-For): reported once
	xff := []string{"X-Forwarded-For", "203.0.113.9"}
	for range 5 {
		if code, _, _ := e.do(o, "GET", "/api/me", nil, xff...); code != 200 {
			t.Fatalf("me: %d", code)
		}
	}
	if code, ip, _ := e.do(o, "GET", "/api/gateway/client-ip", nil, xff...); code != 200 || ip["ip"] != "203.0.113.9" {
		t.Fatalf("client ip: %d %v", code, ip)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(trusted()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := trusted(); len(got) != 1 || got[0] != "203.0.113.9 43200" {
		t.Fatalf("trusted: %v", got)
	}
	// someone who is not signed in is not reported
	if code, _, _ := e.do(e.client(), "GET", "/api/me", nil, "X-Forwarded-For", "203.0.113.50"); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	time.Sleep(50 * time.Millisecond)
	if got := trusted(); len(got) != 1 {
		t.Fatalf("anonymous caller trusted: %v", got)
	}
}
