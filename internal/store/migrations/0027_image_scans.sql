-- 0027: the last vulnerability scan of each app's image (internal/scan, Trivy).
CREATE TABLE image_scans (
    app_id      INTEGER PRIMARY KEY REFERENCES apps(id) ON DELETE CASCADE,
    image       TEXT    NOT NULL,
    status      TEXT    NOT NULL,             -- running | done | failed
    message     TEXT    NOT NULL DEFAULT '',  -- why it failed
    total       INTEGER NOT NULL DEFAULT 0,
    critical    INTEGER NOT NULL DEFAULT 0,
    high        INTEGER NOT NULL DEFAULT 0,
    medium      INTEGER NOT NULL DEFAULT 0,
    low         INTEGER NOT NULL DEFAULT 0,
    unknown     INTEGER NOT NULL DEFAULT 0,
    fixable     INTEGER NOT NULL DEFAULT 0,
    findings    TEXT    NOT NULL DEFAULT '[]', -- JSON: the worst findings
    started_at  INTEGER NOT NULL,
    finished_at INTEGER NOT NULL DEFAULT 0
);
