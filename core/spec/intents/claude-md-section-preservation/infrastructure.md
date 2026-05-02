# Claude MD Section Preservation — Infrastructure

---

## CLAUDE.md Marker-Based Section Preservation

**Modifies**: internal/deployer/claude.go (writeCLAUDEmd function)
**Behavior**: Update the CLAUDE.md writer to use `<!-- parlay:begin -->` / `<!-- parlay:end -->` HTML comment markers around parlay-generated content. On write, check for an existing CLAUDE.md: if markers are found, replace only the content between them and preserve everything outside (above and below). If no markers are found and the file exists, fall back to the agent for an intelligent merge — the agent reads the existing content, identifies parlay-generated vs user-authored sections, produces a merged result with markers in place, and presents a diff for review. If no CLAUDE.md exists, create a new one with markers wrapping the generated content.
**Source**: @claude-md-section-preservation/preserve-user-sections-in-claude-md-during-upgrade
**Backward-Compatible**: yes

**Notes**:
- The markers are HTML comments (`<!-- parlay:begin -->`, `<!-- parlay:end -->`), invisible to markdown rendering but recognized by the deployer
- The marker format must be stable across versions — changing syntax would break preservation on the next upgrade
- User content between the markers is parlay-owned and replaced on every upgrade; user content outside the markers is never touched
- The intelligent-merge fallback for no-markers files uses the same Tier 2 mechanism as brownfield code generation — read existing file, identify sections, produce a diff for review
- The change applies to both `parlay init` and `parlay upgrade` paths since both call `writeCLAUDEmd`

---
