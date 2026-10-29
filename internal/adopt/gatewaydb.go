package adopt

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// BlockedIP is a blocklist entry of a standalone nectar_gateway.
type BlockedIP struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GatewayDB reads the routes and blocked IPs of a standalone nectar_gateway database (schema v1–v3)
// read-only. driver is the database/sql driver name of the SQLite driver linked into the binary.
func GatewayDB(ctx context.Context, driver, path string) ([]Route, []BlockedIP, error) {
	db, err := sql.Open(driver, "file:"+path+"?mode=ro")
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT * FROM routes ORDER BY id`)
	if err != nil {
		return nil, nil, fmt.Errorf("read routes: %w", err)
	}
	cols, _ := rows.Columns()
	var routes []Route
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			rows.Close()
			return nil, nil, err
		}
		m := map[string]any{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		routes = append(routes, gatewayRoute(m))
	}
	rows.Close()
	var blocked []BlockedIP
	if brows, err := db.QueryContext(ctx, `SELECT ip, reason, expires_at FROM blocked_ips WHERE expires_at > ?`, time.Now().Unix()); err == nil {
		for brows.Next() {
			var b BlockedIP
			var exp int64
			if brows.Scan(&b.IP, &b.Reason, &exp) == nil {
				b.ExpiresAt = time.Unix(exp, 0).UTC()
				blocked = append(blocked, b)
			}
		}
		brows.Close()
	}
	return routes, blocked, nil
}

func colStr(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func colInt(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case int64:
		return int(v)
	case int:
		return v
	case float64:
		return int(v)
	case string:
		var n int
		fmt.Sscan(v, &n)
		return n
	}
	return 0
}

func gatewayRoute(m map[string]any) Route {
	r := Route{
		Source:     SourceNectarGateway,
		Ref:        fmt.Sprintf("route #%d", colInt(m, "id")),
		Host:       colStr(m, "domain"),
		PathPrefix: colStr(m, "path_prefix"),
		Status:     StatusOK,
		Target: Target{
			Host:  colStr(m, "target_ip"),
			Port:  colInt(m, "target_port"),
			HTTPS: colInt(m, "target_https") == 1,
		},
		ForceHTTPS:      colInt(m, "force_ssl") == 1,
		BackendProtocol: colStr(m, "backend_protocol"),
		TimeoutSeconds:  colInt(m, "timeout_seconds"),
		MaxBodyMB:       colInt(m, "max_body_mb"),
		RedirectURL:     colStr(m, "redirect_url"),
		RedirectCode:    colInt(m, "redirect_code"),
		Htpasswd:        colStr(m, "basic_auth"),
		Extra: map[string]any{
			"rate_limit":  colInt(m, "rate_limit"),
			"hc_enabled":  colInt(m, "hc_enabled") == 1,
			"hc_path":     colStr(m, "hc_path"),
			"hc_interval": colInt(m, "hc_interval"),
			"hc_type":     colStr(m, "hc_type"),
			"hc_expect":   colStr(m, "hc_expect"),
			"maintenance": colInt(m, "maintenance") == 1,
		},
	}
	if r.Htpasswd != "" {
		r.BasicAuth = "gateway route"
	}
	switch colStr(m, "mode") {
	case "", "proxy":
	case "redirect":
	case "static":
		r.fallback("static mode serves %s from the host disk", colStr(m, "static_dir"))
	default:
		r.fallback("mode %s", colStr(m, "mode"))
	}
	if mb := colStr(m, "managed_by"); mb != "" {
		r.warn("was managed by %s on the standalone gateway; imported as a normal route", mb)
	}
	switch {
	case colStr(m, "cert_pem") != "" && colStr(m, "key_pem") != "":
		r.TLS = true
		r.Cert = &CertRef{CertPEM: colStr(m, "cert_pem"), KeyPEM: colStr(m, "key_pem")}
	case colStr(m, "cert_file") != "" && colStr(m, "key_file") != "":
		r.TLS = true
		r.Cert = &CertRef{CertFile: colStr(m, "cert_file"), KeyFile: colStr(m, "key_file")}
	case colInt(m, "acme") == 1:
		r.TLS = true
	}
	return r
}
