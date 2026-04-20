# Claude Md Section Preservation — Dialogs

---

### Preserve User Sections in CLAUDE.md During Upgrade

**Trigger**: User runs `parlay init` or `parlay upgrade` in a project that may already have a CLAUDE.md.

User: parlay init
System (background): Checks for existing CLAUDE.md.
System (condition: CLAUDE.md exists with markers): Found existing CLAUDE.md with parlay markers. Replacing content between `<!-- parlay:begin -->` and `<!-- parlay:end -->` with updated parlay section. Preserving user content outside markers.
System (condition: CLAUDE.md exists without markers): Found existing CLAUDE.md without parlay markers. Prepending parlay section wrapped in `<!-- parlay:begin -->` / `<!-- parlay:end -->` markers. Appending existing file content below `<!-- parlay:end -->`.
System (condition: no CLAUDE.md): Creating CLAUDE.md with parlay section wrapped in markers.
System: Project bootstrapped.

#### Branch: Upgrade with markers present

User: parlay upgrade
System (background): Reads existing CLAUDE.md. Finds `<!-- parlay:begin -->` on line 1 and `<!-- parlay:end -->` on line 38. Replaces lines 1–38 with updated parlay section. Lines 39+ (user content) preserved verbatim.
System: Upgraded to parlay ==version==:
System:   schemas — ==N== updated
System:   skills  — ==N== deployed for ==agent==
System:   CLAUDE.md — parlay section updated, user sections preserved

#### Branch: Upgrade without markers (agent-assisted merge)

User: parlay upgrade
System (background): Reads existing CLAUDE.md. Scans for `<!-- parlay:begin -->` — not found. Cannot mechanically determine parlay vs user content. Falls back to agent intelligent-merge.
System (background): Agent reads the existing CLAUDE.md, identifies parlay-generated sections (command list, schema loading, file ownership) and user-authored sections (team conventions, project notes, custom rules).
System (background): Agent produces a merged result with `<!-- parlay:begin -->` / `<!-- parlay:end -->` markers wrapping the parlay content, user content preserved outside.
System: CLAUDE.md has no parlay markers. I've identified the parlay-generated and user-authored sections. Proposed merge:
System: ==unified diff showing markers added, parlay content updated, user content preserved==
  A: Apply this merge
  B: Let me review and edit
  C: Skip — I'll add markers manually
User: A
System (background): Applies the merged CLAUDE.md.
System: Upgraded to parlay ==version==:
System:   CLAUDE.md — markers added, parlay section updated, user sections preserved

#### Branch: User content above and below markers

User: parlay upgrade
System (background): Reads existing CLAUDE.md. Finds user content on lines 1–3 above `<!-- parlay:begin -->`. Finds `<!-- parlay:end -->` on line 42. Finds user content on lines 43–80 below the closing marker.
System (background): Preserves lines 1–3 (above). Replaces lines 4–42 (between markers). Preserves lines 43–80 (below).
System: Upgraded to parlay ==version==:
System:   CLAUDE.md — parlay section updated, user sections preserved (3 lines above, 38 lines below)

---
