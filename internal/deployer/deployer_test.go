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

func TestSkillTitle_OnboardSkill(t *testing.T) {
	title := skillTitle("onboard")
	if title != "Onboard existing codebase and draft adapter" {
		t.Errorf("skillTitle(onboard) = %q, want %q", title, "Onboard existing codebase and draft adapter")
	}
}

// parlay-feature: parlay-tool/parlay-loop
// parlay-component: ClaudeAdapterSubagentDeployment
// parlay-artifact: test
func TestClaudeDeployer_DeploysSubagents(t *testing.T) {
	root := t.TempDir()
	d := &ClaudeDeployer{}
	if err := d.Deploy(root, testSkills()); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	wantAgents := []string{"parlay-designer", "parlay-build", "parlay-code"}
	for _, name := range wantAgents {
		path := filepath.Join(root, ".claude", "agents", name+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected agent file at %s: %v", path, err)
		}
		if !strings.Contains(string(data), "name: "+name) {
			t.Errorf("agent %s missing name frontmatter", name)
		}
	}

	// Pre-existing skill and CLAUDE.md deployment must still work.
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "parlay-add-feature", "SKILL.md")); err != nil {
		t.Errorf("skill file missing after agent deployment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md missing after agent deployment: %v", err)
	}
}

// parlay-feature: parlay-tool/parlay-loop
// parlay-component: CursorAdapterSubagentDeployment
// parlay-artifact: test
func TestCursorDeployer_DeploysSubagents(t *testing.T) {
	root := t.TempDir()
	d := &CursorDeployer{}
	if err := d.Deploy(root, testSkills()); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	wantAgents := []string{"parlay-designer", "parlay-build", "parlay-code"}
	for _, name := range wantAgents {
		path := filepath.Join(root, ".cursor", "agents", name+".md")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected agent file at %s: %v", path, err)
		}
	}

	// Existing Cursor layout must still be intact.
	if _, err := os.Stat(filepath.Join(root, ".cursor", "skills", "parlay-add-feature", "SKILL.md")); err != nil {
		t.Errorf("skill file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "rules", "parlay.mdc")); err != nil {
		t.Errorf("parlay.mdc missing: %v", err)
	}
}

// parlay-feature: parlay-tool/parlay-loop
// parlay-component: GenericAdapterSubagentFallback
// parlay-artifact: test
func TestGenericDeployer_EmbedsPhaseGroups(t *testing.T) {
	root := t.TempDir()
	d := &GenericDeployer{}
	if err := d.Deploy(root, testSkills()); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "AGENT_INSTRUCTIONS.md"))
	if err != nil {
		t.Fatalf("AGENT_INSTRUCTIONS.md missing: %v", err)
	}
	content := string(data)

	wantSections := []string{
		"## Phase-Groups (parlay-loop)",
		"### parlay-designer",
		"### parlay-build",
		"### parlay-code",
		"parlay loop <@feature>",
	}
	for _, want := range wantSections {
		if !strings.Contains(content, want) {
			t.Errorf("AGENT_INSTRUCTIONS.md missing expected section/text: %q", want)
		}
	}
}

// parlay-feature: parlay-tool/parlay-loop
// parlay-component: ClaudeAdapterSubagentDeployment
// parlay-artifact: test
func TestSkillTitle_Loop(t *testing.T) {
	got := skillTitle("loop")
	want := "Walk a feature end-to-end through the parlay design pipeline"
	if got != want {
		t.Errorf("skillTitle(loop) = %q, want %q", got, want)
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
