package blueprint

import (
	"strings"
	"testing"
)

func TestTemplatesRender(t *testing.T) {
	ts, err := Templates()
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) < 6 {
		t.Fatalf("only %d templates", len(ts))
	}
	ids := map[string]bool{}
	for _, tpl := range ts {
		if ids[tpl.ID] {
			t.Fatalf("duplicate template id %s", tpl.ID)
		}
		ids[tpl.ID] = true
		b, err := tpl.Render("myapp", nil)
		if err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		if len(b.Apps) == 0 || b.Apps[0].Slug != "myapp" {
			t.Fatalf("%s: main app slug %+v", tpl.ID, b.Apps)
		}
		for _, a := range b.Apps {
			for k, v := range a.Secrets {
				if strings.Contains(v, "[[") || v == "" {
					t.Fatalf("%s: secret %s = %q", tpl.ID, k, v)
				}
			}
		}
	}
	g, _ := TemplateByID("grafana")
	b, _ := g.Render("graf", map[string]string{"admin_user": "boss"})
	if b.Apps[0].Env["GF_SECURITY_ADMIN_USER"] != "boss" || len(b.Apps[0].Secrets["GF_SECURITY_ADMIN_PASSWORD"]) != 24 {
		t.Fatalf("grafana: %+v", b.Apps[0])
	}
	if _, err := g.Render("graf", map[string]string{"admin_user": "a\"b"}); err == nil {
		t.Fatal("quote in input accepted")
	}
	wp, _ := TemplateByID("wordpress")
	b, _ = wp.Render("blog", nil)
	if b.Addons[0].Slug != "blog-db" || b.Apps[0].Env["WORDPRESS_DB_HOST"] != "${blog-db.MYSQL_HOST}" {
		t.Fatalf("wordpress: %+v", b)
	}
}

func TestParse(t *testing.T) {
	b, err := Parse([]byte(`
apps:
  - slug: Web
    image: nginx:1
    port: 80
    env: { A: "1" }
    secrets: { B: "" }
    links: [{ addon: db }]
addons:
  - { slug: db, kind: postgres }
`))
	if err != nil || b.Apps[0].Slug != "web" || b.Apps[0].Runtime != "image" || b.Apps[0].Kind != "web" || b.Addons[0].Name != "db" {
		t.Fatalf("parse: %+v %v", b, err)
	}
	for _, bad := range []string{
		``,
		`apps: [{ slug: web }]`,
		`apps: [{ slug: web, image: x, unknown_field: 1 }]`,
		`addons: [{ slug: db, kind: oracle }]`,
		`apps: [{ slug: a, image: x }, { slug: a, image: y }]`,
		`apps: [{ slug: a, image: x, env: { K: 1 }, secrets: { K: 2 } }]`,
		`apps: [{ slug: a, image: x, links: [{ addon: b }] }, { slug: b, image: y }]`,
		`version: 9
apps: [{ slug: a, image: x }]`,
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	out, err := b.Marshal()
	if err != nil || !strings.Contains(string(out), "version: 1") {
		t.Fatalf("marshal: %s %v", out, err)
	}
	if again, err := Parse(out); err != nil || again.Apps[0].Image != "nginx:1" {
		t.Fatalf("round trip: %v", err)
	}
}

func TestFromCompose(t *testing.T) {
	b, notes, err := FromCompose([]byte(`
services:
  web:
    image: ghcr.io/acme/web:1.2
    ports: ["8080:3000", "9090:9090"]
    environment:
      DATABASE_URL: postgres://app:pw@db:5432/app
      API_TOKEN: abc
      LOG_LEVEL: info
    volumes: ["uploads:/app/uploads", "./config:/app/config:ro"]
    deploy: { replicas: 2, resources: { limits: { memory: 1g } } }
  db:
    image: postgres:16
    ports: ["5432:5432"]
    environment: ["POSTGRES_PASSWORD=pw", "POSTGRES_DB=app"]
    volumes: [pgdata:/var/lib/postgresql/data]
  queue_worker:
    build: .
    command: npm run worker
volumes: { uploads: {}, pgdata: {} }
`))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]App{}
	for _, a := range b.Apps {
		by[a.Slug] = a
	}
	web, db, w := by["web"], by["db"], by["queue-worker"]
	if web.Kind != "web" || web.Port != 3000 || len(web.InternalPorts) != 1 || web.InternalPorts[0] != 9090 || *web.Replicas != 2 || web.MemoryMB != 1024 ||
		web.Secrets["API_TOKEN"] != "abc" || web.Env["LOG_LEVEL"] != "info" || len(web.Volumes) != 1 || web.Volumes[0].Name != "uploads" {
		t.Fatalf("web: %+v", web)
	}
	if db.Kind != "worker" || db.Port != 5432 || db.Secrets["POSTGRES_PASSWORD"] != "pw" || db.Volumes[0].MountPath != "/var/lib/postgresql/data" {
		t.Fatalf("db: %+v", db)
	}
	if !strings.HasPrefix(w.Image, PlaceholderRegistry) || strings.Join(w.Command, " ") != "sh -c npm run worker" || w.Kind != "worker" {
		t.Fatalf("worker: %+v", w)
	}
	joined := strings.Join(notes, "\n")
	for _, want := range []string{"bind mount ./config", "queue_worker became app \"queue-worker\"", "built from source", "reachable by the project's apps only"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes lack %q:\n%s", want, joined)
		}
	}
	if _, _, err := FromCompose([]byte("services: {}")); err == nil {
		t.Fatal("empty compose accepted")
	}
}
