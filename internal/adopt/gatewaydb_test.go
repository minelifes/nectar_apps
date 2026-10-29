package adopt

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteDriver = "sqlite"

func TestGatewayDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nectar.db")
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE routes (id INTEGER PRIMARY KEY, domain TEXT, path_prefix TEXT, mode TEXT, target_ip TEXT, target_port INTEGER,
			target_https INTEGER, rate_limit INTEGER, hc_enabled INTEGER, hc_path TEXT, hc_interval INTEGER, static_dir TEXT,
			cert_file TEXT, key_file TEXT, force_ssl INTEGER, cert_pem TEXT, key_pem TEXT, acme INTEGER, basic_auth TEXT, managed_by TEXT)`,
		`INSERT INTO routes VALUES (1, 'api.example.com', '/', 'proxy', '127.0.0.1', 3000, 0, 20, 1, '/health', 15, '', '', '', 1, 'CERT', 'KEY', 0, 'u:$2y$x', '')`,
		`INSERT INTO routes VALUES (2, 'www.example.com', '/', 'static', '', 0, 0, 0, 0, '/', 30, '/var/www', '/etc/le/fullchain.pem', '/etc/le/privkey.pem', 0, '', '', 0, '', '')`,
		`CREATE TABLE blocked_ips (ip TEXT PRIMARY KEY, reason TEXT, blocked_at INTEGER, expires_at INTEGER)`,
		`INSERT INTO blocked_ips VALUES ('1.2.3.4', 'scanner', 0, 9999999999), ('5.6.7.8', 'old', 0, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	routes, blocked, err := GatewayDB(context.Background(), sqliteDriver, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || len(blocked) != 1 || blocked[0].IP != "1.2.3.4" || blocked[0].ExpiresAt.Before(time.Now()) {
		t.Fatalf("routes %d blocked %+v", len(routes), blocked)
	}
	api := routes[0]
	if !api.Migratable() || api.Target.Port != 3000 || !api.ForceHTTPS || api.Cert.CertPEM != "CERT" || api.Htpasswd != "u:$2y$x" ||
		api.Extra["rate_limit"] != 20 || api.Extra["hc_path"] != "/health" {
		t.Fatalf("api: %+v", api)
	}
	if routes[1].Status != StatusFallback || routes[1].Cert.CertFile != "/etc/le/fullchain.pem" {
		t.Fatalf("static: %+v", routes[1])
	}
	p := api.GatewayPayload("", ImportedOwner)
	if p["rate_limit"] != 20 || p["force_ssl"] != true || p["owner"] != ImportedOwner {
		t.Fatalf("payload: %v", p)
	}
}
