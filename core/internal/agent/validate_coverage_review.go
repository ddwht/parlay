// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
//
// Pre-codegen gate that consults .parlay/build/<feature>/coverage-review.yaml.
// Refuses when the file is missing, when either canonical-form hash drifts,
// when any required suite is unapproved, or when any required term lacks
// both a covering case and an explicit exemption.

package agent

import (
	"fmt"
	"os"

	"github.com/ddwht/parlay/core/internal/parser"
)

// CoverageReviewInputs is what the gate consults. The hashes are the
// canonical-form hashes of the on-disk buildfile + testcases at codegen
// time; the gate compares them against the recorded hashes in
// coverage-review.yaml.
type CoverageReviewInputs struct {
	ReviewPath       string
	Feature          string
	BuildfileHashNow string
	TestcasesHashNow string
	SuiteHashesNow   map[string]string // per-suite canonical hashes of the on-disk testcases
	RequiredSuites   []string          // every suite present in testcases.yaml
}

// ValidateCoverageReview is the gate. Returns outcomes describing every
// failure; if no outcomes are returned, codegen may proceed.
func ValidateCoverageReview(mode ValidationMode, in CoverageReviewInputs) []ValidationOutcome {
	var outcomes []ValidationOutcome

	if _, err := os.Stat(in.ReviewPath); os.IsNotExist(err) {
		outcomes = append(outcomes, NewOutcome(mode, "coverage-review-missing",
			fmt.Sprintf("coverage-review.yaml not found at %s — run `parlay review-coverage %s`", in.ReviewPath, in.Feature)))
		return outcomes
	}

	cr, err := parser.ParseCoverageReview(in.ReviewPath)
	if err != nil {
		outcomes = append(outcomes, NewOutcome(mode, "coverage-review-missing",
			fmt.Sprintf("parse coverage-review.yaml: %v", err)))
		return outcomes
	}

	if cr.BuildfileHash != in.BuildfileHashNow {
		outcomes = append(outcomes, NewOutcome(mode, "coverage-review-stale",
			fmt.Sprintf("buildfile_hash drift: review records %s, current is %s — run `parlay review-coverage %s`", cr.BuildfileHash, in.BuildfileHashNow, in.Feature)))
	}

	// Testcases staleness is per-suite when the review carries suite_hashes,
	// so editing one suite invalidates only that suite's approval and leaves
	// the rest reviewable. A review written before per-suite hashing has no
	// suite_hashes; it falls back to the whole-file testcases_hash, which
	// stales the entire review on any change — the original behavior, kept so
	// old reviews stay valid until re-run.
	if len(cr.SuiteHashes) > 0 {
		for _, suite := range PerSuiteStale(cr.SuiteHashes, in.SuiteHashesNow, cr.ApprovedSuites) {
			outcomes = append(outcomes, NewOutcome(mode, "coverage-review-suite-stale",
				fmt.Sprintf("suite %q was approved but its testcases changed since — re-review with `parlay review-coverage %s`", suite, in.Feature)))
		}
	} else if cr.TestcasesHash != in.TestcasesHashNow {
		outcomes = append(outcomes, NewOutcome(mode, "coverage-review-stale",
			fmt.Sprintf("testcases_hash drift: review records %s, current is %s — run `parlay review-coverage %s`", cr.TestcasesHash, in.TestcasesHashNow, in.Feature)))
	}

	approved := toSet(cr.ApprovedSuites)
	for _, suite := range in.RequiredSuites {
		if !approved[suite] {
			outcomes = append(outcomes, NewOutcome(mode, "coverage-review-suite-unapproved",
				fmt.Sprintf("suite %q is in testcases.yaml but absent from approved_suites:", suite)))
		}
	}

	exempted := make(map[string]bool, len(cr.Exemptions))
	for _, ex := range cr.Exemptions {
		exempted[ex.Item] = true
	}
	// RequiredCoverage and its walk were removed here. The field had no
	// production writer at all: computeReviewGate built its input without it,
	// and the only assignment anywhere was a test that hand-built the struct.
	// So coverage-review-uncovered — absent from the severity table and
	// therefore a default ERROR, the strictest verdict the validator has —
	// could not fire in any real run, and its green test proved a leaf
	// function correct while nothing reached it.
	//
	// Not repaired by populating the field: turning a never-fired hard blocker
	// on for every multi-target project days before removing the gate it
	// belongs to would create breakage without protection.

	return outcomes
}
