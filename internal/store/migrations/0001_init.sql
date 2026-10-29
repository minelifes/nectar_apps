-- 0001: users, sessions, roles, bindings, API tokens, projects, audit log, settings.
-- Timestamps are Unix seconds (UTC).

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    email         TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    name          TEXT    NOT NULL,
    password_hash TEXT    NOT NULL,
    is_owner      INTEGER NOT NULL DEFAULT 0,
    disabled      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

-- One row per login. Web: token = session cookie. Mobile: token = short-lived access token,
-- refresh = long-lived refresh token (both rotated/slid on refresh). Only SHA-256 hashes are stored.
CREATE TABLE sessions (
    id                 INTEGER PRIMARY KEY,
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind               TEXT    NOT NULL CHECK (kind IN ('web', 'mobile')),
    token_hash         TEXT    NOT NULL UNIQUE,
    token_expires_at   INTEGER NOT NULL,
    refresh_hash       TEXT    UNIQUE,
    refresh_expires_at INTEGER,
    user_agent         TEXT    NOT NULL DEFAULT '',
    ip                 TEXT    NOT NULL DEFAULT '',
    created_at         INTEGER NOT NULL,
    last_seen_at       INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE projects (
    id         INTEGER PRIMARY KEY,
    slug       TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    namespace  TEXT    NOT NULL UNIQUE,
    quota_json TEXT,                       -- NULL = unlimited
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE roles (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    description TEXT    NOT NULL DEFAULT '',
    builtin_key TEXT    UNIQUE,            -- non-NULL for built-in roles (read-only)
    permissions TEXT    NOT NULL DEFAULT '[]', -- JSON array of permission keys
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE role_bindings (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id    INTEGER NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE, -- NULL = global
    created_at INTEGER NOT NULL
);
-- SQLite treats NULLs as distinct in UNIQUE constraints, so use an expression index.
CREATE UNIQUE INDEX role_bindings_unique ON role_bindings(user_id, role_id, IFNULL(project_id, 0));

CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT    NOT NULL,
    token_hash   TEXT    NOT NULL UNIQUE,
    role_id      INTEGER REFERENCES roles(id) ON DELETE SET NULL,      -- NULL = same rights as the user
    project_id   INTEGER REFERENCES projects(id) ON DELETE CASCADE,    -- NULL = all projects
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER,
    expires_at   INTEGER
);

CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
    user_email TEXT    NOT NULL DEFAULT '',
    action     TEXT    NOT NULL,
    target     TEXT    NOT NULL DEFAULT '',
    payload    TEXT    NOT NULL DEFAULT '{}',
    ip         TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE INDEX audit_log_created ON audit_log(created_at);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT    NOT NULL,           -- JSON
    updated_at INTEGER NOT NULL
);
