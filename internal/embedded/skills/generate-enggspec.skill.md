# Generate Engineering Specification

Translate feature design artifacts into a formal engineering specification for handoff.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Load project config** — Read `.parlay/config.yaml` to determine the SDD framework format.

2. **Read all feature artifacts**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/surface.md`
   - `spec/intents/{feature}/devspec/buildfile.yaml` (if exists)
   - `spec/intents/{feature}/devspec/testcases.yaml` (if exists)
   - `spec/intents/{feature}/domain-model.md` (if exists)

3. **Generate specification** at `spec/intents/{feature}/enggspec/specification.md`:
   - Feature overview and user stories (from intents — Goal becomes acceptance criteria)
   - Detailed interaction requirements (from dialog flows)
   - UI component specifications (from surface fragments)
   - Data models and API contracts (from buildfile models if available)
   - Test scenarios (from testcases if available)
   - Acceptance criteria (from intent constraints)
   - Format according to the configured SDD framework (e.g., GitHub SpecKit)

4. **Report** — Print the specification path and the SDD format used. Remind the user to review before handoff.
