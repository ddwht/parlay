# Claude MD Section Preservation

> Fix `parlay upgrade` so it only updates parlay-owned sections in CLAUDE.md, preserving any project-local content the user has added. Today upgrade overwrites the entire file, wiping user additions.

---

## Preserve User Sections in CLAUDE.md During Upgrade

**Goal**: Make `parlay upgrade` update only the parlay-managed sections of CLAUDE.md (command list, schema loading, interactive questions, file ownership) while preserving any sections the user has added below — so that project-local instructions (dogfooding rules, team conventions, custom notes) survive upgrades without manual re-addition.
**Persona**: Parlay Developer
**Priority**: P0
**Context**: `parlay upgrade` regenerates CLAUDE.md from a Go template, overwriting the entire file. Any content the user added — dogfooding discipline, team conventions, project-specific notes — is silently deleted. The user has to manually re-add it after every upgrade. This is fragile and defeats the purpose of having a persistent project instructions file. The deployer should own its sections and leave everything else alone.
**Action**: Update the CLAUDE.md writer in the deployer to: (1) mark parlay-managed content with boundary markers (e.g., HTML comments `<!-- parlay:begin -->` / `<!-- parlay:end -->`), (2) on upgrade, read the existing CLAUDE.md, find the marker boundaries, replace only the content between them, and preserve everything outside. On first init (no existing CLAUDE.md), write the full template with markers. On upgrade with no markers found (legacy file), replace the entire file as today but warn the user that their additions were lost, and suggest re-adding them below the new markers.
**Objects**: deployer, CLAUDE.md, upgrade, markers

**Constraints**:
- The markers must be invisible to Claude Code's rendering — HTML comments (`<!-- parlay:begin -->`, `<!-- parlay:end -->`) work in markdown without displaying
- Parlay-managed content lives between the markers. Everything before the opening marker and after the closing marker is user-owned and preserved verbatim
- On `parlay init` (fresh CLAUDE.md), the markers wrap the full generated content. The user can add sections after the closing marker
- On `parlay upgrade` with markers present, only the content between markers is replaced. User content before/after is kept
- On `parlay upgrade` or `parlay init` when CLAUDE.md exists without markers, the deployer cannot mechanically determine which parts are parlay-generated and which are user-authored. It falls back to the agent for an intelligent merge: the agent reads the existing content, identifies parlay-generated sections (command list, schema loading, file ownership) vs user-authored sections, produces a merged result with markers wrapping the parlay content, and presents the diff for review before applying. User data is never overwritten or deleted.
- The marker format must be stable across versions — changing marker syntax would break preservation on the next upgrade
- `parlay upgrade` must not error if CLAUDE.md doesn't exist (fresh checkout) — it creates the file with markers as if running init

**Verify**:
- Running `parlay upgrade` on a CLAUDE.md with markers and user content below `<!-- parlay:end -->` preserves the user content and updates the parlay section
- Running `parlay upgrade` on a CLAUDE.md with markers and user content above `<!-- parlay:begin -->` preserves the user content above
- Running `parlay upgrade` on a CLAUDE.md without markers falls back to agent intelligent-merge: the agent identifies parlay vs user content, presents a diff with markers added, and applies on approval — no user data lost
- Running `parlay upgrade` when CLAUDE.md doesn't exist creates a new file with markers
- The markers are not visible when Claude Code reads the file (HTML comments are invisible in markdown rendering)
- Running `parlay upgrade` twice in a row produces the same CLAUDE.md (idempotent)
- User content between the markers is NOT preserved — that space is parlay-owned

---
