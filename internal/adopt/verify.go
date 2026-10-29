package adopt

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Response is what one endpoint answered.
type Response struct {
	Status      int    `json:"status"`
	Location    string `json:"location,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	BodySHA     string `json:"body_sha,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Probe compares the old proxy with the gateway for one host, path and scheme.
type Probe struct {
	Host     string   `json:"host"`
	Path     string   `json:"path"`
	Scheme   string   `json:"scheme"` // http | https
	Fallback bool     `json:"fallback,omitempty"`
	Old      Response `json:"old"`
	New      Response `json:"new"`
	Verdict  Status   `json:"verdict"` // ok | warn (body differs) | fallback (= mismatch)
	Note     string   `json:"note,omitempty"`
}

// VerifyResult is a set of probes.
type VerifyResult struct {
	At     time.Time `json:"at"`
	Probes []Probe   `json:"probes"`
	OK     int       `json:"ok"`
	Warn   int       `json:"warn"`
	Failed int       `json:"failed"`
}

// Prober performs one request to ip:port with the given Host / SNI, without following redirects.
type Prober func(ctx context.Context, scheme, ip string, port int, host, path string) Response

// DefaultProber sends real requests (TLS certificates are not verified: only the answers are compared).
func DefaultProber(ctx context.Context, scheme, ip string, port int, host, path string) Response {
	addr := net.JoinHostPort(ip, fmt.Sprint(port))
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		},
		TLSClientConfig:   &tls.Config{ServerName: strings.TrimPrefix(host, "*."), InsecureSkipVerify: true}, //nolint:gosec // comparing answers only
		DisableKeepAlives: true,
	}
	cl := &http.Client{Transport: tr, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	h := strings.TrimPrefix(host, "*.")
	if strings.HasPrefix(host, "*.") {
		h = "nectar-probe." + h
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+h+path, nil)
	if err != nil {
		return Response{Error: err.Error()}
	}
	req.Header.Set("User-Agent", "nectar-adopt-verify/1")
	resp, err := cl.Do(req)
	if err != nil {
		return Response{Error: shortErr(err)}
	}
	defer resp.Body.Close()
	sum := sha256.New()
	_, _ = io.Copy(sum, io.LimitReader(resp.Body, 1<<20))
	ct, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	return Response{Status: resp.StatusCode, Location: resp.Header.Get("Location"), ContentType: strings.TrimSpace(ct),
		BodySHA: hex.EncodeToString(sum.Sum(nil))[:16]}
}

func shortErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i > 0 && len(s) > 120 {
		s = s[i+2:]
	}
	return s
}

// compare decides the verdict for one probe.
func compare(p *Probe) {
	o, n := p.Old, p.New
	switch {
	case o.Error != "" && n.Error != "":
		p.Verdict, p.Note = StatusWarn, "both failed: "+o.Error
	case n.Error != "":
		p.Verdict, p.Note = StatusFallback, "gateway: "+n.Error
	case o.Error != "":
		p.Verdict, p.Note = StatusWarn, "old proxy failed ("+o.Error+"), gateway answered"
	case o.Status != n.Status:
		p.Verdict, p.Note = StatusFallback, fmt.Sprintf("status %d → %d", o.Status, n.Status)
	case normLocation(o.Location) != normLocation(n.Location):
		p.Verdict, p.Note = StatusFallback, fmt.Sprintf("redirect %q → %q", o.Location, n.Location)
	case o.ContentType != n.ContentType:
		p.Verdict, p.Note = StatusWarn, fmt.Sprintf("content type %s → %s", o.ContentType, n.ContentType)
	case o.BodySHA != n.BodySHA:
		p.Verdict, p.Note = StatusWarn, "same status, different body (dynamic content?)"
	default:
		p.Verdict = StatusOK
	}
}

// normLocation drops default and shadow ports so that https://a/x and https://a:443/x compare equal.
func normLocation(l string) string {
	for _, p := range []string{":443/", ":80/", ":18443/", ":18080/"} {
		l = strings.Replace(l, p, "/", 1)
	}
	return strings.TrimSuffix(l, "/")
}

// probeTargets returns host/path/scheme combinations to compare: every migrated host (first path of each
// route) and the fallback hosts, which must keep working through the gateway's fallback.
func probeTargets(routes []Route) []Probe {
	seen := map[string]bool{}
	var out []Probe
	add := func(host, path, scheme string, fb bool) {
		if host == "" || strings.HasPrefix(host, "~") {
			return
		}
		k := scheme + "://" + host + path
		if !seen[k] {
			seen[k] = true
			out = append(out, Probe{Host: host, Path: path, Scheme: scheme, Fallback: fb})
		}
	}
	for _, r := range routes {
		fb := !r.Migratable()
		add(r.Host, r.PathPrefix, "http", fb)
		if r.TLS {
			add(r.Host, r.PathPrefix, "https", fb)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func summarize(v *VerifyResult) {
	v.OK, v.Warn, v.Failed = 0, 0, 0
	for _, p := range v.Probes {
		switch p.Verdict {
		case StatusOK:
			v.OK++
		case StatusWarn:
			v.Warn++
		default:
			v.Failed++
		}
	}
}

// Text renders the result as a table for the terminal.
func (v *VerifyResult) Text() string {
	var b strings.Builder
	for _, p := range v.Probes {
		kind := ""
		if p.Fallback {
			kind = " (fallback)"
		}
		fmt.Fprintf(&b, "  %s %s://%s%s%s  %s → %s  %s\n", p.Verdict.Symbol(), p.Scheme, p.Host, p.Path, kind,
			p.Old.short(), p.New.short(), p.Note)
	}
	fmt.Fprintf(&b, "  %d ✓  %d ⚠  %d ✗\n", v.OK, v.Warn, v.Failed)
	return b.String()
}

func (r Response) short() string {
	if r.Error != "" {
		return "error"
	}
	return fmt.Sprint(r.Status)
}
