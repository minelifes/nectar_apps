package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"nectar_apps/internal/gateway"
	"nectar_apps/internal/kube"
)

// Several edge nodes: the primary nectar-gateway (Deployment nectar-gateway on the node labelled
// nectar.dev/edge=true) issues certificates and is what the panel talks to; nodes labelled
// nectar.dev/edge-replica=true run a replica (DaemonSet nectar-gateway-edge) that copies the primary every
// few seconds. DNS then points to every edge node.

const edgeDaemonSet = "nectar-gateway-edge"

// edgeStatus: GET /api/server/edge (server.view).
func (s *Server) edgeStatus(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needKube(w) {
		return
	}
	nodes, err := s.kube.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	type replicaView struct {
		Node     string     `json:"node"`
		Address  string     `json:"address"`
		Ready    bool       `json:"ready"` // node ready
		Synced   bool       `json:"synced"`
		LastSeen *time.Time `json:"last_seen,omitempty"`
		Version  string     `json:"version,omitempty"`
		Error    string     `json:"error,omitempty"`
	}
	out := map[string]any{"primary": "", "replicas": []replicaView{}, "supported": false}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	seen := map[string]gateway.Replica{}
	if s.gateway != nil {
		if v, err := s.gateway.Version(ctx); err == nil && v.Has(gateway.FeatureReplicas) {
			out["supported"] = true
			if list, err := s.gateway.Replicas(ctx); err == nil {
				for _, rp := range list {
					seen[rp.Node] = rp
				}
			}
		}
	}
	var replicas []replicaView
	for _, n := range nodes {
		if n.Edge {
			out["primary"] = n.Name
		}
		if !n.EdgeReplica {
			continue
		}
		v := replicaView{Node: n.Name, Address: n.ExternalIP, Ready: n.Ready}
		if v.Address == "" {
			v.Address = n.InternalIP
		}
		if rp, ok := seen[n.Name]; ok {
			last := rp.LastSeen
			v.LastSeen, v.Version, v.Error = &last, rp.Version, rp.Error
			v.Synced = rp.Error == "" && time.Since(rp.LastSeen) < time.Minute
		}
		replicas = append(replicas, v)
	}
	if replicas != nil {
		out["replicas"] = replicas
	}
	writeJSON(w, http.StatusOK, out)
}

// setEdgeReplica labels (or unlabels) a node as an extra edge node and reconciles the replica DaemonSet.
func (s *Server) setEdgeReplica(ctx context.Context, node kube.Object, name string, on bool) error {
	labels, _ := kube.Field(node, "metadata", "labels").(map[string]any)
	if labels[kube.LabelEdge] == "true" {
		return errors.New("this is the primary edge node: nectar-gateway already runs here")
	}
	if on && s.gateway != nil {
		v, err := s.gateway.Version(ctx)
		if err != nil {
			return fmt.Errorf("nectar-gateway: %w", err)
		}
		if !v.Has(gateway.FeatureReplicas) {
			return errors.New("update nectar-gateway first: this version cannot run on several edge nodes")
		}
	}
	var value any // nil removes the label
	if on {
		value = "true"
	}
	if _, err := s.kube.Patch(ctx, "Node", "", name, map[string]any{"metadata": map[string]any{"labels": map[string]any{kube.LabelEdgeReplica: value}}}); err != nil {
		return fmt.Errorf("kubernetes: %w", err)
	}
	if err := s.reconcileEdge(ctx); err != nil {
		return err
	}
	if st := s.loadSettings(ctx); len(st.PublicIPs) == 0 {
		s.refreshDNS(st) // DNS records now include (or no longer include) this node
	}
	return nil
}

// reconcileEdge keeps the replica DaemonSet in step with the primary gateway (same image and settings):
// created while any node is labelled nectar.dev/edge-replica=true, deleted when none is.
func (s *Server) reconcileEdge(ctx context.Context) error {
	if s.kube == nil {
		return nil
	}
	nodes, err := s.kube.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("kubernetes: %w", err)
	}
	ns := s.cfg.SystemNamespace
	if !slices.ContainsFunc(nodes, func(n kube.Node) bool { return n.EdgeReplica }) {
		if err := s.kube.Delete(ctx, "DaemonSet", ns, edgeDaemonSet); err != nil && !kube.IsNotFound(err) {
			return fmt.Errorf("kubernetes: %w", err)
		}
		return nil
	}
	var dep kube.Object
	if err := s.kube.GetObject(ctx, "Deployment", ns, "nectar-gateway", &dep); err != nil {
		return fmt.Errorf("the primary gateway (Deployment nectar-gateway): %w", err)
	}
	ds, err := edgeDaemonSetFor(dep, ns, s.cfg.GatewayURL)
	if err != nil {
		return err
	}
	if _, err := s.kube.Apply(ctx, ds); err != nil {
		return fmt.Errorf("kubernetes: %w", err)
	}
	return nil
}

// edgeDaemonSetFor builds the replica DaemonSet from the primary's Deployment.
func edgeDaemonSetFor(dep kube.Object, ns, primaryURL string) (kube.Object, error) {
	containers, _ := kube.Field(dep, "spec", "template", "spec", "containers").([]any)
	var gw map[string]any
	for _, c := range containers {
		if m, ok := c.(map[string]any); ok && m["name"] == "gateway" {
			gw = m
		}
	}
	if gw == nil {
		return nil, errors.New("the primary gateway Deployment has no container named gateway")
	}
	if primaryURL == "" {
		primaryURL = "http://nectar-gateway-admin." + ns + ".svc:8081"
	}
	adminPort := "8081"
	env := []any{}
	for _, e := range asSlice(gw["env"]) {
		m, _ := e.(map[string]any)
		switch m["name"] {
		case "ACME", "ACME_EMAIL", "DB_PATH", "NOTIFY_URL", "PRIMARY_URL", "NODE_NAME", "DNS_PROVIDER", "DNS_API_TOKEN":
			continue // replicas never issue certificates; their database is a copy
		case "ADMIN_ADDR":
			if v, _ := m["value"].(string); len(v) > 1 && v[0] == ':' {
				adminPort = v[1:]
			}
		}
		env = append(env, m)
	}
	env = append(env,
		map[string]any{"name": "DB_PATH", "value": "/data/replica.db"},
		map[string]any{"name": "PRIMARY_URL", "value": primaryURL},
		map[string]any{"name": "NODE_NAME", "valueFrom": map[string]any{"fieldRef": map[string]any{"fieldPath": "spec.nodeName"}}},
	)
	probe := func(path string, period int) map[string]any {
		return map[string]any{"httpGet": map[string]any{"path": path, "port": atoiOr(adminPort, 8081)}, "periodSeconds": period}
	}
	container := map[string]any{
		"name": "gateway", "image": gw["image"], "imagePullPolicy": "IfNotPresent", "env": env,
		"securityContext": gw["securityContext"], "resources": gw["resources"],
		"readinessProbe": probe("/readyz", 5), "livenessProbe": probe("/healthz", 20),
		"volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/data"}},
	}
	labels := map[string]any{"app.kubernetes.io/name": edgeDaemonSet, "app.kubernetes.io/part-of": "nectar"}
	return kube.Object{
		"apiVersion": "apps/v1", "kind": "DaemonSet",
		"metadata": map[string]any{"name": edgeDaemonSet, "namespace": ns,
			"labels": map[string]any{"app.kubernetes.io/name": edgeDaemonSet, "app.kubernetes.io/part-of": "nectar", "app.kubernetes.io/managed-by": "nectar-apps"}},
		"spec": map[string]any{
			"selector":       map[string]any{"matchLabels": map[string]any{"app.kubernetes.io/name": edgeDaemonSet}},
			"updateStrategy": map[string]any{"type": "RollingUpdate", "rollingUpdate": map[string]any{"maxUnavailable": 1}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"hostNetwork": true, "dnsPolicy": "ClusterFirstWithHostNet",
					"nodeSelector":                  map[string]any{"nectar.dev/edge-replica": "true"},
					"priorityClassName":             "system-cluster-critical",
					"terminationGracePeriodSeconds": 40,
					"automountServiceAccountToken":  false,
					"containers":                    []any{container},
					"volumes":                       []any{map[string]any{"name": "data", "emptyDir": map[string]any{}}},
				},
			},
		},
	}, nil
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func atoiOr(s string, def int) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return def
	}
	return n
}

// RunEdge re-applies the replica DaemonSet every few minutes, so replicas follow gateway updates.
func (s *Server) RunEdge(ctx context.Context) {
	for {
		if err := s.reconcileEdge(ctx); err != nil {
			s.log.Warn("edge replicas", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Minute):
		}
	}
}
