package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadiness_CreateSurface_Empty(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(""), 0644)

	issues := checkCreateSurfaceReadiness(featureDir)

	hasError := false
	for _, i := range issues {
		if i.Severity == "error" && i.Code == "no-intents" {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected no-intents error, got: %+v", issues)
	}
}

func TestReadiness_CreateSurface_MissingGoal(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	issues := checkCreateSurfaceReadiness(featureDir)

	hasError := false
	for _, i := range issues {
		if i.Code == "missing-goal" {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected missing-goal error, got: %+v", issues)
	}
}

func TestReadiness_CreateSurface_Valid(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Goal**: Do a thing
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)
	os.WriteFile(filepath.Join(featureDir, "dialogs.md"), []byte(""), 0644)

	issues := checkCreateSurfaceReadiness(featureDir)

	for _, i := range issues {
		if i.Severity == "error" {
			t.Errorf("unexpected error: %+v", i)
		}
	}
}

func TestReadiness_BuildFeature_NoSurface(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Goal**: Do a thing
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	issues := checkBuildFeatureReadiness(featureDir, "test-feature")

	hasError := false
	for _, i := range issues {
		if i.Code == "no-surface" {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected no-surface error, got: %+v", issues)
	}
}

func TestReadiness_BuildFeature_FragmentMissingPage(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Goal**: Do a thing
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	surface := `## My Fragment

**Shows**: Some data
**Source**: @test-feature/some-intent
`
	os.WriteFile(filepath.Join(featureDir, "surface.md"), []byte(surface), 0644)

	// Need a config for the build-feature stage
	parlayDir := filepath.Join(dir, ".parlay")
	os.MkdirAll(parlayDir, 0755)
	os.WriteFile(filepath.Join(parlayDir, "config.yaml"), []byte("ai-agent: test\nsdd-framework: test\nprototype-framework: go-cli\n"), 0644)

	issues := checkBuildFeatureReadiness(featureDir, "test-feature")

	hasError := false
	for _, i := range issues {
		if i.Code == "fragment-missing-page" {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected fragment-missing-page error, got: %+v", issues)
	}
}

func TestReadiness_BuildFeature_Valid(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Goal**: Do a thing
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)
	os.WriteFile(filepath.Join(featureDir, "dialogs.md"), []byte(""), 0644)

	surface := `## My Fragment

**Shows**: Some data
**Source**: @test-feature/some-intent
**Page**: dashboard
**Region**: main
`
	os.WriteFile(filepath.Join(featureDir, "surface.md"), []byte(surface), 0644)

	parlayDir := filepath.Join(dir, ".parlay")
	os.MkdirAll(parlayDir, 0755)
	os.WriteFile(filepath.Join(parlayDir, "config.yaml"), []byte("ai-agent: test\nsdd-framework: test\nprototype-framework: go-cli\n"), 0644)

	issues := checkBuildFeatureReadiness(featureDir, "test-feature")

	for _, i := range issues {
		if i.Severity == "error" {
			t.Errorf("unexpected error: %+v", i)
		}
	}
}
