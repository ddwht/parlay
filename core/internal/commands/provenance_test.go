package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// declare writes an emission manifest listing paths, as codegen would by
// appending one line per file it wrote.
func declare(t *testing.T, cfg *config.Context, paths ...string) *emissionDeclaration {
	t.Helper()
	dir := cfg.ProjectBuildPath()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, p := range paths {
		body += p + "\n"
	}
	path := filepath.Join(dir, DefaultEmittedManifest)
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	decl, _, err := loadEmittedManifest(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	return decl
}

// The central defect: a hand-edit to a generated file was blessed into the
// baseline on the next save and became invisible from then on. It must come
// back as `adopted`, not as `stable`.
func TestSaveBuildStateDoesNotBlessAnUndeclaredChange(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	file := filepath.Join(sourceRoot, "widget.go")
	writeMarkedFile(t, file, "my-feature", "widget", "func Widget() {}")

	// 1. Codegen writes the file and declares it.
	first, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, declare(t, cfg, file), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := first.Files[file].Provenance; got != ProvenanceGenerated {
		t.Fatalf("a declared emission must be generated, got %q", got)
	}

	// 2. A human edits it.
	if err := os.WriteFile(file, []byte(`// parlay-feature: my-feature
// parlay-component: widget
func Widget() { /* HAND-EDITED */ }
`), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. A later run saves without declaring this file.
	second, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, declare(t, cfg), first)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Files[file].Provenance; got != ProvenanceAdopted {
		t.Fatalf("an undeclared change must be adopted, got %q", got)
	}

	// And verify-generated must say so rather than calling it stable.
	if err := saveProjectCodeHashesForTest(cfg, second); err != nil {
		t.Fatal(err)
	}
	out, err := computeProjectVerifyOutput(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Adopted) != 1 || out.Adopted[0].Path != file {
		t.Errorf("Adopted = %+v, want the hand-edited file", out.Adopted)
	}
	if len(out.Stable) != 0 {
		t.Errorf("Stable = %+v, want []", out.Stable)
	}
}

// The backward-compatibility path. Reading an empty provenance as
// "generated" would preserve today's silent blessing straight through the
// upgrade, so a pre-provenance snapshot must land in `unknown` — visibly not
// certified — rather than in `stable`.
func TestCodeHashesEmptyProvenanceIsNotGenerated(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	file := filepath.Join(sourceRoot, "old.go")
	writeMarkedFile(t, file, "my-feature", "old", "func Old() {}")

	hash, err := hashFileContent(file)
	if err != nil {
		t.Fatal(err)
	}
	// A version-0 snapshot: no schema-version, no provenance anywhere.
	legacy := &CodeHashes{
		GeneratedAt: "2026-01-01T00:00:00Z",
		Files:       map[string]CodeHashEntry{file: {Component: "old", Hash: hash}},
	}
	if err := saveProjectCodeHashesForTest(cfg, legacy); err != nil {
		t.Fatal(err)
	}

	out, err := computeProjectVerifyOutput(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.SchemaVersion != 0 {
		t.Errorf("schema version = %d, want 0 for a pre-provenance snapshot", out.SchemaVersion)
	}
	if len(out.Unknown) != 1 || out.Unknown[0].Path != file {
		t.Errorf("Unknown = %+v, want the legacy entry", out.Unknown)
	}
	if len(out.Stable) != 0 {
		t.Errorf("Stable = %+v — an undeclared file must not read as certified", out.Stable)
	}
}

// The regression guard for the reasoning, not just the code.
//
// Parlay guarantees FUNCTIONAL determinism, not byte-identity: two codegen
// runs over the same inputs may legitimately produce different bytes. So any
// design that inferred provenance from hash stability — "we ran codegen and
// the hash changed, so it was us" — would misclassify every legitimate
// regeneration. This test fails the moment someone "optimises" by comparing
// hashes across emissions.
func TestReEmissionWithDifferentBytesIsNotAHandEdit(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	file := filepath.Join(sourceRoot, "widget.go")
	writeMarkedFile(t, file, "my-feature", "widget", "func Widget() { return 1 }")

	first, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, declare(t, cfg, file), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Codegen runs again and emits materially different bytes — a different
	// helper name, a reordered import, a reformatted line. All within
	// contract.
	if err := os.WriteFile(file, []byte(`// parlay-feature: my-feature
// parlay-component: widget
func Widget() { const answer = 1; return answer }
`), 0644); err != nil {
		t.Fatal(err)
	}

	second, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, declare(t, cfg, file), first)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Files[file].Provenance; got != ProvenanceGenerated {
		t.Fatalf("a declared re-emission is generated whatever the bytes, got %q", got)
	}
	if second.Files[file].Hash == first.Files[file].Hash {
		t.Fatal("test is not exercising its own premise: the bytes did not change")
	}

	if err := saveProjectCodeHashesForTest(cfg, second); err != nil {
		t.Fatal(err)
	}
	out, err := computeProjectVerifyOutput(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Adopted) != 0 {
		t.Errorf("a re-emission must never be reported as adopted: %+v", out.Adopted)
	}
	if len(out.Stable) != 1 {
		t.Errorf("Stable = %+v, want the re-emitted file", out.Stable)
	}
}

// A file nobody emitted and nobody touched keeps whatever verdict it already
// had. Re-deciding it every run would flip a file adopted three runs ago back
// to generated the moment its neighbours were rebuilt.
func TestUnchangedUndeclaredFileKeepsItsEarlierVerdict(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	file := filepath.Join(sourceRoot, "widget.go")
	writeMarkedFile(t, file, "my-feature", "widget", "func Widget() {}")

	first, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, declare(t, cfg, file), nil)
	if err != nil {
		t.Fatal(err)
	}
	// A later run touches nothing and declares nothing.
	second, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, declare(t, cfg), first)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Files[file].Provenance; got != ProvenanceGenerated {
		t.Errorf("an untouched generated file stays generated, got %q", got)
	}
	if second.Files[file].EmittedAt != first.Files[file].EmittedAt {
		t.Error("carrying an entry forward must not restamp when it was emitted")
	}
}

// No manifest at all is its own state: the run did not say, which is
// different from the run having written nothing. Everything reads unknown.
func TestNoManifestMeansUnknownNotGenerated(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	file := filepath.Join(sourceRoot, "widget.go")
	writeMarkedFile(t, file, "my-feature", "widget", "func Widget() {}")

	decl, path, err := loadEmittedManifest(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if decl != nil || path != "" {
		t.Fatalf("absent manifest should read as nil, got %v / %q", decl, path)
	}

	hashes, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, decl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := hashes.Files[file].Provenance; got != ProvenanceUnknown {
		t.Errorf("provenance = %q, want unknown", got)
	}
}

// The manifest is normalized the same way check-write-set normalizes its
// paths, so a "./"-prefixed line still matches the scanner's view.
func TestManifestPathsAreNormalized(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	file := filepath.Join(sourceRoot, "widget.go")
	writeMarkedFile(t, file, "my-feature", "widget", "func Widget() {}")

	// Blank lines and a comment must be ignored, not treated as paths.
	dirPath := cfg.ProjectBuildPath()
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	body := "# written by generate-code\n\n" + filepath.Join(sourceRoot, ".", "widget.go") + "\n\n"
	if err := os.WriteFile(filepath.Join(dirPath, DefaultEmittedManifest), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	decl, _, err := loadEmittedManifest(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(decl.Paths) != 1 {
		t.Fatalf("comments and blanks must not become paths: %v", decl.Paths)
	}

	hashes, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, decl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := hashes.Files[file].Provenance; got != ProvenanceGenerated {
		t.Errorf("provenance = %q, want generated", got)
	}
}

// saveProjectCodeHashesForTest writes a snapshot to the project sidecar,
// which is where computeProjectVerifyOutput reads from.
func saveProjectCodeHashesForTest(cfg *config.Context, h *CodeHashes) error {
	path := projectCodeHashesPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := marshalCodeHashes(h)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
