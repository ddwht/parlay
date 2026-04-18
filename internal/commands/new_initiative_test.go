package commands

// parlay-feature: initiatives
// parlay-component: EmptyInitiativeCreationResult
// parlay-artifact: test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/internal/config"
)

func TestNewInitiative_CreatesThreeTreeDirs(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)
	os.MkdirAll(filepath.Join(config.SpecDir, config.HandoffDir), 0755)
	os.MkdirAll(config.BuildRoot(), 0755)

	err := runNewInitiative(nil, []string{"auth", "overhaul"})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		filepath.Join(config.SpecDir, config.IntentsDir, "auth-overhaul"),
		filepath.Join(config.SpecDir, config.HandoffDir, "auth-overhaul"),
		filepath.Join(config.BuildRoot(), "auth-overhaul"),
	} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("directory not created: %s", path)
		}
	}
}

func TestNewInitiative_IdempotentSecondRun(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)
	os.MkdirAll(filepath.Join(config.SpecDir, config.HandoffDir), 0755)
	os.MkdirAll(config.BuildRoot(), 0755)

	runNewInitiative(nil, []string{"auth", "overhaul"})
	err := runNewInitiative(nil, []string{"auth", "overhaul"})

	if err != nil {
		t.Errorf("second run should succeed (idempotent), got: %v", err)
	}
}

func TestNewInitiative_RejectsOrphanFeatureCollision(t *testing.T) {
	setupTestDir(t)
	intentsRoot := filepath.Join(config.SpecDir, config.IntentsDir)
	os.MkdirAll(intentsRoot, 0755)
	os.MkdirAll(filepath.Join(config.SpecDir, config.HandoffDir), 0755)
	os.MkdirAll(config.BuildRoot(), 0755)

	featurePath := filepath.Join(intentsRoot, "password-reset")
	os.MkdirAll(featurePath, 0755)
	os.WriteFile(filepath.Join(featurePath, "intents.md"), []byte("# Password Reset\n"), 0644)

	err := runNewInitiative(nil, []string{"password-reset"})

	if err == nil {
		t.Error("expected top-level collision error, got nil")
	}
}

func TestNewInitiative_AllowsSameSlugAsNestedFeature(t *testing.T) {
	setupTestDir(t)
	intentsRoot := filepath.Join(config.SpecDir, config.IntentsDir)
	os.MkdirAll(intentsRoot, 0755)
	os.MkdirAll(filepath.Join(config.SpecDir, config.HandoffDir), 0755)
	os.MkdirAll(config.BuildRoot(), 0755)

	nestedPath := filepath.Join(intentsRoot, "auth-overhaul", "password-reset")
	os.MkdirAll(nestedPath, 0755)
	os.WriteFile(filepath.Join(nestedPath, "intents.md"), []byte("# Password Reset\n"), 0644)

	err := runNewInitiative(nil, []string{"password-reset"})

	if err != nil {
		t.Errorf("nested feature should not block top-level initiative, got: %v", err)
	}
}
