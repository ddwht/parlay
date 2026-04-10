package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
	"gopkg.in/yaml.v3"
)

func TestSaveAndDetectDrift_NoDrift(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)
	os.MkdirAll(config.BuildPath("my-feature"), 0755)

	intents := `## Check Readiness

**Goal**: See if the cluster is ready.
**Persona**: Admin
**Objects**: cluster, upgrade

**Constraints**:
- Must show status

**Verify**:
- Readiness status is displayed
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	// Save baseline
	parsed, _ := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	baseline := Baseline{
		GeneratedAt: "2026-04-06T00:00:00Z",
		Intents:     make(map[string]IntentHash),
	}
	for _, intent := range parsed {
		baseline.Intents[intent.Slug] = hashIntent(intent)
	}
	data, _ := yaml.Marshal(baseline)
	os.WriteFile(baselinePath("my-feature"), data, 0644)

	// Check drift — should be none
	output, err := detectDrift("my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if output.HasDrift {
		t.Error("expected no drift, got drift")
	}
}

func TestDetectDrift_GoalChanged(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)
	os.MkdirAll(config.BuildPath("my-feature"), 0755)

	// Original intent
	original := `## Check Readiness

**Goal**: See if the cluster is ready.
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(original), 0644)

	// Save baseline from original
	parsed, _ := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	baseline := Baseline{
		GeneratedAt: "2026-04-06T00:00:00Z",
		Intents:     make(map[string]IntentHash),
	}
	for _, intent := range parsed {
		baseline.Intents[intent.Slug] = hashIntent(intent)
	}
	data, _ := yaml.Marshal(baseline)
	os.WriteFile(baselinePath("my-feature"), data, 0644)

	// Modify the goal
	modified := `## Check Readiness

**Goal**: Verify all prerequisites are met before upgrading.
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(modified), 0644)

	// Check drift
	output, err := detectDrift("my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if !output.HasDrift {
		t.Fatal("expected drift")
	}
	if len(output.Drifted) != 1 {
		t.Fatalf("Drifted = %d, want 1", len(output.Drifted))
	}
	if output.Drifted[0].Intent != "Check Readiness" {
		t.Errorf("Drifted intent = %q", output.Drifted[0].Intent)
	}
	// Goal changed
	found := false
	for _, f := range output.Drifted[0].ChangedFields {
		if f == "Goal" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Goal in changed fields, got %v", output.Drifted[0].ChangedFields)
	}
}

func TestDetectDrift_NewAndRemovedIntents(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)
	os.MkdirAll(config.BuildPath("my-feature"), 0755)

	// Baseline had two intents
	baseline := Baseline{
		GeneratedAt: "2026-04-06T00:00:00Z",
		Intents: map[string]IntentHash{
			"intent-a": hashIntent(parser.Intent{Title: "Intent A", Slug: "intent-a", Goal: "Do A", Persona: "Admin"}),
			"intent-b": hashIntent(parser.Intent{Title: "Intent B", Slug: "intent-b", Goal: "Do B", Persona: "Admin"}),
		},
	}
	data, _ := yaml.Marshal(baseline)
	os.WriteFile(baselinePath("my-feature"), data, 0644)

	// Current intents: A (unchanged) + C (new), B removed
	current := `## Intent A

**Goal**: Do A
**Persona**: Admin

---

## Intent C

**Goal**: Do C
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(current), 0644)

	output, err := detectDrift("my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if !output.HasDrift {
		t.Fatal("expected drift")
	}
	if len(output.NewIntents) != 1 || output.NewIntents[0] != "Intent C" {
		t.Errorf("NewIntents = %v, want [Intent C]", output.NewIntents)
	}
	if len(output.Removed) != 1 || output.Removed[0] != "intent-b" {
		t.Errorf("Removed = %v, want [intent-b]", output.Removed)
	}
	// Intent A should not be drifted
	if len(output.Drifted) != 0 {
		t.Errorf("Drifted = %d, want 0 (Intent A unchanged)", len(output.Drifted))
	}
}

func TestDetectDrift_NoBaseline(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Goal**: Do something
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	// No baseline file — should return no drift
	output, err := detectDrift("my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if output.HasDrift {
		t.Error("expected no drift when no baseline exists")
	}
}
