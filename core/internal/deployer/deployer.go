package deployer

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/embedded"
)

// Deployer packages agent-agnostic skills into agent-specific format.
type Deployer interface {
	// Name returns the agent identifier.
	Name() string

	// Deploy writes skill files and agent config to the project.
	Deploy(projectRoot string, skills []embedded.SkillEntry) error

	// AgentSurfacePaths returns the deployer-owned paths (relative to a
	// parlay root) that must NOT exist inside a child root — they live
	// only at the repo-level root. Each entry is interpreted as a
	// directory or file name relative to the root being checked.
	AgentSurfacePaths() []string
}

// fileOwnershipSection is the canonical "File Ownership" doctrine block —
// the three-zone layout and the four co-equal spec artifacts — rendered
// verbatim into every agent's project-config file (CLAUDE.md, Cursor's
// parlay.mdc, ...). A single constant here is what keeps deployers from
// drifting out of sync with each other as the artifact set evolves.
const fileOwnershipSection = `## File Ownership

Three-zone layout — strict ownership:
- **spec/intents/<feature>/** (designer-authored): intents.md, dialogs.md — ask permission before modifying
- **spec/intents/<feature>/** (generated, human-reviewed): four co-equal spec artifacts —
  - **surface.yaml** (or surface.md) — visible output, page assemblies, dialog turns
  - **capabilities.yaml** — operation-shaped backend behavior (closed-vocabulary commands and queries)
  - **infrastructure.md** — architectural prose for boundaries, probes, allowlists, dependency pins, and other concerns that do not reduce to operations
  - **domain-model.yaml** (or domain-model.md) — entities, relationships, and shared vocabulary
  - Plus *.page.md for per-page layouts. A feature picks whichever artifacts it needs; capabilities.yaml and infrastructure.md are co-equal, not stand-ins for each other.
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, coverage-review.yaml, .baseline.yaml — never user-facing
- **.parlay/adapter-set.yaml** (tool config, project-owned): pins adapter slot topology — multi-target projects only
`

// renderAvailableCommands returns the "- `/parlay-<name>` — <description>"
// list every deployer's Available Commands section is built from. One
// line per skill, description sourced from the skill's own frontmatter
// (embedded.ReadAllSkills), not a per-deployer title map.
func renderAvailableCommands(skills []embedded.SkillEntry) string {
	var commands string
	for _, skill := range skills {
		commands += fmt.Sprintf("- `/parlay-%s` — %s\n", skill.Name, skill.Description)
	}
	return commands
}

// renderProjectConfigBody renders the full templated project-config
// body shared by every agent adapter that writes a persistent project
// context file (CLAUDE.md, Cursor's parlay.mdc, ...): the intro, the
// Available Commands list, Schema Loading, Interactive Questions, File
// Ownership, and — when the project has registered child roots — the
// Multi-Root Layout section.
//
// This is the single template both ClaudeDeployer and CursorDeployer
// render from. Before this existed the two deployers each inlined their
// own copy of this text, which is exactly how Cursor's File Ownership
// section drifted out of sync with Claude's. Rendering both from one
// function makes that drift structurally impossible: change the text
// once, here, and every deployer picks it up.
//
// The caller is responsible for any deployer-specific wrapping around
// this body — frontmatter, `<!-- parlay:begin/end -->` markers, etc.
func renderProjectConfigBody(skills []embedded.SkillEntry, projectRoot string) string {
	return fmt.Sprintf(`# Parlay Project

This project uses the Parlay intent-driven design toolkit.
All operations are available as /parlay-* slash commands.

## Available Commands

%s
## Schema Loading

Skills load schemas on-demand from .parlay/schemas/. Do not keep schema content in memory across commands.

## Interactive Questions

When a skill step says to "ask the user", "present options", or "wait for the user's response", you MUST use the AskUserQuestion tool to pause execution and collect the user's input before proceeding to the next step. Do not output the question as plain text and continue — the skill requires the user's answer to decide what to do next.

%s%s`, renderAvailableCommands(skills), fileOwnershipSection, renderMultiRootSection(projectRoot))
}

var registry = map[string]func() Deployer{}

// Register adds a deployer factory.
func Register(name string, factory func() Deployer) {
	registry[strings.ToLower(name)] = factory
}

// Get returns the deployer for the given agent name.
func Get(name string) (Deployer, error) {
	factory, ok := registry[strings.ToLower(name)]
	if !ok {
		// Fall back to generic
		if f, ok := registry["generic"]; ok {
			return f(), nil
		}
		return nil, fmt.Errorf("no deployer for agent %q", name)
	}
	return factory(), nil
}

func init() {
	Register("claude code", func() Deployer { return &ClaudeDeployer{} })
	Register("generic", func() Deployer { return &GenericDeployer{} })
}

// renderMultiRootSection returns a markdown subsection listing the
// project's registered child roots, or "" if there are no children.
// Shared by every deployer's agent-rules generator (CLAUDE.md, Cursor
// rules file, AGENT_INSTRUCTIONS.md).
//
// The section is rendered INSIDE the parlay markers (callers are
// responsible for placement); deployers concatenate this string into
// their tool-managed block so user-authored content outside the
// markers is preserved.
func renderMultiRootSection(projectRoot string) string {
	idx, err := config.LoadRootsIndex(projectRoot)
	if err != nil || idx == nil || len(idx.Children) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Multi-Root Layout\n\n")
	b.WriteString("This project has registered child roots. Each child has its own\n")
	b.WriteString("intents, dialogs, and build artifacts; schemas, adapters, and the\n")
	b.WriteString("agent surface live at the repo-level root and are shared.\n\n")
	for _, c := range idx.Children {
		desc := ""
		if c.Description != "" {
			desc = " — " + c.Description
		}
		b.WriteString(fmt.Sprintf("- **%s** (`%s`)%s\n", c.Name, c.RelativePath, desc))
	}
	return b.String()
}

// AllAgentSurfacePaths returns the union of every registered deployer's
// AgentSurfacePaths, deduplicated. Used by the cobra entry layer to
// configure the forbidden-directory check without adapter-specific
// knowledge.
func AllAgentSurfacePaths() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, factory := range registry {
		for _, p := range factory().AgentSurfacePaths() {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}
