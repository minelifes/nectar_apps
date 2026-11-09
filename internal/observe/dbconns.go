package observe

import (
	"context"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// Execer runs a command in a container (kube.Client.ExecRun).
type Execer interface {
	ExecRun(ctx context.Context, namespace, pod, container string, command []string, stdin []byte, maxOut int) (kube.ExecResult, error)
}

// ConnsEvery is how often the collector asks each database add-on for its connections (an exec into the
// pod each time); samples in between repeat the last answer.
const ConnsEvery = time.Minute

type connsSample struct {
	at         time.Time
	conns, max int64
	ok         bool
}

// connsScript prints "<client connections> <limit>" for a database kind, from inside its container
// (credentials from the container's own environment; the query's own connection is not counted).
func connsScript(kind string) string {
	switch kind {
	case "postgres":
		return `psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tA -c "SELECT (SELECT count(*) - 1 FROM pg_stat_activity WHERE backend_type = 'client backend') || ' ' || current_setting('max_connections')"`
	case "mysql", "mariadb":
		// SHOW STATUS works with performance_schema off (small-memory tuning)
		return `cli=$(command -v mariadb || command -v mysql); export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; ` +
			`c=$("$cli" -uroot -N -B -e "SHOW GLOBAL STATUS LIKE 'Threads_connected'" | awk '{print $2}') && ` +
			`m=$("$cli" -uroot -N -B -e "SELECT @@max_connections") && echo "$((c - 1)) $m"`
	case "redis":
		return `c=$(redis-cli -a "$REDIS_PASSWORD" --no-auth-warning info clients | sed -n 's/^connected_clients:\([0-9]*\).*/\1/p'); ` +
			`m=$(redis-cli -a "$REDIS_PASSWORD" --no-auth-warning config get maxclients | tail -n 1 | tr -d '\r'); echo "$((c - 1)) $m"`
	}
	return ""
}

// parseConns reads "<conns> <max>" (tabs or spaces).
func parseConns(out string) (conns, max int64, ok bool) {
	f := strings.Fields(out)
	if len(f) != 2 {
		return 0, 0, false
	}
	c, err1 := strconv.ParseInt(f[0], 10, 64)
	m, err2 := strconv.ParseInt(f[1], 10, 64)
	if err1 != nil || err2 != nil || m <= 0 {
		return 0, 0, false
	}
	return max64(c, 0), m, true
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// dbConns adds the connections of the running database add-ons to their samples (series addon:<id>).
func (c *Collector) dbConns(ctx context.Context, now time.Time, samples []store.MetricSample) {
	if c.Exec == nil {
		return
	}
	bySeries := map[string]*store.MetricSample{}
	for i := range samples {
		bySeries[samples[i].Series] = &samples[i]
	}
	projects, err := c.Store.ListProjects(ctx)
	if err != nil {
		return
	}
	seen := map[int64]bool{}
	for _, p := range projects {
		addons, err := c.Store.ListAddons(ctx, p.ID)
		if err != nil {
			continue
		}
		for _, a := range addons {
			m := bySeries["addon:"+strconv.FormatInt(a.ID, 10)]
			script := connsScript(a.Kind)
			if m == nil || a.Stopped || script == "" {
				continue
			}
			seen[a.ID] = true
			c.mu.Lock()
			last, had := c.conns[a.ID]
			c.mu.Unlock()
			if !had || now.Sub(last.at) >= ConnsEvery {
				last = connsSample{at: now}
				cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				res, err := c.Exec.ExecRun(cctx, p.Namespace, a.Slug+"-0", a.Kind, []string{"sh", "-c", script}, nil, 4096)
				cancel()
				if err == nil && res.ExitCode == 0 {
					last.conns, last.max, last.ok = parseConns(string(res.Stdout))
				} else if c.Log != nil {
					c.Log.Debug("metrics: db connections", "addon", p.Slug+"/"+a.Slug, "err", err, "stderr", res.Stderr)
				}
				c.mu.Lock()
				if c.conns == nil {
					c.conns = map[int64]connsSample{}
				}
				c.conns[a.ID] = last
				c.mu.Unlock()
			}
			if last.ok {
				m.Conns, m.ConnsMax = last.conns, last.max
			}
		}
	}
	c.mu.Lock()
	for id := range c.conns {
		if !seen[id] {
			delete(c.conns, id)
		}
	}
	c.mu.Unlock()
}
