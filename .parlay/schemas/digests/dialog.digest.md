# Dialog Schema — authoring digest

Derived from `dialog.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

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

---

| Turn | Syntax | Content |
|---|---|---|
| User | `User: <content>` | Speech, action, or `/command` |
| System | `System: <content>` | Visible response — always plain text |
| Background | `System (background): <content>` | Action not visible to user — generating, reading, processing |
| Conditional | `System (condition: <when>): <content>` | Response under a specific condition |

---

| Syntax | Meaning |
|---|---|
| Plain text | User-visible speech/message |
| `/command args` | User command |
| `==text==` | Placeholder — dynamic or variable content |
| `@reference` | Feature or intent reference |

---

Indented lettered list under a system turn:

```
System: How would you like to resolve this?
  A: ==Option A description==
  B: ==Option B description==
  C: ==User provides custom input==
User: Selects A
```

---

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

---

| Field | Required | Parse rule |
|---|---|---|
| Dialog Title | No | `### ` heading |
| Trigger | No | `**Trigger**:` line content |

Intent-to-dialog traceability is managed by `/parlay sync`, not by manual annotation.
