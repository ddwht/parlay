---
name: parlay-view-page
description: "Parlay: Assemble and display a page view"
---

# View Page

Assemble and display a cross-feature page view.

## Arguments

- `page`: The page name (e.g., `dashboard`, `cluster-detail`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. Run: `parlay view-page {page}` — this assembles the view from feature surface fragments only; it does not read a locked page manifest's structural options.
2. If `spec/pages/{page}.page.md` exists (see `page.schema.md`), also read it: manifest fragment order overrides the surface-derived order, and an optional `## Layout` section (see `layout.schema.md`) may pin the page to a specific `componentVocabulary`/`schemaVersion` and node tree. Mention these when they diverge from the CLI's fragment-only assembly.
3. If there are conflicts (same region + same order) or unplaced fragments (no Page target), present them and ask the user via AskUserQuestion:
   - A: Resolve conflicts now — walk through each one and ask which fragment should win the order, or whether to renumber
   - B: Lock the page — run `/parlay-lock-page {page}` to pin an explicit manifest order, sidestepping the conflict going forward
   - C: Leave as-is — report the conflicts/unplaced fragments and stop; no files change
