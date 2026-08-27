// parlay-feature: parlay-tool/criterion-authority
// parlay-component: coverage-exception-ledger
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
)

func exceptionsFor(cs []AuthorizedCriterion, exs ...CoverageException) *CoverageExceptions {
	return &CoverageExceptions{
		Feature: "f", GrantedAt: "2026-08-27T00:00:00Z",
		CriteriaHash: CriteriaHash(cs), Exceptions: exs,
	}
}

func TestCoverageExceptions_FreshLedgerExcusesTheBulletItNames(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived,
		Reason: "enforced by a database constraint, not by this operation",
	})

	v := EvaluateCoverageExceptions(t.TempDir(), rec, cs)
	if len(v.Blockers) != 0 {
		t.Fatalf("a fresh ledger should apply: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("the named bullet should be excused")
	}
	if v.Exempt.Excuses(agent.CriterionRef{Ref: cs[1].Ref, Text: cs[1].Text}) {
		t.Error("a bullet-specific exception must not excuse a different one")
	}
}

// The hazard the whole record exists for: removing the blanket gate without a
// binding turns every exemption into a permanent unconditional waiver.
//
// The binding is per-exception, not whole-feature. A bullet-specific exception
// is valid exactly while the bullet it names is still declared — so REWORDING
// that bullet invalidates it, and rewording a different one does not.
func TestCoverageExceptions_ExceptionDiesWithTheBulletItNames(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived, Reason: "r",
	})

	reworded := []AuthorizedCriterion{
		crit(cs[0].Ref, "archiving a customer with unpaid invoices returns a warning"),
		cs[1],
	}
	v := EvaluateCoverageExceptions(t.TempDir(), rec, reworded)
	if len(v.Blockers) == 0 {
		t.Fatal("the bullet this judgment was about no longer exists; the judgment has not been made about the new one")
	}
	if v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("a stale exception excuses nothing")
	}
}

// The improvement over binding to the whole standard: an unrelated change
// should not force re-review of a judgment it did not touch. Criteria authority
// approves the entire standard; an exception is a localized claim.
func TestCoverageExceptions_AnUnrelatedCriterionChangeDoesNotInvalidateIt(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived, Reason: "r",
	})

	elsewhere := []AuthorizedCriterion{
		cs[0],
		crit(cs[1].Ref, "the archive button is HIDDEN while invoices are unpaid"),
	}
	v := EvaluateCoverageExceptions(t.TempDir(), rec, elsewhere)
	if len(v.Blockers) != 0 {
		t.Fatalf("rewording a presentation bullet says nothing about a waived operation bullet: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("the untouched judgment should still hold")
	}
}

func TestCoverageExceptions_EntryWideIsAcceptedAndWarned(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Kind: ExceptionWaived, Reason: "legacy, predates bullet identity",
		EntryHash: entryBulletsHash([]AuthorizedCriterion{cs[0]}),
	})

	v := EvaluateCoverageExceptions(t.TempDir(), rec, cs)
	if len(v.Blockers) != 0 {
		t.Fatalf("every exemption written before bullet identity is this shape: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("entry-wide should excuse the entry")
	}
	if len(v.Warnings) == 0 || !strings.Contains(v.Warnings[0], "every bullet") {
		t.Errorf("it excuses more than one bullet, and should say so: %+v", v.Warnings)
	}

	// entry_hash is what keeps it from also excusing bullets added later, which
	// nobody judged.
	grown := append(twoCriteria(), crit(cs[0].Ref, "a second claim on the same operation"))
	if v := EvaluateCoverageExceptions(t.TempDir(), rec, grown); len(v.Blockers) == 0 {
		t.Error("adding a bullet to an entry-wide exception's entry must invalidate it")
	}
}

// A hand-authored exception claims a test parlay cannot inspect covers the
// criterion. Unnamed, that is not a claim.
func TestCoverageExceptions_HandAuthoredMustNameItsTest(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionHandAuthored, Reason: "covered by an integration suite",
	})
	if v := EvaluateCoverageExceptions(t.TempDir(), rec, cs); len(v.Blockers) == 0 {
		t.Error("an uninspectable test that is also unnamed excuses nothing")
	}
}

func TestCoverageExceptions_ExcusingSomethingTheContractDroppedBlocks(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: "@f/operation:gone", Text: "x", Kind: ExceptionWaived, Reason: "r",
	})
	// Hash matches; the ref does not exist.
	rec.CriteriaHash = CriteriaHash(cs)
	if v := EvaluateCoverageExceptions(t.TempDir(), rec, cs); len(v.Blockers) == 0 {
		t.Error("excusing an entry that declares no criteria is a stale judgment wearing a fresh hash")
	}
}

func TestCoverageExceptions_NoLedgerIsNotAProblem(t *testing.T) {
	v := EvaluateCoverageExceptions(t.TempDir(), nil, twoCriteria())
	if len(v.Blockers) != 0 || len(v.Warnings) != 0 {
		t.Errorf("most features excuse nothing; that is the ordinary case: %+v", v)
	}
}

// A hand-authored exception is a person accepting THAT test as covering the
// criterion. Leaving the hash declared but unchecked would have made the field
// look like a guarantee while the body drifted underneath it.
func TestCoverageExceptions_HandAuthoredIsBoundToTheTestBody(t *testing.T) {
	dir := t.TempDir()
	cs := twoCriteria()

	testPath := filepath.Join(dir, "archive_test.go")
	if err := os.WriteFile(testPath, []byte("func TestArchive(t *testing.T) { /* the original */ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := TestBodyHash(testPath)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(h string) *CoverageExceptions {
		return exceptionsFor(cs, CoverageException{
			Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionHandAuthored,
			Reason: "covered by an integration suite", TestFile: "archive_test.go", TestHash: h,
		})
	}

	if v := EvaluateCoverageExceptions(dir, mk(hash), cs); len(v.Blockers) != 0 {
		t.Fatalf("the test is unchanged; the exception holds: %+v", v.Blockers)
	}

	// Rewritten under the approval.
	if err := os.WriteFile(testPath, []byte("func TestArchive(t *testing.T) { /* rewritten */ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := EvaluateCoverageExceptions(dir, mk(hash), cs)
	if len(v.Blockers) == 0 {
		t.Fatal("a rewritten test is a different claim wearing the old approval")
	}
	if !strings.Contains(v.Blockers[0], "different version") {
		t.Errorf("say what actually changed: %q", v.Blockers[0])
	}

	// Deleted entirely.
	os.Remove(testPath)
	if v := EvaluateCoverageExceptions(dir, mk(hash), cs); len(v.Blockers) == 0 || !strings.Contains(v.Blockers[0], "gone") {
		t.Errorf("a missing test cannot be covering anything: %+v", v.Blockers)
	}

	// Named but never fingerprinted.
	if v := EvaluateCoverageExceptions(dir, mk(""), cs); len(v.Blockers) == 0 {
		t.Error("without a hash the exception is evergreen over a body nobody is watching")
	}
}

// --- production path -----------------------------------------------------
//
// The evaluation above existed and was tested at the leaf while validate.go
// copied only the excused set and discarded every blocker and every error. So
// a stale ledger excused nothing and SAID nothing — the drop-and-proceed
// behaviour the freshness rule exists to prevent, under a comment claiming the
// opposite. These run through the real gate.

func writeExceptions(t *testing.T, cfg *config.Context, slug string, rec *CoverageExceptions) {
	t.Helper()
	if err := saveCoverageExceptions(cfg, slug, rec); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageExceptions_GateReportsAStaleLedger(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	writeExceptions(t, cfg, "graded", &CoverageExceptions{
		Feature: "graded", GrantedAt: "2026-08-27T00:00:00Z",
		Exceptions: []CoverageException{{
			Ref: current[0].Ref, Text: "a claim this contract never made",
			Kind: ExceptionWaived, Reason: "r",
		}},
	})

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "coverage-exception-invalid") {
		t.Fatalf("a ledger excusing a criterion nobody declared must reach the boundary, not vanish: %+v", out.Blockers)
	}
}

func TestCoverageExceptions_GateReportsAnUnreadableLedger(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	path := coverageExceptionsPath(cfg, "graded")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("exceptions: [\n  broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := computeGate(cfg, "graded", gateStageCode)
	if !gateHasCode(out.Blockers, "coverage-exception-invalid") {
		t.Errorf("an unreadable ledger is not a feature with nothing excused: %+v", out.Blockers)
	}
}

func TestCoverageExceptions_GateStaysQuietWithNoLedger(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)

	out, _ := computeGate(testContext(t), "graded", gateStageCode)
	if gateHasCode(out.Blockers, "coverage-exception-invalid") {
		t.Errorf("most features excuse nothing; that is the ordinary case: %+v", out.Blockers)
	}
}

// state-only is not an exemption anywhere else in the schema: it means a real
// case cites the criterion and observes it by state. Letting it excuse a
// criterion with NO case was the opposite claim.
func TestCoverageExceptions_GateRefusesStateOnlyAsAnExemption(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, _ := CurrentCriteria(cfg, "graded")

	writeExceptions(t, cfg, "graded", &CoverageExceptions{
		Feature: "graded", GrantedAt: "2026-08-27T00:00:00Z",
		Exceptions: []CoverageException{{
			Ref: current[0].Ref, Text: current[0].Text,
			Kind: ExceptionStateOnly, Reason: "observed by state",
		}},
	})

	out, _ := computeGate(cfg, "graded", gateStageCode)
	var msg string
	for _, b := range out.Blockers {
		if b.Code == "coverage-exception-invalid" {
			msg = b.Message
		}
	}
	if msg == "" || !strings.Contains(msg, "not an exemption") {
		t.Errorf("state-only must not excuse a criterion with no case at all: %+v", out.Blockers)
	}
}
