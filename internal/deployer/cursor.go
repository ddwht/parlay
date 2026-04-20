package deployer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/internal/embedded"
)

// parlay-feature: parlay-tool/parlay-loop
// parlay-section: cross-cutting
//
// CursorDeployer deploys skills as .cursor/skills/parlay-*/SKILL.md
// and a single always-apply rule in .cursor/rules/parlay.mdc.
type CursorDeployer struct{}

func (d *CursorDeployer) Name() string { return "Cursor" }

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

%s`, skill.Name, skillTitle(skill.Name), string(skill.Content))

		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write skill %s: %w", skillPath, err)
		}
	}

	// Deploy subagents to .cursor/agents/parlay-<name>.md
	if err := writeCursorAgents(projectRoot); err != nil {
		return err
	}

	// Write a single always-apply rule for project context
	return writeCursorProjectRule(projectRoot, skills)
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
	var commands string
	for _, skill := range skills {
		commands += fmt.Sprintf("- `/parlay-%s` — %s\n", skill.Name, skillTitle(skill.Name))
	}

	content := fmt.Sprintf(`---
description: "Parlay project context and available skills"
alwaysApply: true
---

# Parlay Project

This project uses the Parlay intent-driven design toolkit.
All operations are available as /parlay-* slash commands.

## Available Commands

%s
## Schema Loading

Skills load schemas on-demand from .parlay/schemas/. Do not keep schema content in memory across commands.

## Interactive Questions

When a skill step says to "ask the user", "present options", or "wait for the user's response", you MUST use the AskUserQuestion tool to pause execution and collect the user's input before proceeding to the next step. Do not output the question as plain text and continue — the skill requires the user's answer to decide what to do next.

## File Ownership

Three-zone layout — strict ownership:
- **spec/intents/<feature>/** (designer-authored): intents.md, dialogs.md — ask permission before modifying
- **spec/intents/<feature>/** (generated, human-reviewed): surface.md, domain-model.md, *.page.md
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, .baseline.yaml — never user-facing
`, commands)

	rulesDir := filepath.Join(projectRoot, ".cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("failed to create .cursor/rules/: %w", err)
	}
	return os.WriteFile(filepath.Join(rulesDir, "parlay.mdc"), []byte(content), 0644)
}

func init() {
	Register("cursor", func() Deployer { return &CursorDeployer{} })
}
