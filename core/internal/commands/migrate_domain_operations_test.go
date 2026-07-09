// parlay-feature: parlay-tool/multi-adapter
// parlay-component: domain-operations-migration-prompt
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// setupMigrateDomainOperationsProject writes a domain-model.yaml with one
// operation and creates the given candidate feature directories under
// spec/intents/. Returns the intents root.
func setupMigrateDomainOperationsProject(t *testing.T, candidates ...string) string {
	t.Helper()
	dir := setupTestDir(t)

	dm := "schema_version: 1\noperations:\n  - entity: Task\n    title: Create Task\n"
	if err := os.WriteFile(filepath.Join(dir, "domain-model.yaml"), []byte(dm), 0644); err != nil {
		t.Fatal(err)
	}

	intentsRoot := filepath.Join(dir, config.SpecDir, config.IntentsDir)
	for _, c := range candidates {
		featDir := filepath.Join(intentsRoot, c)
		if err := os.MkdirAll(featDir, 0755); err != nil {
			t.Fatal(err)
		}
		// A real candidate must classify as a feature (config.AllFeatures
		// walks spec/intents/ via config.ClassifyDir, which requires
		// intents.md) — a bare directory classifies as "deferred" and is
		// correctly excluded, unlike the old naive os.ReadDir scan this
		// command used to do.
		if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte("# "+c+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return intentsRoot
}

func TestMigrateDomainOperations_HeadlessAmbiguous_HardErrors(t *testing.T) {
	intentsRoot := setupMigrateDomainOperationsProject(t, "feature-a", "feature-b")

	resetFlagsAfterTest(t, migrateDomainOperationsCmd.Flags())
	migrateDomainOperationsNonInteractive = true

	err := runMigrateDomainOperations(testCommandWithContext(t, testContext(t)), nil)
	if err == nil {
		t.Fatal("expected ambiguous-target error in headless mode, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous-target") {
		t.Errorf("expected ambiguous-target error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "feature-a") || !strings.Contains(err.Error(), "feature-b") {
		t.Errorf("expected error to list both candidates, got: %v", err)
	}

	// The whole point of the fix: headless mode must NOT silently write to
	// the first candidate the way the old code did.
	for _, c := range []string{"feature-a", "feature-b"} {
		capPath := filepath.Join(intentsRoot, c, "capabilities.yaml")
		if _, statErr := os.Stat(capPath); statErr == nil {
			t.Errorf("headless hard error must not write capabilities.yaml, but found %s", capPath)
		}
	}
}

func TestMigrateDomainOperations_HeadlessWithExplicitFeature_Succeeds(t *testing.T) {
	intentsRoot := setupMigrateDomainOperationsProject(t, "feature-a", "feature-b")

	resetFlagsAfterTest(t, migrateDomainOperationsCmd.Flags())
	migrateDomainOperationsNonInteractive = true
	migrateDomainOperationsFeature = "@feature-b"

	err := runMigrateDomainOperations(testCommandWithContext(t, testContext(t)), nil)
	if err != nil {
		t.Fatalf("expected --feature to resolve the ambiguity, got error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(intentsRoot, "feature-b", "capabilities.yaml")); statErr != nil {
		t.Errorf("expected stub written to feature-b/capabilities.yaml: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(intentsRoot, "feature-a", "capabilities.yaml")); statErr == nil {
		t.Errorf("did not expect a stub written to feature-a/capabilities.yaml")
	}
}

func TestMigrateDomainOperations_ExplicitFeatureNotACandidate_Errors(t *testing.T) {
	setupMigrateDomainOperationsProject(t, "feature-a", "feature-b")

	resetFlagsAfterTest(t, migrateDomainOperationsCmd.Flags())
	migrateDomainOperationsNonInteractive = true
	migrateDomainOperationsFeature = "@feature-c"

	err := runMigrateDomainOperations(testCommandWithContext(t, testContext(t)), nil)
	if err == nil {
		t.Fatal("expected error for --feature value outside the candidate set, got nil")
	}
	if !strings.Contains(err.Error(), "feature-c") {
		t.Errorf("expected error to name the offending --feature value, got: %v", err)
	}
}

func TestMigrateDomainOperations_UnambiguousSingleCandidate_StillWorksHeadless(t *testing.T) {
	intentsRoot := setupMigrateDomainOperationsProject(t, "feature-a")

	resetFlagsAfterTest(t, migrateDomainOperationsCmd.Flags())
	migrateDomainOperationsNonInteractive = true

	err := runMigrateDomainOperations(testCommandWithContext(t, testContext(t)), nil)
	if err != nil {
		t.Fatalf("expected no error with a single candidate feature, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(intentsRoot, "feature-a", "capabilities.yaml")); statErr != nil {
		t.Errorf("expected stub written to feature-a/capabilities.yaml: %v", statErr)
	}
}

// TestMigrateDomainOperations_QualifiedNestedFeatureCandidate is the
// regression test for the Phase 4 finding: candidate detection used to
// be a bare os.ReadDir(intentsRoot) that only saw top-level entries, so
// a feature nested under an initiative (spec/intents/<initiative>/<feature>/)
// was never a valid candidate at all — not ambiguous, structurally
// invisible — and --feature could never target one either, even with
// the fully-qualified "initiative/feature" form. Now that candidate
// discovery goes through cfg.AllFeatures() (initiative-aware), both the
// candidate listing and --feature must accept the qualified form.
func TestMigrateDomainOperations_QualifiedNestedFeatureCandidate(t *testing.T) {
	intentsRoot := setupMigrateDomainOperationsProject(t, "top-level-feature", "auth-overhaul/login")

	resetFlagsAfterTest(t, migrateDomainOperationsCmd.Flags())
	migrateDomainOperationsNonInteractive = true
	migrateDomainOperationsFeature = "@auth-overhaul/login"

	err := runMigrateDomainOperations(testCommandWithContext(t, testContext(t)), nil)
	if err != nil {
		t.Fatalf("expected --feature to target the nested candidate, got error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(intentsRoot, "auth-overhaul", "login", "capabilities.yaml")); statErr != nil {
		t.Errorf("expected stub written to auth-overhaul/login/capabilities.yaml: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(intentsRoot, "top-level-feature", "capabilities.yaml")); statErr == nil {
		t.Errorf("did not expect a stub written to top-level-feature/capabilities.yaml")
	}
}

// TestMigrateDomainOperations_HeadlessAmbiguous_ListsNestedCandidate
// confirms the ambiguous-target error's candidate list itself includes
// a nested initiative/feature candidate, not just top-level ones.
func TestMigrateDomainOperations_HeadlessAmbiguous_ListsNestedCandidate(t *testing.T) {
	setupMigrateDomainOperationsProject(t, "top-level-feature", "auth-overhaul/login")

	resetFlagsAfterTest(t, migrateDomainOperationsCmd.Flags())
	migrateDomainOperationsNonInteractive = true

	err := runMigrateDomainOperations(testCommandWithContext(t, testContext(t)), nil)
	if err == nil {
		t.Fatal("expected ambiguous-target error, got nil")
	}
	if !strings.Contains(err.Error(), "auth-overhaul/login") {
		t.Errorf("expected the nested candidate listed in the error, got: %v", err)
	}
}
