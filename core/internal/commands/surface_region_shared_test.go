// parlay-feature: parlay-tool/collision-detection-tier
// parlay-component: surface-region-shared
// parlay-artifact: test
//
// Tests the surface-region-shared warning emitted by validatePageReferences:
// two or more different features contributing surface fragments to the same
// (page, region) is a named warning, with a sharper message on an exact-order
// collision, and a single feature stacking its own fragments is not.

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

func writeFeatureSurface(t *testing.T, root, feature, body string) {
	t.Helper()
	dir := filepath.Join(root, config.SpecDir, "intents", feature)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "surface.yaml"), []byte(body), 0644); err != nil {
		t.Fatalf("write surface.yaml: %v", err)
	}
}

func pageRefsForTest(t *testing.T, root, pageName string) []string {
	t.Helper()
	pagePath := filepath.Join(root, config.SpecDir, "pages", pageName+".page.md")
	if err := os.MkdirAll(filepath.Dir(pagePath), 0755); err != nil {
		t.Fatalf("mkdir pages: %v", err)
	}
	// A minimal manifest: the collision detection keys on the surface
	// fragments' own page:/region:, not on the manifest's regions.
	if err := os.WriteFile(pagePath, []byte("---\nname: "+pageName+"\n---\n"), 0644); err != nil {
		t.Fatalf("write page: %v", err)
	}
	page, err := parser.ParsePageFile(pagePath)
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Path: root, Kind: config.RootKindStandalone},
	}, nil)
	cmd := testCommandWithContext(t, cfg)
	errs := validatePageReferences(cmd, pagePath, page)
	var codes []string
	for _, e := range errs {
		if e.Code == "surface-region-shared" {
			codes = append(codes, e.Message)
		}
	}
	return codes
}

func TestSurfaceRegionShared_TwoFeaturesSameRegionWarns(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Beta Panel\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 2\n")

	msgs := pageRefsForTest(t, root, "dashboard")
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one surface-region-shared warning, got %d: %v", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0], "alpha") || !strings.Contains(msgs[0], "beta") {
		t.Errorf("warning should name both features; got %q", msgs[0])
	}
}

func TestSurfaceRegionShared_ExactOrderCollisionSharperMessage(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 3\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Beta Panel\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 3\n")

	msgs := pageRefsForTest(t, root, "dashboard")
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one surface-region-shared warning, got %d: %v", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0], "same order 3") {
		t.Errorf("exact-slot collision should get the sharper message naming the order; got %q", msgs[0])
	}
}

func TestSurfaceRegionShared_SingleFeatureStackDoesNotWarn(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Header\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n  - name: Body\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 2\n")

	msgs := pageRefsForTest(t, root, "dashboard")
	if len(msgs) != 0 {
		t.Errorf("one feature stacking its own fragments must not warn; got %v", msgs)
	}
}
