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
	outcomes := ValidateSupports(ModeBuild, adapter, nil)
	if !findCode(outcomes, "adapter-supports-shape-mismatch") {
		t.Errorf("missing adapter-supports-shape-mismatch; got %+v", outcomes)
	}
}

func TestValidateSupports_NonPresentationWithoutSupportsBlockFails(t *testing.T) {
	adapter := []byte(`name: nestjs-application
kind: application
`)
	outcomes := ValidateSupports(ModeBuild, adapter, nil)
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
	outcomes := ValidateSupports(ModeBuild, adapter, nil)
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
	outcomes := ValidateSupports(ModeBuild, adapter, caps)
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
	outcomes := ValidateSupports(ModeBuild, adapter, caps)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error: %+v", o)
		}
	}
}
