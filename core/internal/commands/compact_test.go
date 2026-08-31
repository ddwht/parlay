package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// WP5 — compaction is an authority-projection operation.

func loadFeatureAmendmentsForTest(t *testing.T, featureDir string) ([]parser.Amendment, error) {
	t.Helper()
	return parser.LoadFeatureAmendments(featureDir)
}

func runCompact_(t *testing.T, featureRef string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := runCompact(cmd, []string{featureRef})
	return buf.String(), err
}

func armCompact(t *testing.T, through int, confirm bool) {
	t.Helper()
	compactThrough = through
	compactConfirm = confirm
	t.Cleanup(func() { compactThrough = 0; compactConfirm = false })
}

// compactFixture: a feature at last-applied 2 with both records trusted.
func compactFixture(t *testing.T, dir string) string {
	t.Helper()
	featureDir := setupLedgerFeature(t, dir)
	// The affects: targets exist, so these tests isolate compaction rather
	// than re-testing applied-history resolution.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 2
		b.Sources.Amendments = map[string]string{}
		for _, n := range []string{"001-first.md", "002-readiness-wording.md"} {
			if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", n)); ok {
				b.Sources.Amendments[n] = hash
			}
		}
	})
	return featureDir
}

func archivedNames(t *testing.T, featureDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(featureDir, "amendments", "archive"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// The happy path, and the property that defines it: authority unchanged.
func TestWP5_CompactPreservesAuthorityProjection(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)

	before, err := computeAuthorityProjection(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	armCompact(t, 0, true)
	if _, err := runCompact_(t, "@my-feature"); err != nil {
		t.Fatalf("compacting a fully applied ledger must succeed: %v", err)
	}

	if got := archivedNames(t, featureDir); len(got) != 2 {
		t.Errorf("archived = %v, want both records", got)
	}
	after, err := computeAuthorityProjection(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if before.canonical() != after.canonical() {
		t.Errorf("compaction changed what the feature promises:\nbefore:\n%s\nafter:\n%s",
			before.canonical(), after.canonical())
	}
	// The capsule is evidence, not location: byte-identical either side.
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 2 || len(bl.Sources.Amendments) != 2 {
		t.Errorf("the authority capsule must be untouched; marker=%d hashes=%v",
			bl.LastAppliedAmendment, bl.Sources.Amendments)
	}
}

// Trust, not "seq <= marker". A hand-moved marker must not let compaction
// retire a record nobody can show was applied.
func TestWP5_UntrustedRecordRefusesCompaction(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := compactFixture(t, dir)
	// Drop 001's evidence: the marker still covers it, nothing proves it.
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 2
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "002-readiness-wording.md")); ok {
			b.Sources.Amendments = map[string]string{"002-readiness-wording.md": hash}
		}
	})

	armCompact(t, 0, true)
	out, err := runCompact_(t, "@my-feature")
	if err == nil {
		t.Fatal("a record with no recorded evidence must not be compacted")
	}
	if !strings.Contains(out+err.Error(), "001-first") {
		t.Errorf("the refusal must name the untrusted record; got %v", err)
	}
	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("a refused compaction must move nothing; archived = %v", got)
	}
}

// A threshold that splits a supersedes edge leaves the active side naming a
// record no longer in the ledger. Refuse rather than dangle.
func TestWP5_SplitSupersedesEdgeRefuses(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", appliedAmendment)
	writeAmendment(t, featureDir, "002-supersedes-first.md", `---
amendment: supersedes-first
date: 2026-08-31
supersedes:
  - first
affects: ["@my-feature/operation:x"]
---

## Change
Corrects 001.

## Acceptance
- Corrected.
`)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 2
		b.Sources.Amendments = map[string]string{}
		for _, n := range []string{"001-first.md", "002-supersedes-first.md"} {
			if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", n)); ok {
				b.Sources.Amendments[n] = hash
			}
		}
	})

	// Archive only 001; 002 stays active naming it.
	armCompact(t, 1, true)
	out, err := runCompact_(t, "@my-feature")
	if err == nil {
		t.Fatal("archiving one end of a supersession edge must refuse")
	}
	if !strings.Contains(out+err.Error(), "supersedes") {
		t.Errorf("the refusal must explain the split edge; got %v", err)
	}
	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("nothing may move; archived = %v", got)
	}
}

// Archiving a record that retired a founding intent would bring that promise
// back into force, because the loader never sees the archived claim.
func TestWP5_ArchivingAnIntentSupersessionRefuses(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-studio-detection-withdrawn.md", govAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-studio-detection-withdrawn.md")); ok {
			b.Sources.Amendments = map[string]string{"001-studio-detection-withdrawn.md": hash}
		}
	})

	armCompact(t, 0, true)
	out, err := runCompact_(t, "@my-feature")
	if err == nil {
		t.Fatal("archiving the only record retiring a founding intent must refuse")
	}
	if !strings.Contains(out+err.Error(), "check-readiness") {
		t.Errorf("the refusal must name the promise that would return; got %v", err)
	}
	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("nothing may move; archived = %v", got)
	}
}

// History is written once. A destination collision refuses rather than
// overwriting what is already archived — and refuses before moving anything.
func TestWP5_DestinationCollisionRefusesBeforeMoving(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := compactFixture(t, dir)
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "002-readiness-wording.md"),
		[]byte("a different history\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	armCompact(t, 0, true)
	out, err := runCompact_(t, "@my-feature")
	if err == nil {
		t.Fatal("a colliding destination must refuse")
	}
	if !strings.Contains(out+err.Error(), "already exists in archive/") {
		t.Errorf("the refusal must name the collision; got %v", err)
	}
	// Preflight-before-move: 001 must still be in the active ledger.
	if _, err := os.Stat(filepath.Join(featureDir, "amendments", "001-first.md")); err != nil {
		t.Error("the collision was discovered after moving another record — preflight must " +
			"check every destination before the first rename")
	}
	// And the pre-existing archived file is untouched.
	data, err := os.ReadFile(filepath.Join(archive, "002-readiness-wording.md"))
	if err != nil || string(data) != "a different history\n" {
		t.Error("existing archived history must never be overwritten")
	}
}

// Without --confirm it reports and stops.
func TestWP5_RequiresConfirmation(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := compactFixture(t, dir)
	armCompact(t, 0, false)
	if _, err := runCompact_(t, "@my-feature"); err == nil {
		t.Fatal("compaction rewrites where history lives and must be confirmed")
	}
	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("an unconfirmed run must move nothing; archived = %v", got)
	}
}

// The terminal retirement record must stay: archiving it would make the
// feature read as live again.
func TestWP5_TerminalRetirementRecordIsNotArchivable(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-closed.md", `---
amendment: closed
date: 2026-08-31
supersedes_intents:
  - check-readiness
retires_feature: true
outcome: obsolete
---

## Change
Closed.

## Why
Nothing remains.

## Acceptance
- Nothing reports readiness.
`)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-closed.md")); ok {
			b.Sources.Amendments = map[string]string{"001-closed.md": hash}
		}
	})

	armCompact(t, 0, true)
	if _, err := runCompact_(t, "@my-feature"); err == nil {
		t.Fatal("the terminal retirement record must not be archivable")
	}
	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("nothing may move; archived = %v", got)
	}
}

// The rollback primitive itself. The projection gate is meant to make a
// post-move mismatch unreachable, but "unreachable" is a claim about today's
// selection logic, and the recovery path must work regardless — a defensive
// path that has never run is a defensive path that does not work.
func TestWP5_RollbackRestoresEveryMovedRecord(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := compactFixture(t, dir)

	amendDir := filepath.Join(featureDir, "amendments")
	before := map[string][]byte{}
	for _, n := range []string{"001-first.md", "002-readiness-wording.md"} {
		data, err := os.ReadFile(filepath.Join(amendDir, n))
		if err != nil {
			t.Fatal(err)
		}
		before[n] = data
	}

	records, err := loadFeatureAmendmentsForTest(t, featureDir)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := archiveRecords(featureDir, records)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(moved) != 2 {
		t.Fatalf("moved = %d, want 2", len(moved))
	}

	if problems := rollbackMoves(featureDir, moved); len(problems) > 0 {
		t.Fatalf("rollback reported problems: %v", problems)
	}

	for n, want := range before {
		got, err := os.ReadFile(filepath.Join(amendDir, n))
		if err != nil {
			t.Errorf("%s was not restored: %v", n, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s was restored with different content", n)
		}
	}
	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("rollback must leave archive/ empty; got %v", got)
	}
}

// Crash recovery. A same-process error hook that calls rollback is not a crash
// test: the process death is the whole point, because it takes the in-memory
// list of what was moved with it. These simulate the crash by leaving the
// filesystem and the journal in the exact state a death would, then invoke
// recovery through a FRESH command call.

// simulateCrashMidCompaction performs the journal write and the first rename,
// then stops — the state a process death after rename 1 leaves behind.
func simulateCrashMidCompaction(t *testing.T, cfg *config.Context, featureDir, slug string) compactJournal {
	t.Helper()
	records, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := computeAuthorityProjection(cfg, slug)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	// Journal only what a real compaction would select: records at or below
	// the applied marker. A pending record could never appear here, and a
	// simulator that journals one is testing an impossible state.
	capsule, err := observeAppliedAuthority(cfg, slug)
	if err != nil {
		t.Fatal(err)
	}
	j := compactJournal{Feature: slug, BeforeProjection: before.canonical()}
	for _, a := range records {
		if a.Seq <= capsule.Through {
			j.Amendments = append(j.Amendments, filepath.Base(a.Path))
		}
	}
	if len(j.Amendments) == 0 {
		t.Fatal("fixture: no applied records to compact")
	}
	if err := writeCompactJournal(compactJournalPath(cfg, slug), j); err != nil {
		t.Fatal(err)
	}
	// Exactly one rename, then "die".
	first := j.Amendments[0]
	if err := os.Rename(filepath.Join(featureDir, "amendments", first),
		filepath.Join(archive, first)); err != nil {
		t.Fatal(err)
	}
	return j
}

// A fresh invocation finds the interrupted run and restores the before state
// before doing anything else.
func TestWP5_RecoversFromCrashAfterFirstRename(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)

	before, err := computeAuthorityProjection(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	simulateCrashMidCompaction(t, cfg, featureDir, "my-feature")

	// Half-compacted: one archived, one still active.
	if got := archivedNames(t, featureDir); len(got) != 1 {
		t.Fatalf("fixture: want a half-compacted ledger, archived = %v", got)
	}

	// A fresh command call. It must recover before it does anything else, so
	// even the unconfirmed report path recovers.
	armCompact(t, 0, false)
	_, _ = runCompact_(t, "@my-feature")

	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("recovery must restore the before state; archive still holds %v", got)
	}
	for _, n := range []string{"001-first.md", "002-readiness-wording.md"} {
		if _, err := os.Stat(filepath.Join(featureDir, "amendments", n)); err != nil {
			t.Errorf("%s was not restored to the active ledger: %v", n, err)
		}
	}
	after, err := computeAuthorityProjection(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if before.canonical() != after.canonical() {
		t.Errorf("recovery did not restore the recorded before state:\nbefore:\n%s\nafter:\n%s",
			before.canonical(), after.canonical())
	}
	if _, err := os.Stat(compactJournalPath(cfg, "my-feature")); err == nil {
		t.Error("a completed recovery must clear its journal, or every later run re-recovers")
	}
}

// A crash after ALL renames but before verification is still rolled back:
// nothing compared the projection, so the compaction was never proven safe.
func TestWP5_RecoversFromCrashAfterAllRenames(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)

	records, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := computeAuthorityProjection(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	j := compactJournal{Feature: "my-feature", BeforeProjection: before.canonical()}
	for _, a := range records {
		j.Amendments = append(j.Amendments, filepath.Base(a.Path))
	}
	if err := writeCompactJournal(compactJournalPath(cfg, "my-feature"), j); err != nil {
		t.Fatal(err)
	}
	for _, n := range j.Amendments {
		if err := os.Rename(filepath.Join(featureDir, "amendments", n),
			filepath.Join(archive, n)); err != nil {
			t.Fatal(err)
		}
	}

	if err := recoverCompaction(cfg, "my-feature"); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if got := archivedNames(t, featureDir); len(got) != 0 {
		t.Errorf("an unverified compaction must be rolled back, not adopted; archived = %v", got)
	}
	after, err := computeAuthorityProjection(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if before.canonical() != after.canonical() {
		t.Error("recovery did not restore the recorded before state")
	}
}

// A record present in BOTH places is ambiguous history. Recovery refuses to
// choose, and refuses to let another compaction start over it.
func TestWP5_RecoveryRefusesAmbiguousResidue(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)
	j := simulateCrashMidCompaction(t, cfg, featureDir, "my-feature")

	// Put a copy back in the active ledger without clearing the archived one.
	first := j.Amendments[0]
	data, err := os.ReadFile(filepath.Join(featureDir, "amendments", "archive", first))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "amendments", first), data, 0o644); err != nil {
		t.Fatal(err)
	}

	err = recoverCompaction(cfg, "my-feature")
	if err == nil {
		t.Fatal("a record in both the ledger and archive/ is ambiguous and must not be resolved silently")
	}
	if !strings.Contains(err.Error(), "BOTH") {
		t.Errorf("the error must name the ambiguity; got %v", err)
	}
	// And a fresh compaction must not proceed over it.
	armCompact(t, 0, true)
	if _, err := runCompact_(t, "@my-feature"); err == nil {
		t.Error("compaction must not start over an unrecovered interrupted run")
	}
}

// A corrupt journal is evidence a run died, not something to discard.
func TestWP5_CorruptJournalRefusesRatherThanDiscards(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)
	_ = featureDir
	if err := os.MkdirAll(cfg.BuildPath("my-feature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compactJournalPath(cfg, "my-feature"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := recoverCompaction(cfg, "my-feature"); err == nil {
		t.Error("a damaged compaction journal must refuse, not be discarded — discarding it " +
			"strands a possibly part-compacted ledger with nothing recording the run")
	}
}

// The journal is DATA, not instructions. These are the recovery equivalents of
// the trust rules the normal command already enforces: a recovery path that
// trusts its own input is not a recovery path.

// A journal recording another feature must not be applied here.
func TestWP5_RecoveryRefusesForeignJournal(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)
	j := simulateCrashMidCompaction(t, cfg, featureDir, "my-feature")
	j.Feature = "someone-elses-feature"
	if err := writeCompactJournal(compactJournalPath(cfg, "my-feature"), j); err != nil {
		t.Fatal(err)
	}

	err := recoverCompaction(cfg, "my-feature")
	if err == nil {
		t.Fatal("a journal about another feature must not be applied to this one")
	}
	if !strings.Contains(err.Error(), "someone-elses-feature") {
		t.Errorf("the refusal must name the mismatch; got %v", err)
	}
}

// A journal naming a path rather than a ledger filename must move nothing.
func TestWP5_RecoveryRefusesPathsAndTraversal(t *testing.T) {
	for _, name := range []string{
		"../../../etc/passwd",
		"/tmp/somewhere/001-first.md",
		"subdir/001-first.md",
		"not-an-amendment.txt",
	} {
		t.Run(name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg := testContext(t)
			featureDir := compactFixture(t, dir)

			// A canary the forged journal must not touch.
			canary := filepath.Join(dir, "canary.txt")
			if err := os.WriteFile(canary, []byte("untouched\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			before, err := computeAuthorityProjection(cfg, "my-feature")
			if err != nil {
				t.Fatal(err)
			}
			j := compactJournal{
				Feature:          "my-feature",
				BeforeProjection: before.canonical(),
				Amendments:       []string{name},
			}
			if err := writeCompactJournal(compactJournalPath(cfg, "my-feature"), j); err != nil {
				t.Fatal(err)
			}

			if err := recoverCompaction(cfg, "my-feature"); err == nil {
				t.Fatal("recovery must derive its own paths and accept none from the journal")
			}
			if data, err := os.ReadFile(canary); err != nil || string(data) != "untouched\n" {
				t.Error("a forged journal reached a file outside the ledger")
			}
			// The real ledger is intact.
			for _, n := range []string{"001-first.md", "002-readiness-wording.md"} {
				if _, err := os.Stat(filepath.Join(featureDir, "amendments", n)); err != nil {
					t.Errorf("%s was disturbed by a refused recovery", n)
				}
			}
		})
	}
}

// The window between the crash and recovery is unbounded. A record whose bytes
// changed while archived must not be moved back and declared recovered — that
// would launder a mutation into the active ledger, and the projection cannot
// notice because it carries the baseline's STORED evidence, not the bytes.
func TestWP5_RecoveryRefusesMutatedArchivedRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)
	j := simulateCrashMidCompaction(t, cfg, featureDir, "my-feature")

	// The mutation keeps the record VALID and its frontmatter identical, so
	// the projection is unchanged and only the bytes differ. That isolates the
	// byte check: a projection comparison alone cannot see this, because the
	// projection carries the baseline's stored evidence rather than the bytes.
	archived := filepath.Join(featureDir, "amendments", "archive", j.Amendments[0])
	mutated := strings.Replace(appliedAmendment, "The original change.", "Quietly reworded.", 1)
	if mutated == appliedAmendment {
		t.Fatal("fixture: the mutation did not change anything")
	}
	if err := os.WriteFile(archived, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}

	err := recoverCompaction(cfg, "my-feature")
	if err == nil {
		t.Fatal("a record whose bytes changed during the interrupted window must not be " +
			"restored and declared recovered")
	}
	if !strings.Contains(err.Error(), "no longer match the evidence") {
		t.Errorf("the error must say the bytes no longer match the recorded evidence; got %v", err)
	}
	if _, err := os.Stat(compactJournalPath(cfg, "my-feature")); err != nil {
		t.Error("an incomplete recovery must LEAVE its journal, or the next run has no record " +
			"that a compaction was in flight")
	}
}

// Locations can be restorable while the ledger has still moved on. Restoring
// files is only half of recovery.
func TestWP5_RecoveryRefusesWhenProjectionMovedOn(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)
	simulateCrashMidCompaction(t, cfg, featureDir, "my-feature")

	// A new pending amendment appears while the compaction is interrupted:
	// the ledger's authority projection is no longer what was recorded.
	writeAmendment(t, featureDir, "003-later-change.md", spliceAmendment)

	err := recoverCompaction(cfg, "my-feature")
	if err == nil {
		t.Fatal("a restored ledger that does not reproduce the recorded projection is not recovered")
	}
	if !strings.Contains(err.Error(), "does not reproduce the projection") {
		t.Errorf("the error must say the projection differs; got %v", err)
	}
	if _, err := os.Stat(compactJournalPath(cfg, "my-feature")); err != nil {
		t.Error("the journal must be left in place on an incomplete recovery")
	}
}

// A compaction journal is only ever written for TRUSTED APPLIED records, and
// trust requires a stored hash under that exact filename. If it is gone at
// recovery time, the authority state changed under the interrupted run or the
// journal is not one this tool wrote — either way, discharging it would verify
// nothing.
func TestWP5_RecoveryRefusesWhenRecordedEvidenceVanished(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)

	// The evidence is removed BEFORE the projection is captured, so the
	// restored state reproduces the recorded projection exactly and the
	// missing-hash check is the only thing that can catch this. That is also
	// the honest shape of the scenario: a journal naming a record with no
	// recorded evidence is one this tool would never have written.
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	delete(bl.Sources.Amendments, "001-first.md")
	data, err := marshalBaseline(&bl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath(cfg, "my-feature"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	simulateCrashMidCompaction(t, cfg, featureDir, "my-feature")

	err = recoverCompaction(cfg, "my-feature")
	if err == nil {
		t.Fatal("recovery must refuse when a journalled record has no recorded evidence")
	}
	if !strings.Contains(err.Error(), "no recorded evidence") {
		t.Errorf("the error must say the evidence is missing; got %v", err)
	}
	if _, statErr := os.Stat(compactJournalPath(cfg, "my-feature")); statErr != nil {
		t.Error("the journal must be retained when recovery refuses")
	}
}

// An unfinished compaction is a GLOBAL authority-mutation barrier, not a
// private concern of the compact command.
//
// Every refusal here is asserted BY IDENTITY, not by err != nil. An earlier
// version of this test checked only that each call failed, which was vacuous
// for apply-governance: the fixture had no pending record, so that call would
// have failed with "no unapplied amendments" even with its guard deleted. Each
// writer is therefore first shown to SUCCEED with no journal present, and only
// then shown to refuse with the barrier's own message once one exists.
func TestWP5_InterruptedCompactionBlocksEveryAuthorityWriter(t *testing.T) {
	const barrier = "interrupted compaction"

	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := compactFixture(t, dir)

	// A second founding intent, so the pending governance record below is an
	// ordinary supersession rather than a whole-feature retirement.
	intents, err := os.ReadFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		t.Fatal(err)
	}
	intents = append(intents, []byte("\n## Survives\n\n**Goal**: Something that stays.\n**Persona**: Admin\n")...)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), intents, 0o644); err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.Join(dir, "cmd")
	sourceFile := filepath.Join(sourceRoot, "mine", "mine.go")
	writeMarkedFile(t, sourceFile, "my-feature", "r", "package mine")

	// Precondition: with no journal and nothing pending, a full save succeeds.
	// If this ever stops being true the refusal below proves nothing.
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("precondition: a full save must succeed with no compaction in flight: %v", err)
	}

	// A PENDING governance record, so apply-governance has real work to do and
	// its refusal cannot be confused with "nothing to apply".
	writeAmendment(t, featureDir, "003-studio-detection-withdrawn.md", govAmendment)

	simulateCrashMidCompaction(t, cfg, featureDir, "my-feature")
	journalPath := compactJournalPath(cfg, "my-feature")
	blBefore, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	archivedBefore := len(archivedNames(t, featureDir))

	// check-amendments surfaces it explicitly, at error severity.
	ca := computeCheckAmendments(cfg, "my-feature")
	var surfaced bool
	for _, iss := range ca.Issues {
		if iss.Code == "amendment-compaction-incomplete" {
			surfaced = true
			if iss.Severity != "error" {
				t.Errorf("severity = %q, want error", iss.Severity)
			}
			if !strings.Contains(iss.Message, "parlay internal compact") {
				t.Errorf("the finding must name the recovery command; got %q", iss.Message)
			}
		}
	}
	if !surfaced {
		t.Errorf("an interrupted compaction must be an explicit finding, not a silently shorter "+
			"listing; issues = %+v", ca.Issues)
	}

	// Each writer refuses, and refuses FOR THIS REASON.
	writeRefineJournal(t, cfg, "my-feature", 3)
	writeEmittedManifest(t, cfg, sourceFile)
	_, stderr, err := runProjectSave(t, cfg, sourceRoot)
	if err == nil {
		t.Error("a partial save must refuse while a compaction is in flight")
	} else if !strings.Contains(err.Error()+stderr, barrier) {
		t.Errorf("the partial save refused for some other reason: %v", err)
	}

	saveBuildStatePartial = false
	saveBuildStateEmitted = ""
	_, stderr, err = runProjectSave(t, cfg, sourceRoot)
	if err == nil {
		t.Error("a full save must refuse while a compaction is in flight")
	} else if !strings.Contains(err.Error()+stderr, barrier) {
		t.Errorf("the full save refused for some other reason: %v", err)
	}

	applyGovernanceConfirmed = true
	t.Cleanup(func() { applyGovernanceConfirmed = false })
	out, err := runApplyGovernance_(t, cfg, "@my-feature")
	if err == nil {
		t.Error("apply-governance must refuse while a compaction is in flight")
	} else if !strings.Contains(err.Error()+out, barrier) {
		t.Errorf("apply-governance refused for some other reason (a pending record exists, so "+
			"this must not be \"nothing to apply\"): %v", err)
	}

	// Nothing was written on the way to refusing.
	blAfter, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	if string(blBefore) != string(blAfter) {
		t.Error("a refused writer still changed the baseline")
	}
	if len(archivedNames(t, featureDir)) != archivedBefore {
		t.Error("a refused writer changed the ledger's locations")
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Error("a refused writer removed the compaction journal")
	}

	// Recovery clears it, and BOTH writers work again — the earlier version
	// proved this only for save.
	armCompact(t, 0, false)
	_, _ = runCompact_(t, "@my-feature")
	if _, err := os.Stat(journalPath); err == nil {
		t.Fatal("recovery must clear the journal")
	}
	for _, iss := range computeCheckAmendments(cfg, "my-feature").Issues {
		if iss.Code == "amendment-compaction-incomplete" {
			t.Error("the finding must clear once recovery has run")
		}
	}
	// Governance first: it is what clears the pending record, after which an
	// ordinary save has nothing unproven to refuse over.
	if _, err := runApplyGovernance_(t, cfg, "@my-feature"); err != nil {
		t.Errorf("apply-governance must work again after recovery: %v", err)
	}
	saveBuildStatePartial = false
	saveBuildStateEmitted = ""
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Errorf("ordinary saves must work again after recovery: %v", err)
	}
}

// The transaction marker probe is fail-closed. An unreadable marker is an
// unknown, and an unknown must never authorise the mutations the marker
// exists to block. A broken symlink is the sharp case: it reads as ENOENT
// through Stat while being plainly present in the directory.
func TestWP5_UnreadableTransactionMarkerIsNotAbsence(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	compactFixture(t, dir)

	path := compactJournalPath(cfg, "my-feature")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nothing-here.json"), path); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	present, err := compactionInFlight(cfg, "my-feature")
	if err != nil {
		t.Fatalf("a dangling marker should read as PRESENT via Lstat, not as an error: %v", err)
	}
	if !present {
		t.Error("a directory entry at the journal path is a barrier however it resolves — " +
			"\"there is something here I cannot follow\" is not \"there is nothing here\"")
	}

	// And it blocks, rather than being silently skipped.
	var found bool
	for _, iss := range computeCheckAmendments(cfg, "my-feature").Issues {
		if iss.Code == "amendment-compaction-incomplete" {
			found = true
		}
	}
	if !found {
		t.Error("an unreadable transaction marker must still block authority writers")
	}
}
