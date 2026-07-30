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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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
	outcomes := ValidateCapabilities(ModeBuild, "test", content, nil)
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

// Suite: subject.entity / output.entity cross-reference (D10)
//
// capabilities.schema.md stated this check as settled fact — it explained what
// input.type lacks by contrasting it with "the way subject.entity/output.entity
// are references into domain-model.yaml's declared entities". Nothing performed
// it: parser.CapabilityOperation.Subject was decoded and read by no code, so an
// operation could name an entity that did not exist and every validator passed
// it. The failure then surfaced in build-feature or codegen as a wiring problem
// with no obvious cause.

const capsWithEntity = `schema_version: 1
feature: task-list
operations:
  - id: task.create
    kind: command
    subject: { entity: Task }
    output: { shape: one, entity: Task }
    steps:
      - { type: validate-input }
      - { type: create-one }
      - { type: return-one }
`

func TestValidateCapabilities_SubjectEntityMustBeDeclared(t *testing.T) {
	outcomes := ValidateCapabilities(ModeBuild, "test", []byte(capsWithEntity), []string{"Project", "User"})
	if !findCode(outcomes, "capabilities-entity-undeclared") {
		t.Fatalf("subject.entity Task is undeclared but no finding; got %+v", outcomes)
	}
	// The message has to name both the offender and what IS declared — otherwise
	// the author has to go read the domain model to learn what they may write.
	if !findMessage(outcomes, "Task") {
		t.Error("finding does not name the undeclared entity")
	}
	if !findMessage(outcomes, "Project") {
		t.Error("finding does not list the declared entities")
	}
}

func TestValidateCapabilities_DeclaredEntityAccepted(t *testing.T) {
	outcomes := ValidateCapabilities(ModeBuild, "test", []byte(capsWithEntity), []string{"Task"})
	if findCode(outcomes, "capabilities-entity-undeclared") {
		t.Fatalf("declared entity Task was rejected; got %+v", outcomes)
	}
}

// output.entity carries the same reference and the schema names the two
// together, so it must be checked too — not just subject.
func TestValidateCapabilities_OutputEntityMustBeDeclared(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.get
    kind: query
    subject: { entity: Task }
    output: { shape: one, entity: Ghost }
    steps:
      - { type: return-one }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content, []string{"Task"})
	if !findCode(outcomes, "capabilities-entity-undeclared") {
		t.Fatalf("output.entity Ghost is undeclared but no finding; got %+v", outcomes)
	}
	if !findMessage(outcomes, "Ghost") {
		t.Error("finding does not name the undeclared output entity")
	}
}

// No domain model must not mean "no valid entities". A project that has not
// authored one yet is a normal state, and firing on every operation there would
// make the check hardest on the projects least ready for it.
func TestValidateCapabilities_NoDomainModelSkipsCrossReference(t *testing.T) {
	for _, entities := range [][]string{nil, {}} {
		outcomes := ValidateCapabilities(ModeBuild, "test", []byte(capsWithEntity), entities)
		if findCode(outcomes, "capabilities-entity-undeclared") {
			t.Fatalf("cross-reference fired with no declared entities (%v); got %+v", entities, outcomes)
		}
	}
	// And the old signature must behave identically — it is the no-entities path.
	if findCode(ValidateCapabilities(ModeBuild, "test", []byte(capsWithEntity), nil), "capabilities-entity-undeclared") {
		t.Error("ValidateCapabilities cross-referenced without a domain model")
	}
}

// subject is Required in the field reference, and that was unenforced too: an
// operation with no subject validated cleanly and then had nothing for
// build-feature to wire against.
func TestValidateCapabilities_MissingSubjectRejected(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.create
    kind: command
    steps:
      - { type: create-one }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content, []string{"Task"})
	if !findCode(outcomes, "capabilities-subject-missing") {
		t.Fatalf("operation with no subject was accepted; got %+v", outcomes)
	}
	// A missing subject must not also be reported as an undeclared entity —
	// one problem, one finding.
	if findCode(outcomes, "capabilities-entity-undeclared") {
		t.Error("a missing subject was also reported as undeclared; that is one condition reported twice")
	}
}

// An empty output shape returns nothing, so it declares no entity and must not
// be cross-referenced.
func TestValidateCapabilities_EmptyOutputShapeNeedsNoEntity(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list
operations:
  - id: task.delete
    kind: command
    subject: { entity: Task }
    output: { shape: empty }
    steps:
      - { type: delete-one }
`)
	outcomes := ValidateCapabilities(ModeBuild, "test", content, []string{"Task"})
	if findCode(outcomes, "capabilities-entity-undeclared") {
		t.Fatalf("an empty output shape was cross-referenced; got %+v", outcomes)
	}
}
