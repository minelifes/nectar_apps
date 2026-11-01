package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
	"nectar_apps/internal/wsock"
)

// addonTunnel: GET /api/addons/{aid}/tunnel (WebSocket) — a TCP connection to a database for clients outside
// the cluster: `nectar db connect` listens on a local port and opens one tunnel per connection. Binary
// messages carry the raw bytes both ways. Databases stay unreachable from outside: the panel dials them
// inside the cluster for a signed-in user who may see their credentials (env.secrets.view), and every tunnel
// is in the audit log. A client still needs the database password.
func (s *Server) addonTunnel(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	if !wsock.IsUpgrade(r) {
		writeError(w, http.StatusBadRequest, "this endpoint needs a WebSocket connection (use `nectar db connect`)")
		return
	}
	if r.Header.Get("Origin") != "" && !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin WebSocket refused")
		return
	}
	if a.Stopped {
		writeError(w, http.StatusConflict, "the database is stopped: start it first")
		return
	}
	k, ok := deploy.AddonKindByID(a.Kind)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown database kind")
		return
	}
	addr := net.JoinHostPort(a.Slug+"."+proj.Namespace+".svc.cluster.local", strconv.Itoa(k.Port))
	dial := s.tunnelDial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	}
	// dial before the upgrade: a client that is not a browser reads the error of a refused handshake
	dctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	db, err := dial(dctx, "tcp", addr)
	cancel()
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot reach the database (is it running?): "+err.Error())
		return
	}
	defer db.Close()
	conn, err := wsock.Upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	s.audit(r, p, "addon.tunnel", proj.Slug+"/"+a.Slug, map[string]any{"from": clientIP(r)})

	done := make(chan struct{})
	// client → database
	go func() {
		defer close(done)
		for {
			op, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if op != wsock.Binary || len(msg) == 0 {
				continue
			}
			if _, err := db.Write(msg); err != nil {
				return
			}
		}
	}()
	// keep proxies from closing an idle tunnel (a client may sit idle between queries)
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if conn.Ping() != nil {
					return
				}
			}
		}
	}()
	// database → client
	go func() {
		<-done
		db.Close() // unblocks the read below when the client leaves
	}()
	buf := make([]byte, 32<<10)
	for {
		n, err := db.Read(buf)
		if n > 0 {
			if werr := conn.WriteMessage(wsock.Binary, buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				s.log.Debug("tunnel", "addon", proj.Slug+"/"+a.Slug, "err", err)
			}
			return
		}
	}
}
