# Domain Model

> Extracting, sharing, and integrating domain models across features and projects.

---

## Extract and Share Domain Models

**Goal**: Extract a domain model from the current project's specifications and share it with other designers or engineers working in the same domain.
**Persona**: UX Designer working with a team
**Priority**: P2
**Context**: The project has matured enough that its domain entities and relationships are valuable to other team members working on related features.
**Action**: AI agent reads through all specifications to extract entities, relationships, and state machines into a portable model file.
**Objects**: domain-model, entity, relationship, state-machine

**Constraints**:
- The domain model must be packable into a portable format that can be loaded into another project
- Loading an external domain model must integrate it with the current project's existing model
- When integration is ambiguous, the AI agent must ask the designer how to resolve it

**Verify**:
- Domain model is written to `spec/intents/{feature}/domain-model.md`
- Entities, relationships, and state machines are extracted from intents and dialogs
- Loading an external model into a project with an existing model triggers disambiguation
- Conflicting entity definitions are flagged for designer resolution

---
