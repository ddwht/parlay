---
name: parlay-lock-page
description: "Parlay: Lock a page layout into a manifest"
---

# Lock Page

Lock a page layout into a manifest with an owner.

## Arguments

- `page`: The page name (e.g., `dashboard`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. Run: `parlay view-page {page}` to show the current layout.
2. Ask the user who should own this page.
3. Run: `echo "<owner>" | parlay lock-page {page}` — the command prompts for an owner on stdin; piping the answer lets a non-interactive agent invocation satisfy the prompt without a live terminal.
4. Tell the user to set the status to "reviewed" or "locked" when satisfied.
