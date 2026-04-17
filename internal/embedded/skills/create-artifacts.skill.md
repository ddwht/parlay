# Create Artifacts

Determine whether a feature needs `surface.md`, `infrastructure.md`, or both — based on its intents and dialogs — then create the appropriate artifacts. Replaces the manual choice between `/parlay-create-surface` and authoring `infrastructure.md` directly.

## Arguments

- `feature`: The feature slug (e.g., `initiatives`)

## Steps

1. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`

2. **Analyze intents for artifact signals** — For each intent, classify whether it contributes to surface, infrastructure, or is ambiguous:

   **Surface signals** (the intent describes user-facing output):
   - Persona is UX Designer
   - Objects reference user-facing concepts (feature, initiative, page, command output, report)
   - Dialog System turns show CLI output, prompts, status messages, or formatted results
   - The intent's Goal describes what the user sees or interacts with

   **Infrastructure signals** (the intent describes behind-the-scenes code changes):
   - Persona is Parlay Developer (or equivalent non-designer role)
   - Objects reference code constructs (config, resolver, validator, schema, helper, function name)
   - Constraints name specific functions, file paths, or detection patterns
   - Dialog System turns describe code modifications, file changes, or internal operations
   - The intent's Goal describes changing how existing code works

   **Ambiguous signals** (conflicting indicators):
   - Persona is UX Designer but Constraints reference internal function names
   - Dialog shows both CLI output and code modification steps
   - Objects mix user-facing and code-level concepts

3. **Determine the artifact set**:
   - All intents are surface → **surface only**
   - All intents are infrastructure → **infrastructure only**
   - Mix of surface and infrastructure intents → **both**
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
