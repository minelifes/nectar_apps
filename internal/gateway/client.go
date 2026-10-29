// Package gateway is the panel's client for the nectar-gateway admin API.
package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ManagedBy marks gateway routes owned by the panel.
const ManagedBy = "nectar-apps"

// Route is the subset of a gateway route the panel manages.
type Route struct {
	ID           int64  `json:"id,omitempty"`
	Domain       string `json:"domain"`
	PathPrefix   string `json:"path_prefix"`
	Mode         string `json:"mode"`
	TargetIP     string `json:"target_ip"`
	TargetPort   int    `json:"target_port"`
	HCEnabled    bool   `json:"hc_enabled"`
	HCPath       string `json:"hc_path"`
	HCInterval   int    `json:"hc_interval"`
	ForceSSL     bool   `json:"force_ssl"`
	ACME         bool   `json:"acme"`
	Maintenance  bool   `json:"maintenance"`
	TargetHTTPS  bool   `json:"target_https,omitempty"`
	ManagedBy    string `json:"managed_by"`
	Owner        string `json:"owner"`
	CertSource   string `json:"cert_source,omitempty"`
	CertNotAfter int64  `json:"cert_not_after,omitempty"`

	RedirectURL     string `json:"redirect_url,omitempty"`
	RedirectCode    int    `json:"redirect_code,omitempty"`
	BackendProtocol string `json:"backend_protocol,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	MaxBodyMB       int    `json:"max_body_mb"`
	RateLimit       int    `json:"rate_limit"` // requests/s per client IP, 0 = unlimited
	// BasicAuthHtpasswd is sent on create/update ("" removes basic auth); never returned.
	BasicAuthHtpasswd *string `json:"basic_auth_htpasswd,omitempty"`
	// AuthFingerprint is returned by the gateway: SHA-256(htpasswd)[:16 hex], "" = none.
	AuthFingerprint string `json:"auth_fingerprint,omitempty"`
}

// AuthFingerprint computes the gateway's fingerprint of htpasswd text (must match the gateway).
func AuthFingerprint(htpasswd string) string {
	if htpasswd == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(htpasswd))
	return hex.EncodeToString(sum[:8])
}

// Version is the gateway's /api/version answer.
type Version struct {
	Version    string `json:"version"`
	APIVersion int    `json:"api_version"`
	Headless   bool   `json:"headless"`
}

// StatusError is a non-2xx answer from the gateway.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string { return fmt.Sprintf("gateway: %d %s", e.Code, e.Message) }

// IsConflict reports a 409 (e.g. host+path already taken by another route).
func IsConflict(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusConflict
}

// IsNotFound reports a 404.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

// Client talks to the gateway admin API with the service token.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client for baseURL (e.g. http://nectar-gateway-admin.nectar-system.svc:8081).
func New(baseURL, token string) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), token: token, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gateway unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		if json.Unmarshal(b, &e) != nil || e.Error == "" {
			e.Error = strings.TrimSpace(string(b))
		}
		return &StatusError{Code: resp.StatusCode, Message: e.Error}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Version returns the gateway version (also a reachability check).
func (c *Client) Version(ctx context.Context) (Version, error) {
	var v Version
	err := c.do(ctx, http.MethodGet, "/api/version", nil, &v)
	return v, err
}

// ListManaged returns the routes managed by the panel.
func (c *Client) ListManaged(ctx context.Context) ([]Route, error) {
	var rs []Route
	err := c.do(ctx, http.MethodGet, "/api/routes?managed_by="+url.QueryEscape(ManagedBy), nil, &rs)
	return rs, err
}

// ListManagedBy returns the routes of one controller ("" = every route).
func (c *Client) ListManagedBy(ctx context.Context, managedBy string) ([]Route, error) {
	var rs []Route
	path := "/api/routes"
	if managedBy != "" {
		path += "?managed_by=" + url.QueryEscape(managedBy)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &rs)
	return rs, err
}

// CreateRaw creates a route from a full gateway route object (fields the Route struct does not carry,
// e.g. rate_limit and health-check settings when importing a standalone gateway).
func (c *Client) CreateRaw(ctx context.Context, r map[string]any) (Route, error) {
	var out Route
	err := c.do(ctx, http.MethodPost, "/api/routes", r, &out)
	return out, err
}

// Settings are the gateway's runtime settings.
type Settings struct {
	FallbackHTTP string `json:"fallback_http"`
	FallbackTLS  string `json:"fallback_tls"`
}

// GetSettings returns the runtime settings.
func (c *Client) GetSettings(ctx context.Context) (Settings, error) {
	var s Settings
	err := c.do(ctx, http.MethodGet, "/api/settings", nil, &s)
	return s, err
}

// PutSettings replaces the runtime settings.
func (c *Client) PutSettings(ctx context.Context, s Settings) (Settings, error) {
	var out Settings
	err := c.do(ctx, http.MethodPut, "/api/settings", s, &out)
	return out, err
}

// FallbackHit is a host that was served by the fallback upstream.
type FallbackHit struct {
	Host      string    `json:"host"`
	HTTP      int64     `json:"http"`
	TLS       int64     `json:"tls"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// FallbackHits lists the hosts that still reach the old proxy through the gateway.
func (c *Client) FallbackHits(ctx context.Context) ([]FallbackHit, error) {
	var out struct {
		Hosts []FallbackHit `json:"hosts"`
	}
	err := c.do(ctx, http.MethodGet, "/api/fallback-hits", nil, &out)
	return out.Hosts, err
}

// Block adds an IP to the gateway blocklist (ttl 0 = practically permanent).
func (c *Client) Block(ctx context.Context, ip, reason string, ttl time.Duration) error {
	return c.do(ctx, http.MethodPost, "/api/blocked", map[string]any{"ip": ip, "reason": reason, "ttl_seconds": int64(ttl.Seconds())}, nil)
}

// Create creates a route.
func (c *Client) Create(ctx context.Context, r Route) (Route, error) {
	var out Route
	err := c.do(ctx, http.MethodPost, "/api/routes", r, &out)
	return out, err
}

// Update replaces a route.
func (c *Client) Update(ctx context.Context, r Route) (Route, error) {
	var out Route
	err := c.do(ctx, http.MethodPut, fmt.Sprintf("/api/routes/%d", r.ID), r, &out)
	return out, err
}

// Delete removes a route (missing routes are fine).
func (c *Client) Delete(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/routes/%d", id), nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// IssueCert asks the gateway to obtain a Let's Encrypt certificate now (synchronous, may take a minute).
func (c *Client) IssueCert(ctx context.Context, id int64) (Route, error) {
	var out Route
	err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/routes/%d/cert", id), nil, &out)
	return out, err
}

// UploadCert stores a PEM certificate for the route's host.
func (c *Client) UploadCert(ctx context.Context, id int64, certPEM, keyPEM string) (Route, error) {
	var out Route
	err := c.do(ctx, http.MethodPut, fmt.Sprintf("/api/routes/%d/cert", id), map[string]string{"cert_pem": certPEM, "key_pem": keyPEM}, &out)
	return out, err
}

// ------------------------------------------------- observability passthrough --

// Raw performs a GET and returns the decoded JSON as-is (metrics, access log, health, blocked IPs, settings).
func (c *Client) Raw(ctx context.Context, path string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// Send performs a write request with a JSON body and returns the decoded answer.
func (c *Client) Send(ctx context.Context, method, path string, in any) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, method, path, in, &out)
	if errors.Is(err, io.EOF) { // 204 No Content
		return nil, nil
	}
	return out, err
}

// Summary is one domain in the gateway's /api/metrics answer.
type Summary struct {
	Domain    string           `json:"domain"`
	Minutes   int              `json:"minutes"`
	Requests  int64            `json:"requests"`
	RPS       float64          `json:"rps"`
	Status    map[string]int64 `json:"status"`
	ErrorRate float64          `json:"error_rate"`
	BytesOut  int64            `json:"bytes_out"`
	BytesIn   int64            `json:"bytes_in"`
	P50MS     int64            `json:"p50_ms"`
	P95MS     int64            `json:"p95_ms"`
	P99MS     int64            `json:"p99_ms"`
	LastSeen  time.Time        `json:"last_seen"`
	Series    []map[string]any `json:"series,omitempty"`
}

// Metrics returns per-domain request statistics for the last `minutes` minutes.
func (c *Client) Metrics(ctx context.Context, minutes int, domain string, series bool) ([]Summary, error) {
	q := url.Values{"minutes": {fmt.Sprint(minutes)}}
	if domain != "" {
		q.Set("domain", domain)
	}
	if series {
		q.Set("series", "1")
	}
	var out struct {
		Domains []Summary `json:"domains"`
	}
	err := c.do(ctx, http.MethodGet, "/api/metrics?"+q.Encode(), nil, &out)
	return out.Domains, err
}

// AccessLog returns recent requests, newest first. query may hold domain, status (e.g. "5xx"), ip, limit.
func (c *Client) AccessLog(ctx context.Context, query url.Values) ([]map[string]any, error) {
	var out []map[string]any
	err := c.do(ctx, http.MethodGet, "/api/access-log?"+query.Encode(), nil, &out)
	return out, err
}
