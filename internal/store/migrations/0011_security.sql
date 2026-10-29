-- 0011: two-factor authentication (TOTP + one-time recovery codes) and one-time user tokens
-- (password reset and invitation links sent by email).
ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT '';   -- secretbox-sealed; set before it is enabled
ALTER TABLE users ADD COLUMN mfa_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN recovery_codes TEXT NOT NULL DEFAULT '[]'; -- JSON array of SHA-256 hashes

CREATE TABLE user_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose    TEXT    NOT NULL,          -- reset | invite
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX user_tokens_user ON user_tokens(user_id);
