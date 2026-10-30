package deploy

import (
	"context"
	"strings"
	"testing"

	"nectar_apps/internal/store"
)

func TestResolveRefs(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, _ := e.app(t, javaApp())
	creds := NewAddonCredentials("postgres", "db")
	sealed, _ := e.d.SealCredentials(creds)
	if _, err := e.st.CreateAddon(ctx, store.Addon{ProjectID: e.p.ID, Slug: "db", Name: "DB", Kind: "postgres", Version: "17", SizeMB: 1024, Credentials: sealed}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"DB_HOST":  "${db.PGHOST}",
		"DSN":      "host=${db.PGHOST} user=${db.PGUSER} password=${db.PGPASSWORD}",
		"SELF":     "${" + a.Slug + ".url}",
		"LITERAL":  "$${db.PGHOST}",
		"PLAIN":    "no refs here $HOME",
		"TEMPLATE": "${not a ref}",
	}
	if err := e.d.ResolveRefs(ctx, e.p, env); err != nil {
		t.Fatal(err)
	}
	if env["DB_HOST"] != "db" || env["DSN"] != "host=db user=app password="+creds.Password || env["LITERAL"] != "${db.PGHOST}" ||
		env["PLAIN"] != "no refs here $HOME" || env["TEMPLATE"] != "${not a ref}" ||
		env["SELF"] != "http://"+a.Slug+"."+e.p.Namespace+".svc.cluster.local:8080" {
		t.Fatalf("resolved: %v", env)
	}
	enc, _ := e.box.Seal("s3cr3t")
	e.st.ReplaceEnv(ctx, a.ID, []store.EnvVar{{Key: "ROOT_PASSWORD", ValueEnc: enc, IsSecret: true}})
	other := map[string]string{"PW": "${" + a.Slug + ".ROOT_PASSWORD}"}
	if err := e.d.ResolveRefs(ctx, e.p, other); err != nil || other["PW"] != "s3cr3t" {
		t.Fatalf("app variable: %v %v", other, err)
	}
	err := e.d.ResolveRefs(ctx, e.p, map[string]string{"X": "${cache.REDIS_URL}", "Y": "${db.NOPE}"})
	if err == nil || !strings.Contains(err.Error(), "${cache.REDIS_URL}") || !strings.Contains(err.Error(), "${db.NOPE}") {
		t.Fatalf("missing refs: %v", err)
	}
}
