// parlay-feature: parlay-tool/phase-gates
// parlay-component: gate-aggregator
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func gateHasCode(fs []gateBlocker, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestGate_UnknownStageErrors(t *testing.T) {
	dir := setupTestDir(t)
	setupLedgerFeature(t, dir)
	if _, err := computeGate(testContext(t), "my-feature", "bogus"); err == nil {
		t.Fatal("expected an error for an unknown gate stage")
	}
}

func TestGate_HandAuthoredUnitPassesEveryStage(t *testing.T) {
	dir := setupTestDir(t)
	if err := os.MkdirAll(filepath.Join(dir, ".parlay"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".parlay", "config.yaml"), []byte("x: y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unitDir := filepath.Join(dir, "spec", "intents", "my-unit")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "intents.md"), []byte("## X\n**Goal**: g\n**Persona**: p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "authored.yaml"), []byte("unit: my-unit\nsources:\n  - \"x.go\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{gateStageBuild, gateStageCode, gateStageDone} {
		out, err := computeGate(testContext(t), "my-unit", stage)
		if err != nil {
			t.Fatalf("stage %s: %v", stage, err)
		}
		if !out.Passed || len(out.Blockers) != 0 {
			t.Errorf("a hand-authored unit must pass the %s gate with no blockers; got %+v", stage, out.Blockers)
		}
	}
}

// The 2b journal matrix — the one subtle piece of gate logic. An unapplied
// ledger tail downgrades from blocker to warning ONLY when a refine journal has
// reached splice-applied AND names the amendment the tail is waiting on.
func TestRefineSanctionsUnappliedTail(t *testing.T) {
	cases := []struct {
		name      string
		journal   *refineJournal
		sanctions bool
	}{
		{"absent", nil, false},
		{"early-amendment-written-only", &refineJournal{Feature: "my-feature", Amendment: 1, Completed: []string{"amendment-written"}}, false},
		{"late-matching", &refineJournal{Feature: "my-feature", Amendment: 1, Completed: []string{"amendment-written", "splice-applied"}}, true},
		{"late-mismatched-amendment", &refineJournal{Feature: "my-feature", Amendment: 2, Completed: []string{"amendment-written", "splice-applied"}}, false},
		{"splice-applied-but-no-amendment", &refineJournal{Feature: "my-feature", Amendment: 0, Completed: []string{"amendment-written", "splice-applied"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			featureDir := setupUnappliedTailFeature(t)
			if tc.journal != nil {
				writeRefineJournalFixture(t, "my-feature", *tc.journal)
			}
			got := refineSanctionsUnappliedTail(testContext(t), "my-feature", featureDir)
			if got != tc.sanctions {
				t.Errorf("refineSanctionsUnappliedTail = %v, want %v", got, tc.sanctions)
			}
		})
	}
}

func TestGate_Build_UnsanctionedTailBlocks(t *testing.T) {
	featureDir := setupUnappliedTailFeature(t)
	_ = featureDir
	out, err := computeGate(testContext(t), "my-feature", gateStageBuild)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "unapplied-amendments") {
		t.Errorf("an unapplied tail with no in-flight refine must block; blockers=%+v", out.Blockers)
	}
	if gateHasCode(out.Warnings, "unapplied-amendments") {
		t.Errorf("unapplied-amendments must not be a warning when unsanctioned; warnings=%+v", out.Warnings)
	}
	if out.Passed {
		t.Error("gate must not pass with an unapplied-amendments blocker")
	}
}

func TestGate_Build_SanctionedTailIsWarning(t *testing.T) {
	featureDir := setupUnappliedTailFeature(t)
	_ = featureDir
	// A refine that has applied its splice (step 5.5 rebuilds here) sanctions
	// the dirty tail — the boundary gate must not hard-fail on it.
	writeRefineJournalFixture(t, "my-feature", refineJournal{
		Feature:   "my-feature",
		Amendment: 1,
		Completed: []string{"amendment-written", "splice-applied"},
	})
	out, err := computeGate(testContext(t), "my-feature", gateStageBuild)
	if err != nil {
		t.Fatal(err)
	}
	if gateHasCode(out.Blockers, "unapplied-amendments") {
		t.Errorf("a sanctioned tail must not block; blockers=%+v", out.Blockers)
	}
	if !gateHasCode(out.Warnings, "unapplied-amendments") {
		t.Errorf("a sanctioned tail must be reported as a warning; warnings=%+v", out.Warnings)
	}
}

func TestGate_ExitCode_BlockedIsNonZero(t *testing.T) {
	setupUnappliedTailFeature(t)
	cmd := testCommandWithContext(t, testContext(t))
	// Route stdout to a throwaway so the JSON does not pollute test output.
	devnull, _ := os.Open(os.DevNull)
	defer devnull.Close()
	cmd.SetOut(devnull)
	resetFlagsAfterTest(t, gateCmd.Flags())
	gateStage = gateStageBuild
	err := runInternalGate(cmd, []string{"my-feature"})
	ece, ok := err.(*ExitCodeError)
	if !ok || ece.Code != 1 {
		t.Fatalf("a blocked gate must return ExitCodeError{1}; got %v", err)
	}
}

func TestDedupeGateFindings_MergesAndSorts(t *testing.T) {
	in := []gateBlocker{
		{Code: "b-code", Message: "second"},
		{Code: "a-code", Message: "first"},
		{Code: "b-code", Message: "second"}, // exact dup of the first entry
	}
	out := dedupeGateFindings(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 findings after dedupe; got %d: %+v", len(out), out)
	}
	if out[0].Code != "a-code" || out[1].Code != "b-code" {
		t.Errorf("findings must be sorted by code; got %+v", out)
	}
}

func TestPhaseToGateStage(t *testing.T) {
	cases := []struct {
		phase FeaturePhase
		stage string
		gated bool
	}{
		{PhaseIntents, "", false},
		{PhaseDialogs, "", false},
		{PhaseArtifacts, gateStageBuild, true},
		{PhaseBuild, gateStageCode, true},
		{PhaseDone, gateStageDone, true},
		{PhaseHandAuthored, "", false},
	}
	for _, tc := range cases {
		stage, gated := phaseToGateStage(tc.phase)
		if gated != tc.gated || stage != tc.stage {
			t.Errorf("phaseToGateStage(%s) = (%q, %v), want (%q, %v)", tc.phase, stage, gated, tc.stage, tc.gated)
		}
	}
}

func TestSweepFeatures_IntentsOnlyIsNotYetGated(t *testing.T) {
	dir := setupTestDir(t)
	setupLedgerFeature(t, dir) // my-feature has intents.md only → PhaseIntents
	rows := sweepFeatures(testContext(t), "root", []string{"my-feature"})
	if len(rows) != 1 {
		t.Fatalf("expected one row; got %d", len(rows))
	}
	if !rows[0].Skipped || !rows[0].Passed {
		t.Errorf("an intents-only feature has no boundary to gate yet; row=%+v", rows[0])
	}
}

// The build gate reports criteria vacancy and still passes.
//
// Both halves matter. Reporting it is the point: the vacancy is invisible to
// every downstream coverage walker, which ask whether STATED criteria are
// discharged. Passing is the transition policy — grading the aggregate an
// error would convert it into a blocker here and stop every feature authored
// under the pre-v0.5.x routing rule before its owner had any way forward.
func TestGate_Build_CriteriaVacancyWarnsWithoutBlocking(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)

	// The observed shape: every intent covered by an operation carrying
	// criteria, every fragment left with none.
	surface := `feature: my-feature
fragments:
  - name: Customers list
    shows: list-of-items
    source: "@my-feature/browse-customers"
    page: customers
    region: main
  - name: Customer detail
    shows: detail-of-one-item
    source: "@my-feature/browse-customers"
    page: customers
    region: aside
`
	capabilities := `feature: my-feature
operations:
  - id: customer.list
    kind: query
    source: "@my-feature/browse-customers"
    verify:
      - "returns customers in name order"
`
	if err := os.WriteFile(filepath.Join(featureDir, "surface.yaml"), []byte(surface), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"), []byte(capabilities), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(testContext(t), "my-feature", gateStageBuild)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Warnings, "surface-fragment-no-criteria") {
		t.Errorf("the build gate did not report the vacant fragments; warnings: %+v", out.Warnings)
	}
	if !gateHasCode(out.Warnings, "feature-surface-no-criteria") {
		t.Errorf("the build gate did not report the vacant surface; warnings: %+v", out.Warnings)
	}
	if gateHasCode(out.Blockers, "surface-fragment-no-criteria") || gateHasCode(out.Blockers, "feature-surface-no-criteria") {
		t.Errorf("criteria vacancy blocked the gate; it must warn while projects migrate: %+v", out.Blockers)
	}
}
