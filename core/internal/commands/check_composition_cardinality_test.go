package commands

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// oneToOne builds a model with a single one-to-one relationship from
// parent to child, realised by the named ref field on the child.
func oneToOne(t *testing.T, refField string) *config.DomainModelArtifact {
	t.Helper()
	return &config.DomainModelArtifact{
		Entities: []config.DomainEntity{
			{Name: "ExpenseReport", Fields: []config.DomainField{{Name: "id", Type: "uuid"}}},
			{Name: "Approval", Fields: []config.DomainField{
				{Name: "id", Type: "uuid"},
				{Name: refField, Type: "ref", Target: "ExpenseReport"},
			}},
		},
		Relationships: []config.DomainRelationship{
			{Name: "report-approval", From: "ExpenseReport", To: "Approval", Cardinality: "one-to-one"},
		},
	}
}

// Two children pointing at one parent under a one-to-one relationship is
// data the model itself forbids. Run 3 shipped exactly this — two mutually
// exclusive records for one report — and every gate passed because the
// ids differed.
func TestOneToOneHoldingTwoChildrenIsAViolation(t *testing.T) {
	errs, notes := findCardinalityViolations(oneToOne(t, "report"), []entityRecord{
		rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
		rec("approvals", "seed", "Approval", "app-2", map[string]interface{}{"report": "rep-1"}),
	})
	if len(errs) != 1 {
		t.Fatalf("want one violation, got %#v (notes: %#v)", errs, notes)
	}
	if errs[0].Code != "composition-cardinality-violated" {
		t.Errorf("unexpected code: %#v", errs[0])
	}
	if errs[0].ID != "rep-1" || errs[0].Field != "report" {
		t.Errorf("the finding must name the parent and the realising field: %#v", errs[0])
	}
}

// The same data under one-to-many is correct, and must produce nothing.
// The check's whole value depends on it not firing on a correct model.
func TestTheSameDataUnderOneToManyIsClean(t *testing.T) {
	model := oneToOne(t, "report")
	model.Relationships[0].Cardinality = "one-to-many"

	errs, notes := findCardinalityViolations(model, []entityRecord{
		rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
		rec("approvals", "seed", "Approval", "app-2", map[string]interface{}{"report": "rep-1"}),
	})
	if len(errs) != 0 || len(notes) != 0 {
		t.Errorf("one-to-many permits this data: errors %#v, notes %#v", errs, notes)
	}
}

// One child per parent satisfies the constraint.
func TestOneChildPerParentIsClean(t *testing.T) {
	errs, notes := findCardinalityViolations(oneToOne(t, "report"), []entityRecord{
		rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
		rec("approvals", "seed", "Approval", "app-2", map[string]interface{}{"report": "rep-2"}),
	})
	if len(errs) != 0 || len(notes) != 0 {
		t.Errorf("one child per parent is correct: errors %#v, notes %#v", errs, notes)
	}
}

// Two relationships between the same pair of entities make the join
// ambiguous. Picking one and failing on the guess would misfire on a
// correct model, so the check declines and says why.
func TestTwoCandidateFieldsAreUnresolvableNotGuessed(t *testing.T) {
	model := oneToOne(t, "report")
	model.Entities[1].Fields = append(model.Entities[1].Fields,
		config.DomainField{Name: "supersedes", Type: "ref", Target: "ExpenseReport"})

	errs, notes := findCardinalityViolations(model, []entityRecord{
		rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
		rec("approvals", "seed", "Approval", "app-2", map[string]interface{}{"report": "rep-1"}),
	})
	if len(errs) != 0 {
		t.Fatalf("an ambiguous join must not fail the build: %#v", errs)
	}
	if len(notes) != 1 || notes[0].Code != "composition-cardinality-unresolvable" {
		t.Fatalf("want one unresolvable note, got %#v", notes)
	}
	if !strings.Contains(notes[0].Message, "report") || !strings.Contains(notes[0].Message, "supersedes") {
		t.Errorf("the note must name the candidates so the author can disambiguate: %q", notes[0].Message)
	}
}

// The `relationship:` back-reference is what makes the ambiguity fixable.
// With it declared, the same two-candidate model checks cleanly.
func TestTheRelationshipBackReferenceSettlesTheAmbiguity(t *testing.T) {
	model := oneToOne(t, "report")
	model.Entities[1].Fields[1].Relationship = "report-approval"
	model.Entities[1].Fields = append(model.Entities[1].Fields,
		config.DomainField{Name: "supersedes", Type: "ref", Target: "ExpenseReport"})

	errs, notes := findCardinalityViolations(model, []entityRecord{
		rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
		rec("approvals", "seed", "Approval", "app-2", map[string]interface{}{"report": "rep-1"}),
	})
	if len(notes) != 0 {
		t.Fatalf("the back-reference settles the join, so there is nothing unresolvable: %#v", notes)
	}
	if len(errs) != 1 || errs[0].Code != "composition-cardinality-violated" {
		t.Fatalf("with the join settled the violation must be reported: %#v", errs)
	}
}

// Nothing on the child targets the parent, so no data realises the
// relationship at all. A note, not a violation.
func TestNoCandidateFieldIsUnresolvable(t *testing.T) {
	model := oneToOne(t, "report")
	model.Entities[1].Fields = model.Entities[1].Fields[:1] // drop the ref field

	errs, notes := findCardinalityViolations(model, nil)
	if len(errs) != 0 {
		t.Fatalf("want no errors, got %#v", errs)
	}
	if len(notes) != 1 || notes[0].Code != "composition-cardinality-unresolvable" {
		t.Fatalf("want one unresolvable note, got %#v", notes)
	}
}

// Only the composed seed counts. A scenario fixture describes a state the
// prototype never boots into, so two of its records hanging off one parent
// are not simultaneously true — the same reasoning that grades a scenario
// disagreement as a note rather than a contradiction.
func TestScenarioFixtureRecordsAreNotCounted(t *testing.T) {
	errs, _ := findCardinalityViolations(oneToOne(t, "report"), []entityRecord{
		rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
		scenarioRec("approvals", "rejection-scenario", "Approval", "app-2",
			map[string]interface{}{"report": "rep-1"}),
	})
	if len(errs) != 0 {
		t.Errorf("a scenario fixture does not boot alongside the seed: %#v", errs)
	}
}

// Only one-to-one is violable by counting. many-to-one and many-to-many
// cannot be, and the check must not invent findings for them — the
// documented scope has to match the behaviour.
func TestOnlyOneToOneIsChecked(t *testing.T) {
	for _, cardinality := range []string{"one-to-many", "many-to-one", "many-to-many"} {
		model := oneToOne(t, "report")
		model.Relationships[0].Cardinality = cardinality
		errs, notes := findCardinalityViolations(model, []entityRecord{
			rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
			rec("approvals", "seed", "Approval", "app-2", map[string]interface{}{"report": "rep-1"}),
		})
		if len(errs) != 0 || len(notes) != 0 {
			t.Errorf("%s produced findings: errors %#v, notes %#v", cardinality, errs, notes)
		}
	}
}

// A project with no domain model is a normal state — the check has
// nothing to hold the data against, and says nothing.
func TestNoDomainModelProducesNothing(t *testing.T) {
	errs, notes := findCardinalityViolations(nil, []entityRecord{
		rec("approvals", "seed", "Approval", "app-1", map[string]interface{}{"report": "rep-1"}),
	})
	if len(errs) != 0 || len(notes) != 0 {
		t.Errorf("no model means nothing to check: errors %#v, notes %#v", errs, notes)
	}
}
