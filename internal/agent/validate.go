package agent

import (
	"fmt"
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
