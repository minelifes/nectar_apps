package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"nectar_apps/internal/wsock"
)

// Database is a database add-on of a project.
type Database struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	KindName  string `json:"kind_name"`
	Version   string `json:"version"`
	Port      int    `json:"port"`
	Stopped   bool   `json:"stopped"`
	Shared    bool   `json:"shared"`
}

// Databases lists the databases of a project.
func (c *Client) Databases(ctx context.Context, projectID int64) ([]Database, error) {
	var ds []Database
	return ds, c.getJSON(ctx, "/projects/"+strconv.FormatInt(projectID, 10)+"/addons", &ds)
}

// ResolveDatabase finds a database by slug in a project ("project/db" or --project; "" = the only project).
func (c *Client) ResolveDatabase(ctx context.Context, ref, projectSlug string) (Database, Project, error) {
	if p, db, ok := strings.Cut(ref, "/"); ok {
		projectSlug, ref = p, db
	}
	proj, err := c.ResolveProject(ctx, projectSlug)
	if err != nil {
		return Database{}, proj, err
	}
	ds, err := c.Databases(ctx, proj.ID)
	if err != nil {
		return Database{}, proj, err
	}
	for _, d := range ds {
		if d.Slug == ref || strconv.FormatInt(d.ID, 10) == ref {
			return d, proj, nil
		}
	}
	names := make([]string, 0, len(ds))
	for _, d := range ds {
		names = append(names, d.Slug)
	}
	return Database{}, proj, fmt.Errorf("no database %q in project %s (databases: %s)", ref, proj.Slug, strings.Join(names, ", "))
}

// DatabaseCredentials are the connection settings of a database (audited on the panel).
type DatabaseCredentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// Credentials returns a database's user, password and database name.
func (c *Client) Credentials(ctx context.Context, id int64) (DatabaseCredentials, error) {
	var cr DatabaseCredentials
	return cr, c.getJSON(ctx, "/addons/"+strconv.FormatInt(id, 10)+"/credentials", &cr)
}

// OpenTunnel opens one TCP connection to a database through the panel (a WebSocket carrying raw bytes).
func (c *Client) OpenTunnel(ctx context.Context, id int64) (*wsock.Conn, error) {
	u := c.URL + "/api/addons/" + strconv.FormatInt(id, 10) + "/tunnel"
	switch {
	case strings.HasPrefix(u, "https://"):
		u = "wss://" + strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		u = "ws://" + strings.TrimPrefix(u, "http://")
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+c.Token)
	conn, _, err := wsock.Dial(ctx, u, h, nil)
	if err != nil {
		var he *wsock.HandshakeError
		if errors.As(err, &he) {
			return nil, fmt.Errorf("the panel refused the tunnel: %s", strings.TrimSpace(he.Error()))
		}
		return nil, err
	}
	return conn, nil
}

// ServeTunnel accepts local connections and forwards each one through its own tunnel until ctx ends.
func ServeTunnel(ctx context.Context, ln net.Listener, open func(context.Context) (*wsock.Conn, error), log io.Writer) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		local, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer local.Close()
			octx, cancel := context.WithTimeout(ctx, 20*time.Second)
			remote, err := open(octx)
			cancel()
			if err != nil {
				fmt.Fprintln(log, "connection from", local.RemoteAddr(), "failed:", err)
				return
			}
			defer remote.Close()
			pipe(ctx, local, remote)
		}()
	}
}

// pipe copies bytes between a local connection and a tunnel until either side closes.
func pipe(ctx context.Context, local net.Conn, remote *wsock.Conn) {
	done := make(chan struct{}, 2)
	go func() { // tunnel → local
		defer func() { done <- struct{}{} }()
		for {
			op, msg, err := remote.ReadMessage()
			if err != nil {
				return
			}
			if op == wsock.Binary && len(msg) > 0 {
				if _, err := local.Write(msg); err != nil {
					return
				}
			}
		}
	}()
	go func() { // local → tunnel
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 32<<10)
		for {
			n, err := local.Read(buf)
			if n > 0 {
				if werr := remote.WriteMessage(wsock.Binary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	local.Close()
	remote.Close()
}

// clientCommand is a ready-to-run client command for a database on a local port.
func clientCommand(kind string, port int, cr DatabaseCredentials) string {
	switch kind {
	case "postgres":
		return fmt.Sprintf(`psql "postgres://%s@127.0.0.1:%d/%s?sslmode=disable"`, cr.User, port, cr.Database)
	case "mysql", "mariadb":
		return fmt.Sprintf("mysql -h 127.0.0.1 -P %d -u %s -p %s", port, cr.User, cr.Database)
	case "redis":
		return fmt.Sprintf("redis-cli -h 127.0.0.1 -p %d --askpass", port)
	}
	return ""
}

// listenLocal listens on 127.0.0.1:port, or on any free port when port is taken and was not asked for.
func listenLocal(port int, explicit bool) (net.Listener, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err == nil || explicit {
		return ln, err
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

// dbCommand runs `nectar db list|connect`.
func dbCommand(ctx context.Context, c *Client, args []string, env Env) error {
	if len(args) == 0 {
		return errors.New("usage: nectar db list [--project SLUG] | nectar db connect DB [--project SLUG] [--port N] [--show-password]")
	}
	sub, args := args[0], args[1:]
	fs := newFlagSet("nectar db "+sub, env)
	project := fs.String("project", "", "project slug")
	switch sub {
	case "list", "ls":
		if _, err := parse(fs, args); err != nil {
			return err
		}
		proj, err := c.ResolveProject(ctx, *project)
		if err != nil {
			return err
		}
		ds, err := c.Databases(ctx, proj.ID)
		if err != nil {
			return err
		}
		tw := newTable(env.Stdout)
		fmt.Fprintln(tw, "DATABASE\tKIND\tSTATE")
		for _, d := range ds {
			state := "running"
			if d.Stopped {
				state = "stopped"
			}
			if d.Shared {
				state += ", shared with the project"
			}
			fmt.Fprintf(tw, "%s/%s\t%s %s\t%s\n", proj.Slug, d.Slug, d.KindName, d.Version, state)
		}
		return tw.Flush()

	case "connect":
		port := fs.Int("port", 0, "local port (default: the database's usual port, or a free one)")
		show := fs.Bool("show-password", false, "print the password and a connection URL")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: nectar db connect DB [--project SLUG] [--port N] [--show-password]")
		}
		d, proj, err := c.ResolveDatabase(ctx, pos[0], *project)
		if err != nil {
			return err
		}
		if d.Stopped {
			return fmt.Errorf("%s/%s is stopped: start it on the panel first", proj.Slug, d.Slug)
		}
		want := *port
		if want == 0 {
			want = d.Port
		}
		ln, err := listenLocal(want, *port != 0)
		if err != nil {
			return fmt.Errorf("cannot listen on 127.0.0.1:%d: %w", want, err)
		}
		local := ln.Addr().(*net.TCPAddr).Port
		// check the tunnel once before printing: a missing permission or a stopped database fails now
		probe, err := c.OpenTunnel(ctx, d.ID)
		if err != nil {
			ln.Close()
			return err
		}
		probe.Close()
		cr, credErr := c.Credentials(ctx, d.ID)
		fmt.Fprintf(env.Stdout, "%s/%s (%s %s) is on 127.0.0.1:%d — press Ctrl+C to close\n", proj.Slug, d.Slug, d.KindName, d.Version, local)
		if credErr == nil {
			if cr.User != "" {
				fmt.Fprintf(env.Stdout, "  user      %s\n", cr.User)
			}
			if cr.Database != "" {
				fmt.Fprintf(env.Stdout, "  database  %s\n", cr.Database)
			}
			if *show {
				fmt.Fprintf(env.Stdout, "  password  %s\n", cr.Password)
			} else {
				fmt.Fprintln(env.Stdout, "  password  (add --show-password, or Show credentials on the database page)")
			}
			if cmd := clientCommand(d.Kind, local, cr); cmd != "" {
				fmt.Fprintf(env.Stdout, "  connect   %s\n", cmd)
			}
		}
		return ServeTunnel(ctx, ln, func(ctx context.Context) (*wsock.Conn, error) { return c.OpenTunnel(ctx, d.ID) }, env.Stderr)
	}
	return fmt.Errorf("unknown command: nectar db %s (list, connect)", sub)
}

func newFlagSet(name string, env Env) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	return fs
}

func newTable(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 2, 4, 2, ' ', 0) }
