// Package deploy turns projects and apps into Kubernetes objects, applies them and drives releases.
package deploy

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

type obj = kube.Object

// Labels.
const (
	LabelManagedBy = "app.kubernetes.io/managed-by"
	ManagedBy      = "nectar-apps"
	LabelProject   = "nectar.dev/project"
	LabelApp       = "nectar.dev/app"
	LabelAppID     = "nectar.dev/app-id"
	LabelRelease   = "nectar.dev/release"
	LabelRole      = "nectar.dev/role" // workload | release-command | task-run
	LabelAdopted   = "nectar.dev/adopted"
	// AppServiceAccount is the (token-less) service account all app pods run as.
	AppServiceAccount = "app"
)

// InitImage fetches artifacts into the pod (busybox: wget, sha256sum, unzip, tar).
var InitImage = "busybox:1.37"

// ProjectSpec is the input for project-level objects.
type ProjectSpec struct {
	Project store.Project
	// NodeIPs are allowed to reach app pods (nectar-gateway runs with hostNetwork, so its traffic
	// arrives from the node addresses; NodePort traffic is SNATed to them too).
	NodeIPs []string
	// SystemNamespace hosts the panel and gateway.
	SystemNamespace string
	// PullSecret is a Docker config.json with registry logins ("" = none): Secret nectar-pull, used by the
	// app ServiceAccount for every pod of the project.
	PullSecret []byte
}

// PullSecretName is the Secret with the private registry logins of a project.
const PullSecretName = "nectar-pull"

// serviceAccount is the ServiceAccount app pods run as (with the pull secret when there is one).
func serviceAccount(s ProjectSpec, ns string, labels map[string]any) obj {
	sa := obj{
		"apiVersion": "v1", "kind": "ServiceAccount",
		"metadata":                     meta(AppServiceAccount, ns, labels),
		"automountServiceAccountToken": false,
	}
	if len(s.PullSecret) > 0 {
		sa["imagePullSecrets"] = []any{map[string]any{"name": PullSecretName}}
	}
	return sa
}

// pullSecretObjects returns the pull Secret (none without logins; callers delete it then).
func pullSecretObjects(s ProjectSpec, ns string, labels map[string]any) []obj {
	if len(s.PullSecret) == 0 {
		return nil
	}
	return []obj{{
		"apiVersion": "v1", "kind": "Secret", "type": "kubernetes.io/dockerconfigjson",
		"metadata": meta(PullSecretName, ns, labels),
		"data":     map[string]any{".dockerconfigjson": base64.StdEncoding.EncodeToString(s.PullSecret)},
	}}
}

// ProjectObjects returns the namespace and its guard rails. The ResourceQuota is only returned when
// the project has a quota (callers delete it otherwise).
func ProjectObjects(s ProjectSpec) []obj {
	ns := s.Project.Namespace
	labels := map[string]any{LabelManagedBy: ManagedBy, LabelProject: s.Project.Slug}
	from := []any{
		map[string]any{"podSelector": map[string]any{}},
		map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": s.SystemNamespace}}},
	}
	ips := slices.Clone(s.NodeIPs)
	sort.Strings(ips)
	for _, ip := range ips {
		cidr := ip + "/32"
		if strings.Contains(ip, ":") {
			cidr = ip + "/128"
		}
		from = append(from, map[string]any{"ipBlock": map[string]any{"cidr": cidr}})
	}
	if s.Project.Adopted {
		// an existing namespace: only mark it and add what Nectar apps need; never touch the user's pods
		// (no isolation NetworkPolicy, no Pod Security labels, no default requests)
		out := []obj{
			{
				"apiVersion": "v1", "kind": "Namespace",
				"metadata": map[string]any{"name": ns, "labels": map[string]any{LabelProject: s.Project.Slug, LabelAdopted: "true"}},
			},
		}
		out = append(out, pullSecretObjects(s, ns, labels)...)
		out = append(out, serviceAccount(s, ns, labels))
		return append(out, quotaObjects(s, ns, labels)...)
	}
	out := []obj{
		{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": map[string]any{"name": ns, "labels": merge(labels, map[string]any{
				// baseline blocks privileged pods, host namespaces and hostPath; restricted is reported as warnings
				"pod-security.kubernetes.io/enforce": "baseline",
				"pod-security.kubernetes.io/warn":    "restricted",
			})},
		},
		{
			"apiVersion": "v1", "kind": "LimitRange",
			"metadata": meta("defaults", ns, labels),
			"spec": map[string]any{"limits": []any{map[string]any{
				"type":           "Container",
				"defaultRequest": map[string]any{"cpu": "50m", "memory": "64Mi"},
			}}},
		},
		{
			"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
			"metadata": meta("nectar-isolation", ns, labels),
			"spec": map[string]any{
				"podSelector": map[string]any{},
				"policyTypes": []any{"Ingress"},
				"ingress":     []any{map[string]any{"from": from}},
			},
		},
	}
	// the pull Secret goes before the ServiceAccount that references it
	out = append(out[:1], append(append(pullSecretObjects(s, ns, labels), serviceAccount(s, ns, labels)), out[1:]...)...)
	out = append(out, quotaObjects(s, ns, labels)...)
	return out
}

// quotaObjects returns the ResourceQuota of a project (none when unlimited).
func quotaObjects(s ProjectSpec, ns string, labels map[string]any) []obj {
	var out []obj
	if q := s.Project.Quota; q != nil && !q.IsZero() {
		hard := map[string]any{}
		if q.CPUMillicores > 0 {
			hard["requests.cpu"] = fmt.Sprintf("%dm", q.CPUMillicores)
		}
		if q.MemoryMB > 0 {
			hard["requests.memory"] = fmt.Sprintf("%dMi", q.MemoryMB)
		}
		if q.StorageGB > 0 {
			hard["requests.storage"] = fmt.Sprintf("%dGi", q.StorageGB)
		}
		if q.Pods > 0 {
			hard["pods"] = strconv.FormatInt(q.Pods, 10)
		}
		if len(hard) > 0 {
			out = append(out, obj{"apiVersion": "v1", "kind": "ResourceQuota", "metadata": meta("quota", ns, labels), "spec": map[string]any{"hard": hard}})
		}
	}
	return out
}

// AppSpec is the input for app-level objects of one release.
type AppSpec struct {
	App         store.App
	Project     store.Project
	Runtime     runtimes.Runtime
	Image       string
	Config      store.AppConfig // the release's config snapshot
	Release     int
	Env         map[string]string // decrypted user env vars
	Files       []UserFile        // decrypted user config files
	Artifact    *store.Artifact   // nil for image-only apps
	ArtifactURL string
	// Archive details from detection (format zip|tar.gz, folder to unwrap).
	ArchiveFormat string
	StripPrefix   string
}

// UserFile is a config file the user mounts into the containers.
type UserFile struct {
	Path    string
	Content string
}

func (s AppSpec) name() string { return s.App.Slug }

// VolumeClaimName is the PVC name of an app volume.
func VolumeClaimName(appSlug, volume string) string { return trimName(appSlug + "-" + volume) }

// LabelVolume marks the PVCs of app volumes with the volume name.
const LabelVolume = "nectar.dev/volume"

func (s AppSpec) labels() map[string]any {
	return map[string]any{LabelManagedBy: ManagedBy, LabelProject: s.Project.Slug, LabelApp: s.App.Slug, LabelAppID: strconv.FormatInt(s.App.ID, 10)}
}

// AppObjects returns the desired objects for an app release, excluding one-off Jobs.
// The release-command Job is built separately with ReleaseJob.
func AppObjects(s AppSpec) ([]obj, error) {
	ns, labels := s.Project.Namespace, s.labels()
	var out []obj

	env, err := s.envData()
	if err != nil {
		return nil, err
	}
	out = append(out, obj{
		"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
		"metadata": meta(s.name()+"-env", ns, labels),
		"data":     env,
	})
	files, err := s.configFiles()
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		data := map[string]any{}
		for k, f := range files {
			data[k] = f.content
		}
		out = append(out, obj{"apiVersion": "v1", "kind": "ConfigMap", "metadata": meta(s.name()+"-files", ns, labels), "data": data})
	}

	if len(s.Files) > 0 {
		data := map[string]any{}
		for i, f := range s.Files {
			data[userFileKey(i)] = base64.StdEncoding.EncodeToString([]byte(f.Content))
		}
		out = append(out, obj{
			"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
			"metadata": meta(s.name()+"-userfiles", ns, labels),
			"data":     data,
		})
	}
	for _, v := range s.Config.Volumes {
		spec := map[string]any{
			"accessModes": []any{"ReadWriteOnce"},
			"resources":   map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dMi", v.SizeMB)}},
		}
		if v.Shared {
			spec["accessModes"] = []any{"ReadWriteMany"}
		}
		if v.StorageClass != "" {
			spec["storageClassName"] = v.StorageClass
		}
		out = append(out, obj{
			"apiVersion": "v1", "kind": "PersistentVolumeClaim",
			"metadata": meta(VolumeClaimName(s.App.Slug, v.Name), ns, merge(labels, map[string]any{LabelVolume: v.Name})),
			"spec":     spec,
		})
	}

	pod, err := s.podSpec(nil, "workload")
	if err != nil {
		return nil, err
	}
	tmplLabels := merge(labels, map[string]any{LabelRelease: strconv.Itoa(s.Release), LabelRole: "workload"})

	switch s.App.Kind {
	case runtimes.KindWeb, runtimes.KindWorker:
		replicas := s.Config.Replicas
		if replicas < 0 {
			replicas = 0
		}
		out = append(out, obj{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": meta(s.name(), ns, labels),
			"spec": map[string]any{
				"replicas":                replicas,
				"revisionHistoryLimit":    5,
				"progressDeadlineSeconds": 600,
				"selector":                map[string]any{"matchLabels": map[string]any{LabelApp: s.App.Slug, LabelRole: "workload"}},
				"strategy": map[string]any{"type": "RollingUpdate",
					"rollingUpdate": map[string]any{"maxSurge": 1, "maxUnavailable": 0}},
				"template": map[string]any{"metadata": map[string]any{"labels": tmplLabels}, "spec": pod},
			},
		})
	case runtimes.KindCron:
		pod["restartPolicy"] = "Never"
		out = append(out, obj{
			"apiVersion": "batch/v1", "kind": "CronJob",
			"metadata": meta(s.name(), ns, labels),
			"spec": map[string]any{
				"schedule":                   s.Config.Schedule,
				"concurrencyPolicy":          "Forbid",
				"successfulJobsHistoryLimit": 3,
				"failedJobsHistoryLimit":     3,
				"suspend":                    s.Config.Replicas == 0,
				"jobTemplate": map[string]any{"spec": map[string]any{
					"backoffLimit": 2,
					"template":     map[string]any{"metadata": map[string]any{"labels": tmplLabels}, "spec": pod},
				}},
			},
		})
	case runtimes.KindTask:
		// nothing long-running: tasks are started on demand with TaskJob
	default:
		return nil, fmt.Errorf("unknown app kind %q", s.App.Kind)
	}

	if (s.App.Kind == runtimes.KindWeb || s.App.Kind == runtimes.KindWorker) && s.Config.Port > 0 {
		// web apps: the gateway's target; workers with a port (a database, a broker): reachable by the
		// project's other apps at <app>:<port>, never from outside
		name := "http"
		if s.App.Kind == runtimes.KindWorker {
			name = "tcp"
		}
		ports := []any{map[string]any{"name": name, "port": s.Config.Port, "targetPort": name, "protocol": "TCP"}}
		for _, p := range s.Config.InternalPorts {
			if p != s.Config.Port {
				ports = append(ports, map[string]any{"name": fmt.Sprintf("i%d", p), "port": p, "targetPort": p, "protocol": "TCP"})
			}
		}
		out = append(out, obj{
			"apiVersion": "v1", "kind": "Service",
			"metadata": meta(s.name(), ns, labels),
			"spec": map[string]any{
				"type":     "ClusterIP",
				"selector": map[string]any{LabelApp: s.App.Slug, LabelRole: "workload"},
				"ports":    ports,
			},
		})
	}
	if len(s.Config.Expose) > 0 && (s.App.Kind == runtimes.KindWeb || s.App.Kind == runtimes.KindWorker) {
		var ports []any
		for i, p := range s.Config.Expose {
			proto := strings.ToUpper(p.Protocol)
			if proto == "" {
				proto = "TCP"
			}
			port := map[string]any{"name": fmt.Sprintf("p%d-%s", i, strings.ToLower(proto)), "port": p.Port, "targetPort": p.Port, "protocol": proto}
			if p.NodePort > 0 {
				port["nodePort"] = p.NodePort
			}
			ports = append(ports, port)
		}
		out = append(out, obj{
			"apiVersion": "v1", "kind": "Service",
			"metadata": meta(s.name()+"-ports", ns, labels),
			"spec": map[string]any{
				"type":                  "NodePort",
				"externalTrafficPolicy": "Cluster",
				"selector":              map[string]any{LabelApp: s.App.Slug, LabelRole: "workload"},
				"ports":                 ports,
			},
		})
	}
	return out, nil
}

// ReleaseJob returns the Job that runs the release command of a release.
func ReleaseJob(s AppSpec) (obj, error) {
	return s.job(fmt.Sprintf("%s-release-%d", s.name(), s.Release), "release-command", []string{"sh", "-c", s.Config.ReleaseCommand}, 1800)
}

// TaskJob returns a one-off Job for a task app (or a manual run of any app). suffix makes the name unique.
func TaskJob(s AppSpec, suffix string, command []string) (obj, error) {
	return s.job(fmt.Sprintf("%s-run-%s", s.name(), suffix), "task-run", command, 6*3600)
}

func (s AppSpec) job(name, role string, command []string, deadline int) (obj, error) {
	pod, err := s.podSpec(command, role)
	if err != nil {
		return nil, err
	}
	pod["restartPolicy"] = "Never"
	labels := s.labels()
	tmplLabels := merge(labels, map[string]any{LabelRelease: strconv.Itoa(s.Release), LabelRole: role})
	return obj{
		"apiVersion": "batch/v1", "kind": "Job",
		"metadata": meta(trimName(name), s.Project.Namespace, merge(labels, map[string]any{LabelRole: role, LabelRelease: strconv.Itoa(s.Release)})),
		"spec": map[string]any{
			"backoffLimit":            0,
			"activeDeadlineSeconds":   deadline,
			"ttlSecondsAfterFinished": 86400,
			"template":                map[string]any{"metadata": map[string]any{"labels": tmplLabels}, "spec": pod},
		},
	}, nil
}

// podSpec builds the pod spec shared by workloads and jobs. command overrides the app command (jobs).
func (s AppSpec) podSpec(command []string, role string) (map[string]any, error) {
	rt, cfg := s.Runtime, s.Config
	sec := map[string]any{"seccompProfile": map[string]any{"type": "RuntimeDefault"}}
	if rt.RunAsUser != nil {
		uid := *rt.RunAsUser
		sec["runAsUser"], sec["runAsGroup"], sec["fsGroup"] = uid, uid, uid
		sec["runAsNonRoot"] = uid != 0
	}

	var volumes, mounts, inits []any
	if s.Artifact != nil && rt.Artifact.Kind != runtimes.ArtifactNone {
		mountDir := rt.Artifact.Path
		if rt.Artifact.Kind == runtimes.ArtifactFile {
			mountDir = path.Dir(rt.Artifact.Path)
		}
		volumes = append(volumes, map[string]any{"name": "artifact", "emptyDir": map[string]any{}})
		mounts = append(mounts, map[string]any{"name": "artifact", "mountPath": mountDir})
		inits = append(inits, s.fetchContainer())
	}
	files, err := s.configFiles()
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		volumes = append(volumes, map[string]any{"name": "files", "configMap": map[string]any{"name": s.name() + "-files"}})
		keys := slices.Sorted(maps.Keys(files))
		for _, k := range keys {
			mounts = append(mounts, map[string]any{"name": "files", "mountPath": files[k].path, "subPath": k, "readOnly": true})
		}
	}

	if len(s.Files) > 0 {
		volumes = append(volumes, map[string]any{"name": "userfiles", "secret": map[string]any{"secretName": s.name() + "-userfiles"}})
		for i, f := range s.Files {
			mounts = append(mounts, map[string]any{"name": "userfiles", "mountPath": f.Path, "subPath": userFileKey(i), "readOnly": true})
		}
	}
	for _, v := range cfg.Volumes {
		volumes = append(volumes, map[string]any{"name": "vol-" + v.Name, "persistentVolumeClaim": map[string]any{"claimName": VolumeClaimName(s.App.Slug, v.Name)}})
		mounts = append(mounts, map[string]any{"name": "vol-" + v.Name, "mountPath": v.MountPath})
	}

	c := map[string]any{
		"name":            "app",
		"image":           s.Image,
		"imagePullPolicy": "IfNotPresent",
		"envFrom":         []any{map[string]any{"secretRef": map[string]any{"name": s.name() + "-env"}}},
		"resources":       resourcesFor(cfg.Resources),
	}
	cmd := cfg.Command
	if len(cmd) == 0 {
		cmd = rt.Command
	}
	if len(command) > 0 {
		cmd = command
	}
	if len(cmd) > 0 {
		c["command"] = toAny(cmd)
		if len(rt.Args) > 0 && len(command) == 0 && len(cfg.Command) == 0 {
			c["args"] = toAny(rt.Args)
		}
	}
	if len(mounts) > 0 {
		c["volumeMounts"] = mounts
	}
	csec := map[string]any{"allowPrivilegeEscalation": false}
	if rt.RunAsUser != nil && *rt.RunAsUser != 0 {
		csec["capabilities"] = map[string]any{"drop": []any{"ALL"}}
	}
	c["securityContext"] = csec

	if role == "workload" {
		var ports []any
		if s.App.Kind == runtimes.KindWeb && cfg.Port > 0 {
			ports = append(ports, map[string]any{"name": "http", "containerPort": cfg.Port, "protocol": "TCP"})
		}
		if s.App.Kind == runtimes.KindWorker && cfg.Port > 0 {
			ports = append(ports, map[string]any{"name": "tcp", "containerPort": cfg.Port, "protocol": "TCP"})
		}
		for i, p := range cfg.Expose {
			if p.Port == cfg.Port && (s.App.Kind == runtimes.KindWeb || s.App.Kind == runtimes.KindWorker) {
				continue
			}
			proto := strings.ToUpper(p.Protocol)
			if proto == "" {
				proto = "TCP"
			}
			ports = append(ports, map[string]any{"name": fmt.Sprintf("p%d-%s", i, strings.ToLower(proto)), "containerPort": p.Port, "protocol": proto})
		}
		if len(ports) > 0 {
			c["ports"] = ports
		}
		if s.App.Kind == runtimes.KindWeb {
			if probe := probeFor(cfg); probe != nil {
				startup := clone(probe)
				startup["periodSeconds"], startup["failureThreshold"] = 5, 60 // up to 5 minutes to start (JVMs…)
				if cfg.Health.InitialDelaySeconds > 0 {
					startup["initialDelaySeconds"] = cfg.Health.InitialDelaySeconds
				}
				ready := clone(probe)
				ready["periodSeconds"], ready["failureThreshold"] = 5, 3
				live := clone(probe)
				live["periodSeconds"], live["failureThreshold"] = 15, 4
				c["startupProbe"], c["readinessProbe"], c["livenessProbe"] = startup, ready, live
			}
		}
	}

	spec := map[string]any{
		"serviceAccountName":            AppServiceAccount,
		"automountServiceAccountToken":  false,
		"enableServiceLinks":            false,
		"securityContext":               sec,
		"terminationGracePeriodSeconds": 30,
		"containers":                    []any{c},
	}
	if len(inits) > 0 {
		spec["initContainers"] = inits
	}
	if len(volumes) > 0 {
		spec["volumes"] = volumes
	}
	return spec, nil
}

// fetchScript downloads the artifact, verifies its checksum and puts it into /out.
const fetchScript = `set -eu
wget -q -O /tmp/artifact "$ARTIFACT_URL"
echo "$ARTIFACT_SHA256  /tmp/artifact" | sha256sum -c - >/dev/null
if [ "$ARTIFACT_KIND" = file ]; then
  cp /tmp/artifact "/out/$ARTIFACT_FILE"
  chmod "$ARTIFACT_MODE" "/out/$ARTIFACT_FILE"
else
  mkdir -p /tmp/x
  if [ "$ARTIFACT_FORMAT" = zip ]; then unzip -q /tmp/artifact -d /tmp/x; else tar -xzf /tmp/artifact -C /tmp/x; fi
  rm -rf /tmp/x/__MACOSX
  src=/tmp/x
  if [ -n "$ARTIFACT_STRIP" ] && [ -d "/tmp/x/$ARTIFACT_STRIP" ]; then src="/tmp/x/$ARTIFACT_STRIP"; fi
  cp -a "$src"/. /out/
fi
rm -rf /tmp/artifact /tmp/x
echo "artifact $ARTIFACT_SHA256 ready"`

func (s AppSpec) fetchContainer() map[string]any {
	rt := s.Runtime
	mode := rt.Artifact.Mode
	if mode == "" {
		mode = "0644"
	}
	env := []any{
		envVar("ARTIFACT_URL", s.ArtifactURL),
		envVar("ARTIFACT_SHA256", s.Artifact.SHA256),
		envVar("ARTIFACT_KIND", rt.Artifact.Kind),
		envVar("ARTIFACT_FILE", path.Base(rt.Artifact.Path)),
		envVar("ARTIFACT_MODE", mode),
		envVar("ARTIFACT_FORMAT", s.ArchiveFormat),
		envVar("ARTIFACT_STRIP", strings.TrimSuffix(s.StripPrefix, "/")),
	}
	csec := map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []any{"ALL"}}}
	return map[string]any{
		"name":            "fetch-artifact",
		"image":           InitImage,
		"imagePullPolicy": "IfNotPresent",
		"command":         []any{"sh", "-c", fetchScript},
		"env":             env,
		"volumeMounts":    []any{map[string]any{"name": "artifact", "mountPath": "/out"}},
		"resources":       map[string]any{"requests": map[string]any{"cpu": "50m", "memory": "32Mi"}, "limits": map[string]any{"memory": "256Mi"}},
		"securityContext": csec,
	}
}

func (s AppSpec) envData() (map[string]any, error) {
	all := map[string]string{}
	for k, v := range s.Runtime.Env {
		all[k] = v
	}
	if s.Config.Port > 0 {
		all["PORT"] = strconv.Itoa(s.Config.Port)
	}
	all["APP_ARGS"] = s.Config.Args
	all["NECTAR_APP"] = s.App.Slug
	all["NECTAR_PROJECT"] = s.Project.Slug
	all["NECTAR_RELEASE"] = strconv.Itoa(s.Release)
	for k, v := range s.Env { // user values win over runtime defaults
		all[k] = v
	}
	out := map[string]any{}
	for k, v := range all {
		if !envKeyRe.MatchString(k) {
			return nil, fmt.Errorf("invalid environment variable name %q", k)
		}
		out[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return out, nil
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// ValidEnvKey reports whether k is a valid environment variable name.
func ValidEnvKey(k string) bool { return envKeyRe.MatchString(k) && len(k) <= 253 }

type renderedFile struct {
	path    string
	content string
}

// configFiles renders the runtime's templated config files, keyed by ConfigMap key.
func (s AppSpec) configFiles() (map[string]renderedFile, error) {
	if len(s.Runtime.ConfigFiles) == 0 {
		return nil, nil
	}
	opts := s.Runtime.OptionDefaults()
	for k, v := range s.Config.Options {
		opts[k] = v
	}
	data := map[string]any{"Port": s.Config.Port, "Options": opts}
	out := map[string]renderedFile{}
	for i, f := range s.Runtime.ConfigFiles {
		t, err := template.New(f.Path).Option("missingkey=zero").Parse(f.Template)
		if err != nil {
			return nil, fmt.Errorf("runtime %s config file %s: %w", s.Runtime.ID, f.Path, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("runtime %s config file %s: %w", s.Runtime.ID, f.Path, err)
		}
		key := fmt.Sprintf("f%d-%s", i, strings.Trim(nonKey.ReplaceAllString(path.Base(f.Path), "-"), "-"))
		out[key] = renderedFile{path: f.Path, content: buf.String()}
	}
	return out, nil
}

var nonKey = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func probeFor(cfg store.AppConfig) map[string]any {
	switch cfg.Health.Type {
	case "http":
		p := cfg.Health.Path
		if p == "" {
			p = "/"
		}
		return map[string]any{"httpGet": map[string]any{"path": p, "port": "http"}, "timeoutSeconds": 3}
	case "tcp", "":
		if cfg.Port == 0 {
			return nil
		}
		return map[string]any{"tcpSocket": map[string]any{"port": "http"}, "timeoutSeconds": 3}
	}
	return nil // none
}

func resourcesFor(r store.Resources) map[string]any {
	req := map[string]any{}
	lim := map[string]any{}
	if r.CPUMillicores > 0 {
		req["cpu"] = fmt.Sprintf("%dm", r.CPUMillicores)
		if !r.NoLimit {
			lim["cpu"] = fmt.Sprintf("%dm", r.CPUMillicores)
		}
	}
	if r.MemoryMB > 0 {
		req["memory"] = fmt.Sprintf("%dMi", r.MemoryMB)
		if !r.NoLimit {
			lim["memory"] = fmt.Sprintf("%dMi", r.MemoryMB)
		}
	}
	out := map[string]any{}
	if len(req) > 0 {
		out["requests"] = req
	}
	if len(lim) > 0 {
		out["limits"] = lim
	}
	return out
}

func meta(name, ns string, labels map[string]any) map[string]any {
	m := map[string]any{"name": name, "labels": maps.Clone(labels)}
	if ns != "" {
		m["namespace"] = ns
	}
	return m
}

func merge(a, b map[string]any) map[string]any {
	out := maps.Clone(a)
	maps.Copy(out, b)
	return out
}

func clone(m map[string]any) map[string]any { return maps.Clone(m) }

func envVar(k, v string) map[string]any { return map[string]any{"name": k, "value": v} }

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// trimName keeps generated names within the 63-character DNS label limit.
func userFileKey(i int) string { return fmt.Sprintf("file-%02d", i) }

func trimName(n string) string {
	if len(n) <= 63 {
		return n
	}
	return strings.TrimRight(n[:63], "-")
}
