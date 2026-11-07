-- 0024: domains without an app ("proxies"): host + path → any address:port the gateway can reach
-- (a port on the server, another machine, a Service Nectar does not manage). app_id becomes optional
-- (SQLite cannot drop NOT NULL, so the table is rebuilt; nothing references domains).

CREATE TABLE domains_new (
    id               INTEGER PRIMARY KEY,
    app_id           INTEGER REFERENCES apps(id) ON DELETE CASCADE, -- NULL = proxy domain
    host             TEXT    NOT NULL COLLATE NOCASE,
    path_prefix      TEXT    NOT NULL DEFAULT '/',
    target_port      INTEGER NOT NULL DEFAULT 0,  -- app domains: 0 = the app's port; proxies: the port
    tls              INTEGER NOT NULL DEFAULT 1,
    force_https      INTEGER NOT NULL DEFAULT 1,
    is_default       INTEGER NOT NULL DEFAULT 0,
    status           TEXT    NOT NULL DEFAULT 'pending',
    status_message   TEXT    NOT NULL DEFAULT '',
    gateway_route_id INTEGER,
    cert_not_after   INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    kind             TEXT    NOT NULL DEFAULT 'app',  -- app | redirect | proxy
    redirect_to      TEXT    NOT NULL DEFAULT '',
    redirect_code    INTEGER NOT NULL DEFAULT 301,
    backend_protocol TEXT    NOT NULL DEFAULT 'http1',
    timeout_seconds  INTEGER NOT NULL DEFAULT 0,
    max_body_mb      INTEGER NOT NULL DEFAULT 0,
    basic_auth       TEXT    NOT NULL DEFAULT '',
    rate_limit       INTEGER NOT NULL DEFAULT 0,
    web              TEXT    NOT NULL DEFAULT '',
    dns_state        TEXT    NOT NULL DEFAULT '',
    dns_message      TEXT    NOT NULL DEFAULT '',
    target_host      TEXT    NOT NULL DEFAULT '', -- proxies: IP or DNS name, e.g. 127.0.0.1 or 10.0.0.5
    target_https     INTEGER NOT NULL DEFAULT 0,  -- proxies: connect to the target over HTTPS
    UNIQUE (host, path_prefix)
);

INSERT INTO domains_new (id, app_id, host, path_prefix, target_port, tls, force_https, is_default, status, status_message,
    gateway_route_id, cert_not_after, created_at, updated_at, kind, redirect_to, redirect_code, backend_protocol,
    timeout_seconds, max_body_mb, basic_auth, rate_limit, web, dns_state, dns_message)
SELECT id, app_id, host, path_prefix, target_port, tls, force_https, is_default, status, status_message,
    gateway_route_id, cert_not_after, created_at, updated_at, kind, redirect_to, redirect_code, backend_protocol,
    timeout_seconds, max_body_mb, basic_auth, rate_limit, web, dns_state, dns_message
FROM domains;

DROP TABLE domains;
ALTER TABLE domains_new RENAME TO domains;
CREATE INDEX domains_app ON domains(app_id);
