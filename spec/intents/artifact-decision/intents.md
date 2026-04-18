# Artifact Decision

> After dialogs are authored, the agent determines whether a feature needs `surface.md`, `infrastructure.md`, or both — based on what the intents describe and what the codebase contains — and proceeds to create the appropriate artifacts without requiring a manual declaration.

---

## Determine Required Artifacts from Intents and Dialogs

**Goal**: Automatically decide whether a feature needs `surface.md` (user-facing output), `infrastructure.md` (behind-the-scenes code changes), or both, so the designer doesn't have to declare artifact types manually and the pipeline always produces the right artifacts for the feature's shape.
**Persona**: Parlay Developer
**Priority**: P1
**Context**: After dialogs are done, the pipeline needs to produce either `surface.md`, `infrastructure.md`, or both before build-feature can run. Today the designer must know which artifact to create — and understanding the distinction between surface and infrastructure shouldn't be the designer's problem. The agent already has enough signal from the intents and dialogs to make this call: intents that describe command output, prompts, and status messages imply a surface; intents that describe behavioral capabilities affecting existing code imply infrastructure; features with both kinds of intents need both artifacts.
**Action**: Add a decision step to the pipeline between dialogs and artifact creation. The agent analyzes the feature's intents and dialogs, optionally scans the codebase for brownfield context, and determines the artifact set. Then it proceeds: generating `surface.md` for user-facing fragments, and guiding `infrastructure.md` authoring for behind-the-scenes fragments. For features that need both, it produces both in sequence.
**Objects**: pipeline, artifact-decision, surface, infrastructure, intent, dialog

**Constraints**:
- The decision is based on signals already in the intents and dialogs — no new metadata or declarations required from the designer
- The signals the agent uses:
  - **Surface signals**: intents with Persona: UX Designer; dialog System turns showing CLI output, prompts, or status messages; intents whose Objects are user-facing concepts (feature, initiative, page, command output)
  - **Infrastructure signals**: intents with Persona: Parlay Developer; dialog System turns describing code modifications; intents whose Objects reference code constructs (config, resolver, validator, schema); Constraints that name specific functions or file paths
  - **Both**: when a feature has intents of both kinds — some user-facing, some behind-the-scenes
- The agent's decision is presented to the designer for confirmation before proceeding — not silently applied. A one-line summary: "This feature needs [surface / infrastructure / both]. Proceeding." The designer can override.
- If the agent can't determine the artifact type (ambiguous intents), it asks the designer via AskUserQuestion
- `/parlay-create-artifacts @feature` is the single entry point for artifact creation. It handles surface generation, infrastructure authoring guidance, or both — the designer never needs to know the distinction up front.

**Verify**:
- For `initiatives` (Persona: UX Designer, intents describe command output) → agent decides "surface"
- For `qualified-identifier-resolver` (Persona: Parlay Developer, intents reference config functions) → agent decides "infrastructure"
- For a hypothetical mixed feature (some UX Designer intents, some Parlay Developer intents) → agent decides "both"
- The agent presents its decision and the designer can override to add or remove an artifact type
- After the decision, the pipeline proceeds to create the appropriate artifacts without further prompting

---
