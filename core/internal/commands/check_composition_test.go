package commands

import "testing"

func rec(feature, fixture, entity, id string, fields map[string]interface{}) entityRecord {
	f := map[string]interface{}{"id": id}
	for k, v := range fields {
		f[k] = v
	}
	return entityRecord{Entity: entity, ID: id, Fields: f, Feature: feature, Fixture: fixture}
}

// Two features disagreeing about the same entity is the finding. Nothing
// reconciles them, and a user navigating between the two pages sees both
// values — which is how one persona was a manager named Morgan Reyes on the
// dashboard and an employee named Nils Ahlgren in the expense list.
func TestCrossFeatureDisagreementIsReported(t *testing.T) {
	got := findContradictions([]entityRecord{
		rec("dashboard", "seed", "Employee", "emp-2", map[string]interface{}{"role": "manager"}),
		rec("expense-list", "new", "Employee", "emp-2", map[string]interface{}{"role": "employee"}),
	})
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d: %#v", len(got), got)
	}
	if got[0].Code != "composition-fixture-contradiction" || got[0].Field != "role" {
		t.Errorf("unexpected finding: %#v", got[0])
	}
}

// Within one feature, fixtures that disagree are the normal way to express
// alternative scenarios — an empty draft and a submitted report are supposed
// to be different states of the same id. Reporting those buries the real
// findings: the first version of this check produced them and they
// outnumbered the genuine ones.
func TestIntraFeatureScenarioFixturesAreNotContradictions(t *testing.T) {
	got := findContradictions([]entityRecord{
		rec("submit-expense", "empty-draft", "ExpenseReport", "rep-1", map[string]interface{}{"status": "draft"}),
		rec("submit-expense", "submitted", "ExpenseReport", "rep-1", map[string]interface{}{"status": "submitted"}),
	})
	if len(got) != 0 {
		t.Fatalf("intra-feature scenario fixtures reported as contradictions: %#v", got)
	}
}

// A value present in one feature and absent from another is not a
// disagreement about a shared runtime unless the features genuinely differ
// on it — a fixture that appears in both features with the same value plus
// one that differs must still report.
func TestAgreementAcrossFeaturesIsSilent(t *testing.T) {
	got := findContradictions([]entityRecord{
		rec("a", "f1", "Employee", "emp-1", map[string]interface{}{"role": "employee"}),
		rec("b", "f2", "Employee", "emp-1", map[string]interface{}{"role": "employee"}),
	})
	if len(got) != 0 {
		t.Fatalf("agreeing fixtures reported: %#v", got)
	}
}

// A reference to an id no fixture defines means the flow depending on it
// cannot be reached. The login form offered emp-2 while the fixture defined
// emp-3, so the account-lockout flow it existed to demonstrate was
// unreachable — and nothing caught it, because the form and the fixture live
// in different features.
func TestDanglingReferenceIsReported(t *testing.T) {
	got := findDanglingReferences([]entityRecord{
		rec("expense-list", "f", "Employee", "emp-3", nil),
		rec("expense-list", "f", "ExpenseReport", "rep-1", map[string]interface{}{"employee": "emp-9"}),
	})
	if len(got) != 1 {
		t.Fatalf("want 1 dangling finding, got %#v", got)
	}
	if got[0].ID != "emp-9" || got[0].Field != "employee" {
		t.Errorf("unexpected: %#v", got[0])
	}
}

// Prose must not be mistaken for an id. Without the prefix test every
// description string in every fixture becomes a candidate reference.
func TestProseIsNotTreatedAsAReference(t *testing.T) {
	got := findDanglingReferences([]entityRecord{
		rec("f", "x", "Employee", "emp-1", nil),
		rec("f", "x", "ExpenseReport", "rep-1", map[string]interface{}{
			"title":       "Berlin conference",
			"description": "a multi-word note - with a dash",
			"employee":    "emp-1",
		}),
	})
	if len(got) != 0 {
		t.Fatalf("prose reported as dangling references: %#v", got)
	}
}

// Resolved references are silent.
func TestResolvedReferenceIsSilent(t *testing.T) {
	got := findDanglingReferences([]entityRecord{
		rec("a", "f", "Employee", "emp-1", nil),
		rec("b", "f", "ExpenseReport", "rep-1", map[string]interface{}{"employee": "emp-1"}),
	})
	if len(got) != 0 {
		t.Fatalf("resolved reference reported: %#v", got)
	}
}
