// parlay-feature: parlay-tool/multi-adapter
// parlay-component: blueprint-scope-and-precedence
// parlay-artifact: test

package agent

import "testing"

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

func TestValidateBlueprintScope_StrategyOutOfVocab(t *testing.T) {
	content := []byte(`data:
  fetching: telepathy
`)
	outcomes := ValidateBlueprintScope(ModeBuild, "test", content)
	if !findCode(outcomes, "blueprint-strategy-unknown") {
		t.Errorf("missing blueprint-strategy-unknown for telepathy; got %+v", outcomes)
	}
}

func TestValidateBlueprintScope_StrategyAcceptsClosedVocab(t *testing.T) {
	content := []byte(`data:
  fetching: stale-while-revalidate
  caching: per-route
auth:
  strategy: jwt
errors:
  retry: writes
`)
	outcomes := ValidateBlueprintScope(ModeBuild, "test", content)
	if findCode(outcomes, "blueprint-strategy-unknown") {
		t.Errorf("closed-vocab values should not trigger unknown; got %+v", outcomes)
	}
}

func TestValidateBlueprintScope_AllowedScopePasses(t *testing.T) {
	content := []byte(`data:
  fetching: stale-while-revalidate
auth:
  strategy: jwt
errors:
  retry: writes
state: {}
navigation: {}
platform: {}
`)
	outcomes := ValidateBlueprintScope(ModeBuild, "test", content)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error: %+v", o)
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
