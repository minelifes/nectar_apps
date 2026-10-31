# First deploy

1. **Create a project** (Projects → New). A project is a Kubernetes namespace with its own quota, members,
   databases and env groups.
2. **New app** → pick a type ([runtimes](runtimes.md)): *Java (JAR)*, *Native binary*, *Static website*,
   *Node.js*, *Python*, *PHP* or *Docker image*; choose a version and the kind:
    - **web** — answers HTTP on a port and gets a domain;
    - **worker** — runs continuously without a domain (queues, bots);
    - **cron** — runs on a schedule;
    - **task** — runs to completion when you start it (migrations, one-off scripts).
3. **Upload** the artifact (drag and drop; large files resume after a broken connection) or give the image.
4. **Deploy.** The panel shows each step (pull, start, health check). The app is live at
   `https://<app>-<project>.<base domain>`.

Every deploy is a **release**: roll back to any earlier one (code and settings) from the Deploys tab.

## Next steps

- **Environment variables** and secrets: app → Environment. Secrets are encrypted with the master key.
  Reference other apps and databases with `${api.url}`, `${db.DATABASE_URL}`, shared values with
  `${group.KEY}` ([environments](environments.md)).
- **Scale**: replicas, CPU and memory limits, health check path (app → Settings).
- **Your own domain**: app → Domains → Add ([domains](domains.md)).
- **A database**: project → Databases ([databases](databases.md)).
- **Deploy from CI**: [CLI](cli.md) and [GitHub Actions](ci.md).
