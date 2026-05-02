---
name: parlay-generate-enggspec
description: "Parlay: Generate engineering specification"
---

# Generate Engineering Specification

Translate feature design artifacts into a formal engineering specification for handoff.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. **Load project config** — Read `.parlay/config.yaml` to determine the SDD framework format.

2. **Read all feature artifacts**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/surface.md`
   - `spec/intents/{feature}/domain-model.md` (if exists)
   - `.parlay/build/{feature}/buildfile.yaml` (if exists — tool-internal)
   - `.parlay/build/{feature}/testcases.yaml` (if exists — tool-internal; project the observable-behavior assertions into the spec's Acceptance Criteria section)

3. **Generate specification** at `spec/handoff/{feature}/specification.md` (this is the only handoff artifact — engineering reads it and writes their own tests):
   - Feature overview and user stories (from intents — Goal becomes acceptance criteria)
   - Detailed interaction requirements (from dialog flows)
   - UI component specifications (from surface fragments)
   - Data models and API contracts (from buildfile models if available)
   - Test scenarios (from testcases if available)
   - Acceptance criteria (from intent constraints)
   - Format according to the configured SDD framework (e.g., GitHub SpecKit)

4. **Report** — Print the specification path and the SDD format used. Remind the user to review before handoff.
