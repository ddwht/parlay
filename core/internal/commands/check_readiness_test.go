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

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "test-feature")

	hasError := false
	for _, i := range issues {
		if i.Code == "no-surface" || i.Code == "no-surface-no-infrastructure" {
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

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "test-feature")

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

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "test-feature")

	for _, i := range issues {
		if i.Severity == "error" {
			t.Errorf("unexpected error: %+v", i)
		}
	}
}

// A multi-target project configures its adapter via adapter-set.yaml and
// carries no (deprecated) prototype-framework field. The build-feature gate
// must treat that as configured, not block it.
func TestReadiness_BuildFeature_AdapterSetSatisfiesAdapterRequirement(t *testing.T) {
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

	// Config with NO prototype-framework, plus an adapter-set.yaml — the
	// modern multi-target shape.
	parlayDir := filepath.Join(dir, ".parlay")
	os.MkdirAll(parlayDir, 0755)
	os.WriteFile(filepath.Join(parlayDir, "config.yaml"), []byte("ai-agent: test\nsdd-framework: test\n"), 0644)
	adapterSet := `name: test-project
targets:
  presentation:
    adapter: some-ui-adapter
    root: internal/ui
  application:
    adapter: some-app-adapter
    root: .
`
	os.WriteFile(filepath.Join(parlayDir, "adapter-set.yaml"), []byte(adapterSet), 0644)

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "test-feature")

	for _, i := range issues {
		if i.Code == "no-adapter-configured" {
			t.Errorf("adapter-set.yaml should satisfy the adapter requirement, got: %+v", i)
		}
		if i.Severity == "error" {
			t.Errorf("unexpected error: %+v", i)
		}
	}
}

// With neither prototype-framework nor adapter-set.yaml, the adapter is
// genuinely unconfigured and the gate must still error.
func TestReadiness_BuildFeature_NoAdapterConfigured(t *testing.T) {
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
	os.WriteFile(filepath.Join(parlayDir, "config.yaml"), []byte("ai-agent: test\nsdd-framework: test\n"), 0644)

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "test-feature")

	hasError := false
	for _, i := range issues {
		if i.Code == "no-adapter-configured" {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected no-adapter-configured error, got: %+v", issues)
	}
}
