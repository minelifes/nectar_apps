-- 0014: Git builds and registry logins.
-- registry_credentials: logins for private registries (pull secrets in every project, base images in builds).
CREATE TABLE registry_credentials (
    id         INTEGER PRIMARY KEY,
    host       TEXT    NOT NULL UNIQUE,     -- ghcr.io, docker.io, registry.gitlab.com, my.registry:5000
    username   TEXT    NOT NULL,
    password   TEXT    NOT NULL,            -- secretbox-sealed
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- app_sources: where an image app is built from (one Git repository per app).
CREATE TABLE app_sources (
    app_id         INTEGER PRIMARY KEY REFERENCES apps(id) ON DELETE CASCADE,
    repo_url       TEXT    NOT NULL,        -- https://github.com/acme/api.git
    branch         TEXT    NOT NULL DEFAULT 'main',
    context_dir    TEXT    NOT NULL DEFAULT '',  -- sub folder with the Dockerfile ("" = repository root)
    dockerfile     TEXT    NOT NULL DEFAULT 'Dockerfile',
    token          TEXT    NOT NULL DEFAULT '', -- sealed access token for private repositories ("" = public)
    webhook_secret TEXT    NOT NULL,        -- sealed; push webhooks are verified with it
    auto_deploy    INTEGER NOT NULL DEFAULT 1,  -- a push to the branch builds and deploys
    build_cpu      INTEGER NOT NULL DEFAULT 2000, -- millicores
    build_memory   INTEGER NOT NULL DEFAULT 2048, -- MiB
    updated_at     INTEGER NOT NULL
);

CREATE TABLE builds (
    id          INTEGER PRIMARY KEY,
    app_id      INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    number      INTEGER NOT NULL,
    trigger     TEXT    NOT NULL,           -- manual | webhook | api
    branch      TEXT    NOT NULL,
    commit_sha  TEXT    NOT NULL DEFAULT '',
    commit_msg  TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL,           -- running | succeeded | failed | canceled
    image       TEXT    NOT NULL DEFAULT '',
    deploy      INTEGER NOT NULL DEFAULT 1, -- deploy the image when the build succeeds
    release     INTEGER,                    -- release number started from the build
    message     TEXT    NOT NULL DEFAULT '',
    log         TEXT    NOT NULL DEFAULT '', -- last 256 KB of the build output
    created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    UNIQUE (app_id, number)
);
CREATE INDEX builds_app ON builds (app_id, number DESC);
