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
	got, notes := findContradictions([]entityRecord{
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
	if len(notes) != 0 {
		t.Errorf("both sides compose into the runtime seed, so this is an error and not a note: %#v", notes)
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

// rec builds a record from the fixture its feature composes into the
// runtime seed — the case where two features' values really do coexist on
// screen. Use scenarioRec for a fixture that never reaches the composed
// runtime.
func rec(feature, fixture, entity, id string, fields map[string]interface{}) entityRecord {
	r := scenarioRec(feature, fixture, entity, id, fields)
	r.Composing = true
	return r
}

func scenarioRec(feature, fixture, entity, id string, fields map[string]interface{}) entityRecord {
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
	got, notes := findContradictions([]entityRecord{
		rec("dashboard", "seed", "Employee", "emp-2", map[string]interface{}{"role": "manager"}),
		rec("expense-list", "new", "Employee", "emp-2", map[string]interface{}{"role": "employee"}),
	})
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d: %#v", len(got), got)
	}
	if got[0].Code != "composition-fixture-contradiction" || got[0].Field != "role" {
		t.Errorf("unexpected finding: %#v", got[0])
	}
	if len(notes) != 0 {
		t.Errorf("both sides compose into the runtime seed, so this is an error and not a note: %#v", notes)
	}
}

// Within one feature, fixtures that disagree are the normal way to express
// alternative scenarios — an empty draft and a submitted report are supposed
// to be different states of the same id. Reporting those buries the real
// findings: the first version of this check produced them and they
// outnumbered the genuine ones.
func TestIntraFeatureScenarioFixturesAreNotContradictions(t *testing.T) {
	got, notes := findContradictions([]entityRecord{
		rec("submit-expense", "empty-draft", "ExpenseReport", "rep-1", map[string]interface{}{"status": "draft"}),
		rec("submit-expense", "submitted", "ExpenseReport", "rep-1", map[string]interface{}{"status": "submitted"}),
	})
	if len(got) != 0 {
		t.Fatalf("intra-feature scenario fixtures reported as contradictions: %#v", got)
	}
	if len(notes) != 0 {
		t.Errorf("an intra-feature disagreement is not reportable at either grade: %#v", notes)
	}
}

// A value present in one feature and absent from another is not a
// disagreement about a shared runtime unless the features genuinely differ
// on it — a fixture that appears in both features with the same value plus
// one that differs must still report.
func TestAgreementAcrossFeaturesIsSilent(t *testing.T) {
	got, notes := findContradictions([]entityRecord{
		rec("a", "f1", "Employee", "emp-1", map[string]interface{}{"role": "employee"}),
		rec("b", "f2", "Employee", "emp-1", map[string]interface{}{"role": "employee"}),
	})
	if len(got) != 0 {
		t.Fatalf("agreeing fixtures reported: %#v", got)
	}
	if len(notes) != 0 {
		t.Errorf("agreeing fixtures reported as a note: %#v", notes)
	}
}

// The split this stage introduces. The check already ignores fixtures that
// disagree *within* one feature, because alternative scenarios are supposed
// to differ. `composes: true` extends the same reasoning across features: a
// fixture that never reaches the composed seed describes a state the running
// prototype never boots into, so it cannot be on screen next to anything.
// Grading that an error is what forced a run-3 build agent to renumber a
// scenario fixture for no reason.
func TestOnlyComposingFixturesContradict(t *testing.T) {
	errors, notes := findContradictions([]entityRecord{
		rec("dashboard", "seed", "ExpenseReport", "rep-1",
			map[string]interface{}{"status": "submitted"}),
		scenarioRec("approvals", "rejection-scenario", "ExpenseReport", "rep-1",
			map[string]interface{}{"status": "rejected"}),
	})
	if len(errors) != 0 {
		t.Fatalf("a scenario fixture cannot contradict the composed runtime: %#v", errors)
	}
	if len(notes) != 1 {
		t.Fatalf("want the divergence reported as a note, got %#v", notes)
	}
	if notes[0].Code != "composition-scenario-fixture-divergence" {
		t.Errorf("unexpected note code: %#v", notes[0])
	}
	if notes[0].Field != "status" || notes[0].ID != "rep-1" {
		t.Errorf("the note must still name what diverged: %#v", notes[0])
	}
}

// An undesignated fixture counts as non-composing, so a feature that is
// mid-authoring or whose designation is ambiguous degrades to a note. The
// alternative — reading "no designation" as "composes" — would fail every
// project that has not yet marked a fixture, which is the state a feature is
// in for most of its life.
func TestUndesignatedFixturesDegradeToANote(t *testing.T) {
	errors, notes := findContradictions([]entityRecord{
		scenarioRec("dashboard", "seed", "Employee", "emp-2",
			map[string]interface{}{"role": "manager"}),
		scenarioRec("expense-list", "new", "Employee", "emp-2",
			map[string]interface{}{"role": "employee"}),
	})
	if len(errors) != 0 {
		t.Fatalf("undesignated fixtures must not fail the build: %#v", errors)
	}
	if len(notes) != 1 || notes[0].Code != "composition-scenario-fixture-divergence" {
		t.Fatalf("want one divergence note, got %#v", notes)
	}
}

// R4-16. A scenario fixture sitting in the same group must not launder a
// disagreement between two COMPOSING fixtures into a note.
//
// The old code computed one `allComposing` flag across every site in the
// group, spanning all values, so any single non-composing record anywhere in
// the group demoted the whole finding. Two features whose composing fixtures
// genuinely contradict — the exact defect this command was built to catch —
// went out as an advisory note and the project reported coherent, because
// unrelated scenario data happened to be nearby.
//
// Both sub-cases matter, and the second is the one that hid: a scenario
// fixture that AGREES with one of the composing sides adds no new value to
// the group at all, so nothing about the group's shape hints that a
// contradiction was suppressed.
func TestAScenarioFixtureCannotDemoteAComposingContradiction(t *testing.T) {
	assertContradiction := func(t *testing.T, errors, notes []compositionFinding) {
		t.Helper()
		if len(errors) != 1 || errors[0].Code != "composition-fixture-contradiction" {
			t.Fatalf("the composing fixtures contradict; want that failed, got errors=%#v notes=%#v", errors, notes)
		}
		// The refusal must name the specific conflict, and a scenario fixture
		// that merely shares the cell is not part of it.
		for _, site := range errors[0].Sites {
			if site == "audit/rejection-scenario" {
				t.Errorf("the contradiction named a non-composing site: %#v", errors[0].Sites)
			}
		}
		if len(notes) != 1 || notes[0].Code != "composition-scenario-fixture-divergence" {
			t.Fatalf("the scenario divergence is a second true statement and must still be reported: %#v", notes)
		}
	}

	t.Run("scenario carries a third value", func(t *testing.T) {
		errors, notes := findContradictions([]entityRecord{
			rec("dashboard", "seed", "ExpenseReport", "rep-1",
				map[string]interface{}{"status": "submitted"}),
			rec("approvals", "seed", "ExpenseReport", "rep-1",
				map[string]interface{}{"status": "approved"}),
			scenarioRec("audit", "rejection-scenario", "ExpenseReport", "rep-1",
				map[string]interface{}{"status": "rejected"}),
		})
		assertContradiction(t, errors, notes)
	})

	t.Run("scenario agrees with one composing side", func(t *testing.T) {
		errors, notes := findContradictions([]entityRecord{
			rec("dashboard", "seed", "ExpenseReport", "rep-1",
				map[string]interface{}{"status": "submitted"}),
			rec("approvals", "seed", "ExpenseReport", "rep-1",
				map[string]interface{}{"status": "approved"}),
			scenarioRec("audit", "rejection-scenario", "ExpenseReport", "rep-1",
				map[string]interface{}{"status": "approved"}),
		})
		assertContradiction(t, errors, notes)
	})
}

// The converse guard, so the fix cannot be "report everything as an error".
// One composing side and any number of scenario sides is still only a note:
// the scenario fixtures never reach the composed seed, so the prototype only
// ever holds the one value.
func TestOneComposingSideAmongScenariosIsStillOnlyANote(t *testing.T) {
	errors, notes := findContradictions([]entityRecord{
		rec("dashboard", "seed", "ExpenseReport", "rep-1",
			map[string]interface{}{"status": "submitted"}),
		scenarioRec("approvals", "rejection-scenario", "ExpenseReport", "rep-1",
			map[string]interface{}{"status": "rejected"}),
		scenarioRec("audit", "draft-scenario", "ExpenseReport", "rep-1",
			map[string]interface{}{"status": "draft"}),
	})
	if len(errors) != 0 {
		t.Fatalf("only one fixture composes, so the runtime holds one value: %#v", errors)
	}
	if len(notes) != 1 || notes[0].Code != "composition-scenario-fixture-divergence" {
		t.Fatalf("want one divergence note, got %#v", notes)
	}
}

// End to end: two features whose composing fixtures disagree exit non-zero;
// two whose scenario fixtures disagree stay coherent with a note. The
// designation is read through agent.ComposingFixture, the same decoder
// scaffold-seed uses — the point of the fix is that the two commands can no
// longer return opposite verdicts about the same file.
func TestComposingDesignationIsReadFromTheBuildfile(t *testing.T) {
	composing := func(fixture, status string) string {
		return "feature: f\nfixtures:\n  " + fixture + ":\n    composes: true\n    data:\n" +
			"      ExpenseReport:\n        - id: rep-1\n          status: " + status + "\n"
	}
	scenario := func(fixture, status string) string {
		return "feature: f\nfixtures:\n  " + fixture + ":\n    data:\n" +
			"      ExpenseReport:\n        - id: rep-1\n          status: " + status + "\n"
	}

	t.Run("composing fixtures that disagree fail", func(t *testing.T) {
		cfg := setupCompositionProject(t, map[string]string{
			"dashboard": composing("seed", "submitted"),
			"approvals": composing("seed", "rejected"),
		})
		out := runCompositionForTest(t, cfg)
		if out.Coherent {
			t.Errorf("two composing fixtures disagreeing must not be coherent: %#v", out)
		}
	})

	// R4-16 end to end. The unit test above proves the partition; this proves
	// the verdict follows it out of the command. A project in this state used
	// to report `coherent: true` with the contradiction filed under notes.
	t.Run("a scenario fixture alongside them does not rescue coherence", func(t *testing.T) {
		cfg := setupCompositionProject(t, map[string]string{
			"dashboard": composing("seed", "submitted"),
			"approvals": composing("seed", "rejected"),
			"audit":     scenario("rejection-scenario", "draft"),
		})
		out := runCompositionForTest(t, cfg)
		if out.Coherent {
			t.Errorf("a nearby scenario fixture must not demote a composing contradiction: %#v", out)
		}
	})

	t.Run("scenario fixtures that disagree stay coherent with a note", func(t *testing.T) {
		cfg := setupCompositionProject(t, map[string]string{
			"dashboard": composing("seed", "submitted"),
			"approvals": scenario("rejection-scenario", "rejected"),
		})
		out := runCompositionForTest(t, cfg)
		if !out.Coherent {
			t.Errorf("a scenario fixture must not break coherence: %#v", out)
		}
		found := false
		for _, n := range out.Notes {
			if n.Code == "composition-scenario-fixture-divergence" {
				found = true
			}
		}
		if !found {
			t.Errorf("the divergence must still be reported as a note: %#v", out.Notes)
		}
	})
}

func runCompositionForTest(t *testing.T, cfg *config.Context) compositionOutput {
	t.Helper()
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	_ = runCheckComposition(cmd, nil)
	var out compositionOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v (raw: %s)", err, buf.String())
	}
	return out
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
