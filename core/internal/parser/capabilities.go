// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-artifact
//
// Parser for spec/intents/<feature>/capabilities.yaml — the closed-vocabulary
// backend artifact replacing prose-shaped operation fragments in
// infrastructure.md.

package parser

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Capabilities is the parsed shape of capabilities.yaml.
type Capabilities struct {
	Path          string                `yaml:"-"`
	SchemaVersion int                   `yaml:"schema_version"`
	Feature       string                `yaml:"feature"`
	Operations    []CapabilityOperation `yaml:"operations"`
}

// CapabilityOperation is one entry in the operations: list. Every term
// (kind, step.type, errors[], policies[]) is closed-vocabulary; the
// validator rejects anything outside the corresponding closed-vocabulary
// schema.
type CapabilityOperation struct {
	ID       string            `yaml:"id"`
	Kind     string            `yaml:"kind"`
	Subject  CapabilitySubject `yaml:"subject"`
	Input    *CapabilityIO     `yaml:"input,omitempty"`
	Output   *CapabilityIO     `yaml:"output,omitempty"`
	Errors   []string          `yaml:"errors,omitempty"`
	Policies []string          `yaml:"policies,omitempty"`
	Steps    []CapabilityStep  `yaml:"steps"`
}

// CapabilitySubject names the entity an operation acts on.
type CapabilitySubject struct {
	Entity string `yaml:"entity"`
}

// CapabilityIO is the shared shape for input and output declarations. Type
// names the input DTO; Shape ("one"/"many"/"empty") and Entity describe
// outputs. Optional fields are zero-valued when absent.
type CapabilityIO struct {
	Type   string `yaml:"type,omitempty"`
	Shape  string `yaml:"shape,omitempty"`
	Entity string `yaml:"entity,omitempty"`
}

// CapabilityStep is one entry in the steps: list. The shape is open by
// design — Type is closed-vocabulary, but adapters may attach arbitrary
// structural metadata (entity, identity, where, etc.).
type CapabilityStep struct {
	Type     string `yaml:"type"`
	Entity   string `yaml:"entity,omitempty"`
	Identity string `yaml:"identity,omitempty"`
	Where    string `yaml:"where,omitempty"`
}

// ParseCapabilities reads capabilities.yaml from disk and parses it.
func ParseCapabilities(path string) (*Capabilities, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read capabilities %s: %w", path, err)
	}
	return ParseCapabilitiesBytes(path, data)
}

// ParseCapabilitiesBytes parses capabilities.yaml content already in memory.
func ParseCapabilitiesBytes(path string, content []byte) (*Capabilities, error) {
	var c Capabilities
	if err := yaml.Unmarshal(content, &c); err != nil {
		return nil, fmt.Errorf("parse capabilities %s: %w", path, err)
	}
	c.Path = path
	return &c, nil
}

// NormalizeOperationID turns a feature-local id ("task.create") into the
// buildfile-canonical reference "@<feature>/operation:<id>". The buildfile
// validator rejects any reference that did not pass through this function.
func NormalizeOperationID(feature, id string) string {
	return fmt.Sprintf("@%s/operation:%s", feature, id)
}
