package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// Importing turns a Deployment or StatefulSet that ran in an adopted namespace before Nectar into a
// Nectar app: image, ports, env (also from Secrets and ConfigMaps), mounted config files, replicas,
// resources, health probe and its existing volumes (the same PVCs: no data is copied). The original is
// scaled to 0 and annotated, and the Services that selected its pods select the app's pods instead, so
// Ingress hosts and the other workloads keep reaching it. Deleting the app undoes this.

const (
	annImportedAs       = "nectar.dev/imported-as"       // on the original workload: the app slug
	annImportedReplicas = "nectar.dev/imported-replicas" // on the original workload: replicas before
	annImportedBy       = "nectar.dev/imported-by"       // on a repointed Service: the app slug
	annOrigSelector     = "nectar.dev/original-selector" // on a repointed Service: JSON selector before
	annOrigPorts        = "nectar.dev/original-ports"    // on a repointed Service: JSON ports before
)

// importPlan is what an import would create (GET returns it so the user can review it).
type importPlan struct {
	Workload string          `json:"workload"` // "Deployment/web"
	Slug     string          `json:"slug"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind"` // web | worker
	Config   store.AppConfig `json:"config"`
	Env      []string        `json:"env"`      // variable names (values are not sent back)
	Files    []string        `json:"files"`    // config files mounted from ConfigMaps / Secrets
	Services []string        `json:"services"` // Services that will select the app's pods
	Domains  []importDomain  `json:"domains"`  // Ingress hosts that become domains of the app
	Hosts    []string        `json:"hosts"`    // Ingress hosts that stay on their Ingress (shared with other workloads)
	Warnings []string        `json:"warnings"` // what is dropped or changes
	Problems []string        `json:"problems"` // why it cannot be imported (empty = it can)

	objKind  string
	replicas int64
	env      []store.EnvVar
	files    []store.AppFile
	services []kube.Object
	ports    map[string]int64 // named container ports
}

// importDomain is a host (and path) of an Ingress that moves to the app.
type importDomain struct {
	Host       string `json:"host"`
	PathPrefix string `json:"path_prefix"`
	TargetPort int    `json:"target_port,omitempty"` // 0 = the app's port
	TLS        bool   `json:"tls"`                   // the Ingress had TLS: HTTPS is forced
}

var importKinds = map[string]string{"deployment": "Deployment", "statefulset": "StatefulSet"}

// importPreview: GET /api/projects/{pid}/external-workloads/{kind}/{name}/import
func (s *Server) importPreview(w http.ResponseWriter, r *http.Request, _ *principal, proj store.Project) {
	plan, code, err := s.planImport(r.Context(), proj, r.PathValue("kind"), r.PathValue("name"), r.URL.Query().Get("slug"), "")
	if err != nil {
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// importWorkload: POST /api/projects/{pid}/external-workloads/{kind}/{name}/import {slug?, name?}
func (s *Server) importWorkload(w http.ResponseWriter, r *http.Request, p *principal, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	if !p.eff.Can("app.deploy", &proj.ID) {
		writeError(w, http.StatusForbidden, "missing permission: app.deploy (importing deploys the app)")
		return
	}
	var in struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	plan, code, err := s.planImport(ctx, proj, r.PathValue("kind"), r.PathValue("name"), in.Slug, in.Name)
	if err != nil {
		writeError(w, code, err.Error())
		return
	}
	if len(plan.Problems) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "this workload cannot be imported: " + strings.Join(plan.Problems, "; "), "plan": plan})
		return
	}
	if proj.Quota != nil && proj.Quota.Apps > 0 && int64(proj.Apps) >= proj.Quota.Apps {
		writeError(w, http.StatusConflict, fmt.Sprintf("project quota allows %d apps", proj.Quota.Apps))
		return
	}
	app, err := s.store.CreateApp(ctx, store.App{ProjectID: proj.ID, Slug: plan.Slug, Name: plan.Name, Kind: plan.Kind,
		RuntimeID: "image", Config: plan.Config})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "an app with this slug already exists in the project")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	undo := func() { _ = s.store.DeleteApp(context.WithoutCancel(ctx), app.ID) }
	if err := s.store.ReplaceEnv(ctx, app.ID, plan.env); err != nil {
		undo()
		s.serverError(w, r, err)
		return
	}
	if err := s.store.ReplaceFiles(ctx, app.ID, plan.files); err != nil {
		undo()
		s.serverError(w, r, err)
		return
	}
	// the original stops first: its volumes (ReadWriteOnce) move to the app's pods
	_, wname := splitWorkload(plan.Workload)
	if _, err := s.cluster.Patch(ctx, plan.objKind, proj.Namespace, wname, map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{annImportedAs: plan.Slug, annImportedReplicas: strconv.FormatInt(plan.replicas, 10)}},
		"spec":     map[string]any{"replicas": 0},
	}); err != nil {
		undo()
		writeError(w, http.StatusBadGateway, "could not scale down "+plan.Workload+": "+err.Error())
		return
	}
	rel, err := s.deployer.Deploy(ctx, deploy.DeployRequest{App: app, By: &p.user.ID, Force: true})
	if err != nil {
		s.restoreWorkload(context.WithoutCancel(ctx), proj.Namespace, plan.objKind, wname, plan.replicas)
		undo()
		s.deployErr(w, err)
		return
	}
	var warnings []string
	for _, svc := range plan.services {
		if err := s.repointService(ctx, proj.Namespace, svc, plan); err != nil {
			warnings = append(warnings, "Service "+kube.Str(svc, "metadata", "name")+" still selects the old pods: "+err.Error())
		}
	}
	// the Ingress hosts become the app's domains: the panel's domains win over Ingress routes, so the
	// Ingress sync drops its routes first and the domain routes take over (the Ingress stays, unused)
	var hosts []string
	for _, d := range plan.Domains {
		dom, err := s.store.CreateDomain(ctx, store.Domain{AppID: app.ID, Host: d.Host, PathPrefix: d.PathPrefix, TargetPort: d.TargetPort,
			TLS: true, ForceHTTPS: d.TLS, DomainOptions: store.DomainOptions{Kind: store.DomainApp}})
		if err != nil {
			warnings = append(warnings, d.Host+d.PathPrefix+" stays on its Ingress: "+err.Error())
			continue
		}
		hosts = append(hosts, dom.Host+dom.PathPrefix)
	}
	if len(hosts) == 0 {
		s.addDefaultDomain(ctx, app, proj)
	}
	if s.ingSync != nil {
		if err := s.ingSync.Sync(ctx); err != nil {
			s.log.Warn("ingress sync after import", "err", err)
		}
	}
	s.resync(r)
	s.audit(r, p, "app.import", proj.Slug+"/"+app.Slug, map[string]any{"workload": plan.Workload, "services": plan.Services, "domains": hosts})
	writeJSON(w, http.StatusCreated, map[string]any{"app": app, "release": rel, "warnings": append(plan.Warnings, warnings...)})
}

func splitWorkload(s string) (kind, name string) {
	kind, name, _ = strings.Cut(s, "/")
	return kind, name
}

// planImport reads the workload and translates it. Problems that block the import are in plan.Problems;
// err is for requests that make no sense (unknown kind, missing workload).
func (s *Server) planImport(ctx context.Context, proj store.Project, kindArg, name, slug, appName string) (*importPlan, int, error) {
	if s.cluster == nil {
		return nil, http.StatusServiceUnavailable, errors.New("Kubernetes is not configured")
	}
	if !proj.Adopted {
		return nil, http.StatusBadRequest, errors.New("only workloads of adopted namespaces can be imported")
	}
	kind, ok := importKinds[strings.ToLower(kindArg)]
	if !ok {
		return nil, http.StatusBadRequest, errors.New("only Deployments and StatefulSets can be imported")
	}
	ns := proj.Namespace
	var obj kube.Object
	if err := s.cluster.GetObject(ctx, kind, ns, name, &obj); err != nil {
		if kube.IsNotFound(err) {
			return nil, http.StatusNotFound, fmt.Errorf("%s %s not found", kind, name)
		}
		return nil, http.StatusBadGateway, err
	}
	if kube.Str(obj, "metadata", "labels", deploy.LabelManagedBy) == deploy.ManagedBy {
		return nil, http.StatusConflict, errors.New("this workload already belongs to a Nectar app")
	}
	if a := kube.Str(obj, "metadata", "annotations", annImportedAs); a != "" {
		return nil, http.StatusConflict, fmt.Errorf("this workload was already imported as the app %s", a)
	}
	plan := &importPlan{Workload: kind + "/" + name, objKind: kind, Env: []string{}, Files: []string{}, Services: []string{},
		Domains: []importDomain{}, Hosts: []string{}, Warnings: []string{}, Problems: []string{}, ports: map[string]int64{}}
	problem := func(f string, a ...any) { plan.Problems = append(plan.Problems, fmt.Sprintf(f, a...)) }
	warn := func(f string, a ...any) { plan.Warnings = append(plan.Warnings, fmt.Sprintf(f, a...)) }

	plan.replicas = 1
	if v := kube.Field(obj, "spec", "replicas"); v != nil {
		plan.replicas = kube.Int(obj, "spec", "replicas")
	}
	if plan.replicas > 20 {
		problem("%d replicas (Nectar apps run at most 20)", plan.replicas)
	}
	if kind == "StatefulSet" && plan.replicas > 1 {
		problem("a StatefulSet with %d replicas has a disk per replica; only single-replica StatefulSets can be imported", plan.replicas)
	}
	tmplLabels, _ := kube.Field(obj, "spec", "template", "metadata", "labels").(map[string]any)
	pod, _ := kube.Field(obj, "spec", "template", "spec").(map[string]any)
	containers := kube.Slice(pod, "containers")
	if len(containers) != 1 {
		problem("the pod has %d containers; only single-container workloads can be imported", len(containers))
	}
	if n := len(kube.Slice(pod, "initContainers")); n > 0 {
		problem("the pod has %d init container(s), which Nectar apps do not support", n)
	}
	if pod["hostNetwork"] == true || pod["hostPID"] == true || pod["hostIPC"] == true {
		problem("the pod uses the host's network or processes")
	}
	if len(containers) == 0 {
		return plan, 0, nil
	}
	c := containers[0]
	cfg := store.AppConfig{Image: kube.Str(c, "image"), Replicas: int(plan.replicas), Health: store.HealthConfig{Type: "none"}}

	// command, arguments
	cfg.Command = strList(c["command"])
	cfg.ContainerArgs = strList(c["args"])
	if kube.Str(c, "workingDir") != "" {
		warn("the working directory %s is dropped (the image's own is used)", kube.Str(c, "workingDir"))
	}
	if sc, _ := c["securityContext"].(map[string]any); sc != nil {
		if sc["privileged"] == true {
			problem("the container runs privileged")
		}
		if add := kube.Field(sc, "capabilities", "add"); add != nil {
			warn("the extra capabilities %v are dropped", add)
		}
		if _, ok := sc["runAsUser"]; ok {
			warn("runAsUser is dropped: the image's default user runs the app")
		}
	}
	if psc, _ := pod["securityContext"].(map[string]any); psc != nil {
		if _, ok := psc["runAsUser"]; ok {
			warn("the pod's runAsUser is dropped: the image's default user runs the app")
		}
		if _, ok := psc["fsGroup"]; ok {
			warn("the pod's fsGroup is dropped: files on the volumes keep their owners")
		}
	}
	if len(kube.Slice(pod, "imagePullSecrets")) > 0 {
		warn("imagePullSecrets are dropped: add the registry login under Settings → Registries if the image is private")
	}

	// resources: limits, else requests without a limit, else no limit at all
	res := store.Resources{CPUMillicores: 250, MemoryMB: 256, NoLimit: true}
	if lim, _ := kube.Field(c, "resources", "limits").(map[string]any); len(lim) > 0 {
		res.NoLimit = false
		res.CPUMillicores, res.MemoryMB = quantityOr(lim["cpu"], 1000, 1000), quantityOr(lim["memory"], 1.0/(1<<20), 512)
	} else if req, _ := kube.Field(c, "resources", "requests").(map[string]any); len(req) > 0 {
		res.CPUMillicores, res.MemoryMB = quantityOr(req["cpu"], 1000, 250), quantityOr(req["memory"], 1.0/(1<<20), 256)
	}
	cfg.Resources = res

	// ports: the container's, then what Services target
	var ports []int
	addPort := func(p int64) {
		if p > 0 && p <= 65535 && !slices.Contains(ports, int(p)) {
			ports = append(ports, int(p))
		}
	}
	for _, p := range kube.Slice(c, "ports") {
		if pn := kube.Str(p, "name"); pn != "" {
			plan.ports[pn] = kube.Int(p, "containerPort")
		}
		if proto := kube.Str(p, "protocol"); proto == "" || proto == "TCP" {
			addPort(kube.Int(p, "containerPort"))
		}
	}
	svcs, err := s.cluster.List(ctx, "Service", ns, "")
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	for _, svc := range svcs {
		sel, _ := kube.Field(svc, "spec", "selector").(map[string]any)
		if len(sel) == 0 || kube.Str(svc, "metadata", "labels", deploy.LabelManagedBy) == deploy.ManagedBy || !selects(sel, tmplLabels) {
			continue
		}
		plan.services = append(plan.services, svc)
		plan.Services = append(plan.Services, kube.Str(svc, "metadata", "name"))
		for _, sp := range kube.Slice(svc, "spec", "ports") {
			if tp, ok := targetPort(sp, plan.ports); ok {
				if proto := kube.Str(sp, "protocol"); proto == "" || proto == "TCP" {
					addPort(tp)
				}
			} else {
				problem("Service %s targets the port %q, which the container does not name", kube.Str(svc, "metadata", "name"), kube.Str(sp, "targetPort"))
			}
		}
	}
	if len(ports) > 0 {
		cfg.Port = ports[0]
		for _, p := range ports[1:] {
			if len(cfg.InternalPorts) < 10 {
				cfg.InternalPorts = append(cfg.InternalPorts, p)
			}
		}
	}
	// hosts of Ingresses that route to these Services: they move to the app as domains when every path of
	// the host goes to this workload, otherwise they stay on the Ingress
	plan.Domains = []importDomain{}
	if ings, err := s.cluster.List(ctx, "Ingress", ns, ""); err == nil {
		plan.Domains, plan.Hosts = importDomains(ings, plan, cfg)
	}

	// health: the readiness probe (else the liveness probe) when it checks the main port
	probe, _ := c["readinessProbe"].(map[string]any)
	if probe == nil {
		probe, _ = c["livenessProbe"].(map[string]any)
	}
	httpProbe := false
	if probe != nil {
		delay := int(kube.Int(probe, "initialDelaySeconds"))
		switch {
		case kube.Field(probe, "httpGet") != nil:
			pp, _ := kube.Field(probe, "httpGet").(map[string]any)
			if port, ok := targetPortValue(pp["port"], plan.ports); ok && int(port) == cfg.Port {
				cfg.Health = store.HealthConfig{Type: "http", Path: orSlash(kube.Str(pp, "path")), InitialDelaySeconds: delay}
				httpProbe = true
			}
		case kube.Field(probe, "tcpSocket") != nil:
			pp, _ := kube.Field(probe, "tcpSocket").(map[string]any)
			if port, ok := targetPortValue(pp["port"], plan.ports); ok && int(port) == cfg.Port {
				cfg.Health = store.HealthConfig{Type: "tcp", InitialDelaySeconds: delay}
			}
		}
		if cfg.Health.Type == "none" {
			warn("the health probe is dropped (only HTTP or TCP checks of the main port are kept)")
		}
	}
	plan.Kind = runtimes.KindWorker
	if cfg.Port > 0 && (len(plan.Domains) > 0 || len(plan.Hosts) > 0 || httpProbe) {
		plan.Kind = runtimes.KindWeb
	}
	if plan.Kind == runtimes.KindWeb && cfg.Health.Type == "none" {
		cfg.Health.Type = "tcp"
	}

	// env
	secrets, configMaps := map[string]kube.Object{}, map[string]kube.Object{}
	lookup := func(kind, name string) (map[string]string, error) {
		cache := configMaps
		if kind == "Secret" {
			cache = secrets
		}
		o, ok := cache[name]
		if !ok {
			if err := s.cluster.GetObject(ctx, kind, ns, name, &o); err != nil {
				return nil, err
			}
			cache[name] = o
		}
		out := map[string]string{}
		data, _ := o["data"].(map[string]any)
		for k, v := range data {
			sv, _ := v.(string)
			if kind == "Secret" {
				b, err := base64.StdEncoding.DecodeString(sv)
				if err != nil {
					continue
				}
				sv = string(b)
			}
			out[k] = sv
		}
		if bd, _ := o["binaryData"].(map[string]any); len(bd) > 0 {
			for k := range bd {
				out[k] = "\x00binary"
			}
		}
		return out, nil
	}
	envSeen := map[string]bool{}
	addEnv := func(key, value string, secret bool) {
		if !deploy.ValidEnvKey(key) {
			warn("the variable %q is dropped (not a valid name)", key)
			return
		}
		if len(value) > 64<<10 {
			problem("the variable %s is longer than 64 KiB", key)
			return
		}
		enc, err := s.envBox.Seal(value)
		if err != nil {
			problem("could not encrypt %s: %v", key, err)
			return
		}
		ev := store.EnvVar{Key: key, ValueEnc: enc, IsSecret: secret}
		if envSeen[key] { // a later definition wins, like in Kubernetes
			for i := range plan.env {
				if plan.env[i].Key == key {
					plan.env[i] = ev
				}
			}
			return
		}
		envSeen[key] = true
		plan.env = append(plan.env, ev)
		plan.Env = append(plan.Env, key)
	}
	for _, ef := range kube.Slice(c, "envFrom") {
		prefix := kube.Str(ef, "prefix")
		for _, src := range []struct{ field, kind string }{{"configMapRef", "ConfigMap"}, {"secretRef", "Secret"}} {
			ref, _ := ef[src.field].(map[string]any)
			if ref == nil {
				continue
			}
			vals, err := lookup(src.kind, kube.Str(ref, "name"))
			if err != nil {
				if ref["optional"] != true {
					problem("%s %s (envFrom) cannot be read: %v", src.kind, kube.Str(ref, "name"), err)
				}
				continue
			}
			for _, k := range sortedKeys(vals) {
				addEnv(prefix+k, vals[k], src.kind == "Secret")
			}
		}
	}
	for _, e := range kube.Slice(c, "env") {
		key := kube.Str(e, "name")
		vf, _ := e["valueFrom"].(map[string]any)
		if vf == nil {
			addEnv(key, kube.Str(e, "value"), false)
			continue
		}
		done := false
		for _, src := range []struct{ field, kind string }{{"configMapKeyRef", "ConfigMap"}, {"secretKeyRef", "Secret"}} {
			ref, _ := vf[src.field].(map[string]any)
			if ref == nil {
				continue
			}
			done = true
			vals, err := lookup(src.kind, kube.Str(ref, "name"))
			v, has := vals[kube.Str(ref, "key")]
			switch {
			case err == nil && has:
				addEnv(key, v, src.kind == "Secret")
			case ref["optional"] == true:
			default:
				problem("%s comes from %s %s/%s, which cannot be read", key, src.kind, kube.Str(ref, "name"), kube.Str(ref, "key"))
			}
		}
		if !done {
			warn("%s is dropped (its value comes from the pod's fields, which Nectar apps do not pass)", key)
		}
	}
	sort.Strings(plan.Env)

	// volumes and mounted files
	vols := map[string]kube.Object{}
	for _, v := range kube.Slice(pod, "volumes") {
		vols[kube.Str(v, "name")] = v
	}
	templates := map[string]kube.Object{}
	if kind == "StatefulSet" {
		for _, t := range kube.Slice(obj, "spec", "volumeClaimTemplates") {
			templates[kube.Str(t, "metadata", "name")] = t
		}
	}
	usedNames, usedPaths := map[string]bool{}, map[string]bool{}
	totalFiles := 0
	addFile := func(p, content string, secret bool) {
		if msg := validMountPath(p); msg != "" {
			problem("the file %s cannot be mounted: %s", p, msg)
			return
		}
		if content == "\x00binary" {
			problem("the file %s holds binary data, which config files cannot", p)
			return
		}
		if usedPaths[p] {
			return
		}
		usedPaths[p] = true
		totalFiles += len(content)
		if len(content) > 256<<10 {
			problem("the file %s is larger than 256 KiB", p)
			return
		}
		enc, err := s.envBox.Seal(content)
		if err != nil {
			problem("could not encrypt %s: %v", p, err)
			return
		}
		plan.files = append(plan.files, store.AppFile{Path: p, ContentEnc: enc, IsSecret: secret})
		plan.Files = append(plan.Files, p)
	}
	for _, m := range kube.Slice(c, "volumeMounts") {
		vname, mountPath, sub := kube.Str(m, "name"), kube.Str(m, "mountPath"), kube.Str(m, "subPath")
		v, isVol := vols[vname]
		tmpl, isTmpl := templates[vname]
		switch {
		case isTmpl:
			if sub != "" {
				problem("the volume %s is mounted with a subPath", vname)
				continue
			}
			addVolume(plan, &cfg, usedNames, vname, mountPath, vname+"-"+name+"-0", tmpl, problem)
		case !isVol:
			problem("the mount %s refers to a missing volume %s", mountPath, vname)
		case v["persistentVolumeClaim"] != nil:
			if sub != "" {
				problem("the volume %s is mounted with a subPath", vname)
				continue
			}
			claim := kube.Str(v, "persistentVolumeClaim", "claimName")
			var pvc kube.Object
			if err := s.cluster.GetObject(ctx, "PersistentVolumeClaim", ns, claim, &pvc); err != nil {
				problem("the PersistentVolumeClaim %s cannot be read: %v", claim, err)
				continue
			}
			addVolume(plan, &cfg, usedNames, vname, mountPath, claim, pvc, problem)
		case v["configMap"] != nil || v["secret"] != nil:
			srcKind, ref, refName := "ConfigMap", v["configMap"], "name"
			if v["secret"] != nil {
				srcKind, ref, refName = "Secret", v["secret"], "secretName"
			}
			rm, _ := ref.(map[string]any)
			vals, err := lookup(srcKind, kube.Str(rm, refName))
			if err != nil {
				if rm["optional"] != true {
					problem("%s %s (mounted at %s) cannot be read: %v", srcKind, kube.Str(rm, refName), mountPath, err)
				}
				continue
			}
			// items map keys to file names; without items every key is a file
			files := map[string]string{}
			if items := kube.Slice(rm, "items"); len(items) > 0 {
				for _, it := range items {
					files[kube.Str(it, "path")] = kube.Str(it, "key")
				}
			} else {
				for k := range vals {
					files[k] = k
				}
			}
			if sub != "" { // one file at mountPath
				if key, ok := files[sub]; ok {
					addFile(mountPath, vals[key], srcKind == "Secret")
				} else {
					problem("%s %s has no %s (mounted at %s)", srcKind, kube.Str(rm, refName), sub, mountPath)
				}
				continue
			}
			for _, fname := range sortedKeys(files) {
				addFile(path.Join(mountPath, fname), vals[files[fname]], srcKind == "Secret")
			}
		case v["emptyDir"] != nil:
			warn("the temporary volume %s (%s) is dropped: the container's own filesystem is used", vname, mountPath)
		default:
			problem("the volume %s (%s) is of a kind Nectar apps cannot mount (only PersistentVolumeClaims, ConfigMaps, Secrets and emptyDir)", vname, mountPath)
		}
	}
	if len(plan.files) > 32 {
		problem("%d mounted files (Nectar apps take at most 32)", len(plan.files))
	}
	if totalFiles > 900<<10 {
		problem("the mounted files are larger than 900 KiB together")
	}
	sort.Strings(plan.Files)
	if len(plan.services) == 0 && cfg.Port > 0 {
		warn("no Service selects this workload: the app is reachable at <app>:%d inside the project", cfg.Port)
	}
	if len(plan.Hosts) > 0 {
		warn("the hosts %s also route to other workloads, so they keep their Ingress (it reaches the app through the Service(s) %s)",
			strings.Join(plan.Hosts, ", "), strings.Join(plan.Services, ", "))
	}

	// slug and name
	if appName = strings.TrimSpace(appName); appName == "" {
		appName = name
	}
	plan.Name = appName
	if slug = strings.ToLower(strings.TrimSpace(slug)); slug == "" {
		slug, err = s.freeImportSlug(ctx, proj, name)
		if err != nil {
			return nil, http.StatusBadGateway, err
		}
	} else if msg := s.importSlugProblem(ctx, proj, slug); msg != "" {
		problem("%s", msg)
	}
	plan.Slug = slug
	plan.Config = cfg
	if rt, ok := s.catalog.Get("image"); ok {
		if msg := validateConfig(rt, plan.Kind, cfg); msg != "" {
			problem("%s", msg)
		}
	}
	return plan, 0, nil
}

// importDomains splits the hosts of the Ingresses that route to the workload's Services into the ones that
// move to the app (every path of the host goes to the workload) and the ones that stay (some path goes
// elsewhere, or to a port the app does not serve).
func importDomains(ings []kube.Object, plan *importPlan, cfg store.AppConfig) (move []importDomain, stay []string) {
	type pathTarget struct {
		path string
		port int // container port, 0 = not this workload
	}
	byHost := map[string][]pathTarget{}
	tlsHosts := map[string]bool{}
	svcByName := map[string]kube.Object{}
	for _, svc := range plan.services {
		svcByName[kube.Str(svc, "metadata", "name")] = svc
	}
	// the container port a backend reaches (0 = another workload or an unknown port)
	target := func(b kube.Object) int {
		svc, ok := svcByName[kube.Str(b, "service", "name")]
		if !ok {
			return 0
		}
		num, pname := kube.Int(b, "service", "port", "number"), kube.Str(b, "service", "port", "name")
		for _, sp := range kube.Slice(svc, "spec", "ports") {
			if (num != 0 && kube.Int(sp, "port") == num) || (pname != "" && kube.Str(sp, "name") == pname) {
				if p, ok := targetPort(sp, plan.ports); ok {
					return int(p)
				}
			}
		}
		return 0
	}
	for _, ing := range ings {
		for _, tl := range kube.Slice(ing, "spec", "tls") {
			for _, h := range strList(tl["hosts"]) {
				tlsHosts[strings.ToLower(h)] = true
			}
		}
		def, _ := kube.Field(ing, "spec", "defaultBackend").(map[string]any)
		for _, rule := range kube.Slice(ing, "spec", "rules") {
			host := strings.ToLower(kube.Str(rule, "host"))
			if host == "" {
				continue
			}
			paths := kube.Slice(rule, "http", "paths")
			if len(paths) == 0 && def != nil {
				byHost[host] = append(byHost[host], pathTarget{"/", target(def)})
			}
			for _, p := range paths {
				b, _ := p["backend"].(map[string]any)
				pp, port := store.NormalizePath(kube.Str(p, "path")), target(b)
				if strings.ContainsAny(pp, " ?#*()$^[]|\\") && port != 0 {
					port = -1 // a regex path (nginx rewrite): it stays on the Ingress
				}
				byHost[host] = append(byHost[host], pathTarget{pp, port})
			}
		}
	}
	for _, host := range sortedKeys(byHost) {
		targets := byHost[host]
		mine, all := 0, true
		for _, pt := range targets {
			if pt.port == -1 {
				all = false
				mine++
			} else if pt.port == 0 || (pt.port != cfg.Port && !slices.Contains(cfg.InternalPorts, pt.port)) {
				all = false
			} else {
				mine++
			}
		}
		if mine == 0 {
			continue // the host is not this workload's at all
		}
		if !all || !hostRe.MatchString(host) {
			stay = append(stay, host)
			continue
		}
		seen := map[string]bool{}
		for _, pt := range targets {
			if seen[pt.path] {
				continue
			}
			seen[pt.path] = true
			d := importDomain{Host: host, PathPrefix: pt.path, TLS: tlsHosts[host]}
			if pt.port != cfg.Port {
				d.TargetPort = pt.port
			}
			move = append(move, d)
		}
	}
	if move == nil {
		move = []importDomain{}
	}
	if stay == nil {
		stay = []string{}
	}
	return move, stay
}

func addVolume(plan *importPlan, cfg *store.AppConfig, used map[string]bool, vname, mountPath, claim string, pvc kube.Object, problem func(string, ...any)) {
	name := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(vname), "-"), "-")
	if len(name) > 30 {
		name = strings.Trim(name[:30], "-")
	}
	if name == "" {
		name = "data"
	}
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", strings.TrimRight(name, "-0123456789"), i)
	}
	used[name] = true
	size := int64(1024)
	if q := kube.Str(pvc, "spec", "resources", "requests", "storage"); q != "" {
		if b, err := kube.ParseQuantity(q); err == nil {
			size = max(64, int64(math.Ceil(b/(1<<20))))
		}
	}
	modes, _ := kube.Field(pvc, "spec", "accessModes").([]any)
	if msg := validMountPath(mountPath); msg != "" {
		problem("the volume %s cannot be mounted at %s: %s", vname, mountPath, msg)
	}
	cfg.Volumes = append(cfg.Volumes, store.Volume{Name: name, MountPath: mountPath, SizeMB: size, Claim: claim,
		StorageClass: kube.Str(pvc, "spec", "storageClassName"), Shared: slices.Contains(modes, any("ReadWriteMany"))})
}

// freeImportSlug picks an app slug from the workload name that collides neither with an app nor with an
// object of the namespace (the original keeps its name, scaled to 0).
func (s *Server) freeImportSlug(ctx context.Context, proj store.Project, name string) (string, error) {
	base := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(base) > 26 {
		base = strings.Trim(base[:26], "-")
	}
	if base == "" {
		base = "app"
	}
	for _, c := range []string{base, base + "-app", base + "-2", base + "-3", base + "-4"} {
		if s.importSlugProblem(ctx, proj, c) == "" {
			return c, nil
		}
	}
	return "", errors.New("no free app name found; pass a slug")
}

// importSlugProblem says why an imported app cannot use slug ("" = it can).
func (s *Server) importSlugProblem(ctx context.Context, proj store.Project, slug string) string {
	if !slugRe.MatchString(slug) {
		return "slug must be 1–30 lowercase letters, digits or dashes"
	}
	if taken, err := s.store.SlugTaken(ctx, proj.ID, slug); err != nil || taken {
		return "an app or add-on " + slug + " already exists in the project"
	}
	for _, o := range []struct{ kind, name string }{
		{"Deployment", slug}, {"StatefulSet", slug}, {"Service", slug}, {"Service", slug + "-ports"},
		{"Secret", slug + "-env"}, {"Secret", slug + "-userfiles"}, {"ConfigMap", slug + "-files"},
	} {
		var obj kube.Object
		if err := s.cluster.GetObject(ctx, o.kind, proj.Namespace, o.name, &obj); err == nil {
			return fmt.Sprintf("the namespace already has a %s named %s; choose another slug", o.kind, o.name)
		}
	}
	return ""
}

// selects reports whether every key of the selector matches the pod labels.
func selects(sel, labels map[string]any) bool {
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func targetPort(sp kube.Object, named map[string]int64) (int64, bool) {
	tp := sp["targetPort"]
	if tp == nil {
		return kube.Int(sp, "port"), true
	}
	return targetPortValue(tp, named)
}

func targetPortValue(v any, named map[string]int64) (int64, bool) {
	switch p := v.(type) {
	case float64:
		return int64(p), true
	case int64:
		return p, true
	case int:
		return int64(p), true
	case string:
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			return n, true
		}
		n, ok := named[p]
		return n, ok
	}
	return 0, false
}

func ingressUses(ing kube.Object, services []string) bool {
	uses := func(b kube.Object) bool {
		return slices.Contains(services, kube.Str(b, "service", "name"))
	}
	if b, _ := kube.Field(ing, "spec", "defaultBackend").(map[string]any); b != nil && uses(b) {
		return true
	}
	for _, rule := range kube.Slice(ing, "spec", "rules") {
		for _, p := range kube.Slice(rule, "http", "paths") {
			if b, _ := p["backend"].(map[string]any); b != nil && uses(b) {
				return true
			}
		}
	}
	return false
}

func strList(v any) []string {
	arr, _ := v.([]any)
	var out []string
	for _, it := range arr {
		out = append(out, fmt.Sprint(it))
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// quantityOr converts a quantity with factor (cores→millicores: 1000, bytes→MiB: 1/2^20), def when missing.
func quantityOr(v any, factor float64, def int64) int64 {
	s, _ := v.(string)
	if s == "" {
		if f, ok := v.(float64); ok {
			s = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}
	q, err := kube.ParseQuantity(s)
	if err != nil || q <= 0 {
		return def
	}
	return max(1, int64(math.Ceil(q*factor)))
}

func orSlash(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

// repointService makes a Service select the app's pods and target numeric ports (the app's container
// does not carry the original port names). The original selector and ports are kept in annotations.
func (s *Server) repointService(ctx context.Context, ns string, svc kube.Object, plan *importPlan) error {
	name := kube.Str(svc, "metadata", "name")
	origSel, _ := kube.Field(svc, "spec", "selector").(map[string]any)
	selJSON, _ := json.Marshal(origSel)
	sel := map[string]any{}
	for k := range origSel {
		sel[k] = nil
	}
	sel[deploy.LabelApp], sel[deploy.LabelRole] = plan.Slug, "workload"
	ann := map[string]any{annImportedBy: plan.Slug, annOrigSelector: string(selJSON)}
	spec := map[string]any{"selector": sel}
	ports := kube.Slice(svc, "spec", "ports")
	changed := false
	newPorts := make([]any, 0, len(ports))
	for _, sp := range ports {
		cp := map[string]any{}
		for k, v := range sp {
			cp[k] = v
		}
		if tp, ok := sp["targetPort"].(string); ok {
			if _, err := strconv.Atoi(tp); err != nil {
				if n, ok := plan.ports[tp]; ok {
					cp["targetPort"] = n
					changed = true
				}
			}
		}
		newPorts = append(newPorts, cp)
	}
	if changed {
		pj, _ := json.Marshal(ports)
		ann[annOrigPorts] = string(pj)
		spec["ports"] = newPorts
	}
	_, err := s.cluster.Patch(ctx, "Service", ns, name, map[string]any{"metadata": map[string]any{"annotations": ann}, "spec": spec})
	return err
}

func (s *Server) restoreWorkload(ctx context.Context, ns, kind, name string, replicas int64) {
	_, _ = s.cluster.Patch(ctx, kind, ns, name, map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{annImportedAs: nil, annImportedReplicas: nil}},
		"spec":     map[string]any{"replicas": replicas},
	})
}

// undoImport runs when an imported app is deleted: the original workload scales back up and its Services
// select its pods again.
func (s *Server) undoImport(ctx context.Context, proj store.Project, slug string) {
	if s.cluster == nil || !proj.Adopted {
		return
	}
	for _, kind := range []string{"Deployment", "StatefulSet"} {
		items, err := s.cluster.List(ctx, kind, proj.Namespace, "")
		if err != nil {
			continue
		}
		for _, o := range items {
			if kube.Str(o, "metadata", "annotations", annImportedAs) != slug {
				continue
			}
			n, err := strconv.ParseInt(kube.Str(o, "metadata", "annotations", annImportedReplicas), 10, 64)
			if err != nil {
				n = 1
			}
			s.restoreWorkload(ctx, proj.Namespace, kind, kube.Str(o, "metadata", "name"), n)
		}
	}
	svcs, err := s.cluster.List(ctx, "Service", proj.Namespace, "")
	if err != nil {
		return
	}
	for _, svc := range svcs {
		if kube.Str(svc, "metadata", "annotations", annImportedBy) != slug {
			continue
		}
		var orig map[string]any
		if json.Unmarshal([]byte(kube.Str(svc, "metadata", "annotations", annOrigSelector)), &orig) != nil {
			continue
		}
		sel := map[string]any{deploy.LabelApp: nil, deploy.LabelRole: nil}
		for k, v := range orig {
			sel[k] = v
		}
		spec := map[string]any{"selector": sel}
		var ports []any
		if pj := kube.Str(svc, "metadata", "annotations", annOrigPorts); pj != "" && json.Unmarshal([]byte(pj), &ports) == nil {
			spec["ports"] = ports
		}
		_, _ = s.cluster.Patch(ctx, "Service", proj.Namespace, kube.Str(svc, "metadata", "name"), map[string]any{
			"metadata": map[string]any{"annotations": map[string]any{annImportedBy: nil, annOrigSelector: nil, annOrigPorts: nil}},
			"spec":     spec,
		})
	}
}
