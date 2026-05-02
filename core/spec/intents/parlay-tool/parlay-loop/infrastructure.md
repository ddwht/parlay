# Parlay-loop — Infrastructure

---

## Subagent Definition Bundle

**Affects**: embedded content bundle, agent-deployment pipeline
**Behavior**: Add a new embedded content type — **subagents** — alongside the existing skills bundle. Parlay ships three subagent definition files as source-of-truth: `designer`, `build`, and `code`, each a markdown file with YAML frontmatter that becomes the system prompt for one phase-group's agent session. The files describe scope, allowed tools, and behavior for each phase-group. They are static — the parlay-loop skill invokes them by name via each agent's native mechanism (Agent tool on Claude Code, slash command on Cursor). This deliverable is a prerequisite for the loop skill: without these files deployed, sub-agent invocation fails with an "agent not found" error.
**Invariants**:
- Three subagent definition files exist in the embedded bundle: `designer`, `build`, `code`.
- Each file includes the required frontmatter: `name`, `description`, and a prompt body describing the phase-group's responsibilities.
- The file format matches the shape documented by Claude Code and Cursor (markdown + YAML frontmatter). A single source file per subagent works for both adapters because their formats overlap.
- The subagent bundle is discoverable via `//go:embed` (or equivalent) from the same embedded package that ships skills today.
**Source**: @parlay-tool/parlay-loop/manage-context-across-phases-via-phase-group-sub-agents, @parlay-tool/parlay-loop/support-all-parlay-supported-agents
**Backward-Compatible**: yes

**Notes**:
- Subagents are pre-defined on disk by both supported adapters. Neither Claude Code nor Cursor supports ad-hoc runtime subagent spawning — this bundle is how the loop gets three agents it can invoke by name.
- Cross-tool compatibility: Cursor also reads `.claude/agents/`, so shipping a single set of files works on both adapters if we target that location (see the per-deployer fragments for where each writes them).
- Claude Code subagents cannot themselves spawn further subagents (no nesting). The loop skill runs in the TOP-LEVEL session; from there it CAN spawn subagents. This is the common invocation path.

---

## Embedded Subagent Loader Support

**Affects**: embedded package, bundle exposure to deployers
**Behavior**: Extend the `internal/embedded` package to expose subagent definitions to deployers the same way it exposes skills today. Add a new function (e.g., `Agents()`) that returns a slice of subagent entries — each with a name and raw file content — loaded from `internal/embedded/agents/*.agent.md` via `//go:embed`. Each deployer's `Deploy()` method iterates this slice alongside the existing skills slice and writes each agent definition to its adapter-specific location.
**Invariants**:
- A new package-level function returns all embedded subagent definitions.
- The loader works with `//go:embed` identically to the current skills loader — no new runtime dependencies.
- The subagent entry type mirrors `SkillEntry` (name, content) so deployer code stays symmetric.
- `parlay init` and `parlay upgrade` include the new bundle automatically — no explicit opt-in needed.
**Source**: @parlay-tool/parlay-loop/manage-context-across-phases-via-phase-group-sub-agents, @parlay-tool/parlay-loop/support-all-parlay-supported-agents
**Backward-Compatible**: yes

**Notes**:
- Symmetry with skills is deliberate — future bundles (hooks, commands, etc.) can follow the same pattern.

---

## Claude Adapter Subagent Deployment

**Affects**: Claude agent adapter, `.claude/agents/` deployment target
**Behavior**: Extend the Claude deployer to write each embedded subagent definition to `.claude/agents/parlay-{name}.md` during `parlay init` and `parlay upgrade`. The deployer re-uses its existing skill-writing pattern (frontmatter wrapping, directory creation), just targeting a different directory. After deployment, the three subagents (`parlay-designer`, `parlay-build`, `parlay-code`) are invokable from any Claude Code session via the Agent tool.
**Invariants**:
- After `parlay upgrade`, `.claude/agents/parlay-designer.md`, `.claude/agents/parlay-build.md`, and `.claude/agents/parlay-code.md` all exist.
- The deployed files include the subagent's YAML frontmatter and markdown body verbatim (or with minimal wrapping needed to be a valid Claude Code subagent file).
- Re-running `parlay upgrade` overwrites the files from the current embedded source — they are derived state, not user-editable.
- Existing skill deployment (`.claude/skills/parlay-*/SKILL.md`) and CLAUDE.md section preservation are unaffected.
**Source**: @parlay-tool/parlay-loop/support-all-parlay-supported-agents
**Backward-Compatible**: yes

**Notes**:
- `.claude/agents/parlay-*.md` is the agent-prefixed convention analogous to `.claude/skills/parlay-*/`.

---

## Cursor Adapter Subagent Deployment

**Affects**: Cursor agent adapter, `.cursor/agents/` deployment target
**Behavior**: Extend the Cursor deployer to write each embedded subagent definition to `.cursor/agents/parlay-{name}.md` during `parlay init` and `parlay upgrade`. Same pattern as the Claude adapter — iterate the embedded subagent bundle, create the directory, write each file with any minimal Cursor-specific frontmatter adjustments. After deployment, the three subagents are invokable in any Cursor session via slash commands (`/parlay-designer`, `/parlay-build`, `/parlay-code`) or `@agent-parlay-designer` mentions.
**Invariants**:
- After `parlay upgrade`, `.cursor/agents/parlay-designer.md`, `.cursor/agents/parlay-build.md`, and `.cursor/agents/parlay-code.md` all exist.
- The file format is Cursor-compatible markdown + YAML frontmatter.
- Existing Cursor deployment (`.cursor/skills/parlay-*/SKILL.md` and `.cursor/rules/parlay.mdc`) is unaffected.
**Source**: @parlay-tool/parlay-loop/support-all-parlay-supported-agents
**Backward-Compatible**: yes

**Notes**:
- Frontmatter fields required by Cursor (`name`, `description`) overlap with Claude's — a single source file satisfies both, possibly with a deployment-time frontmatter massage if a field needs renaming.

---

## Generic Adapter Subagent Fallback

**Affects**: Generic CLI agent adapter, AGENT_INSTRUCTIONS.md output
**Behavior**: The Generic CLI adapter has no native subagent concept. Instead of writing agent files, extend the Generic deployer to include a section in `AGENT_INSTRUCTIONS.md` describing the three phase-group roles (designer / build / code) as embedded guidance text, so a reader can follow the loop phases manually across fresh sessions. The loop skill detects this adapter (by the absence of any native agent-spawning mechanism) and transitions to the fresh-session handoff mode at each phase-group boundary — printing the resume command and exiting.
**Invariants**:
- After `parlay upgrade`, `AGENT_INSTRUCTIONS.md` includes a "Phase-Groups" section describing the three subagent roles.
- No `.generic/agents/` or similar directory is created — the Generic adapter publishes everything through a single concatenated file.
- When the loop skill runs under the Generic adapter, it uses the fresh-session handoff mode rather than attempting agent invocation.
**Source**: @parlay-tool/parlay-loop/support-all-parlay-supported-agents
**Backward-Compatible**: yes

**Notes**:
- Generic does not support real context isolation — the fresh-session handoff simulates it by using the user's session boundaries.

---

## Register parlay-loop Skill in the Embedded Skills Bundle

**Affects**: embedded skills bundle, skill-to-deployer pipeline
**Behavior**: Add a new source-of-truth skill file describing parlay-loop's orchestration behavior to the embedded skills bundle. Because all three deployers consume the embedded skills list during `parlay init` / `parlay upgrade`, this single registration automatically propagates the skill into each agent's deployed surface — `.claude/skills/parlay-loop/SKILL.md`, `.cursor/skills/parlay-loop/SKILL.md`, and the concatenated `AGENT_INSTRUCTIONS.md` for Generic. This follows the canonical "add a new skill" flow documented in the project blueprint.
**Invariants**:
- After running `parlay upgrade`, every supported agent has a parlay-loop skill visible at its agent-specific location.
- The source skill file is the single authoring location — deployed copies are derived state and overwritten on upgrade.
- The skill references the three subagents by name (`parlay-designer`, `parlay-build`, `parlay-code`) and invokes them via the agent's native mechanism, not via any Go interface.
- Adding this skill triggers no per-deployer code change; the embedded skills bundle is iterated by each deployer's existing deployment logic.
**Source**: @parlay-tool/parlay-loop/run-the-full-design-loop-from-one-command, @parlay-tool/parlay-loop/support-all-parlay-supported-agents
**Backward-Compatible**: yes

**Notes**:
- Per blueprint: "Adding/removing a skill: affects all deployers via embedded skills list" — this fragment exercises exactly that rule.
- Deployers wrap the raw skill content with agent-specific frontmatter (Claude YAML, Cursor MDC) during writing; the source file is frontmatter-free.

---

## Register parlay-loop CLI Command

**Affects**: CLI command registry, skill-title mapping, Generic adapter command list
**Behavior**: Register `parlay loop` as a new cobra subcommand so the binary exposes `parlay loop <feature> [--from <phase>]` at the command line, and simultaneously register the corresponding `/parlay-loop` skill title and update the Generic adapter's hardcoded command list. Per the project blueprint's cross-cutting rule for CLI commands, all three registration points — root command registry, skill-title mapping, and Generic adapter's command list — must be updated in one changeset to keep all three deployers consistent. The CLI command itself is thin: like other parlay skills, the cobra entrypoint prints a pointer message directing the user to invoke the skill through their agent; actual orchestration lives in the skill content, which invokes the pre-defined subagents by name.
**Invariants**:
- `parlay loop --help` lists the new subcommand with its usage.
- The `/parlay-loop` slash command appears in Claude's CLAUDE.md, Cursor's parlay.mdc rule, and Generic's AGENT_INSTRUCTIONS.md after `parlay upgrade`.
- `skillTitle("loop")` returns the human-readable title matching the skill's purpose.
- The CLI command itself does not invoke any orchestration logic — it prints a pointer message directing the user to the skill.
**Source**: @parlay-tool/parlay-loop/run-the-full-design-loop-from-one-command
**Backward-Compatible**: yes

**Notes**:
- Per blueprint: "Adding/removing a CLI command: update generic.go hardcoded list AND root.go registration." The skill-title mapping is the third touch point because it drives the human-readable title rendered in `/parlay-*` slash-command descriptions (shared by Claude + Cursor).
- The thin CLI pattern matches how `parlay build-feature` and `parlay generate-code` expose themselves today.

---
