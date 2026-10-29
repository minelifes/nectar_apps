package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Env is where a command reads and writes (tests replace it).
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Client overrides the client built from the config (tests).
	Client  *Client
	Version string
	Poll    time.Duration // release polling, default 2 s
}

const usage = `nectar — deploy to a Nectar Apps panel

Usage:
  nectar login --url https://panel.example.com [--token nat_…]   save the panel and an API token
  nectar whoami                                                   who the token belongs to
  nectar apps [--project SLUG]                                    list apps
  nectar deploy APP [--file app.jar | --image REF] [--no-wait] [--timeout 15m] [--force]
  nectar status APP                                               status, replicas and the live release
  nectar releases APP                                             release history
  nectar rollback APP [--to N]                                    redeploy release N (default: the previous one)
  nectar logs APP [--tail 200]                                    the latest log lines
  nectar version

APP is the app id or project/app (e.g. shop/api). NECTAR_URL and NECTAR_TOKEN override the saved login,
which is what CI uses. Create tokens in the panel: Settings → API tokens (a Deployer token per project
is enough to deploy).
`

// Run executes a command line; it returns the process exit code.
func Run(ctx context.Context, args []string, env Env) int {
	if env.Stdout == nil {
		env.Stdout = os.Stdout
	}
	if env.Stderr == nil {
		env.Stderr = os.Stderr
	}
	if env.Stdin == nil {
		env.Stdin = os.Stdin
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(env.Stdout, usage)
		return 0
	}
	err := run(ctx, args[0], args[1:], env)
	if errors.Is(err, flag.ErrHelp) {
		return 2
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "error:", err)
		return 1
	}
	return 0
}

func (env Env) client() (*Client, error) {
	if env.Client != nil {
		return env.Client, nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return &Client{URL: cfg.URL, Token: cfg.Token}, nil
}

func (env Env) poll() time.Duration {
	if env.Poll > 0 {
		return env.Poll
	}
	return 2 * time.Second
}

// parse handles flags that may come after positional arguments (nectar deploy shop/api --file x.jar).
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func run(ctx context.Context, cmd string, args []string, env Env) error {
	fs := flag.NewFlagSet("nectar "+cmd, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	switch cmd {
	case "version":
		fmt.Fprintln(env.Stdout, "nectar", env.Version)
		return nil

	case "login":
		u := fs.String("url", "", "panel address, e.g. https://panel.apps.example.com")
		tok := fs.String("token", "", "API token (nat_…); read from stdin when empty")
		if _, err := parse(fs, args); err != nil {
			return err
		}
		if *u == "" {
			return errors.New("--url is required")
		}
		if *tok == "" {
			fmt.Fprint(env.Stderr, "API token (Settings → API tokens): ")
			line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
			*tok = strings.TrimSpace(line)
		}
		c := &Client{URL: strings.TrimRight(strings.TrimSpace(*u), "/"), Token: *tok}
		if env.Client != nil {
			c.HTTP = env.Client.HTTP
		}
		me, err := c.Me(ctx)
		if err != nil {
			return err
		}
		path, err := SaveConfig(Config{URL: c.URL, Token: c.Token})
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "Logged in to %s as %s (saved to %s)\n", c.URL, me.Email, path)
		return nil
	}

	c, err := env.client()
	if err != nil {
		return err
	}
	switch cmd {
	case "whoami":
		me, err := c.Me(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "%s (%s) on %s\n", me.Email, me.Name, c.URL)
		return nil

	case "apps":
		project := fs.String("project", "", "only this project (slug)")
		asJSON := fs.Bool("json", false, "JSON output")
		if _, err := parse(fs, args); err != nil {
			return err
		}
		ps, err := c.Projects(ctx)
		if err != nil {
			return err
		}
		type row struct {
			App
			Project string `json:"project"`
		}
		var rows []row
		for _, p := range ps {
			if *project != "" && p.Slug != *project {
				continue
			}
			as, err := c.Apps(ctx, p.ID)
			if err != nil {
				var ae *APIError
				if errors.As(err, &ae) && (ae.Status == 403 || ae.Status == 404) {
					continue // a project the token sees but whose apps it may not list
				}
				return err
			}
			for _, a := range as {
				rows = append(rows, row{a, p.Slug})
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Project+"/"+rows[i].Slug < rows[j].Project+"/"+rows[j].Slug })
		if *asJSON {
			return json.NewEncoder(env.Stdout).Encode(rows)
		}
		tw := tabwriter.NewWriter(env.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tAPP\tRUNTIME\tSTATUS")
		for _, r := range rows {
			fmt.Fprintf(tw, "%d\t%s/%s\t%s\t%s\n", r.ID, r.Project, r.Slug, r.Runtime, r.Status)
		}
		return tw.Flush()

	case "deploy":
		file := fs.String("file", "", "artifact to upload (JAR, binary, zip, tarball …)")
		image := fs.String("image", "", "container image for image apps, e.g. ghcr.io/acme/api:1.4.2")
		noWait := fs.Bool("no-wait", false, "return after starting the release")
		timeout := fs.Duration("timeout", 15*time.Minute, "how long to wait for the release")
		force := fs.Bool("force", false, "deploy even when the capacity check says the server is full")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: nectar deploy APP [--file F | --image REF]")
		}
		if *file != "" && *image != "" {
			return errors.New("use --file or --image, not both")
		}
		app, err := c.ResolveApp(ctx, pos[0])
		if err != nil {
			return err
		}
		req := DeployRequest{Force: *force}
		if *file != "" {
			st, err := os.Stat(*file)
			if err != nil {
				return err
			}
			fmt.Fprintf(env.Stdout, "Uploading %s (%s) to %s …\n", *file, humanSize(st.Size()), app.Name)
			art, err := c.Upload(ctx, app.ID, *file)
			if err != nil {
				return fmt.Errorf("upload: %w", err)
			}
			for _, w := range art.Detect.Warnings {
				fmt.Fprintln(env.Stderr, "warning:", w)
			}
			req.ArtifactID = &art.ID
		}
		if *image != "" {
			req.Image = image
		}
		rel, err := c.Deploy(ctx, app.ID, req)
		if err != nil {
			return fmt.Errorf("deploy: %w", err)
		}
		fmt.Fprintf(env.Stdout, "Release #%d of %s started\n", rel.Number, app.Name)
		if *noWait {
			return nil
		}
		return wait(ctx, c, app, rel.Number, *timeout, env)

	case "rollback":
		to := fs.Int("to", 0, "release number (default: the one before the live release)")
		timeout := fs.Duration("timeout", 15*time.Minute, "how long to wait")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: nectar rollback APP [--to N]")
		}
		app, err := c.ResolveApp(ctx, pos[0])
		if err != nil {
			return err
		}
		n := *to
		if n == 0 {
			rs, err := c.Releases(ctx, app.ID)
			if err != nil {
				return err
			}
			if n = previousRelease(rs); n == 0 {
				return errors.New("there is no earlier release to go back to")
			}
		}
		rel, err := c.Rollback(ctx, app.ID, n)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "Rolling back %s to release #%d (new release #%d)\n", app.Name, n, rel.Number)
		return wait(ctx, c, app, rel.Number, *timeout, env)

	case "status":
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: nectar status APP")
		}
		app, err := c.ResolveApp(ctx, pos[0])
		if err != nil {
			return err
		}
		st, err := c.Status(ctx, app.ID)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "%s: %v", app.Name, st["status"])
		if m, _ := st["message"].(string); m != "" {
			fmt.Fprintf(env.Stdout, " — %s", m)
		}
		fmt.Fprintln(env.Stdout)
		if live, ok := st["live"].(map[string]any); ok {
			fmt.Fprintf(env.Stdout, "replicas: %v ready of %v\n", live["ready"], live["replicas"])
		}
		if p, _ := st["pending"].(bool); p {
			fmt.Fprintln(env.Stdout, "settings changed since the live release: deploy to apply them")
		}
		return nil

	case "releases":
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: nectar releases APP")
		}
		app, err := c.ResolveApp(ctx, pos[0])
		if err != nil {
			return err
		}
		rs, err := c.Releases(ctx, app.ID)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(env.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "#\tSTATUS\tCREATED\tMESSAGE")
		for _, r := range rs {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", r.Number, r.Status, r.CreatedAt.Local().Format("2006-01-02 15:04"), firstLine(r.Message))
		}
		return tw.Flush()

	case "logs":
		tail := fs.Int("tail", 200, "number of lines")
		pos, err := parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: nectar logs APP [--tail N]")
		}
		app, err := c.ResolveApp(ctx, pos[0])
		if err != nil {
			return err
		}
		return c.Logs(ctx, app.ID, *tail, env.Stdout)
	}
	fmt.Fprint(env.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

func wait(ctx context.Context, c *Client, app App, n int, timeout time.Duration, env Env) error {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	rel, err := c.WaitRelease(wctx, app.ID, n, env.poll(), func(r Release) {
		fmt.Fprintf(env.Stdout, "  %s  release #%d: %s\n", time.Since(start).Round(time.Second), r.Number, r.Status)
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "Release #%d of %s is live\n", rel.Number, app.Name)
	return nil
}

// previousRelease is the newest release before the live one that went live once.
func previousRelease(rs []Release) int {
	live := 0
	for _, r := range rs {
		if r.Status == "live" && r.Number > live {
			live = r.Number
		}
	}
	best := 0
	for _, r := range rs {
		if r.Number < live && r.Status == "superseded" && r.Number > best {
			best = r.Number
		}
	}
	return best
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 80 {
		s = s[:77] + "…"
	}
	return s
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
