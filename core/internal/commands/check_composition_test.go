package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// setupCompositionProject builds a temp tree with the named features present
// in spec/intents/ and, for each one that has fixture YAML, a buildfile under
// .parlay/build/. Feature names may be qualified ("approvals/review-queue"),
// which is the case the walk used to drop.
func setupCompositionProject(t *testing.T, fixtures map[string]string) *config.Context {
	t.Helper()
	dir := setupTestDir(t)

	for feature, buildfile := range fixtures {
		featDir := filepath.Join(dir, config.SpecDir, config.IntentsDir, filepath.FromSlash(feature))
		if err := os.MkdirAll(featDir, 0755); err != nil {
			t.Fatal(err)
		}
		// config.ClassifyDir requires intents.md before a directory counts as
		// a feature, so AllFeatures() would not see a bare directory.
		if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte("# "+feature+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if buildfile == "" {
			continue
		}
		buildDir := filepath.Join(dir, config.ParlayDir, config.BuildDir, filepath.FromSlash(feature))
		if err := os.MkdirAll(buildDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte(buildfile), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return testContext(t)
}

// The composition walk and the canonical feature enumeration must return the
// same set. This is the structural assertion, not a spot check: the walk was
// a hand-rolled single-level os.ReadDir that silently skipped every
// initiative-scoped feature, so `check-composition` reported coherent having
// examined half the project. Asserting equality with cfg.AllFeatures() fails
// on any future re-hand-rolling of the traversal, which is the actual
// regression — a second enumeration that drifts from the first.
func TestCompositionWalkReachesNestedFeatures(t *testing.T) {
	const bf = `feature: f
fixtures:
  seed:
    data:
      Employee:
        - id: emp-1
          role: employee
`
	cfg := setupCompositionProject(t, map[string]string{
		"submit-expense":             bf,
		"approvals/review-queue":     bf,
		"approvals/approval-history": bf,
	})

	all, err := cfg.AllFeatures()
	if err != nil {
		t.Fatalf("AllFeatures: %v", err)
	}
	records, contributing, findings := collectFixtureRecords(cfg, all)

	if len(contributing) != len(all) {
		t.Fatalf("walk saw %d features, canonical enumeration has %d: %v vs %v",
			len(contributing), len(all), contributing, all)
	}
	for _, want := range []string{"approvals/review-queue", "approvals/approval-history"} {
		found := false
		for _, got := range contributing {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("nested feature %q missing from walk: %v", want, contributing)
		}
	}
	if len(records) != 3 {
		t.Errorf("want one record per feature, got %d", len(records))
	}
	if len(findings) != 0 {
		t.Errorf("fully-built project produced walk findings: %#v", findings)
	}
}

// Two features under the SAME initiative disagreeing must report. The feature
// used to be recovered from a "feature/fixture" label with
// strings.SplitN(site, "/", 2)[0], which yields "approvals" for both
// approvals/review-queue and approvals/approval-history — so they compared
// equal, spansMultipleFeatures returned false, and a genuine contradiction
// was suppressed. This fails with the walk fixed but the attribution not.
func TestSiblingFeaturesUnderOneInitiativeCanContradict(t *testing.T) {
	got := findContradictions([]entityRecord{
		rec("approvals/review-queue", "queue", "ExpenseReport", "rep-1",
			map[string]interface{}{"status": "submitted"}),
		rec("approvals/approval-history", "history", "ExpenseReport", "rep-1",
			map[string]interface{}{"status": "approved"}),
	})
	if len(got) != 1 {
		t.Fatalf("sibling features under one initiative must be able to contradict; got %d findings: %#v", len(got), got)
	}
	if got[0].Field != "status" || got[0].ID != "rep-1" {
		t.Errorf("unexpected finding: %#v", got[0])
	}
}

// A feature with intents but no buildfile contributes nothing to the composed
// runtime. The old walk skipped it silently, so a half-built project and a
// fully-built one produced identical output.
func TestUnbuiltFeatureIsReportedNotSkipped(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"submit-expense": "feature: submit-expense\nfixtures: {}\n",
		"dashboard":      "", // intents only, never built
	})
	all, err := cfg.AllFeatures()
	if err != nil {
		t.Fatalf("AllFeatures: %v", err)
	}
	_, contributing, notes := collectFixtureRecords(cfg, all)

	if len(all) != 2 {
		t.Fatalf("want 2 features enumerated, got %v", all)
	}
	if len(contributing) != 1 {
		t.Errorf("want 1 contributing feature, got %v", contributing)
	}
	if len(notes) != 1 || notes[0].Code != "composition-feature-unbuilt" {
		t.Fatalf("want one composition-feature-unbuilt note, got %#v", notes)
	}
}

// An unbuilt feature is a coverage fact, not an incoherence. It must be
// reported and must NOT flip the verdict: mid-pipeline, some features always
// lack buildfiles, and failing there would make this command unusable during
// the run it is meant to guard.
func TestUnbuiltFeatureDoesNotBreakCoherence(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"submit-expense": "feature: submit-expense\nfixtures: {}\n",
		"dashboard":      "",
	})
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runCheckComposition(cmd, nil); err != nil {
		t.Fatalf("unbuilt feature must not fail the check: %v", err)
	}
	var out compositionOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !out.Coherent {
		t.Errorf("want coherent:true with only an unbuilt-feature note, got %#v", out)
	}
	if out.Examined != 2 {
		t.Errorf("want features_examined:2, got %d", out.Examined)
	}
	if len(out.Notes) != 1 {
		t.Errorf("want the unbuilt feature reported as a note, got %#v", out.Notes)
	}
}

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
