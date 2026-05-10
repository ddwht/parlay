// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-artifact
//
// Validates spec/intents/<feature>/capabilities.yaml against the closed
// vocabularies declared in operation-kinds/steps/errors/policies schema
// files. Normalizes operation ids on the way into the buildfile.

package agent

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

// ClosedSetOperationKinds — see operation-kinds.schema.md.
var ClosedSetOperationKinds = map[string]bool{
	"command": true,
	"query":   true,
}

// v2DeferredOperationKinds are reserved values that fail with a fix message
// pointing at v2.
var v2DeferredOperationKinds = map[string]bool{
	"subscription": true,
	"job":          true,
}

// ClosedSetSteps — union of write/read/return groups; see steps.schema.md.
var ClosedSetSteps = map[string]bool{
	"validate-input": true,
	"authorize":      true,
	"create-one":     true,
	"update-one":     true,
	"delete-one":     true,
	"read-one":       true,
	"read-many":      true,
	"search":         true,
	"return-one":     true,
	"return-many":    true,
	"return-empty":   true,
}

// ClosedSetErrors — see errors.schema.md.
var ClosedSetErrors = map[string]bool{
	"validation-failed": true,
	"unauthorized":      true,
	"forbidden":         true,
	"not-found":         true,
	"conflict":          true,
	"server-error":      true,
}

// ClosedSetPolicies — see policies.schema.md.
var ClosedSetPolicies = map[string]bool{
	"auth-required":        true,
	"permission-required":  true,
	"transaction-required": true,
}

// ValidateCapabilities walks every operation in capabilities.yaml against
// the closed vocabularies, validates duplicate ids, and surfaces stub
// operations that need explicit kind:.
func ValidateCapabilities(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	var outcomes []ValidationOutcome

	caps, err := parser.ParseCapabilitiesBytes(path, content)
	if err != nil {
		outcomes = append(outcomes, NewOutcome(mode, "capabilities-not-closed-form",
			fmt.Sprintf("parse %s: %v", path, err)))
		return outcomes
	}

	if caps.SchemaVersion == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "capabilities-not-closed-form",
			fmt.Sprintf("%s: schema_version is required", path)))
	}
	if caps.Feature == "" {
		outcomes = append(outcomes, NewOutcome(mode, "capabilities-not-closed-form",
			fmt.Sprintf("%s: feature is required", path)))
	}

	seen := make(map[string]bool)
	for _, op := range caps.Operations {
		if op.ID == "" {
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-not-closed-form",
				fmt.Sprintf("%s: operation has no id", path)))
			continue
		}
		if seen[op.ID] {
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-duplicate-operation-id",
				fmt.Sprintf("%s: operation id %q appears more than once", path, op.ID)))
		}
		seen[op.ID] = true

		// Stub detection.
		if op.Kind == "unknown" {
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-stub-unfilled",
				fmt.Sprintf("%s: operation %q has kind: unknown — fill in kind: explicitly before build mode", path, op.ID)))
			continue
		}

		// Closed-vocabulary checks.
		if op.Kind != "" && !ClosedSetOperationKinds[op.Kind] {
			msg := fmt.Sprintf("%s: operation %q kind %q is outside the closed set {command, query}", path, op.ID, op.Kind)
			if v2DeferredOperationKinds[op.Kind] {
				msg += " — v2-deferred (subscription and job are reserved for v2)"
			}
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-unknown-term", msg))
		}
		for _, s := range op.Steps {
			if s.Type == "" {
				continue
			}
			if !ClosedSetSteps[s.Type] {
				outcomes = append(outcomes, NewOutcome(mode, "capabilities-unknown-term",
					fmt.Sprintf("%s: operation %q step %q is outside the closed steps vocabulary", path, op.ID, s.Type)))
			}
		}
		for _, e := range op.Errors {
			if !ClosedSetErrors[e] {
				outcomes = append(outcomes, NewOutcome(mode, "capabilities-unknown-term",
					fmt.Sprintf("%s: operation %q error %q is outside the closed errors vocabulary", path, op.ID, e)))
			}
		}
		for _, p := range op.Policies {
			if !ClosedSetPolicies[p] {
				outcomes = append(outcomes, NewOutcome(mode, "capabilities-unknown-term",
					fmt.Sprintf("%s: operation %q policy %q is outside the closed policies vocabulary", path, op.ID, p)))
			}
		}
	}
	return outcomes
}

// ValidateOperationRefNormalized confirms that a buildfile-canonical
// operation reference has the @<feature>/operation:<id> shape produced by
// parser.NormalizeOperationID.
func ValidateOperationRefNormalized(mode ValidationMode, ref string) (ValidationOutcome, bool) {
	if strings.HasPrefix(ref, "@") && strings.Contains(ref, "/operation:") {
		return ValidationOutcome{}, true
	}
	return NewOutcome(mode, "buildfile-operation-ref-unnormalized",
		fmt.Sprintf("operation reference %q is not normalized (expected @<feature>/operation:<id>)", ref)), false
}
