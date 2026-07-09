---
name: lock-page
description: "Lock a page layout into a manifest"
---

# Lock Page

Lock a page layout into a manifest with an owner.

## Arguments

- `page`: The page name (e.g., `dashboard`)

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

## Steps

1. Run: `parlay view-page {page}` to show the current layout.
2. Ask the user who should own this page.
3. Run: `echo "<owner>" | parlay lock-page {page}` — the command prompts for an owner on stdin; piping the answer lets a non-interactive agent invocation satisfy the prompt without a live terminal.
4. Tell the user to set the status to "reviewed" or "locked" when satisfied.
