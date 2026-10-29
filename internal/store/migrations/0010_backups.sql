-- 0010: backups with restic: where to (targets), what and when (plans), and every run.
CREATE TABLE backup_targets (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    kind       TEXT    NOT NULL,           -- s3 | sftp | local
    config     TEXT    NOT NULL,           -- secretbox-sealed JSON (keys, repository password)
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE backup_plans (
    id            INTEGER PRIMARY KEY,
    target_id     INTEGER NOT NULL REFERENCES backup_targets(id) ON DELETE RESTRICT,
    scope         TEXT    NOT NULL,        -- app (its volumes) | addon (a database dump) | panel (panel data)
    subject_id    INTEGER NOT NULL DEFAULT 0,
    schedule      TEXT    NOT NULL,        -- cron, UTC; "" = manual only
    keep_daily    INTEGER NOT NULL DEFAULT 7,
    keep_weekly   INTEGER NOT NULL DEFAULT 4,
    keep_monthly  INTEGER NOT NULL DEFAULT 6,
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    UNIQUE (target_id, scope, subject_id)
);

CREATE TABLE backup_runs (
    id          INTEGER PRIMARY KEY,
    plan_id     INTEGER REFERENCES backup_plans(id) ON DELETE SET NULL,
    kind        TEXT    NOT NULL,          -- backup | restore
    scope       TEXT    NOT NULL,
    subject_id  INTEGER NOT NULL DEFAULT 0,
    subject     TEXT    NOT NULL,          -- human name, kept after the subject is deleted
    namespace   TEXT    NOT NULL,
    job         TEXT    NOT NULL,
    snapshot    TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL,          -- running | succeeded | failed
    message     TEXT    NOT NULL DEFAULT '',
    bytes_added INTEGER NOT NULL DEFAULT 0,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER
);
CREATE INDEX backup_runs_plan ON backup_runs(plan_id, id);
CREATE INDEX backup_runs_status ON backup_runs(status);
