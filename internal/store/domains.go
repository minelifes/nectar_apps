package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Domain routes a host (+ path prefix) to an app through the gateway.
type Domain struct {
	ID             int64      `json:"id"`
	AppID          int64      `json:"app_id"`
	Host           string     `json:"host"`
	PathPrefix     string     `json:"path_prefix"`
	TargetPort     int        `json:"target_port"` // 0 = the app's port
	TLS            bool       `json:"tls"`
	ForceHTTPS     bool       `json:"force_https"`
	IsDefault      bool       `json:"is_default"`
	Status         string     `json:"status"` // pending | active | waiting | error
	StatusMessage  string     `json:"status_message,omitempty"`
	GatewayRouteID *int64     `json:"gateway_route_id,omitempty"`
	CertNotAfter   *time.Time `json:"cert_not_after,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	DomainOptions
}

// Domain kinds.
const (
	DomainApp      = "app"      // proxied to the app
	DomainRedirect = "redirect" // answers with a redirect (e.g. www.example.com → https://example.com)
)

// DomainOptions are the per-domain gateway options.
type DomainOptions struct {
	Kind            string `json:"kind"`                  // app | redirect
	RedirectTo      string `json:"redirect_to,omitempty"` // redirect target URL
	RedirectCode    int    `json:"redirect_code,omitempty"`
	BackendProtocol string `json:"backend_protocol"` // http1 | h2c (gRPC)
	TimeoutSeconds  int    `json:"timeout_seconds"`
	MaxBodyMB       int    `json:"max_body_mb"`
	RateLimit       int    `json:"rate_limit"` // requests/s per client IP, 0 = unlimited
	// BasicAuth is htpasswd text (user:<bcrypt>); never sent to clients — see AuthUsers.
	BasicAuth string   `json:"-"`
	AuthUsers []string `json:"auth_users"`
}

const domainCols = `id, app_id, host, path_prefix, target_port, tls, force_https, is_default, status, status_message,
	gateway_route_id, cert_not_after, created_at, kind, redirect_to, redirect_code, backend_protocol, timeout_seconds,
	max_body_mb, basic_auth, rate_limit`

func scanDomain(r scanner) (Domain, error) {
	var d Domain
	var gw sql.NullInt64
	var notAfter, created int64
	err := r.Scan(&d.ID, &d.AppID, &d.Host, &d.PathPrefix, &d.TargetPort, &d.TLS, &d.ForceHTTPS, &d.IsDefault,
		&d.Status, &d.StatusMessage, &gw, &notAfter, &created, &d.Kind, &d.RedirectTo, &d.RedirectCode,
		&d.BackendProtocol, &d.TimeoutSeconds, &d.MaxBodyMB, &d.BasicAuth, &d.RateLimit)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	d.AuthUsers = HtpasswdUsers(d.BasicAuth)
	d.GatewayRouteID, d.CreatedAt = intPtr(gw), ts(created)
	if notAfter > 0 {
		t := ts(notAfter)
		d.CertNotAfter = &t
	}
	return d, err
}

// NormalizeHost lower-cases a host and strips a trailing dot.
func NormalizeHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
}

// NormalizePath returns "/" or "/segment[/…]" without a trailing slash.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimSuffix(p, "/")
}

// CreateDomain adds a domain. Returns ErrConflict if host+path is already used.
func (s *Store) CreateDomain(ctx context.Context, d Domain) (Domain, error) {
	now := s.unix()
	o := d.DomainOptions.withDefaults()
	res, err := s.db.ExecContext(ctx, `INSERT INTO domains (app_id, host, path_prefix, target_port, tls, force_https, is_default, status,
		created_at, updated_at, kind, redirect_to, redirect_code, backend_protocol, timeout_seconds, max_body_mb, basic_auth, rate_limit)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.AppID, NormalizeHost(d.Host), NormalizePath(d.PathPrefix), d.TargetPort, b2i(d.TLS), b2i(d.ForceHTTPS), b2i(d.IsDefault), now, now,
		o.Kind, o.RedirectTo, o.RedirectCode, o.BackendProtocol, o.TimeoutSeconds, o.MaxBodyMB, o.BasicAuth, o.RateLimit)
	if isUnique(err) {
		return Domain{}, ErrConflict
	}
	if err != nil {
		return Domain{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetDomain(ctx, id)
}

// GetDomain returns a domain by id.
func (s *Store) GetDomain(ctx context.Context, id int64) (Domain, error) {
	return scanDomain(s.db.QueryRowContext(ctx, `SELECT `+domainCols+` FROM domains WHERE id = ?`, id))
}

// ListDomains returns the domains of an app (appID 0 = all domains).
func (s *Store) ListDomains(ctx context.Context, appID int64) ([]Domain, error) {
	q, args := `SELECT `+domainCols+` FROM domains ORDER BY host, path_prefix`, []any{}
	if appID != 0 {
		q, args = `SELECT `+domainCols+` FROM domains WHERE app_id = ? ORDER BY is_default DESC, host, path_prefix`, []any{appID}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDomainSettings changes the user-editable fields.
func (s *Store) UpdateDomainSettings(ctx context.Context, id int64, targetPort int, tls, forceHTTPS bool) (Domain, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE domains SET target_port = ?, tls = ?, force_https = ?, updated_at = ? WHERE id = ?`,
		targetPort, b2i(tls), b2i(forceHTTPS), s.unix(), id)
	if err != nil {
		return Domain{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Domain{}, ErrNotFound
	}
	return s.GetDomain(ctx, id)
}

// UpdateDomainOptions replaces the gateway options of a domain.
func (s *Store) UpdateDomainOptions(ctx context.Context, id int64, o DomainOptions) (Domain, error) {
	o = o.withDefaults()
	res, err := s.db.ExecContext(ctx, `UPDATE domains SET kind = ?, redirect_to = ?, redirect_code = ?, backend_protocol = ?,
		timeout_seconds = ?, max_body_mb = ?, basic_auth = ?, rate_limit = ?, updated_at = ? WHERE id = ?`,
		o.Kind, o.RedirectTo, o.RedirectCode, o.BackendProtocol, o.TimeoutSeconds, o.MaxBodyMB, o.BasicAuth, o.RateLimit, s.unix(), id)
	if err != nil {
		return Domain{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Domain{}, ErrNotFound
	}
	return s.GetDomain(ctx, id)
}

func (o DomainOptions) withDefaults() DomainOptions {
	if o.Kind == "" {
		o.Kind = DomainApp
	}
	if o.RedirectCode == 0 {
		o.RedirectCode = 301
	}
	if o.BackendProtocol == "" {
		o.BackendProtocol = "http1"
	}
	return o
}

// HtpasswdUsers lists the user names in htpasswd text.
func HtpasswdUsers(htpasswd string) []string {
	out := []string{}
	for _, line := range strings.Split(htpasswd, "\n") {
		if u, _, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && u != "" {
			out = append(out, u)
		}
	}
	return out
}

// HtpasswdHash returns the hash stored for user.
func HtpasswdHash(htpasswd, user string) (string, bool) {
	for _, line := range strings.Split(htpasswd, "\n") {
		if u, h, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && u == user {
			return h, true
		}
	}
	return "", false
}

// SetDomainState records the sync result for a domain.
func (s *Store) SetDomainState(ctx context.Context, id int64, status, message string, routeID *int64, certNotAfter int64) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.ExecContext(ctx, `UPDATE domains SET status = ?, status_message = ?, gateway_route_id = ?, cert_not_after = ? WHERE id = ?`,
		status, message, nullInt(routeID), certNotAfter, id)
	return err
}

// DeleteDomain removes a domain.
func (s *Store) DeleteDomain(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM domains WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
