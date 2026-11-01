package deploy

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// LabelAddon marks the objects of a database add-on with its slug.
const LabelAddon = "nectar.dev/addon"

// AddonKind describes a database the panel can run.
type AddonKind struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Port     int            `json:"port"`
	Versions []AddonVersion `json:"versions"`
	// DataPath is where the volume is mounted.
	DataPath string `json:"-"`
	// MinMemoryMB is the smallest sensible memory limit.
	MinMemoryMB int64 `json:"min_memory_mb"`
	// RunAs is the image's own database user (uid = gid). The container starts as that user instead of root:
	// the entrypoints then skip their root → user switch (gosu), which hangs forever on hosts whose
	// AppArmor refuses signals inside containers (Ubuntu 26.04 + k3s 1.36: gosu signals its own threads
	// while switching). 0 = start as the image says (Redis: the panel runs redis-server directly).
	RunAs int64 `json:"-"`
}

// AddonVersion is an offered version with its image.
type AddonVersion struct {
	ID      string `json:"id"`
	Image   string `json:"image"`
	Default bool   `json:"default,omitempty"`
}

// AddonKinds is the add-on catalog.
var AddonKinds = []AddonKind{
	{ID: "postgres", Name: "PostgreSQL", Port: 5432, DataPath: "/var/lib/postgresql/data", MinMemoryMB: 128, RunAs: 70, Versions: []AddonVersion{
		{ID: "17", Image: "postgres:17-alpine", Default: true}, {ID: "16", Image: "postgres:16-alpine"}, {ID: "15", Image: "postgres:15-alpine"},
	}},
	{ID: "mysql", Name: "MySQL", Port: 3306, DataPath: "/var/lib/mysql", MinMemoryMB: 384, RunAs: 999, Versions: []AddonVersion{
		{ID: "8.4", Image: "mysql:8.4", Default: true}, {ID: "8.0", Image: "mysql:8.0"},
	}},
	{ID: "mariadb", Name: "MariaDB", Port: 3306, DataPath: "/var/lib/mysql", MinMemoryMB: 256, RunAs: 999, Versions: []AddonVersion{
		{ID: "11.4", Image: "mariadb:11.4", Default: true}, {ID: "10.11", Image: "mariadb:10.11"},
	}},
	{ID: "redis", Name: "Redis", Port: 6379, DataPath: "/data", MinMemoryMB: 64, Versions: []AddonVersion{
		{ID: "7.4", Image: "redis:7.4-alpine", Default: true},
	}},
}

// AddonKindByID returns a catalog entry.
func AddonKindByID(id string) (AddonKind, bool) {
	for _, k := range AddonKinds {
		if k.ID == id {
			return k, true
		}
	}
	return AddonKind{}, false
}

// Version returns a version of the kind ("" = default).
func (k AddonKind) Version(id string) (AddonVersion, bool) {
	for _, v := range k.Versions {
		if v.ID == id || (id == "" && v.Default) {
			return v, true
		}
	}
	return AddonVersion{}, false
}

// AddonCredentials are generated once when an add-on is created.
type AddonCredentials struct {
	User     string `json:"user,omitempty"`
	Password string `json:"password"`
	Database string `json:"database,omitempty"`
	RootPass string `json:"root_password,omitempty"`
}

// reservedDatabases are system databases of MySQL/MariaDB and PostgreSQL. An add-on named like one of
// them (slug "mysql" is the default of the MySQL template) must not get it as its app database: the app
// user would own MySQL's grant tables, or share PostgreSQL's maintenance database.
var reservedDatabases = map[string]bool{
	"mysql": true, "information_schema": true, "performance_schema": true, "sys": true,
	"postgres": true, "template0": true, "template1": true,
}

// NewAddonCredentials generates credentials for a kind.
func NewAddonCredentials(kind, slug string) AddonCredentials {
	db := sanitizeIdent(slug)
	if reservedDatabases[db] {
		db = "app"
	}
	switch kind {
	case "redis":
		return AddonCredentials{Password: randomSecret()}
	case "postgres":
		return AddonCredentials{User: "app", Password: randomSecret(), Database: db}
	default:
		return AddonCredentials{User: "app", Password: randomSecret(), Database: db, RootPass: randomSecret()}
	}
}

func sanitizeIdent(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c == '-' {
			b[i] = '_'
		}
	}
	return string(b)
}

func randomSecret() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:]) // URL-safe: fits into connection URLs as is
}

// SealCredentials encrypts credentials for the store.
func (d *Deployer) SealCredentials(c AddonCredentials) (string, error) {
	b, _ := json.Marshal(c)
	return d.envBox.Seal(string(b))
}

// AddonCredentials decrypts the credentials of an add-on.
func (d *Deployer) AddonCredentials(a store.Addon) (AddonCredentials, error) {
	var c AddonCredentials
	pt, err := d.envBox.Open(a.Credentials)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(pt), &c)
	return c, err
}

// AddonEnv returns the variables a linked app gets (host = the add-on's Service in the same namespace).
func AddonEnv(a store.Addon, c AddonCredentials, prefix string) map[string]string {
	k, _ := AddonKindByID(a.Kind)
	host, port := a.Slug, strconv.Itoa(k.Port)
	env := map[string]string{}
	switch a.Kind {
	case "postgres":
		env["DATABASE_URL"] = "postgres://" + url.UserPassword(c.User, c.Password).String() + "@" + host + ":" + port + "/" + c.Database + "?sslmode=disable"
		env["PGHOST"], env["PGPORT"], env["PGUSER"], env["PGPASSWORD"], env["PGDATABASE"] = host, port, c.User, c.Password, c.Database
	case "mysql", "mariadb":
		env["DATABASE_URL"] = "mysql://" + url.UserPassword(c.User, c.Password).String() + "@" + host + ":" + port + "/" + c.Database
		env["MYSQL_HOST"], env["MYSQL_PORT"], env["MYSQL_USER"], env["MYSQL_PASSWORD"], env["MYSQL_DATABASE"] = host, port, c.User, c.Password, c.Database
	case "redis":
		env["REDIS_URL"] = "redis://:" + url.QueryEscape(c.Password) + "@" + host + ":" + port + "/0"
		env["REDIS_HOST"], env["REDIS_PORT"], env["REDIS_PASSWORD"] = host, port, c.Password
	}
	if prefix == "" {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[prefix+k] = v
	}
	return out
}

func (a addonSpec) labels() map[string]any {
	return map[string]any{LabelManagedBy: ManagedBy, LabelProject: a.Project.Slug, LabelAddon: a.Addon.Slug}
}

type addonSpec struct {
	Addon   store.Addon
	Project store.Project
	Kind    AddonKind
	Image   string
	Creds   AddonCredentials
}

// AddonObjects returns the Secret, Service and StatefulSet of an add-on.
func AddonObjects(s addonSpec) []obj {
	a, ns, labels := s.Addon, s.Project.Namespace, s.labels()
	env := map[string]string{}
	var args []any
	switch a.Kind {
	case "postgres":
		env["POSTGRES_USER"], env["POSTGRES_PASSWORD"], env["POSTGRES_DB"] = s.Creds.User, s.Creds.Password, s.Creds.Database
		env["PGDATA"] = s.Kind.DataPath + "/pgdata" // a subfolder: the volume root may hold lost+found
	case "mysql", "mariadb":
		env["MYSQL_USER"], env["MYSQL_PASSWORD"], env["MYSQL_DATABASE"], env["MYSQL_ROOT_PASSWORD"] = s.Creds.User, s.Creds.Password, s.Creds.Database, s.Creds.RootPass
		// the first start loads the time zone tables (slow on a small CPU limit); apps use UTC offsets
		env["MYSQL_INITDB_SKIP_TZINFO"] = "1"
		if a.Kind == "mariadb" {
			env["MARIADB_AUTO_UPGRADE"] = "1"
			env["MARIADB_INITDB_SKIP_TZINFO"] = "1"
		}
		// Newer container runtimes give containers an open-files limit of ~1 billion; MySQL's start (and
		// MariaDB's) then spins at full CPU for minutes right after "Switching to dedicated user 'mysql'"
		// and never opens its port. Lower the limit to a sane 1M, then run the image's own entrypoint.
		server := "mysqld"
		if a.Kind == "mariadb" {
			server = "mariadbd"
		}
		args = []any{"sh", "-c", nofileWrapper, "db", server}
	case "redis":
		env["REDIS_PASSWORD"] = s.Creds.Password
		args = []any{"redis-server", "--requirepass", "$(REDIS_PASSWORD)", "--appendonly", "yes", "--dir", "/data"}
	}
	data := map[string]any{}
	for k, v := range env {
		data[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	replicas := 1
	if a.Stopped {
		replicas = 0
	}
	c := map[string]any{
		"name":            a.Kind,
		"image":           s.Image,
		"imagePullPolicy": "IfNotPresent",
		"envFrom":         []any{map[string]any{"secretRef": map[string]any{"name": a.Slug + "-credentials"}}},
		"ports":           []any{map[string]any{"name": "db", "containerPort": s.Kind.Port, "protocol": "TCP"}},
		"resources":       resourcesFor(store.Resources{CPUMillicores: a.CPUMillicores, MemoryMB: a.MemoryMB}),
		"volumeMounts":    []any{map[string]any{"name": "data", "mountPath": s.Kind.DataPath}},
		// Probes are TCP checks of the database port, run by the kubelet: every image initializes its data
		// directory with networking off (the port opens once the server takes connections), and a TCP check
		// needs nothing inside the container. Exec probes (pg_isready, mariadb-admin) failed on hosts where
		// the container runtime refuses exec'd processes, and they ate the 250m CPU limit of small databases.
		// The first start can take minutes on a small CPU limit: the startup probe allows 10 minutes and the
		// liveness probe only begins after it passed (a restart halfway would never finish).
		"startupProbe":   map[string]any{"tcpSocket": map[string]any{"port": "db"}, "periodSeconds": 10, "timeoutSeconds": 5, "failureThreshold": 60},
		"readinessProbe": map[string]any{"tcpSocket": map[string]any{"port": "db"}, "periodSeconds": 5, "timeoutSeconds": 3, "failureThreshold": 3},
		"livenessProbe":  map[string]any{"tcpSocket": map[string]any{"port": "db"}, "periodSeconds": 20, "timeoutSeconds": 5, "failureThreshold": 6},
	}
	if len(args) > 0 {
		switch a.Kind {
		case "redis":
			c["command"] = args[:1]
			c["args"] = args[1:]
		case "mysql", "mariadb":
			c["command"] = args
		default:
			c["args"] = args
		}
	}
	podSec := map[string]any{"seccompProfile": map[string]any{"type": "RuntimeDefault"}}
	if u := s.Kind.RunAs; u > 0 {
		podSec["runAsUser"], podSec["runAsGroup"], podSec["fsGroup"], podSec["runAsNonRoot"] = u, u, u, true
		podSec["fsGroupChangePolicy"] = "OnRootMismatch" // network storage: chown the volume once, not every start
		c["securityContext"] = map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []any{"ALL"}}}
	}
	tmplLabels := merge(labels, map[string]any{})
	return []obj{
		{
			"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
			"metadata": meta(a.Slug+"-credentials", ns, labels),
			"data":     data,
		},
		{
			"apiVersion": "v1", "kind": "Service",
			"metadata": meta(a.Slug, ns, labels),
			"spec": map[string]any{
				"type":     "ClusterIP",
				"selector": map[string]any{LabelAddon: a.Slug},
				"ports":    []any{map[string]any{"name": "db", "port": s.Kind.Port, "targetPort": "db", "protocol": "TCP"}},
			},
		},
		{
			"apiVersion": "apps/v1", "kind": "StatefulSet",
			"metadata": meta(a.Slug, ns, labels),
			"spec": map[string]any{
				"replicas":    replicas,
				"serviceName": a.Slug,
				"selector":    map[string]any{"matchLabels": map[string]any{LabelAddon: a.Slug}},
				"template": map[string]any{
					"metadata": map[string]any{"labels": tmplLabels},
					"spec": map[string]any{
						"automountServiceAccountToken":  false,
						"enableServiceLinks":            false,
						"terminationGracePeriodSeconds": 60,
						"securityContext":               podSec,
						"containers":                    []any{c},
					},
				},
				"volumeClaimTemplates": []any{map[string]any{
					"metadata": map[string]any{"name": "data", "labels": labels},
					"spec": map[string]any{
						"accessModes": []any{"ReadWriteOnce"},
						"resources":   map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dMi", a.SizeMB)}},
					},
				}},
			},
		},
	}
}

func (d *Deployer) addonSpec(ctx context.Context, a store.Addon) (addonSpec, error) {
	k, ok := AddonKindByID(a.Kind)
	if !ok {
		return addonSpec{}, fmt.Errorf("unknown add-on kind %q", a.Kind)
	}
	v, ok := k.Version(a.Version)
	if !ok {
		return addonSpec{}, fmt.Errorf("%s has no version %q", k.Name, a.Version)
	}
	p, err := d.store.GetProject(ctx, a.ProjectID)
	if err != nil {
		return addonSpec{}, err
	}
	creds, err := d.AddonCredentials(a)
	if err != nil {
		return addonSpec{}, err
	}
	return addonSpec{Addon: a, Project: p, Kind: k, Image: v.Image, Creds: creds}, nil
}

// ApplyAddon creates or updates the objects of an add-on (the volume of an existing one keeps its size:
// StatefulSet volume templates cannot change).
func (d *Deployer) ApplyAddon(ctx context.Context, a store.Addon) error {
	s, err := d.addonSpec(ctx, a)
	if err != nil {
		return err
	}
	if err := d.EnsureProject(ctx, s.Project); err != nil {
		return fmt.Errorf("prepare project: %w", err)
	}
	for _, o := range AddonObjects(s) {
		kind, ns, name := kube.Meta(o)
		if kind == "StatefulSet" {
			var cur kube.Object
			if err := d.kube.GetObject(ctx, kind, ns, name, &cur); err == nil {
				// volumeClaimTemplates are immutable: keep the existing ones
				if spec, ok := o["spec"].(map[string]any); ok {
					spec["volumeClaimTemplates"] = kube.Field(cur, "spec", "volumeClaimTemplates")
				}
			} else if !kube.IsNotFound(err) {
				return err
			}
		}
		if _, err := d.kube.Apply(ctx, o); err != nil {
			return err
		}
	}
	status := StatusRunning
	if a.Stopped {
		status = StatusStopped
	}
	return d.store.SetAddonStatus(ctx, a.ID, status, "")
}

// AddonLive is the live state of an add-on.
type AddonLive struct {
	Ready    bool   `json:"ready"`
	Phase    string `json:"phase,omitempty"` // of the pod
	Restarts int64  `json:"restarts"`
	Problem  string `json:"problem,omitempty"`
	Volume   string `json:"volume,omitempty"` // PVC name
	UsedB    int64  `json:"used_bytes"`       // size of the data, -1 = not measured yet
}

// AddonStatus reads the live state.
func (d *Deployer) AddonStatus(ctx context.Context, a store.Addon, p store.Project) (AddonLive, error) {
	live := AddonLive{Volume: "data-" + a.Slug + "-0", UsedB: -1}
	if b, ok := d.measuredUsage(p.Namespace + "/" + live.Volume); ok {
		live.UsedB = b
	}
	var ss kube.Object
	if err := d.kube.GetObject(ctx, "StatefulSet", p.Namespace, a.Slug, &ss); err != nil {
		if kube.IsNotFound(err) {
			return live, nil
		}
		return live, err
	}
	live.Ready = kube.Int(ss, "status", "readyReplicas") > 0
	pods, err := d.kube.List(ctx, "Pod", p.Namespace, LabelAddon+"="+a.Slug)
	if err != nil {
		return live, err
	}
	for _, pod := range pods {
		live.Phase = kube.Str(pod, "status", "phase")
		live.Problem = podProblem(pod)
		for _, cs := range kube.Slice(pod, "status", "containerStatuses") {
			live.Restarts += kube.Int(cs, "restartCount")
		}
	}
	return live, nil
}

// DeleteAddon removes every object of an add-on, including its data volume.
func (d *Deployer) DeleteAddon(ctx context.Context, a store.Addon, p store.Project) error {
	sel := LabelAddon + "=" + a.Slug + "," + LabelManagedBy + "=" + ManagedBy
	for _, kind := range []string{"StatefulSet", "Service", "Secret", "PersistentVolumeClaim"} {
		items, err := d.kube.List(ctx, kind, p.Namespace, sel)
		if kube.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, it := range items {
			_, _, name := kube.Meta(it)
			if err := d.kube.Delete(ctx, kind, p.Namespace, name); err != nil && !kube.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

// LinkedEnv returns the env vars an app gets from its linked add-ons and the ones shared with its project.
func (d *Deployer) LinkedEnv(ctx context.Context, appID int64) (map[string]string, error) {
	app, err := d.store.GetApp(ctx, appID)
	if err != nil {
		return nil, err
	}
	links, err := d.store.EffectiveAddonLinks(ctx, app)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, l := range links {
		a, err := d.store.GetAddon(ctx, l.AddonID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		c, err := d.AddonCredentials(a)
		if err != nil {
			return nil, fmt.Errorf("add-on %s: %w", a.Slug, err)
		}
		for k, v := range AddonEnv(a, c, l.Prefix) {
			out[k] = v
		}
	}
	return out, nil
}

// nofileWrapper lowers the open-files limit when it is above 1M (or unlimited) and runs the image's entrypoint
// with the server command ($@). Used for MySQL/MariaDB (see AddonObjects).
const nofileWrapper = `n=$(ulimit -n); if [ "$n" = unlimited ] || [ "$n" -gt 1048576 ]; then ulimit -n 1048576; fi; exec docker-entrypoint.sh "$@"`
