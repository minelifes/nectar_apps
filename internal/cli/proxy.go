package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/tabwriter"
)

// Proxy is a domain without an app (GET /api/proxies).
type Proxy struct {
	ID           int64  `json:"id"`
	Host         string `json:"host"`
	PathPrefix   string `json:"path_prefix"`
	Kind         string `json:"kind"`
	TargetHost   string `json:"target_host"`
	TargetPort   int    `json:"target_port"`
	TargetHTTPS  bool   `json:"target_https"`
	RedirectTo   string `json:"redirect_to"`
	RedirectCode int    `json:"redirect_code"`
	TLS          bool   `json:"tls"`
	Status       string `json:"status"`
	Message      string `json:"status_message"`
}

func (p Proxy) where() string {
	if p.PathPrefix == "" || p.PathPrefix == "/" {
		return p.Host
	}
	return p.Host + p.PathPrefix
}

func (p Proxy) target() string {
	if p.Kind == "redirect" {
		return fmt.Sprintf("redirect %d → %s", p.RedirectCode, p.RedirectTo)
	}
	scheme := "http"
	if p.TargetHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, p.TargetHost, p.TargetPort)
}

const proxyUsage = `usage:
  nectar proxy list
  nectar proxy add HOST[/PATH] ADDRESS:PORT [--https-target] [--no-tls] [--strip]
  nectar proxy add HOST --redirect URL [--code 301|302|307|308]
  nectar proxy set HOST[/PATH] [ADDRESS:PORT] [--https-target=true|false] [--redirect URL] [--code N]
  nectar proxy rm HOST[/PATH]
ADDRESS is what the gateway reaches: 127.0.0.1 (the server itself), another machine's IP, or a host name.`

// proxyCommand: domains that go straight to an address:port (or redirect) without an app.
func proxyCommand(ctx context.Context, c *Client, args []string, env Env) error {
	if len(args) == 0 {
		return errors.New(proxyUsage)
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("nectar proxy "+sub, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	httpsTarget := fs.Bool("https-target", false, "the target speaks HTTPS (its certificate is not checked)")
	noTLS := fs.Bool("no-tls", false, "serve plain HTTP only (no certificate)")
	strip := fs.Bool("strip", false, "remove the path prefix before forwarding")
	redirect := fs.String("redirect", "", "redirect to this domain or URL instead of proxying")
	code := fs.Int("code", 0, "redirect status: 301, 302, 307 or 308")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	switch sub {
	case "list", "ls":
		var list []Proxy
		if err := c.getJSON(ctx, "/proxies", &list); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(env.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "DOMAIN\tTARGET\tHTTPS\tSTATUS")
		for _, p := range list {
			st := p.Status
			if p.Message != "" {
				st += " (" + p.Message + ")"
			}
			fmt.Fprintf(tw, "%s\t%s\t%v\t%s\n", p.where(), p.target(), p.TLS, st)
		}
		return tw.Flush()

	case "add":
		if len(pos) < 1 || (len(pos) < 2 && *redirect == "") {
			return errors.New(proxyUsage)
		}
		host, path := splitHostPath(pos[0])
		body := map[string]any{"host": host, "path_prefix": path, "tls": !*noTLS, "strip_prefix": *strip}
		if *redirect != "" {
			body["kind"], body["redirect_to"] = "redirect", *redirect
			if *code != 0 {
				body["redirect_code"] = *code
			}
		} else {
			addr, port, err := splitTarget(pos[1])
			if err != nil {
				return err
			}
			body["kind"], body["target_host"], body["target_port"], body["target_https"] = "proxy", addr, port, *httpsTarget
		}
		var p Proxy
		if err := c.postJSON(ctx, "/proxies", body, &p); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "✓ %s → %s (%s)\n", p.where(), p.target(), p.Status)
		return nil

	case "set":
		if len(pos) < 1 {
			return errors.New(proxyUsage)
		}
		p, err := findProxy(ctx, c, pos[0])
		if err != nil {
			return err
		}
		body := map[string]any{}
		if len(pos) > 1 {
			addr, port, err := splitTarget(pos[1])
			if err != nil {
				return err
			}
			body["target_host"], body["target_port"] = addr, port
		}
		if set["https-target"] {
			body["target_https"] = *httpsTarget
		}
		if set["no-tls"] {
			body["tls"] = !*noTLS
		}
		if set["strip"] {
			body["strip_prefix"] = *strip
		}
		if set["redirect"] {
			body["redirect_to"] = *redirect
		}
		if set["code"] {
			body["redirect_code"] = *code
		}
		if len(body) == 0 {
			return errors.New("nothing to change\n" + proxyUsage)
		}
		var out Proxy
		if err := c.do(ctx, http.MethodPatch, "/proxies/"+strconv.FormatInt(p.ID, 10), jsonBody(body), "application/json", &out); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "✓ %s → %s (%s)\n", out.where(), out.target(), out.Status)
		return nil

	case "rm", "remove", "delete":
		if len(pos) != 1 {
			return errors.New(proxyUsage)
		}
		p, err := findProxy(ctx, c, pos[0])
		if err != nil {
			return err
		}
		if err := c.do(ctx, http.MethodDelete, "/proxies/"+strconv.FormatInt(p.ID, 10), nil, "", nil); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "✓ removed %s\n", p.where())
		return nil
	}
	return errors.New(proxyUsage)
}

// splitHostPath: "nas.example.com/grafana" → host, "/grafana".
func splitHostPath(s string) (string, string) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	host, path, ok := strings.Cut(s, "/")
	if !ok {
		return strings.ToLower(host), "/"
	}
	return strings.ToLower(host), "/" + strings.TrimSuffix(path, "/")
}

// splitTarget: "127.0.0.1:9000" / "[::1]:80" / "grafana.svc:3000".
func splitTarget(s string) (string, int, error) {
	i := strings.LastIndex(s, ":")
	if i <= 0 {
		return "", 0, fmt.Errorf("%q: give ADDRESS:PORT, e.g. 127.0.0.1:9000", s)
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("%q: the port must be 1–65535", s)
	}
	return strings.Trim(s[:i], "[]"), port, nil
}

func findProxy(ctx context.Context, c *Client, ref string) (Proxy, error) {
	host, path := splitHostPath(ref)
	var list []Proxy
	if err := c.getJSON(ctx, "/proxies", &list); err != nil {
		return Proxy{}, err
	}
	for _, p := range list {
		pp := p.PathPrefix
		if pp == "" {
			pp = "/"
		}
		if p.Host == host && pp == path {
			return p, nil
		}
	}
	return Proxy{}, fmt.Errorf("no proxy %s (see `nectar proxy list`)", ref)
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}
