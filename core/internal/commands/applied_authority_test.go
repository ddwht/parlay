package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

// WP0 — the governance bypass.
//
// buildBaseline stamps LastAppliedAmendment as the MAXIMUM sequence in the
// ledger (baseline.go), justified by a comment asserting that save-build-state
// only ever runs after a green build "at which point the ledger is by
// definition fully applied". That assumption does not hold for a --partial
// save, which is exactly what /parlay-refine step 9 runs.
//
// The consequence is not a missing feature, it is a forged one. A governance
// amendment — one that supersedes a founding intent or retires a feature —
// may only be applied through `apply-governance --confirm`, which prints the
// promises that would stop being in force, because "it, not the amendment
// filename, is what you are approving". A partial save that sweeps a pending
// governance record past the applied marker withdraws those promises with no
// list ever shown, AND records the record's hash in sources.amendments. It
// therefore manufactures the very evidence that a later trusted-applied check
// would read as proof the withdrawal was authorised.
//
// The guard that ought to catch this is structurally blind to it: refine's
// post-splice check asserts dirty_set is "exactly the amendment you just
// added", and a pure governance amendment carries no affects:, so it
// contributes nothing to dirty_set and the check passes clean.
//
// These tests state the REQUIRED behaviour. They fail on today's tree; WP1
// and WP2 make them pass.

// ledgerFeatureWithSource lays down the ledger fixture feature plus one
// marker-tagged source file it owns, and returns (featureDir, sourceRoot,
// sourceFile).
func ledgerFeatureWithSource(t *testing.T, dir string) (string, string, string) {
	t.Helper()
	featureDir := setupLedgerFeature(t, dir)
	sourceRoot := filepath.Join(dir, "cmd")
	sourceFile := filepath.Join(sourceRoot, "mine", "mine.go")
	writeMarkedFile(t, sourceFile, "my-feature", "readiness", "package mine")
	return featureDir, sourceRoot, sourceFile
}

// writeEmittedManifest declares the files a partial run wrote and arms the
// partial-save flags for the duration of the test.
func writeEmittedManifest(t *testing.T, cfg *config.Context, files ...string) string {
	t.Helper()
	emittedPath := filepath.Join(cfg.ProjectBuildPath(), DefaultEmittedManifest)
	if err := os.MkdirAll(filepath.Dir(emittedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, f := range files {
		body += f + "\n"
	}
	if err := os.WriteFile(emittedPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	saveBuildStatePartial = true
	saveBuildStateEmitted = emittedPath
	t.Cleanup(func() { saveBuildStatePartial = false; saveBuildStateEmitted = "" })
	return emittedPath
}

// writeRefineJournal persists the machine half of a /parlay-refine run: the
// journal it stamps as each step completes. save-build-state cannot read the
// prose skill, but it can read this, and WP1 makes it the proof that a
// partial save is authorised to advance a particular record.
//
// Steps run through "tested" — "re-baselined" is stamped by step 9 itself,
// after the save this fixture is exercising.
func writeRefineJournal(t *testing.T, cfg *config.Context, slug string, amendment int) {
	t.Helper()
	j := refineJournal{
		Feature:   slug,
		Ask:       "reword the readiness output",
		Amendment: amendment,
		StartedAt: "2026-08-31T00:00:00Z",
		Completed: []string{"amendment-written", "splice-applied", "rebuilt", "emitted", "tested"},
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	// Capture the pre-splice inventory exactly as the real journal command
	// does when it stamps amendment-written, so a fixture is not a weaker
	// artefact than the thing it stands in for.
	if err := captureScopeBefore(cfg, slug, &j, amendment); err != nil {
		t.Fatal(err)
	}
	data, err = yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	path := refineJournalPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

const govAmendment = `---
amendment: studio-detection-withdrawn
date: 2026-08-31
supersedes_intents:
  - check-readiness
---

## Change

The readiness promise is withdrawn. Nothing takes it over.

## Why

The cluster it described no longer exists.

## Acceptance

- No command reports readiness.
`

const spliceAmendment = `---
amendment: readiness-wording
date: 2026-08-31
affects: ["@my-feature/operation:x"]
---

## Change

Reword the readiness output.

## Acceptance

- The new wording is used.
`

const appliedAmendment = `---
amendment: first
date: 2026-08-30
affects: ["@my-feature/operation:x"]
---

## Change

The original change.

## Acceptance

- It happened.
`

// readBaseline loads a feature's .baseline.yaml.
func readFeatureBaseline(t *testing.T, path string) Baseline {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var bl Baseline
	if err := yaml.Unmarshal(data, &bl); err != nil {
		t.Fatal(err)
	}
	return bl
}

// TestWP0_PartialSaveMustNotSweepPendingGovernance is the exploit regression.
//
// The fixture is the reachable workflow, not an abstraction of it: a feature
// at last-applied 1, a PENDING pure-governance 002 that only
// `apply-governance --confirm` may move, a PENDING splice 003 of the sort
// /parlay-refine authors, a refine journal completed through "tested" naming
// 003, and the emitted manifest listing the one file the run wrote. That
// journal plus that manifest ARE the machine inputs of step 9; the prose skill
// contributes nothing else the save can read.
//
// The journal names 003, but the tail is 002+003. The run is therefore
// authorised for one record and would advance two. Required behaviour: refuse,
// naming the record it will not advance past, and leave every byte of the
// feature baseline alone — the refusal must land BEFORE any write, not be
// repaired after one.
func TestWP0_PartialSaveMustNotSweepPendingGovernance(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)

	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})

	// 002 is pure governance: no affects:, so it is invisible to dirty_set.
	writeAmendment(t, featureDir, "002-studio-detection-withdrawn.md", govAmendment)
	// 003 is the splice amendment this refine legitimately applied.
	writeAmendment(t, featureDir, "003-readiness-wording.md", spliceAmendment)
	// The run is journalled as authorised for 003 ONLY.
	writeRefineJournal(t, cfg, "my-feature", 3)

	blPath := baselinePath(cfg, "my-feature")
	before, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	emittedPath := writeEmittedManifest(t, cfg, sourceFile)

	_, stderr, saveErr := runProjectSave(t, cfg, sourceRoot)
	report := ""
	if saveErr != nil {
		report = saveErr.Error()
	}
	report += stderr

	if saveErr == nil {
		t.Error("BYPASS: a partial save advanced past a PENDING governance amendment. " +
			"Only `apply-governance --confirm` may withdraw founding promises, because the " +
			"promise list it prints is what the user approves. This save showed nothing and " +
			"withdrew them anyway.")
	}
	if saveErr != nil {
		if !strings.Contains(report, "002-studio-detection-withdrawn") {
			t.Errorf("the refusal must name the owed decision by its full identity so the "+
				"operator knows which record is still unapplied; got %q", report)
		}
		if !strings.Contains(report, "apply-governance") {
			t.Errorf("the refusal must route the operator to the one command allowed to move "+
				"a governance record; got %q", report)
		}
	}

	// Preflight-before-write: not "the fields were restored", but "nothing was
	// written at all".
	after, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		bl := readFeatureBaseline(t, blPath)
		t.Errorf("the feature baseline was rewritten by a save that must refuse before any "+
			"write (last-applied now %d). WP1 promises preflight-before-write precisely so a "+
			"late-discovered invalid claim cannot have already advanced the tail.",
			bl.LastAppliedAmendment)

		if _, recorded := bl.Sources.Amendments["002-studio-detection-withdrawn.md"]; recorded {
			t.Error("FORGED EVIDENCE: the pending governance record's hash was written into " +
				"sources.amendments. A later trusted-applied check (seq <= last-applied AND " +
				"stored hash matches) would read this as proof the withdrawal was authorised. " +
				"Hashes may only be recorded for records actually proven applied.")
		}
	}

	// A refused run consumes nothing and creates no project-level state.
	if _, err := os.Stat(emittedPath); err != nil {
		t.Errorf("the emitted manifest must survive a refusal — the run did not happen, so its "+
			"inputs are still owed to the retry; stat: %v", err)
	}
	if _, err := os.Stat(emittedPath + ".consumed"); err == nil {
		t.Error("the manifest was consumed by a save that refused")
	}
	if _, err := os.Stat(projectBaselinePath(cfg)); err == nil {
		t.Error("a refused save created project-level baseline state")
	}
}

// TestWP0_FullSaveMustNotSweepPendingGovernance pins the same property on the
// non-partial path. buildBaseline is shared, so the ledger-maximum stamp is a
// primitive of BOTH saves; the loop's gate normally blocks earlier, which
// makes this the latent half rather than the reachable one. Green tests do not
// mean every visible amendment is applied.
//
// The contract is deliberately "either safe outcome": refuse, or succeed while
// preserving the marker and hashes. WP1 closes it by refusing; WP2 may later
// switch to preserve-and-succeed without rewriting the test.
func TestWP0_FullSaveMustNotSweepPendingGovernance(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, _ := ledgerFeatureWithSource(t, dir)

	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})
	writeAmendment(t, featureDir, "002-studio-detection-withdrawn.md", govAmendment)

	blPath := baselinePath(cfg, "my-feature")
	before, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, saveErr := runProjectSave(t, cfg, sourceRoot); saveErr == nil {
		// Safe-success branch: authority must be preserved WHOLE. Comparing
		// raw bytes catches not only 002 being swept in, but 001's trusted
		// evidence being rewritten or dropped — a save that preserves the
		// marker while losing the hash that proves 001 was honestly applied
		// has still destroyed the trusted-applied predicate.
		after, err := os.ReadFile(blPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			bl := readFeatureBaseline(t, blPath)
			t.Errorf("a full save that succeeds must leave the applied authority untouched, "+
				"but the baseline was rewritten (marker 1 -> %d). A green build is not evidence "+
				"that a promise-withdrawal was approved.", bl.LastAppliedAmendment)

			if _, recorded := bl.Sources.Amendments["002-studio-detection-withdrawn.md"]; recorded {
				t.Error("a full save recorded a pending governance record's hash, manufacturing " +
					"the evidence a trusted-applied check would trust")
			}
			if bl.Sources == nil || bl.Sources.Amendments["001-first.md"] == "" {
				t.Error("001's recorded hash was dropped — the evidence that a legitimately " +
					"applied record was honoured must survive every save")
			}
		}
	}
}

// TestWP0_PartialSaveAdvancesOnlyItsOwnRecord is the companion that must stay
// green: the ordinary one-amendment refine still works. A single pending
// splice amendment, journalled and emitted, advances exactly to itself. The
// journal names the whole tail here, which is what makes this run authorised
// where the exploit's is not — so a fix that simply refuses whenever any tail
// exists would fail this and be caught.
func TestWP0_PartialSaveAdvancesOnlyItsOwnRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)

	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)

	writeEmittedManifest(t, cfg, sourceFile)
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("a lone pending splice amendment, journalled as this run's own, is the "+
			"ordinary refine case and must save: %v", err)
	}

	after := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if after.LastAppliedAmendment != 2 {
		t.Errorf("last-applied = %d, want 2 — the refined amendment must be recorded applied",
			after.LastAppliedAmendment)
	}
	if after.Sources == nil || after.Sources.Amendments["002-readiness-wording.md"] == "" {
		t.Error("the applied amendment's hash must be recorded, or the integrity check cannot " +
			"notice a later edit to a record the tool already honoured")
	}
}

// ---------------------------------------------------------------------
// WP1 — the authority preflight's own cases.
// ---------------------------------------------------------------------

const combinedAmendment = `---
amendment: readiness-withdrawn-and-reworded
date: 2026-08-31
supersedes_intents:
  - check-readiness
affects: ["@my-feature/operation:x"]
---

## Change

Withdraw the readiness promise and reword what remains.

## Why

Half decision, half edit — which is the problem.

## Acceptance

- Neither half is applied without the other.
`

// ledgerAt1 is the shared opening position: 001 applied and recorded.
func ledgerAt1(t *testing.T, cfg *config.Context, featureDir string) {
	t.Helper()
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})
}

// assertRefused runs a partial save expected to fail and returns the message.
func assertRefused(t *testing.T, cfg *config.Context, sourceRoot, because string) string {
	t.Helper()
	_, stderr, err := runProjectSave(t, cfg, sourceRoot)
	if err == nil {
		t.Fatalf("the save must refuse: %s", because)
	}
	return err.Error() + stderr
}

// A combined record carries both affects: and supersedes_intents:. The schema
// permits it, and no single operation can apply it — apply-governance refuses
// anything with affects:, while a splice would withdraw the named promises
// with no confirmation. WP1 refuses rather than inventing a half-applied
// state; the two-proof transaction is the eventual answer.
func TestWP1_CombinedRecordIsRefusedRatherThanHalfApplied(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-readiness-withdrawn-and-reworded.md", combinedAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot,
		"a record carrying both affects: and supersedes_intents: has no sanctioned single applier")
	if !strings.Contains(msg, "002-readiness-withdrawn-and-reworded") {
		t.Errorf("the refusal must name the combined record; got %q", msg)
	}
	if !strings.Contains(msg, "affects:") || !strings.Contains(msg, "supersedes_intents:") {
		t.Errorf("the refusal must say WHY no single path applies it; got %q", msg)
	}

	after := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if after.LastAppliedAmendment != 1 {
		t.Errorf("marker moved to %d — half of a combined record was applied", after.LastAppliedAmendment)
	}
}

// A partial save advances the marker, so it must prove what it advanced past.
// With no journal there is no proof at all.
func TestWP1_PartialSaveWithoutJournalIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	// No journal written.
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "no journal means no proof of what this run applied")
	if !strings.Contains(msg, "002-readiness-wording") {
		t.Errorf("the refusal must name the unproven record; got %q", msg)
	}
}

// The journal authorises exactly one record. A tail of two means the save
// would advance past one nobody applied — and the refusal must name it, not
// merely report a count.
func TestWP1_UnaccountedTailRecordIsNamed(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-earlier-edit.md", spliceAmendment)
	writeAmendment(t, featureDir, "003-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 3)
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "002 is unapplied and unaccounted for by the journal")
	if !strings.Contains(msg, "002-earlier-edit") {
		t.Errorf("the refusal must name the UNACCOUNTED record so the operator knows what is "+
			"still owed; got %q", msg)
	}
}

// Re-baselining records "this output is blessed", and blessing untested code
// is the one thing the build state must never do. A journal that stopped
// before `tested` is not proof of a completed refinement.
func TestWP1_JournalShortOfTestedIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	j := refineJournal{
		Feature:   "my-feature",
		Amendment: 2,
		Completed: []string{"amendment-written", "splice-applied", "rebuilt"},
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refineJournalPath(cfg, "my-feature"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "the refinement never reached the test step")
	if !strings.Contains(msg, "emitted") {
		t.Errorf("the refusal must name the step the run stopped before; got %q", msg)
	}
}

// Journal proof is refine-specific. A journal left lying around must not
// silently grant a FULL save the authority to withdraw promises — the full
// path never entered that workflow, so its evidence does not apply.
func TestWP1_StrayJournalGrantsNoFullSaveAuthority(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, _ := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-studio-detection-withdrawn.md", govAmendment)
	// A journal that names the governance record, as if it were refinable.
	writeRefineJournal(t, cfg, "my-feature", 2)

	_, stderr, err := runProjectSave(t, cfg, sourceRoot)
	if err == nil {
		t.Fatal("a stray refine journal must not authorise a full save to sweep a governance record")
	}
	if !strings.Contains(err.Error()+stderr, "apply-governance") {
		t.Errorf("the refusal must route to the only command allowed to move it; got %v", err)
	}
}

// A corrupt baseline is not an empty one. lastAppliedAmendment folds every
// read and parse failure to 0, which would make the whole ledger look pending
// — and a one-record tail with a matching journal would then let the save
// overwrite authority state it could not read. Unknown must refuse.
func TestWP1_UnreadableBaselineRefusesBeforeAnyWrite(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	blPath := baselinePath(cfg, "my-feature")
	if err := os.WriteFile(blPath, []byte("{{ not: [valid yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	corrupt, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	writeRefineJournal(t, cfg, "my-feature", 1)
	emittedPath := writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "the applied marker could not be read")
	if !strings.Contains(msg, "my-feature") {
		t.Errorf("the refusal must name the feature whose authority is unreadable; got %q", msg)
	}

	after, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(corrupt) != string(after) {
		t.Error("the unreadable baseline was overwritten — a save must not replace authority " +
			"state it could not inspect")
	}
	if _, err := os.Stat(emittedPath); err != nil {
		t.Errorf("the manifest must survive a refusal; stat: %v", err)
	}
}

// A journal marked through `re-baselined` while its record is still pending is
// inconsistent evidence, not stronger evidence: a finished refinement clears
// its journal. NextRefineStep returns "" here, which is exactly why the set
// answer cannot be the authority answer.
func TestWP1_JournalAlreadyRebaselinedButRecordPendingIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	j := refineJournal{
		Feature:   "my-feature",
		Amendment: 2,
		Completed: append([]string{}, refineJournalSteps...),
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refineJournalPath(cfg, "my-feature"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot,
		"a journal claiming re-baselined while the record is still pending is self-contradictory")
	if !strings.Contains(msg, "re-baselined") {
		t.Errorf("the refusal must name the contradiction; got %q", msg)
	}
}

// Authority proof needs the exact ordered prefix. A journal holding the right
// step names out of order passes NextRefineStep's set test and must still be
// refused.
func TestWP1_OutOfOrderJournalIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	j := refineJournal{
		Feature:   "my-feature",
		Amendment: 2,
		// "tested" before the work it attests to.
		Completed: []string{"amendment-written", "tested", "splice-applied", "rebuilt", "emitted"},
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refineJournalPath(cfg, "my-feature"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "the pipeline was not completed in order")
	if !strings.Contains(msg, "order") {
		t.Errorf("the refusal must say the steps are out of order; got %q", msg)
	}
}

const invalidAmendment = `---
amendment: declares-nothing
date: 2026-08-31
---

## Change

Nothing is declared.

## Acceptance

- Nothing.
`

// A record declaring neither affects: nor a governance field has nothing it
// could have applied. "Nothing to prove" must not read as "proven".
func TestWP1_RecordDeclaringNothingIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-declares-nothing.md", invalidAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "the record declares neither work nor decision")
	if !strings.Contains(msg, "002-declares-nothing") {
		t.Errorf("the refusal must name the empty record; got %q", msg)
	}
}

// A tail holding both a governance record and a combined one must report BOTH
// in one preflight. Reporting only the first hides the second until the next
// run, which is the opposite of telling an operator what is owed.
func TestWP1_AllRefusingRecordsReportedTogether(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-studio-detection-withdrawn.md", govAmendment)
	writeAmendment(t, featureDir, "003-readiness-withdrawn-and-reworded.md", combinedAmendment)
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "two distinct unapplicable records are pending")
	if !strings.Contains(msg, "002-studio-detection-withdrawn") {
		t.Errorf("the governance record must be reported; got %q", msg)
	}
	if !strings.Contains(msg, "003-readiness-withdrawn-and-reworded") {
		t.Errorf("the combined record must be reported in the SAME preflight, not held back "+
			"until the governance one is resolved; got %q", msg)
	}
}

// The journal's Feature field exists so a journal read out of context is
// self-describing. An anonymous one is malformed, and malformed evidence must
// not be grandfathered into an authority grant.
func TestWP1_AnonymousJournalIsNotAuthority(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	j := refineJournal{
		// Feature deliberately absent.
		Amendment: 2,
		Completed: refineJournalSteps[:5],
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refineJournalPath(cfg, "my-feature"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeEmittedManifest(t, cfg, sourceFile)

	msg := assertRefused(t, cfg, sourceRoot, "a journal naming no feature is not self-describing")
	if !strings.Contains(msg, "no feature") {
		t.Errorf("the refusal must say the journal names no feature; got %q", msg)
	}
}

// Scope. The preflight covers exactly the features whose baselines this
// invocation will mutate — no more. An untouched feature carrying a pending
// governance decision must NOT block a partial save that cannot advance its
// marker, or the guard recreates the over-refusal it exists to prevent.
func TestWP1_UntouchedFeatureWithPendingGovernanceDoesNotBlock(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)

	// Feature A: emitted, valid, one journalled splice amendment.
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)

	// Feature B: untouched by this run, sitting on a pending governance record.
	authorFeature(t, cfg, "other-feature", "Other")
	otherDir := cfg.FeaturePath("other-feature")
	writeAmendment(t, otherDir, "001-studio-detection-withdrawn.md", govAmendment)
	if err := os.MkdirAll(cfg.BuildPath("other-feature"), 0o755); err != nil {
		t.Fatal(err)
	}
	otherBaseline := baselinePath(cfg, "other-feature")
	if err := os.WriteFile(otherBaseline, []byte("schema-version: 1\nlast-applied-amendment: 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(otherBaseline)
	if err != nil {
		t.Fatal(err)
	}

	writeEmittedManifest(t, cfg, sourceFile)
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("an untouched feature's pending governance must not block a partial save that "+
			"cannot advance its marker: %v", err)
	}

	after, err := os.ReadFile(otherBaseline)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the untouched feature's baseline was modified by a partial save that did not " +
			"emit it")
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 2 {
		t.Errorf("the emitted feature must still advance to its own record; last-applied = %d",
			bl.LastAppliedAmendment)
	}
}

// ---------------------------------------------------------------------
// WP2 — the authority capsule is copied, never re-derived.
// ---------------------------------------------------------------------

// A save with no pending tail must leave the capsule byte-identical: the
// marker AND the evidence map. "Re-derive the same number" is not the same
// operation as "change nothing", and only the latter is safe.
func TestWP2_NoTailSavePreservesAuthorityCapsuleExactly(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)

	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	// A deliberately stale recorded hash: what the baseline would hold if 001
	// were edited after being honoured. A preserving save must not "fix" it.
	const staleHash = "0000badc0ffee000"
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{"001-first.md": staleHash}
	})

	writeRefineJournal(t, cfg, "my-feature", 1)
	writeEmittedManifest(t, cfg, sourceFile)
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("a feature with no pending tail must save: %v", err)
	}

	after := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if after.LastAppliedAmendment != 1 {
		t.Errorf("marker = %d, want 1 preserved", after.LastAppliedAmendment)
	}
	if got := after.Sources.Amendments["001-first.md"]; got != staleHash {
		t.Errorf("001's recorded hash = %q, want the stored %q untouched. Re-hashing an "+
			"already-applied record silently re-blesses an edit to it and mints fresh trusted "+
			"evidence for a write-once violation", got, staleHash)
	}
}

// The advance case, end to end through the save. Advancing to 002 must append
// only 002's hash and leave 001's stored evidence exactly as it was.
func TestWP2_AdvanceAppendsOnlyTheNewlyProvenRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)

	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	const staleHash = "0000badc0ffee000"
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{"001-first.md": staleHash}
	})
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg, sourceFile)

	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("a journalled single-record tail must save: %v", err)
	}

	after := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if after.LastAppliedAmendment != 2 {
		t.Errorf("marker = %d, want 2", after.LastAppliedAmendment)
	}
	if got := after.Sources.Amendments["001-first.md"]; got != staleHash {
		t.Errorf("001's recorded hash = %q, want %q — an advance appends, it does not restate "+
			"history", got, staleHash)
	}
	if after.Sources.Amendments["002-readiness-wording.md"] == "" {
		t.Error("the newly proven record's hash must be recorded, or the integrity check cannot " +
			"notice a later edit to it")
	}
}

// The zero value of an authority operation must be "preserve". A caller that
// says nothing about authority must not thereby grant it — which is exactly
// how buildBaseline used to grant it to everyone.
func TestWP2_ZeroAuthorityOperationGrantsNothing(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)

	var zero authorityOp
	baseline, err := buildBaselineWithAuthority(cfg, "my-feature", zero)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.LastAppliedAmendment != 0 {
		t.Errorf("the zero authority operation granted last-applied %d — the default must be "+
			"preserve, never advance", baseline.LastAppliedAmendment)
	}
	if len(baseline.Sources.Amendments) != 0 {
		t.Errorf("the zero authority operation recorded hashes %v", baseline.Sources.Amendments)
	}
}

// The surviving full-path inference must be something a caller opts into, not
// something a helper inherits. With noInference, an unproven splice tail on
// the full path refuses rather than advancing.
func TestWP2_FullPathWithoutInferenceRefusesUnprovenTail(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)

	_, err := planAppliedAuthority(cfg, []string{"my-feature"}, false, nil, noInference, outputlessClaim{})
	if err == nil {
		t.Fatal("a caller asserting no proof must not advance an unapplied tail")
	}
	if !strings.Contains(err.Error(), "001-first") {
		t.Errorf("the refusal must name the unproven record; got %v", err)
	}

	// The same tail advances when the caller explicitly stands behind the
	// generate-code workflow contract.
	plan, err := planAppliedAuthority(cfg, []string{"my-feature"}, false, nil, fullGreenBuildInference, outputlessClaim{})
	if err != nil {
		t.Fatalf("the full-save workflow assertion must still advance a splice tail: %v", err)
	}
	if op := plan.Ops["my-feature"]; op.Mode != authorityAdvance || op.To != 1 {
		t.Errorf("expected an advance to 1, got mode=%v to=%d", op.Mode, op.To)
	}
}

// hashWholeFile is advisory everywhere else: it returns false for a missing or
// unreadable file so ordinary hashing never fails a baseline. Used for
// authority that would be forgery in the other direction — the marker advances
// to To while the evidence for a record it advanced past is quietly dropped,
// leaving marker authority with nothing to check it against.
func TestWP2_AdvanceWithUnhashableRecordFails(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)

	amendments, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(amendments) != 1 {
		t.Fatalf("fixture: want 1 amendment, got %d", len(amendments))
	}
	// The record vanishes between planning and baseline construction.
	if err := os.Remove(amendments[0].Path); err != nil {
		t.Fatal(err)
	}

	baseline, err := buildBaselineWithAuthority(cfg, "my-feature",
		advanceAuthority(appliedAuthority{}, 1, amendments))
	if err == nil {
		t.Fatalf("an advance whose evidence cannot be read must fail; got a baseline marked "+
			"through %d", baseline.LastAppliedAmendment)
	}
	if !strings.Contains(err.Error(), "001-first.md") {
		t.Errorf("the error must name the record whose evidence is missing; got %v", err)
	}
}

// The capsule API is an authority boundary callable from outside the planner,
// so it validates its input rather than trusting it.
func TestWP2_AuthorityOperationInvariants(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	all, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		op   authorityOp
		want string
	}{
		{
			name: "advance naming no records",
			op:   authorityOp{Mode: authorityAdvance, To: 2},
			want: "names no records",
		},
		{
			name: "To past the last authorised record",
			op:   authorityOp{Mode: authorityAdvance, To: 9, Newly: all},
			want: "past a record nobody proved",
		},
		{
			name: "record at or below the prior marker",
			op: authorityOp{
				Mode:  authorityAdvance,
				Prior: appliedAuthority{Through: 1},
				To:    2,
				Newly: all, // 001 is at the prior marker
			},
			want: "strictly increasing",
		},
		{
			name: "preserve carrying an advance payload",
			op:   authorityOp{Mode: authorityPreserve, To: 2, Newly: all},
			want: "must not be expressible at once",
		},
		{
			name: "unknown mode",
			op:   authorityOp{Mode: authorityMode(42)},
			want: "unknown authority mode",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildBaselineWithAuthority(cfg, "my-feature", tc.op); err == nil {
				t.Fatal("the operation must be refused at the boundary")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// Legitimate compaction leaves sequence gaps, so the invariant is strictly
// increasing above the prior marker — never consecutive.
func TestWP2_AdvanceAcceptsCompactedSequenceGaps(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "004-first.md", appliedAmendment)
	writeAmendment(t, featureDir, "009-readiness-wording.md", spliceAmendment)
	all, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}

	baseline, err := buildBaselineWithAuthority(cfg, "my-feature",
		advanceAuthority(appliedAuthority{Through: 2}, 9, all))
	if err != nil {
		t.Fatalf("a compacted ledger's gaps are legitimate, not an invariant violation: %v", err)
	}
	if baseline.LastAppliedAmendment != 9 {
		t.Errorf("marker = %d, want 9", baseline.LastAppliedAmendment)
	}
}

// ---------------------------------------------------------------------
// WP4 — the output-less blessing path.
// ---------------------------------------------------------------------

// armOutputless sets the claim flags for the duration of a test.
func armOutputless(t *testing.T, slug string, confirmed bool) {
	t.Helper()
	saveBuildStateOutputless = slug
	saveBuildStateConfirmOutless = confirmed
	t.Cleanup(func() { saveBuildStateOutputless = ""; saveBuildStateConfirmOutless = false })
}

// specOnlyFeature: a feature with a ledger and a baseline but no source file
// of its own, which is exactly the shape that could never be re-blessed.
func specOnlyFeature(t *testing.T, dir string) (string, string) {
	t.Helper()
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	// Another feature owns the only marked source, so the source root is not
	// empty but nothing in it belongs to my-feature.
	sourceRoot := filepath.Join(dir, "cmd")
	writeMarkedFile(t, filepath.Join(sourceRoot, "other", "other.go"), "other-feature", "o", "package other")
	authorFeature(t, cfg, "other-feature", "Other")
	return featureDir, sourceRoot
}

// The headline: a spec-only feature's amendment can now be recorded applied.
func TestWP4_OutputlessFeatureCanBeBlessed(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)

	writeEmittedManifest(t, cfg) // explicitly present, empty
	armOutputless(t, "@my-feature", true)

	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("a confirmed output-less feature with a proven tail must be blessable: %v", err)
	}
	after := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if after.LastAppliedAmendment != 2 {
		t.Errorf("last-applied = %d, want 2", after.LastAppliedAmendment)
	}

	// The durable record: the authority is on disk with the reason it was
	// granted beside it, not justified only by a flag that has evaporated.
	var pbl ProjectBaseline
	data, err := os.ReadFile(projectBaselinePath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &pbl); err != nil {
		t.Fatal(err)
	}
	if len(pbl.Outputless) != 1 || pbl.Outputless[0] != "my-feature" {
		t.Errorf("outputless record = %v, want [my-feature]", pbl.Outputless)
	}
	// And emittedFeatures was NOT forged: the feature emitted nothing, so it
	// must not appear in the provenance/reporting set.
	for _, e := range pbl.Emitted {
		if e == "my-feature" {
			t.Error("the output-less slug leaked into emitted: — that set is provenance, and a " +
				"feature that wrote no files must not be reported as having emitted")
		}
	}
}

// The confirmation is user authority and the zero value refuses.
func TestWP4_OutputlessWithoutConfirmationRefuses(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg)
	armOutputless(t, "@my-feature", false)

	msg := assertRefused(t, cfg, sourceRoot, "nothing may default the human assertion to true")
	if !strings.Contains(msg, "--confirm-outputless") {
		t.Errorf("the refusal must name the confirmation it requires; got %q", msg)
	}
}

// The one Codex asked for specifically. A combined record carries both
// affects: and supersedes_intents:. --confirm-outputless asserts "this feature
// owes no generated code" — it is NOT confirmation of a promise list, and a
// perfect empty manifest with a tested journal must not withdraw promises.
func TestWP4_OutputlessCannotDowngradeACombinedRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-withdrawn-and-reworded.md", combinedAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg)
	armOutputless(t, "@my-feature", true)

	msg := assertRefused(t, cfg, sourceRoot,
		"an output-less confirmation is not a promise-list confirmation")
	if !strings.Contains(msg, "002-readiness-withdrawn-and-reworded") {
		t.Errorf("the refusal must name the combined record; got %q", msg)
	}
	after := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if after.LastAppliedAmendment != 1 {
		t.Errorf("marker moved to %d — an output-less flag downgraded a combined record",
			after.LastAppliedAmendment)
	}
}

// Pure governance likewise: the output-less path is not a second door to it.
func TestWP4_OutputlessCannotApplyGovernance(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-studio-detection-withdrawn.md", govAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg)
	armOutputless(t, "@my-feature", true)

	msg := assertRefused(t, cfg, sourceRoot, "governance stays apply-governance's alone")
	if !strings.Contains(msg, "apply-governance") {
		t.Errorf("the refusal must route to apply-governance; got %q", msg)
	}
}

// A missing manifest says "nothing was recorded", not "nothing was written".
// The two must not be spelled the same way.
func TestWP4_MissingManifestIsNotAnEmptyOne(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)

	saveBuildStatePartial = true
	saveBuildStateEmitted = filepath.Join(cfg.ProjectBuildPath(), "does-not-exist")
	t.Cleanup(func() { saveBuildStatePartial = false; saveBuildStateEmitted = "" })
	armOutputless(t, "@my-feature", true)

	if _, _, err := runProjectSave(t, cfg, sourceRoot); err == nil {
		t.Fatal("a missing manifest must not satisfy an output-less claim")
	}
}

// A feature that owes generated code is not output-less, however empty this
// particular run's manifest happens to be.
func TestWP4_FeatureWithPlannedOutputIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)

	bf := "feature: my-feature\nplan:\n  creates:\n    - path: cmd/mine/mine.go\n"
	if err := os.WriteFile(filepath.Join(cfg.BuildPath("my-feature"), "buildfile.yaml"), []byte(bf), 0o644); err != nil {
		t.Fatal(err)
	}
	writeEmittedManifest(t, cfg)
	armOutputless(t, "@my-feature", true)

	msg := assertRefused(t, cfg, sourceRoot, "the buildfile predicts a file this run did not write")
	if !strings.Contains(msg, "owes generated code") {
		t.Errorf("the refusal must say the feature owes output; got %q", msg)
	}
}

// A feature that already owns tracked generated files goes through the
// ordinary manifest, not this path. Driven against the validator directly with
// a repo-root-relative snapshot key, which is the shape production writes.
func TestWP4_FeatureOwningGeneratedFilesIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	setupLedgerFeature(t, dir)

	rel := filepath.Join("cmd", "mine", "mine.go")
	writeMarkedFile(t, filepath.Join(dir, rel), "my-feature", "readiness", "package mine")
	snapshot := "generated-at: \"2026-08-31T00:00:00Z\"\nfiles:\n    " + rel +
		":\n        component: my-feature/readiness\n        hash: aaaaaaaaaaaaaaaa\n"
	if err := os.MkdirAll(cfg.ProjectBuildPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectCodeHashesPath(cfg), []byte(snapshot), 0o644); err != nil {
		t.Fatal(err)
	}

	owned, failures := generatedFilesOwnedBy(cfg, "my-feature")
	if len(failures) > 0 || len(owned) == 0 {
		t.Fatalf("fixture: the feature must own tracked output; owned=%v failures=%v", owned, failures)
	}

	err := validateOutputlessClaim(cfg,
		outputlessClaim{Feature: "my-feature", Confirmed: true},
		[]string{"my-feature"}, true, &emissionDeclaration{Paths: map[string]bool{}})
	if err == nil {
		t.Fatal("a feature owning tracked generated files is not output-less")
	}
	if !strings.Contains(err.Error(), "not output-less") {
		t.Errorf("the refusal must say it is not output-less; got %v", err)
	}
}

// An unreadable ownership answer is not an empty one.
func TestWP4_UnreadableOwnershipRefuses(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	setupLedgerFeature(t, dir)
	if err := os.MkdirAll(cfg.ProjectBuildPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectCodeHashesPath(cfg), []byte("{{ not: [valid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = dir

	err := validateOutputlessClaim(cfg,
		outputlessClaim{Feature: "my-feature", Confirmed: true},
		[]string{"my-feature"}, true, &emissionDeclaration{Paths: map[string]bool{}})
	if err == nil {
		t.Fatal("an output-less claim is not safe on an unreadable ownership answer")
	}
}

// The output-less path relaxes membership, never what counts as a completed
// refinement: the same journal evidence is required.
func TestWP4_OutputlessStillRequiresAProvenTail(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	// No journal at all.
	writeEmittedManifest(t, cfg)
	armOutputless(t, "@my-feature", true)

	msg := assertRefused(t, cfg, sourceRoot, "an output-less claim proves membership, not work")
	if !strings.Contains(msg, "002-readiness-wording") {
		t.Errorf("the refusal must name the unproven record; got %q", msg)
	}
}

// The durable evidence lives with the authority it explains, in the same file
// and the same atomic write, keyed by the exact amendment filename.
func TestWP4_OutputlessEvidenceLivesInTheFeatureCapsule(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg)
	armOutputless(t, "@my-feature", true)

	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("outputless save: %v", err)
	}

	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 2 {
		t.Fatalf("marker = %d, want 2", bl.LastAppliedAmendment)
	}
	// Marker, hash and method together, keyed by the same exact filename.
	if bl.Sources.Amendments["002-readiness-wording.md"] == "" {
		t.Error("the hash must be recorded")
	}
	if !bl.OutputlessAmendments["002-readiness-wording.md"] {
		t.Errorf("the output-less method must be recorded against the exact amendment; got %v",
			bl.OutputlessAmendments)
	}
}

// The evidence must survive later unrelated saves. Under the old
// project-baseline placement the next save replaced it wholesale while the
// marker and hash it explained stayed put.
func TestWP4_OutputlessEvidenceSurvivesLaterSaves(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot := specOnlyFeature(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg)
	armOutputless(t, "@my-feature", true)
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("outputless save: %v", err)
	}

	// A later, entirely unrelated full save.
	saveBuildStateOutputless = ""
	saveBuildStateConfirmOutless = false
	saveBuildStatePartial = false
	saveBuildStateEmitted = ""
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("later unrelated save: %v", err)
	}

	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if !bl.OutputlessAmendments["002-readiness-wording.md"] {
		t.Errorf("a later save erased the explanation while keeping the authority it justified; "+
			"outputless-amendments = %v", bl.OutputlessAmendments)
	}
	if bl.Sources.Amendments["002-readiness-wording.md"] == "" {
		t.Error("the hash must survive too")
	}
}

// Preserve copies the map forward untouched.
func TestWP4_PreserveRetainsOutputlessEvidence(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)

	prior := appliedAuthority{
		Through:    1,
		Hashes:     map[string]string{"001-first.md": "0000badc0ffee000"},
		Outputless: map[string]bool{"001-first.md": true},
	}
	bl, err := buildBaselineWithAuthority(cfg, "my-feature", preserveAuthority(prior))
	if err != nil {
		t.Fatal(err)
	}
	if !bl.OutputlessAmendments["001-first.md"] {
		t.Errorf("preserve must copy the method forward untouched; got %v", bl.OutputlessAmendments)
	}
	if bl.Sources.Amendments["001-first.md"] != "0000badc0ffee000" {
		t.Error("preserve must copy the hash forward untouched")
	}
}

// An ordinary emitted advance must not label its record output-less.
func TestWP4_OrdinaryAdvanceIsNotLabelledOutputless(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg, sourceFile)

	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("ordinary partial save: %v", err)
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.OutputlessAmendments["002-readiness-wording.md"] {
		t.Error("a record blessed on emitted files must not be recorded as output-less")
	}
}

// A confirmation with no named subject confirms nothing.
func TestWP4_OrphanConfirmationIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	_, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	writeEmittedManifest(t, cfg, sourceFile)
	armOutputless(t, "", true)

	msg := assertRefused(t, cfg, sourceRoot, "a confirmation with no subject must not be a no-op")
	if !strings.Contains(msg, "--outputless-feature") {
		t.Errorf("the refusal must name the missing subject flag; got %q", msg)
	}
}

// ---------------------------------------------------------------------
// Cross-path: apply-governance writes authority too, and used to forge it.
// ---------------------------------------------------------------------

func runApplyGovernance_(t *testing.T, cfg *config.Context, ref string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := runApplyGovernance(cmd, []string{ref})
	return buf.String(), err
}

// The forging primitive: apply-governance used to re-hash ALL applied history
// on every run, so an edit to an already-applied amendment was silently
// re-blessed with fresh evidence. WP3 now trusts exactly these hashes.
func TestXP_GovernanceApplicationPreservesPriorEvidence(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)

	// A second founding intent, so withdrawing the first is an ordinary
	// governance amendment rather than a whole-feature retirement.
	intents, err := os.ReadFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		t.Fatal(err)
	}
	intents = append(intents, []byte("\n## Survives\n\n**Goal**: Something that stays.\n**Persona**: Admin\n")...)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), intents, 0o644); err != nil {
		t.Fatal(err)
	}
	// 001's affects: target exists, so its ref resolves on its own merits and
	// this test isolates the capsule behaviour rather than re-testing WP3.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	const staleHash = "0000badc0ffee000"
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{"001-first.md": staleHash}
		b.OutputlessAmendments = map[string]bool{"001-first.md": true}
	})
	writeAmendment(t, featureDir, "002-studio-detection-withdrawn.md", govAmendment)

	applyGovernanceConfirmed = true
	t.Cleanup(func() { applyGovernanceConfirmed = false })
	if _, err := runApplyGovernance_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("applying a pending governance record must succeed: %v", err)
	}

	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 2 {
		t.Errorf("marker = %d, want 2", bl.LastAppliedAmendment)
	}
	if got := bl.Sources.Amendments["001-first.md"]; got != staleHash {
		t.Errorf("001's recorded hash = %q, want the stored %q. Re-hashing already-applied "+
			"history re-blesses an edit to it and mints fresh trusted evidence for a write-once "+
			"violation — and applied-history resolution now trusts exactly this value", got, staleHash)
	}
	if !bl.OutputlessAmendments["001-first.md"] {
		t.Error("the prior application method must survive governance application")
	}
	if bl.Sources.Amendments["002-studio-detection-withdrawn.md"] == "" {
		t.Error("the newly applied governance record's hash must be recorded")
	}
}

// The other half: advancing past a record whose file cannot be read must
// refuse, not advance with no evidence behind it.
func TestXP_GovernanceApplicationRefusesUnhashableRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-studio-detection-withdrawn.md", govAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 0
		b.Sources.Amendments = map[string]string{}
	})

	applyGovernanceConfirmed = true
	t.Cleanup(func() { applyGovernanceConfirmed = false })

	// The record vanishes between the ledger read and the capsule write.
	amendPath := filepath.Join(featureDir, "amendments", "001-studio-detection-withdrawn.md")
	data, err := os.ReadFile(amendPath)
	if err != nil {
		t.Fatal(err)
	}
	amendments, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(amendPath); err != nil {
		t.Fatal(err)
	}
	err = advanceAppliedMarker(cfg, "my-feature", 1, amendments)
	if err == nil {
		t.Error("advancing past a record whose evidence cannot be read must refuse — a marker " +
			"with nothing behind it is authority nobody can check")
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 0 {
		t.Errorf("the marker moved to %d on a refused advance", bl.LastAppliedAmendment)
	}
	_ = data
}

// An unreadable baseline refuses rather than being rebuilt from nothing.
func TestXP_GovernanceApplicationRefusesCorruptBaseline(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-studio-detection-withdrawn.md", govAmendment)
	if err := os.MkdirAll(cfg.BuildPath("my-feature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath(cfg, "my-feature"), []byte("{{ not: [valid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	amendments, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := advanceAppliedMarker(cfg, "my-feature", 1, amendments); err == nil {
		t.Error("a corrupt baseline must refuse a governance advance")
	}
}
