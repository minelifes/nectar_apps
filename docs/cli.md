# The `nectar` CLI

`install.sh` puts the CLI on the server (`/usr/local/bin/nectar`, already pointed at the local panel, so
`nectar login` only asks for a token). For your own machine, binaries for Linux, macOS and Windows are attached
to every [release](https://github.com/minelifes/nectar_apps/releases); `make cli` builds it from source.

Create a token under **Settings → API tokens** (for CI: role **Deployer**, limited to one project).

```bash
nectar login --url https://panel.apps.example.com          # asks for the token, saves it in ~/.config/nectar
nectar apps
nectar deploy shop/api --file target/app.jar                # uploads, deploys, waits until it is live
nectar deploy shop/web --image ghcr.io/acme/web:1.4.2       # apps of the Docker image type
nectar status shop/api
nectar logs shop/api --tail 100
nectar rollback shop/api                                    # or --to 12
```

| Command | |
|---|---|
| `nectar login [--url URL] [--token nat_…]` | save the panel and an API token |
| `nectar whoami` | who the token belongs to |
| `nectar apps [--project SLUG]` | list apps |
| `nectar deploy APP [--file F \| --image REF] [--no-wait] [--timeout 15m] [--force]` | deploy and wait until live |
| `nectar status APP` | status, replicas and the live release |
| `nectar releases APP` | release history |
| `nectar rollback APP [--to N]` | redeploy release N (default: the previous one) |
| `nectar logs APP [--tail 200]` | the latest log lines |
| `nectar export --project SLUG` | the project as [nectar.yaml](blueprints.md) |
| `nectar apply -f nectar.yaml --project SLUG [--no-deploy]` | create what is missing, update what exists |
| `nectar import-compose -f docker-compose.yml --project SLUG [--dry-run] [--no-deploy]` | convert a compose file |
| `nectar db list [--project SLUG]` | the project's databases |
| `nectar db connect DB [--project SLUG] [--port N] [--show-password]` | open a database on `127.0.0.1` through the panel |
| `nectar templates` | one-click apps |
| `nectar install TEMPLATE --project SLUG [--name NAME] [--set key=value]…` | install a one-click app |
| `nectar version` | |

## Connecting to a database from your computer

Databases are only reachable from inside their project. `nectar db connect` opens one on your computer
through the panel, with no SSH or open port needed:

```sh
$ nectar db connect vps/mysql
vps/mysql (MySQL 8.4) is on 127.0.0.1:3306 — press Ctrl+C to close
  user      app
  database  app
  password  (add --show-password, or Show credentials on the database page)
  connect   mysql -h 127.0.0.1 -P 3306 -u app -p app
```

Point any client at that address: `psql`, `mysql`, `redis-cli`, TablePlus, DBeaver or DataGrip. It uses the
database's usual port when it is free, otherwise a free one (`--port` picks one). Each connection your client
makes gets its own tunnel. It needs the permission to see the database's credentials, every tunnel is in the
audit log, and the client still logs in with the database password.

`APP` is the app id or `project/app`. `NECTAR_URL` and `NECTAR_TOKEN` override the saved login (that is what
CI uses). A rollback brings back the code and settings of that release; the app keeps its current number of
replicas and its volumes.
