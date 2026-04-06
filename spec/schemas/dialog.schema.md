# Dialog Schema

File: `spec/intents/<feature-name>/dialogs.md`
Contains one or more dialog segments separated by `---`.

## Template

```
# <Feature Name> — Dialogs

---

### <Dialog Title>

**Trigger**: <What starts this dialog — a command, user action, or system event>

User: <speech or command>
System: <visible response to user>
System (background): <action not visible to user>
System (condition: <when>): <conditional response>
```

## Turn types

| Turn | Syntax | Content |
|---|---|---|
| User | `User: <content>` | Speech, action, or `/command` |
| System | `System: <content>` | Visible response — always plain text |
| Background | `System (background): <content>` | Action not visible to user — generating, reading, processing |
| Conditional | `System (condition: <when>): <content>` | Response under a specific condition |

## Content syntax

| Syntax | Meaning |
|---|---|
| Plain text | User-visible speech/message |
| `/command args` | User command |
| `==text==` | Placeholder — dynamic or variable content |
| `@reference` | Feature or intent reference |

## Options

Indented lettered list under a system turn:

```
System: How would you like to resolve this?
  A: ==Option A description==
  B: ==Option B description==
  C: ==User provides custom input==
User: Selects A
```

## Branching

Single-turn branch — use conditional turns:

```
System (condition: eligible): Ready to proceed.
System (condition: not eligible): Issues must be resolved first.
```

Multi-turn branch — use a subheading after the main dialog flow:

```
#### Branch: <Branch Name>

User: <alternative action>
System: <alternative response>
```

## Metadata

| Field | Required | Parse rule |
|---|---|---|
| Dialog Title | No | `### ` heading |
| Trigger | No | `**Trigger**:` line content |

Intent-to-dialog traceability is managed by `/parlay sync`, not by manual annotation.

## Parsing

- Segment boundaries: `---` separators
- Turn identification: line-start `User:` or `System:` or `System (modifier):`
- Turn type: parenthetical `(background)` or `(condition: ...)`
- Options: indented lines starting with `A:`, `B:`, `C:`, etc.
- Placeholders: `==...==` delimiters
- References: `@` prefix
- Branch sections: `#### Branch:` heading
- Commands: `/` prefix in user turn content

## References

- By title slug: `@<feature>/<dialog-slug>`
- By position (if untitled): `@<feature>/dialog-1`, `@<feature>/dialog-2`
