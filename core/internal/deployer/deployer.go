package deployer

import (
	"github.com/ddwht/parlay/core/internal/atomicfile"

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
	// Deploy writes the agent surface and returns the number of files it
	// actually wrote. The count is writes, not files considered: the
	// content-hash skip means a re-deploy over unchanged sources returns 0, and
	// reporting the considered count would describe work that did not happen.
	Deploy(projectRoot string, skills []embedded.SkillEntry) (int, error)

	// AgentSurfacePaths returns the deployer-owned paths (relative to a
	// parlay root) that must NOT exist inside a child root — they live
	// only at the repo-level root. Each entry is interpreted as a
	// directory or file name relative to the root being checked.
	AgentSurfacePaths() []string
}

// fileOwnershipSection is the canonical "File Ownership" doctrine block —
// the four-zone layout and the four co-equal spec artifacts — rendered
// verbatim into every agent's project-config file (CLAUDE.md, Cursor's
// parlay.mdc, ...). A single constant here is what keeps deployers from
// drifting out of sync with each other as the artifact set evolves.
const fileOwnershipSection = `## File Ownership

Four-zone layout — strict ownership:
- **spec/intents/<feature>/** (designer-authored): intents.md, dialogs.md — **frozen founding documents** after first build: do not modify them at all (not even with permission); change goes through an amendment via /parlay-refine, and check-drift reports frozen-doc edits as ledger_integrity violations. Before first build they are ordinary designer-authored files — ask permission before modifying.
- **spec/intents/<feature>/amendments/** (designer-authored, append-only): NNN-<slug>.md — one file per change, written once and NEVER edited; a correction is a new amendment naming the old one in supersedes:. Compaction may move files to amendments/archive/, never delete them.
- **spec/intents/<feature>/** (generated, human-reviewed): four co-equal spec artifacts — the current truth the amendments apply to —
  - **surface.yaml** — visible output, page assemblies, dialog turns. (surface.md is retired: a lingering one goes stale against the amended contract and misleads; run ` + "`parlay migrate-spec --retire-md`" + ` and never mirror edits into a surface.md.)
  - **capabilities.yaml** — operation-shaped backend behavior (closed-vocabulary commands and queries)
  - **infrastructure.md** — architectural prose for boundaries, probes, allowlists, dependency pins, and other concerns that do not reduce to operations
  - **domain-model.yaml** — entities, relationships, and shared vocabulary
  - Plus *.page.md for per-page layouts. A feature picks whichever artifacts it needs; capabilities.yaml and infrastructure.md are co-equal, not stand-ins for each other.
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, criteria-authority.yaml, coverage-decisions.yaml, .baseline.yaml — never user-facing
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
Start with %s/parlay-loop%s — it walks a feature through the whole pipeline and
runs the individual phases for you. The other two commands are for entering
from somewhere other than the start.

## Available Commands

%s
The pipeline phases (intents, dialogs, artifacts, build, code) are not commands.
Their instructions live in .parlay/modules/, and the loop's phase subagents read
the one they need. The full CLI is still there — run %sparlay --help%s.

## Schema Loading

Two derived layers sit in front of the schema corpus. Both are generated at
deploy time from the schemas themselves, so neither can drift from them.

**Diagnosing** — read .parlay/schemas/DIGEST.md first. It lists every error
code the tool can emit and when each fires, in a fraction of the corpus. It
tells you which schema to open and which mistakes are pre-checkable, so you
are not discovering validator rules by triggering them.

**Authoring** — read .parlay/schemas/digests/<name>.digest.md. It carries the
normative half of that schema (field tables, closed vocabularies, required
shapes, invariants) without the rationale and history, which is what you
need to WRITE the artifact rather than to understand why it is shaped that
way.

Open the full .parlay/schemas/<name>.schema.md when a finding routes you
there, when a digest does not answer a question you actually have, or when
you are changing the schema itself. Do not open one out of caution — the
digests are derived, so what they state is what the schema states. Load on
demand either way, and do not keep schema content in memory across commands.

## Capturing Discoveries

When you notice something real that is out of scope for what you are doing — a
defect, a gap the spec never covered, a shortcut taken knowingly — record it
rather than mentioning it:

    parlay note --kind gap --title "..." --by <you>

The kind is one of defect, gap, debt or idea. Concrete, evidenced work only. Never speculation, and never a priority nobody
gave you. A note that fails to write must never fail the work that tried.

## Interactive Questions

When a skill step says to "ask the user", "present options", or "wait for the user's response", use the AskUserQuestion tool to pause and collect the answer before continuing. Do not output the question as plain text and keep going — the step needs the answer to decide what happens next.

**Unless you are a subagent.** A parlay phase subagent (parlay-designer, parlay-build, parlay-code) has no AskUserQuestion, so a question asked there reaches nobody and the phase ends up answering itself — a skipped confirmation is indistinguishable from a granted one. In a subagent, stop and return a %sparlay-decision%s block instead; the loop driver prompts and resumes you with the answer.

%s%s`,
		"`", "`",
		renderAvailableCommands(skills),
		"`", "`",
		"`", "`",
		fileOwnershipSection, renderMultiRootSection(projectRoot))
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

// atomicWrite writes content to path through the shared primitive and reports
// whether it wrote, for the call sites that regenerate a whole file.
func atomicWrite(path string, content []byte) (bool, error) {
	return atomicfile.WriteIfChanged(path, content)
}
