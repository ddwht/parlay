// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-kind-discriminator
//
// Parser for .parlay/adapter-set.yaml — the file that pins which adapter
// occupies each adapter-kind slot in a project, declares per-target source
// roots, and authorizes cross-kind link relations.

package parser

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AdapterSet is the parsed shape of .parlay/adapter-set.yaml.
type AdapterSet struct {
	Path    string                      `yaml:"-"`
	Name    string                      `yaml:"name"`
	Targets map[string]AdapterSetTarget `yaml:"targets"`
	Links   []AdapterSetLink            `yaml:"links,omitempty"`
}

// AdapterSetTarget pins one slot of the adapter-kind topology to a specific
// adapter slug + the source root it emits into.
type AdapterSetTarget struct {
	Adapter string `yaml:"adapter"`
	Root    string `yaml:"root"`
}

// AdapterSetLink authorizes a cross-kind edge between two slots. Closed set
// of relations: calls, dispatches, persists.
type AdapterSetLink struct {
	From     string `yaml:"from"`
	Relation string `yaml:"relation"`
	To       string `yaml:"to"`
}

// ParseAdapterSet reads adapter-set.yaml from disk and parses it.
func ParseAdapterSet(path string) (*AdapterSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read adapter-set %s: %w", path, err)
	}
	return ParseAdapterSetBytes(path, data)
}

// ParseAdapterSetBytes parses adapter-set YAML content already in memory.
// The Path field on the returned struct is populated from the supplied path
// argument.
func ParseAdapterSetBytes(path string, content []byte) (*AdapterSet, error) {
	var as AdapterSet
	if err := yaml.Unmarshal(content, &as); err != nil {
		return nil, fmt.Errorf("parse adapter-set %s: %w", path, err)
	}
	as.Path = path
	return &as, nil
}

// IsMultiTarget reports whether more than the presentation slot is filled.
// A nil receiver, an empty AdapterSet, or a presentation-only AdapterSet
// returns false. The check is the gate that backend validation rules consult
// before applying.
func (a *AdapterSet) IsMultiTarget() bool {
	if a == nil {
		return false
	}
	count := 0
	for kind := range a.Targets {
		if kind != "" {
			count++
		}
	}
	if count <= 1 {
		// Either zero or only presentation-slot.
		if _, hasPresentation := a.Targets["presentation"]; hasPresentation && count == 1 {
			return false
		}
		return count > 1
	}
	return true
}
