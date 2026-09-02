package commands

import (
	"os"
	"strings"
	"testing"
)

// Drift honesty, on both axes.
//
// A comment IS bytes, so whether it shows up as drift depends entirely on how
// the source is hashed — and the two answers are opposite:
//
//   - A founding document is hashed over PARSED content, and no parser reads
//     comments. Annotating intents.md moves nothing: no drift, no integrity
//     finding. That is the property WP0 bought.
//   - The root domain model is hashed over BYTES. Annotating it changes the
//     hash, and the existing shared-source comparison reports it — correctly.
//
// So "adding a comment never changes has_drift" is FALSE, and an earlier
// version of this test asserted it. The real invariant is narrower: the
// review-thread summary EXPLAINS the verdict and never contributes to it.
func TestReviewThreadSummaryExplainsDriftWithoutCausingIt(t *testing.T) {
	t.Run("a founding comment is summarised and changes no verdict", func(t *testing.T) {
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)

		before, err := detectDrift(cfg, "graded", cfg.FeaturePath("graded"))
		if err != nil {
			t.Fatal(err)
		}
		if len(before.ReviewThreads) != 0 {
			t.Fatalf("the control reports threads already: %v", before.ReviewThreads)
		}

		annotateGradedIntents(t, cfg, "<!-- @dwht: this promise is too narrow -->\n")

		after, err := detectDrift(cfg, "graded", cfg.FeaturePath("graded"))
		if err != nil {
			t.Fatal(err)
		}
		if len(after.ReviewThreads) != 1 {
			t.Fatalf("review_threads = %v, want one line naming intents.md", after.ReviewThreads)
		}
		line := after.ReviewThreads[0]
		if !strings.Contains(line, "intents.md") || !strings.Contains(line, "1 open") {
			t.Errorf("line does not name the file and its count: %q", line)
		}
		// NEUTRAL wording: nothing here reports intents.md as changed, so a
		// line claiming changed bytes would contradict the verdict beside it.
		if strings.Contains(line, "reported changed above") {
			t.Errorf("the line claims changed bytes beside a verdict that reports none: %q", line)
		}

		if after.HasDrift != before.HasDrift {
			t.Errorf("a comment in a frozen founding document changed has_drift %v → %v", before.HasDrift, after.HasDrift)
		}
		if len(after.LedgerIntegrity) != 0 {
			t.Errorf("it read as an integrity violation: %v", after.LedgerIntegrity)
		}
	})

	t.Run("a byte-hashed source drifts, and the summary says why", func(t *testing.T) {
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)

		// Give the project a root domain model and re-record the baseline, so
		// its byte hash is one the comparison below actually has to compare
		// against. A skipped assertion proves nothing.
		path := cfg.DomainModelPath()
		const model = "entities:\n  - name: Task\n    fields:\n      - name: text\n"
		if err := os.WriteFile(path, []byte(model), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := saveBuildStateForFeature(cfg, "graded", cfg.Root.Path); err != nil {
			t.Fatal(err)
		}
		recorded, err := detectDrift(cfg, "graded", cfg.FeaturePath("graded"))
		if err != nil {
			t.Fatal(err)
		}
		if sharedChanged(recorded, "domain-model") {
			t.Fatalf("the re-recorded baseline still reports the domain model changed: %v", recorded.SharedSourcesChanged)
		}

		// The root domain model is hashed over BYTES, so a comment does move
		// it — through the comparison that already existed, not through
		// anything this field does.
		if err := os.WriteFile(path, []byte(model+"# @dwht: Task needs an owner\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		after, err := detectDrift(cfg, "graded", cfg.FeaturePath("graded"))
		if err != nil {
			t.Fatal(err)
		}
		if !sharedChanged(after, "domain-model") {
			t.Fatalf("the existing shared-source comparison did not notice the comment bytes: %v", after.SharedSourcesChanged)
		}
		if !after.HasDrift {
			t.Error("has_drift stayed false while a shared source is reported changed")
		}

		explained := lineFor(after.ReviewThreads, "domain-model.yaml")
		if explained == "" {
			t.Fatalf("the summary does not mention the file it just explained: %v", after.ReviewThreads)
		}
		if !strings.Contains(explained, "reported changed above") {
			t.Errorf("the file IS reported changed, so the line should say so: %q", explained)
		}
		assertHonestAttribution(t, explained)
	})

	// The same file, edited AND annotated in one pass — the ordinary case for
	// somebody working through a review. The wording must stay honest: all the
	// code knows is that the hash moved and that the file has a thread, never
	// that the comment is the whole of the change.
	t.Run("an edit and a comment in the same pass are not attributed to the comment", func(t *testing.T) {
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)

		path := cfg.DomainModelPath()
		const model = "entities:\n  - name: Task\n    fields:\n      - name: text\n"
		if err := os.WriteFile(path, []byte(model), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := saveBuildStateForFeature(cfg, "graded", cfg.Root.Path); err != nil {
			t.Fatal(err)
		}

		// A real content change AND a comment.
		const edited = "entities:\n  - name: Task\n    fields:\n      - name: text\n      - name: owner\n# @dwht: is owner required?\n"
		if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		after, err := detectDrift(cfg, "graded", cfg.FeaturePath("graded"))
		if err != nil {
			t.Fatal(err)
		}
		if !sharedChanged(after, "domain-model") {
			t.Fatalf("the edit was not reported: %v", after.SharedSourcesChanged)
		}
		explained := lineFor(after.ReviewThreads, "domain-model.yaml")
		if explained == "" {
			t.Fatalf("the summary omits the file it just explained: %v", after.ReviewThreads)
		}
		assertHonestAttribution(t, explained)
	})
}

// lineFor returns the summary line naming a file, or "" — so a caller can fail
// on its absence rather than looping over nothing and asserting nothing.
func lineFor(lines []string, file string) string {
	for _, line := range lines {
		if strings.Contains(line, file) {
			return line
		}
	}
	return ""
}

// assertHonestAttribution pins the one thing the summary must never say: that
// the comment IS the change. It can only know the hash moved and the file has
// a thread.
func assertHonestAttribution(t *testing.T, line string) {
	t.Helper()
	for _, overclaim := range []string{"the comment bytes are the change", "are the change"} {
		if strings.Contains(line, overclaim) {
			t.Errorf("the line attributes the whole change to the comment: %q", line)
		}
	}
	// BOTH halves, not either. The subject and the qualification each do work:
	// "include review comments" says what contributed, "not only edits" says it
	// was not the whole of it. A line carrying one without the other is a
	// different claim than the one this wording was chosen to make.
	for _, required := range []string{"include review comments", "not only edits"} {
		if !strings.Contains(line, required) {
			t.Errorf("the line is missing %q, so it no longer says the comments are PART of the change: %q", required, line)
		}
	}
}

func sharedChanged(out *driftOutput, name string) bool {
	for _, s := range out.SharedSourcesChanged {
		if s == name {
			return true
		}
	}
	return false
}

// The annotation codes come before the drift findings, so a reviewer meets
// their own comment as the cause before they meet stale-buildfile.
func TestReadinessOrdersAnnotationsBeforeDrift(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	annotateGradedIntents(t, cfg, "<!-- @dwht: this promise is too narrow -->\n")

	issues := buildFeatureStageIssues(cfg, cfg.FeaturePath("graded"), "graded")
	firstAnnotation, firstDrift := -1, -1
	for i, issue := range issues {
		if strings.HasSuffix(issue.Code, "-annotations") && firstAnnotation < 0 {
			firstAnnotation = i
		}
		if (issue.Code == "stale-buildfile" || strings.Contains(issue.Code, "drift")) && firstDrift < 0 {
			firstDrift = i
		}
	}
	if firstAnnotation < 0 {
		t.Fatalf("no annotation issue reported: %+v", issues)
	}
	if firstDrift >= 0 && firstAnnotation > firstDrift {
		t.Errorf("drift finding at %d comes before the annotation at %d — the reviewer meets the symptom before the cause",
			firstDrift, firstAnnotation)
	}
}
