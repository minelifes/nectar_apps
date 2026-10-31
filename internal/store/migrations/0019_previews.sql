-- 0019: preview environments per pull request. A preview is an ordinary app of the same project (a copy of
-- the parent app) built from the pull request's head and deleted when the pull request closes.
ALTER TABLE app_sources ADD COLUMN previews INTEGER NOT NULL DEFAULT 0;          -- pull requests get a preview app
ALTER TABLE app_sources ADD COLUMN preview_databases INTEGER NOT NULL DEFAULT 0; -- previews keep the parent's database links
ALTER TABLE app_sources ADD COLUMN preview_forks INTEGER NOT NULL DEFAULT 0;     -- also pull requests from forks
ALTER TABLE app_sources ADD COLUMN preview_ttl_days INTEGER NOT NULL DEFAULT 14; -- delete previews without activity after N days (0 = never)
CREATE TABLE previews (
    app_id      INTEGER PRIMARY KEY REFERENCES apps(id) ON DELETE CASCADE, -- the preview app
    parent_id   INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    number      INTEGER NOT NULL,           -- pull / merge request number
    title       TEXT    NOT NULL DEFAULT '',
    url         TEXT    NOT NULL DEFAULT '', -- the pull request page
    author      TEXT    NOT NULL DEFAULT '',
    head_ref    TEXT    NOT NULL,           -- refs/pull/12/head, refs/merge-requests/12/head
    commit_sha  TEXT    NOT NULL DEFAULT '',
    provider    TEXT    NOT NULL DEFAULT '', -- github | gitlab | gitea
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (parent_id, number)
);
