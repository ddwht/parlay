// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
// parlay-artifact: test

package parser

import "testing"

func TestParseCoverageReview_RoundTrip(t *testing.T) {
	content := []byte(`feature: task-list
reviewed_at: "2026-01-15T10:00:00Z"
reviewed_by: designer@example.com
review_method: cli
buildfile_hash: "sha256:abc"
testcases_hash: "sha256:def"
approved_suites:
  - task-create-form-presentation
  - task-create-operation
exemptions:
  - { suite: rare-error, item: server-error, reason: "covered manually" }
`)
	cr, err := ParseCoverageReviewBytes("test", content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cr.Feature != "task-list" {
		t.Errorf("feature: got %q", cr.Feature)
	}
	if got := len(cr.ApprovedSuites); got != 2 {
		t.Errorf("approved_suites: got %d, want 2", got)
	}
	if got := len(cr.Exemptions); got != 1 {
		t.Fatalf("exemptions: got %d, want 1", got)
	}
	if cr.Exemptions[0].Reason != "covered manually" {
		t.Errorf("exemption reason: got %q", cr.Exemptions[0].Reason)
	}
}
