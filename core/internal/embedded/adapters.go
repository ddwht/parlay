package embedded

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed adapters/*.adapter.yaml
var adaptersFS embed.FS

// ReadAdapter returns the content of a bundled adapter by name.
func ReadAdapter(name string) ([]byte, error) {
	filename := fmt.Sprintf("adapters/%s.adapter.yaml", name)
	return adaptersFS.ReadFile(filename)
}

// AdapterNames returns the slugs of every bundled adapter, sorted.
//
// There was no way to enumerate them, which is why `parlay init` hardcodes four
// slugs in its menu and never offers the three backend adapters that ship
// alongside them. Anything presenting "pick a starting template" — init, the
// create-adapter skill — needs this. Mirrors PresetNames.
func AdapterNames() ([]string, error) {
	entries, err := fs.ReadDir(adaptersFS, "adapters")
	if err != nil {
		return nil, fmt.Errorf("list adapters: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".adapter.yaml")
		if name == e.Name() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// AdapterKind reports a bundled adapter's declared kind, defaulting an absent
// one to presentation per adapter.schema.md Section 0 — so a caller can group
// the templates by layer without parsing the YAML itself.
func AdapterKind(name string) (string, error) {
	data, err := ReadAdapter(name)
	if err != nil {
		return "", err
	}
	var shape struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &shape); err != nil {
		return "", fmt.Errorf("parse adapter %s: %w", name, err)
	}
	if shape.Kind == "" {
		return "presentation", nil
	}
	return shape.Kind, nil
}
