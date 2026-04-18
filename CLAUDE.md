<!-- parlay:begin -->
# Parlay Project

This project uses the Parlay intent-driven design toolkit.
All operations are available as /parlay-* slash commands.

## Available Commands

- `/parlay-add-feature` — Create a new feature
- `/parlay-build-feature` — Generate buildfile and testcases
- `/parlay-collect-questions` — Collect open questions from intents
- `/parlay-create-artifacts` — Determine and create surface.md, infrastructure.md, or both
- `/parlay-extract-domain-model` — Extract domain model from all features
- `/parlay-generate-code` — Generate prototype code from buildfile
- `/parlay-generate-enggspec` — Generate engineering specification
- `/parlay-load-domain-model` — Load and integrate external domain model
- `/parlay-lock-page` — Lock a page layout into a manifest
- `/parlay-new-initiative` — Create an empty initiative directory
- `/parlay-onboard` — Onboard existing codebase and draft adapter
- `/parlay-reference-design-spec` — Extract design spec from Figma
- `/parlay-register-adapter` — Register a framework adapter
- `/parlay-scaffold-dialogs` — Scaffold dialog templates from intents
- `/parlay-sync` — Check intent-dialog coverage
- `/parlay-view-page` — Assemble and display a page view

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
<!-- parlay:end -->

## Skill and Schema Updates (dogfooding rule)

This project develops parlay AND uses parlay. Skills and schemas exist in two places:
- **Source** (authoritative): `internal/embedded/skills/<name>.skill.md` and `internal/embedded/schemas/<name>.schema.md`. Bundled into the binary at compile time via `//go:embed`. This is what new projects receive via `parlay init`.
- **Deployed for this project**: `.claude/skills/parlay-<name>/SKILL.md` and `.parlay/schemas/<name>.schema.md`. What Claude Code and the running tool actually load in this repo. Treat these as derived state.

When changing skill or schema behavior, follow the strict three-step source-first rule, in order:

1. **Edit the source** under `internal/embedded/{skills,schemas}/`.
2. **Rebuild** the binary: `make build`.
3. **Re-deploy** to this project: `./parlay upgrade`. This overwrites `.claude/skills/parlay-*/SKILL.md` and `.parlay/schemas/*.schema.md` from the freshly-built binary.

Or use `make sync-skills` to do steps 2+3 in one shot. Verify sync with `make verify-skills`.

**Warning**: `parlay upgrade` overwrites this CLAUDE.md file. The dogfooding section below the "File Ownership" header is project-local and must be re-added manually after each upgrade until the deployer supports preserving user sections.

**Adapters are NOT covered by this rule.** Per-project adapters under `.parlay/adapters/` are project-owned and may be customized via `parlay onboard`. `parlay upgrade` deliberately leaves them alone.
