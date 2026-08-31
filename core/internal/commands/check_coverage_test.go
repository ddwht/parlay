package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/parser"
)

func TestCheckChain_NoDownstreamArtifacts(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := []parser.Intent{
		{Title: "Do Something", Slug: "do-something"},
	}

	chain := checkChain(testContext(t), featureDir, "test-feature", intents)
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

	chain := checkChain(testContext(t), featureDir, "test-feature", intents)
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

	chain := checkChain(testContext(t), featureDir, "test-feature", intents)
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
	buildDir := testContext(t).BuildPath("my-feature")
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

	chain := checkChain(testContext(t), featureDir, "my-feature", intents)
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

// TestParseBuildfileRefs_V1 pins the legacy single-target shape: components
// live at the top level. This is the byte-for-byte behavior that must not
// regress when the reader is made v2-aware.
func TestParseBuildfileRefs_V1(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "buildfile.yaml")
	buildfile := `feature: notes
adapter: go-cli
components:
  note-list:
    source: "@notes/note-list"
  note-form:
    source: "@notes/note-form"
`
	os.WriteFile(path, []byte(buildfile), 0644)

	refs, err := parseBuildfileRefs(path, "notes")
	if err != nil {
		t.Fatalf("parseBuildfileRefs: %v", err)
	}
	if got := refs["note-list"]; got != "note-list" {
		t.Errorf("note-list source = %q, want %q", got, "note-list")
	}
	if got := refs["note-form"]; got != "note-form" {
		t.Errorf("note-form source = %q, want %q", got, "note-form")
	}
}

// TestParseBuildfileRefs_V2 is the BP1 regression. A v2 (multi-target)
// buildfile keeps its components under targets.presentation.components:. The
// old private top-level `components:` decode saw nothing here, so
// check-coverage reported every fragment of a multi-target feature uncovered
// while validate --deep — reading the same file through the shared resolution
// this now shares — reported it complete. Both must agree: the components must
// resolve.
func TestParseBuildfileRefs_V2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "buildfile.yaml")
	buildfile := `feature: notes
schema_version: 1
adapter-set: notes-stack
targets:
  presentation:
    adapter: react-antd
    components:
      note-list:
        source: "@notes/note-list"
        widget: Table
      note-form:
        source: "@notes/note-form"
        widget: Form
    routes:
      - path: /notes
        page: NotesPage
`
	os.WriteFile(path, []byte(buildfile), 0644)

	refs, err := parseBuildfileRefs(path, "notes")
	if err != nil {
		t.Fatalf("parseBuildfileRefs: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("v2 buildfile resolved %d components, want 2 (BP1: top-level-only reader saw 0)", len(refs))
	}
	if got := refs["note-list"]; got != "note-list" {
		t.Errorf("note-list source = %q, want %q", got, "note-list")
	}
	if got := refs["note-form"]; got != "note-form" {
		t.Errorf("note-form source = %q, want %q", got, "note-form")
	}
}

// A dialog belonging to a superseded intent is history, not debt.
//
// The hazard is specific: matchedDialogs is populated while iterating intents,
// so feeding it only the active set drops every superseded intent's dialog
// through to the orphan walk. Coverage would then report preserved history as
// cleanup work — telling a designer to delete the record of a decision the
// project deliberately kept.
func TestCheckCoverage_RetiredIntentsDialogIsNotAnOrphan(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	if err := os.WriteFile(filepath.Join(featDir, "dialogs.md"), []byte(`# Verify Fixture — Dialogs

---

### Create The Thing

**Trigger**: The designer creates a thing.

User: /create
System: Created.

---

### Browse The Things

**Trigger**: The designer browses.

User: /list
System: Here they are.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	writeAmendment(t, featDir, "001-browsing-moves-to-search.md", supersedeBrowse)
	writeBaselineApplied(t, "verify-fixture", 1)

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	if err := runCheckCoverage(cmd, []string{"@verify-fixture"}); err != nil {
		t.Fatalf("check-coverage failed: %v\n%s", err, buf.String())
	}

	var out coverageOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}

	for _, o := range out.Orphans {
		if o == "Browse The Things" {
			t.Error("a retired intent's dialog must not be reported as an orphan — that presents preserved history as cleanup debt")
		}
	}
	if len(out.Retired) != 1 || out.Retired[0].Dialog != "Browse The Things" {
		t.Errorf("expected the dialog to be classified as retired, got %+v", out.Retired)
	}
	// And the retired promise itself no longer demands coverage.
	for _, u := range out.Uncovered {
		if u == "Browse The Things" {
			t.Error("a withdrawn promise must not still demand a dialog")
		}
	}
}

// writeBaselineApplied pins how far the ledger has been applied.
// writeBaselineApplied marks a feature applied THROUGH seq, with the evidence
// a real applied record carries.
//
// The evidence is not decoration. Current-state resolution refuses a record at
// or below the marker with no recorded hash, because resolving without it
// answers with the text that preceded it — so a fixture that stamps a marker
// and records nothing is a hand-advanced capsule, which is exactly the state
// the rule exists to catch. It stopped being a usable shortcut the moment
// promises could be revised rather than only retired.
func writeBaselineApplied(t *testing.T, slug string, seq int) {
	t.Helper()
	cfg := testContext(t)
	dir := cfg.BuildPath(slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hashes := map[string]string{}
	records, err := parser.LoadFeatureAmendments(cfg.FeaturePath(slug))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range records {
		if a.Seq > seq {
			continue
		}
		h, ok := hashWholeFile(a.Path)
		if !ok {
			t.Fatalf("hash %s", a.Path)
		}
		hashes[filepath.Base(a.Path)] = h
	}
	b := Baseline{
		SchemaVersion: BaselineSchemaVersion, LastAppliedAmendment: seq,
		Sources: &HashedSources{Amendments: hashes},
	}
	data, err := yaml.Marshal(&b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".baseline.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
