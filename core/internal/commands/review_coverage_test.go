package commands

// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
// parlay-artifact: test

import (
	"strings"
	"testing"
)

// runGate writes a coverage-review.yaml and runs the real validator over
// it. Deliberately not a reimplementation of the validator's term loop:
// this bug was invisible for exactly as long as nothing exercised the two
// halves together, and a test that mirrors the logic it is checking would
// have passed against the broken code too.
// The coverage-review-uncovered walk these tests exercised is gone: its input
// field had no production writer, so the code — a default ERROR — could never
// fire in a real run, and these tests proved a leaf function correct while
// nothing reached it. They were the demonstration of that failure, not a guard
// against it.
//
// The one real property among them, that a suite-name-keyed exemption must not
// discharge a term, now holds by construction in the live path: validate.go
// keys exemptions on the criterion ref, so an item naming a suite creates an
// entry that no criterion ref can match. It is enforced where exemptions are
// actually consumed rather than in a walk nothing called.

func TestLegacySuiteWithoutSourceRefsFallsBackToSuiteName(t *testing.T) {
	exemptions := exemptionsForSuite("legacy-suite", nil, "why")
	if len(exemptions) != 1 || exemptions[0].Item != "legacy-suite" {
		t.Errorf("want a single suite-name-keyed fallback, got %+v", exemptions)
	}
}

// --exempt's reason is free text and routinely contains both separators.
// Splitting on the last one instead of the first silently truncates it,
// which turns a reviewable justification into a fragment.
func TestParseExemptFlags(t *testing.T) {
	got, err := parseExemptFlags([]string{
		"report-suite:@expenses/operation:submit=covered by the engine: see ADR-4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d exemptions, want 1", len(got))
	}
	if got[0].Suite != "report-suite" {
		t.Errorf("Suite = %q", got[0].Suite)
	}
	// The item keeps its own colon — only the FIRST colon separates.
	if got[0].Item != "@expenses/operation:submit" {
		t.Errorf("Item = %q, want the full operation ref", got[0].Item)
	}
	if got[0].Reason != "covered by the engine: see ADR-4" {
		t.Errorf("Reason = %q — the reason must survive its own punctuation", got[0].Reason)
	}
}

func TestParseExemptFlags_Rejects(t *testing.T) {
	for _, bad := range []string{
		"report-suite:@op",  // no reason
		"report-suite=why",  // no item
		"report-suite:@op=", // empty reason
		":@op=why",          // empty suite
		"report-suite:=why", // empty item
	} {
		if _, err := parseExemptFlags([]string{bad}); err == nil {
			t.Errorf("parseExemptFlags(%q) should have failed", bad)
		}
	}
}

// A suite is skipped only when EVERY term it covers is exempted. Skipping
// on a partial match would leave the remaining terms silently unapproved.
func TestSuiteFullyExemptedRequiresEveryTerm(t *testing.T) {
	refs := []string{"@f/operation:a", "@f/operation:b"}
	partial := map[string]map[string]bool{"s": {"@f/operation:a": true}}
	if suiteFullyExempted("s", refs, partial) {
		t.Error("a partially exempted suite must still be prompted for")
	}
	full := map[string]map[string]bool{"s": {"@f/operation:a": true, "@f/operation:b": true}}
	if !suiteFullyExempted("s", refs, full) {
		t.Error("a fully exempted suite must not be prompted for")
	}
	if suiteFullyExempted("other", refs, full) {
		t.Error("exemptions must not leak across suites")
	}
}

// The forged-approval bug, pinned.
//
// EOF used to be indistinguishable from a person pressing Enter: the read
// error was discarded, the empty string matched the [Y/n] default, and an
// agent-driven run approved every suite and wrote a coverage-review.yaml
// stamped with a real username. The gate it guards then passed.
func TestEOFIsNotApproval(t *testing.T) {
	if err := errNoReviewerPresent("expenses", "submit-suite"); err == nil {
		t.Fatal("stdin ending must be an error, never an approval")
	} else {
		msg := err.Error()
		for _, want := range []string{
			"coverage-review-no-reviewer", // greppable code
			"submit-suite",                // which suite it stopped on
			"--exempt",                    // the unattended alternative
			"raise a decision",            // what a phase module should do
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("refusal message omits %q — a reader has to know what to do instead:\n%s", want, msg)
			}
		}
	}
}

// A declined suite with no reason recorded is unreviewable later, so it is
// refused rather than written with an empty justification.
func TestDeclineWithoutAReasonIsRefused(t *testing.T) {
	// exemptionsForSuite itself does not validate — the guard is at the
	// prompt — so this pins the shape the caller must preserve: an
	// exemption always carries a non-empty reason by the time it is built.
	ex := exemptionsForSuite("s", []string{"@f/operation:a"}, "a real reason")
	if len(ex) != 1 || strings.TrimSpace(ex[0].Reason) == "" {
		t.Errorf("an exemption must carry its justification: %+v", ex)
	}
}
