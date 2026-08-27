// parlay-feature: parlay-tool/phase-gates
// parlay-component: gate-aggregator
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
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

// A wholly vacant surface BLOCKS the designer->build boundary, and the
// per-fragment findings ride along as warnings that say where.
//
// The aggregate is the only one that blocks. It shipped as a warning on the
// reasoning that an error would stop projects with no way forward; the way
// forward now exists (`migrate-verify --fragments`, plus a routing rule that
// tells the designer to author them), and the benchmark showed a warning stops
// nothing — the agent diagnosed this exact condition, tried migrate-verify,
// and shipped 21 criterion-less cases anyway.
func TestGate_Build_TotalCriteriaVacancyBlocks(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeGateSurface(t, featureDir, false)

	out, err := computeGate(testContext(t), "my-feature", gateStageBuild)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "feature-surface-no-criteria") {
		t.Errorf("a wholly vacant surface did not block the boundary; blockers: %+v", out.Blockers)
	}
	if out.Passed {
		t.Error("gate passed with an empty presentation contract")
	}
	if !gateHasCode(out.Warnings, "surface-fragment-no-criteria") {
		t.Errorf("the per-fragment findings did not ride along to say where; warnings: %+v", out.Warnings)
	}
	if gateHasCode(out.Blockers, "surface-fragment-no-criteria") {
		t.Error("a per-fragment finding blocked; only the aggregate may")
	}
}

// Partial vacancy locates without stopping. A single fragment without criteria
// may be structural or assembly-only — a judgement a reviewer makes, and one no
// exemption machinery exists to record — so it must not hold up the boundary.
func TestGate_Build_PartialCriteriaVacancyDoesNotBlock(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeGateSurface(t, featureDir, true)

	out, err := computeGate(testContext(t), "my-feature", gateStageBuild)
	if err != nil {
		t.Fatal(err)
	}
	if gateHasCode(out.Blockers, "feature-surface-no-criteria") {
		t.Error("the aggregate fired while one fragment still states criteria")
	}
	if !gateHasCode(out.Warnings, "surface-fragment-no-criteria") {
		t.Errorf("the vacant fragment was not reported; warnings: %+v", out.Warnings)
	}
	for _, b := range out.Blockers {
		if strings.Contains(b.Code, "no-criteria") {
			t.Errorf("partial vacancy produced a blocker: %+v", b)
		}
	}
}

// writeGateSurface lays down a two-fragment surface; firstHasCriteria decides
// whether the presentation contract is partially or wholly vacant.
func writeGateSurface(t *testing.T, featureDir string, firstHasCriteria bool) {
	t.Helper()
	first := ""
	if firstHasCriteria {
		first = "    verify:\n      - \"the list shows each customer\"\n"
	}
	surface := `feature: my-feature
fragments:
  - name: Customers list
    shows: list-of-items
    source: "@my-feature/browse-customers"
    page: customers
    region: main
` + first + `  - name: Customer detail
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
}

// --- ledger state at every advancing boundary ----------------------------
//
// Regression tests for a bypass that predates intent supersession. Only
// gateBuild aggregated ledger integrity and the unapplied tail; gateCode and
// gateDone aggregated neither. A caller entering with --from code therefore
// never crossed the boundary that asks whether a recorded decision had been
// applied, and codegen ran against a specification its author had already
// superseded — reporting success.

func TestGate_Code_UnappliedTailBlocks(t *testing.T) {
	setupUnappliedTailFeature(t)
	out, err := computeGate(testContext(t), "my-feature", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "unapplied-amendments") {
		t.Errorf("--from code must not walk past a recorded-but-unapplied decision; blockers=%+v", out.Blockers)
	}
	if out.Passed {
		t.Error("the build->code boundary must not pass with an unapplied tail")
	}
}

func TestGate_Done_UnappliedTailBlocks(t *testing.T) {
	setupUnappliedTailFeature(t)
	out, err := computeGate(testContext(t), "my-feature", gateStageDone)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "unapplied-amendments") {
		t.Errorf("completing a feature asserts the strongest thing the ladder can say; blockers=%+v", out.Blockers)
	}
}

// The journal-aware downgrade must be the SAME rule at every boundary, not a
// build-only nicety: a refinement mid-apply is not stale, and blocking it at
// the code boundary would stop the workflow that resolves the finding.
func TestGate_Code_SanctionedTailIsWarning(t *testing.T) {
	setupUnappliedTailFeature(t)
	writeRefineJournalFixture(t, "my-feature", refineJournal{
		Feature:   "my-feature",
		Amendment: 1,
		Completed: []string{"amendment-written", "splice-applied"},
	})
	out, err := computeGate(testContext(t), "my-feature", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if gateHasCode(out.Blockers, "unapplied-amendments") {
		t.Errorf("a sanctioned tail must not block the code boundary either; blockers=%+v", out.Blockers)
	}
	if !gateHasCode(out.Warnings, "unapplied-amendments") {
		t.Errorf("it must still be reported; warnings=%+v", out.Warnings)
	}
}
