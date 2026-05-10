// parlay-feature: parlay-tool/multi-adapter
// parlay-component: surface-yaml-and-migrator
//
// YAML loader for surface.yaml — the structured form of the surface artifact
// that coexists with the legacy surface.md form during the migration window.
// Both formats parse to the same in-memory []Fragment representation, so the
// build pipeline does not branch on serialization.

package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// surfaceYAMLDoc is the on-disk shape of surface.yaml. Field names mirror
// the legacy surface.md sections so authors can move between the forms
// without re-learning the vocabulary.
type surfaceYAMLDoc struct {
	Feature   string                `yaml:"feature,omitempty"`
	Fragments []surfaceYAMLFragment `yaml:"fragments"`
}

type surfaceYAMLFragment struct {
	Name    string   `yaml:"name"`
	Shows   string   `yaml:"shows"`
	Actions string   `yaml:"actions"`
	Source  string   `yaml:"source"`
	Page    string   `yaml:"page"`
	Region  string   `yaml:"region"`
	Order   int      `yaml:"order,omitempty"`
	Notes   []string `yaml:"notes,omitempty"`
}

// LoadSurfaceYAML reads surface.yaml at path and returns the same []Fragment
// shape the legacy surface.md parser produces. The Feature field on each
// fragment is populated from the doc-level `feature:` key when present, or
// from the parent directory name otherwise (matching surface.md behavior).
func LoadSurfaceYAML(path string) ([]Fragment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read surface yaml %s: %w", path, err)
	}
	return LoadSurfaceYAMLBytes(path, data)
}

// LoadSurfaceYAMLBytes parses surface.yaml content already in memory and
// returns the equivalent []Fragment.
func LoadSurfaceYAMLBytes(path string, content []byte) ([]Fragment, error) {
	var doc surfaceYAMLDoc
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parse surface yaml %s: %w", path, err)
	}

	feature := doc.Feature
	if feature == "" {
		feature = featureFromSurfacePath(path)
	}

	out := make([]Fragment, 0, len(doc.Fragments))
	for _, f := range doc.Fragments {
		out = append(out, Fragment{
			Name:    f.Name,
			Shows:   f.Shows,
			Actions: f.Actions,
			Source:  f.Source,
			Page:    f.Page,
			Region:  f.Region,
			Order:   f.Order,
			Notes:   f.Notes,
			Feature: feature,
		})
	}
	return out, nil
}

// ResolveSurfacePath returns the on-disk surface artifact path inside a
// feature directory, preferring surface.yaml (the v1 multi-adapter target
// format) over the legacy surface.md. Returns an empty string when neither
// file is present — callers handle this as "feature has no surface."
//
// parlay-feature: parlay-tool/multi-adapter
// parlay-component: surface-yaml-and-migrator
func ResolveSurfacePath(featureDir string) string {
	yamlPath := filepath.Join(featureDir, "surface.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}
	mdPath := filepath.Join(featureDir, "surface.md")
	if _, err := os.Stat(mdPath); err == nil {
		return mdPath
	}
	return ""
}

// featureFromSurfacePath derives the feature slug from the directory layout
// spec/intents/<feature>/surface.yaml. Returns empty string when the path
// does not match the expected layout — callers are expected to fill the
// Feature field via other means in that case.
func featureFromSurfacePath(path string) string {
	dir := filepath.Dir(path)
	idx := strings.Index(dir, "intents"+string(filepath.Separator))
	if idx < 0 {
		return filepath.Base(dir)
	}
	rel := dir[idx+len("intents")+1:]
	return rel
}
