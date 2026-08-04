package commands

// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
// parlay-artifact: test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

// runGate writes a coverage-review.yaml and runs the real validator over
// it. Deliberately not a reimplementation of the validator's term loop:
// this bug was invisible for exactly as long as nothing exercised the two
// halves together, and a test that mirrors the logic it is checking would
// have passed against the broken code too.
func runGate(t *testing.T, review parser.CoverageReview, requiredTerms []string) []agent.ValidationOutcome {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage-review.yaml")

	review.Feature = "expenses"
	review.ReviewedAt = "2026-01-01T00:00:00Z"
	review.ReviewedBy = "cli"
	review.ReviewMethod = "cli"
	review.BuildfileHash = "bf"
	review.TestcasesHash = "tc"

	data, err := yaml.Marshal(&review)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	return agent.ValidateCoverageReview(agent.ModeBuild, agent.CoverageReviewInputs{
		ReviewPath:       path,
		Feature:          "expenses",
		BuildfileHashNow: "bf",
		TestcasesHashNow: "tc",
		RequiredCoverage: requiredTerms,
	})
}

func uncoveredTerms(outcomes []agent.ValidationOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Code == "coverage-review-uncovered" {
			n++
		}
	}
	return n
}

// The live bug: the CLI recorded an exemption keyed on the SUITE name
// while the gate keys on the covered TERM. A reviewer could answer the
// prompt, watch their reason get written to disk, and still have the gate
// report the term as uncovered — with no way to discharge it short of
// hand-editing coverage-review.yaml.
func TestCLIExemptionActuallySatisfiesTheGate(t *testing.T) {
	const term = "@expenses/operation:submit-report"

	exemptions := exemptionsForSuite("submit-report-suite", []string{term}, "covered by the engine's own suite")
	if got := exemptions[0].Item; got != term {
		t.Errorf("Item = %q, want the covered term %q", got, term)
	}

	outcomes := runGate(t, parser.CoverageReview{Exemptions: exemptions}, []string{term})
	if n := uncoveredTerms(outcomes); n != 0 {
		t.Errorf("coverage-review-uncovered still fires for an exempted term (%d outcome(s)): %+v", n, outcomes)
	}
}

// The guard for the regression itself: keying on the suite name must NOT
// satisfy the gate. If this ever passes, the bug is back and the test
// above has stopped meaning anything.
func TestSuiteNameKeyedExemptionDoesNotSatisfyTheGate(t *testing.T) {
	const term = "@expenses/operation:submit-report"
	broken := []parser.CoverageExemption{
		{Suite: "submit-report-suite", Item: "submit-report-suite", Reason: "the old shape"},
	}
	outcomes := runGate(t, parser.CoverageReview{Exemptions: broken}, []string{term})
	if uncoveredTerms(outcomes) == 0 {
		t.Error("a suite-name-keyed exemption must not discharge a term — if it does, the gate has become unkeyed")
	}
}

// A suite covering several terms produces an exemption for each. One entry
// for a suite covering three operations leaves two uncovered while the
// reviewer believes they answered.
func TestExemptionIsRecordedPerCoveredTerm(t *testing.T) {
	terms := []string{
		"@expenses/operation:submit-report",
		"@expenses/operation:approve-report",
	}
	exemptions := exemptionsForSuite("report-suite", terms, "external")
	if len(exemptions) != 2 {
		t.Fatalf("expected one exemption per term, got %d", len(exemptions))
	}
	outcomes := runGate(t, parser.CoverageReview{Exemptions: exemptions}, terms)
	if n := uncoveredTerms(outcomes); n != 0 {
		t.Errorf("every term the suite covered must be discharged, %d still uncovered", n)
	}
}

// Legacy v1 suites carry no source_refs, so there is no term to key on and
// the suite name is the only available fallback — no worse than before,
// and it keeps those testcases reviewable at all.
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
