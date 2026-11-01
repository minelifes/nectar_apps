package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/cli"
	"nectar_apps/internal/wsock"
)

// `nectar db connect`: a local port → WebSocket tunnel through the panel → the database inside the cluster.
func TestDatabaseTunnel(t *testing.T) {
	e := newEnv(t)
	_, v, _ := e.do(e.client(), "POST", "/api/setup", map[string]string{"token": "setup-123", "email": "o@x.io", "name": "O", "password": "owner-password", "client": "mobile"})
	tok := v["access_token"].(string)
	auth := []string{"Authorization", "Bearer " + tok}
	o := &http.Client{}
	e.do(o, "POST", "/api/projects", map[string]string{"slug": "vps", "name": "VPS"}, auth...)
	code, db, _ := e.do(o, "POST", "/api/projects/1/addons", map[string]any{"kind": "mysql", "slug": "mysql"}, auth...)
	if code != 201 {
		t.Fatalf("create: %d %v", code, db)
	}
	id := int64(db["id"].(float64))

	// the "database": an echo server; remember where the panel dialed
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	var mu sync.Mutex
	var dialed []string
	e.api.tunnelDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		dialed = append(dialed, addr)
		mu.Unlock()
		return (&net.Dialer{}).DialContext(ctx, "tcp", echo.Addr().String())
	}

	c := &cli.Client{URL: e.srv.URL, Token: tok}
	d, _, err := c.ResolveDatabase(t.Context(), "vps/mysql", "")
	if err != nil || d.ID != id || d.Port != 3306 {
		t.Fatalf("resolve: %+v %v", d, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)
	go func() {
		served <- cli.ServeTunnel(ctx, ln, func(ctx context.Context) (*wsock.Conn, error) { return c.OpenTunnel(ctx, id) }, io.Discard)
	}()
	for i := 0; i < 2; i++ { // two client connections, two tunnels
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		msg := strings.Repeat("select 1; ", 5000) // larger than one read buffer
		go conn.Write([]byte(msg))
		got := make([]byte, len(msg))
		if _, err := io.ReadFull(conn, got); err != nil || string(got) != msg {
			t.Fatalf("echo %d: %v (%d bytes)", i, err, len(got))
		}
		conn.Close()
	}
	cancel()
	if err := <-served; err != nil {
		t.Fatalf("serve: %v", err)
	}
	mu.Lock()
	if len(dialed) != 2 || dialed[0] != "mysql.prj-vps.svc.cluster.local:3306" {
		t.Fatalf("dialed %v", dialed)
	}
	mu.Unlock()
	if code, _, list := e.do(o, "GET", "/api/audit?limit=20", nil, auth...); code != 200 || !strings.Contains(stringsOf(list), "addon.tunnel") {
		t.Fatalf("audit: %d", code)
	}

	// refused: not a WebSocket, a stopped database, a database the panel cannot reach
	if code, _, _ := e.do(o, "GET", "/api/addons/"+itoa(float64(id))+"/tunnel", nil, auth...); code != 400 {
		t.Fatalf("plain GET: %d", code)
	}
	e.do(o, "POST", "/api/addons/"+itoa(float64(id))+"/stop", nil, auth...)
	if _, err := c.OpenTunnel(t.Context(), id); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("stopped: %v", err)
	}
	e.do(o, "POST", "/api/addons/"+itoa(float64(id))+"/start", nil, auth...)
	e.api.tunnelDial = func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: io.ErrUnexpectedEOF}
	}
	if _, err := c.OpenTunnel(t.Context(), id); err == nil || !strings.Contains(err.Error(), "cannot reach the database") {
		t.Fatalf("unreachable: %v", err)
	}
}

func stringsOf(list []any) string {
	var b strings.Builder
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			b.WriteString(m["action"].(string) + " ")
		}
	}
	return b.String()
}
