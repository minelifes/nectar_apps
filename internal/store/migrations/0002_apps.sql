-- 0002: apps, environment variables, artifacts, resumable uploads, releases.

CREATE TABLE apps (
    id              INTEGER PRIMARY KEY,
    project_id      INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    slug            TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    kind            TEXT    NOT NULL CHECK (kind IN ('web', 'worker', 'cron', 'task')),
    runtime_id      TEXT    NOT NULL,
    runtime_version TEXT    NOT NULL DEFAULT '',
    config          TEXT    NOT NULL DEFAULT '{}', -- JSON AppConfig
    status          TEXT    NOT NULL DEFAULT 'new',
    status_message  TEXT    NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (project_id, slug)
);

-- All values are encrypted with the master key (secretbox "env"); is_secret only controls display.
CREATE TABLE env_vars (
    id        INTEGER PRIMARY KEY,
    app_id    INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key       TEXT    NOT NULL,
    value_enc TEXT    NOT NULL,
    is_secret INTEGER NOT NULL DEFAULT 0,
    UNIQUE (app_id, key)
);

-- Uploaded files, stored content-addressed under <data>/artifacts/<sha256>.
CREATE TABLE artifacts (
    id         INTEGER PRIMARY KEY,
    app_id     INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    sha256     TEXT    NOT NULL,
    filename   TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    detect     TEXT    NOT NULL DEFAULT '{}', -- JSON detect.Result
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX artifacts_app ON artifacts(app_id);
CREATE INDEX artifacts_sha ON artifacts(sha256);

-- In-progress resumable uploads (tus protocol).
CREATE TABLE uploads (
    id         TEXT    PRIMARY KEY,
    app_id     INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    filename   TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    offset     INTEGER NOT NULL DEFAULT 0,
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE TABLE releases (
    id          INTEGER PRIMARY KEY,
    app_id      INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    number      INTEGER NOT NULL,
    artifact_id INTEGER REFERENCES artifacts(id) ON DELETE SET NULL,
    image       TEXT    NOT NULL,
    config      TEXT    NOT NULL DEFAULT '{}', -- snapshot of the app config at deploy time
    status      TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'deploying', 'live', 'failed', 'superseded')),
    message     TEXT    NOT NULL DEFAULT '',
    created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  INTEGER NOT NULL,
    finished_at INTEGER,
    UNIQUE (app_id, number)
);
