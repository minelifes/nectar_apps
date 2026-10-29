-- 0007: alerts (PLAN.md §7 "Alerts"). Rules are a fixed catalog configured in settings ("alert_rules");
-- channels carry their sealed configuration (tokens, SMTP passwords).
CREATE TABLE alert_channels (
    id         INTEGER PRIMARY KEY,
    kind       TEXT    NOT NULL,           -- ntfy | telegram | email | webhook
    name       TEXT    NOT NULL,
    config     TEXT    NOT NULL,           -- secretbox-sealed JSON
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- One row per incident: firing until the condition clears.
CREATE TABLE alerts (
    id          INTEGER PRIMARY KEY,
    rule        TEXT    NOT NULL,          -- rule kind, e.g. app_down
    key         TEXT    NOT NULL,          -- rule + subject, e.g. app_down:12
    subject     TEXT    NOT NULL,          -- human name of what is affected
    severity    TEXT    NOT NULL,          -- warning | critical
    message     TEXT    NOT NULL,
    app_id      INTEGER REFERENCES apps(id) ON DELETE SET NULL,
    state       TEXT    NOT NULL,          -- firing | resolved
    started_at  INTEGER NOT NULL,
    resolved_at INTEGER,
    notified    INTEGER NOT NULL DEFAULT 0 -- channels reached when it fired
);
CREATE UNIQUE INDEX alerts_open ON alerts(key) WHERE state = 'firing';
CREATE INDEX alerts_started ON alerts(started_at);
