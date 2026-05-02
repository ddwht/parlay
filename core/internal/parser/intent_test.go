package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIntentsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "intents.md")

	content := `# Test Feature

> A test feature.

---

## Check Readiness

**Goal**: See if the cluster is ready.
**Persona**: Admin
**Priority**: P0
**Context**: Before upgrading.
**Action**: Open detail page.
**Objects**: cluster, upgrade

**Constraints**:
- Must show status
- Must not require manual checks

**Verify**:
- Readiness status is displayed for each cluster
- Partial readiness shows a warning indicator

**Questions**:
- What about partial readiness?

---

## Start Upgrade

**Goal**: Begin the upgrade.
**Persona**: Admin
`

	os.WriteFile(path, []byte(content), 0644)

	intents, err := ParseIntentsFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(intents) != 2 {
		t.Fatalf("expected 2 intents, got %d", len(intents))
	}

	i := intents[0]
	if i.Title != "Check Readiness" {
		t.Errorf("Title = %q, want %q", i.Title, "Check Readiness")
	}
	if i.Slug != "check-readiness" {
		t.Errorf("Slug = %q, want %q", i.Slug, "check-readiness")
	}
	if i.Goal != "See if the cluster is ready." {
		t.Errorf("Goal = %q", i.Goal)
	}
	if i.Persona != "Admin" {
		t.Errorf("Persona = %q", i.Persona)
	}
	if i.Priority != "P0" {
		t.Errorf("Priority = %q, want %q", i.Priority, "P0")
	}
	if i.Context != "Before upgrading." {
		t.Errorf("Context = %q", i.Context)
	}
	if i.Action != "Open detail page." {
		t.Errorf("Action = %q", i.Action)
	}
	if len(i.Objects) != 2 {
		t.Errorf("Objects count = %d, want 2", len(i.Objects))
	}
	if len(i.Constraints) != 2 {
		t.Errorf("Constraints count = %d, want 2", len(i.Constraints))
	}
	if len(i.Verify) != 2 {
		t.Errorf("Verify count = %d, want 2", len(i.Verify))
	}
	if len(i.Questions) != 1 {
		t.Errorf("Questions count = %d, want 1", len(i.Questions))
	}

	// Minimal intent — only required fields
	i2 := intents[1]
	if i2.Title != "Start Upgrade" {
		t.Errorf("Title = %q", i2.Title)
	}
	if i2.Priority != "" {
		t.Errorf("Priority should be empty, got %q", i2.Priority)
	}
	if i2.Context != "" {
		t.Errorf("Context should be empty, got %q", i2.Context)
	}
}
