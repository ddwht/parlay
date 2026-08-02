# Intent Schema

File: `spec/intents/<feature-name>/intents.md`
Contains one or more intent blocks. Starts with an optional feature header.

## Template

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

## Fields

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

## Parsing

- Intent boundaries: `## ` headings
- Field extraction: `**Field**:` pattern on a single line
- List items: `- ` prefixed lines under Constraints/Verify/Questions
- Objects: comma-separated on a single line
- Priority: single token on a single line (P0, P1, P2)
- Feature-to-intent hierarchy: file location + heading structure

## Validation

`parlay validate --type intent spec/intents/<feature>/intents.md` checks a file
against this schema. The same rules back `parlay check-readiness --stage
create-surface`, which runs them in build mode — one implementation, two callers.

| Code | When it fires |
|---|---|
| `intents-not-readable` | The file does not exist, or cannot be read as markdown |
| `no-intents` | The file has no `## ` intent headings. Build mode fails; authoring mode warns, because a scaffolded intents.md legitimately starts empty. |
| `missing-goal` | An intent block has no `**Goal**:` line |
| `missing-persona` | An intent block has no `**Persona**:` line |
| `duplicate-intent-title` | Two intent titles in one feature slugify to the same value, so `@<feature>/<intent-slug>` cannot name one of them |
| `invalid-priority` | A `**Priority**:` line carries something other than `P0`, `P1` or `P2`. An omitted line is valid and means `P1`. |

## References

- Feature: `@<feature-name>` (folder name)
- Specific intent: `@<feature-name>/<intent-slug>`
