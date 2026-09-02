package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// writeTwoFeaturesAndAProjectThread builds the shape the cardinality rule is
// about: two features, and one review thread in a file that belongs to neither
// and gates both.
func writeTwoFeaturesAndAProjectThread(t *testing.T) *config.Context {
	t.Helper()
	setupTestDir(t)
	cfg := testContext(t)

	for _, slug := range []string{"alpha", "beta"} {
		dir := cfg.FeaturePath(slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Each feature carries one thread of its own, so that "the project
		// thread was not folded in" is an assertion about a non-empty list
		// rather than a vacuous one.
		if err := os.WriteFile(filepath.Join(dir, "intents.md"),
			[]byte("# "+slug+"\n\n## Do A Thing\n\n**Goal**: something\n<!-- @dwht: which something? -->\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(cfg.DomainModelPath(),
		[]byte("entities:\n  - name: Task\n  # @dwht: Task needs an owner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// One shared thread is ONE thread.
//
// Folding the project scans into every feature reported it once per feature,
// inflated the totals, and handed a resolver working without a feature
// argument the same request over and over. `features[]` is per-feature;
// `threads[]` beside it is what belongs to no feature and blocks all of them.
func TestProjectThreadIsReportedOnceAcrossFeatures(t *testing.T) {
	cfg := writeTwoFeaturesAndAProjectThread(t)

	// Through the COMMAND, not its helpers. The bug this guards lived in
	// runCollectAnnotations' loop, and a test that called the helper with the
	// right argument would have passed against it — which is the failure mode
	// of testing the piece you already know is correct.
	out := runCollectAnnotationsJSON(t, cfg)

	if len(out.Features) != 2 {
		t.Fatalf("features = %d, want 2", len(out.Features))
	}
	for _, f := range out.Features {
		if len(f.Threads) != 1 {
			t.Errorf("feature %s reports %d threads, want only its own: %+v", f.Feature, len(f.Threads), f.Threads)
			continue
		}
		if strings.HasSuffix(f.Threads[0].File, "domain-model.yaml") {
			t.Errorf("feature %s carried the shared project thread", f.Feature)
		}
	}
	if len(out.Threads) != 1 {
		t.Fatalf("top-level threads = %d, want exactly 1 — the shared thread once", len(out.Threads))
	}
	if !strings.HasSuffix(out.Threads[0].File, "domain-model.yaml") {
		t.Errorf("thread file = %q", out.Threads[0].File)
	}
	if out.Counts.Open != 3 {
		t.Errorf("open count = %d, want 3 — one per feature plus the shared one ONCE", out.Counts.Open)
	}
}

// runCollectAnnotationsJSON drives `parlay internal collect-annotations` with
// no feature argument and decodes what it printed.
func runCollectAnnotationsJSON(t *testing.T, cfg *config.Context) annotationsOutput {
	t.Helper()
	var buf strings.Builder
	cmd := collectAnnotationsCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(config.WithCtx(context.Background(), cfg))
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("collect-annotations: %v", err)
	}
	var out annotationsOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	return out
}

// A NAMED feature must see it, though. Its boundary is gated by that thread,
// so an answer that omitted it would send the reviewer to a command reporting
// nothing while the gate stayed shut — the dead end this pairing exists to
// prevent.
func TestNamedFeatureSeesTheProjectThreadThatGatesIt(t *testing.T) {
	cfg := writeTwoFeaturesAndAProjectThread(t)

	out, err := annotationsForFeature(cfg, "alpha", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Threads) != 2 || out.Counts.Open != 2 {
		t.Fatalf("named-feature scan = %d threads / %d open, want its own plus the project's", len(out.Threads), out.Counts.Open)
	}
	sawProject := false
	for _, th := range out.Threads {
		if strings.HasSuffix(th.File, "domain-model.yaml") {
			sawProject = true
		}
	}
	if !sawProject {
		t.Error("the named feature's answer omits the project thread that gates it")
	}

	// And the gate agrees with the listing — that agreement is the fix.
	issues := annotationReadinessIssues(cfg, "alpha")
	var blocked bool
	for _, issue := range issues {
		if issue.Code == openAnnotationsCode && issue.Severity == "error" {
			blocked = true
		}
	}
	if !blocked {
		t.Errorf("the gate did not block on the project thread: %+v", issues)
	}

	// The listing and the sweep reach it from the feature too.
	scans, err := annotationScansFor(cfg, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, scan := range scans {
		if strings.HasSuffix(scan.Path, "domain-model.yaml") && len(scan.Threads) == 1 {
			found = true
			if scan.Feature != "" {
				t.Errorf("a project file was attributed to feature %q", scan.Feature)
			}
		}
	}
	if !found {
		t.Error("annotations list @alpha does not reach the project thread that blocks alpha")
	}
}
