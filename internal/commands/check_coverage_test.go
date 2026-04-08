package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/parser"
)

func TestCheckChain_NoDownstreamArtifacts(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := []parser.Intent{
		{Title: "Do Something", Slug: "do-something"},
	}

	chain := checkChain(featureDir, "test-feature", intents)
	if chain != nil {
		t.Error("expected nil chain when no downstream artifacts exist")
	}
}

func TestCheckChain_IntentsWithoutSurface(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	surface := `# Test — Surface

---

## First Fragment

**Shows**: Some data
**Source**: @test-feature/do-something
`
	os.WriteFile(filepath.Join(featureDir, "surface.md"), []byte(surface), 0644)

	intents := []parser.Intent{
		{Title: "Do Something", Slug: "do-something"},
		{Title: "Do Another", Slug: "do-another"},
	}

	chain := checkChain(featureDir, "test-feature", intents)
	if chain == nil {
		t.Fatal("expected chain coverage report")
	}

	if len(chain.IntentsWithoutSurface) != 1 {
		t.Fatalf("IntentsWithoutSurface = %d, want 1", len(chain.IntentsWithoutSurface))
	}
	if chain.IntentsWithoutSurface[0].Name != "Do Another" {
		t.Errorf("gap name = %q, want %q", chain.IntentsWithoutSurface[0].Name, "Do Another")
	}
}

func TestCheckChain_OrphanedSurfaceReference(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	surface := `## Stale Fragment

**Shows**: Old data
**Source**: @test-feature/removed-intent
`
	os.WriteFile(filepath.Join(featureDir, "surface.md"), []byte(surface), 0644)

	intents := []parser.Intent{
		{Title: "Active Intent", Slug: "active-intent"},
	}

	chain := checkChain(featureDir, "test-feature", intents)
	if chain == nil {
		t.Fatal("expected chain coverage report")
	}

	if len(chain.OrphanedReferences) != 1 {
		t.Fatalf("OrphanedReferences = %d, want 1", len(chain.OrphanedReferences))
	}
}

func TestCheckChain_FullChain(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)
	buildDir := config.BuildPath("my-feature")
	os.MkdirAll(buildDir, 0755)

	// Surface with two fragments
	surface := `## Fragment A

**Shows**: Data A
**Source**: @my-feature/intent-a

---

## Fragment B

**Shows**: Data B
**Source**: @my-feature/intent-b
`
	os.WriteFile(filepath.Join(featureDir, "surface.md"), []byte(surface), 0644)

	// Buildfile with only one component (Fragment A has a component, Fragment B doesn't)
	buildfile := `feature: my-feature
adapter: go-cli
components:
  comp-a:
    source: "@my-feature/fragment-a"
`
	os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte(buildfile), 0644)

	// Testcases with a suite for comp-a
	testcases := `feature: my-feature
framework: vitest
suites:
  - name: "test comp-a"
    component: comp-a
    fixture: default
    intent: "@my-feature/intent-a"
    cases: []
`
	os.WriteFile(filepath.Join(buildDir, "testcases.yaml"), []byte(testcases), 0644)

	intents := []parser.Intent{
		{Title: "Intent A", Slug: "intent-a"},
		{Title: "Intent B", Slug: "intent-b"},
	}

	chain := checkChain(featureDir, "my-feature", intents)
	if chain == nil {
		t.Fatal("expected chain coverage report")
	}

	// Fragment B has no buildfile component
	if len(chain.FragmentsWithoutBuildfile) != 1 {
		t.Errorf("FragmentsWithoutBuildfile = %d, want 1", len(chain.FragmentsWithoutBuildfile))
	}

	// All components have tests
	if len(chain.ComponentsWithoutTests) != 0 {
		t.Errorf("ComponentsWithoutTests = %d, want 0", len(chain.ComponentsWithoutTests))
	}
}

func TestParseSourceRefs(t *testing.T) {
	refs := parseSourceRefs("@my-feature/intent-a, @my-feature/intent-b", "my-feature")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0] != "intent-a" {
		t.Errorf("refs[0] = %q", refs[0])
	}
	if refs[1] != "intent-b" {
		t.Errorf("refs[1] = %q", refs[1])
	}

	// Different feature prefix should be ignored
	refs = parseSourceRefs("@other-feature/intent-x", "my-feature")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs for different feature, got %d", len(refs))
	}
}
