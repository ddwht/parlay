// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
// parlay-artifact: test

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCoverageReview_Missing(t *testing.T) {
	dir := t.TempDir()
	in := CoverageReviewInputs{
		ReviewPath:       filepath.Join(dir, "coverage-review.yaml"),
		Feature:          "task-list",
		BuildfileHashNow: "sha256:bf",
		TestcasesHashNow: "sha256:tc",
	}
	outcomes := ValidateCoverageReview(ModeBuild, in)
	if !findCode(outcomes, "coverage-review-missing") {
		t.Errorf("missing coverage-review-missing; got %+v", outcomes)
	}
}

func TestValidateCoverageReview_Stale(t *testing.T) {
	dir := t.TempDir()
	reviewPath := filepath.Join(dir, "coverage-review.yaml")
	if err := os.WriteFile(reviewPath, []byte(`feature: task-list
buildfile_hash: sha256:stale-bf
testcases_hash: sha256:current-tc
approved_suites: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := CoverageReviewInputs{
		ReviewPath:       reviewPath,
		Feature:          "task-list",
		BuildfileHashNow: "sha256:current-bf",
		TestcasesHashNow: "sha256:current-tc",
	}
	outcomes := ValidateCoverageReview(ModeBuild, in)
	if !findCode(outcomes, "coverage-review-stale") {
		t.Errorf("missing coverage-review-stale; got %+v", outcomes)
	}
}

func TestValidateCoverageReview_SuiteUnapproved(t *testing.T) {
	dir := t.TempDir()
	reviewPath := filepath.Join(dir, "coverage-review.yaml")
	if err := os.WriteFile(reviewPath, []byte(`feature: task-list
buildfile_hash: sha256:bf
testcases_hash: sha256:tc
approved_suites: [a]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := CoverageReviewInputs{
		ReviewPath:       reviewPath,
		Feature:          "task-list",
		BuildfileHashNow: "sha256:bf",
		TestcasesHashNow: "sha256:tc",
		RequiredSuites:   []string{"a", "b"},
	}
	outcomes := ValidateCoverageReview(ModeBuild, in)
	if !findCode(outcomes, "coverage-review-suite-unapproved") {
		t.Errorf("missing coverage-review-suite-unapproved; got %+v", outcomes)
	}
}

func TestValidateCoverageReview_FreshAndApprovedPasses(t *testing.T) {
	dir := t.TempDir()
	reviewPath := filepath.Join(dir, "coverage-review.yaml")
	if err := os.WriteFile(reviewPath, []byte(`feature: task-list
buildfile_hash: sha256:bf
testcases_hash: sha256:tc
approved_suites: [a, b]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := CoverageReviewInputs{
		ReviewPath:       reviewPath,
		Feature:          "task-list",
		BuildfileHashNow: "sha256:bf",
		TestcasesHashNow: "sha256:tc",
		RequiredSuites:   []string{"a", "b"},
	}
	outcomes := ValidateCoverageReview(ModeBuild, in)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error: %+v", o)
		}
	}
}
