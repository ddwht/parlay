# Engineering Handoff — Dialogs

---

### Generate Engineering Specification

**Trigger**: /parlay-generate-enggspec @feature-name

User: /parlay-generate-enggspec @upgrade-plan-creation
System: We're ready to hand over to engineering. Let me read through our specifications and generate an engineering spec in ==SDD framework== format.
System (background): Reads intents, dialogs, surface from spec/intents/ and buildfile + testcases from .parlay/build/. Translates to configured SDD format.
System: The specification is ready: spec/handoff/upgrade-plan-creation/specification.md
System: Review it and hand it over to the engineering team.

---
