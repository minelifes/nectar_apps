package deploy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"nectar_apps/internal/store"
)

// Apps marked as databases (an imported MySQL, a PostgreSQL run from an image): they stay apps (deploys,
// logs, charts, scaling) and also get what database add-ons have — `nectar db connect`, dump backups and
// connection variables for other apps. Credentials are read from the app's own variables.

// AppDatabaseEngines are the engines an app can be marked as.
var AppDatabaseEngines = []string{"postgres", "mysql", "mariadb", "redis"}

// AppDatabasePort is the port of a database app (its own setting, else the engine's usual one).
func AppDatabasePort(a store.App) int {
	if a.Database == nil {
		return 0
	}
	if a.Database.Port > 0 {
		return a.Database.Port
	}
	k, _ := AddonKindByID(a.Database.Engine)
	return k.Port
}

// AppDatabaseCredentials resolves user, password and database of a database app from its variables.
func AppDatabaseCredentials(db *store.AppDatabase, env map[string]string) (AddonCredentials, error) {
	if db == nil {
		return AddonCredentials{}, errors.New("the app is not marked as a database")
	}
	pick := func(literal, key, def string) string {
		if key != "" {
			if v, ok := env[key]; ok && v != "" {
				return v
			}
		}
		if literal != "" {
			return literal
		}
		return def
	}
	c := AddonCredentials{}
	switch db.Engine {
	case "postgres":
		c.User = pick(db.User, db.UserVar, "postgres")
		c.Database = pick(db.Database, db.DatabaseVar, c.User)
	case "mysql", "mariadb":
		c.User = pick(db.User, db.UserVar, "root")
		c.Database = pick(db.Database, db.DatabaseVar, "")
	case "redis":
	default:
		return c, fmt.Errorf("unknown database engine %q", db.Engine)
	}
	if db.PasswordVar != "" {
		v, ok := env[db.PasswordVar]
		if !ok {
			return c, fmt.Errorf("the app has no variable %s (its database password)", db.PasswordVar)
		}
		c.Password = v
	}
	return c, nil
}

// AppDatabaseEnv returns the connection variables other apps get from a database app.
func (d *Deployer) AppDatabaseEnv(ctx context.Context, a store.App, prefix string) (map[string]string, error) {
	env, err := d.DecryptEnv(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	c, err := AppDatabaseCredentials(a.Database, env)
	if err != nil {
		return nil, err
	}
	return DatabaseEnv(a.Database.Engine, a.Slug, AppDatabasePort(a), c, prefix), nil
}

// DetectAppDatabase guesses how an app is a database from its image and variables (nil = no idea).
func DetectAppDatabase(a store.App, env map[string]string) *store.AppDatabase {
	img := strings.ToLower(a.Config.Image)
	if i := strings.LastIndex(img, "/"); i >= 0 {
		img = img[i+1:]
	}
	has := func(k string) bool { _, ok := env[k]; return ok }
	first := func(keys ...string) string {
		for _, k := range keys {
			if has(k) {
				return k
			}
		}
		return ""
	}
	engine := ""
	switch {
	case strings.Contains(img, "postgres") || strings.Contains(img, "postgis") || strings.Contains(img, "timescale"):
		engine = "postgres"
	case strings.Contains(img, "mariadb"):
		engine = "mariadb"
	case strings.Contains(img, "mysql") || strings.Contains(img, "percona"):
		engine = "mysql"
	case strings.Contains(img, "redis") || strings.Contains(img, "valkey") || strings.Contains(img, "keydb"):
		engine = "redis"
	case has("POSTGRES_PASSWORD") || has("POSTGRESQL_PASSWORD"):
		engine = "postgres"
	case has("MARIADB_ROOT_PASSWORD") || has("MARIADB_PASSWORD"):
		engine = "mariadb"
	case has("MYSQL_ROOT_PASSWORD") || has("MYSQL_PASSWORD"):
		engine = "mysql"
	default:
		return nil
	}
	db := &store.AppDatabase{Engine: engine}
	switch engine {
	case "postgres":
		db.UserVar = first("POSTGRES_USER", "POSTGRESQL_USERNAME", "POSTGRESQL_USER")
		db.PasswordVar = first("POSTGRES_PASSWORD", "POSTGRESQL_PASSWORD")
		db.DatabaseVar = first("POSTGRES_DB", "POSTGRESQL_DATABASE")
		if db.UserVar == "" {
			db.User = "postgres"
		}
	case "mysql", "mariadb":
		// the application user when there is one (what apps should use), else root
		if u := first("MYSQL_USER", "MARIADB_USER"); u != "" && first("MYSQL_PASSWORD", "MARIADB_PASSWORD") != "" {
			db.UserVar, db.PasswordVar = u, first("MYSQL_PASSWORD", "MARIADB_PASSWORD")
		} else {
			db.User, db.PasswordVar = "root", first("MYSQL_ROOT_PASSWORD", "MARIADB_ROOT_PASSWORD")
		}
		db.DatabaseVar = first("MYSQL_DATABASE", "MARIADB_DATABASE")
	case "redis":
		db.PasswordVar = first("REDIS_PASSWORD", "REDIS_PASS", "REQUIREPASS")
	}
	// the port: the engine's usual one when the app serves it, else the app's main port
	k, _ := AddonKindByID(engine)
	if a.Config.Port != k.Port && !slices.Contains(a.Config.InternalPorts, k.Port) && a.Config.Port > 0 {
		db.Port = a.Config.Port
	}
	return db
}
