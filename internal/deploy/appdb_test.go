package deploy

import (
	"testing"

	"nectar_apps/internal/store"
)

func TestDetectAppDatabase(t *testing.T) {
	app := func(image string, port int) store.App {
		return store.App{Slug: "db", Config: store.AppConfig{Image: image, Port: port}}
	}
	cases := []struct {
		name string
		app  store.App
		env  map[string]string
		want store.AppDatabase
	}{
		{"mysql app user", app("mysql:8.4", 3306), map[string]string{"MYSQL_ROOT_PASSWORD": "r", "MYSQL_USER": "u", "MYSQL_PASSWORD": "p", "MYSQL_DATABASE": "d"},
			store.AppDatabase{Engine: "mysql", UserVar: "MYSQL_USER", PasswordVar: "MYSQL_PASSWORD", DatabaseVar: "MYSQL_DATABASE"}},
		{"mariadb root", app("docker.io/library/mariadb:11", 3306), map[string]string{"MARIADB_ROOT_PASSWORD": "r"},
			store.AppDatabase{Engine: "mariadb", User: "root", PasswordVar: "MARIADB_ROOT_PASSWORD"}},
		{"postgres default user", app("postgis/postgis:16-3.4", 5432), map[string]string{"POSTGRES_PASSWORD": "p"},
			store.AppDatabase{Engine: "postgres", User: "postgres", PasswordVar: "POSTGRES_PASSWORD"}},
		{"redis other port", app("valkey/valkey:8", 6380), map[string]string{"REDIS_PASSWORD": "p"},
			store.AppDatabase{Engine: "redis", PasswordVar: "REDIS_PASSWORD", Port: 6380}},
		{"by variables", app("ghcr.io/me/custom:1", 5432), map[string]string{"POSTGRES_PASSWORD": "p", "POSTGRES_USER": "me"},
			store.AppDatabase{Engine: "postgres", UserVar: "POSTGRES_USER", PasswordVar: "POSTGRES_PASSWORD"}},
	}
	for _, c := range cases {
		got := DetectAppDatabase(c.app, c.env)
		if got == nil || *got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
	if got := DetectAppDatabase(app("nginx:1", 80), map[string]string{"FOO": "1"}); got != nil {
		t.Errorf("nginx: %+v", got)
	}
	// credentials: literal and variable sources, defaults
	c, err := AppDatabaseCredentials(&store.AppDatabase{Engine: "mysql", User: "root", PasswordVar: "PW"}, map[string]string{"PW": "x"})
	if err != nil || c.User != "root" || c.Password != "x" || c.Database != "" {
		t.Errorf("mysql creds: %+v %v", c, err)
	}
	if _, err := AppDatabaseCredentials(&store.AppDatabase{Engine: "postgres", PasswordVar: "GONE"}, map[string]string{}); err == nil {
		t.Error("a missing password variable must fail")
	}
	if env := DatabaseEnv("redis", "db", 6380, AddonCredentials{Password: "p w"}, "CACHE_"); env["CACHE_REDIS_URL"] == "" || env["CACHE_REDIS_PORT"] != "6380" {
		t.Errorf("redis env: %v", env)
	}
}
