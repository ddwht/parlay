package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"gopkg.in/yaml.v3"
)

// runProjectSave drives the project-level save with stderr captured, the
// path most WP1 guards live on. Returns the result, the WARN/error text
// written to stderr, and the save error.
func runProjectSave(t *testing.T, cfg *config.Context, sourceRoot string) (*projectSaveResult, string, error) {
	t.Helper()
	cmd := testCommandWithContext(t, cfg)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetOut(&bytes.Buffer{})
	res, err := saveProjectBuildState(cmd, cfg, sourceRoot)
	return res, errBuf.String(), err
}

// authorFeature writes a minimal feature so discoverFeatures finds it and
// stage 1 can baseline it.
func authorFeature(t *testing.T, cfg *config.Context, slug, goal string) {
	t.Helper()
	dir := cfg.FeaturePath(slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "## " + goal + "\n\n**Goal**: " + goal + "\n**Persona**: User\n"
	if err := os.WriteFile(filepath.Join(dir, "intents.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestSaveBuildState_HappyPath writes a feature with intents/dialogs/surface
// plus a marker-tagged source file, runs saveBuildState, and verifies both
// .baseline.yaml and .code-hashes.yaml exist with the expected content.
func TestSaveBuildState_HappyPath(t *testing.T) {
	dir := setupTestDir(t)

	// Author a minimal feature.
	featureDir := testContext(t).FeaturePath("my-feature")
	os.MkdirAll(featureDir, 0755)
	intents := `## Do Something

**Goal**: Do the thing
**Persona**: User
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	// Drop a marker-tagged source file in the source root.
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	writeMarkedFile(t, filepath.Join(sourceRoot, "do.go"),
		"my-feature", "do-something", "func DoSomething() {}")

	err := saveBuildStateForFeature(testContext(t), "my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Both files should exist on disk
	if _, err := os.Stat(baselinePath(testContext(t), "my-feature")); err != nil {
		t.Errorf("baseline file missing: %v", err)
	}
	if _, err := os.Stat(codeHashesPath(testContext(t), "my-feature")); err != nil {
		t.Errorf("code-hashes file missing: %v", err)
	}

	// Content should round-trip via the load helpers.
	blData, _ := os.ReadFile(baselinePath(testContext(t), "my-feature"))
	var loaded Baseline
	if err := yaml.Unmarshal(blData, &loaded); err != nil {
		t.Fatalf("baseline yaml invalid: %v", err)
	}
	if _, ok := loaded.Intents["do-something"]; !ok {
		t.Error("baseline missing do-something intent hash")
	}
	if loaded.Sources == nil || loaded.Sources.Intents["do-something"] == "" {
		t.Error("baseline.Sources missing do-something content hash")
	}

	hashes, err := loadCodeHashes(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if hashes == nil {
		t.Fatal("loaded code-hashes is nil")
	}
	if len(hashes.Files) != 1 {
		t.Errorf("Files count = %d, want 1", len(hashes.Files))
	}
}

// TestSaveBuildState_NoTempFilesLeftBehind verifies that writeFileAtomic
// cleans up its scratch files on success — no .tmp-* lingering in the
// build directory after a successful save.
func TestSaveBuildState_NoTempFilesLeftBehind(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := testContext(t).FeaturePath("clean-feature")
	os.MkdirAll(featureDir, 0755)
	os.WriteFile(filepath.Join(featureDir, "intents.md"),
		[]byte("## X\n\n**Goal**: x\n**Persona**: u\n"), 0644)

	sourceRoot := filepath.Join(dir, "cmd", "clean-feature")
	os.MkdirAll(sourceRoot, 0755)

	if err := saveBuildStateForFeature(testContext(t), "clean-feature", sourceRoot); err != nil {
		t.Fatal(err)
	}

	// Walk the build dir and assert no .tmp-* files remain.
	buildDir := testContext(t).BuildPath("clean-feature")
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}

// TestSaveBuildState_OverwritesPreviousState writes the build state twice
// in a row and confirms the second write replaces the first cleanly (no
// stale data, both files reflect the second invocation's input).
func TestSaveBuildState_OverwritesPreviousState(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := testContext(t).FeaturePath("twice-feature")
	os.MkdirAll(featureDir, 0755)
	os.WriteFile(filepath.Join(featureDir, "intents.md"),
		[]byte("## First\n\n**Goal**: first\n**Persona**: u\n"), 0644)

	sourceRoot := filepath.Join(dir, "cmd", "twice-feature")
	writeMarkedFile(t, filepath.Join(sourceRoot, "first.go"),
		"twice-feature", "first-comp", "package twice")

	if err := saveBuildStateForFeature(testContext(t), "twice-feature", sourceRoot); err != nil {
		t.Fatal(err)
	}

	// Modify the feature: replace the intent and replace the marker file.
	os.WriteFile(filepath.Join(featureDir, "intents.md"),
		[]byte("## Second\n\n**Goal**: second\n**Persona**: u\n"), 0644)
	os.Remove(filepath.Join(sourceRoot, "first.go"))
	writeMarkedFile(t, filepath.Join(sourceRoot, "second.go"),
		"twice-feature", "second-comp", "package twice")

	if err := saveBuildStateForFeature(testContext(t), "twice-feature", sourceRoot); err != nil {
		t.Fatal(err)
	}

	// Baseline must reflect the new intent only.
	var loaded Baseline
	blData, _ := os.ReadFile(baselinePath(testContext(t), "twice-feature"))
	yaml.Unmarshal(blData, &loaded)
	if _, ok := loaded.Intents["first"]; ok {
		t.Error("baseline still contains stale 'first' intent after overwrite")
	}
	if _, ok := loaded.Intents["second"]; !ok {
		t.Error("baseline missing 'second' intent after overwrite")
	}

	// Code hashes must reflect the new file only.
	hashes, _ := loadCodeHashes(testContext(t), "twice-feature")
	if _, ok := hashes.Files[filepath.Join(sourceRoot, "first.go")]; ok {
		t.Error("code-hashes still contains stale first.go entry")
	}
	if _, ok := hashes.Files[filepath.Join(sourceRoot, "second.go")]; !ok {
		t.Error("code-hashes missing second.go entry after overwrite")
	}
}

// TestSaveBuildState_MissingIntentsFails confirms that saveBuildState refuses
// to commit when the source files can't be parsed — there's nothing to commit.
func TestSaveBuildState_MissingIntentsFails(t *testing.T) {
	setupTestDir(t)
	// No feature directory created at all.

	err := saveBuildStateForFeature(testContext(t), "nonexistent", "cmd/nonexistent")
	if err == nil {
		t.Fatal("expected error when feature directory is missing, got nil")
	}
	if !strings.Contains(err.Error(), "compute baseline") {
		t.Errorf("error = %v, want 'compute baseline' wrapped error", err)
	}
}

// TestSaveBuildState_EmptyDialogsStubBaselines confirms that a dialogs.md
// that exists on disk but parses to zero dialog blocks (the intentional
// terminal stub of a CLI/backend feature — "no interactive dialog turns")
// baselines successfully, contributing no dialogs source-hash section. This
// keeps buildBaseline consistent with check-readiness and build-feature,
// which treat dialogs as recommended-but-not-required; refusing here would
// strand a legitimate CLI feature at the final save-build-state step.
func TestSaveBuildState_EmptyDialogsStubBaselines(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := testContext(t).FeaturePath("stub-dialogs")
	os.MkdirAll(featureDir, 0755)
	os.WriteFile(filepath.Join(featureDir, "intents.md"),
		[]byte("## Do Something\n\n**Goal**: Do the thing\n**Persona**: User\n"), 0644)
	// A dialogs.md that exists but has no dialog blocks — header only,
	// the form an intentional CLI/backend stub takes.
	os.WriteFile(filepath.Join(featureDir, "dialogs.md"),
		[]byte("# Stub Dialogs — Dialogs\n\n_CLI/backend feature — no interactive dialog turns._\n\n---\n"), 0644)

	sourceRoot := filepath.Join(dir, "cmd", "stub-dialogs")
	os.MkdirAll(sourceRoot, 0755)

	if err := saveBuildStateForFeature(testContext(t), "stub-dialogs", sourceRoot); err != nil {
		t.Fatalf("expected success for a zero-block CLI dialogs stub, got: %v", err)
	}

	blPath := baselinePath(testContext(t), "stub-dialogs")
	if _, statErr := os.Stat(blPath); statErr != nil {
		t.Fatalf("baseline.yaml must be written for a CLI feature with a zero-block dialogs stub: %v", statErr)
	}

	// The dialogs source-hash section is omitted (nothing to hash), exactly
	// as it would be for a feature whose dialogs.md is simply absent.
	blData, _ := os.ReadFile(blPath)
	var loaded Baseline
	if err := yaml.Unmarshal(blData, &loaded); err != nil {
		t.Fatalf("baseline yaml invalid: %v", err)
	}
	if loaded.Sources != nil && len(loaded.Sources.Dialogs) != 0 {
		t.Errorf("dialogs source-hash section = %v, want empty for a zero-block stub", loaded.Sources.Dialogs)
	}
}

// TestSaveBuildState_MissingDialogsStillSucceeds confirms the guard is
// specific to an empty-but-present dialogs.md — a feature that simply
// hasn't authored dialogs.md yet (file absent) still baselines fine, since
// the dialogs source-hash section is optional.
func TestSaveBuildState_MissingDialogsStillSucceeds(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := testContext(t).FeaturePath("no-dialogs-yet")
	os.MkdirAll(featureDir, 0755)
	os.WriteFile(filepath.Join(featureDir, "intents.md"),
		[]byte("## Do Something\n\n**Goal**: Do the thing\n**Persona**: User\n"), 0644)
	// No dialogs.md written at all.

	sourceRoot := filepath.Join(dir, "cmd", "no-dialogs-yet")
	os.MkdirAll(sourceRoot, 0755)

	if err := saveBuildStateForFeature(testContext(t), "no-dialogs-yet", sourceRoot); err != nil {
		t.Fatalf("expected success when dialogs.md is simply absent, got: %v", err)
	}
	if _, statErr := os.Stat(baselinePath(testContext(t), "no-dialogs-yet")); statErr != nil {
		t.Errorf("baseline.yaml missing: %v", statErr)
	}
}

// TestSaveBuildState_EmptyIntentsFails confirms the same empty-artifact
// guard applies to intents.md: a file that exists but has zero intent
// blocks refuses to baseline rather than committing an empty Intents map.
func TestSaveBuildState_EmptyIntentsFails(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := testContext(t).FeaturePath("stub-intents")
	os.MkdirAll(featureDir, 0755)
	os.WriteFile(filepath.Join(featureDir, "intents.md"),
		[]byte("# Stub Intents\n\n> nothing authored yet\n"), 0644)

	sourceRoot := filepath.Join(dir, "cmd", "stub-intents")
	os.MkdirAll(sourceRoot, 0755)

	err := saveBuildStateForFeature(testContext(t), "stub-intents", sourceRoot)
	if err == nil {
		t.Fatal("expected error for empty intents.md stub, got nil")
	}
	if !strings.Contains(err.Error(), "intents.md") {
		t.Errorf("error = %v, want it to name intents.md", err)
	}
}

// TestWriteFileAtomic_RoundTrip writes then reads bytes through writeFileAtomic
// and confirms they're identical.
func TestWriteFileAtomic_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	want := []byte("hello world\nmultiple lines\n")

	if err := writeFileAtomic(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("read = %q, want %q", got, want)
	}

	// Mode should be 0644, not the 0600 that CreateTemp produces.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

// TestWriteFileAtomic_NoTempFilesOnSuccess confirms that the rename consumes
// the temp file (no .tmp-* lingering after a successful write).
func TestWriteFileAtomic_NoTempFilesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	if err := writeFileAtomic(path, []byte("data")); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("temp file left after success: %s", entry.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("dir entries = %d, want 1 (out.txt only)", len(entries))
	}
}

// TestWriteFileAtomic_PreservesPreviousOnFailure writes content to a path,
// then simulates a write failure on a second invocation by making the
// destination directory read-only after the first write. The previous
// content must remain intact (no partial overwrite).
//
// This test is skipped on platforms where mode bits don't enforce write
// permissions for the running user (e.g., root on POSIX).
func TestWriteFileAtomic_PreservesPreviousOnFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; chmod write-protection is bypassed")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	original := []byte("original content\n")

	if err := writeFileAtomic(path, original); err != nil {
		t.Fatal(err)
	}

	// Make the directory read+exec only (no write). Subsequent CreateTemp
	// in this directory should fail.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	err := writeFileAtomic(path, []byte("new content that should not land\n"))
	if err == nil {
		t.Fatal("expected error when temp file creation fails, got nil")
	}

	// Restore write so we can read the original.
	os.Chmod(dir, 0755)
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("destination corrupted: got %q, want %q", got, original)
	}
}

// --- WP1: fail-loud state boundary + write-if-changed ---

// An explicit --emitted path that does not exist is a hard error, not an
// empty "emitted nothing" declaration: it is overwhelmingly a re-run against
// a manifest a previous save already consumed, and reading it as silence
// would downgrade a tracked run to look exactly like the feature working.
func TestSaveBuildState_ExplicitMissingManifestErrors(t *testing.T) {
	setupTestDir(t)
	cfg := testContext(t)

	_, _, err := loadEmittedManifest(cfg, filepath.Join(cfg.ProjectBuildPath(), "never-written.emitted"))
	if err == nil {
		t.Fatal("explicit --emitted path that does not exist must error, not read as an empty declaration")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should name the missing path, got: %v", err)
	}
	if !strings.Contains(err.Error(), consumedManifestSuffix) {
		t.Errorf("error should point at the consumed manifest (%s), so a re-run diagnosis is one ls away, got: %v", consumedManifestSuffix, err)
	}

	// The no-flag case is unchanged: absent default manifest reads as nil
	// (no declaration), not an error.
	decl, _, err := loadEmittedManifest(cfg, "")
	if err != nil {
		t.Fatalf("absent default manifest must not error, got %v", err)
	}
	if decl != nil {
		t.Errorf("absent default manifest must read as nil (no declaration), got %v", decl)
	}
}

// A --source-root under which none of the previously-tracked files sit means
// the invocation disagrees with the last about where generated code lives —
// a mis-guessed flag, refused before it commits a snapshot rooted elsewhere.
func TestSaveBuildState_SourceRootPrefixMismatchErrors(t *testing.T) {
	prev := &CodeHashes{Files: map[string]CodeHashEntry{
		"/abs/cmd/widget/do.go": {Component: "widget", Hash: "h1"},
		"/abs/cmd/gadget/go.go": {Component: "gadget", Hash: "h2"},
	}}

	if err := checkSourceRootMatchesSnapshot(prev, "cmd/widget"); err == nil {
		t.Error("a relative root disjoint from every absolute stored key must error")
	} else if !strings.Contains(err.Error(), "source-root") {
		t.Errorf("error should name --source-root, got: %v", err)
	}

	// The true root the snapshot was taken under is accepted.
	if err := checkSourceRootMatchesSnapshot(prev, "/abs/cmd"); err != nil {
		t.Errorf("the matching root must be accepted, got %v", err)
	}
	// A nested-narrower root still has at least one file under it — that is a
	// scope shrink for the narrowing check to judge, not a shape mismatch.
	if err := checkSourceRootMatchesSnapshot(prev, "/abs/cmd/widget"); err != nil {
		t.Errorf("a nested-narrower root must pass the shape check, got %v", err)
	}

	// First-ever save: nothing to compare, falls back to convention.
	if err := checkSourceRootMatchesSnapshot(nil, "cmd/widget"); err != nil {
		t.Errorf("first-ever save (nil snapshot) must not be blocked, got %v", err)
	}
	if err := checkSourceRootMatchesSnapshot(&CodeHashes{Files: map[string]CodeHashEntry{}}, "cmd/widget"); err != nil {
		t.Errorf("empty snapshot must not be blocked, got %v", err)
	}
}

// A narrower --source-root that drops previously-tracked files still on disk
// refuses without --allow-narrowing (mirroring --strict for adopted files),
// and proceeds with it.
func TestSaveBuildState_NarrowingRefusesWithoutFlag(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "my-feature", "Do")

	root := filepath.Join(dir, "cmd")
	writeMarkedFile(t, filepath.Join(root, "a", "x.go"), "my-feature", "a", "package a")
	writeMarkedFile(t, filepath.Join(root, "b", "y.go"), "my-feature", "b", "package b")

	// First save under the wide root tracks both files.
	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatalf("first save must succeed: %v", err)
	}

	// Second save under a nested-narrower root drops b/y.go, which still
	// exists — refuse without the flag.
	narrow := filepath.Join(root, "a")
	_, stderr, err := runProjectSave(t, cfg, narrow)
	if err == nil {
		t.Fatal("narrowing --source-root must refuse without --allow-narrowing")
	}
	if !strings.Contains(err.Error(), "allow-narrowing") {
		t.Errorf("refusal should name --allow-narrowing as the escape hatch, got: %v", err)
	}
	if !strings.Contains(stderr, "y.go") {
		t.Errorf("the dropped file should be listed on stderr, got: %q", stderr)
	}

	// With the flag, the same narrowing save proceeds.
	saveBuildStateAllowNarrowing = true
	t.Cleanup(func() { saveBuildStateAllowNarrowing = false })
	if _, _, err := runProjectSave(t, cfg, narrow); err != nil {
		t.Fatalf("narrowing save with --allow-narrowing must proceed: %v", err)
	}
}

// The F2 regression: re-saving an unedited feature must not rewrite its
// baseline with nothing but a fresh timestamp. Proven deterministically by
// stamping the on-disk baseline with a distinctly old generated-at and
// showing a no-change save leaves those exact bytes untouched.
func TestSaveBuildState_UnchangedBaselineByteIdentical(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "my-feature", "Do")
	root := filepath.Join(dir, "cmd")
	writeMarkedFile(t, filepath.Join(root, "do.go"), "my-feature", "do", "package do")

	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatal(err)
	}

	// Rewrite the on-disk baseline with an unmistakably old generated-at, so
	// a later overwrite (if write-if-changed were broken) would be visible.
	blPath := baselinePath(cfg, "my-feature")
	data, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	var bl Baseline
	if err := yaml.Unmarshal(data, &bl); err != nil {
		t.Fatal(err)
	}
	bl.GeneratedAt = "2000-01-01T00:00:00Z"
	stamped, err := marshalBaseline(&bl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blPath, stamped, 0644); err != nil {
		t.Fatal(err)
	}

	// Re-save with no source change. Write-if-changed must skip the baseline
	// write, leaving the old-stamped bytes exactly as they are.
	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stamped, after) {
		t.Errorf("unchanged feature's baseline was rewritten by a no-change save:\n before: %s\n after:  %s", stamped, after)
	}
}

// The other half of write-if-changed: a real content change still writes.
func TestSaveBuildState_ChangedFeatureStillWritten(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "my-feature", "Do")
	root := filepath.Join(dir, "cmd")
	writeMarkedFile(t, filepath.Join(root, "do.go"), "my-feature", "do", "package do")

	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatal(err)
	}
	// Stamp old, then change the feature's intent so content genuinely moves.
	blPath := baselinePath(cfg, "my-feature")
	data, _ := os.ReadFile(blPath)
	var bl Baseline
	yaml.Unmarshal(data, &bl)
	bl.GeneratedAt = "2000-01-01T00:00:00Z"
	stamped, _ := marshalBaseline(&bl)
	os.WriteFile(blPath, stamped, 0644)

	os.WriteFile(filepath.Join(cfg.FeaturePath("my-feature"), "intents.md"),
		[]byte("## Do It Differently\n\n**Goal**: a genuinely new goal\n**Persona**: User\n"), 0644)

	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(blPath)
	if bytes.Equal(stamped, after) {
		t.Fatal("a changed feature's baseline must be rewritten, but the old-stamped bytes survived")
	}
	var reloaded Baseline
	if err := yaml.Unmarshal(after, &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.GeneratedAt == "2000-01-01T00:00:00Z" {
		t.Error("a rewrite must refresh generated-at, but the old stamp survived")
	}
	if _, ok := reloaded.Intents["do-it-differently"]; !ok {
		t.Errorf("rewritten baseline missing the new intent, has: %v", reloaded.Intents)
	}
}

// Features stage 1 cannot baseline (no parseable intents.md) are collected
// and reported, not dropped silently.
func TestSaveBuildState_SkippedFeaturesReported(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "good", "Do")
	// A feature dir with an empty intents.md: discovered, but buildBaseline
	// refuses the empty artifact.
	emptyDir := cfg.FeaturePath("empty")
	os.MkdirAll(emptyDir, 0755)
	os.WriteFile(filepath.Join(emptyDir, "intents.md"), []byte(""), 0644)

	root := filepath.Join(dir, "cmd")
	writeMarkedFile(t, filepath.Join(root, "do.go"), "good", "do", "package do")

	res, stderr, err := runProjectSave(t, cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Slug != "empty" {
		t.Fatalf("expected 'empty' reported as skipped, got %v", res.Skipped)
	}
	if res.Skipped[0].Reason == "" {
		t.Error("a skipped feature must carry a reason")
	}
	// The good feature still baselined.
	if _, err := os.Stat(baselinePath(cfg, "good")); err != nil {
		t.Errorf("the good feature should have a baseline: %v", err)
	}
	// runSaveBuildState surfaces the summary; saveProjectBuildState itself
	// records it on the result. Stderr here carries no skip line (the CLI
	// entrypoint prints it), so assert on the result rather than stderr.
	_ = stderr
}

// The BP6 regression. A partial save advances the baseline ONLY for the
// feature it emitted; a feature whose source drifted but which this run did
// not regenerate keeps its baseline and still reports dirty afterward. Before
// WP6, stage 1 re-derived every feature's baseline from current source, so a
// refine on feature A silently re-blessed feature B and cleared B's real
// "this is behind" signal — a false-stable verdict worse than churn.
func TestSaveBuildState_PartialBlessesOnlyEmittedFeature_BP6(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "feat-a", "Alpha")
	authorFeature(t, cfg, "feat-b", "Beta")

	root := filepath.Join(dir, "cmd")
	fileA := filepath.Join(root, "a", "a.go")
	fileB := filepath.Join(root, "b", "b.go")
	writeMarkedFile(t, fileA, "feat-a", "a", "package a")
	writeMarkedFile(t, fileB, "feat-b", "b", "package b")

	// A full save blesses both features at one instant.
	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatalf("initial full save: %v", err)
	}
	if d, err := detectDrift(cfg, "feat-b", cfg.FeaturePath("feat-b")); err != nil {
		t.Fatal(err)
	} else if d.HasDrift {
		t.Fatalf("feat-b must be clean right after the full save, got drift: %+v", d)
	}

	// Both features' sources now drift. The refine-shaped workload regenerates
	// and emits only feat-a; feat-b is left behind.
	os.WriteFile(filepath.Join(cfg.FeaturePath("feat-a"), "intents.md"),
		[]byte("## Alpha Prime\n\n**Goal**: alpha moved on\n**Persona**: User\n"), 0644)
	os.WriteFile(filepath.Join(cfg.FeaturePath("feat-b"), "intents.md"),
		[]byte("## Beta Prime\n\n**Goal**: beta moved on\n**Persona**: User\n"), 0644)

	// Partial save declaring ONLY feat-a's file.
	emittedPath := filepath.Join(cfg.ProjectBuildPath(), DefaultEmittedManifest)
	if err := os.MkdirAll(filepath.Dir(emittedPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(emittedPath, []byte(fileA+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	saveBuildStatePartial = true
	saveBuildStateEmitted = emittedPath
	t.Cleanup(func() { saveBuildStatePartial = false; saveBuildStateEmitted = "" })

	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatalf("partial save: %v", err)
	}

	// The BP6 property: feat-b was not emitted, so its baseline was left
	// untouched and its source drift survives the save.
	dB, err := detectDrift(cfg, "feat-b", cfg.FeaturePath("feat-b"))
	if err != nil {
		t.Fatal(err)
	}
	if !dB.HasDrift {
		t.Error("BP6: an un-emitted feature whose source drifted must still report dirty after a partial save — its baseline was wrongly advanced, minting a false-stable verdict")
	}

	// The other half: the emitted feature IS re-blessed and clean again.
	dA, err := detectDrift(cfg, "feat-a", cfg.FeaturePath("feat-a"))
	if err != nil {
		t.Fatal(err)
	}
	if dA.HasDrift {
		t.Errorf("the emitted feature must advance to clean, got drift: %+v", dA)
	}

	// The project baseline records exactly what the partial save blessed.
	var pbl ProjectBaseline
	if data, err := os.ReadFile(projectBaselinePath(cfg)); err != nil {
		t.Fatal(err)
	} else if err := yaml.Unmarshal(data, &pbl); err != nil {
		t.Fatal(err)
	}
	if len(pbl.Emitted) != 1 || pbl.Emitted[0] != "feat-a" {
		t.Errorf("project baseline emitted = %v, want [feat-a]", pbl.Emitted)
	}
}

// WP6.1 finding 1 (the headline false-stable). A --partial save must NOT
// advance the project-level merged-section baseline, because those hashes are
// a whole-project instant: the `blueprint` section covers a cross-cutting
// artifact no single feature owns, and it lives ONLY in the project merged
// baseline (the per-feature hashers never read the blueprint). Before WP6.1,
// stage 2 recomputed and stored the merged sections on every save, partial
// included, so an unrelated refine on feat-a would absorb a blueprint change
// and flip `parlay diff`'s project verdict from `changed` to `stable` with no
// other detector to catch it — a false-stable. The fix carries the stored
// merged sections forward verbatim under --partial.
func TestWP6_PartialSaveClearsBlueprintDrift_FALSESTABLE(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "feat-a", "Alpha")
	authorFeature(t, cfg, "feat-b", "Beta")

	root := filepath.Join(dir, "cmd")
	fileA := filepath.Join(root, "a", "a.go")
	fileB := filepath.Join(root, "b", "b.go")
	writeMarkedFile(t, fileA, "feat-a", "a", "package a")
	writeMarkedFile(t, fileB, "feat-b", "b", "package b")

	// A blueprint exists and is part of the project-wide merged instant.
	if err := os.MkdirAll(filepath.Dir(cfg.BlueprintPath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BlueprintPath(), []byte("version: 1\nshells: [app]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Full save blesses everything, storing the blueprint hash in the project
	// merged sections.
	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatalf("initial full save: %v", err)
	}
	features, err := discoverFeatures(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if v := computeProjectSectionDiff(cfg, features)["blueprint"]; v != "stable" {
		t.Fatalf("blueprint must be stable right after the full save, got %q", v)
	}

	// The blueprint changes (cross-cutting code is now behind). The project
	// diff must see it.
	if err := os.WriteFile(cfg.BlueprintPath(), []byte("version: 2\nshells: [app, admin]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if v := computeProjectSectionDiff(cfg, features)["blueprint"]; v != "changed" {
		t.Fatalf("blueprint must report changed after it is edited, got %q", v)
	}

	// An unrelated partial refine on feat-a, which touched nothing about the
	// blueprint, emits only feat-a's file.
	emittedPath := filepath.Join(cfg.ProjectBuildPath(), DefaultEmittedManifest)
	if err := os.WriteFile(emittedPath, []byte(fileA+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	saveBuildStatePartial = true
	saveBuildStateEmitted = emittedPath
	t.Cleanup(func() { saveBuildStatePartial = false; saveBuildStateEmitted = "" })
	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatalf("partial save: %v", err)
	}

	// The false-stable: after the partial save the blueprint drift must STILL
	// be visible. A partial run has no standing to bless a cross-cutting change
	// it never addressed.
	if v := computeProjectSectionDiff(cfg, features)["blueprint"]; v == "stable" {
		t.Error("FALSE-STABLE: a --partial save that never touched the blueprint cleared its project-level drift; blueprint reported stable when it must stay changed until a full save")
	}
}

// WP6.1 finding 2 (project-aggregate false-stable). If an un-emitted feature's
// buildfile changes (re-planned, code not regenerated) and then a --partial
// save emits only a different feature, stage 2 must not fold the un-emitted
// feature's current buildfile into the blessed merged sections — doing so
// masks its buildfile drift at the project view. Same root cause as finding 1:
// stage 2 is a whole-project instant a partial run cannot advance.
func TestWP6_PartialSaveMasksUnemittedBuildfileAtProjectLevel(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "feat-a", "Alpha")
	authorFeature(t, cfg, "feat-b", "Beta")

	root := filepath.Join(dir, "cmd")
	fileA := filepath.Join(root, "a", "a.go")
	fileB := filepath.Join(root, "b", "b.go")
	writeMarkedFile(t, fileA, "feat-a", "a", "package a")
	writeMarkedFile(t, fileB, "feat-b", "b", "package b")

	// Both features have buildfiles with a models section, which the project
	// merged-section hash concatenates across features.
	writeBuildfileModels := func(slug, models string) {
		bfDir := cfg.BuildPath(slug)
		if err := os.MkdirAll(bfDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bfDir, "buildfile.yaml"), []byte(models), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeBuildfileModels("feat-a", "models:\n  - name: Alpha\n")
	writeBuildfileModels("feat-b", "models:\n  - name: Beta\n")

	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatalf("initial full save: %v", err)
	}
	features, err := discoverFeatures(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if v := computeProjectSectionDiff(cfg, features)["models"]; v != "stable" {
		t.Fatalf("models must be stable right after the full save, got %q", v)
	}

	// feat-b's buildfile is re-planned; its code is not regenerated.
	writeBuildfileModels("feat-b", "models:\n  - name: BetaPrime\n  - name: BetaExtra\n")
	if v := computeProjectSectionDiff(cfg, features)["models"]; v != "changed" {
		t.Fatalf("models must report changed after feat-b's buildfile moves, got %q", v)
	}

	// Partial save emits only feat-a.
	emittedPath := filepath.Join(cfg.ProjectBuildPath(), DefaultEmittedManifest)
	if err := os.WriteFile(emittedPath, []byte(fileA+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	saveBuildStatePartial = true
	saveBuildStateEmitted = emittedPath
	t.Cleanup(func() { saveBuildStatePartial = false; saveBuildStateEmitted = "" })
	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatalf("partial save: %v", err)
	}

	// feat-b's buildfile drift must survive at the project view — the partial
	// save that emitted only feat-a must not bless it.
	if v := computeProjectSectionDiff(cfg, features)["models"]; v == "stable" {
		t.Error("FALSE-STABLE: a --partial save emitting only feat-a folded feat-b's re-planned buildfile into the blessed merged sections; models reported stable when feat-b's buildfile is still ahead of its code")
	}
}

// A non-partial (full) save records no `emitted:` scope on the project
// baseline: it blessed every feature at one instant, so there is nothing
// narrower to audit. Guards the honest-record contract — an empty list would
// wrongly read as "this save blessed no features".
func TestSaveBuildState_FullSaveRecordsNoEmittedScope(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	authorFeature(t, cfg, "solo", "Solo")
	root := filepath.Join(dir, "cmd")
	writeMarkedFile(t, filepath.Join(root, "s.go"), "solo", "s", "package s")

	if _, _, err := runProjectSave(t, cfg, root); err != nil {
		t.Fatal(err)
	}
	var pbl ProjectBaseline
	data, _ := os.ReadFile(projectBaselinePath(cfg))
	if err := yaml.Unmarshal(data, &pbl); err != nil {
		t.Fatal(err)
	}
	if pbl.Emitted != nil {
		t.Errorf("a full save must record no emitted scope, got %v", pbl.Emitted)
	}
	// And the field is dropped from the serialized file, not written empty.
	if strings.Contains(string(data), "emitted:") {
		t.Errorf("emitted: must be omitted from a full save's project baseline, got:\n%s", data)
	}
}
