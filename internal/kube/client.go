// Package kube is a small Kubernetes REST client built on net/http.
//
// It supports two modes:
//   - in-cluster: service-account token + CA from /var/run/secrets/kubernetes.io/serviceaccount
//   - override URL: e.g. `kubectl proxy` on http://127.0.0.1:8001 for local development
//
// This keeps the binary small and dependency-free for M0. If later features need informers
// or SPDY exec, switching parts to client-go stays an option.
package kube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// ErrNotConfigured means neither in-cluster config nor an override URL is available.
var ErrNotConfigured = errors.New("kubernetes is not configured (not running in a cluster and NECTAR_KUBE_API is not set)")

// StatusError is a non-2xx response from the API server.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string { return fmt.Sprintf("kubernetes: %d %s", e.Code, e.Message) }

// IsNotFound reports whether err is a 404 from the API server.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

// Client talks to the Kubernetes API.
type Client struct {
	base      string
	http      *http.Client
	tokenFile string

	mu        sync.Mutex
	token     string
	tokenRead time.Time
}

// New returns a client. If override is non-empty it is used as the API base URL without auth
// (kubectl proxy). Otherwise the in-cluster configuration is used.
func New(override string) (*Client, error) {
	if override != "" {
		return &Client{base: strings.TrimRight(override, "/"), http: &http.Client{Timeout: 30 * time.Second}}, nil
	}
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, ErrNotConfigured
	}
	caPEM, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("kubernetes: read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("kubernetes: invalid CA certificate")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &Client{
		base:      "https://" + net.JoinHostPort(host, port),
		http:      &http.Client{Transport: tr, Timeout: 30 * time.Second},
		tokenFile: saDir + "/token",
	}, nil
}

// bearer returns the service-account token. Projected tokens rotate, so the file is re-read every minute.
func (c *Client) bearer() (string, error) {
	if c.tokenFile == "" {
		return c.token, nil // static token from a kubeconfig ("" = none)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Since(c.tokenRead) < time.Minute {
		return c.token, nil
	}
	b, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return "", fmt.Errorf("kubernetes: read token: %w", err)
	}
	c.token, c.tokenRead = strings.TrimSpace(string(b)), time.Now()
	return c.token, nil
}

// Do performs a request. body may be nil. out may be nil (response discarded).
func (c *Client) Do(ctx context.Context, method, path, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	tok, err := c.bearer()
	if err != nil {
		return err
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var st struct {
			Message string `json:"message"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if json.Unmarshal(b, &st) != nil || st.Message == "" {
			st.Message = strings.TrimSpace(string(b))
		}
		return &StatusError{Code: resp.StatusCode, Message: st.Message}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Stream performs a GET without the client timeout (pod logs with follow=true) and returns the body.
// The caller closes it; cancelling ctx ends the stream.
func (c *Client) Stream(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	tok, err := c.bearer()
	if err != nil {
		return nil, err
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	hc := *c.http
	hc.Timeout = 0
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		var st struct {
			Message string `json:"message"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if json.Unmarshal(b, &st) != nil || st.Message == "" {
			st.Message = strings.TrimSpace(string(b))
		}
		return nil, &StatusError{Code: resp.StatusCode, Message: st.Message}
	}
	return resp.Body, nil
}

// Get performs a GET and decodes the JSON response into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.Do(ctx, http.MethodGet, path, "", nil, out)
}

// ServerVersion returns the API server's git version (e.g. "v1.34.1+k3s1"). Used as a health check.
func (c *Client) ServerVersion(ctx context.Context) (string, error) {
	var v struct {
		GitVersion string `json:"gitVersion"`
	}
	if err := c.Get(ctx, "/version", &v); err != nil {
		return "", err
	}
	return v.GitVersion, nil
}
