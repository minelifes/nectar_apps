// Package blueprint is the "nectar.yaml" format: the apps, databases, variables, volumes and domains of a
// project as data. It is what project export writes and `nectar apply` / the panel apply, what app templates
// (one-click apps) are made of, and what a docker-compose file is converted to.
//
//	apps:
//	  - slug: web
//	    image: ghcr.io/acme/web:1.4
//	    port: 8080
//	    env: { LOG_LEVEL: info, DATABASE_URL: "${db.DATABASE_URL}" }
//	    secrets: { API_KEY: "" }          # secret values are never exported; "" keeps the current one
//	    volumes: [{ name: data, mount_path: /data, size_mb: 1024 }]
//	    domains: [shop.example.com]
//	addons:
//	  - { slug: db, kind: postgres, version: "17", size_mb: 2048 }
//
// Applying never deletes anything: missing apps, add-ons, links and domains are created, existing apps get
// the settings and variables of the file, everything else stays.
package blueprint

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Version is the format version in the "version" field (optional).
const Version = 1

// Blueprint is a project's content.
type Blueprint struct {
	Version int    `json:"version,omitempty" yaml:"version,omitempty"`
	Project string `json:"project,omitempty" yaml:"project,omitempty"` // informational (export)
	// EnvGroups are variables shared by several apps of the project (apps list them in env_groups).
	EnvGroups []EnvGroup `json:"env_groups,omitempty" yaml:"env_groups,omitempty"`
	Addons    []Addon    `json:"addons,omitempty" yaml:"addons,omitempty"`
	Apps      []App      `json:"apps,omitempty" yaml:"apps,omitempty"`
}

// EnvGroup is a named set of variables.
type EnvGroup struct {
	Name    string            `json:"name" yaml:"name"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Secrets map[string]string `json:"secrets,omitempty" yaml:"secrets,omitempty"`
}

// Addon is a database.
type Addon struct {
	Slug     string `json:"slug" yaml:"slug"`
	Name     string `json:"name,omitempty" yaml:"name,omitempty"`
	Kind     string `json:"kind" yaml:"kind"` // postgres | mysql | mariadb | redis
	Version  string `json:"version,omitempty" yaml:"version,omitempty"`
	SizeMB   int64  `json:"size_mb,omitempty" yaml:"size_mb,omitempty"`
	MemoryMB int64  `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
}

// App is an app. Only image apps can be created from a blueprint without an upload; other runtimes are
// created and wait for their first artifact.
type App struct {
	Slug           string            `json:"slug" yaml:"slug"`
	Name           string            `json:"name,omitempty" yaml:"name,omitempty"`
	Kind           string            `json:"kind,omitempty" yaml:"kind,omitempty"`       // web (default) | worker | cron | task
	Runtime        string            `json:"runtime,omitempty" yaml:"runtime,omitempty"` // image (default), java, node, …
	RuntimeVersion string            `json:"runtime_version,omitempty" yaml:"runtime_version,omitempty"`
	Image          string            `json:"image,omitempty" yaml:"image,omitempty"`
	Port           int               `json:"port,omitempty" yaml:"port,omitempty"`
	InternalPorts  []int             `json:"internal_ports,omitempty" yaml:"internal_ports,omitempty"` // reachable by the project's apps only
	Command        []string          `json:"command,omitempty" yaml:"command,omitempty"`
	Args           string            `json:"args,omitempty" yaml:"args,omitempty"`
	Replicas       *int              `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	CPUMillicores  int64             `json:"cpu_millicores,omitempty" yaml:"cpu_millicores,omitempty"`
	MemoryMB       int64             `json:"memory_mb,omitempty" yaml:"memory_mb,omitempty"`
	HealthPath     string            `json:"health_path,omitempty" yaml:"health_path,omitempty"` // HTTP check; "" = TCP
	Schedule       string            `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	ReleaseCommand string            `json:"release_command,omitempty" yaml:"release_command,omitempty"`
	Env            map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Secrets        map[string]string `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	Volumes        []Volume          `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	Expose         []Port            `json:"expose,omitempty" yaml:"expose,omitempty"`
	Domains        []string          `json:"domains,omitempty" yaml:"domains,omitempty"`
	// Links connects databases: their connection variables (DATABASE_URL, PGHOST, …) are set on the app.
	Links []Link `json:"links,omitempty" yaml:"links,omitempty"`
	// EnvGroups attaches project env groups (all their variables).
	EnvGroups []string `json:"env_groups,omitempty" yaml:"env_groups,omitempty"`
}

// Volume is a persistent disk.
type Volume struct {
	Name         string `json:"name" yaml:"name"`
	MountPath    string `json:"mount_path" yaml:"mount_path"`
	SizeMB       int64  `json:"size_mb,omitempty" yaml:"size_mb,omitempty"`
	StorageClass string `json:"storage_class,omitempty" yaml:"storage_class,omitempty"`
}

// Port is an extra TCP/UDP port opened on the server.
type Port struct {
	Port     int    `json:"port" yaml:"port"`
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"` // tcp (default) | udp
}

// Link connects a database to the app.
type Link struct {
	Addon  string `json:"addon" yaml:"addon"`
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"` // e.g. CACHE_ → CACHE_REDIS_URL
}

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$`)
	envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
)

// Parse reads YAML or JSON (JSON is YAML) and validates the structure.
func Parse(data []byte) (Blueprint, error) {
	var b Blueprint
	if len(bytes.TrimSpace(data)) == 0 {
		return b, errors.New("the file is empty")
	}
	if err := yaml.UnmarshalWithOptions(data, &b, yaml.Strict()); err != nil {
		return b, fmt.Errorf("not a nectar.yaml: %s", firstLine(err.Error()))
	}
	return b, b.Validate()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// Validate checks the structure (the panel checks the details again when it creates each part).
func (b *Blueprint) Validate() error {
	if b.Version > Version {
		return fmt.Errorf("version %d is newer than this panel understands (%d)", b.Version, Version)
	}
	if len(b.Apps)+len(b.Addons)+len(b.EnvGroups) == 0 {
		return errors.New("no apps or add-ons")
	}
	if len(b.Apps) > 50 || len(b.Addons) > 20 {
		return errors.New("at most 50 apps and 20 add-ons")
	}
	groups := map[string]bool{}
	for _, g := range b.EnvGroups {
		if !slugRe.MatchString(g.Name) || groups[g.Name] {
			return fmt.Errorf("env group %q: names are 1–30 lowercase letters, digits or dashes, each once", g.Name)
		}
		groups[g.Name] = true
		for k := range g.Env {
			if !envKeyRe.MatchString(k) {
				return fmt.Errorf("env group %s: invalid variable name %q", g.Name, k)
			}
		}
		for k := range g.Secrets {
			if !envKeyRe.MatchString(k) {
				return fmt.Errorf("env group %s: invalid variable name %q", g.Name, k)
			}
		}
	}
	seen := map[string]string{}
	for i := range b.Addons {
		a := &b.Addons[i]
		a.Slug = strings.ToLower(strings.TrimSpace(a.Slug))
		if !slugRe.MatchString(a.Slug) {
			return fmt.Errorf("add-on %q: slug must be 1–30 lowercase letters, digits or dashes", a.Slug)
		}
		if seen[a.Slug] != "" {
			return fmt.Errorf("%q is used twice", a.Slug)
		}
		seen[a.Slug] = "addon"
		if !slices.Contains([]string{"postgres", "mysql", "mariadb", "redis"}, a.Kind) {
			return fmt.Errorf("add-on %s: kind must be postgres, mysql, mariadb or redis", a.Slug)
		}
		if a.Name == "" {
			a.Name = a.Slug
		}
	}
	for i := range b.Apps {
		a := &b.Apps[i]
		a.Slug = strings.ToLower(strings.TrimSpace(a.Slug))
		if !slugRe.MatchString(a.Slug) {
			return fmt.Errorf("app %q: slug must be 1–30 lowercase letters, digits or dashes", a.Slug)
		}
		if seen[a.Slug] != "" {
			return fmt.Errorf("%q is used twice", a.Slug)
		}
		seen[a.Slug] = "app"
		if a.Name == "" {
			a.Name = a.Slug
		}
		if a.Runtime == "" {
			a.Runtime = "image"
		}
		if a.Kind == "" {
			a.Kind = "web"
		}
		if a.Runtime == "image" && a.Image == "" {
			return fmt.Errorf("app %s: image is required", a.Slug)
		}
		for k := range a.Env {
			if !envKeyRe.MatchString(k) {
				return fmt.Errorf("app %s: invalid variable name %q", a.Slug, k)
			}
		}
		for k := range a.Secrets {
			if !envKeyRe.MatchString(k) {
				return fmt.Errorf("app %s: invalid variable name %q", a.Slug, k)
			}
			if _, dup := a.Env[k]; dup {
				return fmt.Errorf("app %s: %s is both in env and secrets", a.Slug, k)
			}
		}
	}
	for _, a := range b.Apps {
		for _, l := range a.Links {
			if seen[l.Addon] == "app" {
				return fmt.Errorf("app %s links %s, which is an app, not a database", a.Slug, l.Addon)
			}
		}
	}
	return nil
}

// Marshal writes the blueprint as YAML.
func (b Blueprint) Marshal() ([]byte, error) {
	if b.Version == 0 {
		b.Version = Version
	}
	return yaml.MarshalWithOptions(b, yaml.IndentSequence(true))
}

// SortedKeys returns map keys in order (stable output).
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
