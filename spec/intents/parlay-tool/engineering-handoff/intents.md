# Engineering Handoff

> Translating design artifacts into formal engineering specifications for the development team.

---

## Generate Engineering Specification

**Goal**: Translate the design artifacts into a formal engineering specification that the development team can use to build the production system.
**Persona**: UX Designer working with engineers
**Priority**: P1
**Context**: The prototype has been validated and the design is stable — it's time to hand off to engineering in their preferred specification format.
**Objects**: engineering-spec, sdd-framework, intent, dialog, surface

**Constraints**:
- Must support popular SDD frameworks and be extensible for new formats
- The generated specification must be reviewable by the designer before handoff
- The engineering spec lives in `spec/handoff/{feature}/`, separate from designer-facing inputs in `spec/intents/{feature}/`
- `specification.md` is currently the only handoff artifact

**Verify**:
- Engineering spec is generated in the format matching the configured SDD framework
- The generated spec is written to `spec/handoff/{feature}/specification.md`
- The designer can review the spec before it is shared with engineering

---
