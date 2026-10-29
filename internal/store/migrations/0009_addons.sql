-- 0009: database add-ons (PostgreSQL, MySQL, MariaDB, Redis) and their links to apps.
CREATE TABLE addons (
    id              INTEGER PRIMARY KEY,
    project_id      INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    slug            TEXT    NOT NULL,           -- also the Service name: apps reach it at <slug>:<port>
    name            TEXT    NOT NULL,
    kind            TEXT    NOT NULL,           -- postgres | mysql | mariadb | redis
    version         TEXT    NOT NULL,
    size_mb         INTEGER NOT NULL,
    cpu_millicores  INTEGER NOT NULL,
    memory_mb       INTEGER NOT NULL,
    stopped         INTEGER NOT NULL DEFAULT 0,
    credentials     TEXT    NOT NULL,           -- secretbox-sealed JSON (user, password, database)
    status          TEXT    NOT NULL DEFAULT 'new',
    status_message  TEXT    NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (project_id, slug)
);

-- An app linked to an add-on gets its connection settings as env vars (PREFIX + DATABASE_URL, …).
CREATE TABLE addon_links (
    addon_id   INTEGER NOT NULL REFERENCES addons(id) ON DELETE CASCADE,
    app_id     INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    prefix     TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (addon_id, app_id)
);
CREATE INDEX addon_links_app ON addon_links(app_id);
