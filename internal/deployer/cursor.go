package deployer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/parlay/internal/embedded"
)

// CursorDeployer deploys skills as .cursor/skills/parlay-*.md for Cursor.
type CursorDeployer struct{}

func (d *CursorDeployer) Name() string { return "Cursor" }

func (d *CursorDeployer) Deploy(projectRoot string, skills []embedded.SkillEntry) error {
	skillsDir := filepath.Join(projectRoot, ".cursor", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create .cursor/skills/: %w", err)
	}

	for _, skill := range skills {
		path := filepath.Join(skillsDir, "parlay-"+skill.Name+".md")
		if err := os.WriteFile(path, skill.Content, 0644); err != nil {
			return fmt.Errorf("failed to write skill %s: %w", path, err)
		}
	}

	// Write .cursorrules with project context (not skills — those are in .cursor/skills/)
	return writeCursorRules(projectRoot, skills)
}

func writeCursorRules(projectRoot string, skills []embedded.SkillEntry) error {
	var commands string
	for _, skill := range skills {
		commands += fmt.Sprintf("- parlay-%s — %s\n", skill.Name, skillTitle(skill.Name))
	}

	content := fmt.Sprintf(`# Parlay Project

This project uses the Parlay intent-driven design toolkit.
Skills are available in .cursor/skills/parlay-*.md.

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

	return os.WriteFile(filepath.Join(projectRoot, ".cursorrules"), []byte(content), 0644)
}

func init() {
	Register("cursor", func() Deployer { return &CursorDeployer{} })
}
