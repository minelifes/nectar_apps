package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/wsock"
)

func TestWebTerminal(t *testing.T) {
	e := newEnv(t)
	_, v, _ := e.do(e.client(), "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password", "client": "mobile"})
	tok := v["access_token"].(string)
	auth := []string{"Authorization", "Bearer " + tok}
	plain := &http.Client{}
	e.do(plain, "POST", "/api/projects", map[string]string{"slug": "shop", "name": "Shop"}, auth...)
	e.do(plain, "POST", "/api/projects/1/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image", "config": map[string]any{"image": "nginx:1.29"}}, auth...)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/apps/1/exec/ws?access_token=" + tok
	c, _, err := wsock.Dial(ctx, ws, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	read := func() termMsg {
		t.Helper()
		_, b, err := c.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var m termMsg
		json.Unmarshal(b, &m)
		return m
	}
	if m := read(); m.T != "s" || m.Pod != "api-1" {
		t.Fatalf("start: %+v", m)
	}
	c.WriteText([]byte(`{"t":"r","c":100,"r":30}`))
	if m := read(); m.T != "o" || decode64(m.B) != `size:{"Height":30,"Width":100}` {
		t.Fatalf("resize: %+v %q", m, decode64(m.B))
	}
	c.WriteText([]byte(`{"t":"i","d":"ls\r"}`))
	if m := read(); m.T != "o" || decode64(m.B) != "echo:ls\r" {
		t.Fatalf("output: %+v", m)
	}
	c.WriteText([]byte(`{"t":"i","d":"exit\r"}`))
	if m := read(); m.T != "x" || m.Code == nil || *m.Code != 3 {
		t.Fatalf("exit: %+v", m)
	}

	// a crash-looping pod (phase Running, container waiting) cannot take a shell: the reason arrives in band
	c2, _, err := wsock.Dial(ctx, ws+"&pod=api-2", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, b, _ := c2.ReadMessage()
	var m termMsg
	json.Unmarshal(b, &m)
	if m.T != "x" || !strings.Contains(m.Msg, "not running") {
		t.Fatalf("crash-looping pod: %+v", m)
	}
	c2.Close()

	// the query token is accepted on WebSocket upgrades only
	if code, _, _ := e.do(plain, "GET", "/api/apps/1/exec/ws?access_token="+tok, nil); code != 401 {
		t.Fatalf("plain GET with query token: %d", code)
	}
}

func decode64(s string) string {
	b, _ := base64.StdEncoding.DecodeString(s)
	return string(b)
}
