// parlay-feature: parlay-tool/parlay-loop
// parlay-section: cross-cutting
package deployer

import (
	"fmt"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// GenericDeployer writes all skills into a single AGENT_INSTRUCTIONS.md for agents without specific integration.
type GenericDeployer struct{}

func (d *GenericDeployer) Name() string { return "Generic" }

// AgentSurfacePaths returns paths Generic's deployer writes at the
// repo-level root. Used by the multi-root forbidden-directory check.
func (d *GenericDeployer) AgentSurfacePaths() []string {
	return []string{"AGENT_INSTRUCTIONS.md"}
}

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
	content += "Beyond the skills above, `parlay` also exposes plain CLI utility commands with no AI step " +
		"(`parlay init`, `parlay validate`, `parlay repair`, `parlay status`, and others). " +
		"Run `parlay --help` for the full, always-current list, or `parlay <command> --help` for a specific command's flags. " +
		"This file intentionally does not mirror that list inline — the registered cobra command set is the single source of truth, " +
		"and a hand-copied list here drifts the moment a command is added, renamed, or removed.\n\n"

	if section := renderMultiRootSection(projectRoot); section != "" {
		content += section
	}

	return atomicWrite(filepath.Join(projectRoot, "AGENT_INSTRUCTIONS.md"), []byte(content))
}
