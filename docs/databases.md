# Databases

A project can run **PostgreSQL, MySQL, MariaDB or Redis** next to its apps (project page → Databases). Each
database gets its own volume and generated credentials and is only reachable from inside the project.

**Link** a database to an app and the app gets `DATABASE_URL` (or `REDIS_URL`) plus the usual `PG*` /
`MYSQL_*` variables with its next deploy. A prefix such as `CACHE_` lets one app use several databases
(`CACHE_REDIS_URL`). Without linking, reference a single value anywhere: `${db.DATABASE_URL}`.

Databases can be backed up (a dump on a schedule) and restored from the panel — see [Backups](backups.md).
