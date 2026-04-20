// parlay-feature: parlay-tool/parlay-loop
// parlay-section: cross-cutting
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

	// Phase-group guidance — the Generic adapter has no native sub-agents,
	// so we embed the designer/build/code agent definitions as reference text.
	// parlay-loop detects this adapter and uses fresh-session handoff between groups.
	agents, err := embedded.ReadAllAgents()
	if err != nil {
		return fmt.Errorf("failed to read embedded agents: %w", err)
	}
	if len(agents) > 0 {
		content += "---\n\n## Phase-Groups (parlay-loop)\n\n"
		content += "This adapter has no native sub-agent support. The definitions below describe the three phase-group roles; parlay-loop uses fresh-session handoff between them.\n\n"
		for _, a := range agents {
			content += fmt.Sprintf("### parlay-%s\n\n%s\n\n", a.Name, string(a.Content))
		}
	}

	content += "---\n\n## Interactive Questions\n\n"
	content += "When a skill step says to \"ask the user\", \"present options\", or \"wait for the user's response\", "
	content += "you MUST present the question and stop. Do not continue to the next step until the user has responded. "
	content += "The skill requires the user's answer to decide what to do next.\n\n"
	content += "---\n\n## CLI Utility Commands\n\n"
	content += "- `parlay init` — Bootstrap project\n"
	content += "- `parlay add-feature <name> [--initiative <name>]` — Create feature folder\n"
	content += "- `parlay new-initiative <name>` — Create empty initiative directory\n"
	content += "- `parlay move-feature @<feature> --to <initiative> | --out` — Relocate a feature\n"
	content += "- `parlay create-dialogs @<feature>` — Scaffold dialog templates\n"
	content += "- `parlay simplify` — Detect duplicated helpers and propose extractions\n"
	content += "- `parlay validate --type <type> <path>` — Validate a file\n"
	content += "- `parlay parse --type <type> <path>` — Parse to JSON\n"
	content += "- `parlay check-coverage @<feature>` — Coverage check (JSON)\n"
	content += "- `parlay view-page <page>` — Assemble page view\n"
	content += "- `parlay lock-page <page>` — Lock page layout\n"
	content += "- `parlay repair [--dry-run] [--yes]` — Validate and reconcile the three parallel trees\n"
	content += "- `parlay loop <@feature> [--from <phase>]` — Walk a feature end-to-end through the design pipeline\n"
	content += "- `parlay upgrade` — Redeploy skills and schemas from the binary\n"

	return os.WriteFile(filepath.Join(projectRoot, "AGENT_INSTRUCTIONS.md"), []byte(content), 0644)
}
