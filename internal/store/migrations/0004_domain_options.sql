-- 0004: per-domain gateway options: redirects (www ↔ apex), gRPC (h2c), timeouts, body limit, basic auth.

ALTER TABLE domains ADD COLUMN kind TEXT NOT NULL DEFAULT 'app';          -- app | redirect
ALTER TABLE domains ADD COLUMN redirect_to TEXT NOT NULL DEFAULT '';      -- redirect: target URL, e.g. https://example.com
ALTER TABLE domains ADD COLUMN redirect_code INTEGER NOT NULL DEFAULT 301;
ALTER TABLE domains ADD COLUMN backend_protocol TEXT NOT NULL DEFAULT 'http1'; -- http1 | h2c (gRPC)
ALTER TABLE domains ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 0;     -- 0 = gateway default (60s)
ALTER TABLE domains ADD COLUMN max_body_mb INTEGER NOT NULL DEFAULT 0;         -- 0 = unlimited
ALTER TABLE domains ADD COLUMN basic_auth TEXT NOT NULL DEFAULT '';            -- htpasswd lines user:<bcrypt>
