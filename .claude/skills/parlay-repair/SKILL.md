---
name: parlay-repair
description: "Parlay: Validate and reconcile the three parallel trees"
---

# Repair

Validate and reconcile the three parallel trees (spec/intents/, spec/handoff/, .parlay/build/) after external filesystem operations.

## Arguments

None.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. Run: `parlay repair`
2. For each detected mismatch, the command will prompt interactively. Confirm or skip each repair.
3. If the designer wants to preview without applying: `parlay repair --dry-run`
4. If the designer wants to auto-confirm unambiguous repairs: `parlay repair --yes`
