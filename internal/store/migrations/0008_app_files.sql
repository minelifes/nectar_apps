-- 0008: config files mounted into app containers, and when env vars/files last changed (to tell the UI that
-- a redeploy is needed).
CREATE TABLE app_files (
    app_id      INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    path        TEXT    NOT NULL,           -- absolute path inside the container
    content_enc TEXT    NOT NULL,           -- secretbox-sealed (same box as env vars)
    is_secret   INTEGER NOT NULL DEFAULT 0, -- hidden in the UI unless revealed
    PRIMARY KEY (app_id, path)
);

ALTER TABLE apps ADD COLUMN env_changed_at INTEGER NOT NULL DEFAULT 0;
