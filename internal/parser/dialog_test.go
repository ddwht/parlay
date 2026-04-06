package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDialogsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dialogs.md")

	content := `# Test — Dialogs

---

### Check Readiness

**Trigger**: Open cluster detail

User: I want to check readiness.
System: Let me check.
System (background): Evaluates cluster status.
System: The cluster is eligible.

---

### Resolve Blockers

User: Fix the critical blocker.
System: Options:
  A: Restart instances
  B: Replace instances
  C: ==Custom remediation==
User: Selects A
System (background): Restarts instances.
System (condition: success): Instances restarted.
System (condition: failure): Restart failed.

---
`

	os.WriteFile(path, []byte(content), 0644)

	dialogs, err := ParseDialogsFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(dialogs) != 2 {
		t.Fatalf("expected 2 dialogs, got %d", len(dialogs))
	}

	d1 := dialogs[0]
	if d1.Title != "Check Readiness" {
		t.Errorf("Title = %q", d1.Title)
	}
	if d1.Trigger != "Open cluster detail" {
		t.Errorf("Trigger = %q", d1.Trigger)
	}
	if len(d1.Turns) != 4 {
		t.Errorf("Turns count = %d, want 4", len(d1.Turns))
	}

	// Check background turn
	bgTurn := d1.Turns[2]
	if bgTurn.Type != "background" {
		t.Errorf("Turn type = %q, want background", bgTurn.Type)
	}

	d2 := dialogs[1]
	if len(d2.Turns) < 4 {
		t.Fatalf("Turns count = %d, want at least 4", len(d2.Turns))
	}

	// Check options on system turn
	optionTurn := d2.Turns[1] // "System: Options:"
	if len(optionTurn.Options) != 3 {
		t.Errorf("Options count = %d, want 3", len(optionTurn.Options))
	}

	if optionTurn.Options[2].IsFreeform != true {
		t.Error("Third option should be freeform")
	}

	// Check conditional turns
	var conditionals int
	for _, turn := range d2.Turns {
		if turn.Type == "conditional" {
			conditionals++
		}
	}
	if conditionals != 2 {
		t.Errorf("conditional turns = %d, want 2", conditionals)
	}
}
