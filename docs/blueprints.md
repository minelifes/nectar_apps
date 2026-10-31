# nectar.yaml, one-click apps and docker-compose

## nectar.yaml

A project's apps, databases, variables, env groups, volumes, links and domains as one file you can keep in Git:

```yaml
version: 1
env_groups:
  - name: shared
    env: { REGION: eu-central }
addons:
  - { slug: db, kind: postgres, version: "17", size_mb: 2048 }
apps:
  - slug: web
    image: ghcr.io/acme/web:1.4
    port: 8080
    env: { LOG_LEVEL: info, API_URL: "${api.url}" }
    secrets: { SESSION_KEY: "" }        # values are never exported; "" keeps the current one
    env_groups: [shared]
    links: [{ addon: db }]              # DATABASE_URL, PG* …
    volumes: [{ name: data, mount_path: /data, size_mb: 1024 }]
    domains: [shop.example.com]
  - slug: api
    image: ghcr.io/acme/api:2.0
    port: 3000
    replicas: 2
    health_path: /healthz
```

- **Export**: project → ⋮ → Export, or `nectar export --project shop > nectar.yaml`.
- **Apply**: project → ⋮ → Apply, or `nectar apply -f nectar.yaml --project shop`. Applying creates what is
  missing and updates existing apps; it **never deletes** anything.

Fields per app: `slug`, `name`, `kind` (web, worker, cron, task), `runtime` (image, java, node, …),
`runtime_version`, `image`, `port`, `internal_ports`, `command`, `args`, `replicas`, `cpu_millicores`,
`memory_mb`, `health_path`, `schedule`, `release_command`, `env`, `secrets`, `volumes` (`storage_class`),
`expose` (extra TCP/UDP ports), `domains`, `links` (`addon`, `prefix`), `env_groups`.

## References

Filled in at every deploy:

| Reference | Value |
|---|---|
| `${db.DATABASE_URL}`, `${db.PGHOST}` … | a database's connection settings (without linking it) |
| `${api.host}`, `${api.port}`, `${api.url}` | another app of the project (internal address) |
| `${api.SOME_VAR}` | one of another app's variables |
| `${group.KEY}` | a value from a project env group |

Workers with a port (e.g. MongoDB) and `internal_ports` are reachable by the project's apps at
`<app>:<port>`, never from outside.

## One-click apps

Project → **One-click app**. Search the gallery or filter it by category, pick a template, name it and
install it. The panel creates the apps, a managed database where one is needed, the volumes and generated
passwords, then deploys them. After installing, the dialog shows how the project's apps connect (the
`*_URL` to use).

| Category | Templates |
|---|---|
| Messaging | Mosquitto (MQTT), EMQX (MQTT with dashboard), Apache Kafka + Kafka UI, NATS (JetStream), RabbitMQ |
| Databases | Redis, PostgreSQL, MySQL (managed databases), MongoDB, Memcached, ClickHouse, InfluxDB, Qdrant |
| Search | Meilisearch, Typesense |
| Monitoring | Grafana, Prometheus, Jaeger, Uptime Kuma |
| Developer tools | Gitea, Mailpit, pgAdmin, Adminer, Verdaccio |
| Auth | Keycloak |
| Websites, automation, storage | WordPress, n8n, MinIO |

Brokers and data stores without a login of their own (Kafka, NATS, Memcached, ClickHouse, Prometheus) run as
workers, so only the project's apps can reach them. The MQTT brokers also open port 1883 on the server for
devices; the node port is on the app's Overview. Tools with a web UI either have a login (Kafka UI, Mailpit,
pgAdmin, Grafana) or say in their notes to restrict the domain with Allowed IPs (Jaeger, Adminer). `nectar templates` lists them and `nectar install <template> --project shop` installs one. The
catalog is data (`internal/blueprint/templates.yaml`).

## docker-compose

Project → ⋮ → **Import docker-compose.yml**, or `nectar import-compose -f docker-compose.yml --project shop`.
Every service becomes an app with the same name (so services still find each other), services with published
ports get a domain, databases and brokers stay internal, named volumes become volumes. **Check** shows the
converted file and what could not be imported (bind mounts, `build:`, `env_file`).
