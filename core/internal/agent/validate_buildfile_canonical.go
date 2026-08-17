// parlay-feature: parlay-tool/multi-adapter
// parlay-component: multi-target-buildfile-schema
//
// Validates the multi-target canonical-once rule on buildfile.yaml:
// canonical fields (kind, subject, input, output, errors, policies, steps)
// belong under operations: and only there; targets.<kind>: carries
// per-target projection metadata only; bindings: rules and
// targets.<kind>.operations[] entries reference operations by the
// normalized @<feature>/operation:<id> form.

package agent

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// canonicalFields are the operation fields that may appear ONLY under
// operations:. Any restating of these inside a targets.<kind>: entry fails
// with buildfile-target-restates-canonical.
var canonicalFields = map[string]bool{
	"kind":     true,
	"subject":  true,
	"input":    true,
	"output":   true,
	"errors":   true,
	"policies": true,
	"steps":    true,
}

// canonicalBuildfileShape is the projection used for canonical-once + ref
// resolution. We only need the shape of operations: + targets: + bindings:.
type canonicalBuildfileShape struct {
	Operations map[string]yaml.Node            `yaml:"operations,omitempty"`
	Targets    map[string]canonicalTargetEntry `yaml:"targets,omitempty"`
	Bindings   yaml.Node                       `yaml:"bindings,omitempty"`
	Components yaml.Node                       `yaml:"components,omitempty"`
	Models     yaml.Node                       `yaml:"models,omitempty"`
}

type canonicalTargetEntry struct {
	Components yaml.Node `yaml:"components,omitempty"`
	Routes     yaml.Node `yaml:"routes,omitempty"`
	// Operations may take two shapes: a list of maps (for the
	// canonical-once-restatement check) or a keyed map (for ref
	// resolution). We capture it as a raw node and dispatch in code.
	Operations yaml.Node `yaml:"operations,omitempty"`
}

// targetOperationsRefShape projects the keyed-map shape some adapters use:
//
//	targets.transport.operations:
//	  "@feature/operation:x": { exposure: rest-endpoint, ... }
//
// alongside the listed shape captured by canonicalTargetEntry.Operations.
// Only one shape resolves per target — the parser tries the keyed form
// first, then the listed form.
type targetOperationsRefShape struct {
	OperationsKeyed map[string]yaml.Node `yaml:"operations,omitempty"`
}

// ValidateBuildfileCanonical enforces the canonical-once rule and the
// operation-ref resolution rules on a multi-target buildfile.
func ValidateBuildfileCanonical(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	var outcomes []ValidationOutcome

	var bf canonicalBuildfileShape
	if err := yaml.Unmarshal(content, &bf); err != nil {
		// Parse failure is reported by the upstream YAML validator; this
		// validator stays silent rather than double-reporting.
		return outcomes
	}

	// Canonical-once: no targets entry restates canonical fields.
	// Walk both shapes the operations: block can take.
	for kind, target := range bf.Targets {
		walkTargetCanonicalFields(target.Operations, func(fieldName string) {
			if canonicalFields[fieldName] {
				outcomes = append(outcomes, NewOutcome(mode, "buildfile-target-restates-canonical",
					fmt.Sprintf("%s: targets.%s repeats canonical field %q (canonical fields belong under operations: only)", path, kind, fieldName)))
			}
		})
	}

	// Components double-declaration.
	hasTopLevel := !isEmptyNode(bf.Components)
	hasInTarget := false
	for _, target := range bf.Targets {
		if !isEmptyNode(target.Components) {
			hasInTarget = true
			break
		}
	}
	if hasTopLevel && hasInTarget {
		outcomes = append(outcomes, NewOutcome(mode, "buildfile-components-double-declared",
			fmt.Sprintf("%s: top-level components: AND targets.<kind>.components: are both populated", path)))
	}

	// Top-level models: removed in v0.3.
	if !isEmptyNode(bf.Models) {
		outcomes = append(outcomes, NewOutcome(mode, "buildfile-models-unsupported",
			fmt.Sprintf("%s: top-level models: was removed in v0.3 (entity declarations belong in domain-model.yaml)", path)))
	}

	// Operation-ref resolution: every reference under targets[].operations
	// and under bindings: must resolve to a key declared in operations:.
	// This is the load-bearing check architecture §9 + §12 calls for —
	// without it, a target can name an operation that the canonical
	// declarations don't carry, and codegen would silently emit nothing.
	declared := make(map[string]bool, len(bf.Operations))
	for opRef := range bf.Operations {
		declared[opRef] = true
	}

	// Re-decode targets in the keyed-map shape to catch
	// targets.<kind>.operations: "@feature/operation:x" entries.
	var keyedShape struct {
		Targets map[string]targetOperationsRefShape `yaml:"targets"`
	}
	if err := yaml.Unmarshal(content, &keyedShape); err == nil {
		for kind, target := range keyedShape.Targets {
			for opRef := range target.OperationsKeyed {
				if opRef == "" {
					continue
				}
				if outcome, ok := ValidateOperationRefNormalized(mode, opRef); !ok {
					outcomes = append(outcomes, outcome)
					continue
				}
				if !declared[opRef] {
					outcomes = append(outcomes, NewOutcome(mode, "buildfile-target-operation-missing",
						fmt.Sprintf("%s: targets.%s.operations references %q which is not declared under operations:", path, kind, opRef)))
				}
			}
		}
	}

	// Bindings: each leaf entry's domain_element may reference an
	// operation in the form @feature/operation:x. Walk the bindings tree
	// and check every such reference resolves.
	walkBindingOperationRefs(bf.Bindings, func(opRef string) {
		if opRef == "" {
			return
		}
		if outcome, ok := ValidateOperationRefNormalized(mode, opRef); !ok {
			outcomes = append(outcomes, outcome)
			return
		}
		if !declared[opRef] {
			outcomes = append(outcomes, NewOutcome(mode, "buildfile-binding-operation-missing",
				fmt.Sprintf("%s: bindings reference %q which is not declared under operations:", path, opRef)))
		}
	})

	return outcomes
}

// walkBindingOperationRefs walks a bindings: yaml.Node tree and emits every
// domain_element value that looks like an operation reference. The bindings
// section is keyed feature → page → node-path → entry; each leaf entry has
// a domain_element field that may name an operation (the form
// @feature/operation:id).
func walkBindingOperationRefs(node yaml.Node, emit func(opRef string)) {
	if node.Kind != yaml.MappingNode {
		return
	}
	// Look for direct domain_element children at this level.
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]
		if key.Value == "domain_element" && val.Kind == yaml.ScalarNode {
			s := val.Value
			if strings.HasPrefix(s, "@") && strings.Contains(s, "/operation:") {
				emit(s)
			}
			continue
		}
		// Recurse into any nested mapping/sequence.
		switch val.Kind {
		case yaml.MappingNode:
			walkBindingOperationRefs(*val, emit)
		case yaml.SequenceNode:
			for _, child := range val.Content {
				if child.Kind == yaml.MappingNode {
					walkBindingOperationRefs(*child, emit)
				}
			}
		}
	}
}

// walkTargetCanonicalFields walks a target's operations: node and emits
// every field name that appears at the per-operation level. Handles both
// shapes:
//
//	operations:
//	  - kind: command          # listed shape (sequence of maps)
//	    errors: [...]
//
//	operations:
//	  "@feature/operation:x":  # keyed-map shape — fields appear inside the value
//	    kind: command
//	    errors: [...]
func walkTargetCanonicalFields(node yaml.Node, emit func(fieldName string)) {
	switch node.Kind {
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if child.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(child.Content); i += 2 {
					emit(child.Content[i].Value)
				}
			}
		}
	case yaml.MappingNode:
		// Each value is the per-operation block; iterate its keys.
		for i := 1; i < len(node.Content); i += 2 {
			val := node.Content[i]
			if val.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(val.Content); j += 2 {
					emit(val.Content[j].Value)
				}
			}
		}
	}
}

func isEmptyNode(n yaml.Node) bool {
	if n.Kind == 0 {
		return true
	}
	if n.Kind == yaml.MappingNode && len(n.Content) == 0 {
		return true
	}
	if n.Kind == yaml.SequenceNode && len(n.Content) == 0 {
		return true
	}
	if n.Kind == yaml.ScalarNode && n.Value == "" {
		return true
	}
	return false
}
