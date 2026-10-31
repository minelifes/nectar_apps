package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/alerts"
)

func TestPushNotifications(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	var mu sync.Mutex
	var sent []map[string]any
	fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			r.ParseForm()
			if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || strings.Count(r.Form.Get("assertion"), ".") != 2 {
				w.WriteHeader(400)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "ya29.test", "expires_in": 3600})
		case r.URL.Path == "/v1/projects/nectar-push/messages:send":
			if r.Header.Get("Authorization") != "Bearer ya29.test" {
				w.WriteHeader(401)
				return
			}
			var body struct {
				Message map[string]any `json:"message"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Message["token"] == "gone-device-token-00000000" {
				w.WriteHeader(404)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"status": "NOT_FOUND", "details": []any{map[string]any{"errorCode": "UNREGISTERED"}}}})
				return
			}
			mu.Lock()
			sent = append(sent, body.Message)
			mu.Unlock()
			w.Write([]byte(`{"name":"projects/nectar-push/messages/1"}`))
		}
	}))
	defer fcm.Close()
	e.api.pushClient.Endpoint = fcm.URL

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	sa, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": "nectar-push", "client_email": "push@nectar-push.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": fcm.URL + "/token"})

	// off by default: the app learns it should not register
	if _, v, _ := e.do(o, "GET", "/api/push/config", nil); v["enabled"] != false {
		t.Fatalf("config while off: %v", v)
	}
	if code, _, _ := e.do(o, "PUT", "/api/settings/push", map[string]any{"enabled": true, "sender_id": "123"}); code != 400 {
		t.Fatalf("enabled without a key: %d", code)
	}
	settings := map[string]any{"enabled": true, "sender_id": "123456", "service_account": string(sa),
		"android": map[string]any{"api_key": "AIza-android", "app_id": "1:123456:android:abc"}}
	if code, v, _ := e.do(o, "PUT", "/api/settings/push", settings); code != 200 || v["service_account_email"] != "push@nectar-push.iam.gserviceaccount.com" {
		t.Fatalf("save: %d %v", code, v)
	}
	if _, _, body := e.raw(o, "GET", "/api/settings/push", nil); strings.Contains(string(body), "PRIVATE KEY") {
		t.Fatal("service account returned")
	}
	_, v, _ := e.do(o, "GET", "/api/push/config", nil)
	if v["enabled"] != true || v["project_id"] != "nectar-push" || v["android"].(map[string]any)["app_id"] != "1:123456:android:abc" {
		t.Fatalf("app config: %v", v)
	}

	// no device yet → the test explains what to do
	if code, v, _ := e.do(o, "POST", "/api/settings/push/test", nil); code != 502 || !strings.Contains(v["error"].(string), "no device") {
		t.Fatalf("test without devices: %d %v", code, v)
	}
	if code, _, _ := e.do(o, "PUT", "/api/me/push-devices", map[string]any{"token": "short", "platform": "android"}); code != 400 {
		t.Fatalf("bad token accepted: %d", code)
	}
	for _, tok := range []string{"phone-device-token-1234567890", "gone-device-token-00000000"} {
		if code, _, _ := e.do(o, "PUT", "/api/me/push-devices", map[string]any{"token": tok, "platform": "android", "name": "Pixel"}); code != 200 {
			t.Fatalf("register: %d", code)
		}
	}
	if code, v, _ := e.do(o, "POST", "/api/settings/push/test", nil); code != 200 || v["sent"].(float64) != 1 {
		t.Fatalf("test: %d %v", code, v)
	}
	if _, _, list := e.do(o, "GET", "/api/me/push-devices", nil); len(list) != 1 {
		t.Fatalf("the unregistered token was kept: %v", list)
	}

	// an alert channel of kind push reaches the owner (who can see the server)
	err := e.api.SendPush(t.Context(), alerts.ChannelConfig{}, alerts.Notification{Title: "App down", Subject: "shop/api", Message: "not answering",
		Severity: alerts.Critical, State: "firing", Link: "https://panel.example.com/apps/3", Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.api.SendPush(t.Context(), alerts.ChannelConfig{To: "nobody@example.com"}, alerts.Notification{Title: "x"}); err == nil {
		t.Fatal("push to a user without devices succeeded")
	}
	mu.Lock()
	last := sent[len(sent)-1]
	mu.Unlock()
	if last["notification"].(map[string]any)["title"] != "App down · shop/api" || last["data"].(map[string]any)["path"] != "/apps/3" {
		t.Fatalf("message: %v", last)
	}
	// signing out removes the device
	if code, _, _ := e.do(o, "POST", "/api/me/push-devices/unregister", map[string]any{"token": "phone-device-token-1234567890"}); code != 204 {
		t.Fatalf("unregister: %d", code)
	}
	if _, _, list := e.do(o, "GET", "/api/me/push-devices", nil); len(list) != 0 {
		t.Fatalf("devices after unregister: %v", list)
	}
}
