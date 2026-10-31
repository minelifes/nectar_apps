-- 0020: passkeys (WebAuthn credentials) — sign in without a password, also counts as the second factor.
CREATE TABLE passkeys (
    id              INTEGER PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id   BLOB    NOT NULL UNIQUE,
    public_key      BLOB    NOT NULL,        -- COSE_Key
    sign_count      INTEGER NOT NULL DEFAULT 0,
    aaguid          BLOB,
    rp_id           TEXT    NOT NULL,        -- the panel host it was created for
    name            TEXT    NOT NULL DEFAULT '',
    backup_eligible INTEGER NOT NULL DEFAULT 0, -- synced passkey (iCloud Keychain, Google Password Manager, …)
    transports      TEXT    NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER
);
CREATE INDEX passkeys_user ON passkeys(user_id);
