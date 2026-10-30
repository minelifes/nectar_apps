-- 0016: per-domain web options (redirect rules, IP allowlist, headers, CORS, gzip, HSTS — JSON, sent to
-- nectar-gateway as the route's "web") and the state of the automatic DNS record.
ALTER TABLE domains ADD COLUMN web TEXT NOT NULL DEFAULT '';
-- dns_state: '' (not managed) | managed (the gateway created the A/AAAA records) | conflict | error
ALTER TABLE domains ADD COLUMN dns_state TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN dns_message TEXT NOT NULL DEFAULT '';
