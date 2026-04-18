# Agent Integration — Dialogs

---

### Resolve Ambiguities

**Trigger**: During any skill execution when the agent encounters ambiguous content

System (background): Analyzing intents and dialogs for ambiguities.
System (condition: ambiguities found): I found some things that need your input before I can generate the surface:
System: ==context excerpt==
System: ==description of ambiguity==
  A: ==option A description==
  B: ==option B description== (recommended)
  C: ==custom input==
User: B
System: Got it. Should I update ==affected-file== to reflect this?
User: Yes
System (background): Updates source file with the resolved decision.

---
