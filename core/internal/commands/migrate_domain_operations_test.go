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
		if err := os.MkdirAll(filepath.Join(intentsRoot, c), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return intentsRoot
}

// resetMigrateDomainOperationsFlags restores the package-level flag vars
// migrate-domain-operations reads, so tests don't leak state into each
// other (mirrors the defer pattern in move_feature_test.go).
func resetMigrateDomainOperationsFlags() {
	migrateDomainOperationsFeature = ""
	migrateDomainOperationsNonInteractive = false
}

func TestMigrateDomainOperations_HeadlessAmbiguous_HardErrors(t *testing.T) {
	intentsRoot := setupMigrateDomainOperationsProject(t, "feature-a", "feature-b")

	migrateDomainOperationsNonInteractive = true
	defer resetMigrateDomainOperationsFlags()

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

	migrateDomainOperationsNonInteractive = true
	migrateDomainOperationsFeature = "@feature-b"
	defer resetMigrateDomainOperationsFlags()

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

	migrateDomainOperationsNonInteractive = true
	migrateDomainOperationsFeature = "@feature-c"
	defer resetMigrateDomainOperationsFlags()

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

	migrateDomainOperationsNonInteractive = true
	defer resetMigrateDomainOperationsFlags()

	err := runMigrateDomainOperations(testCommandWithContext(t, testContext(t)), nil)
	if err != nil {
		t.Fatalf("expected no error with a single candidate feature, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(intentsRoot, "feature-a", "capabilities.yaml")); statErr != nil {
		t.Errorf("expected stub written to feature-a/capabilities.yaml: %v", statErr)
	}
}
