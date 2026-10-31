package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// joinInfo is how another machine joins the cluster (install.sh stored it in Secret nectar-join).
func (s *Server) joinInfo(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needKube(w) {
		return
	}
	var sec kube.Object
	err := s.kube.GetObject(r.Context(), "Secret", s.cfg.SystemNamespace, "nectar-join", &sec)
	if kube.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "no join details: re-run install.sh on the server (it stores them), or add nodes with the k3s docs")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	dec := func(k string) string {
		v, _ := decodeB64(kube.Str(sec, "data", k))
		return strings.TrimSpace(v)
	}
	joinURL, token := dec("url"), dec("token")
	s.audit(r, p, "server.join_info", "", nil) // shows the cluster token
	cmd := "curl -fsSL https://raw.githubusercontent.com/minelifes/nectar_apps/main/deploy/install.sh | sudo sh -s -- --join " + joinURL + " --token " + token
	if s.cfg.RegistryPassword != "" {
		cmd += " --registry-password " + s.cfg.RegistryPassword
	}
	host := joinURL
	if u, err := url.Parse(joinURL); err == nil {
		host = u.Hostname()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url": joinURL, "command": cmd,
		"requirements": []string{
			"A Linux machine that reaches " + host + " on port 6443/tcp (Kubernetes API) and 8472/udp (pod network).",
			"On the server, allow the new machine through the firewall: sudo ufw allow from <new-ip>.",
			"The new node runs apps. To serve traffic there too (a second gateway on ports 80/443), make it an edge node on the Server page.",
		},
	})
}

func decodeB64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	return string(b), err
}

func (s *Server) nodeAction(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needKube(w) {
		return
	}
	name, action := r.PathValue("name"), r.PathValue("action")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var node kube.Object
	if err := s.kube.GetObject(ctx, "Node", "", name, &node); kube.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	switch action {
	case "cordon", "uncordon":
		if err := s.kube.Cordon(ctx, name, action == "cordon"); err != nil {
			writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
			return
		}
		s.audit(r, p, "node."+action, name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "drain":
		if r.URL.Query().Get("force") != "true" {
			if msg, err := s.drainBlocked(ctx, node, name); err != nil {
				writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
				return
			} else if msg != "" {
				writeError(w, http.StatusConflict, msg)
				return
			}
		}
		res, err := s.drain(ctx, name)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.audit(r, p, "node.drain", name, res)
		writeJSON(w, http.StatusOK, res)
	case "edge-on", "edge-off":
		if err := s.setEdgeReplica(ctx, node, name, action == "edge-on"); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.audit(r, p, "node."+action, name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeError(w, http.StatusNotFound, "unknown action")
	}
}

// drainBlocked says why draining would take things offline: the edge node runs nectar-gateway (all
// traffic), and without another ready, schedulable node the evicted pods (the panel too) have nowhere to go.
func (s *Server) drainBlocked(ctx context.Context, node kube.Object, name string) (string, error) {
	if labels, _ := kube.Field(node, "metadata", "labels").(map[string]any); labels["nectar.dev/edge"] == "true" {
		return "this is the edge node: nectar-gateway (all app traffic) runs here and stops while it is drained; move the edge first, or drain with force", nil
	}
	nodes, err := s.kube.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	for _, n := range nodes {
		if n.Name != name && n.Ready && !n.Unschedulable {
			return "", nil
		}
	}
	return "no other ready node can take the pods (the panel included): add a node first, or drain with force", nil
}

type drainResult struct {
	Evicted []string `json:"evicted"`
	Failed  []string `json:"failed,omitempty"` // "ns/pod: reason" (e.g. a PodDisruptionBudget)
	Skipped int      `json:"skipped"`          // DaemonSet and static pods stay
}

// drain cordons a node and evicts its pods (like kubectl drain --ignore-daemonsets --delete-emptydir-data).
func (s *Server) drain(ctx context.Context, node string) (drainResult, error) {
	res := drainResult{Evicted: []string{}}
	if err := s.kube.Cordon(ctx, node, true); err != nil {
		return res, err
	}
	var pods struct {
		Items []kube.Object `json:"items"`
	}
	if err := s.kube.Get(ctx, "/api/v1/pods?fieldSelector="+url.QueryEscape("spec.nodeName="+node), &pods); err != nil {
		return res, err
	}
	for _, p := range pods.Items {
		_, ns, name := kube.Meta(p)
		phase := kube.Str(p, "status", "phase")
		if phase == "Succeeded" || phase == "Failed" || skipOnDrain(p) {
			res.Skipped++
			continue
		}
		if err := s.kube.Evict(ctx, ns, name); err != nil {
			var se *kube.StatusError
			reason := err.Error()
			if errors.As(err, &se) && se.Code == http.StatusTooManyRequests {
				reason = "a disruption budget does not allow it now"
			}
			res.Failed = append(res.Failed, ns+"/"+name+": "+reason)
			continue
		}
		res.Evicted = append(res.Evicted, ns+"/"+name)
	}
	return res, nil
}

func skipOnDrain(p kube.Object) bool {
	if _, mirror := kube.Field(p, "metadata", "annotations", "kubernetes.io/config.mirror").(string); mirror {
		return true
	}
	for _, o := range kube.Slice(p, "metadata", "ownerReferences") {
		if kube.Str(o, "kind") == "DaemonSet" {
			return true
		}
	}
	return false
}

// deleteNode removes a drained node from the cluster (the machine keeps k3s until it is uninstalled there).
func (s *Server) deleteNode(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.needKube(w) {
		return
	}
	name := r.PathValue("name")
	var node kube.Object
	if err := s.kube.GetObject(r.Context(), "Node", "", name, &node); kube.IsNotFound(err) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	labels, _ := kube.Field(node, "metadata", "labels").(map[string]any)
	switch {
	case labels["node-role.kubernetes.io/control-plane"] == "true" || labels["node-role.kubernetes.io/master"] == "true":
		writeError(w, http.StatusConflict, "the server (control-plane) node cannot be removed")
		return
	case labels["nectar.dev/edge"] == "true":
		writeError(w, http.StatusConflict, "this is the edge node (nectar-gateway and DNS point here); move the edge first")
		return
	case kube.Field(node, "spec", "unschedulable") != true:
		writeError(w, http.StatusConflict, "drain the node first")
		return
	}
	if err := s.kube.Delete(r.Context(), "Node", "", name); err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	s.audit(r, p, "node.delete", name, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "On the machine, run /usr/local/bin/k3s-agent-uninstall.sh to remove k3s."})
}
