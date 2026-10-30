package blueprint

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

//go:embed templates.yaml
var templatesYAML []byte

// Template is a one-click app: a blueprint with placeholders.
type Template struct {
	ID          string  `json:"id" yaml:"id"`
	Name        string  `json:"name" yaml:"name"`
	Category    string  `json:"category" yaml:"category"`
	Icon        string  `json:"icon" yaml:"icon"`
	Description string  `json:"description" yaml:"description"`
	Docs        string  `json:"docs,omitempty" yaml:"docs"`
	Notes       string  `json:"notes,omitempty" yaml:"notes"`
	Inputs      []Input `json:"inputs,omitempty" yaml:"inputs"`
	// Blueprint is kept as YAML: placeholders are replaced in the text before it is parsed.
	Blueprint yaml.MapSlice `json:"-" yaml:"blueprint"`
	// Images lists the images of its apps (shown before installing).
	Images []string `json:"images" yaml:"-"`
	// Addons lists the database kinds it creates.
	Addons []string `json:"addons" yaml:"-"`
}

// Input is a value the user enters when installing.
type Input struct {
	Key      string `json:"key" yaml:"key"`
	Label    string `json:"label" yaml:"label"`
	Default  string `json:"default,omitempty" yaml:"default"`
	Required bool   `json:"required,omitempty" yaml:"required"`
	Secret   bool   `json:"secret,omitempty" yaml:"secret"`
}

// Templates returns the catalog.
func Templates() ([]Template, error) {
	var doc struct {
		Templates []Template `yaml:"templates"`
	}
	if err := yaml.Unmarshal(templatesYAML, &doc); err != nil {
		return nil, err
	}
	for i := range doc.Templates {
		t := &doc.Templates[i]
		b, err := t.Render("x", nil)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", t.ID, err)
		}
		for _, a := range b.Apps {
			t.Images = append(t.Images, a.Image)
		}
		for _, a := range b.Addons {
			t.Addons = append(t.Addons, a.Kind)
		}
	}
	return doc.Templates, nil
}

// TemplateByID returns one template.
func TemplateByID(id string) (Template, bool) {
	ts, err := Templates()
	if err != nil {
		return Template{}, false
	}
	for _, t := range ts {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

var placeholderRe = regexp.MustCompile(`\[\[(name|input\.[a-z0-9_]+|random:[a-z0-9_]+)\]\]`)

// Render fills in the placeholders and returns the blueprint. name is the installation's name (the slug
// of its main app); inputs are the user's values (missing ones use the defaults).
func (t Template) Render(name string, inputs map[string]string) (Blueprint, error) {
	raw, err := yaml.Marshal(t.Blueprint)
	if err != nil {
		return Blueprint{}, err
	}
	values := map[string]string{}
	for _, in := range t.Inputs {
		v := strings.TrimSpace(inputs[in.Key])
		if v == "" {
			v = in.Default
		}
		if v == "" && in.Required {
			return Blueprint{}, fmt.Errorf("%s is required", in.Label)
		}
		if strings.ContainsAny(v, "\n\r\"\\") {
			return Blueprint{}, fmt.Errorf("%s: quotes, backslashes and line breaks are not allowed", in.Label)
		}
		values["input."+in.Key] = v
	}
	var missing []string
	text := placeholderRe.ReplaceAllStringFunc(string(raw), func(m string) string {
		key := m[2 : len(m)-2]
		switch {
		case key == "name":
			return name
		case strings.HasPrefix(key, "random:"):
			if v, ok := values[key]; ok {
				return v
			}
			values[key] = randomToken()
			return values[key]
		}
		v, ok := values[key]
		if !ok {
			missing = append(missing, key)
		}
		return v
	})
	if len(missing) > 0 {
		return Blueprint{}, fmt.Errorf("unknown placeholders: %s", strings.Join(missing, ", "))
	}
	return Parse([]byte(text))
}

func randomToken() string {
	var b [18]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:]) // 24 characters, safe in URLs and YAML
}
