// Package helm installs, upgrades, rolls back and removes Helm charts in a project namespace.
//
// The panel does not link Helm. Each operation is a short Job in the project's namespace that runs the
// helm CLI (Image) under its own ServiceAccount, which may do anything inside that namespace and nothing
// outside it: a chart can't touch other projects or the cluster (charts that install CRDs or
// cluster-wide roles fail with Helm's "forbidden" error). Values and repository logins go in through a
// Secret that is deleted after the run. The release's history is read straight from Helm's own release
// Secrets (sh.helm.release.v1.<name>.v<N>), so the panel always shows what Helm really has.
package helm

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// Image runs the helm CLI (pinned).
const Image = "alpine/helm:3.17.3"

// Names of the objects the runner keeps in a project namespace.
const (
	ServiceAccount = "nectar-helm"
	labelManagedBy = "app.kubernetes.io/managed-by"
	managedBy      = "nectar-apps"
	labelHelmOp    = "nectar.dev/helm-release"
)

// Kube is the part of the cluster client the runner uses.
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Delete(ctx context.Context, kind, namespace, name string) error
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error)
}

// Op is one helm action.
type Op struct {
	Kind     string // install (helm upgrade --install) | rollback | uninstall
	Name     string // release name
	Repo     string // https://… chart repository, or oci://registry/path
	Chart    string // chart name in the repository
	Version  string // "" = latest
	Values   string // values.yaml
	Username string // repository login (optional)
	Password string
	Revision int // rollback target
}

var (
	nameRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,51}[a-z0-9])?$`)
	chartRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)
	verRe   = regexp.MustCompile(`^[A-Za-z0-9.+_~^<>=*, -]{0,64}$`)
)

// ValidName: a Helm release name that also works in object names (≤ 53 characters, DNS label).
func ValidName(n string) bool { return nameRe.MatchString(n) }

// Check validates the chart reference of an install.
func (o Op) Check() error {
	switch {
	case !ValidName(o.Name):
		return errors.New("name: lowercase letters, digits and -, at most 53 characters")
	case !strings.HasPrefix(o.Repo, "https://") && !strings.HasPrefix(o.Repo, "http://") && !strings.HasPrefix(o.Repo, "oci://"):
		return errors.New("repository must be an https:// chart repository or an oci:// registry path")
	case strings.ContainsAny(o.Repo, " \t\n'\"`$\\"):
		return errors.New("repository: invalid characters")
	case !chartRe.MatchString(o.Chart) || strings.Contains(o.Chart, ".."):
		return errors.New("chart: a chart name such as nginx")
	case !verRe.MatchString(o.Version):
		return errors.New("version: a chart version or range such as 1.2.3 or ^1.2")
	case len(o.Values) > 512<<10:
		return errors.New("values are larger than 512 KB")
	}
	return nil
}

// Runner runs helm Jobs.
type Runner struct {
	Kube    Kube
	Image   string        // default Image
	Timeout time.Duration // per operation, default 15 min
	Poll    time.Duration // default 2 s
}

// script runs one operation; everything comes in through the environment and /input.
const script = `set -eu
export HOME=/tmp HELM_CACHE_HOME=/tmp/cache HELM_CONFIG_HOME=/tmp/config HELM_DATA_HOME=/tmp/data
user=$(cat /input/username 2>/dev/null || true); pass=$(cat /input/password 2>/dev/null || true)
case "$OP" in
install)
  set -- upgrade --install "$NAME" -n "$NS" -f /input/values.yaml --wait --timeout 10m --atomic --history-max 10
  [ -n "$VERSION" ] && set -- "$@" --version "$VERSION"
  case "$REPO" in
  oci://*)
    if [ -n "$user" ]; then
      host=${REPO#oci://}; host=${host%%/*}
      printf '%s' "$pass" | helm registry login "$host" --username "$user" --password-stdin
    fi
    set -- "$@" "$REPO/$CHART" ;;
  *)
    set -- "$@" --repo "$REPO" "$CHART"
    [ -n "$user" ] && set -- "$@" --username "$user" --password "$pass"
    ;;
  esac
  helm "$@" ;;
rollback)
  helm rollback "$NAME" "$REVISION" -n "$NS" --wait --timeout 10m --history-max 10 ;;
uninstall)
  helm uninstall "$NAME" -n "$NS" --wait --timeout 10m ;;
*)
  echo "unknown operation $OP" >&2; exit 2 ;;
esac`

// EnsureAccess creates the namespace-only ServiceAccount, Role and RoleBinding the Jobs run as.
func (r *Runner) EnsureAccess(ctx context.Context, ns string) error {
	labels := map[string]any{labelManagedBy: managedBy}
	for _, o := range []kube.Object{
		{"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": map[string]any{"name": ServiceAccount, "namespace": ns, "labels": labels}},
		{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
			"metadata": map[string]any{"name": ServiceAccount, "namespace": ns, "labels": labels},
			"rules":    []any{map[string]any{"apiGroups": []any{"*"}, "resources": []any{"*"}, "verbs": []any{"*"}}}},
		{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
			"metadata": map[string]any{"name": ServiceAccount, "namespace": ns, "labels": labels},
			"roleRef":  map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": ServiceAccount},
			"subjects": []any{map[string]any{"kind": "ServiceAccount", "name": ServiceAccount, "namespace": ns}}},
	} {
		if _, err := r.Kube.Apply(ctx, o); err != nil {
			return fmt.Errorf("%v %s: %w", o["kind"], ServiceAccount, err)
		}
	}
	return nil
}

// Run performs op in namespace ns and returns helm's output. A failed operation returns the output
// and an error carrying its last lines.
func (r *Runner) Run(ctx context.Context, ns string, op Op) (string, error) {
	if op.Kind == "install" {
		if err := op.Check(); err != nil {
			return "", err
		}
	} else if !ValidName(op.Name) {
		return "", errors.New("invalid release name")
	}
	if err := r.EnsureAccess(ctx, ns); err != nil {
		return "", err
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "nectar-helm-" + op.Name + "-" + hex.EncodeToString(b)
	labels := map[string]any{labelManagedBy: managedBy, labelHelmOp: op.Name}
	input := kube.Object{"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
		"data": map[string]any{
			"values.yaml": base64.StdEncoding.EncodeToString([]byte(op.Values)),
			"username":    base64.StdEncoding.EncodeToString([]byte(op.Username)),
			"password":    base64.StdEncoding.EncodeToString([]byte(op.Password)),
		}}
	if _, err := r.Kube.Apply(ctx, input); err != nil {
		return "", fmt.Errorf("input secret: %w", err)
	}
	defer r.Kube.Delete(context.WithoutCancel(ctx), "Secret", ns, name)

	env := []any{}
	for k, v := range map[string]string{"OP": op.Kind, "NAME": op.Name, "NS": ns, "REPO": op.Repo, "CHART": op.Chart,
		"VERSION": op.Version, "REVISION": strconv.Itoa(op.Revision)} {
		env = append(env, map[string]any{"name": k, "value": v})
	}
	sort.Slice(env, func(i, j int) bool {
		return env[i].(map[string]any)["name"].(string) < env[j].(map[string]any)["name"].(string)
	})
	image := r.Image
	if image == "" {
		image = Image
	}
	job := kube.Object{"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
		"spec": map[string]any{
			"backoffLimit":            0,
			"activeDeadlineSeconds":   int(r.timeout().Seconds()),
			"ttlSecondsAfterFinished": 3600,
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"serviceAccountName": ServiceAccount,
					"restartPolicy":      "Never",
					"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532, "runAsGroup": 65532,
						"seccompProfile": map[string]any{"type": "RuntimeDefault"}},
					"containers": []any{map[string]any{
						"name": "helm", "image": image, "command": []any{"sh", "-c", script}, "env": env,
						"resources": map[string]any{
							"requests": map[string]any{"cpu": "50m", "memory": "64Mi"},
							"limits":   map[string]any{"cpu": "1", "memory": "512Mi"},
						},
						"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true,
							"capabilities": map[string]any{"drop": []any{"ALL"}}},
						"volumeMounts": []any{
							map[string]any{"name": "input", "mountPath": "/input", "readOnly": true},
							map[string]any{"name": "tmp", "mountPath": "/tmp"},
						},
					}},
					"volumes": []any{
						map[string]any{"name": "input", "secret": map[string]any{"secretName": name}},
						map[string]any{"name": "tmp", "emptyDir": map[string]any{}},
					},
				},
			},
		}}
	if _, err := r.Kube.Apply(ctx, job); err != nil {
		return "", fmt.Errorf("job: %w", err)
	}
	ok, err := r.wait(ctx, ns, name)
	out := r.logs(context.WithoutCancel(ctx), ns, name)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, errors.New(lastLines(out, 6))
	}
	return out, nil
}

func (r *Runner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return 15 * time.Minute
}

// wait: true when the Job succeeded, false when it failed.
func (r *Runner) wait(ctx context.Context, ns, name string) (bool, error) {
	poll := r.Poll
	if poll <= 0 {
		poll = 2 * time.Second
	}
	deadline := time.Now().Add(r.timeout() + time.Minute)
	for {
		var job kube.Object
		if err := r.Kube.GetObject(ctx, "Job", ns, name, &job); err != nil {
			return false, err
		}
		switch {
		case kube.Int(job, "status", "succeeded") > 0:
			return true, nil
		case kube.Int(job, "status", "failed") > 0:
			return false, nil
		case time.Now().After(deadline):
			return false, errors.New("timed out")
		}
		// a pod that can't start (image pull, quota) never fails the Job: report it
		if msg := r.stuck(ctx, ns, name); msg != "" {
			return false, errors.New(msg)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (r *Runner) stuck(ctx context.Context, ns, job string) string {
	pods, err := r.Kube.List(ctx, "Pod", ns, "job-name="+job)
	if err != nil {
		return ""
	}
	for _, p := range pods {
		sts, _ := kube.Field(p, "status", "containerStatuses").([]any)
		for _, s := range sts {
			w, _ := s.(map[string]any)
			reason := kube.Str(kube.Object(w), "state", "waiting", "reason")
			switch reason {
			case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "CreateContainerConfigError":
				return "the helm pod can't start: " + reason + " " + kube.Str(kube.Object(w), "state", "waiting", "message")
			}
		}
	}
	return ""
}

func (r *Runner) logs(ctx context.Context, ns, job string) string {
	pods, err := r.Kube.List(ctx, "Pod", ns, "job-name="+job)
	if err != nil || len(pods) == 0 {
		return ""
	}
	_, _, pod := kube.Meta(pods[0])
	out, _ := r.Kube.PodLogs(ctx, ns, pod, "helm", 400, false)
	return strings.TrimSpace(out)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	msg := strings.TrimSpace(strings.Join(lines, "\n"))
	if msg == "" {
		return "helm failed"
	}
	return msg
}

// Revision is one entry of a release's history, read from Helm's release Secret.
type Revision struct {
	Revision     int       `json:"revision"`
	Status       string    `json:"status"` // deployed | superseded | failed | pending-install | pending-upgrade | …
	Chart        string    `json:"chart"`
	ChartVersion string    `json:"chart_version"`
	AppVersion   string    `json:"app_version"`
	Updated      time.Time `json:"updated"`
	Description  string    `json:"description"`
	Notes        string    `json:"notes,omitempty"`
}

// History returns the release's revisions, newest first (empty when Helm has no such release).
func History(ctx context.Context, k Kube, ns, name string) ([]Revision, error) {
	secrets, err := k.List(ctx, "Secret", ns, "owner=helm,name="+name)
	if err != nil {
		return nil, err
	}
	var out []Revision
	for _, s := range secrets {
		data := kube.Str(s, "data", "release")
		if data == "" {
			continue
		}
		rev, err := DecodeRelease(data)
		if err != nil {
			continue
		}
		out = append(out, rev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out, nil
}

// DecodeRelease reads a Helm 3 release Secret's "release" field (as the API returns it: base64 of
// Helm's own base64 of gzipped JSON).
func DecodeRelease(data string) (Revision, error) {
	var rev Revision
	outer, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return rev, err
	}
	inner, err := base64.StdEncoding.DecodeString(string(outer))
	if err != nil {
		return rev, err
	}
	if len(inner) > 2 && inner[0] == 0x1f && inner[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(inner))
		if err != nil {
			return rev, err
		}
		if inner, err = io.ReadAll(io.LimitReader(zr, 16<<20)); err != nil {
			return rev, err
		}
	}
	var r struct {
		Version int `json:"version"`
		Info    struct {
			Status       string    `json:"status"`
			LastDeployed time.Time `json:"last_deployed"`
			Description  string    `json:"description"`
			Notes        string    `json:"notes"`
		} `json:"info"`
		Chart struct {
			Metadata struct {
				Name       string `json:"name"`
				Version    string `json:"version"`
				AppVersion string `json:"appVersion"`
			} `json:"metadata"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(inner, &r); err != nil {
		return rev, err
	}
	notes := r.Info.Notes
	if len(notes) > 8<<10 {
		notes = notes[:8<<10] + "\n…"
	}
	return Revision{Revision: r.Version, Status: r.Info.Status, Chart: r.Chart.Metadata.Name,
		ChartVersion: r.Chart.Metadata.Version, AppVersion: r.Chart.Metadata.AppVersion,
		Updated: r.Info.LastDeployed, Description: r.Info.Description, Notes: notes}, nil
}

// Workload is a Deployment, StatefulSet or DaemonSet of a release.
type Workload struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Ready    int64  `json:"ready"`
	Replicas int64  `json:"replicas"`
}

// Workloads lists what the release runs (by Helm's release-name annotation).
func Workloads(ctx context.Context, k Kube, ns, release string) []Workload {
	var out []Workload
	for _, kind := range []string{"Deployment", "StatefulSet", "DaemonSet"} {
		items, err := k.List(ctx, kind, ns, "")
		if err != nil {
			continue
		}
		for _, o := range items {
			if kube.Str(o, "metadata", "annotations", "meta.helm.sh/release-name") != release {
				continue
			}
			_, _, name := kube.Meta(o)
			w := Workload{Kind: kind, Name: name, Ready: kube.Int(o, "status", "readyReplicas"), Replicas: kube.Int(o, "spec", "replicas")}
			if kind == "DaemonSet" {
				w.Ready, w.Replicas = kube.Int(o, "status", "numberReady"), kube.Int(o, "status", "desiredNumberScheduled")
			}
			out = append(out, w)
		}
	}
	return out
}

// Services lists the release's Services with their ports (to put a domain on one: Proxies).
func Services(ctx context.Context, k Kube, ns, release string) []string {
	items, err := k.List(ctx, "Service", ns, "")
	if err != nil {
		return nil
	}
	var out []string
	for _, o := range items {
		if kube.Str(o, "metadata", "annotations", "meta.helm.sh/release-name") != release {
			continue
		}
		_, _, name := kube.Meta(o)
		ports, _ := kube.Field(o, "spec", "ports").([]any)
		for _, p := range ports {
			pm, _ := p.(map[string]any)
			out = append(out, fmt.Sprintf("%s.%s.svc.cluster.local:%v", name, ns, pm["port"]))
		}
	}
	sort.Strings(out)
	return out
}
