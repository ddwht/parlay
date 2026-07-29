package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanAllSurfaces_FindsYAMLAndNestedFeatures guards two defects that
// together made page assembly return nothing on a spec-conformant project:
// the scanner hardcoded "surface.md" (so surface.yaml, the target format
// create-artifacts emits, was invisible), and it only looked one level deep
// (so initiative-nested features were skipped entirely). The user-visible
// symptom was "No fragments target page X" — which reads as "you have not
// authored them yet", not "this format is unsupported".
func TestScanAllSurfaces_FindsYAMLAndNestedFeatures(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Top-level feature in the target YAML format.
	mk("intents/expense-list/surface.yaml", `feature: expense-list

fragments:
  - name: My reports grid
    shows: data-table
    source: "@expense-list/browse"
    page: expenses
    region: main
    order: 1
`)
	// Top-level feature still in the legacy markdown format.
	mk("intents/legacy-feature/surface.md", `# Legacy — Surface

## A legacy fragment

**Shows**: data-value
**Page**: expenses
**Region**: main
**Order**: 2
**Source**: @legacy-feature/x
`)
	// Initiative-nested feature.
	mk("intents/approvals/review-queue/surface.yaml", `feature: review-queue

fragments:
  - name: Submitted queue
    shows: data-table
    source: "@approvals/review-queue/review"
    page: review
    region: main
    order: 1
`)

	got, err := ScanAllSurfaces(root)
	if err != nil {
		t.Fatalf("ScanAllSurfaces: %v", err)
	}

	byFeature := map[string]int{}
	for _, f := range got {
		byFeature[f.Feature]++
	}
	for _, want := range []string{"expense-list", "legacy-feature", "approvals/review-queue"} {
		if byFeature[want] == 0 {
			t.Errorf("no fragments found for %q — got %v", want, byFeature)
		}
	}
	if n := len(got); n != 3 {
		t.Errorf("got %d fragments, want 3 (one per feature): %v", n, byFeature)
	}
}
