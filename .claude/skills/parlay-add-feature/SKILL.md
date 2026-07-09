---
name: parlay-add-feature
description: "Parlay: Create a new feature"
---

# Add Feature

Create a new feature folder with intents.md and dialogs.md. Optionally place it inside an initiative.

## Arguments

- `name`: The feature name (e.g., `upgrade plan creation`)
- `initiative` (optional): The initiative to create the feature inside (e.g., `auth overhaul`). Auto-creates the initiative if it doesn't exist.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. If the user specified an initiative: run `parlay add-feature {name} --initiative {initiative}`
2. Otherwise: run `parlay add-feature {name}`
3. Tell the user to start authoring intents.md.
