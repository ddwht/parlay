package deployer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/embedded"
)

// testSkills returns a small set of fake skills for testing deployers.
func testSkills() []embedded.SkillEntry {
	return []embedded.SkillEntry{
		{Name: "add-feature", Content: []byte("# Add Feature\nStep 1: do something\n")},
		{Name: "build-feature", Content: []byte("# Build Feature\nStep 1: build it\n")},
	}
}

func TestCursorDeployer_Layout(t *testing.T) {
	root := t.TempDir()
	skills := testSkills()

	d := &CursorDeployer{}
	if err := d.Deploy(root, skills); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	// Verify skills are in .cursor/skills/parlay-<name>/SKILL.md
	for _, skill := range skills {
		skillPath := filepath.Join(root, ".cursor", "skills", "parlay-"+skill.Name, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatalf("expected skill file at %s: %v", skillPath, err)
		}
		content := string(data)

		if !strings.Contains(content, "name: parlay-"+skill.Name) {
			t.Errorf("skill %s missing name frontmatter", skill.Name)
		}
		if !strings.Contains(content, "description: \"Parlay: ") {
			t.Errorf("skill %s missing description frontmatter", skill.Name)
		}
		if !strings.Contains(content, string(skill.Content)) {
			t.Errorf("skill %s missing body content", skill.Name)
		}
	}

	// Verify single always-apply rule at .cursor/rules/parlay.mdc
	rulePath := filepath.Join(root, ".cursor", "rules", "parlay.mdc")
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("expected rule file at %s: %v", rulePath, err)
	}
	rule := string(data)

	if !strings.Contains(rule, "alwaysApply: true") {
		t.Error("parlay.mdc missing alwaysApply: true")
	}
	for _, skill := range skills {
		if !strings.Contains(rule, "/parlay-"+skill.Name) {
			t.Errorf("parlay.mdc missing command listing for %s", skill.Name)
		}
	}

	// Verify NO skill .mdc files exist in .cursor/rules/
	entries, err := os.ReadDir(filepath.Join(root, ".cursor", "rules"))
	if err != nil {
		t.Fatalf("failed to read rules dir: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "parlay.mdc" {
			t.Errorf("unexpected file in .cursor/rules/: %s", entry.Name())
		}
	}
}

func TestClaudeDeployer_Layout(t *testing.T) {
	root := t.TempDir()
	skills := testSkills()

	d := &ClaudeDeployer{}
	if err := d.Deploy(root, skills); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	// Verify skills are in .claude/skills/parlay-<name>/SKILL.md
	for _, skill := range skills {
		skillPath := filepath.Join(root, ".claude", "skills", "parlay-"+skill.Name, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatalf("expected skill file at %s: %v", skillPath, err)
		}
		content := string(data)

		if !strings.Contains(content, "name: parlay-"+skill.Name) {
			t.Errorf("skill %s missing name frontmatter", skill.Name)
		}
		if !strings.Contains(content, string(skill.Content)) {
			t.Errorf("skill %s missing body content", skill.Name)
		}
	}

	// Verify CLAUDE.md exists with command listings
	data, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("expected CLAUDE.md: %v", err)
	}
	claudeMd := string(data)

	for _, skill := range skills {
		if !strings.Contains(claudeMd, "/parlay-"+skill.Name) {
			t.Errorf("CLAUDE.md missing command listing for %s", skill.Name)
		}
	}
}

func TestGenericDeployer_Layout(t *testing.T) {
	root := t.TempDir()
	skills := testSkills()

	d := &GenericDeployer{}
	if err := d.Deploy(root, skills); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "AGENT_INSTRUCTIONS.md"))
	if err != nil {
		t.Fatalf("expected AGENT_INSTRUCTIONS.md: %v", err)
	}
	content := string(data)

	for _, skill := range skills {
		if !strings.Contains(content, "parlay-"+skill.Name) {
			t.Errorf("AGENT_INSTRUCTIONS.md missing skill %s", skill.Name)
		}
	}
}

func TestRegistry(t *testing.T) {
	tests := []struct {
		name     string
		wantType string
	}{
		{"cursor", "Cursor"},
		{"claude code", "Claude Code"},
		{"generic", "Generic"},
	}
	for _, tt := range tests {
		d, err := Get(tt.name)
		if err != nil {
			t.Errorf("Get(%q) returned error: %v", tt.name, err)
			continue
		}
		if d.Name() != tt.wantType {
			t.Errorf("Get(%q).Name() = %q, want %q", tt.name, d.Name(), tt.wantType)
		}
	}

	// Unknown agent should fall back to generic
	d, err := Get("unknown-agent")
	if err != nil {
		t.Fatalf("Get(unknown) should fall back to generic: %v", err)
	}
	if d.Name() != "Generic" {
		t.Errorf("fallback deployer Name() = %q, want Generic", d.Name())
	}
}
