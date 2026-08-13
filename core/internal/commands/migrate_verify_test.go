// parlay-feature: parlay-tool/multi-adapter
// parlay-component: verify-relocation-migration
// parlay-artifact: test

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func mustParseCapabilities(t *testing.T, path string) *parser.Capabilities {
	t.Helper()
	caps, err := parser.ParseCapabilities(path)
	if err != nil {
		t.Fatalf("re-parse capabilities after splice: %v", err)
	}
	return caps
}

func mustLoadSurface(t *testing.T, path string) []parser.Fragment {
	t.Helper()
	frags, err := parser.LoadSurfaceYAML(path)
	if err != nil {
		t.Fatalf("re-parse surface after splice: %v", err)
	}
	return frags
}

// writeVerifyFixture lays down a feature with two intents carrying Verify
// bullets, a capabilities.yaml whose one operation sources the first intent,
// and a surface.yaml whose fragments source both intents. The expected
// routing: intent one lands on the operation (and NOT on the fragment that
// also sources it); intent two has no operation and lands on its fragment.
func writeVerifyFixture(t *testing.T, dir string) string {
	t.Helper()
	featDir := filepath.Join(dir, "spec", "intents", "verify-fixture")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	intents := `# Verify Fixture

## Create The Thing

**Goal**: Things get created.
**Persona**: Designer.
**Priority**: P0

**Verify**:
- Creating a thing returns its id.
- A duplicate name is rejected with "conflict: already exists".

## Browse The Things

**Goal**: Things get browsed.
**Persona**: Designer.
**Priority**: P1

**Verify**:
- The list shows every thing, newest first.
`
	caps := `schema_version: 1
feature: verify-fixture

operations:
  - id: thing.create
    source: '@verify-fixture/create-the-thing'
    kind: command
    subject:
      entity: Thing
    steps:
      - { type: validate-input }
      - { type: create-one, entity: Thing }
`
	surface := `feature: verify-fixture
fragments:
    - actions: invoke
      name: Create Form
      order: 1
      page: things
      region: main
      shows: form
      source: '@verify-fixture/create-the-thing'
    - actions: select-one
      name: Thing List
      order: 2
      page: things
      region: main
      shows: data-list, empty-state
      source: '@verify-fixture/browse-the-things'
`
	for name, content := range map[string]string{
		"intents.md":        intents,
		"capabilities.yaml": caps,
		"surface.yaml":      surface,
	} {
		if err := os.WriteFile(filepath.Join(featDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return featDir
}

func runVerifyMigration(t *testing.T, dryRun bool) string {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateVerifyDryRun = dryRun
	defer func() { migrateVerifyDryRun = false }()
	if err := runMigrateVerify(cmd, nil); err != nil {
		t.Fatalf("runMigrateVerify: %v", err)
	}
	return buf.String()
}

func TestMigrateVerify_RoutesOperationsFirstThenFragments(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	out := runVerifyMigration(t, false)

	caps, err := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(caps), "verify:") ||
		!strings.Contains(string(caps), "Creating a thing returns its id.") {
		t.Errorf("operation sourcing create-the-thing should carry its Verify bullets; got:\n%s", caps)
	}
	// The quoted bullet must survive the splice YAML-safely.
	if !strings.Contains(string(caps), "conflict: already exists") {
		t.Errorf("bullet with a colon must survive; got:\n%s", caps)
	}

	surface, err := os.ReadFile(filepath.Join(featDir, "surface.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(surface), "The list shows every thing, newest first.") {
		t.Errorf("fragment sourcing browse-the-things should carry its bullets; got:\n%s", surface)
	}
	if strings.Contains(string(surface), "Creating a thing returns its id.") {
		t.Errorf("intent covered by an operation must not also land on its fragment; got:\n%s", surface)
	}

	if !strings.Contains(out, "attached to 1 operation(s)") || !strings.Contains(out, "attached to 1 fragment(s)") {
		t.Errorf("routing summary should count one operation and one fragment; got:\n%s", out)
	}
}

func TestMigrateVerify_SplicedFilesStillParse(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	runVerifyMigration(t, false)

	// The splice must produce valid YAML that the typed parsers accept, with
	// verify attached to the right entries.
	capsPath := filepath.Join(featDir, "capabilities.yaml")
	caps := mustParseCapabilities(t, capsPath)
	if len(caps.Operations) != 1 || len(caps.Operations[0].Verify) != 2 {
		t.Fatalf("expected 1 op with 2 verify bullets after splice; got %+v", caps.Operations)
	}
	frags := mustLoadSurface(t, filepath.Join(featDir, "surface.yaml"))
	if len(frags) != 2 {
		t.Fatalf("expected 2 fragments, got %d", len(frags))
	}
	if len(frags[0].Verify) != 0 {
		t.Errorf("fragment covered by the operation should have no verify; got %v", frags[0].Verify)
	}
	if len(frags[1].Verify) != 1 {
		t.Errorf("browse fragment should have 1 bullet; got %v", frags[1].Verify)
	}
}

func TestMigrateVerify_SecondRunIsNoOp(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	runVerifyMigration(t, false)

	capsAfterFirst, _ := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	surfaceAfterFirst, _ := os.ReadFile(filepath.Join(featDir, "surface.yaml"))

	out := runVerifyMigration(t, false)

	capsAfterSecond, _ := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	surfaceAfterSecond, _ := os.ReadFile(filepath.Join(featDir, "surface.yaml"))
	if !bytes.Equal(capsAfterFirst, capsAfterSecond) {
		t.Errorf("second run modified capabilities.yaml")
	}
	if !bytes.Equal(surfaceAfterFirst, surfaceAfterSecond) {
		t.Errorf("second run modified surface.yaml")
	}
	if strings.Contains(out, "attached to") {
		t.Errorf("second run should attach nothing; got:\n%s", out)
	}
}

func TestMigrateVerify_DryRunTouchesNothing(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	capsBefore, _ := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	surfaceBefore, _ := os.ReadFile(filepath.Join(featDir, "surface.yaml"))

	out := runVerifyMigration(t, true)

	capsAfter, _ := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	surfaceAfter, _ := os.ReadFile(filepath.Join(featDir, "surface.yaml"))
	if !bytes.Equal(capsBefore, capsAfter) || !bytes.Equal(surfaceBefore, surfaceAfter) {
		t.Errorf("dry run modified files")
	}
	if !strings.Contains(out, "dry run") {
		t.Errorf("dry run should announce itself; got:\n%s", out)
	}
	if !strings.Contains(out, "attached to 1 operation(s)") {
		t.Errorf("dry run should still report the would-be routing; got:\n%s", out)
	}
}

func TestMigrateVerify_UnroutedIntentReported(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "orphan")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	intents := `# Orphan

## Nobody Sources This

**Goal**: G.
**Persona**: P.

**Verify**:
- A bullet with nowhere to go.
`
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte(intents), 0o644); err != nil {
		t.Fatal(err)
	}
	out := runVerifyMigration(t, false)
	if !strings.Contains(out, "unrouted: nobody-sources-this") {
		t.Errorf("intent with no artifact entry should be reported unrouted; got:\n%s", out)
	}
}
