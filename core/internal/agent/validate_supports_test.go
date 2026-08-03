// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-supports-contract
// parlay-artifact: test

package agent

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func TestValidateSupports_PresentationWithSupportsBlockFails(t *testing.T) {
	adapter := []byte(`name: react-antd
kind: presentation
supports:
  steps: [validate-input]
`)
	outcomes := ValidateSupports(ModeBuild, adapter)
	if !findCode(outcomes, "adapter-supports-shape-mismatch") {
		t.Errorf("missing adapter-supports-shape-mismatch; got %+v", outcomes)
	}
}

func TestValidateSupports_NonPresentationWithoutSupportsBlockFails(t *testing.T) {
	adapter := []byte(`name: nestjs-application
kind: application
`)
	outcomes := ValidateSupports(ModeBuild, adapter)
	if !findCode(outcomes, "adapter-supports-shape-mismatch") {
		t.Errorf("missing adapter-supports-shape-mismatch; got %+v", outcomes)
	}
}

func TestValidateSupports_UnknownTermFails(t *testing.T) {
	adapter := []byte(`name: weird
kind: application
supports:
  steps: [foo-bar]
  errors: [validation-failed]
`)
	outcomes := ValidateSupports(ModeBuild, adapter)
	if !findCode(outcomes, "adapter-supports-unknown-term") {
		t.Errorf("missing adapter-supports-unknown-term; got %+v", outcomes)
	}
}

func TestValidateSupports_MissingStepFails(t *testing.T) {
	adapter := []byte(`name: nestjs-application
kind: application
supports:
  operation_kinds: [command, query]
  steps: [validate-input]
  policies: []
  errors: [validation-failed]
`)
	caps := &parser.Capabilities{
		Operations: []parser.CapabilityOperation{
			{
				ID:    "task.search",
				Kind:  "query",
				Steps: []parser.CapabilityStep{{Type: "telepathy"}},
			},
		},
	}
	outcomes := ValidateOperationsCoverage(ModeBuild, map[string][]byte{"application": adapter}, caps)
	if !findCode(outcomes, "adapter-supports-missing-step") {
		t.Errorf("missing adapter-supports-missing-step; got %+v", outcomes)
	}
	if !findMessage(outcomes, "telepathy") {
		t.Errorf("expected message to name telepathy; got %+v", outcomes)
	}
}

func TestValidateSupports_AllTermsCoveredPasses(t *testing.T) {
	adapter := []byte(`name: nestjs-application
kind: application
supports:
  operation_kinds: [command]
  steps: [validate-input, create-one, return-one]
  policies: [transaction-required]
  errors: [validation-failed, conflict]
`)
	caps := &parser.Capabilities{
		Operations: []parser.CapabilityOperation{
			{
				ID:       "task.create",
				Kind:     "command",
				Errors:   []string{"validation-failed", "conflict"},
				Policies: []string{"transaction-required"},
				Steps: []parser.CapabilityStep{
					{Type: "validate-input"},
					{Type: "create-one"},
					{Type: "return-one"},
				},
			},
		},
	}
	outcomes := ValidateOperationsCoverage(ModeBuild, map[string][]byte{"application": adapter}, caps)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error: %+v", o)
		}
	}
}

// The load-bearing test: a term supported by ONE backend but not the other
// passes under union coverage — the exact case the old intersection gate wrongly
// rejected. validate-input lives only on the application adapter, create-one
// only on persistence; both must pass. A term in NEITHER must still fail.
func TestValidateOperationsCoverage_UnionAcrossBackends(t *testing.T) {
	app := []byte(`name: nestjs-application
kind: application
supports:
  operation_kinds: [command]
  steps: [validate-input, return-one]
  policies: [auth-required]
  errors: [validation-failed]
`)
	persist := []byte(`name: prisma-postgres
kind: persistence
supports:
  operation_kinds: [command]
  steps: [create-one]
  policies: []
  errors: [conflict]
`)
	backends := map[string][]byte{"application": app, "persistence": persist}

	covered := &parser.Capabilities{Operations: []parser.CapabilityOperation{{
		ID:       "notes.create",
		Kind:     "command",
		Steps:    []parser.CapabilityStep{{Type: "validate-input"}, {Type: "create-one"}, {Type: "return-one"}},
		Policies: []string{"auth-required"},
		Errors:   []string{"validation-failed", "conflict"},
	}}}
	if out := ValidateOperationsCoverage(ModeBuild, backends, covered); len(out) != 0 {
		t.Fatalf("union coverage should pass when each term is in SOME backend; got %+v", out)
	}

	uncovered := &parser.Capabilities{Operations: []parser.CapabilityOperation{{
		ID:    "notes.search",
		Kind:  "command",
		Steps: []parser.CapabilityStep{{Type: "search"}}, // in neither backend
	}}}
	out := ValidateOperationsCoverage(ModeBuild, backends, uncovered)
	if !findCode(out, "adapter-supports-missing-step") {
		t.Fatalf("a step supported by no backend must fail; got %+v", out)
	}
}
