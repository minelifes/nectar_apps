// Package scan looks for known vulnerabilities in container images with Trivy.
//
// The panel does not link Trivy. A scan is a short Job in its own namespace (DefaultNamespace: nothing may
// connect to its pods, they get no Kubernetes token) that runs the trivy CLI (Image) against the image in
// its registry — the image is not started. Registry logins go in through a Secret that is deleted after the
// run. The Job prints a summary and the worst findings, which the panel reads from the pod's log.
package scan

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// Image runs the trivy CLI (pinned; NECTAR_TRIVY_IMAGE overrides).
const Image = "aquasec/trivy:0.76.1"

// DefaultNamespace is where scan Jobs run.
const DefaultNamespace = "nectar-scans"

// MaxFindings is how many vulnerabilities a scan lists (the worst first); the counts cover all of them.
const MaxFindings = 300

// Kube is the part of the cluster client the scanner uses.
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Delete(ctx context.Context, kind, namespace, name string) error
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error)
}

// Scanner runs scans.
type Scanner struct {
	Kube      Kube
	Namespace string        // default DefaultNamespace
	Image     string        // default Image
	Timeout   time.Duration // default 20 min
	Poll      time.Duration // default 2 s
}

// Request is one image to scan.
type Request struct {
	Image        string // as a pod inside the cluster reaches it
	PlainHTTP    bool   // the registry speaks HTTP (the built-in one, inside the cluster)
	DockerConfig []byte // config.json with registry logins (nil = anonymous)
}

// Finding is one vulnerability.
type Finding struct {
	ID        string `json:"id"`       // CVE-2026-1234
	Severity  string `json:"severity"` // CRITICAL | HIGH | MEDIUM | LOW | UNKNOWN
	Package   string `json:"package"`
	Installed string `json:"installed"`
	Fixed     string `json:"fixed,omitempty"` // version with the fix ("" = none yet)
	Target    string `json:"target,omitempty"`
	Title     string `json:"title,omitempty"`
}

// Result is what a scan found.
type Result struct {
	Total    int       `json:"total"`
	Critical int       `json:"critical"`
	High     int       `json:"high"`
	Medium   int       `json:"medium"`
	Low      int       `json:"low"`
	Unknown  int       `json:"unknown"`
	Fixable  int       `json:"fixable"` // a fixed version exists: updating the image helps
	Findings []Finding `json:"findings"`
}

// tpl prints one tab-separated line per vulnerability (strings quoted, so they hold no tab or newline).
const tpl = `{{ range . }}{{ $t := .Target }}{{ range .Vulnerabilities }}NV	{{ .Severity }}	{{ .VulnerabilityID }}	{{ printf "%q" .PkgName }}	{{ printf "%q" .InstalledVersion }}	{{ printf "%q" .FixedVersion }}	{{ printf "%q" $t }}	{{ printf "%q" .Title }}
{{ end }}{{ end }}`

// script scans, then prints a summary (NVSUM), the worst findings and NVDONE.
const script = `set -u
export HOME=/tmp
trivy image --quiet --no-progress --scanners vuln --timeout "$SCAN_TIMEOUT" --cache-dir /tmp/cache \
  --format template --template "$TPL" --output /tmp/out.txt "$IMAGE" || exit 1
grep '^NV' /tmp/out.txt > /tmp/v.txt || true
awk -F'\t' '{c[$2]++; if ($6 != "\"\"") f++} END {printf "NVSUM\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", NR, c["CRITICAL"], c["HIGH"], c["MEDIUM"], c["LOW"], c["UNKNOWN"], f}' /tmp/v.txt
for s in CRITICAL HIGH MEDIUM LOW UNKNOWN; do awk -F'\t' -v s="$s" '$2 == s' /tmp/v.txt; done | head -n "$MAX" | cut -c1-600
echo NVDONE`

func (s *Scanner) ns() string {
	if s.Namespace != "" {
		return s.Namespace
	}
	return DefaultNamespace
}

func (s *Scanner) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return 20 * time.Minute
}

func (s *Scanner) ensureNamespace(ctx context.Context) error {
	ns := s.ns()
	labels := map[string]any{"app.kubernetes.io/part-of": "nectar", "app.kubernetes.io/managed-by": "nectar-apps",
		"pod-security.kubernetes.io/enforce": "restricted"}
	if _, err := s.Kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]any{"name": ns, "labels": labels}}); err != nil {
		return err
	}
	_, err := s.Kube.Apply(ctx, kube.Object{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
		"metadata": map[string]any{"name": "deny-ingress", "namespace": ns, "labels": map[string]any{"app.kubernetes.io/part-of": "nectar"}},
		"spec":     map[string]any{"podSelector": map[string]any{}, "policyTypes": []any{"Ingress"}}})
	return err
}

// Scan scans one image and waits for the result. label names the scan in the Job (e.g. "app-12").
func (s *Scanner) Scan(ctx context.Context, label string, req Request) (Result, error) {
	if strings.TrimSpace(req.Image) == "" || strings.ContainsAny(req.Image, " \t\n") {
		return Result{}, errors.New("no image to scan")
	}
	if err := s.ensureNamespace(ctx); err != nil {
		return Result{}, fmt.Errorf("namespace: %w", err)
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	ns, name := s.ns(), "scan-"+label+"-"+hex.EncodeToString(b)
	labels := map[string]any{"app.kubernetes.io/part-of": "nectar", "nectar.dev/scan": label}
	cfg := req.DockerConfig
	if len(cfg) == 0 {
		cfg = []byte(`{"auths":{}}`)
	}
	if _, err := s.Kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
		"data":     map[string]any{"config.json": base64.StdEncoding.EncodeToString(cfg)}}); err != nil {
		return Result{}, fmt.Errorf("input secret: %w", err)
	}
	defer s.Kube.Delete(context.WithoutCancel(ctx), "Secret", ns, name)

	env := []any{
		map[string]any{"name": "IMAGE", "value": req.Image},
		map[string]any{"name": "TPL", "value": tpl},
		map[string]any{"name": "MAX", "value": strconv.Itoa(MaxFindings)},
		map[string]any{"name": "SCAN_TIMEOUT", "value": fmt.Sprintf("%dm", int(s.timeout().Minutes())-2)},
		map[string]any{"name": "DOCKER_CONFIG", "value": "/input"},
	}
	if req.PlainHTTP {
		env = append(env, map[string]any{"name": "TRIVY_NON_SSL", "value": "true"}, map[string]any{"name": "TRIVY_INSECURE", "value": "true"})
	}
	image := s.Image
	if image == "" {
		image = Image
	}
	job := kube.Object{"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
		"spec": map[string]any{
			"backoffLimit":            0,
			"activeDeadlineSeconds":   int(s.timeout().Seconds()),
			"ttlSecondsAfterFinished": 600,
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"restartPolicy":                "Never",
					"automountServiceAccountToken": false,
					"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532, "runAsGroup": 65532, "fsGroup": 65532,
						"seccompProfile": map[string]any{"type": "RuntimeDefault"}},
					"containers": []any{map[string]any{
						"name": "trivy", "image": image, "command": []any{"sh", "-c", script}, "env": env,
						"resources": map[string]any{
							"requests": map[string]any{"cpu": "100m", "memory": "256Mi"},
							"limits":   map[string]any{"cpu": "1", "memory": "1536Mi"},
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
						map[string]any{"name": "tmp", "emptyDir": map[string]any{}}, // the vulnerability database and the image's layers
					},
				},
			},
		}}
	if _, err := s.Kube.Apply(ctx, job); err != nil {
		return Result{}, fmt.Errorf("job: %w", err)
	}
	defer s.Kube.Delete(context.WithoutCancel(ctx), "Job", ns, name)
	ok, err := s.wait(ctx, ns, name)
	out := s.logs(context.WithoutCancel(ctx), ns, name)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, errors.New(failure(out))
	}
	res, done := Parse(out)
	if !done {
		return Result{}, errors.New("the scan's output was cut off: " + failure(out))
	}
	return res, nil
}

// wait: true when the Job succeeded, false when it failed.
func (s *Scanner) wait(ctx context.Context, ns, name string) (bool, error) {
	poll := s.Poll
	if poll <= 0 {
		poll = 2 * time.Second
	}
	deadline := time.Now().Add(s.timeout() + time.Minute)
	for {
		var job kube.Object
		if err := s.Kube.GetObject(ctx, "Job", ns, name, &job); err != nil {
			return false, err
		}
		switch {
		case kube.Int(job, "status", "succeeded") > 0:
			return true, nil
		case kube.Int(job, "status", "failed") > 0:
			return false, nil
		case time.Now().After(deadline):
			return false, errors.New("the scan timed out")
		}
		if msg := s.stuck(ctx, ns, name); msg != "" {
			return false, errors.New(msg)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(poll):
		}
	}
}

// stuck: a pod that can't start never fails the Job.
func (s *Scanner) stuck(ctx context.Context, ns, job string) string {
	pods, err := s.Kube.List(ctx, "Pod", ns, "job-name="+job)
	if err != nil {
		return ""
	}
	for _, p := range pods {
		sts, _ := kube.Field(p, "status", "containerStatuses").([]any)
		for _, st := range sts {
			w, _ := st.(map[string]any)
			switch reason := kube.Str(kube.Object(w), "state", "waiting", "reason"); reason {
			case "ErrImagePull", "ImagePullBackOff", "InvalidImageName", "CreateContainerConfigError":
				return "the scanner pod can't start: " + reason + " " + kube.Str(kube.Object(w), "state", "waiting", "message")
			}
		}
	}
	return ""
}

func (s *Scanner) logs(ctx context.Context, ns, job string) string {
	pods, err := s.Kube.List(ctx, "Pod", ns, "job-name="+job)
	if err != nil || len(pods) == 0 {
		return ""
	}
	_, _, pod := kube.Meta(pods[0])
	out, _ := s.Kube.PodLogs(ctx, ns, pod, "trivy", MaxFindings+50, false)
	return out
}

// failure is trivy's own error from its output (its last lines that are not findings).
func failure(out string) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "NV") {
			lines = append(lines, l)
		}
	}
	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}
	if len(lines) == 0 {
		return "the scan failed"
	}
	msg := strings.Join(lines, "\n")
	if len(msg) > 1000 {
		msg = msg[len(msg)-1000:]
	}
	return msg
}

// Parse reads the Job's output. done is false when the end marker is missing (failed or cut off).
func Parse(out string) (res Result, done bool) {
	res.Findings = []Finding{}
	sum := false
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		switch f[0] {
		case "NVDONE":
			done = true
		case "NVSUM":
			if len(f) != 8 {
				continue
			}
			n := make([]int, 7)
			for i := range n {
				n[i], _ = strconv.Atoi(f[i+1])
			}
			res.Total, res.Critical, res.High, res.Medium, res.Low, res.Unknown, res.Fixable = n[0], n[1], n[2], n[3], n[4], n[5], n[6]
			sum = true
		case "NV":
			if len(f) < 7 {
				continue
			}
			v := Finding{Severity: f[1], ID: f[2], Package: unq(f[3]), Installed: unq(f[4]), Fixed: unq(f[5]), Target: unq(f[6])}
			if len(f) > 7 {
				v.Title = unq(strings.Join(f[7:], "\t"))
			}
			res.Findings = append(res.Findings, v)
		}
	}
	return res, done && sum
}

// unq undoes the template's %q (a line cut by the length limit loses its closing quote).
func unq(s string) string {
	if u, err := strconv.Unquote(s); err == nil {
		return u
	}
	s = strings.TrimPrefix(s, `"`)
	if u, err := strconv.Unquote(`"` + strings.TrimRight(s, `\`) + `"`); err == nil {
		return u + "…"
	}
	return s
}
