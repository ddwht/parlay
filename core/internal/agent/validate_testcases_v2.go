// parlay-feature: parlay-tool/multi-adapter
// parlay-component: testcases-v2
//
// Validates testcases.yaml v2 — the discriminated-suite-kind shape with
// {presentation, operation} kinds, source_refs[] requirement, and
// operation-coverage walker.

package agent

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// SuiteKind is the closed discriminator over v2 suite kinds.
type SuiteKind string

const (
	SuiteKindPresentation SuiteKind = "presentation"
	SuiteKindOperation    SuiteKind = "operation"
)

var validSuiteKinds = map[SuiteKind]bool{
	SuiteKindPresentation: true,
	SuiteKindOperation:    true,
}

type testcasesV2Shape struct {
	SchemaVersion int            `yaml:"schema_version"`
	Feature       string         `yaml:"feature"`
	Suites        []suiteV2Shape `yaml:"suites"`
}

type suiteV2Shape struct {
	Name       string             `yaml:"name"`
	Kind       string             `yaml:"kind,omitempty"`   // v2 only
	Component  string             `yaml:"component,omitempty"`
	Operation  string             `yaml:"operation,omitempty"` // v2 operation suites
	Intent     string             `yaml:"intent,omitempty"`    // v1 legacy
	SourceRefs []string           `yaml:"source_refs,omitempty"`
	Output     map[string]yaml.Node `yaml:"output,omitempty"`
	Cases      []map[string]yaml.Node `yaml:"cases,omitempty"`
}

// ValidateTestcasesV2 validates the v2 shape and walks operation coverage
// against the supplied list of canonical operation refs.
func ValidateTestcasesV2(mode ValidationMode, path string, content []byte, canonicalOperations []string) []ValidationOutcome {
	var outcomes []ValidationOutcome

	var tc testcasesV2Shape
	if err := yaml.Unmarshal(content, &tc); err != nil {
		// Upstream YAML validator handles parse errors.
		return outcomes
	}

	covered := make(map[string]bool)
	for _, suite := range tc.Suites {
		// Discriminator check.
		kind := suite.Kind
		if kind == "" {
			// Legacy v1 — load as presentation. If source_refs is empty AND no
			// intent string, surface the legacy warning.
			if len(suite.SourceRefs) == 0 {
				if suite.Intent == "" {
					outcomes = append(outcomes, NewOutcome(mode, "testcases-source-refs-missing-legacy",
						fmt.Sprintf("%s: legacy v1 suite %q has neither source_refs nor intent — auto-population would be approximate", path, suite.Name)))
				} else {
					outcomes = append(outcomes, NewOutcome(mode, "testcases-source-refs-missing-legacy",
						fmt.Sprintf("%s: legacy v1 suite %q auto-populated source_refs from intent string — regenerate v2 testcases to clear this warning", path, suite.Name)))
				}
			}
			continue
		}

		if !validSuiteKinds[SuiteKind(kind)] {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-suite-kind-unknown",
				fmt.Sprintf("%s: suite %q kind %q is outside the v2 set {presentation, operation}", path, suite.Name, kind)))
			continue
		}

		// New v2 suites must declare source_refs.
		if len(suite.SourceRefs) == 0 {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-source-refs-missing",
				fmt.Sprintf("%s: v2 suite %q declares no source_refs (every v2 suite must cite at least one)", path, suite.Name)))
		}

		if SuiteKind(kind) == SuiteKindOperation {
			if suite.Operation == "" {
				outcomes = append(outcomes, NewOutcome(mode, "testcases-operation-shape-mismatch",
					fmt.Sprintf("%s: operation suite %q has no operation: reference", path, suite.Name)))
				continue
			}
			covered[suite.Operation] = true
		}
	}

	// Coverage walker — every canonical operation must have a covering
	// operation suite.
	for _, op := range canonicalOperations {
		if !covered[op] {
			outcomes = append(outcomes, NewOutcome(mode, "testcases-operation-uncovered",
				fmt.Sprintf("%s: operation %q has no covering kind: operation suite", path, op)))
		}
	}

	return outcomes
}
