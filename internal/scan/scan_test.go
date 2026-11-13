package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
)

const sample = "2026-11-12T10:00:00Z\tINFO\tsome trivy log line\n" +
	"NVSUM\t4\t1\t1\t1\t1\t0\t2\n" +
	"NV\tCRITICAL\tCVE-2026-0001\t\"openssl\"\t\"3.0.1\"\t\"3.0.9\"\t\"app (debian 12.5)\"\t\"openssl: remote \\\"code\\\" execution\"\n" +
	"NV\tHIGH\tCVE-2026-0002\t\"zlib\"\t\"1.2.11\"\t\"\"\t\"app (debian 12.5)\"\t\"zlib: overflow\tin inflate\"\n" +
	"NV\tMEDIUM\tGHSA-xxxx\t\"lodash\"\t\"4.17.20\"\t\"4.17.21\"\t\"package-lock.json\"\t\"a title that was cut off by the len\n" +
	"NVDONE\n"

func TestParse(t *testing.T) {
	res, done := Parse(sample)
	if !done || res.Total != 4 || res.Critical != 1 || res.High != 1 || res.Medium != 1 || res.Low != 1 || res.Fixable != 2 || len(res.Findings) != 3 {
		t.Fatalf("%v %+v", done, res)
	}
	f := res.Findings
	if f[0].ID != "CVE-2026-0001" || f[0].Package != "openssl" || f[0].Fixed != "3.0.9" || f[0].Title != `openssl: remote "code" execution` || f[0].Target != "app (debian 12.5)" {
		t.Fatalf("first: %+v", f[0])
	}
	if f[1].Fixed != "" || f[1].Title != "zlib: overflow\tin inflate" {
		t.Fatalf("second: %+v", f[1])
	}
	if !strings.HasSuffix(f[2].Title, "len…") {
		t.Fatalf("cut title: %q", f[2].Title)
	}
	if _, done := Parse("NV\tHIGH\tCVE-1\t\"a\"\t\"1\"\t\"\"\t\"t\"\t\"x\"\n"); done {
		t.Fatal("output without the end marker accepted")
	}
	if res, done := Parse("NVSUM\t0\t0\t0\t0\t0\t0\t0\nNVDONE\n"); !done || res.Total != 0 || res.Findings == nil {
		t.Fatalf("clean image: %v %+v", done, res)
	}
}

// The Job's shell part (summary, worst first, limit) with the real sh and awk.
func TestScriptSummary(t *testing.T) {
	if _, err := exec.LookPath("awk"); err != nil {
		t.Skip("no awk")
	}
	dir := t.TempDir()
	lines := "NV\tLOW\tCVE-3\t\"c\"\t\"1\"\t\"\"\t\"t\"\t\"low\"\n" +
		"NV\tCRITICAL\tCVE-1\t\"a\"\t\"1\"\t\"2\"\t\"t\"\t\"crit\"\n" +
		"NV\tHIGH\tCVE-2\t\"b\"\t\"1\"\t\"2\"\t\"t\"\t\"high\"\n"
	os.WriteFile(filepath.Join(dir, "out.txt"), []byte("\n"+lines), 0o644)
	// the part of the script after trivy, on the prepared output
	_, tail, ok := strings.Cut(script, "|| exit 1\n")
	if !ok {
		t.Fatal("script changed")
	}
	cmd := exec.Command("sh", "-c", strings.ReplaceAll(tail, "/tmp/", dir+"/"))
	cmd.Env = append(os.Environ(), "MAX=2")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	res, done := Parse(string(out))
	if !done || res.Total != 3 || res.Critical != 1 || res.High != 1 || res.Low != 1 || res.Fixable != 2 {
		t.Fatalf("%s → %+v", out, res)
	}
	if len(res.Findings) != 2 || res.Findings[0].Severity != "CRITICAL" || res.Findings[1].Severity != "HIGH" {
		t.Fatalf("worst first, limited: %+v", res.Findings)
	}
}

type logKube struct {
	*kubefake.Fake
	logs string
}

func (l logKube) PodLogs(context.Context, string, string, string, int, bool) (string, error) {
	return l.logs, nil
}

func (l logKube) List(ctx context.Context, kind, ns, sel string) ([]kube.Object, error) {
	if kind == "Pod" {
		return []kube.Object{{"metadata": map[string]any{"name": "scan-pod", "namespace": ns}}}, nil
	}
	return l.Fake.List(ctx, kind, ns, sel)
}

func TestScanJob(t *testing.T) {
	fk := kubefake.New()
	s := &Scanner{Kube: logKube{fk, sample}, Poll: time.Millisecond}
	ctx := context.Background()
	res, err := s.Scan(ctx, "app-7", Request{Image: "nectar-registry.nectar-system.svc:5000/shop/web:12", PlainHTTP: true,
		DockerConfig: []byte(`{"auths":{"x":{}}}`)})
	if err != nil || res.Critical != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	ns := fk.Get("Namespace", "", DefaultNamespace)
	if ns == nil || kube.Str(ns, "metadata", "labels", "pod-security.kubernetes.io/enforce") != "restricted" || fk.Get("NetworkPolicy", DefaultNamespace, "deny-ingress") == nil {
		t.Fatalf("namespace: %v", ns)
	}
	// the Job and its Secret are gone afterwards
	if jobs, _ := fk.List(ctx, "Job", DefaultNamespace, ""); len(jobs) != 0 {
		t.Fatalf("job left: %v", jobs)
	}
	if secrets, _ := fk.List(ctx, "Secret", DefaultNamespace, ""); len(secrets) != 0 {
		t.Fatalf("secret left: %v", secrets)
	}

	// a failed scan reports trivy's message, not the findings
	fk.JobOK = false
	s.Kube = logKube{fk, "NV\tHIGH\tCVE-1\t\"a\"\t\"1\"\t\"\"\t\"t\"\t\"x\"\nFATAL\timage scan error: unable to find the specified image\n"}
	if _, err := s.Scan(ctx, "app-7", Request{Image: "ghcr.io/x/y:1"}); err == nil || !strings.Contains(err.Error(), "unable to find the specified image") || strings.Contains(err.Error(), "CVE-1") {
		t.Fatalf("failure: %v", err)
	}
	if _, err := s.Scan(ctx, "app-7", Request{Image: "a b"}); err == nil {
		t.Fatal("bad image accepted")
	}
}
