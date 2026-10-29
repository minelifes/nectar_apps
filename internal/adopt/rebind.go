package adopt

import (
	"fmt"
	"regexp"
	"strings"
)

// Internal ports a host proxy is moved to during the cutover (it keeps running behind the gateway).
const (
	RebindHTTPPort  = 10080
	RebindHTTPSPort = 10443
)

var (
	ngListenRe   = regexp.MustCompile(`(^|[\s{;])(listen[ \t]+)([^\s;]+)([^;]*;)`)
	apListenRe   = regexp.MustCompile(`(?mi)^(\s*Listen\s+)(\S+)(.*)$`)
	apVhostRe    = regexp.MustCompile(`(?mi)^(\s*<VirtualHost\s+)([^>]+)(>)`)
	hapBindRe    = regexp.MustCompile(`(?m)^(\s*bind\s+)(\S+)(.*)$`)
	caddySiteRe  = regexp.MustCompile(`(?m)^([^\s#{}][^{}\n]*?)\s*\{\s*$`)
	caddyPortSuf = regexp.MustCompile(`:(80|443)(\b|,|$)`)
)

// rebindAddr maps an edge listen address to its internal replacement ("" = not an edge address).
// IPv4/wildcard → 127.0.0.1:<internal>, IPv6 → [::1]:<internal>.
func rebindAddr(addr string, ipv6Brackets bool) string {
	host, port := "", addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host, port = addr[:i], addr[i+1:]
	}
	var internal int
	switch port {
	case "80":
		internal = RebindHTTPPort
	case "443":
		internal = RebindHTTPSPort
	default:
		return ""
	}
	switch host {
	case "127.0.0.1", "[::1]", "localhost", "::1":
		return "" // already internal
	}
	if strings.Contains(host, ":") || strings.HasPrefix(host, "[") {
		if ipv6Brackets {
			return fmt.Sprintf("[::1]:%d", internal)
		}
		return fmt.Sprintf("::1:%d", internal)
	}
	return fmt.Sprintf("127.0.0.1:%d", internal)
}

// RebindConfig rewrites a host proxy configuration file so it listens on the internal ports
// (127.0.0.1:10080 / :10443) instead of 80/443. It returns the new content and whether it changed.
func RebindConfig(kind, content string) (string, bool) {
	out := content
	switch kind {
	case "nginx":
		out = ngListenRe.ReplaceAllStringFunc(content, func(m string) string {
			p := ngListenRe.FindStringSubmatch(m)
			if na := rebindAddr(p[3], true); na != "" {
				return p[1] + p[2] + na + p[4]
			}
			return m
		})
	case "apache":
		out = apListenRe.ReplaceAllStringFunc(content, func(m string) string {
			p := apListenRe.FindStringSubmatch(m)
			if na := rebindAddr(p[2], true); na != "" {
				return p[1] + na + p[3]
			}
			return m
		})
		out = apVhostRe.ReplaceAllStringFunc(out, func(m string) string {
			p := apVhostRe.FindStringSubmatch(m)
			var addrs []string
			for _, a := range strings.Fields(p[2]) {
				switch {
				case strings.HasSuffix(a, ":80"):
					a = strings.TrimSuffix(a, ":80") + fmt.Sprintf(":%d", RebindHTTPPort)
				case strings.HasSuffix(a, ":443"):
					a = strings.TrimSuffix(a, ":443") + fmt.Sprintf(":%d", RebindHTTPSPort)
				}
				addrs = append(addrs, a)
			}
			return p[1] + strings.Join(addrs, " ") + p[3]
		})
	case "haproxy":
		out = hapBindRe.ReplaceAllStringFunc(content, func(m string) string {
			p := hapBindRe.FindStringSubmatch(m)
			addr := p[2]
			if strings.HasPrefix(addr, ":::") {
				addr = "[::]:" + strings.TrimPrefix(addr, ":::")
			}
			if strings.HasPrefix(addr, "*:") {
				addr = "0.0.0.0:" + strings.TrimPrefix(addr, "*:")
			}
			if strings.HasPrefix(addr, ":") {
				addr = "0.0.0.0" + addr
			}
			if na := rebindAddr(addr, false); na != "" {
				rest := p[3]
				if strings.Contains(rest, "v4v6") {
					// a dual-stack bind also served IPv4: the gateway's fallback connects to 127.0.0.1
					na = "127.0.0.1" + na[strings.LastIndex(na, ":"):]
					rest = strings.ReplaceAll(rest, " v4v6", "")
				}
				return p[1] + na + rest
			}
			return m
		})
	case "caddy":
		out = caddySiteRe.ReplaceAllStringFunc(content, func(m string) string {
			return caddyPortSuf.ReplaceAllStringFunc(m, func(s string) string {
				if strings.HasPrefix(s, ":80") {
					return fmt.Sprintf(":%d", RebindHTTPPort) + strings.TrimPrefix(s, ":80")
				}
				return fmt.Sprintf(":%d", RebindHTTPSPort) + strings.TrimPrefix(s, ":443")
			})
		})
		global := fmt.Sprintf("\thttp_port %d\n\thttps_port %d\n\tdefault_bind 127.0.0.1 [::1]\n", RebindHTTPPort, RebindHTTPSPort)
		if strings.Contains(out, "http_port "+fmt.Sprint(RebindHTTPPort)) {
			break
		}
		trimmed := strings.TrimLeft(out, " \t\r\n")
		if strings.HasPrefix(trimmed, "{") {
			// existing global options block: add our options at its start
			i := strings.Index(out, "{")
			out = out[:i+1] + "\n" + global + out[i+1:]
		} else {
			out = "{\n" + global + "}\n\n" + out
		}
	}
	return out, out != content
}

// validateCmd returns the command that checks a host proxy configuration.
func validateCmd(kind, config string) []string {
	switch kind {
	case "nginx":
		return []string{"nginx", "-t", "-c", config}
	case "apache":
		return []string{"apachectl", "-t"}
	case "haproxy":
		return []string{"haproxy", "-c", "-f", config}
	case "caddy":
		return []string{"caddy", "validate", "--config", config, "--adapter", "caddyfile"}
	}
	return nil
}
