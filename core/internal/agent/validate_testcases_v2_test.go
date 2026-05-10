// parlay-feature: parlay-tool/multi-adapter
// parlay-component: testcases-v2
// parlay-artifact: test

package agent

import "testing"

func TestValidateTestcasesV2_OperationUncovered(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: presentation
    component: x
    source_refs: ["@f/x"]
`)
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, []string{"@f/operation:task.delete"})
	if !findCode(outcomes, "testcases-operation-uncovered") {
		t.Errorf("missing testcases-operation-uncovered; got %+v", outcomes)
	}
}

func TestValidateTestcasesV2_SourceRefsMissing(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: presentation
    component: x
`)
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
	if !findCode(outcomes, "testcases-source-refs-missing") {
		t.Errorf("missing testcases-source-refs-missing; got %+v", outcomes)
	}
}

func TestValidateTestcasesV2_LegacyV1Warning(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    component: x
    intent: legacy intent
`)
	outcomes := ValidateTestcasesV2(ModeAuthoring, "test", content, nil)
	if !findCode(outcomes, "testcases-source-refs-missing-legacy") {
		t.Errorf("missing testcases-source-refs-missing-legacy; got %+v", outcomes)
	}
	for _, o := range outcomes {
		if o.Code == "testcases-source-refs-missing-legacy" && o.Severity != SeverityWarning {
			t.Errorf("legacy warning should be SeverityWarning; got %v", o.Severity)
		}
	}
}

func TestValidateTestcasesV2_UnknownKind(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: integration
    source_refs: ["@f/x"]
`)
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
	if !findCode(outcomes, "testcases-suite-kind-unknown") {
		t.Errorf("missing testcases-suite-kind-unknown; got %+v", outcomes)
	}
}

func TestValidateTestcasesV2_OperationCoverageWalker(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: operation
    operation: "@f/operation:task.create"
    source_refs: ["@f/operation:task.create"]
`)
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, []string{"@f/operation:task.create"})
	if findCode(outcomes, "testcases-operation-uncovered") {
		t.Errorf("operation suite should cover the canonical op; got %+v", outcomes)
	}
}
