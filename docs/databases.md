# Databases

A project can run **PostgreSQL, MySQL, MariaDB or Redis** next to its apps (project page → Databases). Each
database gets its own volume and generated credentials and is only reachable from inside the project.

**Link** a database to an app and the app gets `DATABASE_URL` (or `REDIS_URL`) plus the usual `PG*` /
`MYSQL_*` variables with its next deploy. A prefix such as `CACHE_` lets one app use several databases
(`CACHE_REDIS_URL`). Without linking, reference a single value anywhere: `${db.DATABASE_URL}`.

**Share with the whole project** (database page → Overview) instead of linking app by app: every app in the
project gets the variables, including apps you add later. Use a prefix when two shared databases would set
the same variable (two PostgreSQL databases both set `DATABASE_URL`). If an app also has its own link to the
database, the link's prefix wins. Like a link, sharing takes effect when each app is next deployed.

## Managing a database like an app

The database page has the same tabs as an app:

- **Metrics**: CPU and memory over time, with the requested amount as a dashed line.
- **Logs**: the live log of the database pod, with download.
- **Terminal**: a shell in the database container, where `psql`, `mysql` or `redis-cli` are already installed.
- **Events**: Kubernetes events, such as a failed volume mount or an out-of-memory kill.

Change CPU, memory and storage on the Overview tab. Storage can only grow. **Restart** restarts the pod;
**Stop** frees its CPU and memory and keeps the data.

**From your computer:** `nectar db connect project/db` opens the database on `127.0.0.1` through the panel,
for any client (`psql`, `mysql`, TablePlus, DBeaver…). See [CLI](cli.md#connecting-to-a-database-from-your-computer).

Databases can be backed up (a dump on a schedule) and restored from the panel — see [Backups](backups.md).
