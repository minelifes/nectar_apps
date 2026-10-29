package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLogsEventsUsage(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	_, v, _ := e.do(c, "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password", "client": "mobile"})
	tok := v["access_token"].(string)
	auth := []string{"Authorization", "Bearer " + tok}
	plain := &http.Client{}

	e.do(plain, "POST", "/api/projects", map[string]string{"slug": "shop", "name": "Shop"}, auth...)
	if code, _, _ := e.do(plain, "POST", "/api/projects/1/apps", map[string]any{"slug": "api", "name": "API", "runtime": "image", "config": map[string]any{"image": "nginx:1.29"}}, auth...); code != 201 {
		t.Fatalf("app: %d", code)
	}

	if code, _, lines := e.do(plain, "GET", "/api/apps/1/logs?tail=10", nil, auth...); code != 200 || len(lines) != 1 || lines[0].(map[string]any)["line"] != "hello from api" {
		t.Fatalf("logs: %d %v", code, lines)
	}
	if code, _, evs := e.do(plain, "GET", "/api/apps/1/events", nil, auth...); code != 200 || len(evs) != 1 {
		t.Fatalf("events: %d %v", code, evs)
	}
	if code, v, _ := e.do(plain, "GET", "/api/apps/1/usage?range=24h", nil, auth...); code != 200 || v["range"] != "24h" || v["request_cpu_m"].(float64) <= 0 {
		t.Fatalf("usage: %d %v", code, v)
	}
	if code, v, _ := e.do(plain, "GET", "/api/server/metrics", nil, auth...); code != 200 || len(v["nodes"].([]any)) != 1 {
		t.Fatalf("server metrics: %d %v", code, v)
	}
	// ?access_token= is only accepted on the WebSocket endpoint
	if code, _, _ := e.do(plain, "GET", "/api/apps/1/logs?access_token="+tok, nil); code != 401 {
		t.Fatalf("query token on a normal endpoint: %d", code)
	}

	// live logs over a WebSocket with the token in the query (as browsers must)
	conn, err := net.Dial("tcp", strings.TrimPrefix(e.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	io.WriteString(conn, "GET /api/apps/1/logs/ws?tail=5&access_token="+tok+" HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("upgrade: %v %+v", err, resp)
	}
	var got []string
	for len(got) < 3 {
		var h [2]byte
		if _, err := io.ReadFull(br, h[:]); err != nil {
			t.Fatalf("read: %v (got %v)", err, got)
		}
		n := int(h[1] & 0x7F)
		if n == 126 {
			var b [2]byte
			io.ReadFull(br, b[:])
			n = int(b[0])<<8 | int(b[1])
		}
		p := make([]byte, n)
		io.ReadFull(br, p)
		var m map[string]any
		json.Unmarshal(p, &m)
		switch m["type"] {
		case "pods":
			got = append(got, "pods")
		case "line":
			got = append(got, m["line"].(string))
		}
	}
	if strings.Join(got, "|") != "pods|hello from api|live line" {
		t.Fatalf("stream: %v", got)
	}
}
