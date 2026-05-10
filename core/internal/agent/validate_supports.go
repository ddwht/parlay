// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-supports-contract
//
// Validates that every operation in capabilities.yaml uses only terms the
// occupying adapter's supports: block declares. Fires before any AI
// invocation; failures are pre-codegen gates.

package agent

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/parser"
)

// adapterSupports is the parsed shape of an adapter file's supports: block.
type adapterSupports struct {
	OperationKinds []string `yaml:"operation_kinds,omitempty"`
	Steps          []string `yaml:"steps,omitempty"`
	Policies       []string `yaml:"policies,omitempty"`
	Errors         []string `yaml:"errors,omitempty"`
}

// adapterSupportsShape is the projection of an adapter file used for
// supports-contract validation: kind: + supports: block.
type adapterSupportsShape struct {
	Name     string           `yaml:"name"`
	Kind     string           `yaml:"kind"`
	Supports *adapterSupports `yaml:"supports,omitempty"`
}

// ValidateSupports walks the operations in capabilities and asserts that
// every term (kind, step.type, errors[], policies[]) appears in the
// supplied adapter's supports block. The adapter is the one occupying the
// slot whose kind matches the operation's primary concern (transport for
// HTTP exposure, application for orchestration, persistence for storage).
//
// Callers usually invoke this once per non-presentation slot of the
// resolved adapter-set.
func ValidateSupports(mode ValidationMode, adapterContent []byte, capabilities *parser.Capabilities) []ValidationOutcome {
	var outcomes []ValidationOutcome

	var shape adapterSupportsShape
	if err := yaml.Unmarshal(adapterContent, &shape); err != nil {
		outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-parse-failed", err.Error()))
		return outcomes
	}

	// Presentation kind MUST NOT have a supports block.
	if shape.Kind == "presentation" || shape.Kind == "" {
		if shape.Supports != nil {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-shape-mismatch",
				fmt.Sprintf("adapter %q has kind %q but declares supports: (presentation kind forbids supports)", shape.Name, shape.Kind)))
		}
		return outcomes
	}

	// Non-presentation kinds REQUIRE a supports block.
	if shape.Supports == nil {
		outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-shape-mismatch",
			fmt.Sprintf("adapter %q has kind %q but no supports: block is declared", shape.Name, shape.Kind)))
		return outcomes
	}

	// Validate that every declared term is in the closed vocabulary.
	if extras := outsideClosedSet(shape.Supports.OperationKinds, ClosedSetOperationKinds); len(extras) > 0 {
		for _, t := range extras {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-unknown-term",
				fmt.Sprintf("adapter %q declares supports.operation_kinds entry %q outside the closed vocabulary", shape.Name, t)))
		}
	}
	if extras := outsideClosedSet(shape.Supports.Steps, ClosedSetSteps); len(extras) > 0 {
		for _, t := range extras {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-unknown-term",
				fmt.Sprintf("adapter %q declares supports.steps entry %q outside the closed vocabulary", shape.Name, t)))
		}
	}
	if extras := outsideClosedSet(shape.Supports.Policies, ClosedSetPolicies); len(extras) > 0 {
		for _, t := range extras {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-unknown-term",
				fmt.Sprintf("adapter %q declares supports.policies entry %q outside the closed vocabulary", shape.Name, t)))
		}
	}
	if extras := outsideClosedSet(shape.Supports.Errors, ClosedSetErrors); len(extras) > 0 {
		for _, t := range extras {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-unknown-term",
				fmt.Sprintf("adapter %q declares supports.errors entry %q outside the closed vocabulary", shape.Name, t)))
		}
	}

	// Walk capabilities operations and assert support coverage.
	if capabilities == nil {
		return outcomes
	}
	supKinds := toSet(shape.Supports.OperationKinds)
	supSteps := toSet(shape.Supports.Steps)
	supPolicies := toSet(shape.Supports.Policies)
	supErrors := toSet(shape.Supports.Errors)
	for _, op := range capabilities.Operations {
		if op.Kind != "" && !supKinds[op.Kind] {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-operation-kind",
				fmt.Sprintf("operation %s: adapter %q does not support kind %q", op.ID, shape.Name, op.Kind)))
		}
		for _, s := range op.Steps {
			if s.Type != "" && !supSteps[s.Type] {
				outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-step",
					fmt.Sprintf("operation %s: adapter %q does not support step %q", op.ID, shape.Name, s.Type)))
			}
		}
		for _, p := range op.Policies {
			if !supPolicies[p] {
				outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-policy",
					fmt.Sprintf("operation %s: adapter %q does not support policy %q", op.ID, shape.Name, p)))
			}
		}
		for _, e := range op.Errors {
			if !supErrors[e] {
				outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-error",
					fmt.Sprintf("operation %s: adapter %q does not support error %q", op.ID, shape.Name, e)))
			}
		}
	}

	return outcomes
}

func outsideClosedSet(values []string, closed map[string]bool) []string {
	var out []string
	for _, v := range values {
		if !closed[v] {
			out = append(out, v)
		}
	}
	return out
}

func toSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}
