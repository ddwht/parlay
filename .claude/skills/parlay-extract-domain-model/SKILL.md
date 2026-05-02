---
name: parlay-extract-domain-model
description: "Parlay: Extract domain model from all features"
---

# Extract Domain Model

Analyze all features in the project and extract a domain model.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. **Load schemas** — Read `.parlay/schemas/intent.schema.md`, `.parlay/schemas/dialog.schema.md`, `.parlay/schemas/surface.schema.md`.

2. **Scan all features** — Read `spec/intents/*/intents.md`, `dialogs.md`, and `surface.md`.

3. **Extract entities** — From intent Objects fields and implicit references in dialogs and surfaces:
   - For each entity, derive typed properties from how it's described and used
   - Identify relationships (belongs-to, has-many, references)
   - Identify state machines from dialog conditions and intent constraints

4. **Write domain model** — Create `spec/domain-model.md` with sections:
   - Entities (with properties and relationships for each)
   - State Machines (with explicit transitions)
   - Operation Catalog (operations implied by dialogs, mapped to commands)
   - Entity Relationship Summary (tree diagram)

5. **Report** — Print the model path and a summary of what was extracted (entity count, relationships, state machines).
