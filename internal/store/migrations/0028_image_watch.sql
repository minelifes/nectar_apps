-- 0028: image update watcher — per app, the digest it runs and the digest its tag points to now.
CREATE TABLE image_watch (
    app_id          INTEGER PRIMARY KEY REFERENCES apps(id) ON DELETE CASCADE,
    image           TEXT    NOT NULL,             -- the tag watched (as in the app's settings)
    running_digest  TEXT    NOT NULL DEFAULT '',  -- what the app's pods run
    latest_digest   TEXT    NOT NULL DEFAULT '',  -- what the tag points to in the registry
    status          TEXT    NOT NULL,             -- current | update | error | unknown
    message         TEXT    NOT NULL DEFAULT '',
    seen_at         INTEGER NOT NULL DEFAULT 0,   -- when latest_digest was first seen
    handled_digest  TEXT    NOT NULL DEFAULT '',  -- the newest digest already announced or deployed
    checked_at      INTEGER NOT NULL
);
