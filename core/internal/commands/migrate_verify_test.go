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
	return runVerifyMigrationWith(t, dryRun, false)
}

func runVerifyMigrationWith(t *testing.T, dryRun, fragments bool) string {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateVerifyDryRun = dryRun
	migrateVerifyFragments = fragments
	defer func() { migrateVerifyDryRun = false; migrateVerifyFragments = false }()
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

// ---------------------------------------------------------------------------
// WS C — the migration path for artifacts written under the old routing rule.
// ---------------------------------------------------------------------------

// The report a project written under the old rule needs: the routing leaves the
// Create Form fragment with no criteria, because its intent is covered by an
// operation and operations are routed first. Nothing said so before — the run
// looked fully migrated.
func TestMigrateVerify_ReportsProjectedVacancy(t *testing.T) {
	dir := setupTestDir(t)
	writeVerifyFixture(t, dir)

	out := runVerifyMigration(t, false)
	if !strings.Contains(out, `no criteria: fragment "Create Form"`) {
		t.Errorf("the vacancy report did not name the fragment left empty:\n%s", out)
	}
	if strings.Contains(out, `no criteria: fragment "Thing List"`) {
		t.Errorf("a fragment that gained criteria was reported vacant:\n%s", out)
	}
	if !strings.Contains(out, "Fragments still without criteria: 1") {
		t.Errorf("summary count missing or wrong:\n%s", out)
	}
	if !strings.Contains(out, "--fragments") {
		t.Errorf("the report does not say what to do about it:\n%s", out)
	}
}

// The dry-run correctness the projection exists for. --dry-run writes nothing,
// so a report that re-read the file would see pre-splice state and name every
// fragment — including the one the real run fills.
func TestMigrateVerify_VacancyReportIsCorrectUnderDryRun(t *testing.T) {
	dir := setupTestDir(t)
	writeVerifyFixture(t, dir)

	dry := runVerifyMigration(t, true)
	if strings.Contains(dry, `no criteria: fragment "Thing List"`) {
		t.Errorf("--dry-run reported a fragment the run would have filled:\n%s", dry)
	}
	if !strings.Contains(dry, "Fragments still without criteria: 1") {
		t.Errorf("--dry-run vacancy count disagrees with the real run:\n%s", dry)
	}
	// And it really did touch nothing.
	for _, f := range mustLoadSurface(t, filepath.Join(dir, "spec", "intents", "verify-fixture", "surface.yaml")) {
		if len(f.Verify) > 0 {
			t.Errorf("--dry-run wrote verify: to fragment %q", f.Name)
		}
	}
}

// --fragments copies an operation-covered intent's bullets onto the fragments
// sourcing it, which the default routing deliberately does not.
func TestMigrateVerify_FragmentsFlagFillsOperationCoveredFragments(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	runVerifyMigrationWith(t, false, true)

	frags := mustLoadSurface(t, filepath.Join(featDir, "surface.yaml"))
	for _, f := range frags {
		if len(f.Verify) == 0 {
			t.Errorf("fragment %q still has no verify: under --fragments", f.Name)
		}
	}
	// The operations keep theirs — this duplicates, it does not move.
	caps := mustParseCapabilities(t, filepath.Join(featDir, "capabilities.yaml"))
	if len(caps.Operations[0].Verify) == 0 {
		t.Error("--fragments moved the operation's criteria instead of copying them")
	}
}

// Idempotence under merge. Skipping a non-empty entry wholesale used to supply
// this for free; now that a non-empty entry is merged into, de-duplication has
// to supply it instead.
func TestMigrateVerify_FragmentsFlagIsIdempotent(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	surfacePath := filepath.Join(featDir, "surface.yaml")

	runVerifyMigrationWith(t, false, true)
	afterFirst, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatal(err)
	}

	runVerifyMigrationWith(t, false, true)
	afterSecond, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterFirst, afterSecond) {
		t.Errorf("a second --fragments run changed the file:\n--- first ---\n%s\n--- second ---\n%s", afterFirst, afterSecond)
	}
}

// The merge itself: a fragment that already carries one criterion gains the
// missing one rather than being skipped wholesale. Under the old splice a
// fragment with any verify: could never gain a second bullet.
func TestMigrateVerify_MergesIntoExistingVerifyBlock(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	surfacePath := filepath.Join(featDir, "surface.yaml")

	// Give the Create Form fragment one of its intent's two bullets.
	seeded := `feature: verify-fixture
fragments:
    - actions: invoke
      name: Create Form
      order: 1
      page: things
      region: main
      shows: form
      source: '@verify-fixture/create-the-thing'
      verify:
        - Creating a thing returns its id.
    - actions: select-one
      name: Thing List
      order: 2
      page: things
      region: main
      shows: data-list, empty-state
      source: '@verify-fixture/browse-the-things'
`
	if err := os.WriteFile(surfacePath, []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}

	runVerifyMigrationWith(t, false, true)

	frags := mustLoadSurface(t, surfacePath)
	var createForm parser.Fragment
	for _, f := range frags {
		if f.Name == "Create Form" {
			createForm = f
		}
	}
	if len(createForm.Verify) != 2 {
		t.Fatalf("Create Form verify: = %v, want both bullets merged", createForm.Verify)
	}
	joined := strings.Join(createForm.Verify, "\n")
	if !strings.Contains(joined, "returns its id") || !strings.Contains(joined, "conflict") {
		t.Errorf("merge lost or duplicated a bullet: %v", createForm.Verify)
	}
}

// ---------------------------------------------------------------------------
// Splice layout edges. The merge path walks YAML as text, so the layouts it can
// meet are the ones worth pinning: a single forward scan from source: gets two
// of these wrong.
// ---------------------------------------------------------------------------

func spliceFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "surface.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// YAML key order is arbitrary, and a hand-authored fragment may list verify:
// before source:. A forward-only scan finds no block, drops the merged bullets,
// and still counts the entry as touched — silent loss reported as success.
func TestSplice_VerifyBeforeSourceStillMerges(t *testing.T) {
	p := spliceFixture(t, `feature: f
fragments:
    - name: A
      verify:
        - existing one
      source: '@f/one'
    - name: B
      source: '@f/two'
      page: p
`)
	attached, err := spliceAfterSourceLines(p, []verifyInsert{
		{Append: []string{"added one"}},
		{NewBlock: []string{"fresh"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if attached != 2 {
		t.Errorf("attached = %d, want 2", attached)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "added one") {
		t.Errorf("the appended bullet was dropped:\n%s", got)
	}
	frags := mustLoadSurface(t, p)
	if len(frags[0].Verify) != 2 {
		t.Errorf("fragment A verify: = %v, want both bullets", frags[0].Verify)
	}
}

// Other keys between source: and verify:, which is the ordinary generated
// layout once page/region are present.
func TestSplice_MergesAcrossInterveningKeys(t *testing.T) {
	p := spliceFixture(t, `feature: f
fragments:
    - name: A
      source: '@f/one'
      page: p
      region: main
      verify:
        - existing one
`)
	if _, err := spliceAfterSourceLines(p, []verifyInsert{{Append: []string{"added one"}}}); err != nil {
		t.Fatal(err)
	}
	frags := mustLoadSurface(t, p)
	if len(frags[0].Verify) != 2 {
		t.Errorf("verify: = %v, want the bullet merged into the existing block", frags[0].Verify)
	}
}

// The last entry ends at EOF with no trailing newline.
func TestSplice_MergesAtEOFWithoutTrailingNewline(t *testing.T) {
	p := spliceFixture(t, `feature: f
fragments:
    - name: A
      source: '@f/one'
      verify:
        - existing one`)
	if _, err := spliceAfterSourceLines(p, []verifyInsert{{Append: []string{"added one"}}}); err != nil {
		t.Fatal(err)
	}
	frags := mustLoadSurface(t, p)
	if len(frags[0].Verify) != 2 {
		t.Errorf("verify: = %v, want the bullet merged at EOF", frags[0].Verify)
	}
}

// An append against an entry with no verify: block writes nothing and counts
// nothing, rather than guessing at a position.
func TestSplice_AppendWithNoBlockIsNotCounted(t *testing.T) {
	body := `feature: f
fragments:
    - name: A
      source: '@f/one'
      page: p
`
	p := spliceFixture(t, body)
	attached, err := spliceAfterSourceLines(p, []verifyInsert{{Append: []string{"orphan"}}})
	if err != nil {
		t.Fatal(err)
	}
	if attached != 0 {
		t.Errorf("attached = %d, want 0 — there was no block to append to", attached)
	}
	got, _ := os.ReadFile(p)
	if string(got) != body {
		t.Errorf("the file was rewritten anyway:\n%s", got)
	}
}
