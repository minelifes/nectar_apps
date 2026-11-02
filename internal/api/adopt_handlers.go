package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

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
	out, err := s.listExternal(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// projectExternalWorkloads: GET /api/projects/{pid}/external-workloads — what runs in an adopted project's
// namespace without being a Nectar app (read-only; empty for projects the panel created).
func (s *Server) projectExternalWorkloads(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	empty := externalNamespace{Name: proj.Namespace, Workloads: []externalWorkload{}, Services: []externalService{}, Hosts: []string{}}
	if !proj.Adopted || s.cluster == nil {
		writeJSON(w, http.StatusOK, empty)
		return
	}
	out, err := s.listExternal(r.Context(), proj.Namespace)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if len(out) == 1 {
		writeJSON(w, http.StatusOK, out[0])
		return
	}
	writeJSON(w, http.StatusOK, empty)
}

// listExternal returns namespaces the panel did not create (only = just that one) with what runs in them:
// workloads, Services and Ingress hosts. Adopted namespaces carry their project id; Nectar's own objects in
// them are left out.
func (s *Server) listExternal(ctx context.Context, only string) ([]externalNamespace, error) {
	nss, err := s.cluster.List(ctx, "Namespace", "", "")
	if err != nil {
		return nil, err
	}
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return nil, err
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
		if only != "" && name != only {
			continue
		}
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
			if kube.Str(o, "metadata", "namespace") != name || kube.Str(o, "metadata", "labels", deploy.LabelManagedBy) == deploy.ManagedBy {
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
		if name == "default" && len(en.Workloads) == 0 && len(en.Hosts) == 0 && only == "" {
			continue // only the "kubernetes" Service
		}
		out = append(out, en)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9-]+`)

// adoptNamespace turns an existing namespace into a project. Existing workloads are not modified: the panel
// only manages the apps created in it afterwards, and deleting the project never deletes the namespace.
func (s *Server) adoptNamespace(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	proj, code, err := s.adoptNS(r.Context(), r.PathValue("ns"), in.Slug, in.Name)
	if err != nil {
		if code == http.StatusInternalServerError {
			s.serverError(w, r, err)
		} else {
			writeError(w, code, err.Error())
		}
		return
	}
	s.audit(r, p, "project.adopt", proj.Slug, map[string]string{"namespace": proj.Namespace})
	writeJSON(w, http.StatusCreated, proj)
}

// adoptNS turns an existing namespace into an adopted project (its workloads are not changed). On error
// it returns the HTTP status that describes it.
func (s *Server) adoptNS(ctx context.Context, ns, slug, name string) (store.Project, int, error) {
	if s.cluster == nil || s.deployer == nil {
		return store.Project{}, http.StatusServiceUnavailable, errors.New("Kubernetes is not configured")
	}
	if systemNamespaces[ns] || ns == s.cfg.SystemNamespace {
		return store.Project{}, http.StatusBadRequest, errors.New("system namespaces cannot be adopted")
	}
	var nsObj kube.Object
	if err := s.cluster.GetObject(ctx, "Namespace", "", ns, &nsObj); err != nil {
		if kube.IsNotFound(err) {
			return store.Project{}, http.StatusNotFound, errors.New("namespace not found")
		}
		return store.Project{}, http.StatusBadGateway, err
	}
	if kube.Str(nsObj, "metadata", "labels", deploy.LabelManagedBy) == deploy.ManagedBy {
		return store.Project{}, http.StatusConflict, errors.New("this namespace already belongs to a Nectar project")
	}
	// the panel's apps use the ServiceAccount "app": never take over one the user created
	var sa kube.Object
	if err := s.cluster.GetObject(ctx, "ServiceAccount", ns, deploy.AppServiceAccount, &sa); err == nil &&
		kube.Str(sa, "metadata", "labels", deploy.LabelManagedBy) != deploy.ManagedBy {
		return store.Project{}, http.StatusConflict, errors.New("the namespace has its own ServiceAccount named \"app\"; rename it before adopting the namespace")
	}
	if slug == "" {
		slug = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(ns), "-"), "-")
		if len(slug) > 30 {
			slug = strings.Trim(slug[:30], "-")
		}
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRe.MatchString(slug) {
		return store.Project{}, http.StatusBadRequest, errors.New("slug must be 1–30 lowercase letters, digits or dashes (not starting or ending with a dash)")
	}
	if strings.TrimSpace(name) == "" {
		name = ns
	}
	if len(name) > 100 {
		return store.Project{}, http.StatusBadRequest, errors.New("name must be 1–100 characters")
	}
	proj, err := s.store.CreateAdoptedProject(ctx, slug, name, ns)
	if errors.Is(err, store.ErrConflict) {
		return store.Project{}, http.StatusConflict, errors.New("a project with this slug or namespace already exists")
	}
	if err != nil {
		return store.Project{}, http.StatusInternalServerError, err
	}
	if err := s.deployer.EnsureProject(ctx, proj); err != nil {
		_ = s.store.DeleteProject(ctx, proj.ID)
		return store.Project{}, http.StatusBadGateway, errors.New("could not prepare the namespace: " + err.Error())
	}
	return proj, http.StatusCreated, nil
}

// RunAdoptRequests adopts the namespaces the installer asked for (install.sh --adopt writes them to the
// ConfigMap adopt.ProjectRequestsConfigMap, because the installer has no panel login). Requests that
// succeed or can never succeed are removed; the ConfigMap is deleted when none is left.
func (s *Server) RunAdoptRequests(ctx context.Context) {
	if s.cluster == nil {
		return
	}
	for {
		s.processAdoptRequests(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
	}
}

func (s *Server) processAdoptRequests(ctx context.Context) {
	var cm kube.Object
	if err := s.cluster.GetObject(ctx, "ConfigMap", s.cfg.SystemNamespace, adopt.ProjectRequestsConfigMap, &cm); err != nil {
		return
	}
	var left []string
	for _, ns := range strings.Fields(kube.Str(cm, "data", "namespaces")) {
		proj, code, err := s.adoptNS(ctx, ns, "", "")
		switch {
		case err == nil:
			s.log.Info("adopted namespace as a project (installer request)", "namespace", ns, "project", proj.Slug)
			if aerr := s.store.Audit(ctx, nil, "installer", "project.adopt", proj.Slug, "", map[string]string{"namespace": ns}); aerr != nil {
				s.log.Error("audit write failed", "action", "project.adopt", "err", aerr)
			}
		case code >= 500:
			left = append(left, ns) // the cluster or the database: try again later
			s.log.Warn("could not adopt namespace yet", "namespace", ns, "err", err)
		default:
			s.log.Warn("namespace not adopted", "namespace", ns, "err", err)
		}
	}
	if len(left) == 0 {
		_ = s.cluster.Delete(ctx, "ConfigMap", s.cfg.SystemNamespace, adopt.ProjectRequestsConfigMap)
		return
	}
	_, _ = s.cluster.Patch(ctx, "ConfigMap", s.cfg.SystemNamespace, adopt.ProjectRequestsConfigMap,
		map[string]any{"data": map[string]any{"namespaces": strings.Join(left, " ")}})
}
