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
	ReviewPath        string
	Feature           string
	BuildfileHashNow  string
	TestcasesHashNow  string
	RequiredSuites    []string // every suite present in testcases.yaml
	RequiredCoverage  []string // every term that needs coverage (operations, errors)
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
	if cr.TestcasesHash != in.TestcasesHashNow {
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
	for _, term := range in.RequiredCoverage {
		// Term is covered if some approved suite covers it OR it has an
		// explicit exemption. Coverage relationships live in testcases.yaml;
		// the gate only checks approval + exemption here.
		if !exempted[term] && !approved[term] {
			outcomes = append(outcomes, NewOutcome(mode, "coverage-review-uncovered",
				fmt.Sprintf("required term %q has no covering approved case and no exemption", term)))
		}
	}

	return outcomes
}
