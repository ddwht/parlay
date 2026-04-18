# Artifact Decision — Surface

---

## Artifact Decision Prompt

**Shows**: message, data-list, status
**Actions**: select-one
**Source**: @artifact-decision/determine-required-artifacts-from-intents-and-dialogs

**Page**: create-artifacts
**Region**: main
**Order**: 1

**Notes**:
- The developer-facing prompt shown by `/parlay-create-artifacts @feature` after analyzing intents and dialogs.
- `message` states the agent's decision and its reasoning: which signals it found (Persona, Objects, Constraints, dialog turns) and what they imply (surface, infrastructure, or both).
- `data-list` names the specific intents that contributed to the decision, grouped by artifact type — e.g., "Surface: Intent A, Intent B. Infrastructure: Intent C."
- `status` shows [OK] when the decision is unambiguous, or [?] when the agent needs the designer's input.
- `select-one` presents override options: (A) proceed with the agent's recommendation, (B/C) switch to a different artifact set, (D) explain what the feature does (for ambiguous cases).
- After confirmation, the agent proceeds to create the appropriate artifacts — generating surface.md for surface fragments, guiding infrastructure.md authoring for infrastructure fragments, or both in sequence.

---
