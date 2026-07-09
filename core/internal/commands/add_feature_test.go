package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
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

	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul")
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

	runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul")
	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "sso setup", "sso-setup", "auth overhaul")

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

	runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul")
	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul")

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

	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "login", "login", "password-reset")

	if err == nil {
		t.Error("expected top-level collision error, got nil")
	}
}

func TestAddFeatureWithInitiative_SameSlugDifferentInitiative(t *testing.T) {
	setupTestDir(t)
	for _, root := range threeTreeRoots(testContext(t)) {
		os.MkdirAll(root, 0755)
	}

	runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "auth overhaul")
	err := runAddFeatureWithInitiative(testCommandWithContext(t, testContext(t)), testContext(t), "password reset", "password-reset", "billing")

	if err != nil {
		t.Errorf("same slug in different initiative should succeed, got: %v", err)
	}
}
