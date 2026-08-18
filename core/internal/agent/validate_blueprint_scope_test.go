// parlay-feature: parlay-tool/multi-adapter
// parlay-component: blueprint-scope-and-precedence
// parlay-artifact: test

package agent

import (
	"strings"
	"testing"
)

func TestValidateBlueprintScope_TopologyRejected(t *testing.T) {
	content := []byte(`data:
  fetching: stale-while-revalidate
targets:
  presentation: { adapter: react-antd, root: src }
`)
	outcomes := ValidateBlueprintScope(ModeBuild, "test", content)
	if !findCode(outcomes, "blueprint-topology-not-allowed") {
		t.Errorf("missing blueprint-topology-not-allowed; got %+v", outcomes)
	}
}

func TestValidateBlueprintScope_OutOfScopeKey(t *testing.T) {
	content := []byte(`data: {}
deployment:
  region: us-east-1
`)
	outcomes := ValidateBlueprintScope(ModeBuild, "test", content)
	if !findCode(outcomes, "blueprint-scope-violation") {
		t.Errorf("missing blueprint-scope-violation; got %+v", outcomes)
	}
}

// The two strategy tests that used to live here are gone with the gate they
// covered. Both passed against the wrong thing: one asserted `caching:
// per-route`, `auth.strategy: jwt` and `retry: writes` were accepted, and they
// were — but only because the scalar shapes made the unmarshal fail, so the
// whole gate was skipped and NOTHING was checked. A green test over a check
// that never ran is what kept three wrong vocabularies alive. Strategy
// coverage now sits with the owner, in validate_test.go, at the schema's
// shapes.

func TestValidateBlueprintScope_AllowedScopePasses(t *testing.T) {
	// Every key blueprint.schema.md's "Owned scope" section documents. This
	// fixture used to read `auth:` with no `shells:` — the vocabulary the map
	// had, not the one the schema documented — so the disagreement between the
	// two validators was locked in by its own test.
	content := []byte(`app: demo
shells: {}
navigation: {}
authorization:
  strategy: role-based
data:
  fetching: stale-while-revalidate
errors: {}
state: {}
platform: {}
`)
	outcomes := ValidateBlueprintScope(ModeBuild, "test", content)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error: %+v", o)
		}
	}
}

// TestValidateBlueprintScope_SchemaShapedBlueprintIsClean is the regression
// test for the reported defect: a blueprint written straight from the schema
// body produced two blueprint-scope-violation errors on the project pass while
// `validate --type blueprint` called the same file clean.
func TestValidateBlueprintScope_SchemaShapedBlueprintIsClean(t *testing.T) {
	content := []byte(`app: plateworks
shells:
  main:
    regions: [header, sidebar, content]
authorization:
  strategy: permission-based
`)
	outcomes := ValidateBlueprintScope(ModeBuild, "test", content)
	for _, o := range outcomes {
		if o.Code == "blueprint-scope-violation" {
			t.Errorf("schema-documented key rejected: %+v", o)
		}
	}
}

// TestBlueprintScopeMessageMatchesTheCheck pins the message to the map. The
// literal it replaced named six keys while the map allowed seven, so the
// sentence explaining the rejection could disagree with the rule that caused
// it.
func TestBlueprintScopeMessageMatchesTheCheck(t *testing.T) {
	list := blueprintScopeList()
	for key := range blueprintAllowedScope {
		if !strings.Contains(list, key) {
			t.Errorf("closed-scope message omits allowed key %q: %s", key, list)
		}
	}
}

func TestValidateBlueprintStrategy_UnknownVocab(t *testing.T) {
	vocab := map[string]bool{"on-mount": true, "stale-while-revalidate": true}
	support := map[string]bool{"on-mount": true}
	outcomes := ValidateBlueprintStrategy(ModeBuild, "data.fetching", "telepathy", vocab, support)
	if !findCode(outcomes, "blueprint-strategy-unknown") {
		t.Errorf("missing blueprint-strategy-unknown; got %+v", outcomes)
	}
}

func TestValidateBlueprintStrategy_VocabButUnsupported(t *testing.T) {
	vocab := map[string]bool{"on-mount": true, "stale-while-revalidate": true}
	support := map[string]bool{"on-mount": true}
	outcomes := ValidateBlueprintStrategy(ModeBuild, "data.fetching", "stale-while-revalidate", vocab, support)
	if !findCode(outcomes, "blueprint-strategy-unsupported") {
		t.Errorf("missing blueprint-strategy-unsupported; got %+v", outcomes)
	}
}
