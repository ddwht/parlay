package deployer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/internal/embedded"
)

// GenericDeployer writes all skills into a single AGENT_INSTRUCTIONS.md for agents without specific integration.
type GenericDeployer struct{}

func (d *GenericDeployer) Name() string { return "Generic" }

func (d *GenericDeployer) Deploy(projectRoot string, skills []embedded.SkillEntry) error {
	var content string
	content += "# Parlay Agent Instructions\n\n"
	content += "This project uses the Parlay intent-driven design toolkit.\n"
	content += "Below are the available skills. Execute them when the user requests.\n\n"

	for _, skill := range skills {
		content += fmt.Sprintf("---\n\n## Skill: parlay-%s\n\n%s\n\n", skill.Name, string(skill.Content))
	}

	content += "---\n\n## Interactive Questions\n\n"
	content += "When a skill step says to \"ask the user\", \"present options\", or \"wait for the user's response\", "
	content += "you MUST present the question and stop. Do not continue to the next step until the user has responded. "
	content += "The skill requires the user's answer to decide what to do next.\n\n"
	content += "---\n\n## CLI Utility Commands\n\n"
	content += "- `parlay init` — Bootstrap project\n"
	content += "- `parlay add-feature <name>` — Create feature folder\n"
	content += "- `parlay create-dialogs @<feature>` — Scaffold dialog templates\n"
	content += "- `parlay validate --type <type> <path>` — Validate a file\n"
	content += "- `parlay parse --type <type> <path>` — Parse to JSON\n"
	content += "- `parlay check-coverage @<feature>` — Coverage check (JSON)\n"
	content += "- `parlay view-page <page>` — Assemble page view\n"
	content += "- `parlay lock-page <page>` — Lock page layout\n"

	return os.WriteFile(filepath.Join(projectRoot, "AGENT_INSTRUCTIONS.md"), []byte(content), 0644)
}
