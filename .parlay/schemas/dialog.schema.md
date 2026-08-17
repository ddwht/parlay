# Dialog Schema

File: `spec/intents/<feature-name>/dialogs.md`
Contains one or more dialog segments separated by `---`.

## Template
<!-- parlay:normative -->



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

<!-- /parlay:normative -->

## Turn types
<!-- parlay:normative -->



| Turn | Syntax | Content |
|---|---|---|
| User | `User: <content>` | Speech, action, or `/command` |
| System | `System: <content>` | Visible response — always plain text |
| Background | `System (background): <content>` | Action not visible to user — generating, reading, processing |
| Conditional | `System (condition: <when>): <content>` | Response under a specific condition |

<!-- /parlay:normative -->

## Content syntax
<!-- parlay:normative -->



| Syntax | Meaning |
|---|---|
| Plain text | User-visible speech/message |
| `/command args` | User command |
| `==text==` | Placeholder — dynamic or variable content |
| `@reference` | Feature or intent reference |

<!-- /parlay:normative -->

## Options
<!-- parlay:normative -->



Indented lettered list under a system turn:

```
System: How would you like to resolve this?
  A: ==Option A description==
  B: ==Option B description==
  C: ==User provides custom input==
User: Selects A
```

<!-- /parlay:normative -->

## Branching
<!-- parlay:normative -->



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

<!-- /parlay:normative -->

## Metadata
<!-- parlay:normative -->



| Field | Required | Parse rule |
|---|---|---|
| Dialog Title | No | `### ` heading |
| Trigger | No | `**Trigger**:` line content |

Intent-to-dialog traceability is managed by `/parlay sync`, not by manual annotation.

<!-- /parlay:normative -->

## Parsing

- Segment boundaries: `---` separators
- Turn identification: line-start `User:` or `System:` or `System (modifier):`
- Turn type: parenthetical `(background)` or `(condition: ...)`
- Options: indented lines starting with `A:`, `B:`, `C:`, etc.
- Placeholders: `==...==` delimiters
- References: `@` prefix
- Branch sections: `#### Branch:` heading
- Commands: `/` prefix in user turn content

## Validation

`parlay validate --type dialog spec/intents/<feature>/dialogs.md` checks a file
against this schema.

Dialogs have no required fields — a dialogs.md with no dialogs in it is what
`parlay add-feature` writes and is a valid starting point. What is checkable is
the closed set of four turn forms above. Anything else the parser ignores, so a
near-miss turn (`System (foo):`, `Systems:`, `System (condition: x) : y`) is not
a syntax error anywhere — the turn simply is not there, and every artifact
derived from the dialog is missing it with no indication why.

| Code | When it fires |
|---|---|
| `dialogs-not-readable` | The file does not exist, or cannot be read as markdown |
| `unknown-turn-form` | A line names `User` or `System` as its speaker but matches none of the four documented turn forms, so it is dropped rather than read as a turn. Lines inside fenced code blocks are not checked — those are examples, not transcript. |

## References

- By title slug: `@<feature>/<dialog-slug>`
- By position (if untitled): `@<feature>/dialog-1`, `@<feature>/dialog-2`
