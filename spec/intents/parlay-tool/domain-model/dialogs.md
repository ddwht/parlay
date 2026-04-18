# Domain Model — Dialogs

---

### Extract Domain Model

**Trigger**: /parlay-extract-domain-model

User: /parlay-extract-domain-model
System (background): Reads through all specifications to extract entities, relationships, and state machines.
System: The domain model is saved at ==path== and can be shared with others.

---

### Load Domain Model

**Trigger**: /parlay-load-domain-model ==path==

User: /parlay-load-domain-model ./external-model.md
System (background): Loads external model and current project model.
System (background): Compares entity definitions between both models.
System (condition: no conflicts): No conflicts found. Merging models now.
System: I've integrated the model into the current project. Everything mapped cleanly.
System (condition: conflicts found): I found ==number== conflicts that need your input:
System: Entity "==name==" exists in both models with different definitions.
  A: Keep current project definition
  B: Use external definition
  C: Merge properties from both
  D: ==custom mapping==
User: C
System (background): Merges models with resolved decisions.
System: Domain model integrated and saved.

---
