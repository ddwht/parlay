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

func TestValidateCapabilities_PolicyTieRulesSatisfied(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.delete
    kind: command
    subject: { entity: Task }
    errors: [unauthorized, forbidden]
    policies: [auth-required, permission-required]
    steps:
      - { type: authorize }
      - { type: delete-one }
      - { type: return-empty }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	if findCode(outcomes, "capabilities-policy-missing-step") || findCode(outcomes, "capabilities-policy-missing-error") {
		t.Errorf("policy tie rules should be satisfied; got %+v", outcomes)
	}
}

func TestValidateCapabilities_AuthRequiredMissingStepAndError(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.delete
    kind: command
    subject: { entity: Task }
    policies: [auth-required]
    steps:
      - { type: delete-one }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	if !findCode(outcomes, "capabilities-policy-missing-step") {
		t.Errorf("missing capabilities-policy-missing-step; got %+v", outcomes)
	}
	if !findCode(outcomes, "capabilities-policy-missing-error") {
		t.Errorf("missing capabilities-policy-missing-error; got %+v", outcomes)
	}
}

func TestValidateCapabilities_PermissionRequiredMissingStepAndError(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.delete
    kind: command
    subject: { entity: Task }
    policies: [permission-required]
    steps:
      - { type: delete-one }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	if !findCode(outcomes, "capabilities-policy-missing-step") {
		t.Errorf("missing capabilities-policy-missing-step; got %+v", outcomes)
	}
	if !findCode(outcomes, "capabilities-policy-missing-error") {
		t.Errorf("missing capabilities-policy-missing-error; got %+v", outcomes)
	}
}

func TestValidateCapabilities_TransactionRequiredHasNoTie(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.create
    kind: command
    subject: { entity: Task }
    policies: [transaction-required]
    steps:
      - { type: create-one }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content)
	if findCode(outcomes, "capabilities-policy-missing-step") || findCode(outcomes, "capabilities-policy-missing-error") {
		t.Errorf("transaction-required has no tied step/error; got %+v", outcomes)
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
