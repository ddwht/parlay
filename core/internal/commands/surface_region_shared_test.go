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

// pageRefsWithManifest writes a page manifest with the given body and returns
// the messages for the requested code emitted by validatePageReferences.
func pageRefsWithManifest(t *testing.T, root, pageName, manifest, code string) []string {
	t.Helper()
	pagePath := filepath.Join(root, config.SpecDir, "pages", pageName+".page.md")
	if err := os.MkdirAll(filepath.Dir(pagePath), 0755); err != nil {
		t.Fatalf("mkdir pages: %v", err)
	}
	if err := os.WriteFile(pagePath, []byte(manifest), 0644); err != nil {
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
	var msgs []string
	for _, e := range errs {
		if e.Code == code {
			msgs = append(msgs, e.Message)
		}
	}
	return msgs
}

// pageRefsForTest keeps the minimal-manifest (no region ordering) helper the
// original WP3 tests used; under WP8 an unordered shared region is a conflict.
func pageRefsForTest(t *testing.T, root, pageName, code string) []string {
	t.Helper()
	return pageRefsWithManifest(t, root, pageName, "---\nname: "+pageName+"\n---\n", code)
}

// Two features, one region, no manifest ordering and no supersedes: the WP3
// warning has escalated to a blocking surface-region-conflict (WP8).
func TestSurfaceRegionShared_UnresolvedStackConflicts(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Beta Panel\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 2\n")

	if shared := pageRefsForTest(t, root, "dashboard", "surface-region-shared"); len(shared) != 0 {
		t.Fatalf("unresolved stack must not be a mere warning; got %v", shared)
	}
	msgs := pageRefsForTest(t, root, "dashboard", "surface-region-conflict")
	if len(msgs) != 1 {
		t.Fatalf("expected one surface-region-conflict, got %d: %v", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0], "alpha") || !strings.Contains(msgs[0], "beta") {
		t.Errorf("conflict should name both features; got %q", msgs[0])
	}
}

// Two fragments both superseding the same target is a two-headed chain: an
// error, mirroring duplicate amendment sequence numbers.
func TestSupersedesConflicts_TwoHeadsError(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    supersedes: \"@base/viewport\"\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Beta Panel\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    supersedes: \"@base/viewport\"\n")

	conflicts := supersedesConflicts(filepath.Join(root, config.SpecDir))
	if len(conflicts) != 1 {
		t.Fatalf("expected one surface-supersedes-conflict, got %d: %v", len(conflicts), conflicts)
	}
	if conflicts[0].Code != "surface-supersedes-conflict" || conflicts[0].Severity != "error" {
		t.Errorf("expected blocking surface-supersedes-conflict, got %+v", conflicts[0])
	}
	if !strings.Contains(conflicts[0].Message, "@base/viewport") {
		t.Errorf("message should name the contested target; got %q", conflicts[0].Message)
	}
}

// A single fragment superseding a target is a normal chain — no conflict.
//
// The target is declared here. It was not before: the fixture superseded
// "@base/viewport" while no surface in the tree declared it, and the old
// detect-only pass never looked a target up, so the fixture asserted "clean"
// over a tree whose supersedes: pointed at nothing. Resolving the edge is what
// makes the dangling ref visible, so the fixture now has to be sound.
func TestSupersedesConflicts_SingleHeadOK(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "base",
		"fragments:\n  - name: Viewport\n    shows: read-collection\n    source: \"@base/intent\"\n    page: dashboard\n    region: main\n")
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    supersedes: \"@base/viewport\"\n")

	if conflicts := supersedesConflicts(filepath.Join(root, config.SpecDir)); len(conflicts) != 0 {
		t.Errorf("a single superseding fragment must not conflict; got %v", conflicts)
	}
}

// The dangling-target shape the fixture above used to have, asserted directly
// rather than left as an accident of another test's setup.
func TestSupersedesConflicts_UnknownTargetIsRefused(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    supersedes: \"@base/viewport\"\n")

	conflicts := supersedesConflicts(filepath.Join(root, config.SpecDir))
	if len(conflicts) != 1 || conflicts[0].Code != "surface-supersedes-target-unknown" {
		t.Fatalf("expected one surface-supersedes-target-unknown, got %v", conflicts)
	}
}

func TestSurfaceRegionShared_ExactOrderCollisionSharperMessage(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 3\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Beta Panel\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 3\n")

	msgs := pageRefsForTest(t, root, "dashboard", "surface-region-conflict")
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one surface-region-conflict, got %d: %v", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0], "same order 3") {
		t.Errorf("exact-slot collision should get the sharper message naming the order; got %q", msgs[0])
	}
}

func TestSurfaceRegionShared_SingleFeatureStackDoesNotWarn(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Header\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n  - name: Body\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 2\n")

	if c := pageRefsForTest(t, root, "dashboard", "surface-region-conflict"); len(c) != 0 {
		t.Errorf("one feature stacking its own fragments must not conflict; got %v", c)
	}
	if w := pageRefsForTest(t, root, "dashboard", "surface-region-shared"); len(w) != 0 {
		t.Errorf("one feature stacking its own fragments must not warn; got %v", w)
	}
}

// A supersedes: annotation naming another occupant resolves the stack — back to
// a non-blocking surface-region-shared note, no conflict.
func TestSurfaceRegionShared_SupersedesResolves(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Beta Panel\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 2\n    supersedes: \"@alpha/alpha-panel\"\n")

	if c := pageRefsForTest(t, root, "dashboard", "surface-region-conflict"); len(c) != 0 {
		t.Fatalf("supersedes: must resolve the stack, expected no conflict; got %v", c)
	}
	w := pageRefsForTest(t, root, "dashboard", "surface-region-shared")
	if len(w) != 1 {
		t.Fatalf("resolved stack should still emit one shared note; got %d: %v", len(w), w)
	}
}

// A page manifest that orders the region resolves the stack as well.
func TestSurfaceRegionShared_ManifestOrderingResolves(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Alpha Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Beta Panel\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 2\n")

	manifest := "# Dashboard\n\n## main\n\n1. @alpha/alpha-panel\n2. @beta/beta-panel\n"
	if c := pageRefsWithManifest(t, root, "dashboard", manifest, "surface-region-conflict"); len(c) != 0 {
		t.Fatalf("manifest ordering must resolve the stack, expected no conflict; got %v", c)
	}
	w := pageRefsWithManifest(t, root, "dashboard", manifest, "surface-region-shared")
	if len(w) != 1 {
		t.Fatalf("manifest-ordered stack should emit one shared note; got %d: %v", len(w), w)
	}
}
