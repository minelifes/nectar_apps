-- 0012: panel / gateway / k3s updates: every run of the updater Job (plan + live status from its ConfigMap).
CREATE TABLE update_runs (
    id          INTEGER PRIMARY KEY,
    trigger     TEXT    NOT NULL,          -- manual | auto | rollback
    state       TEXT    NOT NULL,          -- pending | running | succeeded | failed | rolled_back | rollback_failed
    summary     TEXT    NOT NULL DEFAULT '', -- "panel 0.6.0 → 0.7.0, gateway 1.2.1 → 1.3.0"
    plan        TEXT    NOT NULL,          -- JSON update.Plan
    status      TEXT    NOT NULL DEFAULT '{}', -- JSON update.RunStatus
    error       TEXT    NOT NULL DEFAULT '',
    user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER
);
CREATE INDEX update_runs_started ON update_runs (started_at DESC);
