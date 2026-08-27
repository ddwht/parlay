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

// The hazard the whole record exists for. Removing the blanket gate without
// this turns every recorded exemption into a permanent unconditional waiver,
// aimed precisely at the criteria a person once said needed no test.
func TestCoverageExceptions_StaleLedgerBlocksAndExcusesNothing(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived, Reason: "r",
	})

	moved := []AuthorizedCriterion{
		cs[0],
		crit(cs[1].Ref, "the archive button is HIDDEN while invoices are unpaid"),
	}
	v := EvaluateCoverageExceptions(t.TempDir(), rec, moved)

	if len(v.Blockers) == 0 {
		t.Fatal("a judgment about a different contract must not silently apply to this one")
	}
	// Dropping quietly would turn the waiver back into an uncovered criterion,
	// which under warning severities may still proceed — making freshness
	// advisory, which is the opposite of the point.
	if v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("a stale ledger excuses nothing")
	}
	if !strings.Contains(v.Blockers[0], "Re-review") {
		t.Errorf("the refusal should say what to do: %q", v.Blockers[0])
	}
}

func TestCoverageExceptions_EntryWideIsAcceptedAndWarned(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Kind: ExceptionWaived, Reason: "legacy, predates bullet identity",
	})

	v := EvaluateCoverageExceptions(t.TempDir(), rec, cs)
	if len(v.Blockers) != 0 {
		t.Fatalf("every exemption written before bullet identity is this shape: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("entry-wide should excuse the entry")
	}
	if len(v.Warnings) == 0 || !strings.Contains(v.Warnings[0], "added later") {
		t.Errorf("it excuses bullets nobody considered, and should say so: %+v", v.Warnings)
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
