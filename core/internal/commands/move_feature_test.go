// parlay-feature: move-feature
// parlay-component: MoveResult
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func setupMoveTestProject(t *testing.T) {
	t.Helper()
	setupTestDir(t)
	for _, root := range threeTreeRoots(testContext(t)) {
		os.MkdirAll(root, 0755)
	}
}

func createFeatureInTree(t *testing.T, qualifiedPath string) {
	t.Helper()
	for _, root := range threeTreeRoots(testContext(t)) {
		dir := filepath.Join(root, qualifiedPath)
		os.MkdirAll(dir, 0755)
	}
	intentsPath := filepath.Join(config.SpecDir, config.IntentsDir, qualifiedPath, "intents.md")
	os.WriteFile(intentsPath, []byte("# Feature\n"), 0644)
}

func TestMoveFeature_OrphanToInitiative(t *testing.T) {
	setupMoveTestProject(t)
	createFeatureInTree(t, "password-reset")

	initDir := filepath.Join(config.SpecDir, config.IntentsDir, "auth-overhaul")
	os.MkdirAll(initDir, 0755)

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "auth-overhaul"
	moveOutFlag = false

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@password-reset"})
	if err != nil {
		t.Fatal(err)
	}

	for _, root := range threeTreeRoots(testContext(t)) {
		dest := filepath.Join(root, "auth-overhaul", "password-reset")
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			t.Errorf("feature not moved in %s tree", root)
		}
	}
}

func TestMoveFeature_AutoCreateInitiative(t *testing.T) {
	setupMoveTestProject(t)
	createFeatureInTree(t, "sso-setup")

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "auth-redesign"
	moveOutFlag = false

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@sso-setup"})
	if err != nil {
		t.Fatal(err)
	}

	initDir := filepath.Join(config.SpecDir, config.IntentsDir, "auth-redesign")
	if _, err := os.Stat(initDir); os.IsNotExist(err) {
		t.Error("initiative not auto-created")
	}
}

func TestMoveFeature_MoveOut(t *testing.T) {
	setupMoveTestProject(t)
	initDir := filepath.Join(config.SpecDir, config.IntentsDir, "auth-overhaul")
	os.MkdirAll(initDir, 0755)
	createFeatureInTree(t, "auth-overhaul/password-reset")

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = ""
	moveOutFlag = true

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@auth-overhaul/password-reset"})
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(config.SpecDir, config.IntentsDir, "password-reset")
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		t.Error("feature not moved to top level")
	}
}

func TestMoveFeature_ScopeCollision(t *testing.T) {
	setupMoveTestProject(t)
	createFeatureInTree(t, "password-reset")
	createFeatureInTree(t, "billing/password-reset")

	billingInit := filepath.Join(config.SpecDir, config.IntentsDir, "billing")
	os.MkdirAll(billingInit, 0755)

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "billing"
	moveOutFlag = false

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@password-reset"})
	if err == nil {
		t.Error("expected scope collision error, got nil")
	}
}

func TestMoveFeature_NoopSameLocation(t *testing.T) {
	setupMoveTestProject(t)
	createFeatureInTree(t, "auth-overhaul/password-reset")

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "auth-overhaul"
	moveOutFlag = false

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@auth-overhaul/password-reset"})
	if err != nil {
		t.Errorf("same-location move should be a no-op, got: %v", err)
	}
}

func TestMoveFeature_NotFound(t *testing.T) {
	setupMoveTestProject(t)

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "auth-overhaul"
	moveOutFlag = false

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@nonexistent"})
	if err == nil {
		t.Error("expected not-found error, got nil")
	}
}

func TestMoveFeature_MutuallyExclusiveFlags(t *testing.T) {
	setupMoveTestProject(t)

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "auth-overhaul"
	moveOutFlag = true

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@password-reset"})
	if err == nil {
		t.Error("expected mutually exclusive flag error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutually exclusive error message, got: %v", err)
	}
}

func TestMoveFeature_MissingFlags(t *testing.T) {
	setupMoveTestProject(t)

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = ""
	moveOutFlag = false

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@password-reset"})
	if err == nil {
		t.Error("expected missing destination error, got nil")
	}
}

func TestMoveFeature_WrongType_Initiative(t *testing.T) {
	setupMoveTestProject(t)

	initDir := filepath.Join(config.SpecDir, config.IntentsDir, "auth-overhaul")
	os.MkdirAll(initDir, 0755)
	childDir := filepath.Join(initDir, "login")
	os.MkdirAll(childDir, 0755)
	os.WriteFile(filepath.Join(childDir, "intents.md"), []byte("# Login\n"), 0644)

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "other"
	moveOutFlag = false

	err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@auth-overhaul"})
	if err == nil {
		t.Error("expected wrong-type error for initiative, got nil")
	}
	if !strings.Contains(err.Error(), "initiative, not a feature") {
		t.Errorf("expected initiative error message, got: %v", err)
	}
}

// A ledger feature's amendments/ directory is part of its spec tree and must
// travel with the move — a ledger left behind reads as "this feature has no
// history", which is the silent-loss shape the ledger model exists to
// prevent. The move is a whole-directory rename, so this is pinned rather
// than implemented; the test exists so a future per-file move cannot drop
// the zone unnoticed.
//
// parlay-feature: parlay-tool/ledger-and-contract
func TestMoveFeature_AmendmentsTravelWithTheFeature(t *testing.T) {
	setupMoveTestProject(t)
	createFeatureInTree(t, "billing")

	amDir := filepath.Join(config.SpecDir, config.IntentsDir, "billing", "amendments")
	os.MkdirAll(amDir, 0755)
	amendment := "---\namendment: first\ndate: 2026-08-13\naffects: [\"@billing/operation:x\"]\n---\n## Change\na\n"
	os.WriteFile(filepath.Join(amDir, "001-first.md"), []byte(amendment), 0644)

	resetFlagsAfterTest(t, moveFeatureCmd.Flags())
	moveToFlag = "revenue"
	moveOutFlag = false

	if err := runMoveFeature(testCommandWithContext(t, testContext(t)), []string{"@billing"}); err != nil {
		t.Fatal(err)
	}

	moved := filepath.Join(config.SpecDir, config.IntentsDir, "revenue", "billing", "amendments", "001-first.md")
	data, err := os.ReadFile(moved)
	if err != nil {
		t.Fatalf("amendment did not travel with the feature: %v", err)
	}
	if string(data) != amendment {
		t.Errorf("amendment content changed during the move")
	}
}
