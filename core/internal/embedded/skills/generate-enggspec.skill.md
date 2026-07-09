---
name: generate-enggspec
description: "Generate engineering specification"
---

# Generate Engineering Specification

Translate feature design artifacts into a formal engineering specification for handoff.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

## Steps

1. **Load project config** — Read `.parlay/config.yaml` to determine the SDD framework format.

2. **Read all feature artifacts** — <!-- parlay:expand-co-equal-artifacts -->; read whichever subset the feature has:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/surface.yaml` or `surface.md` (if exists — YAML wins when both are present)
   - `spec/intents/{feature}/capabilities.yaml` (if exists)
   - `spec/intents/{feature}/infrastructure.md` (if exists)
   - `<activeRoot>/domain-model.yaml` (if exists — the project's one canonical domain model, not per-feature) or legacy `spec/intents/{feature}/domain-model.md`
   - `.parlay/build/{feature}/buildfile.yaml` (if exists — tool-internal)
   - `.parlay/build/{feature}/testcases.yaml` (if exists — tool-internal; project the observable-behavior assertions into the spec's Acceptance Criteria section)

3. **Generate specification** at `spec/handoff/{feature}/specification.md` (this is the only handoff artifact — engineering reads it and writes their own tests):
   - Feature overview and user stories (from intents — Goal becomes acceptance criteria)
   - Detailed interaction requirements (from dialog flows)
   - UI component specifications (from surface fragments)
   - Operation contracts (from `capabilities.yaml`, if present — each command/query's input, output shape, allowed errors, and policies)
   - Constraints and invariants (from `infrastructure.md`, if present — one entry per fragment's Behavior and Invariants)
   - Data models and API contracts (from buildfile models if available)
   - Test scenarios (from testcases if available)
   - Acceptance criteria (from intent constraints)
   - Format according to the configured SDD framework (e.g., GitHub SpecKit)

4. **Report** — Print the specification path and the SDD format used. Remind the user to review before handoff.

## Error Handling

- Absent optional artifacts (`surface.yaml`/`surface.md`, `capabilities.yaml`, `infrastructure.md`, `domain-model.yaml`) are skipped silently — the corresponding specification section is simply omitted.
- `no-spec-artifacts` — the feature has none of the four spec artifacts. Tell the user to run `/parlay-create-artifacts @{feature}` first.
