# Agent Integration

> How parlay integrates with AI agents — skills as agent-agnostic markdown, deployers that package them, and conversational disambiguation.

---

## Integrate with AI Agent via Skills

**Goal**: Provide AI-heavy capabilities as agent skills — markdown files the AI agent reads and executes natively — while keeping the CLI as a helper binary for mechanical operations.
**Persona**: Tool creator
**Priority**: P0
**Context**: Commands that need intelligence cannot be implemented in a CLI alone. The agent should orchestrate the workflow and call the CLI for parsing, validation, and scaffolding.
**Action**: Each AI-heavy command is defined as a skill file (plain English markdown) that the agent reads. The skill instructs the agent what schemas to load, what files to read, what analysis to do, and what to generate. The agent calls the parlay binary for validation and structured parsing.
**Objects**: skill, agent-deployer, schema, framework-adapter

**Constraints**:
- Skills are authored once as agent-agnostic markdown — plain English instructions any AI can follow
- Agent-specific deployers package skills into the right format per agent
- Adding a new agent requires only a new deployer — zero changes to skill content or schemas
- The helper binary exposes parsing, validation, and coverage checking as JSON-output subcommands the agent can call
- Skills reference schemas from .parlay/schemas/ for on-demand loading — not embedded in the skill itself
- Disambiguation is handled conversationally by the agent — no YAML round-trip needed

**Verify**:
- Skill files are plain markdown readable by any AI agent
- The same skill file works across different agents via deployers
- Adding a new agent requires only a new deployer, not skill or schema changes
- The parlay binary responds to subcommands with JSON output

---

## Resolve Ambiguities Through AI Dialogue

**Goal**: Have the AI agent identify and resolve ambiguities in intents, dialogs, and surfaces by asking the designer directly during specification creation.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer has written intents and dialogs, but some details are ambiguous, incomplete, or contradictory — the AI agent needs to clarify before generating output.
**Action**: The agent analyzes documents, identifies ambiguities, presents each one to the designer as a conversational question with lettered options, waits for the response, then proceeds with generation.
**Objects**: intent, dialog, surface

**Constraints**:
- The agent talks directly to the designer — no CLI mediator, no YAML round-trip, no side-channel cache
- Each ambiguity is presented with lettered options and an optional freeform choice
- The agent must ask permission before updating any human-owned file
- Resolved decisions are incorporated back into the source documents — once the source is updated, the ambiguity is gone
- Deferred decisions are added to the affected intent's `**Questions**:` field

**Verify**:
- Ambiguities are presented one at a time with lettered options
- The designer's choice is incorporated into the source document
- Deferred decisions land in the relevant intent's `**Questions**:` field
- No human-owned file is modified without explicit permission

---
