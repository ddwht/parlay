// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
// parlay-artifact: test

package agent

import (
	"os"
	"path/filepath"
	"strings"
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

func TestValidateCoverageReview_PerSuiteStaleTargetsOnlyDriftedSuite(t *testing.T) {
	dir := t.TempDir()
	reviewPath := filepath.Join(dir, "coverage-review.yaml")
	if err := os.WriteFile(reviewPath, []byte(`feature: task-list
buildfile_hash: sha256:bf
testcases_hash: sha256:tc-old
suite_hashes:
  a: sha256:a1
  b: sha256:b1
approved_suites: [a, b]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Buildfile unchanged; suite b drifted, a did not. The whole-file
	// testcases_hash is stale too, but a review carrying suite_hashes must not
	// report whole-file staleness — that is the entire point of the field.
	in := CoverageReviewInputs{
		ReviewPath:       reviewPath,
		Feature:          "task-list",
		BuildfileHashNow: "sha256:bf",
		TestcasesHashNow: "sha256:tc-new",
		SuiteHashesNow:   map[string]string{"a": "sha256:a1", "b": "sha256:b2"},
		RequiredSuites:   []string{"a", "b"},
	}
	outcomes := ValidateCoverageReview(ModeBuild, in)
	if !findCode(outcomes, "coverage-review-suite-stale") {
		t.Errorf("expected coverage-review-suite-stale for the drifted suite; got %+v", outcomes)
	}
	if findCode(outcomes, "coverage-review-stale") {
		t.Errorf("a review with suite_hashes must not raise whole-file testcases staleness; got %+v", outcomes)
	}
	// The message must name b (drifted), not a (clean).
	var staleMsg string
	for _, o := range outcomes {
		if o.Code == "coverage-review-suite-stale" {
			staleMsg = o.Message
		}
	}
	if !strings.Contains(staleMsg, `"b"`) || strings.Contains(staleMsg, `"a"`) {
		t.Errorf("stale outcome should name only suite b; got %q", staleMsg)
	}
}

func TestValidateCoverageReview_PerSuiteFreshPasses(t *testing.T) {
	dir := t.TempDir()
	reviewPath := filepath.Join(dir, "coverage-review.yaml")
	if err := os.WriteFile(reviewPath, []byte(`feature: task-list
buildfile_hash: sha256:bf
testcases_hash: sha256:tc
suite_hashes:
  a: sha256:a1
  b: sha256:b1
approved_suites: [a, b]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := CoverageReviewInputs{
		ReviewPath:       reviewPath,
		Feature:          "task-list",
		BuildfileHashNow: "sha256:bf",
		TestcasesHashNow: "sha256:tc",
		SuiteHashesNow:   map[string]string{"a": "sha256:a1", "b": "sha256:b1"},
		RequiredSuites:   []string{"a", "b"},
	}
	for _, o := range ValidateCoverageReview(ModeBuild, in) {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error on a fresh, fully-approved review: %+v", o)
		}
	}
}

// A review predating per-suite hashing carries no suite_hashes, so testcases
// staleness must still be caught by the whole-file fallback.
func TestValidateCoverageReview_OldFormatFallsBackToWholeFile(t *testing.T) {
	dir := t.TempDir()
	reviewPath := filepath.Join(dir, "coverage-review.yaml")
	if err := os.WriteFile(reviewPath, []byte(`feature: task-list
buildfile_hash: sha256:bf
testcases_hash: sha256:tc-old
approved_suites: [a]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := CoverageReviewInputs{
		ReviewPath:       reviewPath,
		Feature:          "task-list",
		BuildfileHashNow: "sha256:bf",
		TestcasesHashNow: "sha256:tc-new",
		SuiteHashesNow:   map[string]string{"a": "sha256:a-new"},
		RequiredSuites:   []string{"a"},
	}
	if !findCode(ValidateCoverageReview(ModeBuild, in), "coverage-review-stale") {
		t.Errorf("old-format review with drifted testcases must fall back to whole-file coverage-review-stale")
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
