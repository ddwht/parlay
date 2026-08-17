// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: refine-preflight
// parlay-artifact: test

package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCheckApplied_ runs the pre-flight against the current test project and
// decodes its JSON.
func runCheckApplied_(t *testing.T, feature string) checkAppliedOutput {
	t.Helper()
	cmd := testCommandWithContext(t, testContext(t))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runCheckApplied(cmd, []string{feature}); err != nil {
		t.Fatalf("check-applied: %v", err)
	}
	var out checkAppliedOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("decode check-applied output %q: %v", buf.String(), err)
	}
	return out
}

func TestCheckApplied_CleanStateWithLedgerIndex(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md",
		"---\namendment: first\ndate: 2026-08-13\ntrigger: shopper feedback\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\na\n## Acceptance\n- b\n")
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})

	out := runCheckApplied_(t, "my-feature")

	if !out.CleanState {
		t.Errorf("settled project must report clean_state; drift=%v integrity=%v", out.HasDrift, out.LedgerIntegrity)
	}
	if !out.HasBaseline {
		t.Error("a built feature must report has_baseline")
	}
	if len(out.Amendments) != 1 {
		t.Fatalf("expected the ledger index to carry one entry, got %+v", out.Amendments)
	}
	entry := out.Amendments[0]
	if entry.Seq != 1 || entry.Slug != "first" || entry.Trigger != "shopper feedback" {
		t.Errorf("index entry lost its frontmatter identity: %+v", entry)
	}
	if !entry.Applied {
		t.Error("an amendment at or below last-applied must report applied:true")
	}
	if entry.Path != "spec/intents/my-feature/amendments/001-first.md" {
		t.Errorf("index path must be project-relative and openable, got %q", entry.Path)
	}
	if out.LastAppliedAmendment != 1 {
		t.Errorf("last_applied_amendment = %d, want 1", out.LastAppliedAmendment)
	}
}

// TestCheckApplied_UnappliedTailIsNotClean pins the distinction the whole
// pre-flight exists for: an amendment on disk whose delta never reached the
// contract is NOT an already-applied ask, and must not license a no-op exit.
func TestCheckApplied_UnappliedTailIsNotClean(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, nil)
	writeAmendment(t, featureDir, "001-first.md",
		"---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\na\n## Acceptance\n- b\n")

	out := runCheckApplied_(t, "my-feature")

	if out.CleanState {
		t.Error("an unapplied tail must not read as clean state")
	}
	if len(out.UnappliedAmendments) != 1 {
		t.Errorf("expected the unapplied tail to be named, got %v", out.UnappliedAmendments)
	}
	if len(out.Amendments) != 1 || out.Amendments[0].Applied {
		t.Errorf("an amendment beyond last-applied must report applied:false, got %+v", out.Amendments)
	}
}

func TestCheckApplied_DriftIsNotClean(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, nil)
	edited := strings.Replace(ledgerTestIntents, "See if the cluster is ready.", "See if EVERYTHING is ready.", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runCheckApplied_(t, "my-feature")

	if out.CleanState {
		t.Error("a frozen-doc edit must not read as clean state")
	}
	if len(out.LedgerIntegrity) == 0 {
		t.Error("the integrity finding must be carried through to the pre-flight")
	}
}

func TestCheckApplied_ReportsInFlightRun(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, nil)

	stampRefineJournal(t, "my-feature", "--ask", "widen the timeout", "--step", "amendment-written", "--amendment", "2")

	out := runCheckApplied_(t, "my-feature")

	if out.InFlight == nil {
		t.Fatal("an interrupted run must be reported as in_flight")
	}
	if out.CleanState {
		t.Error("an in-flight refinement must not read as clean state — the project is mid-write")
	}
	if out.InFlight.Amendment != 2 {
		t.Errorf("in-flight amendment = %d, want 2 (a resumed run must amend this file, not mint a duplicate)", out.InFlight.Amendment)
	}
	if got := out.InFlight.NextRefineStep(); got != "splice-applied" {
		t.Errorf("next step = %q, want splice-applied", got)
	}
}

// stampRefineJournal runs the journal command with the given flags.
func stampRefineJournal(t *testing.T, feature string, flags ...string) string {
	t.Helper()
	resetFlagsAfterTest(t, refineJournalCmd.Flags())
	if err := refineJournalCmd.Flags().Parse(flags); err != nil {
		t.Fatalf("parse journal flags: %v", err)
	}
	cmd := testCommandWithContext(t, testContext(t))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runRefineJournal(cmd, []string{feature}); err != nil {
		t.Fatalf("refine-journal %v: %v", flags, err)
	}
	return buf.String()
}

func TestRefineJournal_StampResumeAndClear(t *testing.T) {
	dir := setupTestDir(t)
	setupLedgerFeature(t, dir)

	stampRefineJournal(t, "my-feature", "--step", "amendment-written", "--amendment", "1")
	stampRefineJournal(t, "my-feature", "--step", "splice-applied")

	journal, err := loadRefineJournal(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if journal == nil {
		t.Fatal("expected a journal on disk")
	}
	if got := journal.NextRefineStep(); got != "rebuilt" {
		t.Errorf("next step = %q, want rebuilt", got)
	}

	// Re-stamping a completed step is a no-op, not a duplicate entry: a
	// retried CLI call must not corrupt the order.
	stampRefineJournal(t, "my-feature", "--step", "splice-applied")
	journal, _ = loadRefineJournal(testContext(t), "my-feature")
	if len(journal.Completed) != 2 {
		t.Errorf("re-stamping must be idempotent, got %v", journal.Completed)
	}

	stampRefineJournal(t, "my-feature", "--clear")
	journal, err = loadRefineJournal(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if journal != nil {
		t.Error("--clear must remove the journal so the next run starts fresh")
	}
}

// TestRefineJournal_RejectsUnknownStep pins the closed vocabulary: a journal
// carrying an invented step name resumes at the wrong place, which is worse
// than no journal because it looks authoritative.
func TestRefineJournal_RejectsUnknownStep(t *testing.T) {
	dir := setupTestDir(t)
	setupLedgerFeature(t, dir)

	resetFlagsAfterTest(t, refineJournalCmd.Flags())
	if err := refineJournalCmd.Flags().Parse([]string{"--step", "mostly-done"}); err != nil {
		t.Fatal(err)
	}
	cmd := testCommandWithContext(t, testContext(t))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := runRefineJournal(cmd, []string{"my-feature"})
	if err == nil {
		t.Fatal("an unknown step name must be an error at write time")
	}
	if !strings.Contains(err.Error(), "vocabulary is closed") {
		t.Errorf("the error must name the closed vocabulary, got %v", err)
	}
	if _, statErr := os.Stat(refineJournalPath(testContext(t), "my-feature")); statErr == nil {
		t.Error("a rejected step must not leave a journal behind")
	}
}

// TestRefineJournal_ClearIsIdempotent — step 9 clears the journal, and a
// refine that never journaled (or a repeated clear) must not fail.
func TestRefineJournal_ClearIsIdempotent(t *testing.T) {
	dir := setupTestDir(t)
	setupLedgerFeature(t, dir)

	stampRefineJournal(t, "my-feature", "--clear")
	stampRefineJournal(t, "my-feature", "--clear")
}
