<!-- parlay:begin -->
# Parlay Project

This project uses the Parlay intent-driven design toolkit.
Start with `/parlay-loop` — it walks a feature through the whole pipeline and
runs the individual phases for you. The other two commands are for entering
from somewhere other than the start.

## Available Commands

- `/parlay-create-adapter` — Author a new framework adapter from scratch and validate it
- `/parlay-doctor` — Diagnose and repair project state — coverage, drift, tree consistency, and pending migrations
- `/parlay-loop` — Walk a feature end-to-end through the parlay design pipeline
- `/parlay-onboard` — Onboard existing codebase and draft adapter
- `/parlay-refine` — Make a small, precise change to an existing feature — spec, code, tests and baselines together

The pipeline phases (intents, dialogs, artifacts, build, code) are not commands.
Their instructions live in .parlay/modules/, and the loop's phase subagents read
the one they need. The full CLI is still there — run `parlay --help`.

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

## Interactive Questions

When a skill step says to "ask the user", "present options", or "wait for the user's response", use the AskUserQuestion tool to pause and collect the answer before continuing. Do not output the question as plain text and keep going — the step needs the answer to decide what happens next.

**Unless you are a subagent.** A parlay phase subagent (parlay-designer, parlay-build, parlay-code) has no AskUserQuestion, so a question asked there reaches nobody and the phase ends up answering itself — a skipped confirmation is indistinguishable from a granted one. In a subagent, stop and return a `parlay-decision` block instead; the loop driver prompts and resumes you with the answer.

## File Ownership

Four-zone layout — strict ownership:
- **spec/intents/<feature>/** (designer-authored): intents.md, dialogs.md — **frozen founding documents** after first build: do not modify them at all (not even with permission); change goes through an amendment via /parlay-refine, and check-drift reports frozen-doc edits as ledger_integrity violations. Before first build they are ordinary designer-authored files — ask permission before modifying.
- **spec/intents/<feature>/amendments/** (designer-authored, append-only): NNN-<slug>.md — one file per change, written once and NEVER edited; a correction is a new amendment naming the old one in supersedes:. Compaction may move files to amendments/archive/, never delete them.
- **spec/intents/<feature>/** (generated, human-reviewed): four co-equal spec artifacts — the current truth the amendments apply to —
  - **surface.yaml** — visible output, page assemblies, dialog turns. (surface.md is retired: a lingering one goes stale against the amended contract and misleads; run `parlay migrate-spec --retire-md` and never mirror edits into a surface.md.)
  - **capabilities.yaml** — operation-shaped backend behavior (closed-vocabulary commands and queries)
  - **infrastructure.md** — architectural prose for boundaries, probes, allowlists, dependency pins, and other concerns that do not reduce to operations
  - **domain-model.yaml** — entities, relationships, and shared vocabulary
  - Plus *.page.md for per-page layouts. A feature picks whichever artifacts it needs; capabilities.yaml and infrastructure.md are co-equal, not stand-ins for each other.
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, criteria-authority.yaml, coverage-decisions.yaml, .baseline.yaml — never user-facing
- **.parlay/adapter-set.yaml** (tool config, project-owned): pins adapter slot topology — multi-target projects only

## Multi-Root Layout

This project has registered child roots. Each child has its own
intents, dialogs, and build artifacts; schemas, adapters, and the
agent surface live at the repo-level root and are shared.

- **core** (`core`) — Parlay Core — the spec-driven design and codegen engine
- **studio** (`studio`) — Parlay Studio — designer-facing extension on top of Core (Figma round-trip, domain model editor)
<!-- parlay:end -->

## Skill and Schema Updates (dogfooding rule)

This project develops parlay AND uses parlay. Skills and schemas exist in two places:
- **Source** (authoritative): `core/internal/embedded/skills/<name>.skill.md` and `core/internal/embedded/schemas/<name>.schema.md`. Bundled into the binary at compile time via `//go:embed`. This is what new projects receive via `parlay init`.
- **Deployed for this project**: `.claude/skills/parlay-<name>/SKILL.md` and `.parlay/schemas/<name>.schema.md`. What Claude Code and the running tool actually load in this repo. Treat these as derived state.

When changing skill or schema behavior, follow the strict three-step source-first rule, in order:

1. **Edit the source** under `core/internal/embedded/{skills,schemas}/`.
2. **Rebuild** the binary: `make build`.
3. **Re-deploy** to this project: `./parlay upgrade`. This overwrites `.claude/skills/parlay-*/SKILL.md` and `.parlay/schemas/*.schema.md` from the freshly-built binary.

Or use `make sync-skills` to do steps 2+3 in one shot. Verify sync with `make verify-skills`.

**Warning**: `parlay upgrade` overwrites this CLAUDE.md file. The dogfooding section below the "File Ownership" header is project-local and must be re-added manually after each upgrade until the deployer supports preserving user sections.

**Adapters are NOT covered by this rule.** Per-project adapters under `.parlay/adapters/` are project-owned and may be customized via `parlay onboard`. `parlay upgrade` never redeploys them from the binary. The one exception is an explicit, interactive opt-in: `upgrade` offers to add `kind:` to adapters that predate the field, and rewrites only the files you approve.

In a multi-root project adapters are resolved **child-first with parent fallback**, so a child root inherits the repo-level adapters and only needs its own file when it wants to override one.
