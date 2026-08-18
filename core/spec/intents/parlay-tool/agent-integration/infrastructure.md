# Agent Integration — Infrastructure

---

## Deployer Registry

**Affects**: `core/internal/deployer/` (registry and every per-agent deployer), `core/internal/embedded/{skills,schemas}/`

**Behavior**: Parlay supports multiple AI agents through a deployer registry. Skills are authored once as agent-agnostic markdown; each deployer packages the same embedded set into one agent's format. Adding an agent means adding a deployer and nothing else — no skill content changes, no schema changes.

The registry lives at `core/internal/deployer/deployer.go`. Three inputs are shared by every deployer:

| Shared input | Location |
|---|---|
| Skill sources | `core/internal/embedded/skills/*.skill.md` |
| Skill titles | `core/internal/deployer/claude.go` → `skillTitle()` |
| Schemas | `core/internal/embedded/schemas/*.schema.md` |

Per-agent deployers and what each writes:

| Agent | Deployer | Output | Format notes |
|---|---|---|---|
| Claude Code | `claude.go` | `.claude/skills/parlay-*/SKILL.md`; `CLAUDE.md` | YAML frontmatter per skill (name, description); CLAUDE.md section preservation via HTML comment markers; `AskUserQuestion` available for interactive prompts |
| Cursor | `cursor.go` | `.cursor/skills/parlay-*/SKILL.md`; `.cursor/rules/parlay.mdc` | YAML frontmatter per skill; MDC format with `alwaysApply` |
| Generic | `generic.go` | `AGENT_INSTRUCTIONS.md` | One file, all skills concatenated; no per-skill files; **hardcoded CLI command list** |

**Invariants**:
- A skill's destination is decided by its `surface:` frontmatter key, not by its filename — `command` skills deploy to the agent's skills directory, `module` skills to `.parlay/modules/`.
- `skillTitle()` is shared by the Claude and Cursor deployers; a title added for one appears in the other.
- The generic deployer's CLI command list is hardcoded and does not derive from the registered command set, so it goes stale silently.

**Cross-cutting rules for deployer changes**:
- Adding or removing a skill affects every deployer via the embedded skills list — no per-deployer edit needed.
- Adding or removing a skill title: update `skillTitle()` in `claude.go` (shared by Claude and Cursor).
- Adding or removing a CLI command: update `generic.go`'s hardcoded list **and** the registration in `root.go`. Missing the first is invisible until a generic-deployer user reads instructions naming a command that no longer exists.
- Changing the CLAUDE.md template: edit `writeCLAUDEmd` in `claude.go`.
- Changing the Cursor rule template: edit `writeCursorProjectRule` in `cursor.go`.

**Provenance**: this section was a `deployers:` block at the top level of `core/.parlay/blueprint.yaml`. Blueprint's owned scope is closed — `app`, `shells`, `navigation`, `authorization`, `data`, `errors`, `state`, `platform` — and `deployers:` was outside it under both the current list and the one that preceded it. It survived only because the scope check runs on the multi-target project pass and `core/` has no `adapter-set.yaml`, so it was never reached. It is architectural prose about a boundary and its cross-cutting change rules, which is what `infrastructure.md` is for.
