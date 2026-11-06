package backup

import (
	"errors"
	"fmt"
	"strconv"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
)

// Backups of apps marked as databases are dumps (pg_dump, mysqldump/mariadb-dump, a Redis RDB), not copies
// of their volumes: a copy of a running database's files is not consistent. The dump runs in the app's own
// image (client and server match) with the credentials from the app's variables (its Secret "<app>-env").

// appDBEnv passes user, password and database to the dump and load containers as NECTAR_DB_*.
func appDBEnv(a store.App) []any {
	db := a.Database
	secret := a.Slug + "-env"
	val := func(name, literal, key string) map[string]any {
		if key != "" {
			return map[string]any{"name": name, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": secret, "key": key, "optional": true}}}
		}
		return map[string]any{"name": name, "value": literal}
	}
	user := db.User
	if user == "" && db.UserVar == "" {
		switch db.Engine {
		case "postgres":
			user = "postgres"
		case "mysql", "mariadb":
			user = "root"
		}
	}
	return []any{
		val("NECTAR_DB_USER", user, db.UserVar),
		val("NECTAR_DB_PASSWORD", "", db.PasswordVar),
		val("NECTAR_DB_NAME", db.Database, db.DatabaseVar),
	}
}

func appDBTarget(a store.App) (host, port string) {
	return a.Slug, strconv.Itoa(deploy.AppDatabasePort(a))
}

// appDumpContainer dumps a database app into /work/<file>.
func appDumpContainer(a store.App) (map[string]any, string, error) {
	if a.Database == nil {
		return nil, "", errors.New("the app is not marked as a database")
	}
	if a.Config.Image == "" {
		return nil, "", errors.New("database backups need an image app (the dump runs in its image)")
	}
	host, port := appDBTarget(a)
	var script, file string
	switch a.Database.Engine {
	case "postgres":
		file = "db.dump"
		script = `PGPASSWORD="$NECTAR_DB_PASSWORD" pg_dump -h ` + host + ` -p ` + port + ` -U "$NECTAR_DB_USER" -d "${NECTAR_DB_NAME:-$NECTAR_DB_USER}" -Fc -f /work/db.dump`
	case "mysql", "mariadb":
		file = "db.sql"
		script = `if [ -n "$NECTAR_DB_NAME" ]; then set -- --databases "$NECTAR_DB_NAME"; else set -- --all-databases; fi
dump=mysqldump; command -v mariadb-dump >/dev/null 2>&1 && dump=mariadb-dump
"$dump" -h ` + host + ` -P ` + port + ` -u"$NECTAR_DB_USER" -p"$NECTAR_DB_PASSWORD" --single-transaction --routines --triggers --no-tablespaces "$@" > /work/db.sql`
	case "redis":
		file = "dump.rdb"
		script = `if [ -n "$NECTAR_DB_PASSWORD" ]; then set -- -a "$NECTAR_DB_PASSWORD" --no-auth-warning; else set --; fi
redis-cli -h ` + host + ` -p ` + port + ` "$@" --rdb /work/dump.rdb`
	default:
		return nil, "", fmt.Errorf("unknown database engine %q", a.Database.Engine)
	}
	return map[string]any{
		"name":            "dump",
		"image":           a.Config.Image,
		"imagePullPolicy": "IfNotPresent",
		"command":         []any{"sh", "-c", "set -eu\n" + script + "\nls -l /work"},
		"env":             appDBEnv(a),
		"volumeMounts":    []any{map[string]any{"name": "work", "mountPath": "/work"}},
	}, file, nil
}

// appLoadContainer loads /work/<file> into a running PostgreSQL / MySQL / MariaDB app.
func appLoadContainer(a store.App) (map[string]any, string, error) {
	host, port := appDBTarget(a)
	var load, file string
	switch a.Database.Engine {
	case "postgres":
		file = "db.dump"
		load = `PGPASSWORD="$NECTAR_DB_PASSWORD" pg_restore --clean --if-exists --no-owner -h ` + host + ` -p ` + port + ` -U "$NECTAR_DB_USER" -d "${NECTAR_DB_NAME:-$NECTAR_DB_USER}" /work/db.dump`
	case "mysql", "mariadb":
		file = "db.sql"
		load = `client=mysql; command -v mariadb >/dev/null 2>&1 && client=mariadb
"$client" -h ` + host + ` -P ` + port + ` -u"$NECTAR_DB_USER" -p"$NECTAR_DB_PASSWORD" < /work/db.sql`
	default:
		return nil, "", fmt.Errorf("%s dumps are restored into the stopped app's volume", a.Database.Engine)
	}
	return map[string]any{
		"name":            "load",
		"image":           a.Config.Image,
		"imagePullPolicy": "IfNotPresent",
		"command":         []any{"sh", "-c", "set -eu\n" + load + "\necho restored"},
		"env":             appDBEnv(a),
		"volumeMounts":    []any{map[string]any{"name": "work", "mountPath": "/work"}},
	}, file, nil
}

// redisDataVolume is the volume a Redis app keeps dump.rdb in (mounted at /data, else its only volume).
func redisDataVolume(a store.App) (store.Volume, bool) {
	for _, v := range a.Config.Volumes {
		if v.MountPath == "/data" {
			return v, true
		}
	}
	if len(a.Config.Volumes) == 1 {
		return a.Config.Volumes[0], true
	}
	return store.Volume{}, false
}
