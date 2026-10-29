// Package runtimes loads the runtime catalog (runtimes.yaml): the app types offered by the wizard
// and how each one is turned into a container.
package runtimes

import (
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

//go:embed runtimes.yaml
var builtin []byte

// Artifact kinds.
const (
	ArtifactFile    = "file"
	ArtifactArchive = "archive"
	ArtifactNone    = "none"
)

// App kinds.
const (
	KindWeb    = "web"
	KindWorker = "worker"
	KindCron   = "cron"
	KindTask   = "task"
)

// Runtime describes one app type.
type Runtime struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description" yaml:"description"`
	Icon        string            `json:"icon" yaml:"icon"`
	Kinds       []string          `json:"kinds" yaml:"kinds"`
	Artifact    Artifact          `json:"artifact" yaml:"artifact"`
	Versions    []Version         `json:"versions" yaml:"versions"`
	Command     []string          `json:"command,omitempty" yaml:"command"`
	Args        []string          `json:"args,omitempty" yaml:"args"`
	Env         map[string]string `json:"env,omitempty" yaml:"env"`
	Port        int               `json:"port" yaml:"port"`
	Health      Health            `json:"health" yaml:"health"`
	RunAsUser   *int64            `json:"run_as_user,omitempty" yaml:"run_as_user"`
	Options     map[string]Option `json:"options,omitempty" yaml:"options"`
	ConfigFiles []ConfigFile      `json:"-" yaml:"config_files"`
}

// Artifact describes what the user uploads and where it goes.
type Artifact struct {
	Kind   string   `json:"kind" yaml:"kind"`
	Accept []string `json:"accept,omitempty" yaml:"accept"`
	Path   string   `json:"path,omitempty" yaml:"path"`
	Mode   string   `json:"mode,omitempty" yaml:"mode"`
}

// Version maps a selectable version to a container image.
type Version struct {
	ID      string `json:"id" yaml:"id"`
	Label   string `json:"label" yaml:"label"`
	Image   string `json:"image" yaml:"image"`
	Default bool   `json:"default,omitempty" yaml:"default"`
}

// Health is the default health check.
type Health struct {
	Type string `json:"type" yaml:"type"` // tcp | http | none
	Path string `json:"path,omitempty" yaml:"path"`
}

// Option is a runtime-specific setting shown in the wizard.
type Option struct {
	Type    string `json:"type" yaml:"type"` // bool | string
	Default any    `json:"default" yaml:"default"`
	Label   string `json:"label" yaml:"label"`
}

// ConfigFile is a templated file mounted into the container.
type ConfigFile struct {
	Path     string `yaml:"path"`
	Template string `yaml:"template"`
}

// Catalog is the loaded set of runtimes.
type Catalog struct {
	Runtimes []Runtime `json:"runtimes" yaml:"runtimes"`
}

// LoadBuiltin parses the embedded catalog.
func LoadBuiltin() (*Catalog, error) { return Parse(builtin) }

// Parse parses and validates a catalog.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("runtimes: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Catalog) validate() error {
	if len(c.Runtimes) == 0 {
		return errors.New("runtimes: catalog is empty")
	}
	seen := map[string]bool{}
	for _, r := range c.Runtimes {
		if r.ID == "" || seen[r.ID] {
			return fmt.Errorf("runtimes: missing or duplicate id %q", r.ID)
		}
		seen[r.ID] = true
		if len(r.Kinds) == 0 {
			return fmt.Errorf("runtimes: %s has no kinds", r.ID)
		}
		for _, k := range r.Kinds {
			if !slices.Contains([]string{KindWeb, KindWorker, KindCron, KindTask}, k) {
				return fmt.Errorf("runtimes: %s has unknown kind %q", r.ID, k)
			}
		}
		switch r.Artifact.Kind {
		case ArtifactFile, ArtifactArchive:
			if !strings.HasPrefix(r.Artifact.Path, "/") {
				return fmt.Errorf("runtimes: %s artifact.path must be absolute", r.ID)
			}
			if len(r.Versions) == 0 {
				return fmt.Errorf("runtimes: %s needs at least one version", r.ID)
			}
		case ArtifactNone:
		default:
			return fmt.Errorf("runtimes: %s has unknown artifact kind %q", r.ID, r.Artifact.Kind)
		}
		defaults := 0
		for _, v := range r.Versions {
			if v.ID == "" || v.Image == "" {
				return fmt.Errorf("runtimes: %s has a version without id or image", r.ID)
			}
			if v.Default {
				defaults++
			}
		}
		if defaults > 1 {
			return fmt.Errorf("runtimes: %s has more than one default version", r.ID)
		}
		if r.Port <= 0 || r.Port > 65535 {
			return fmt.Errorf("runtimes: %s has invalid port %d", r.ID, r.Port)
		}
		if !slices.Contains([]string{"tcp", "http", "none"}, r.Health.Type) {
			return fmt.Errorf("runtimes: %s has invalid health type %q", r.ID, r.Health.Type)
		}
	}
	return nil
}

// Get returns a runtime by id.
func (c *Catalog) Get(id string) (Runtime, bool) {
	for _, r := range c.Runtimes {
		if r.ID == id {
			return r, true
		}
	}
	return Runtime{}, false
}

// Version returns the version with the given id, or the default (first if none marked) when id is "".
func (r Runtime) Version(id string) (Version, bool) {
	if len(r.Versions) == 0 {
		return Version{}, false
	}
	if id == "" {
		for _, v := range r.Versions {
			if v.Default {
				return v, true
			}
		}
		return r.Versions[0], true
	}
	for _, v := range r.Versions {
		if v.ID == id {
			return v, true
		}
	}
	return Version{}, false
}

// Supports reports whether the runtime supports an app kind.
func (r Runtime) Supports(kind string) bool { return slices.Contains(r.Kinds, kind) }

// OptionDefaults returns the default value of every option.
func (r Runtime) OptionDefaults() map[string]any {
	out := map[string]any{}
	for k, o := range r.Options {
		out[k] = o.Default
	}
	return out
}
