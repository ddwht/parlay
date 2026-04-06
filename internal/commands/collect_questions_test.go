package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectQuestions_SingleFeature(t *testing.T) {
	dir := setupTestDir(t)

	// Create feature with questions
	featureDir := filepath.Join(dir, "spec", "intents", "test-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `# Test Feature

> Test

---

## First Intent

**Goal**: Do something
**Persona**: Admin
**Priority**: P0

**Questions**:
- How should errors be handled?
- What about timeouts?

---

## Second Intent

**Goal**: Do another thing
**Persona**: Admin

**Constraints**:
- Must be fast
`

	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	output, err := collectForFeature("test-feature")
	if err != nil {
		t.Fatal(err)
	}

	if output.Feature != "test-feature" {
		t.Errorf("Feature = %q, want %q", output.Feature, "test-feature")
	}
	if output.Count != 2 {
		t.Errorf("Count = %d, want 2", output.Count)
	}
	if len(output.Questions) != 2 {
		t.Fatalf("Questions count = %d, want 2", len(output.Questions))
	}
	if output.Questions[0].Intent != "First Intent" {
		t.Errorf("Questions[0].Intent = %q", output.Questions[0].Intent)
	}
	if output.Questions[0].Priority != "P0" {
		t.Errorf("Questions[0].Priority = %q, want P0", output.Questions[0].Priority)
	}
	if output.Questions[0].Question != "How should errors be handled?" {
		t.Errorf("Questions[0].Question = %q", output.Questions[0].Question)
	}
}

func TestCollectQuestions_DefaultPriority(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "no-priority")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Goal**: Test default priority
**Persona**: Admin

**Questions**:
- A question
`

	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	output, err := collectForFeature("no-priority")
	if err != nil {
		t.Fatal(err)
	}

	if output.Questions[0].Priority != "P1" {
		t.Errorf("Expected default priority P1, got %q", output.Questions[0].Priority)
	}
}

func TestCollectQuestions_NoQuestions(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "clean-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Clean Intent

**Goal**: No questions here
**Persona**: Admin
`

	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	output, err := collectForFeature("clean-feature")
	if err != nil {
		t.Fatal(err)
	}

	if output.Count != 0 {
		t.Errorf("Count = %d, want 0", output.Count)
	}
}
