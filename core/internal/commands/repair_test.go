// parlay-feature: repair-project-state
// parlay-component: RepairReport
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------
// yaml.Node surgical editing — removeYAMLField / setYAMLField.
// ---------------------------------------------------------------------

func TestRemoveYAMLField_PreservesCommentsAndOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "# top-level comment\nai-agent: Claude Code # inline comment\nsdd-framework: None\nprototype-framework: Go CLI\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	if err := removeYAMLField(path, "ai-agent"); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "ai-agent") {
		t.Errorf("expected ai-agent removed, got:\n%s", got)
	}
	if !strings.Contains(got, "# top-level comment") {
		t.Errorf("expected top-level comment preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "sdd-framework: None") || !strings.Contains(got, "prototype-framework: Go CLI") {
		t.Errorf("expected other fields preserved, got:\n%s", got)
	}
	// Order: sdd-framework must still precede prototype-framework.
	if strings.Index(got, "sdd-framework") > strings.Index(got, "prototype-framework") {
		t.Errorf("expected key order preserved, got:\n%s", got)
	}
}

func TestRemoveYAMLField_MissingFieldIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "sdd-framework: None\n"
	os.WriteFile(path, []byte(body), 0644)

	if err := removeYAMLField(path, "ai-agent"); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	if string(out) != body {
		t.Errorf("expected file untouched when field absent, got:\n%s", out)
	}
}

func TestSetYAMLField_PreservesCommentsAndOtherFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "# a comment\nsdd-framework: None\nprototype-framework: Go CLI\n"
	os.WriteFile(path, []byte(body), 0644)

	if err := setYAMLField(path, "ai-agent", "Claude Code"); err != nil {
		t.Fatal(err)
	}

	out, _ := os.ReadFile(path)
	got := string(out)
	if !strings.Contains(got, "# a comment") {
		t.Errorf("expected comment preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "ai-agent: Claude Code") {
		t.Errorf("expected ai-agent set, got:\n%s", got)
	}
	if !strings.Contains(got, "sdd-framework: None") || !strings.Contains(got, "prototype-framework: Go CLI") {
		t.Errorf("expected existing fields preserved, got:\n%s", got)
	}
}

func TestSetYAMLField_UpdatesExistingValueInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "ai-agent: Cursor\nsdd-framework: None\n"
	os.WriteFile(path, []byte(body), 0644)

	if err := setYAMLField(path, "ai-agent", "Claude Code"); err != nil {
		t.Fatal(err)
	}

	var cfg config.ProjectConfig
	out, _ := os.ReadFile(path)
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AIAgent != "Claude Code" {
		t.Errorf("AIAgent = %q, want Claude Code", cfg.AIAgent)
	}
	if strings.Count(string(out), "ai-agent") != 1 {
		t.Errorf("expected exactly one ai-agent key, got:\n%s", out)
	}
}

func TestSetYAMLField_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	if err := setYAMLField(path, "ai-agent", "Claude Code"); err != nil {
		t.Fatal(err)
	}

	var cfg config.ProjectConfig
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AIAgent != "Claude Code" {
		t.Errorf("AIAgent = %q, want Claude Code", cfg.AIAgent)
	}
}

// ---------------------------------------------------------------------
// Pure fix-application functions.
// ---------------------------------------------------------------------

func TestApplyFieldRemovals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("ai-agent: Claude Code\nsdd-framework: None\n"), 0644)

	err := applyFieldRemovals([]config.FieldRemoval{{File: path, Field: "ai-agent"}})
	if err != nil {
		t.Fatal(err)
	}
	if config.HasAIAgentField(path) {
		t.Error("expected ai-agent removed")
	}
}

func TestApplyCreates_SkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := []byte("ai-agent: Cursor\n")
	os.WriteFile(path, original, 0644)

	called := false
	err := applyCreates([]string{path}, func(string) ([]byte, bool) {
		called = true
		return []byte("ai-agent: Claude Code\n"), true
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("content callback must not run when the path already exists")
	}
	out, _ := os.ReadFile(path)
	if string(out) != string(original) {
		t.Errorf("existing file must be untouched, got: %s", out)
	}
}

func TestApplyCreates_WritesWhenContentProvided(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	err := applyCreates([]string{path}, func(string) ([]byte, bool) {
		return []byte("ai-agent: Claude Code\n"), true
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fileExistsAt(path) {
		t.Fatal("expected file created")
	}
}

func TestApplyCreates_SkipsSilentlyWhenNoContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := applyCreates([]string{path}, func(string) ([]byte, bool) {
		return nil, false
	})
	if err != nil {
		t.Fatal(err)
	}
	if fileExistsAt(path) {
		t.Error("expected no file created when content callback returns ok=false")
	}
}

func TestApplyModifies_SkipsWhenNoContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("sdd-framework: None\n"), 0644)

	err := applyModifies([]string{path}, func(string) (string, string, bool) {
		return "", "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.HasAIAgentField(path) {
		t.Error("expected no field written when content callback returns ok=false")
	}
}

// ---------------------------------------------------------------------
// applyMismatchFix — end-to-end per Kind.
// ---------------------------------------------------------------------

func TestApplyMismatchFix_BareParentCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, config.ParlayDir, config.ConfigFile)

	m := config.Mismatch{
		Kind: config.MismatchBareParent,
		ProposedFix: config.FixDescriptor{
			Creates: []string{cfgPath},
		},
	}
	if err := applyMismatchFix(m); err != nil {
		t.Fatal(err)
	}
	if !config.HasAIAgentField(cfgPath) {
		t.Error("expected bare-parent fix to create a config.yaml with ai-agent set")
	}
}

func TestApplyMismatchFix_AgentAtChildWritesMigratedValueToParent(t *testing.T) {
	dir := t.TempDir()
	childPath := filepath.Join(dir, "child", "config.yaml")
	parentPath := filepath.Join(dir, "parent", "config.yaml")
	os.MkdirAll(filepath.Dir(childPath), 0755)
	os.WriteFile(childPath, []byte("ai-agent: Cursor\n"), 0644)

	m := config.Mismatch{
		Kind:   config.MismatchAgentAtChild,
		Values: []string{"Cursor"},
		ProposedFix: config.FixDescriptor{
			Creates:  []string{parentPath},
			Modifies: []string{parentPath},
			RemovesFields: []config.FieldRemoval{
				{File: childPath, Field: "ai-agent"},
			},
		},
	}
	if err := applyMismatchFix(m); err != nil {
		t.Fatal(err)
	}
	if config.HasAIAgentField(childPath) {
		t.Error("expected ai-agent removed from child")
	}
	var parentCfg config.ProjectConfig
	data, err := os.ReadFile(parentPath)
	if err != nil {
		t.Fatalf("expected parent config created: %v", err)
	}
	yaml.Unmarshal(data, &parentCfg)
	if parentCfg.AIAgent != "Cursor" {
		t.Errorf("parent ai-agent = %q, want Cursor (migrated from child)", parentCfg.AIAgent)
	}
}

// ---------------------------------------------------------------------
// detectMismatches — missing-directory / extra-directory.
// ---------------------------------------------------------------------

func setupThreeTreeDirs(t *testing.T) (intentsRoot, handoffRoot, buildRoot string) {
	t.Helper()
	dir := t.TempDir()
	intentsRoot = filepath.Join(dir, config.SpecDir, config.IntentsDir)
	handoffRoot = filepath.Join(dir, config.SpecDir, config.HandoffDir)
	buildRoot = filepath.Join(dir, config.ParlayDir, config.BuildDir)
	for _, r := range []string{intentsRoot, handoffRoot, buildRoot} {
		if err := os.MkdirAll(r, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return intentsRoot, handoffRoot, buildRoot
}

func TestDetectMismatches_MissingDirectory(t *testing.T) {
	intentsRoot, handoffRoot, buildRoot := setupThreeTreeDirs(t)
	os.MkdirAll(filepath.Join(intentsRoot, "my-feature"), 0755)
	os.WriteFile(filepath.Join(intentsRoot, "my-feature", "intents.md"), []byte("# X"), 0644)
	// Note: no matching directory created under handoffRoot or buildRoot.

	mismatches, err := detectMismatches(intentsRoot, []string{intentsRoot, handoffRoot, buildRoot})
	if err != nil {
		t.Fatal(err)
	}

	found := 0
	for _, m := range mismatches {
		if m.Category == "missing-directory" && strings.Contains(m.NewPath, "my-feature") {
			found++
		}
	}
	if found != 2 {
		t.Errorf("expected 2 missing-directory mismatches (handoff + build), got %d: %+v", found, mismatches)
	}
}

func TestDetectMismatches_ExtraDirectory(t *testing.T) {
	intentsRoot, handoffRoot, buildRoot := setupThreeTreeDirs(t)
	// A build directory with no matching intents source at all.
	staleDir := filepath.Join(buildRoot, "deleted-feature")
	os.MkdirAll(staleDir, 0755)
	os.WriteFile(filepath.Join(staleDir, "buildfile.yaml"), []byte("feature: deleted-feature\n"), 0644)

	mismatches, err := detectMismatches(intentsRoot, []string{intentsRoot, handoffRoot, buildRoot})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, m := range mismatches {
		if m.Category == "extra-directory" && m.OldPath == staleDir {
			found = true
		}
	}
	if !found {
		t.Errorf("expected extra-directory mismatch for %s, got: %+v", staleDir, mismatches)
	}
}

func TestDetectMismatches_CleanTreesProduceNoMismatches(t *testing.T) {
	intentsRoot, handoffRoot, buildRoot := setupThreeTreeDirs(t)
	for _, root := range []string{intentsRoot, handoffRoot, buildRoot} {
		os.MkdirAll(filepath.Join(root, "in-sync-feature"), 0755)
	}
	os.WriteFile(filepath.Join(intentsRoot, "in-sync-feature", "intents.md"), []byte("# X"), 0644)

	mismatches, err := detectMismatches(intentsRoot, []string{intentsRoot, handoffRoot, buildRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 0 {
		t.Errorf("expected no mismatches for in-sync trees, got: %+v", mismatches)
	}
}

// ---------------------------------------------------------------------
// detectStaleInitiativeBuildfiles — the parlay-tool-monolith case.
// ---------------------------------------------------------------------

func TestDetectStaleInitiativeBuildfiles_FlagsBuildfileUnderInitiative(t *testing.T) {
	intentsRoot, _, buildRoot := setupThreeTreeDirs(t)

	// spec/intents/parlay-tool/ is an INITIATIVE: no intents.md of its
	// own, but a sub-feature directory that has one.
	initDir := filepath.Join(intentsRoot, "parlay-tool")
	os.MkdirAll(filepath.Join(initDir, "sub-feature"), 0755)
	os.WriteFile(filepath.Join(initDir, "sub-feature", "intents.md"), []byte("# Sub"), 0644)

	// .parlay/build/parlay-tool/buildfile.yaml — stale monolith leftover.
	staleBuildDir := filepath.Join(buildRoot, "parlay-tool")
	os.MkdirAll(staleBuildDir, 0755)
	os.WriteFile(filepath.Join(staleBuildDir, "buildfile.yaml"), []byte("feature: parlay-tool\n"), 0644)

	mismatches, err := detectStaleInitiativeBuildfiles(intentsRoot, buildRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 stale-initiative-buildfile mismatch, got %d: %+v", len(mismatches), mismatches)
	}
	if mismatches[0].Category != "stale-initiative-buildfile" {
		t.Errorf("Category = %q, want stale-initiative-buildfile", mismatches[0].Category)
	}
	if mismatches[0].OldPath != filepath.Join(staleBuildDir, "buildfile.yaml") {
		t.Errorf("OldPath = %q, want %s", mismatches[0].OldPath, filepath.Join(staleBuildDir, "buildfile.yaml"))
	}
}

func TestDetectStaleInitiativeBuildfiles_IgnoresRealFeature(t *testing.T) {
	intentsRoot, _, buildRoot := setupThreeTreeDirs(t)

	// A genuine feature (not an initiative) with its own buildfile.
	os.MkdirAll(filepath.Join(intentsRoot, "real-feature"), 0755)
	os.WriteFile(filepath.Join(intentsRoot, "real-feature", "intents.md"), []byte("# X"), 0644)
	buildDir := filepath.Join(buildRoot, "real-feature")
	os.MkdirAll(buildDir, 0755)
	os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte("feature: real-feature\n"), 0644)

	mismatches, err := detectStaleInitiativeBuildfiles(intentsRoot, buildRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 0 {
		t.Errorf("expected no mismatches for a genuine feature's own buildfile, got: %+v", mismatches)
	}
}

func TestDetectStaleInitiativeBuildfiles_MissingBuildRootIsNotAnError(t *testing.T) {
	intentsRoot, _, buildRoot := setupThreeTreeDirs(t)
	os.RemoveAll(buildRoot)

	mismatches, err := detectStaleInitiativeBuildfiles(intentsRoot, buildRoot)
	if err != nil {
		t.Fatalf("expected no error for a missing build root, got: %v", err)
	}
	if mismatches != nil {
		t.Errorf("expected nil mismatches, got: %+v", mismatches)
	}
}

// TestFoldRenamePairs_RenameBecomesMove guards the data-loss defect: a
// renamed feature produced an unrelated (missing-directory, extra-directory)
// pair in every other tree, and the offered remedy was "create an empty
// directory" plus "delete the old one" — discarding the buildfile,
// testcases and baseline the old directory held. It also never converged,
// because destructive prompts decline by default on a non-TTY and --yes
// does not cover them.
func TestFoldRenamePairs_RenameBecomesMove(t *testing.T) {
	tree := "/p/.parlay/build"
	in := []mismatch{
		{Category: "missing-directory", NewPath: tree + "/expenses-list", Tree: tree},
		{Category: "extra-directory", OldPath: tree + "/expense-list", Tree: tree},
	}
	got := foldRenamePairs(in)
	if len(got) != 1 {
		t.Fatalf("got %d mismatches, want 1 folded move: %+v", len(got), got)
	}
	if got[0].Category != "feature-move" {
		t.Errorf("Category = %q, want feature-move (a move preserves the directory's contents)", got[0].Category)
	}
	if got[0].OldPath != tree+"/expense-list" || got[0].NewPath != tree+"/expenses-list" {
		t.Errorf("move %s -> %s, want expense-list -> expenses-list", got[0].OldPath, got[0].NewPath)
	}
}

// TestFoldRenamePairs_AmbiguousLeftAlone keeps the conservative guard: with
// more than one candidate on either side the pairing is not determinable,
// so the per-event behaviour stands and a human decides.
func TestFoldRenamePairs_AmbiguousLeftAlone(t *testing.T) {
	tree := "/p/.parlay/build"
	in := []mismatch{
		{Category: "missing-directory", NewPath: tree + "/a-new", Tree: tree},
		{Category: "missing-directory", NewPath: tree + "/b-new", Tree: tree},
		{Category: "extra-directory", OldPath: tree + "/a-old", Tree: tree},
	}
	got := foldRenamePairs(in)
	if len(got) != 3 {
		t.Errorf("got %d mismatches, want all 3 left unfolded when the pairing is ambiguous", len(got))
	}
	for _, m := range got {
		if m.Category == "feature-move" {
			t.Errorf("folded an ambiguous pair into %+v", m)
		}
	}
}

// TestFoldRenamePairs_DifferentTreesNotPaired ensures a missing dir in one
// tree is never paired with an extra dir in another.
func TestFoldRenamePairs_DifferentTreesNotPaired(t *testing.T) {
	in := []mismatch{
		{Category: "missing-directory", NewPath: "/p/spec/handoff/x", Tree: "/p/spec/handoff"},
		{Category: "extra-directory", OldPath: "/p/.parlay/build/y", Tree: "/p/.parlay/build"},
	}
	got := foldRenamePairs(in)
	if len(got) != 2 {
		t.Errorf("got %d, want 2 — cross-tree pairs must not fold", len(got))
	}
}
