# Intent Schema — authoring digest

Derived from `intent.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

```
# <Feature Name>

> <One-line description>

---

## <Intent Title>

**Goal**: <Why — what the user is trying to accomplish>
**Persona**: <Who — role performing the action>
**Priority**: <P0 | P1 | P2 — importance level; defaults to P1 if omitted>
**Context**: <When — the triggering situation>
**Action**: <How — one-line approach or method>
**Objects**: <What — domain entities involved, comma-separated>

**Constraints**:
- <Hard requirement or boundary>

**Verify**:
- <Expected outcome, observable state, or edge case behavior>

**Questions**:
- <Open design question or unresolved uncertainty>
```

---

| Field | Required | Parse rule |
|---|---|---|
| Feature Name | No | `# ` heading, first line of file |
| Intent Title | Yes | `## ` heading. Slug: lowercase, spaces → hyphens, no punctuation. Must be unique within feature. |
| Goal | Yes | `**Goal**:` line content |
| Persona | Yes | `**Persona**:` line content |
| Priority | No | `**Priority**:` line content. Values: P0 (critical), P1 (important), P2 (nice-to-have). Defaults to P1 if omitted. |
| Context | No | `**Context**:` line content |
| Action | No | `**Action**:` line content |
| Objects | No | `**Objects**:` line content, comma-separated values |
| Constraints | No | `**Constraints**:` followed by `- ` prefixed lines |
| Verify | No | `**Verify**:` followed by `- ` prefixed lines. State-based assertions — expected outcomes, observable states, and edge case behaviors. |
| Questions | No | `**Questions**:` followed by `- ` prefixed lines. Open design questions or unresolved uncertainties. |
