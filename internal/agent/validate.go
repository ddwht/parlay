package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validator checks a file's content against its schema.
type Validator func(path string, content []byte) error

// ValidateYAML checks that a file is valid YAML.
func ValidateYAML(path string, content []byte) error {
	var v interface{}
	if err := yaml.Unmarshal(content, &v); err != nil {
		return fmt.Errorf("%s is not valid YAML: %w", path, err)
	}
	return nil
}

// ValidateBuildfile checks buildfile.yaml has required fields.
func ValidateBuildfile(path string, content []byte) error {
	if err := ValidateYAML(path, content); err != nil {
		return err
	}
	var bf struct {
		Feature    string      `yaml:"feature"`
		Adapter    string      `yaml:"adapter"`
		Components interface{} `yaml:"components"`
	}
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return fmt.Errorf("buildfile structure invalid: %w", err)
	}
	if bf.Feature == "" {
		return fmt.Errorf("buildfile missing 'feature' field")
	}
	if bf.Adapter == "" {
		return fmt.Errorf("buildfile missing 'adapter' field")
	}
	return nil
}

// ValidateSurface checks surface.md has fragment headings with Shows fields.
func ValidateSurface(path string, content []byte) error {
	text := string(content)
	if !strings.Contains(text, "## ") {
		return fmt.Errorf("surface.md has no fragment headings (## )")
	}
	if !strings.Contains(text, "**Shows**:") {
		return fmt.Errorf("surface.md has no **Shows**: fields")
	}
	return nil
}

// deepBuildfile is the parsed structure for deep validation.
type deepBuildfile struct {
	Feature    string                       `yaml:"feature"`
	Adapter    string                       `yaml:"adapter"`
	Models     map[string]interface{}       `yaml:"models"`
	Fixtures   map[string]deepFixture       `yaml:"fixtures"`
	Routes     []deepRoute                  `yaml:"routes"`
	Components map[string]deepComponent     `yaml:"components"`
}

type deepFixture struct {
	Data map[string]interface{} `yaml:"data"`
}

type deepRoute struct {
	Path    string                   `yaml:"path"`
	Regions map[string]deepRegion    `yaml:"regions"`
}

type deepRegion struct {
	Components []string `yaml:"components"`
}

type deepComponent struct {
	Source string         `yaml:"source"`
	Widget string         `yaml:"widget"`
	Type   string         `yaml:"type"`
	Data   *deepData      `yaml:"data"`
	Children []string     `yaml:"children"`
}

type deepData struct {
	Inputs []deepInput `yaml:"inputs"`
}

type deepInput struct {
	Model string `yaml:"model"`
}

// deepAdapter is the parsed adapter structure for vocabulary validation.
type deepAdapter struct {
	ComponentTypes map[string]interface{} `yaml:"component-types"`
	ElementTypes   map[string]interface{} `yaml:"element-types"`
	ActionTypes    map[string]interface{} `yaml:"action-types"`
}

// ValidateBuildfileDeep performs cross-reference validation on a buildfile.
// It checks: model references, component references in routes, fixture-model alignment,
// component children references, and adapter vocabulary when an adapter path is provided.
func ValidateBuildfileDeep(buildfilePath, adapterPath string) []string {
	var errors []string

	content, err := os.ReadFile(buildfilePath)
	if err != nil {
		return []string{fmt.Sprintf("cannot read buildfile: %s", err)}
	}

	var bf deepBuildfile
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return []string{fmt.Sprintf("invalid buildfile YAML: %s", err)}
	}

	// 1. Component references in routes must exist in components
	for _, route := range bf.Routes {
		for regionName, region := range route.Regions {
			for _, compRef := range region.Components {
				if _, ok := bf.Components[compRef]; !ok {
					errors = append(errors, fmt.Sprintf(
						"route %q region %q references component %q which is not defined",
						route.Path, regionName, compRef))
				}
			}
		}
	}

	// 2. Model references in component data.inputs must exist in models
	for compName, comp := range bf.Components {
		if comp.Data != nil {
			for _, input := range comp.Data.Inputs {
				if input.Model != "" {
					if _, ok := bf.Models[input.Model]; !ok {
						errors = append(errors, fmt.Sprintf(
							"component %q references model %q which is not defined",
							compName, input.Model))
					}
				}
			}
		}

		// 3. Children references must exist in components
		for _, child := range comp.Children {
			if _, ok := bf.Components[child]; !ok {
				errors = append(errors, fmt.Sprintf(
					"component %q references child %q which is not defined",
					compName, child))
			}
		}
	}

	// 4. Fixture data keys must match defined models
	for fixtureName, fixture := range bf.Fixtures {
		for modelName := range fixture.Data {
			if _, ok := bf.Models[modelName]; !ok {
				errors = append(errors, fmt.Sprintf(
					"fixture %q references model %q which is not defined",
					fixtureName, modelName))
			}
		}
	}

	// 5. Adapter vocabulary validation (if adapter path provided)
	if adapterPath != "" {
		adapterErrors := validateAdapterVocabulary(bf, adapterPath)
		errors = append(errors, adapterErrors...)
	}

	return errors
}

func validateAdapterVocabulary(bf deepBuildfile, adapterPath string) []string {
	var errors []string

	data, err := os.ReadFile(adapterPath)
	if err != nil {
		// Adapter file doesn't exist — try resolving from .parlay/adapters/
		resolved := filepath.Join(".parlay", "adapters", bf.Adapter+".adapter.yaml")
		data, err = os.ReadFile(resolved)
		if err != nil {
			return []string{fmt.Sprintf("cannot read adapter %q: %s", adapterPath, err)}
		}
	}

	var adapter deepAdapter
	if err := yaml.Unmarshal(data, &adapter); err != nil {
		return []string{fmt.Sprintf("invalid adapter YAML: %s", err)}
	}

	// Check component types against adapter
	for compName, comp := range bf.Components {
		if comp.Type != "" && adapter.ComponentTypes != nil {
			if _, ok := adapter.ComponentTypes[comp.Type]; !ok {
				errors = append(errors, fmt.Sprintf(
					"component %q uses type %q which is not in adapter %q",
					compName, comp.Type, bf.Adapter))
			}
		}
	}

	return errors
}
