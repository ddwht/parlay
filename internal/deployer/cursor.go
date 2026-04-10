package deployer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/internal/embedded"
)

// CursorDeployer deploys skills as .cursor/rules/parlay-*.mdc for Cursor.
type CursorDeployer struct{}

func (d *CursorDeployer) Name() string { return "Cursor" }

func (d *CursorDeployer) Deploy(projectRoot string, skills []embedded.SkillEntry) error {
	rulesDir := filepath.Join(projectRoot, ".cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("failed to create .cursor/rules/: %w", err)
	}

	// Deploy each skill as an .mdc file with frontmatter
	for _, skill := range skills {
		frontmatter := fmt.Sprintf(`---
description: "Parlay skill: %s"
alwaysApply: false
---

`, skillTitle(skill.Name))
		content := frontmatter + string(skill.Content)
		path := filepath.Join(rulesDir, "parlay-"+skill.Name+".mdc")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write rule %s: %w", path, err)
		}
	}

	// Write a parlay-project.mdc with alwaysApply: true for project context
	return writeCursorProjectRule(projectRoot, skills)
}

func writeCursorProjectRule(projectRoot string, skills []embedded.SkillEntry) error {
	var commands string
	for _, skill := range skills {
		commands += fmt.Sprintf("- parlay-%s — %s\n", skill.Name, skillTitle(skill.Name))
	}

	content := fmt.Sprintf(`---
description: "Parlay project context and available skills"
alwaysApply: true
---

# Parlay Project

This project uses the Parlay intent-driven design toolkit.
Skills are available as .cursor/rules/parlay-*.mdc files.

## Available Skills

%s
## Schema Loading

Load schemas on-demand from .parlay/schemas/. Do not keep schema content in memory across commands.

## File Ownership

Three-zone layout — strict ownership:
- **spec/intents/<feature>/** (designer-authored): intents.md, dialogs.md — ask permission before modifying
- **spec/intents/<feature>/** (generated, human-reviewed): surface.md, domain-model.md, *.page.md
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, .baseline.yaml — never user-facing
`, commands)

	rulesDir := filepath.Join(projectRoot, ".cursor", "rules")
	return os.WriteFile(filepath.Join(rulesDir, "parlay-project.mdc"), []byte(content), 0644)
}

func init() {
	Register("cursor", func() Deployer { return &CursorDeployer{} })
}
