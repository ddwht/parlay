---
name: parlay-lock-page
description: "Parlay: Lock a page layout into a manifest"
---

# Lock Page

Lock a page layout into a manifest with an owner.

## Arguments

- `page`: The page name (e.g., `dashboard`)

## Steps

1. Run: `parlay view-page {page}` to show the current layout.
2. Ask the user who should own this page.
3. Run: `parlay lock-page {page}` and pipe the owner name.
4. Tell the user to set the status to "reviewed" or "locked" when satisfied.
