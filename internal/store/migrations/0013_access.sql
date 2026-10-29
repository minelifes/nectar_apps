-- 0013: per-domain rate limit (requests per second per client IP, enforced by nectar-gateway; 0 = unlimited).
ALTER TABLE domains ADD COLUMN rate_limit INTEGER NOT NULL DEFAULT 0;
