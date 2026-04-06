package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/parlay/internal/config"
)

func TestAddFeature_CreatesFeatureFolder(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	err := runAddFeature(nil, []string{"upgrade", "plan", "creation"})
	if err != nil {
		t.Fatal(err)
	}

	featurePath := config.FeaturePath("upgrade-plan-creation")
	if _, err := os.Stat(featurePath); os.IsNotExist(err) {
		t.Error("feature directory not created")
	}
}

func TestAddFeature_CreatesIntentsMd(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	runAddFeature(nil, []string{"upgrade", "plan"})

	content, err := os.ReadFile(filepath.Join(config.FeaturePath("upgrade-plan"), "intents.md"))
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

	runAddFeature(nil, []string{"fleet", "overview"})

	path := filepath.Join(config.FeaturePath("fleet-overview"), "dialogs.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("dialogs.md not created")
	}
}

func TestAddFeature_RejectsDuplicate(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	runAddFeature(nil, []string{"my", "feature"})
	err := runAddFeature(nil, []string{"my", "feature"})

	if err == nil {
		t.Error("expected error for duplicate feature, got nil")
	}
}

func TestAddFeature_SlugifiesName(t *testing.T) {
	setupTestDir(t)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)

	runAddFeature(nil, []string{"Fleet", "Health", "Overview"})

	if _, err := os.Stat(config.FeaturePath("fleet-health-overview")); os.IsNotExist(err) {
		t.Error("slug not correctly derived from name")
	}
}
