// parlay-feature: domain-model-editor/feature-contributions
// parlay-component: cross-cutting/contribution-impact
// parlay-artifact: test

package agent

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/editor/domain"
)

func rootModel() domain.Model {
	return domain.Model{
		SchemaVersion: 1,
		Entities: []domain.Entity{
			{Name: "ExpenseReport", Fields: []domain.Field{{Name: "id", Type: "uuid"}}},
		},
	}
}

// A new field on an entity other features already use is the case the report
// exists for: the proposal is one line, and the work it creates is spread
// across everyone else's fixtures.
func TestImpactNamesTheFeaturesAndFixturesANewFieldReaches(t *testing.T) {
	contribution := domain.Model{SchemaVersion: 1, Entities: []domain.Entity{
		{Name: "ExpenseReport", Fields: []domain.Field{
			{Name: "id", Type: "uuid"},
			{Name: "settledAt", Type: "datetime"},
		}},
	}}
	facts := ProjectFacts{
		EntityUsers: map[string][]string{
			"ExpenseReport": {"dashboard", "expense-list", "submit-expense"},
		},
		Fixtures: []FixtureEntity{
			{Feature: "dashboard", Fixture: "seed", Entity: "ExpenseReport"},
			{Feature: "expense-list", Fixture: "mixed-status", Entity: "ExpenseReport"},
			{Feature: "submit-expense", Fixture: "seed", Entity: "ExpenseReport"},
			{Feature: "dashboard", Fixture: "seed", Entity: "Employee"},
		},
	}

	imp := Impact("submit-expense", "p", rootModel(), contribution, facts)

	if !imp.Applicable {
		t.Fatalf("an additive contribution is applicable: %#v", imp.Conflicts)
	}
	if len(imp.Affects) != 1 || imp.Affects[0].Entity != "ExpenseReport" {
		t.Fatalf("affects = %#v", imp.Affects)
	}
	a := imp.Affects[0]

	// The proposing feature is not its own audience — it already knows.
	want := []string{"dashboard", "expense-list"}
	if len(a.Features) != len(want) {
		t.Fatalf("features = %v, want %v (the proposer is excluded)", a.Features, want)
	}
	for i := range want {
		if a.Features[i] != want[i] {
			t.Errorf("features = %v, want %v", a.Features, want)
		}
	}

	if len(a.Fixtures) != 2 {
		t.Fatalf("fixtures = %#v — the proposer's own fixture is excluded, Employee is unrelated", a.Fixtures)
	}
	if a.Fixtures[0].Feature != "dashboard" || a.Fixtures[0].Fixture != "seed" {
		t.Errorf("fixtures[0] = %#v", a.Fixtures[0])
	}
	if len(a.Fixtures[0].Fields) != 1 || a.Fixtures[0].Fields[0] != "settledAt" {
		t.Errorf("the fixture entry must name the field it would need: %#v", a.Fixtures[0])
	}
}

// A brand-new entity reaches nobody: no existing feature references it and no
// existing fixture holds one. Reporting an empty audience would read as "we
// checked and found people".
func TestABrandNewEntityHasNoAudience(t *testing.T) {
	contribution := domain.Model{SchemaVersion: 1, Entities: []domain.Entity{
		{Name: "Approval", Fields: []domain.Field{{Name: "id", Type: "uuid"}}},
	}}
	imp := Impact("approvals", "p", rootModel(), contribution, ProjectFacts{
		EntityUsers: map[string][]string{"ExpenseReport": {"dashboard"}},
	})

	if len(imp.Additions) != 1 || imp.Additions[0].Kind != domain.KindEntity {
		t.Fatalf("additions = %#v", imp.Additions)
	}
	if len(imp.Affects) != 0 {
		t.Errorf("a new entity affects nobody yet: %#v", imp.Affects)
	}
}

// A conflict makes the contribution inapplicable, and the people relying on
// the root's description are exactly who needs to know.
func TestAConflictIsReportedAndBlocksApplication(t *testing.T) {
	contribution := domain.Model{SchemaVersion: 1, Entities: []domain.Entity{
		{Name: "ExpenseReport", Fields: []domain.Field{{Name: "id", Type: "string"}}},
	}}
	imp := Impact("submit-expense", "p", rootModel(), contribution, ProjectFacts{
		EntityUsers: map[string][]string{"ExpenseReport": {"dashboard"}},
	})

	if imp.Applicable {
		t.Error("a conflicting contribution is not applicable")
	}
	if len(imp.Conflicts) != 1 {
		t.Fatalf("conflicts = %#v", imp.Conflicts)
	}
	if len(imp.Affects) != 1 || len(imp.Affects[0].Features) != 1 {
		t.Errorf("a conflict must still name who relies on the root's description: %#v", imp.Affects)
	}
}

func TestProposedEntitiesMapsEachNameToItsProposer(t *testing.T) {
	got := ProposedEntities(map[string]domain.Model{
		"approvals": {Entities: []domain.Entity{{Name: "Approval"}}},
		"payments":  {Entities: []domain.Entity{{Name: "Payment"}}},
	})
	if got["Approval"] != "approvals" || got["Payment"] != "payments" {
		t.Errorf("proposals = %#v", got)
	}
}

// Two features proposing the same name is possible mid-authoring. Whichever
// is named, the answer has to be the same on every run — a message that
// changes between two identical runs reads as the project changing.
func TestASharedProposalNamesAStableProposer(t *testing.T) {
	in := map[string]domain.Model{
		"zebra": {Entities: []domain.Entity{{Name: "Approval"}}},
		"alpha": {Entities: []domain.Entity{{Name: "Approval"}}},
	}
	for i := 0; i < 20; i++ {
		if got := ProposedEntities(in)["Approval"]; got != "alpha" {
			t.Fatalf("proposer = %q, want the first in sorted feature order", got)
		}
	}
}

const capsReferencingApproval = `schema_version: 1
feature: approvals
operations:
  - id: decide
    kind: command
    subject:
      entity: Approval
    steps:
      - type: update-one
`

// The reference is to an entity a sibling feature proposes. Before this, that
// was indistinguishable from a typo and graded the same way, which is what
// forced two features to ship placeholders.
func TestAReferenceToAProposedEntityIsPendingNotUndeclared(t *testing.T) {
	outcomes := ValidateCapabilitiesWithProposals(ModeBuild, "capabilities.yaml",
		[]byte(capsReferencingApproval),
		[]string{"ExpenseReport"},
		map[string]string{"Approval": "approvals-feature"})

	var pending, undeclared int
	var message string
	for _, o := range outcomes {
		switch o.Code {
		case "capabilities-entity-pending":
			pending++
			message = o.Message
		case "capabilities-entity-undeclared":
			undeclared++
		}
	}
	if pending != 1 {
		t.Fatalf("want one capabilities-entity-pending, got %v", codesOf(outcomes))
	}
	if undeclared != 0 {
		t.Errorf("the undeclared finding must be replaced, not accompanied — two findings about one reference leaves a reader unsure which applies")
	}
	if !strings.Contains(message, "approvals-feature") {
		t.Errorf("the finding must name the proposing feature: %q", message)
	}
}

// A genuine typo is still an error. The softening applies only to names some
// contribution actually proposes.
func TestAnUnproposedUndeclaredEntityIsStillAnError(t *testing.T) {
	outcomes := ValidateCapabilitiesWithProposals(ModeBuild, "capabilities.yaml",
		[]byte(capsReferencingApproval),
		[]string{"ExpenseReport"},
		map[string]string{"SomethingElse": "other-feature"})

	if !hasOutcomeCode(outcomes, "capabilities-entity-undeclared") {
		t.Errorf("want capabilities-entity-undeclared for a name nothing proposes, got %v", codesOf(outcomes))
	}
	if hasOutcomeCode(outcomes, "capabilities-entity-pending") {
		t.Errorf("nothing proposes this entity, so it is not pending: %v", codesOf(outcomes))
	}
}

// A project with no contributions must behave exactly as it did before they
// existed.
func TestNoProposalsLeavesTheValidatorUnchanged(t *testing.T) {
	with := ValidateCapabilitiesWithProposals(ModeBuild, "capabilities.yaml",
		[]byte(capsReferencingApproval), []string{"ExpenseReport"}, nil)
	without := ValidateCapabilities(ModeBuild, "capabilities.yaml",
		[]byte(capsReferencingApproval), []string{"ExpenseReport"})

	if len(with) != len(without) {
		t.Fatalf("with proposals %v, without %v", codesOf(with), codesOf(without))
	}
	for i := range with {
		if with[i].Code != without[i].Code || with[i].Message != without[i].Message {
			t.Errorf("finding %d differs: %#v vs %#v", i, with[i], without[i])
		}
	}
}

func TestCapabilityEntitiesReadsSubjectAndOutput(t *testing.T) {
	const caps = `schema_version: 1
feature: reporting
operations:
  - id: list
    kind: query
    subject:
      entity: ExpenseReport
    output:
      shape: many
      entity: LineItem
    steps:
      - type: read-many
`
	got := CapabilityEntities("capabilities.yaml", []byte(caps))
	if len(got) != 2 || got[0] != "ExpenseReport" || got[1] != "LineItem" {
		t.Errorf("entities = %v, want [ExpenseReport LineItem]", got)
	}
}
