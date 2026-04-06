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
**Context**: <When — the triggering situation>
**Action**: <How — one-line approach or method>
**Objects**: <What — domain entities involved, comma-separated>

**Constraints**:
- <Hard requirement or boundary>

**Hints**:
- <Edge case, alternative path, or open design question>
```

## Fields

| Field | Required | Parse rule |
|---|---|---|
| Feature Name | No | `# ` heading, first line of file |
| Intent Title | Yes | `## ` heading. Slug: lowercase, spaces → hyphens, no punctuation. Must be unique within feature. |
| Goal | Yes | `**Goal**:` line content |
| Persona | Yes | `**Persona**:` line content |
| Context | No | `**Context**:` line content |
| Action | No | `**Action**:` line content |
| Objects | No | `**Objects**:` line content, comma-separated values |
| Constraints | No | `**Constraints**:` followed by `- ` prefixed lines |
| Hints | No | `**Hints**:` followed by `- ` prefixed lines |

## Parsing

- Intent boundaries: `## ` headings
- Field extraction: `**Field**:` pattern on a single line
- List items: `- ` prefixed lines under Constraints/Hints
- Objects: comma-separated on a single line
- Feature-to-intent hierarchy: file location + heading structure

## References

- Feature: `@<feature-name>` (folder name)
- Specific intent: `@<feature-name>/<intent-slug>`
