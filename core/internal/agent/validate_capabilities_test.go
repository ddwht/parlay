// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-artifact
// parlay-artifact: test

package agent

import "testing"

func TestValidateCapabilities_ClosedFormPasses(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.create
    kind: command
    subject: { entity: Task }
    errors: [validation-failed]
    policies: [transaction-required]
    steps:
      - { type: validate-input }
      - { type: create-one }
      - { type: return-one }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error: %+v", o)
		}
	}
}

func TestValidateCapabilities_SubscriptionV2Deferred(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.watch
    kind: subscription
    subject: { entity: Task }
    steps:
      - { type: validate-input }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	if !findCode(outcomes, "capabilities-unknown-term") {
		t.Errorf("missing capabilities-unknown-term; got %+v", outcomes)
	}
	if !findMessage(outcomes, "v2-deferred") {
		t.Errorf("expected v2-deferred mention; got %+v", outcomes)
	}
}

func TestValidateCapabilities_DuplicateID(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.create
    kind: command
    subject: { entity: Task }
    steps: [{ type: validate-input }]
  - id: task.create
    kind: query
    subject: { entity: Task }
    steps: [{ type: read-one }]
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	if !findCode(outcomes, "capabilities-duplicate-operation-id") {
		t.Errorf("missing capabilities-duplicate-operation-id; got %+v", outcomes)
	}
}

func TestValidateCapabilities_StubKindUnknown(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.unknown
    kind: unknown
    subject: { entity: Task }
    steps: []
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	if !findCode(outcomes, "capabilities-stub-unfilled") {
		t.Errorf("missing capabilities-stub-unfilled; got %+v", outcomes)
	}
}

func TestValidateCapabilities_UnknownStep(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.search
    kind: query
    subject: { entity: Task }
    steps:
      - { type: telepathy }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	found := false
	for _, o := range outcomes {
		if o.Code == "capabilities-unknown-term" && findMessage([]ValidationOutcome{o}, "telepathy") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing capabilities-unknown-term naming telepathy; got %+v", outcomes)
	}
}

func TestValidateOperationRefNormalized(t *testing.T) {
	if outcome, ok := ValidateOperationRefNormalized(ModeBuild, "@task-list/operation:task.create"); !ok {
		t.Errorf("normalized ref rejected: %+v", outcome)
	}
	if outcome, ok := ValidateOperationRefNormalized(ModeBuild, "task.create"); ok {
		t.Errorf("bare ref accepted: %+v", outcome)
	}
}
