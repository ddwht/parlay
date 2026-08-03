// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-supports-contract
//
// Validates that every operation in capabilities.yaml uses only terms the
// occupying adapter's supports: block declares. Fires before any AI
// invocation; failures are pre-codegen gates.

package agent

import (
	"fmt"
	"sort"
	"strings"

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

// ValidateSupports validates a SINGLE adapter's own supports: block — that it
// parses, has the right shape for its kind (presentation forbids it,
// non-presentation requires it), and declares only closed-vocabulary terms.
// It does NOT check operation coverage; that is a project-level, cross-adapter
// question answered by ValidateOperationsCoverage (union across all filled
// backend slots). Splitting the two is what lets the gate reason per-layer: an
// adapter legitimately supports only its own layer's terms.
//
// Callers invoke this once per non-presentation slot of the resolved
// adapter-set.
func ValidateSupports(mode ValidationMode, adapterContent []byte) []ValidationOutcome {
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

	return outcomes
}

// ValidateOperationsCoverage asserts that every term of every operation is
// supported by AT LEAST ONE of the filled backend adapters — union coverage,
// not intersection. backendAdapters maps each non-presentation slot's kind to
// its raw adapter bytes. With honest per-layer supports listings this is the
// correct and complete gate: a term no filled layer implements (e.g. a
// create-one step in a project with no persistence slot) is listed by nobody
// and fails, while a term one layer legitimately owns (validate-input on the
// application adapter, create-one on the persistence adapter) passes even
// though the other backend adapter does not list it.
//
// Shape/vocabulary problems in an individual adapter are reported separately
// by ValidateSupports; this function only answers the coverage question.
func ValidateOperationsCoverage(mode ValidationMode, backendAdapters map[string][]byte, capabilities *parser.Capabilities) []ValidationOutcome {
	var outcomes []ValidationOutcome
	if capabilities == nil || len(backendAdapters) == 0 {
		return outcomes
	}

	supKinds := map[string]bool{}
	supSteps := map[string]bool{}
	supPolicies := map[string]bool{}
	supErrors := map[string]bool{}
	var filledKinds []string
	for kind, content := range backendAdapters {
		filledKinds = append(filledKinds, kind)
		var shape adapterSupportsShape
		if err := yaml.Unmarshal(content, &shape); err != nil || shape.Supports == nil {
			// A malformed or supports-less adapter contributes nothing to the
			// union; ValidateSupports reports it per-adapter.
			continue
		}
		for _, k := range shape.Supports.OperationKinds {
			supKinds[k] = true
		}
		for _, s := range shape.Supports.Steps {
			supSteps[s] = true
		}
		for _, p := range shape.Supports.Policies {
			supPolicies[p] = true
		}
		for _, e := range shape.Supports.Errors {
			supErrors[e] = true
		}
	}
	sort.Strings(filledKinds)
	slots := strings.Join(filledKinds, ", ")

	for _, op := range capabilities.Operations {
		if op.Kind != "" && !supKinds[op.Kind] {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-operation-kind",
				fmt.Sprintf("operation %s: kind %q is supported by no configured backend layer (filled: %s)", op.ID, op.Kind, slots)))
		}
		for _, s := range op.Steps {
			if s.Type != "" && !supSteps[s.Type] {
				outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-step",
					fmt.Sprintf("operation %s: step %q is supported by no configured backend layer (filled: %s)", op.ID, s.Type, slots)))
			}
		}
		for _, p := range op.Policies {
			if !supPolicies[p] {
				outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-policy",
					fmt.Sprintf("operation %s: policy %q is supported by no configured backend layer (filled: %s)", op.ID, p, slots)))
			}
		}
		for _, e := range op.Errors {
			if !supErrors[e] {
				outcomes = append(outcomes, NewOutcome(mode, "adapter-supports-missing-error",
					fmt.Sprintf("operation %s: error %q is supported by no configured backend layer (filled: %s)", op.ID, e, slots)))
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
