package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/observe"
	"nectar_apps/internal/store"
	"nectar_apps/internal/wsock"
)

// shellCommand starts bash when the image has it, sh otherwise.
var shellCommand = []string{"sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash -l; else exec sh -l; fi"}

// termMsg is one message between the browser terminal and the panel:
//
//	browser → panel  {"t":"i","d":"ls\r"}            keystrokes
//	                 {"t":"r","c":120,"r":32}         terminal size
//	panel → browser  {"t":"o","b":"<base64>"}        output bytes
//	                 {"t":"s","pod":"api-…"}          connected to this pod
//	                 {"t":"x","code":0,"msg":"…"}     the shell ended (or could not start)
type termMsg struct {
	T    string `json:"t"`
	D    string `json:"d,omitempty"`
	B    string `json:"b,omitempty"`
	C    int    `json:"c,omitempty"`
	R    int    `json:"r,omitempty"`
	Pod  string `json:"pod,omitempty"`
	Code *int   `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

// appExecWS opens a shell in a running container of the app (web terminal).
func (s *Server) appExecWS(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	s.workloadExecWS(w, r, p, appWorkload(app, proj))
}

// addonExecWS opens a shell in the database container (psql, mysql, redis-cli … are on the image).
func (s *Server) addonExecWS(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	s.workloadExecWS(w, r, p, addonWorkload(a, proj))
}

func (s *Server) workloadExecWS(w http.ResponseWriter, r *http.Request, p *principal, wl workload) {
	if s.kube == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes is not configured")
		return
	}
	if !wsock.IsUpgrade(r) {
		writeError(w, http.StatusBadRequest, "this endpoint needs a WebSocket connection")
		return
	}
	if o := r.Header.Get("Origin"); o != "" && !sameOrigin(r) && !(s.cfg.Dev && isLocalOrigin(o)) {
		writeError(w, http.StatusForbidden, "cross-origin WebSocket refused")
		return
	}
	// pick the pod after the upgrade: a browser cannot read the body of a refused WebSocket handshake, so
	// "no running pod" must arrive as a message, not as a 409
	pod, container, podErr := s.execPod(r.Context(), wl, r.URL.Query().Get("pod"))
	conn, err := wsock.Upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	send := func(m termMsg) error {
		b, _ := json.Marshal(m)
		return conn.WriteText(b)
	}
	fail := func(msg string) {
		code := -1
		_ = send(termMsg{T: "x", Code: &code, Msg: msg})
	}

	if podErr != nil {
		fail(podErr.Error())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dctx, dcancel := context.WithTimeout(ctx, 20*time.Second)
	ex, err := s.kube.Exec(dctx, wl.namespace, pod, container, shellCommand, true)
	dcancel()
	if err != nil {
		s.log.Warn("exec", wl.kind, wl.ref, "pod", pod, "container", container, "err", err)
		fail("could not open a shell: " + err.Error())
		return
	}
	defer ex.Close()
	s.audit(r, p, wl.kind+".exec", wl.ref, map[string]string{"pod": pod})
	_ = send(termMsg{T: "s", Pod: pod})

	// browser → container
	go func() {
		defer cancel()
		_ = conn.ReadLoop(func(b []byte) {
			var m termMsg
			if json.Unmarshal(b, &m) != nil {
				return
			}
			switch m.T {
			case "i":
				_ = ex.WriteMessage(wsock.Binary, append([]byte{kube.ExecStdin}, m.D...))
			case "r":
				if m.C > 0 && m.R > 0 && m.C < 1000 && m.R < 1000 {
					size, _ := json.Marshal(map[string]int{"Width": m.C, "Height": m.R})
					_ = ex.WriteMessage(wsock.Binary, append([]byte{kube.ExecResize}, size...))
				}
			}
		})
	}()
	// keep proxies from closing an idle terminal
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if conn.Ping() != nil {
					cancel()
					return
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		ex.Close() // unblocks ReadMessage below when the browser leaves
	}()

	// container → browser
	for {
		op, msg, err := ex.ReadMessage()
		if err != nil {
			if ctx.Err() == nil {
				code := 0
				_ = send(termMsg{T: "x", Code: &code, Msg: "connection closed"})
			}
			return
		}
		if op != wsock.Binary || len(msg) == 0 {
			continue
		}
		switch msg[0] {
		case kube.ExecStdout, kube.ExecStderr:
			if len(msg) > 1 {
				if send(termMsg{T: "o", B: base64.StdEncoding.EncodeToString(msg[1:])}) != nil {
					return
				}
			}
		case kube.ExecError:
			code, text := execExit(msg[1:])
			_ = send(termMsg{T: "x", Code: &code, Msg: text})
			return
		}
	}
}

// execExit reads the exit status the API server sends on the error channel.
func execExit(b []byte) (int, string) {
	var st struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Details struct {
			Causes []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"causes"`
		} `json:"details"`
	}
	if json.Unmarshal(b, &st) != nil {
		return -1, strings.TrimSpace(string(b))
	}
	if st.Status == "Success" {
		return 0, "exited"
	}
	for _, c := range st.Details.Causes {
		if c.Reason == "ExitCode" {
			if n, err := strconv.Atoi(c.Message); err == nil {
				return n, "exited with code " + c.Message
			}
		}
	}
	return -1, st.Message
}

// execPod picks the pod to open a shell in: the requested one, or the newest running workload pod.
func (s *Server) execPod(ctx context.Context, wl workload, want string) (string, string, error) {
	pods, err := wl.pods(ctx, s.kube)
	if err != nil {
		return "", "", err
	}
	var best, fallback *observe.Pod
	crashing := false
	for i := range pods {
		p := &pods[i]
		if p.Phase != "Running" {
			continue
		}
		if !slices.Contains(p.Running, execContainer(*p)) {
			crashing = true // phase Running, but the container is restarting (crash loop) or not started yet
			if p.Name == want {
				return "", "", errNoPod("the container of " + want + " is not running (restarting after a crash?): check the Logs tab")
			}
			continue
		}
		if want != "" {
			if p.Name == want {
				return p.Name, execContainer(*p), nil
			}
			continue
		}
		switch p.Role { // pods are sorted oldest first: the last one is the newest
		case "workload":
			best = p
		case "":
			fallback = p // apps deployed before roles existed, adopted workloads, databases
		}
	}
	if want != "" {
		return "", "", errNoPod("pod " + want + " is not running")
	}
	if best == nil {
		best = fallback
	}
	if best == nil && crashing {
		return "", "", errNoPod("the " + wl.noun() + "'s container is not running (restarting after a crash?): check the Logs tab")
	}
	if best == nil {
		if wl.kind == "addon" {
			return "", "", errNoPod("no running pod: start the database first")
		}
		return "", "", errNoPod("no running pod: start or deploy the app first")
	}
	return best.Name, execContainer(*best), nil
}

// execContainer is the container to open the shell in: "app" (every app the panel deploys), otherwise the
// pod's first container (adopted workloads keep their own names).
func execContainer(p observe.Pod) string {
	for _, c := range p.Containers {
		if c == "app" {
			return c
		}
	}
	if len(p.Containers) > 0 {
		return p.Containers[0]
	}
	return "app"
}

type errNoPod string

func (e errNoPod) Error() string { return string(e) }

func (wl workload) noun() string {
	if wl.kind == "addon" {
		return "database"
	}
	return "app"
}
