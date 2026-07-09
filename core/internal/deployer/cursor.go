package deployer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// parlay-feature: parlay-tool/parlay-loop
// parlay-section: cross-cutting
// parlay-extends: parlay-tool/create-domain-model/deployer-cursor-stale-skill-cleanup
//
// CursorDeployer deploys skills as .cursor/skills/parlay-*/SKILL.md
// and a single always-apply rule in .cursor/rules/parlay.mdc.
type CursorDeployer struct{}

func (d *CursorDeployer) Name() string { return "Cursor" }

// AgentSurfacePaths returns paths Cursor's deployer writes at the
// repo-level root. Used by the multi-root forbidden-directory check.
func (d *CursorDeployer) AgentSurfacePaths() []string {
	return []string{
		".cursor/skills",
		".cursor/agents",
		".cursor/rules",
	}
}

func (d *CursorDeployer) Deploy(projectRoot string, skills []embedded.SkillEntry) error {
	// Deploy each skill as .cursor/skills/parlay-<name>/SKILL.md
	for _, skill := range skills {
		skillDir := filepath.Join(projectRoot, ".cursor", "skills", "parlay-"+skill.Name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return fmt.Errorf("failed to create skill directory %s: %w", skillDir, err)
		}

		content := fmt.Sprintf(`---
name: parlay-%s
description: "Parlay: %s"
---

%s`, skill.Name, skill.Description, string(skill.Content))

		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write skill %s: %w", skillPath, err)
		}
	}

	// parlay-extends: parlay-tool/create-domain-model/deployer-cursor-stale-skill-cleanup
	// Stale-skill cleanup pass — symmetric with the Claude
	// deployer. After the wanted skills are written and before
	// agents and the project rule are regenerated, prune any
	// .cursor/skills/parlay-<old-slug>/ directory whose slug is not
	// present in the embedded skills set.
	wanted := make(map[string]bool, len(skills))
	for _, s := range skills {
		wanted[s.Name] = true
	}
	if err := pruneStaleCursorSkills(projectRoot, wanted); err != nil {
		return err
	}

	// Deploy subagents to .cursor/agents/parlay-<name>.md
	if err := writeCursorAgents(projectRoot); err != nil {
		return err
	}

	// Write a single always-apply rule for project context
	return writeCursorProjectRule(projectRoot, skills)
}

// pruneStaleCursorSkills removes any .cursor/skills/parlay-<slug>/
// directory whose slug is not present in the wanted set. Same shape
// as pruneStaleClaudeSkills — user-owned directories without the
// parlay- prefix are preserved, and a missing .cursor/skills/ is a
// no-op.
func pruneStaleCursorSkills(projectRoot string, wanted map[string]bool) error {
	skillsDir := filepath.Join(projectRoot, ".cursor", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	const prefix = "parlay-"
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		slug := strings.TrimPrefix(name, prefix)
		if wanted[slug] {
			continue
		}
		stale := filepath.Join(skillsDir, name)
		if err := os.RemoveAll(stale); err != nil {
			return fmt.Errorf("prune stale Cursor skill %s: %w", stale, err)
		}
	}
	return nil
}

func writeCursorAgents(projectRoot string) error {
	agents, err := embedded.ReadAllAgents()
	if err != nil {
		return fmt.Errorf("failed to read embedded agents: %w", err)
	}
	agentsDir := filepath.Join(projectRoot, ".cursor", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create .cursor/agents/: %w", err)
	}
	for _, a := range agents {
		path := filepath.Join(agentsDir, "parlay-"+a.Name+".md")
		if err := os.WriteFile(path, a.Content, 0644); err != nil {
			return fmt.Errorf("failed to write agent %s: %w", path, err)
		}
	}
	return nil
}

func writeCursorProjectRule(projectRoot string, skills []embedded.SkillEntry) error {
	header := "---\n" +
		`description: "Parlay project context and available skills"` + "\n" +
		"alwaysApply: true\n" +
		"---"

	content := header + "\n\n" + renderProjectConfigBody(skills, projectRoot)

	rulesDir := filepath.Join(projectRoot, ".cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("failed to create .cursor/rules/: %w", err)
	}
	return os.WriteFile(filepath.Join(rulesDir, "parlay.mdc"), []byte(content), 0644)
}

func init() {
	Register("cursor", func() Deployer { return &CursorDeployer{} })
}
