-- 0003: domains (host + path → app), synced to nectar-gateway routes.

CREATE TABLE domains (
    id               INTEGER PRIMARY KEY,
    app_id           INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    host             TEXT    NOT NULL COLLATE NOCASE,
    path_prefix      TEXT    NOT NULL DEFAULT '/',
    target_port      INTEGER NOT NULL DEFAULT 0,  -- 0 = the app's port
    tls              INTEGER NOT NULL DEFAULT 1,  -- obtain a Let's Encrypt certificate
    force_https      INTEGER NOT NULL DEFAULT 1,  -- redirect HTTP → HTTPS once a certificate exists
    is_default       INTEGER NOT NULL DEFAULT 0,  -- auto-created <app>-<project>.<base domain>
    status           TEXT    NOT NULL DEFAULT 'pending', -- pending | active | waiting | error
    status_message   TEXT    NOT NULL DEFAULT '',
    gateway_route_id INTEGER,
    cert_not_after   INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    UNIQUE (host, path_prefix)
);
CREATE INDEX domains_app ON domains(app_id);
