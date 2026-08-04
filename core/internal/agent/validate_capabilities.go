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

// ValidateCapabilities walks every operation in capabilities.yaml against the
// closed vocabularies, validates duplicate ids, surfaces stub operations that
// need explicit kind:, and cross-checks the entity references the schema has
// always claimed: subject.entity and output.entity name entities declared in
// domain-model.yaml.
//
// The schema states that as settled fact — it explains what input.type lacks by
// contrast, "input.type is not a reference into a closed vocabulary the way
// subject.entity/output.entity are references into domain-model.yaml's declared
// entities". It was not true. parser.CapabilityOperation.Subject was decoded and
// then read by nothing, so an operation could name an entity that did not exist
// and every validator passed it. The failure surfaces later, in build-feature or
// codegen, as a wiring problem with no obvious cause.
//
// declaredEntities is the entity names from the resolved root's
// domain-model.yaml. A nil or empty slice disables the cross-reference rather
// than failing every operation: a project with no domain model yet is a normal
// state, and treating "no model" as "no valid entities" would make the check
// fire hardest on projects that have not reached it. Absence of the model is a
// separate condition with its own diagnostics; this check is about references
// disagreeing with a model that exists.
// There is deliberately no second entry point that skips the cross-reference.
// An entity-blind ValidateCapabilities alongside an entity-aware one is two
// validators with different contracts and one CLI wired to whichever was
// convenient — the shape this consolidation found five times, and the reason
// TestConformance_CanonicalValidatorsAreReachable exists. It caught this: the
// first version of this change left the old signature in place as a wrapper, and
// the test correctly reported an exported validator no command called. Callers
// with no domain model pass nil.
func ValidateCapabilities(mode ValidationMode, path string, content []byte, declaredEntities []string) []ValidationOutcome {
	var outcomes []ValidationOutcome

	known := make(map[string]bool, len(declaredEntities))
	for _, e := range declaredEntities {
		known[e] = true
	}
	entityList := strings.Join(declaredEntities, ", ")

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

		// source: is required on generate and tolerated absent on read, so
		// this is a warning in both modes rather than an error. Every
		// capabilities.yaml predates the field; erroring would fail all of
		// them at once over a fact none could have recorded, which is the
		// same shape as buildfile-models-deprecated.
		//
		// Reported at all because the reverse traceability walk is the one
		// thing that cannot degrade gracefully in silence: without a
		// source, "which artifact owns this change" has to be answered by
		// name similarity, which both misses renames and blesses
		// contradictions.
		if strings.TrimSpace(op.Source) == "" {
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-source-missing",
				fmt.Sprintf("%s: operation %q declares no source: — nothing records which intent it came from, so a change described in prose cannot be routed to it", path, op.ID)))
		}

		// subject: is Required. It was marked so in the field reference and
		// enforced nowhere, so an operation with no subject validated cleanly and
		// then had nothing for build-feature to wire against.
		if strings.TrimSpace(op.Subject.Entity) == "" {
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-subject-missing",
				fmt.Sprintf("%s: operation %q declares no subject.entity — every operation acts on a primary entity, and the wiring downstream is derived from it", path, op.ID)))
		} else if len(known) > 0 && !known[op.Subject.Entity] {
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-entity-undeclared",
				fmt.Sprintf("%s: operation %q has subject.entity %q, which is not declared in domain-model.yaml (declared: %s)", path, op.ID, op.Subject.Entity, entityList)))
		}

		// output.entity carries the same reference, and the schema names the two
		// in one breath. A non-empty shape returns an entity; an empty one does
		// not, so only check when the shape says something comes back.
		if op.Output != nil && op.Output.Entity != "" && len(known) > 0 && !known[op.Output.Entity] {
			outcomes = append(outcomes, NewOutcome(mode, "capabilities-entity-undeclared",
				fmt.Sprintf("%s: operation %q has output.entity %q, which is not declared in domain-model.yaml (declared: %s)", path, op.ID, op.Output.Entity, entityList)))
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

		// Policy-step-error tie rules — see capabilities.schema.md's
		// "Policy-step-error tie rules" section. auth-required and
		// permission-required each imply a specific step (authorize) and
		// error (unauthorized / forbidden respectively); the tie is
		// one-directional — declaring the step or error without the
		// policy is fine, but declaring the policy without its tied step
		// or error is not. transaction-required has no tie (see the
		// schema section for why).
		hasAuthorizeStep := false
		for _, s := range op.Steps {
			if s.Type == "authorize" {
				hasAuthorizeStep = true
				break
			}
		}
		hasError := func(name string) bool {
			for _, e := range op.Errors {
				if e == name {
					return true
				}
			}
			return false
		}
		for _, p := range op.Policies {
			var tiedError string
			switch p {
			case "auth-required":
				tiedError = "unauthorized"
			case "permission-required":
				tiedError = "forbidden"
			default:
				continue
			}
			if !hasAuthorizeStep {
				outcomes = append(outcomes, NewOutcome(mode, "capabilities-policy-missing-step",
					fmt.Sprintf("%s: operation %q declares policy %q but has no authorize step in steps:", path, op.ID, p)))
			}
			if !hasError(tiedError) {
				outcomes = append(outcomes, NewOutcome(mode, "capabilities-policy-missing-error",
					fmt.Sprintf("%s: operation %q declares policy %q but errors: has no %q entry", path, op.ID, p, tiedError)))
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
