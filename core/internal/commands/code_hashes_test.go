package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// writeMarkedFile creates a file with a parlay marker for the given
// feature/component plus arbitrary body content.
func writeMarkedFile(t *testing.T, path, feature, component, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "// parlay-feature: " + feature + "\n" +
		"// parlay-component: " + component + "\n" +
		body + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveCodeHashes_Roundtrip(t *testing.T) {
	dir := setupTestDir(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")

	writeMarkedFile(t, filepath.Join(sourceRoot, "alpha.go"),
		"my-feature", "alpha", "func Alpha() {}")
	writeMarkedFile(t, filepath.Join(sourceRoot, "beta.go"),
		"my-feature", "beta", "func Beta() {}")

	hashes, skipped, err := buildCodeHashes(testContext(t), "my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(hashes.Files) != 2 {
		t.Fatalf("Files count = %d, want 2", len(hashes.Files))
	}
	for path, entry := range hashes.Files {
		if entry.Component == "" || entry.Hash == "" {
			t.Errorf("incomplete entry for %s: %+v", path, entry)
		}
	}

	// Save and reload — content must round-trip identically.
	if err := saveCodeHashes(testContext(t), "my-feature", hashes); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadCodeHashes(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("loadCodeHashes returned nil after save")
	}
	if len(loaded.Files) != 2 {
		t.Errorf("loaded.Files count = %d, want 2", len(loaded.Files))
	}
	for path, originalEntry := range hashes.Files {
		loadedEntry, ok := loaded.Files[path]
		if !ok {
			t.Errorf("loaded sidecar missing %s", path)
			continue
		}
		if loadedEntry.Hash != originalEntry.Hash {
			t.Errorf("hash mismatch for %s: %s vs %s",
				path, loadedEntry.Hash, originalEntry.Hash)
		}
	}
}

func TestSaveCodeHashes_FiltersForeignFeature(t *testing.T) {
	dir := setupTestDir(t)
	sourceRoot := filepath.Join(dir, "cmd", "shared")

	writeMarkedFile(t, filepath.Join(sourceRoot, "mine.go"),
		"my-feature", "mine", "package shared")
	writeMarkedFile(t, filepath.Join(sourceRoot, "yours.go"),
		"other-feature", "yours", "package shared")

	hashes, skipped, err := buildCodeHashes(testContext(t), "my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(hashes.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(hashes.Files))
	}
	for _, entry := range hashes.Files {
		if entry.Component != "mine" {
			t.Errorf("expected only 'mine' component, got %q", entry.Component)
		}
	}
}

func TestVerifyGenerated_NoHashes(t *testing.T) {
	setupTestDir(t)

	output, err := computeVerifyOutput(testContext(t), "brand-new")
	if err != nil {
		t.Fatal(err)
	}
	if output.HasHashes {
		t.Error("expected has_hashes=false when no sidecar exists")
	}
	if len(output.Stable)+len(output.Modified)+len(output.Missing) != 0 {
		t.Errorf("expected empty classification, got %+v", output)
	}
}

func TestVerifyGenerated_StableAndModified(t *testing.T) {
	dir := setupTestDir(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")

	stableFile := filepath.Join(sourceRoot, "stable.go")
	modifiedFile := filepath.Join(sourceRoot, "modified.go")
	writeMarkedFile(t, stableFile, "my-feature", "stable-comp", "func Stable() {}")
	writeMarkedFile(t, modifiedFile, "my-feature", "modified-comp", "func Modified() {}")

	hashes, _, err := buildCodeHashes(testContext(t), "my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCodeHashes(testContext(t), "my-feature", hashes); err != nil {
		t.Fatal(err)
	}

	// Hand-edit one file (simulating a designer tweak).
	os.WriteFile(modifiedFile, []byte(`// parlay-feature: my-feature
// parlay-component: modified-comp
func Modified() { /* HAND-EDITED */ }
`), 0644)

	output, err := computeVerifyOutput(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if !output.HasHashes {
		t.Fatal("expected has_hashes=true")
	}
	// The snapshot was written without an emission declaration, so nothing
	// in it can be certified as generated. The untouched file is therefore
	// `unknown`, not `stable` — a snapshot that never learned who wrote its
	// files must not read as a clean bill of health.
	if len(output.Stable) != 0 {
		t.Errorf("Stable = %+v, want [] for an undeclared snapshot", output.Stable)
	}
	if len(output.Unknown) != 1 || output.Unknown[0].Component != "stable-comp" {
		t.Errorf("Unknown = %+v, want [stable-comp]", output.Unknown)
	}
	if len(output.Modified) != 1 || output.Modified[0].Component != "modified-comp" {
		t.Errorf("Modified = %+v, want [modified-comp]", output.Modified)
	}
	if len(output.Missing) != 0 {
		t.Errorf("Missing = %+v, want []", output.Missing)
	}
}

func TestVerifyGenerated_MissingFile(t *testing.T) {
	dir := setupTestDir(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")

	gone := filepath.Join(sourceRoot, "gone.go")
	writeMarkedFile(t, gone, "my-feature", "gone-comp", "func Gone() {}")

	hashes, _, err := buildCodeHashes(testContext(t), "my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCodeHashes(testContext(t), "my-feature", hashes); err != nil {
		t.Fatal(err)
	}

	// Delete the file (simulating user removal).
	os.Remove(gone)

	output, err := computeVerifyOutput(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Missing) != 1 || output.Missing[0].Component != "gone-comp" {
		t.Errorf("Missing = %+v, want [gone-comp]", output.Missing)
	}
}

// R4-18. A snapshot with no `schema-version` predates provenance, so it
// cannot legitimately carry any — yet the field was read, reported as
// `schema_version: 0`, and then never routed on. A v0 snapshot whose entries
// happened to say `provenance: generated` graded every one of its files
// `stable`: the least trustworthy snapshot in the tree produced the most
// reassuring report in it, and `--strict` passed on it.
//
// The fix is the third answer — "could not be checked" — and the point of
// this test is that it must not be reachable by writing the right word into
// a file format that never promised it.
func TestVerifyGenerated_PreProvenanceSnapshotIsUnknownNotStable(t *testing.T) {
	dir := setupTestDir(t)
	sourceRoot := filepath.Join(dir, "cmd", "legacy")

	generated := filepath.Join(sourceRoot, "generated.go")
	adopted := filepath.Join(sourceRoot, "adopted.go")
	gone := filepath.Join(sourceRoot, "gone.go")
	writeMarkedFile(t, generated, "legacy", "generated-comp", "func Generated() {}")
	writeMarkedFile(t, adopted, "legacy", "adopted-comp", "func Adopted() {}")
	writeMarkedFile(t, gone, "legacy", "gone-comp", "func Gone() {}")

	cfg := testContext(t)
	hashes, _, err := buildCodeHashes(cfg, "legacy", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Forge the pre-provenance snapshot: no schema-version, but entries
	// carrying provenance words a v0 writer could never have written.
	hashes.SchemaVersion = 0
	for path, entry := range hashes.Files {
		switch entry.Component {
		case "generated-comp":
			entry.Provenance = ProvenanceGenerated
		case "adopted-comp":
			entry.Provenance = ProvenanceAdopted
		case "gone-comp":
			entry.Provenance = ProvenanceGenerated
		}
		hashes.Files[path] = entry
	}
	if err := saveCodeHashes(cfg, "legacy", hashes); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	output, err := computeVerifyOutput(cfg, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if output.SchemaVersion != 0 {
		t.Fatalf("test setup: wanted a v0 snapshot, got schema_version=%d", output.SchemaVersion)
	}
	if len(output.Stable) != 0 {
		t.Errorf("Stable = %+v, want [] — a v0 snapshot cannot certify anything", output.Stable)
	}
	if len(output.Adopted) != 0 {
		t.Errorf("Adopted = %+v, want [] — a v0 snapshot's provenance is not readable", output.Adopted)
	}
	if len(output.Unknown) != 2 {
		t.Errorf("Unknown = %+v, want both present files", output.Unknown)
	}
	// Missing survives the version check. It is not a claim about provenance:
	// the file is absent, which this command established itself. Folding it
	// into Unknown would discard a fact to express a doubt about another one.
	if len(output.Missing) != 1 || output.Missing[0].Component != "gone-comp" {
		t.Errorf("Missing = %+v, want [gone-comp] regardless of schema version", output.Missing)
	}
}

// The control. A current snapshot still reads its provenance, so the fix
// above is scoped to the version that cannot be trusted rather than
// disabling provenance everywhere.
func TestVerifyGenerated_CurrentSnapshotStillReadsProvenance(t *testing.T) {
	dir := setupTestDir(t)
	sourceRoot := filepath.Join(dir, "cmd", "current")

	generated := filepath.Join(sourceRoot, "generated.go")
	writeMarkedFile(t, generated, "current", "generated-comp", "func Generated() {}")

	cfg := testContext(t)
	hashes, _, err := buildCodeHashes(cfg, "current", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	hashes.SchemaVersion = CodeHashesSchemaVersion
	for path, entry := range hashes.Files {
		entry.Provenance = ProvenanceGenerated
		hashes.Files[path] = entry
	}
	if err := saveCodeHashes(cfg, "current", hashes); err != nil {
		t.Fatal(err)
	}

	output, err := computeVerifyOutput(cfg, "current")
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Stable) != 1 || output.Stable[0].Component != "generated-comp" {
		t.Errorf("Stable = %+v, want [generated-comp]", output.Stable)
	}
	if len(output.Unknown) != 0 {
		t.Errorf("Unknown = %+v, want []", output.Unknown)
	}
}

func TestCodeHashesPath(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	got := codeHashesPath(cfg, "foo")
	want := filepath.Join(dir, config.ParlayDir, config.BuildDir, "foo", CodeHashesFile)
	wantClean := filepath.Clean(want)
	gotClean := filepath.Clean(got)
	if filepath.Base(gotClean) != filepath.Base(wantClean) ||
		filepath.Base(filepath.Dir(gotClean)) != "foo" {
		t.Errorf("codeHashesPath = %q, want %q", got, want)
	}
}
