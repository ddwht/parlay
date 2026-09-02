package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

func TestAddFeature_CreatesFeatureFolder(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"upgrade", "plan", "creation"})
	if err != nil {
		t.Fatal(err)
	}

	featurePath := testContext(t).FeaturePath("upgrade-plan-creation")
	if _, err := os.Stat(featurePath); os.IsNotExist(err) {
		t.Error("feature directory not created")
	}
}

// A standalone feature must be born in all three trees, exactly as one
// created inside an initiative is. The other standalone tests assert
// only on FeaturePath, which is why the asymmetry survived: every
// feature added without --initiative was immediately in a state
// `parlay repair` calls a mismatch.
func TestAddFeature_CreatesAllThreeTrees(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	if err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"fleet", "overview"}); err != nil {
		t.Fatal(err)
	}

	cfg := testContext(t)
	for _, path := range []string{
		cfg.FeaturePath("fleet-overview"),
		cfg.HandoffPath("fleet-overview"),
		cfg.BuildPath("fleet-overview"),
	} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("directory not created: %s", path)
		}
	}
}

// The tree-parity detector must be satisfied by a freshly-added
// standalone feature. This is the assertion that ties the fix to the
// rule `repair` and `status`'s trees: line enforce, rather than to the
// three paths above.
func TestAddFeature_LeavesNoRepairMismatch(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	if err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"fleet", "overview"}); err != nil {
		t.Fatal(err)
	}

	cfg := testContext(t)
	mismatches, err := detectMismatches(cfg.IntentsRoot(), threeTreeRoots(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 0 {
		t.Errorf("a newly added feature should need no repair, got %d mismatches: %+v", len(mismatches), mismatches)
	}
}

func TestAddFeature_CreatesIntentsMd(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	runAddFeature(testCommandWithContext(t, testContext(t)), []string{"upgrade", "plan"})

	content, err := os.ReadFile(filepath.Join(testContext(t).FeaturePath("upgrade-plan"), "intents.md"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "# Upgrade Plan") {
		t.Error("intents.md missing feature header")
	}
}

func TestAddFeature_CreatesDialogsMd(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	runAddFeature(testCommandWithContext(t, testContext(t)), []string{"fleet", "overview"})

	path := filepath.Join(testContext(t).FeaturePath("fleet-overview"), "dialogs.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("dialogs.md not created")
	}
}

func TestAddFeature_RejectsDuplicate(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	runAddFeature(testCommandWithContext(t, testContext(t)), []string{"my", "feature"})
	err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"my", "feature"})

	if err == nil {
		t.Error("expected error for duplicate feature, got nil")
	}
}

func TestAddFeature_SlugifiesName(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	runAddFeature(testCommandWithContext(t, testContext(t)), []string{"Fleet", "Health", "Overview"})

	if _, err := os.Stat(testContext(t).FeaturePath("fleet-health-overview")); os.IsNotExist(err) {
		t.Error("slug not correctly derived from name")
	}
}

// --- Initiative tests (parlay-feature: initiatives) ---

func TestAddFeatureWithInitiative_CreatesInitiativeAndFeature(t *testing.T) {
	setupTestDir(t)
	for _, root := range threeTreeRoots(testContext(t)) {
		os.MkdirAll(root, 0755)
	}

	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul", false, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		filepath.Join(config.SpecDir, config.IntentsDir, "auth-overhaul", "password-reset"),
		filepath.Join(config.SpecDir, config.HandoffDir, "auth-overhaul", "password-reset"),
		filepath.Join(testContext(t).BuildRoot(), "auth-overhaul", "password-reset"),
	} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("directory not created: %s", path)
		}
	}

	intentsPath := filepath.Join(config.SpecDir, config.IntentsDir, "auth-overhaul", "password-reset", "intents.md")
	if _, err := os.Stat(intentsPath); os.IsNotExist(err) {
		t.Error("intents.md not created inside initiative feature")
	}
}

func TestAddFeatureWithInitiative_ReusesExistingInitiative(t *testing.T) {
	setupTestDir(t)
	for _, root := range threeTreeRoots(testContext(t)) {
		os.MkdirAll(root, 0755)
	}

	runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul", false, false)
	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "sso setup", "sso-setup", "auth overhaul", false, false)

	if err != nil {
		t.Fatalf("adding second feature to existing initiative should succeed, got: %v", err)
	}

	path := filepath.Join(config.SpecDir, config.IntentsDir, "auth-overhaul", "sso-setup")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("second feature not created inside initiative")
	}
}

func TestAddFeatureWithInitiative_ScopeCollision(t *testing.T) {
	setupTestDir(t)
	for _, root := range threeTreeRoots(testContext(t)) {
		os.MkdirAll(root, 0755)
	}

	runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul", false, false)
	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul", false, false)

	if err == nil {
		t.Error("expected scope collision error, got nil")
	}
}

func TestAddFeatureWithInitiative_TopLevelCollision(t *testing.T) {
	setupTestDir(t)
	for _, root := range threeTreeRoots(testContext(t)) {
		os.MkdirAll(root, 0755)
	}

	orphanPath := filepath.Join(config.SpecDir, config.IntentsDir, "password-reset")
	os.MkdirAll(orphanPath, 0755)
	os.WriteFile(filepath.Join(orphanPath, "intents.md"), []byte("# Password Reset\n"), 0644)

	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "login", "login", "password-reset", false, false)

	if err == nil {
		t.Error("expected top-level collision error, got nil")
	}
}

func TestAddFeatureWithInitiative_SameSlugDifferentInitiative(t *testing.T) {
	setupTestDir(t)
	for _, root := range threeTreeRoots(testContext(t)) {
		os.MkdirAll(root, 0755)
	}

	runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul", false, false)
	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "billing", false, false)

	if err != nil {
		t.Errorf("same slug in different initiative should succeed, got: %v", err)
	}
}

// withAuthoredFlags sets the --authored flag globals for one test and
// restores them afterwards. They are package-level cobra bindings, so a
// test that forgets to reset them leaks into every later test in the file.
func withAuthoredFlags(t *testing.T, sources, tests []string, summary string) {
	t.Helper()
	authoredFlag, authoredSourcesFlag, authoredTestsFlag, authoredSummaryFlag = true, sources, tests, summary
	t.Cleanup(func() {
		authoredFlag, authoredSourcesFlag, authoredTestsFlag, authoredSummaryFlag = false, nil, nil, ""
	})
}

// A unit occupies two trees, not three, and the tree-parity detector must
// be satisfied by that — this is the assertion tying --authored to the
// rule repair and status enforce, rather than to a list of paths.
func TestAddFeature_AuthoredLeavesNoRepairMismatch(t *testing.T) {
	dir := setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)
	os.MkdirAll(filepath.Join(dir, "src", "codec"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "codec", "decode.go"), []byte("package codec\n"), 0644)
	withAuthoredFlags(t, []string{"src/codec/**"}, nil, "hand-rolled codec")

	if err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"codec", "core"}); err != nil {
		t.Fatal(err)
	}

	cfg := testContext(t)
	if _, err := os.Stat(filepath.Join(cfg.FeaturePath("codec-core"), config.AuthoredFile)); err != nil {
		t.Errorf("authored.yaml not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.FeaturePath("codec-core"), "dialogs.md")); err == nil {
		t.Error("a unit must not get a dialogs.md — nothing authors dialog turns for code that already exists")
	}
	if _, err := os.Stat(cfg.HandoffPath("codec-core")); err == nil {
		t.Error("a unit must not get a handoff directory")
	}
	if _, err := os.Stat(cfg.BuildPath("codec-core")); err != nil {
		t.Errorf("a unit still needs its build directory (hashes, coverage): %v", err)
	}

	mismatches, err := detectMismatches(cfg.IntentsRoot(), threeTreeRoots(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(mismatches) != 0 {
		t.Errorf("a newly declared unit should need no repair, got %d: %+v", len(mismatches), mismatches)
	}
}

// The declaration this writes must survive its own validator. A scaffold
// that emits something the next command rejects is worse than no scaffold.
func TestAddFeature_AuthoredWritesAValidDeclaration(t *testing.T) {
	dir := setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)
	os.MkdirAll(filepath.Join(dir, "src", "codec"), 0755)
	withAuthoredFlags(t, []string{"src/codec/**"}, []string{"tests/codec/**"}, "hand-rolled codec")

	if err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"codec", "core"}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(testContext(t).FeaturePath("codec-core"), config.AuthoredFile)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if outcomes := agent.ValidateAuthoredUnit(agent.ModeAuthoring, path, content); len(outcomes) != 0 {
		t.Errorf("scaffolded declaration does not validate: %+v", outcomes)
	}
}

// A unit owning no files declares nothing, so the flag combination that
// would produce one is refused before anything is written.
func TestAddFeature_AuthoredRequiresSources(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)
	withAuthoredFlags(t, nil, nil, "")

	err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"codec", "core"})
	if err == nil {
		t.Fatal("expected --authored with no --sources to be refused")
	}
	if _, statErr := os.Stat(testContext(t).FeaturePath("codec-core")); statErr == nil {
		t.Error("the refusal must happen before any directory is created")
	}
}

// The scaffold must prompt without pretending to be an intent.
//
// `no-intents` warns while authoring and ERRORS at build, so a scaffold whose
// template parsed as a real intent would satisfy that check falsely — a
// feature would look authored when nobody had written anything.
//
// Checked through parser.ParseIntentsFile, not by inspecting the string. The
// first version of this test truncated the scaffold at `<!--` and then
// asserted no `## ` remained, which is an invented parser path: the real
// scanner had no comment state and DID parse the commented template as an
// intent titled "<What the user wants...>". The test passed while the thing it
// claimed to prevent was happening.
func TestScaffoldedIntents_PromptsWithoutFakingAnIntent(t *testing.T) {
	out := scaffoldedIntents("Tax Filing")

	if !strings.HasPrefix(out, "# Tax Filing\n") {
		t.Errorf("scaffold must open with the feature heading; got %q", out[:40])
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "intents.md")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, err := parser.ParseIntentsFile(path)
	if err != nil {
		t.Fatalf("scaffold does not parse: %v", err)
	}
	if len(parsed) != 0 {
		t.Fatalf("scaffold parsed as %d intent(s) — no-intents would pass on an unauthored feature: %+v", len(parsed), parsed)
	}

	// And the graded verdict, not just the parse: authoring warns, build errors.
	for _, tc := range []struct {
		mode agent.ValidationMode
		want agent.Severity
	}{{agent.ModeAuthoring, agent.SeverityWarning}, {agent.ModeBuild, agent.SeverityError}} {
		found := false
		for _, o := range agent.ValidateIntentsDeep(tc.mode, path, []byte(out)) {
			if o.Code == "no-intents" {
				found = true
				if o.Severity != tc.want {
					t.Errorf("no-intents severity in %v = %v, want %v", tc.mode, o.Severity, tc.want)
				}
			}
		}
		if !found {
			t.Errorf("scaffold did not report no-intents in %v — it is being read as authored", tc.mode)
		}
	}

	// The prompt has to actually carry the boundaries, or it is the old empty
	// file with extra bytes.
	for _, field := range []string{"**Goal**", "**Persona**", "**Priority**", "**Context**",
		"**Action**", "**Objects**", "**Constraints**", "**Verify**", "**Questions**"} {
		if !strings.Contains(out, field) {
			t.Errorf("scaffold omits %s", field)
		}
	}
	for _, cue := range []string{"not \"click Upload\"", "not \"accountant\"", "infrastructure.md"} {
		if !strings.Contains(out, cue) {
			t.Errorf("scaffold omits the guidance cue %q", cue)
		}
	}
	if !strings.Contains(out, "Soft boundaries") {
		t.Error("scaffold does not point at the full guidance")
	}
}

// The initiative path must honour its `authored` ARGUMENT, not the
// package global.
//
// It read authoredFlag while taking authored as a parameter, so the
// argument was decorative. Promotion always passes false, so the two
// agreed and nothing failed — but a function that ignores its own
// argument is a defect waiting for the first caller that disagrees with
// the global, and it cannot be caught by any test that leaves them equal.
func TestAddFeatureWithInitiative_HonoursTheAuthoredArgumentNotTheGlobal(t *testing.T) {
	setupTestDir(t)
	if err := os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testContext(t)

	// The global says NOT authored; the argument says authored.
	authoredFlag = false
	authoredSourcesFlag = []string{"src/**/*.go"}
	t.Cleanup(func() { authoredFlag, authoredSourcesFlag = false, nil })

	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})
	if err := runAddFeatureWithInitiative(cmd, cfg, "reset password", "reset-password", "auth", true, false); err != nil {
		t.Fatal(err)
	}

	// An authored unit lives in the unit trees only — a handoff twin is
	// exactly what `authored` suppresses. If the global had won, the
	// handoff directory would be there.
	handoff := filepath.Join(cfg.Root.Path, config.SpecDir, "handoff", "auth", "reset-password")
	if _, err := os.Stat(handoff); err == nil {
		t.Error("the initiative path built a handoff tree — it read the global, not its argument")
	}
}
