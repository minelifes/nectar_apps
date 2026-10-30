package blueprint

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// docker-compose import: every service becomes an app with the same name, so the host names the services
// use for each other ("db", "redis") keep working inside the project. Services with published ports become
// web apps (the first published port is the app port and gets a domain), the others workers; databases and
// brokers stay internal even when the file publishes their port. Named volumes become app volumes. What
// has no equivalent is listed in the notes.

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string   `yaml:"image"`
	Build       any      `yaml:"build"`
	Command     any      `yaml:"command"`
	Entrypoint  any      `yaml:"entrypoint"`
	Environment any      `yaml:"environment"`
	EnvFile     any      `yaml:"env_file"`
	Ports       []any    `yaml:"ports"`
	Expose      []any    `yaml:"expose"`
	Volumes     []any    `yaml:"volumes"`
	Restart     string   `yaml:"restart"`
	Profiles    []string `yaml:"profiles"`
	Deploy      struct {
		Replicas  *int `yaml:"replicas"`
		Resources struct {
			Limits struct {
				CPUs   any    `yaml:"cpus"`
				Memory string `yaml:"memory"`
			} `yaml:"limits"`
		} `yaml:"resources"`
	} `yaml:"deploy"`
	NetworkMode string `yaml:"network_mode"`
	Privileged  bool   `yaml:"privileged"`
}

// internal services: their port is never published on a domain.
var internalImages = map[string]int{
	"postgres": 5432, "postgis/postgis": 5432, "mysql": 3306, "mariadb": 3306, "redis": 6379, "valkey/valkey": 6379,
	"mongo": 27017, "rabbitmq": 5672, "memcached": 11211, "bitnami/postgresql": 5432, "bitnami/redis": 6379,
	"bitnami/mysql": 3306, "bitnami/mariadb": 3306, "bitnami/mongodb": 27017, "nats": 4222, "eclipse-mosquitto": 1883,
}

// PlaceholderRegistry prefixes the image of services that are built from source (not deployed on import).
const PlaceholderRegistry = "registry.invalid/"

var nonSlug = regexp.MustCompile(`[^a-z0-9-]+`)

func composeSlug(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 30 {
		s = strings.TrimRight(s[:30], "-")
	}
	return s
}

func imageBase(image string) string {
	ref := image
	if i := strings.LastIndex(ref, "@"); i > 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	ref = strings.TrimPrefix(ref, "docker.io/")
	return strings.TrimPrefix(ref, "library/")
}

// FromCompose converts a docker-compose file. notes explain what was changed or left out.
func FromCompose(data []byte) (Blueprint, []string, error) {
	var f composeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Blueprint{}, nil, fmt.Errorf("not a docker-compose file: %s", firstLine(err.Error()))
	}
	if len(f.Services) == 0 {
		return Blueprint{}, nil, fmt.Errorf("no services in the file")
	}
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	var b Blueprint
	var notes []string
	note := func(format string, a ...any) { notes = append(notes, fmt.Sprintf(format, a...)) }
	used := map[string]bool{}
	for _, name := range names {
		svc := f.Services[name]
		slug := composeSlug(name)
		if slug == "" || used[slug] {
			return Blueprint{}, notes, fmt.Errorf("service %q: cannot make a unique app name of it", name)
		}
		used[slug] = true
		if slug != name {
			note("%s became app %q: other services must use %q as its host name", name, slug, slug)
		}
		if svc.Image == "" {
			if svc.Build != nil {
				note("%s is built from source (build:): push the image to a registry and set it, or connect the Git repository on the app page (Git builds)", name)
			} else {
				note("%s has no image and was left out", name)
				continue
			}
		}
		if len(svc.Profiles) > 0 {
			note("%s belongs to profiles %v: imported anyway", name, svc.Profiles)
		}
		if svc.Privileged || svc.NetworkMode == "host" {
			note("%s asks for privileged mode or the host network, which apps do not get", name)
		}
		app := App{Slug: slug, Name: name, Image: svc.Image, Runtime: "image"}
		if app.Image == "" {
			app.Image = PlaceholderRegistry + slug + ":latest" // placeholder, see the note
		}

		// command / entrypoint
		cmd := composeList(svc.Command)
		if ep := composeList(svc.Entrypoint); len(ep) > 0 {
			app.Command = append(ep, cmd...)
		} else if len(cmd) > 0 {
			app.Command = cmd
			note("%s: the command replaces the image's entrypoint here (Nectar runs it as the container command)", name)
		}

		// environment
		env := composeEnv(svc.Environment)
		if svc.EnvFile != nil {
			note("%s: env_file is not imported — add its variables on the Environment tab", name)
		}
		for k, v := range env {
			if strings.Contains(v, "${") && !strings.Contains(v, "$${") {
				note("%s: %s uses compose interpolation (%s) — set the value on the Environment tab", name, k, v)
			}
			if looksSecret(k) {
				if app.Secrets == nil {
					app.Secrets = map[string]string{}
				}
				app.Secrets[k] = v
				continue
			}
			if app.Env == nil {
				app.Env = map[string]string{}
			}
			app.Env[k] = v
		}

		// ports
		var published, exposed []int
		for _, p := range svc.Ports {
			if t := composePortTarget(p); t > 0 {
				published = append(published, t)
			}
		}
		for _, p := range svc.Expose {
			if t := composePortTarget(p); t > 0 {
				exposed = append(exposed, t)
			}
		}
		internalPort, isInternal := internalImages[imageBase(svc.Image)]
		switch {
		case isInternal:
			app.Kind = "worker"
			app.Port = internalPort
			if len(published) > 0 {
				note("%s (%s) is reachable by the project's apps only; its published port is not opened", name, imageBase(svc.Image))
			}
		case len(published) > 0:
			app.Kind = "web"
			app.Port = published[0]
			app.InternalPorts = dedupe(append(published[1:], exposed...), app.Port)
			if len(published) > 1 {
				note("%s publishes several ports: %d gets the domain, %v are reachable inside the project", name, app.Port, app.InternalPorts)
			}
		case len(exposed) > 0:
			app.Kind = "worker"
			app.Port = exposed[0]
			app.InternalPorts = dedupe(exposed[1:], app.Port)
		default:
			app.Kind = "worker"
		}

		// volumes
		for _, v := range svc.Volumes {
			src, target := composeVolume(v)
			switch {
			case target == "":
				continue
			case strings.HasPrefix(src, ".") || strings.HasPrefix(src, "/") || strings.HasPrefix(src, "~"):
				note("%s: the bind mount %s → %s is not imported — upload the files to a volume (Volumes → Files) or use config files", name, src, target)
				continue
			}
			vname := composeSlug(src)
			if vname == "" {
				vname = "data" + strconv.Itoa(len(app.Volumes)+1)
			}
			app.Volumes = append(app.Volumes, Volume{Name: vname, MountPath: target, SizeMB: 1024})
		}

		// replicas / resources
		if svc.Deploy.Replicas != nil {
			app.Replicas = svc.Deploy.Replicas
		}
		if m := composeMemory(svc.Deploy.Resources.Limits.Memory); m > 0 {
			app.MemoryMB = m
		}
		if svc.Restart == "no" {
			note("%s has restart: no — Nectar always restarts crashed apps", name)
		}
		b.Apps = append(b.Apps, app)
	}
	if len(b.Apps) == 0 {
		return b, notes, fmt.Errorf("nothing to import")
	}
	return b, notes, b.Validate()
}

func dedupe(ports []int, main int) []int {
	var out []int
	seen := map[int]bool{main: true}
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func composeList(v any) []string {
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		return []string{"sh", "-c", x}
	case []any:
		var out []string
		for _, s := range x {
			out = append(out, fmt.Sprint(s))
		}
		return out
	}
	return nil
}

func composeEnv(v any) map[string]string {
	out := map[string]string{}
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if val == nil {
				val = ""
			}
			out[k] = fmt.Sprint(val)
		}
	case []any:
		for _, it := range x {
			k, val, _ := strings.Cut(fmt.Sprint(it), "=")
			out[k] = val
		}
	}
	return out
}

func looksSecret(key string) bool {
	k := strings.ToUpper(key)
	for _, w := range []string{"PASSWORD", "PASSWD", "SECRET", "TOKEN", "API_KEY", "PRIVATE_KEY", "_KEY"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

// composePortTarget returns the container port of "8080:80", "127.0.0.1:8080:80/tcp", "80", 80 or
// {target: 80}.
func composePortTarget(p any) int {
	switch x := p.(type) {
	case int:
		return x
	case uint64:
		return int(x)
	case string:
		s := x
		if i := strings.Index(s, "/"); i > 0 {
			if strings.ToLower(s[i+1:]) == "udp" {
				return 0
			}
			s = s[:i]
		}
		parts := strings.Split(s, ":")
		last := parts[len(parts)-1]
		if i := strings.Index(last, "-"); i > 0 { // ranges: take the first
			last = last[:i]
		}
		n, _ := strconv.Atoi(last)
		return n
	case map[string]any:
		n, _ := strconv.Atoi(fmt.Sprint(x["target"]))
		return n
	}
	return 0
}

// composeVolume returns source and target of "name:/path[:ro]", "./dir:/path", "/path" or {source, target}.
func composeVolume(v any) (src, target string) {
	switch x := v.(type) {
	case string:
		parts := strings.Split(x, ":")
		switch len(parts) {
		case 1:
			return "", parts[0] // anonymous volume
		default:
			return parts[0], parts[1]
		}
	case map[string]any:
		if fmt.Sprint(x["type"]) == "bind" {
			return "./" + fmt.Sprint(x["source"]), fmt.Sprint(x["target"])
		}
		return fmt.Sprint(x["source"]), fmt.Sprint(x["target"])
	}
	return "", ""
}

// composeMemory parses "512m", "1g", "1024M" into MB.
func composeMemory(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	mult := 1.0 / (1 << 20)
	switch {
	case strings.HasSuffix(s, "g") || strings.HasSuffix(s, "gb"):
		mult, s = 1024, strings.TrimRight(s, "gb")
	case strings.HasSuffix(s, "m") || strings.HasSuffix(s, "mb"):
		mult, s = 1, strings.TrimRight(s, "mb")
	case strings.HasSuffix(s, "k") || strings.HasSuffix(s, "kb"):
		mult, s = 1.0/1024, strings.TrimRight(s, "kb")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * mult)
}
