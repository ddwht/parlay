package deployer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// testSkills returns a small set of fake skills for testing deployers.
// Description mirrors what embedded.ReadAllSkills would parse out of a
// real skill's frontmatter — deployers read it directly off the field
// rather than deriving it, so fixtures need it populated to exercise
// that path meaningfully.
func testSkills() []embedded.SkillEntry {
	return []embedded.SkillEntry{
		{Name: "add-feature", Description: "Create a new feature", Content: []byte("# Add Feature\nStep 1: do something\n")},
		{Name: "build-feature", Description: "Generate buildfile and testcases", Content: []byte("# Build Feature\nStep 1: build it\n")},
	}
}

func TestCursorDeployer_Layout(t *testing.T) {
	root := t.TempDir()
	skills := testSkills()

	d := &CursorDeployer{}
	if _, err := d.Deploy(root, skills); err != nil {
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
	if _, err := d.Deploy(root, skills); err != nil {
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
	if _, err := d.Deploy(root, skills); err != nil {
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

// TestSkillDescription_OnboardSkill guards the single-sourced
// replacement for the old skillTitle map: the "onboard" skill's
// human-facing description now lives in its own frontmatter
// (embedded/skills/onboard.skill.md) and is parsed out by
// embedded.ReadAllSkills, not hand-duplicated in this package.
func TestSkillDescription_OnboardSkill(t *testing.T) {
	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	for _, s := range skills {
		if s.Name != "onboard" {
			continue
		}
		if s.Description != "Onboard existing codebase and draft adapter" {
			t.Errorf("onboard skill Description = %q, want %q", s.Description, "Onboard existing codebase and draft adapter")
		}
		return
	}
	t.Fatal("onboard skill not found in embedded bundle")
}

// parlay-feature: parlay-tool/parlay-loop
// parlay-component: ClaudeAdapterSubagentDeployment
// parlay-artifact: test
func TestClaudeDeployer_DeploysSubagents(t *testing.T) {
	root := t.TempDir()
	d := &ClaudeDeployer{}
	if _, err := d.Deploy(root, testSkills()); err != nil {
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
	if _, err := d.Deploy(root, testSkills()); err != nil {
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
	if _, err := d.Deploy(root, testSkills()); err != nil {
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
		"## CLI Utility Commands",
		"parlay --help",
	}
	for _, want := range wantSections {
		if !strings.Contains(content, want) {
			t.Errorf("AGENT_INSTRUCTIONS.md missing expected section/text: %q", want)
		}
	}
}

// TestGenericDeployer_SkillsCarryExpandedActiveRootProse guards a
// structural property of the marker-expansion design: expansion happens
// once, inside embedded.ReadAllSkills, upstream of every deployer — not
// per-deployer. GenericDeployer never re-implements the substitution
// (it just concatenates skill.Content into AGENT_INSTRUCTIONS.md), so
// this asserts real skill content flowing through Generic ends up with
// the marker fully expanded, exactly like Claude and Cursor get it, and
// never leaks the raw `<!-- parlay:expand-active-root -->` placeholder
// to a generic-adapter project.
func TestGenericDeployer_SkillsCarryExpandedActiveRootProse(t *testing.T) {
	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	root := t.TempDir()
	if _, err := (&GenericDeployer{}).Deploy(root, skills); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENT_INSTRUCTIONS.md"))
	if err != nil {
		t.Fatalf("AGENT_INSTRUCTIONS.md missing: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "<!-- parlay:expand-active-root -->") {
		t.Error("AGENT_INSTRUCTIONS.md leaks the unexpanded active-root marker — Generic must receive already-expanded skill content")
	}
	if !strings.Contains(content, "## Active root") {
		t.Error("AGENT_INSTRUCTIONS.md is missing the expanded '## Active root' prose — expected at least one embedded skill to carry the marker")
	}
}

// parlay-feature: parlay-tool/parlay-loop
// parlay-component: ClaudeAdapterSubagentDeployment
// parlay-artifact: test
//
// TestSkillDescription_Loop is the loop-skill counterpart to
// TestSkillDescription_OnboardSkill — same single-sourcing guard.
func TestSkillDescription_Loop(t *testing.T) {
	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	want := "Walk a feature end-to-end through the parlay design pipeline"
	for _, s := range skills {
		if s.Name != "loop" {
			continue
		}
		if s.Description != want {
			t.Errorf("loop skill Description = %q, want %q", s.Description, want)
		}
		return
	}
	t.Fatal("loop skill not found in embedded bundle")
}

// TestClaudeAndCursorProjectConfigBodyIdentical is the general parity
// guard for the single-templated-source refactor: both ClaudeDeployer's
// CLAUDE.md and CursorDeployer's parlay.mdc render their project-config
// body from the one shared renderProjectConfigBody function (Available
// Commands, Schema Loading, Interactive Questions, File Ownership,
// Multi-Root Layout). Before that refactor each deployer inlined its
// own copy of this text — which is exactly how Cursor's File Ownership
// section drifted to a stale, non-current description of the
// spec-artifact set while Claude's stayed current. This test strips
// each file down to just that shared body (Claude's parlay-marker
// contents; Cursor's content after its frontmatter) and asserts the two
// are byte-for-byte identical, so any future re-inlining or
// per-deployer edit of the shared block fails the build immediately
// instead of silently drifting again.
func TestClaudeAndCursorProjectConfigBodyIdentical(t *testing.T) {
	skills := testSkills()

	claudeRoot := t.TempDir()
	if _, err := (&ClaudeDeployer{}).Deploy(claudeRoot, skills); err != nil {
		t.Fatalf("Claude Deploy failed: %v", err)
	}
	claudeMd, err := os.ReadFile(filepath.Join(claudeRoot, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("expected CLAUDE.md: %v", err)
	}
	// A fresh deploy (no pre-existing CLAUDE.md) appends one trailing
	// newline after the closing marker — see writeCLAUDEmd's
	// no-existing-file branch.
	if !strings.HasPrefix(string(claudeMd), parlayMarkerBegin+"\n") || !strings.HasSuffix(string(claudeMd), parlayMarkerEnd+"\n") {
		t.Fatalf("CLAUDE.md is not wrapped in the expected parlay markers:\n%s", claudeMd)
	}
	claudeBody := strings.TrimSuffix(strings.TrimPrefix(string(claudeMd), parlayMarkerBegin+"\n"), parlayMarkerEnd+"\n")

	cursorRoot := t.TempDir()
	if _, err := (&CursorDeployer{}).Deploy(cursorRoot, skills); err != nil {
		t.Fatalf("Cursor Deploy failed: %v", err)
	}
	cursorRule, err := os.ReadFile(filepath.Join(cursorRoot, ".cursor", "rules", "parlay.mdc"))
	if err != nil {
		t.Fatalf("expected parlay.mdc: %v", err)
	}
	const cursorHeader = "---\ndescription: \"Parlay project context and available skills\"\nalwaysApply: true\n---\n\n"
	if !strings.HasPrefix(string(cursorRule), cursorHeader) {
		t.Fatalf("parlay.mdc does not start with the expected Cursor frontmatter header:\n%s", cursorRule)
	}
	cursorBody := strings.TrimPrefix(string(cursorRule), cursorHeader)

	if claudeBody != cursorBody {
		t.Errorf("CLAUDE.md and parlay.mdc project-config bodies differ despite both being rendered from renderProjectConfigBody.\nCLAUDE.md body:\n%s\n\nparlay.mdc body:\n%s", claudeBody, cursorBody)
	}
	// Sanity: make sure the shared body isn't accidentally empty (which
	// would make the equality check above vacuously true).
	if !strings.Contains(claudeBody, "## File Ownership") || !strings.Contains(claudeBody, "## Available Commands") {
		t.Error("shared project-config body is missing expected sections — test fixture may have drifted")
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
