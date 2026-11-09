-- 0025: Helm charts installed into a project (internal/helm). Values and the repository password are
-- sealed with the env box (rekey: columns).
CREATE TABLE helm_releases (
    id             INTEGER PRIMARY KEY,
    project_id     INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT    NOT NULL,
    repo           TEXT    NOT NULL,
    chart          TEXT    NOT NULL,
    version        TEXT    NOT NULL DEFAULT '',  -- requested ("" = latest)
    values_enc     TEXT    NOT NULL DEFAULT '',
    repo_username  TEXT    NOT NULL DEFAULT '',
    repo_password  TEXT    NOT NULL DEFAULT '',  -- sealed
    status         TEXT    NOT NULL DEFAULT 'pending', -- installing | upgrading | rolling_back | uninstalling | deployed | failed
    message        TEXT    NOT NULL DEFAULT '',
    output         TEXT    NOT NULL DEFAULT '',  -- helm's output of the last operation
    created_by     INTEGER,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (project_id, name)
);
