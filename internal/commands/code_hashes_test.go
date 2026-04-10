package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/internal/config"
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

	hashes, skipped, err := buildCodeHashes("my-feature", sourceRoot)
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
	if err := saveCodeHashes("my-feature", hashes); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadCodeHashes("my-feature")
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

	hashes, skipped, err := buildCodeHashes("my-feature", sourceRoot)
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

	output, err := computeVerifyOutput("brand-new")
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

	hashes, _, err := buildCodeHashes("my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCodeHashes("my-feature", hashes); err != nil {
		t.Fatal(err)
	}

	// Hand-edit one file (simulating a designer tweak).
	os.WriteFile(modifiedFile, []byte(`// parlay-feature: my-feature
// parlay-component: modified-comp
func Modified() { /* HAND-EDITED */ }
`), 0644)

	output, err := computeVerifyOutput("my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if !output.HasHashes {
		t.Fatal("expected has_hashes=true")
	}
	if len(output.Stable) != 1 || output.Stable[0].Component != "stable-comp" {
		t.Errorf("Stable = %+v, want [stable-comp]", output.Stable)
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

	hashes, _, err := buildCodeHashes("my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCodeHashes("my-feature", hashes); err != nil {
		t.Fatal(err)
	}

	// Delete the file (simulating user removal).
	os.Remove(gone)

	output, err := computeVerifyOutput("my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Missing) != 1 || output.Missing[0].Component != "gone-comp" {
		t.Errorf("Missing = %+v, want [gone-comp]", output.Missing)
	}
}

func TestCodeHashesPath(t *testing.T) {
	got := codeHashesPath("foo")
	want := filepath.Join(config.BuildPath("foo"), CodeHashesFile)
	if got != want {
		t.Errorf("codeHashesPath = %q, want %q", got, want)
	}
}
