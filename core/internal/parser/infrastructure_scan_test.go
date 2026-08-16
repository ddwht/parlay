package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanAllInfrastructure_FindsTopLevelAndNestedFeatures mirrors the surface
// scanner's guarantee: every feature's infrastructure.md is found, including
// features nested one level under an initiative directory, with Feature stamped
// in the qualified form. A feature with no infrastructure.md contributes
// nothing rather than erroring.
func TestScanAllInfrastructure_FindsTopLevelAndNestedFeatures(t *testing.T) {
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

	mk("intents/caching/infrastructure.md", `## Response cache

**Affects**: the response cache
**Behavior**: caches responses for the process lifetime
**Source**: @caching/x
`)
	// A feature directory with no infrastructure.md — contributes nothing.
	mk("intents/surface-only/surface.yaml", "feature: surface-only\nfragments: []\n")
	// Initiative-nested feature.
	mk("intents/approvals/review-queue/infrastructure.md", `## Queue boundary

**Affects**: the approvals boundary
**Behavior**: only the approvals package may enqueue
**Source**: @approvals/review-queue/y
`)

	frags, err := ScanAllInfrastructure(filepath.Join(root))
	if err != nil {
		t.Fatalf("ScanAllInfrastructure: %v", err)
	}
	byFeature := map[string]string{}
	for _, f := range frags {
		byFeature[f.Feature] = f.Affects
	}
	if len(frags) != 2 {
		t.Fatalf("expected 2 fragments, got %d: %+v", len(frags), frags)
	}
	if byFeature["caching"] != "the response cache" {
		t.Errorf("top-level feature fragment missing or Feature unstamped: %+v", byFeature)
	}
	if byFeature["approvals/review-queue"] != "the approvals boundary" {
		t.Errorf("initiative-nested feature should carry the qualified Feature slug: %+v", byFeature)
	}
}
