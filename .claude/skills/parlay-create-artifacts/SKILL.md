---
name: parlay-create-artifacts
description: "Parlay: Determine and create surface.md, infrastructure.md, or both"
---

# Create Artifacts

Determine whether a feature needs `surface.md`, `infrastructure.md`, or both — based on its intents and dialogs — then create the appropriate artifacts.

## Arguments

- `feature`: The feature slug (e.g., `initiatives`)

## Steps

1. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`

2. **Analyze intents for artifact signals** — For each intent, classify whether it contributes to surface, infrastructure, or is ambiguous. The classification is based on what the intent DESCRIBES, not on persona names (which are project-specific):

   **Surface signals** (the intent describes visible output):
   - Dialog System turns show visible output — rendered results, prompts, status messages, formatted data
   - The intent's Goal describes what someone sees or interacts with when the feature runs
   - Objects reference output-facing concepts (reports, prompts, displays, confirmations)

   **Infrastructure signals** (the intent describes internal code changes with no direct visible output):
   - Dialog System turns describe code modifications, internal operations, or structural changes
   - The intent's Goal describes changing how existing code works behind the scenes
   - Constraints name implementation details — specific functions, file paths, detection patterns
   - Objects reference internal constructs (helpers, resolvers, validators, registries)

   **Ambiguous signals** (conflicting indicators):
   - The intent describes both visible output AND internal code changes
   - Dialog shows both rendered results and code modification steps
   - Objects mix output-facing and internal concepts

   **Blueprint check** (after per-intent analysis): If `.parlay/blueprint.yaml` exists, check whether the feature's intents imply changes to any cross-cutting system documented there (e.g., deployers, registries, shared layers). Features that appear surface-only from their intents may also need infrastructure to integrate with the project's shared architecture. When in doubt, recommend **both** and explain the blueprint-derived reason to the designer.

3. **Determine the artifact set**:
   - All intents are surface, no blueprint implications → **surface only**
   - All intents are infrastructure → **infrastructure only**
   - Mix of surface and infrastructure intents → **both**
   - Surface intents with blueprint-derived infrastructure implications → **both** (explain why)
   - Any ambiguous intents → **ask the designer** (step 4)

4. **Present the decision** — Show the designer:
   - The decision (surface / infrastructure / both)
   - Per-intent breakdown: which intent maps to which artifact type and what signals drove the classification
   - Override options:
     ```
     A: Proceed with this recommendation
     B: Also add [the other artifact type]
     C: Switch to [the other artifact type] only
     D: Let me explain what this feature does (for ambiguous cases)
     ```
   - Wait for the designer's confirmation or override via AskUserQuestion

5. **Create the artifacts**:
   - **If surface**: run the existing create-surface flow (load schemas, analyze for ambiguities, generate surface.md, validate)
   - **If infrastructure**: guide the designer to author `infrastructure.md` — show the schema format, field descriptions, and an example fragment derived from the feature's intents
   - **If both**: run create-surface first, then guide infrastructure.md authoring

6. **Report** — Confirm which artifacts were created and what the next pipeline step is (`/parlay-build-feature @{feature}`).

## Error Handling

- `no-intents` — intents.md is empty or missing. Tell user to author intents first.
- `no-dialogs` — dialogs.md doesn't exist. Warn that the decision will be based on intents only (less signal). Ask whether to proceed or scaffold dialogs first.
- `artifacts-already-exist` — surface.md or infrastructure.md already exists. Ask whether to regenerate (overwrite) or skip.
