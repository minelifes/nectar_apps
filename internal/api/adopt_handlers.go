package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"nectar_apps/internal/adopt"
	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// adoptStatus returns the migration state written by `nectar-apps adopt` (ConfigMap nectar-adopt), the
// Ingress sync result and the hosts that still reach the old proxy through the gateway's fallback.
func (s *Server) adoptStatus(w http.ResponseWriter, r *http.Request, _ *principal) {
	resp := map[string]any{"state": nil, "report": nil, "report_markdown": "", "ingress_sync": nil, "fallback_hits": []any{}}
	if s.cluster != nil {
		var cm kube.Object
		if err := s.cluster.GetObject(r.Context(), "ConfigMap", s.cfg.SystemNamespace, adopt.ConfigMapName, &cm); err == nil {
			var st, rep any
			if json.Unmarshal([]byte(kube.Str(cm, "data", "state.json")), &st) == nil {
				resp["state"] = st
			}
			if json.Unmarshal([]byte(kube.Str(cm, "data", "report.json")), &rep) == nil {
				resp["report"] = rep
			}
			resp["report_markdown"] = kube.Str(cm, "data", "report.md")
		}
	}
	if s.ingSync != nil {
		resp["ingress_sync"] = s.ingSync.Status()
	}
	if s.gateway != nil {
		if hits, err := s.gateway.FallbackHits(r.Context()); err == nil && hits != nil {
			resp["fallback_hits"] = hits
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) resetFallbackHits(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.gateway == nil {
		writeError(w, http.StatusServiceUnavailable, "no gateway configured")
		return
	}
	if _, err := s.gateway.Send(r.Context(), http.MethodDelete, "/api/fallback-hits", nil); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, p, "adopt.fallback_hits.reset", "", nil)
	w.WriteHeader(http.StatusNoContent)
}

// systemNamespaces are never shown as "external".
var systemNamespaces = map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

type externalWorkload struct {
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Replicas int64    `json:"replicas"`
	Ready    int64    `json:"ready"`
	Images   []string `json:"images"`
}

type externalService struct {
	Name  string  `json:"name"`
	Type  string  `json:"type"`
	Ports []int64 `json:"ports"`
}

type externalNamespace struct {
	Name      string             `json:"name"`
	Workloads []externalWorkload `json:"workloads"`
	Services  []externalService  `json:"services"`
	Hosts     []string           `json:"hosts"` // from Ingress objects
	ProjectID *int64             `json:"project_id,omitempty"`
}

// externalNamespaces lists namespaces the panel did not create (read-only view; they can be adopted).
func (s *Server) externalNamespaces(w http.ResponseWriter, r *http.Request, _ *principal) {
	if s.cluster == nil {
		writeError(w, http.StatusServiceUnavailable, "Kubernetes is not configured")
		return
	}
	ctx := r.Context()
	nss, err := s.cluster.List(ctx, "Namespace", "", "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	adopted := map[string]int64{}
	owned := map[string]bool{}
	for _, p := range projects {
		if p.Adopted {
			adopted[p.Namespace] = p.ID
		} else {
			owned[p.Namespace] = true
		}
	}
	deps, _ := s.cluster.List(ctx, "Deployment", "", "")
	sts, _ := s.cluster.List(ctx, "StatefulSet", "", "")
	dss, _ := s.cluster.List(ctx, "DaemonSet", "", "")
	svcs, _ := s.cluster.List(ctx, "Service", "", "")
	ings, _ := s.cluster.List(ctx, "Ingress", "", "")
	out := []externalNamespace{}
	for _, ns := range nss {
		name := kube.Str(ns, "metadata", "name")
		if systemNamespaces[name] || name == s.cfg.SystemNamespace || owned[name] ||
			(kube.Str(ns, "metadata", "labels", deploy.LabelManagedBy) == deploy.ManagedBy && kube.Str(ns, "metadata", "labels", deploy.LabelAdopted) == "") {
			continue
		}
		en := externalNamespace{Name: name, Workloads: []externalWorkload{}, Services: []externalService{}, Hosts: []string{}}
		if id, ok := adopted[name]; ok {
			en.ProjectID = &id
		}
		for _, group := range []struct {
			kind  string
			items []kube.Object
		}{{"Deployment", deps}, {"StatefulSet", sts}, {"DaemonSet", dss}} {
			for _, o := range group.items {
				if kube.Str(o, "metadata", "namespace") != name || kube.Str(o, "metadata", "labels", deploy.LabelManagedBy) == deploy.ManagedBy {
					continue
				}
				wl := externalWorkload{Kind: group.kind, Name: kube.Str(o, "metadata", "name"),
					Replicas: kube.Int(o, "spec", "replicas"), Ready: kube.Int(o, "status", "readyReplicas")}
				if group.kind == "DaemonSet" {
					wl.Replicas, wl.Ready = kube.Int(o, "status", "desiredNumberScheduled"), kube.Int(o, "status", "numberReady")
				}
				for _, c := range kube.Slice(o, "spec", "template", "spec", "containers") {
					wl.Images = append(wl.Images, kube.Str(c, "image"))
				}
				en.Workloads = append(en.Workloads, wl)
			}
		}
		for _, o := range svcs {
			if kube.Str(o, "metadata", "namespace") != name {
				continue
			}
			es := externalService{Name: kube.Str(o, "metadata", "name"), Type: kube.Str(o, "spec", "type"), Ports: []int64{}}
			for _, p := range kube.Slice(o, "spec", "ports") {
				es.Ports = append(es.Ports, kube.Int(p, "port"))
			}
			en.Services = append(en.Services, es)
		}
		for _, o := range ings {
			if kube.Str(o, "metadata", "namespace") != name {
				continue
			}
			for _, rule := range kube.Slice(o, "spec", "rules") {
				if h := kube.Str(rule, "host"); h != "" {
					en.Hosts = append(en.Hosts, h)
				}
			}
		}
		sort.Strings(en.Hosts)
		if name == "default" && len(en.Workloads) == 0 && len(en.Hosts) == 0 {
			continue // only the "kubernetes" Service
		}
		out = append(out, en)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9-]+`)

// adoptNamespace turns an existing namespace into a project. Existing workloads are not modified: the panel
// only manages the apps created in it afterwards, and deleting the project never deletes the namespace.
func (s *Server) adoptNamespace(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.cluster == nil || s.deployer == nil {
		writeError(w, http.StatusServiceUnavailable, "Kubernetes is not configured")
		return
	}
	ns := r.PathValue("ns")
	var in struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	if systemNamespaces[ns] || ns == s.cfg.SystemNamespace {
		writeError(w, http.StatusBadRequest, "system namespaces cannot be adopted")
		return
	}
	ctx := r.Context()
	var nsObj kube.Object
	if err := s.cluster.GetObject(ctx, "Namespace", "", ns, &nsObj); err != nil {
		if kube.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "namespace not found")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if kube.Str(nsObj, "metadata", "labels", deploy.LabelManagedBy) == deploy.ManagedBy {
		writeError(w, http.StatusConflict, "this namespace already belongs to a Nectar project")
		return
	}
	// the panel's apps use the ServiceAccount "app": never take over one the user created
	var sa kube.Object
	if err := s.cluster.GetObject(ctx, "ServiceAccount", ns, deploy.AppServiceAccount, &sa); err == nil &&
		kube.Str(sa, "metadata", "labels", deploy.LabelManagedBy) != deploy.ManagedBy {
		writeError(w, http.StatusConflict, "the namespace has its own ServiceAccount named \"app\"; rename it before adopting the namespace")
		return
	}
	if in.Slug == "" {
		in.Slug = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(ns), "-"), "-")
		if len(in.Slug) > 30 {
			in.Slug = strings.Trim(in.Slug[:30], "-")
		}
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugRe.MatchString(in.Slug) {
		writeError(w, http.StatusBadRequest, "slug must be 1–30 lowercase letters, digits or dashes (not starting or ending with a dash)")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = ns
	}
	if len(in.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name must be 1–100 characters")
		return
	}
	proj, err := s.store.CreateAdoptedProject(ctx, in.Slug, in.Name, ns)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "a project with this slug or namespace already exists")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.deployer.EnsureProject(ctx, proj); err != nil {
		_ = s.store.DeleteProject(ctx, proj.ID)
		writeError(w, http.StatusBadGateway, "could not prepare the namespace: "+err.Error())
		return
	}
	s.audit(r, p, "project.adopt", proj.Slug, map[string]string{"namespace": ns})
	writeJSON(w, http.StatusCreated, proj)
}
