# Parlay Agent Instructions

This project uses the Parlay intent-driven design toolkit.
Below are the available skills. Execute them when the user requests.

---

## Skill: parlay-add-feature

# Add Feature

Create a new feature folder with intents.md and dialogs.md.

## Arguments

- `name`: The feature name (e.g., `upgrade plan creation`)

## Steps

1. Run: `parlay add-feature {name}`
2. Tell the user to start authoring intents.md.


---

## Skill: parlay-build-feature

# Build Feature

Generate buildfile.yaml and testcases.yaml for a feature using the configured framework adapter.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Load schemas** — Read these files before generating:
   - `.parlay/schemas/buildfile.schema.md`
   - `.parlay/schemas/testcases.schema.md`
   - `.parlay/schemas/adapter.schema.md`
   - `.parlay/schemas/surface.schema.md`
   - `.parlay/schemas/intent.schema.md`
   - `.parlay/schemas/dialog.schema.md`

2. **Load project config** — Read `.parlay/config.yaml` to determine the prototype framework.

3. **Load framework adapter** — Read `.parlay/adapters/{framework-slug}.adapter.yaml` for framework-specific vocabulary (component types, element types, action types, layout patterns, file conventions).

4. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/surface.md`
   - `spec/intents/{feature}/domain-model.md` (if exists)

5. **Generate buildfile.yaml** at `spec/intents/{feature}/devspec/buildfile.yaml`:
   - Set `feature:` and `adapter:` fields
   - Define `models:` from domain entities referenced in intents (Objects fields) and dialogs
   - Create `fixtures:` with representative sample data
   - Map each surface fragment to a `component:`:
     - Set `type:` to an abstract component type from the buildfile schema
     - Set `widget:` to the framework-specific widget from the adapter
     - Define `data:` inputs and computed values
     - Define `elements:` with adapter element-types
     - Define `actions:` with adapter action-types
     - Define `operations:` (file reads, writes, directory creation)
   - Define `routes:` mapping commands to components

6. **Generate testcases.yaml** at `spec/intents/{feature}/devspec/testcases.yaml`:
   - One test suite per component
   - Cover: rendering, element presence, visibility conditions, actions, state transitions
   - Reference fixtures from the buildfile
   - Follow the testcases schema exactly

7. **Validate** — Run both:
   - `parlay validate --type buildfile spec/intents/{feature}/devspec/buildfile.yaml`
   - `parlay validate --type yaml spec/intents/{feature}/devspec/testcases.yaml`
   - If validation fails, fix the issues and retry

8. **Report** — Print the created file paths and confirm success.


---

## Skill: parlay-create-surface

# Create Surface

Generate a surface.md file for a feature by analyzing its intents and dialogs.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Load schemas** — Read these files before generating:
   - `.parlay/schemas/surface.schema.md`
   - `.parlay/schemas/intent.schema.md`
   - `.parlay/schemas/dialog.schema.md`

2. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/disambiguation.yaml` (if exists — contains prior decisions, skip resolved issues)

3. **Analyze for ambiguities** — Read the intents and dialogs carefully. Identify any:
   - Ambiguities: where the same intent could be interpreted multiple ways
   - Conflicts: where intents and dialogs contradict each other
   - Missing information: where there's not enough detail to determine UI fragments

4. **If ambiguities found** — Present each one to the user:
   - Quote the relevant text from intents or dialogs
   - Explain what's ambiguous
   - Offer lettered options (A, B, C) with a recommended choice
   - Wait for the user's response
   - Ask if they want the source file updated to reflect the decision
   - Save the decision to `spec/intents/{feature}/disambiguation.yaml`

5. **Generate surface.md** — For each distinct UI piece implied by the intents and dialogs:
   - Create a fragment with a descriptive `## Name` heading
   - `**Shows**:` what the user sees (derived from intent Goal and dialog system turns)
   - `**Actions**:` what the user can do (derived from dialog options and user turns)
   - `**Source**:` `@{feature}/{intent-slug}` reference
   - If a surface.md already exists, preserve existing fragments and only add new ones

6. **Validate** — Run: `parlay validate --type surface spec/intents/{feature}/surface.md`
   - If validation fails, fix the issues and try again

7. **Report** — Tell the user what was generated and remind them to add Page and Region targets.


---

## Skill: parlay-extract-domain-model

# Extract Domain Model

Analyze all features in the project and extract a domain model.

## Steps

1. **Load schemas** — Read `.parlay/schemas/intent.schema.md`, `.parlay/schemas/dialog.schema.md`, `.parlay/schemas/surface.schema.md`.

2. **Scan all features** — Read `spec/intents/*/intents.md`, `dialogs.md`, and `surface.md`.

3. **Extract entities** — From intent Objects fields and implicit references in dialogs and surfaces:
   - For each entity, derive typed properties from how it's described and used
   - Identify relationships (belongs-to, has-many, references)
   - Identify state machines from dialog conditions and intent constraints

4. **Write domain model** — Create `spec/domain-model.md` with sections:
   - Entities (with properties and relationships for each)
   - State Machines (with explicit transitions)
   - Operation Catalog (operations implied by dialogs, mapped to commands)
   - Entity Relationship Summary (tree diagram)

5. **Report** — Print the model path and a summary of what was extracted (entity count, relationships, state machines).


---

## Skill: parlay-generate-enggspec

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


---

## Skill: parlay-load-domain-model

# Load Domain Model

Load an external domain model and integrate it with the current project's model.

## Arguments

- `path`: Path to the external domain model file

## Steps

1. **Read both models**:
   - External model at `{path}`
   - Current project model at `spec/domain-model.md` (may not exist yet)

2. **Compare entities** — For each entity in the external model:
   - If it only exists in the external model → will be added
   - If it only exists in the current model → will be kept
   - If it exists in both with different definitions → conflict

3. **If conflicts found** — Present each one to the user:
   - Show the entity name and both definitions
   - Offer options:
     - A: Keep current project definition
     - B: Use external definition
     - C: Merge properties from both
     - D: Custom mapping (user describes)
   - Wait for the user's response for each conflict
   - Save decisions to `disambiguation.yaml`

4. **Merge models** — Apply the user's decisions and write the merged result to `spec/domain-model.md`.

5. **Report** — Confirm integration and summarize what changed.


---

## Skill: parlay-lock-page

# Lock Page

Lock a page layout into a manifest with an owner.

## Arguments

- `page`: The page name (e.g., `dashboard`)

## Steps

1. Run: `parlay view-page {page}` to show the current layout.
2. Ask the user who should own this page.
3. Run: `parlay lock-page {page}` and pipe the owner name.
4. Tell the user to set the status to "reviewed" or "locked" when satisfied.


---

## Skill: parlay-register-adapter

# Register Adapter

Register a framework adapter from a YAML file.

## Arguments

- `path`: Path to the adapter YAML file

## Steps

1. Run: `parlay register-adapter {path}`
2. Tell the user the adapter is available and how to set it as the project framework.


---

## Skill: parlay-scaffold-dialogs

# Scaffold Dialogs

Generate dialog templates from authored intents so the designer has a starting point.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. Run: `parlay create-dialogs @{feature}`
2. Tell the user to review and rewrite the templates with real conversations.


---

## Skill: parlay-sync

# Sync Intents and Dialogs

Check coverage between intents and dialogs, using AI for semantic matching.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Get structural coverage** — Run: `parlay check-coverage @{feature}`
   - This outputs JSON with covered intents, uncovered intents, and orphan dialogs based on title/word matching

2. **Enhance with semantic matching** — Review the uncovered intents and orphan dialogs from the JSON output:
   - Check if any "uncovered" intent is actually covered by an orphan dialog with a different name
   - For example: intent "Configure Project Tools" might match dialog "Bootstrap Project" — these are semantically related even though the titles don't overlap
   - Present any suspected matches to the user for confirmation

3. **Report the final coverage** — Show:
   - Covered intents (structural + semantic matches)
   - Truly uncovered intents (no matching dialog at all)
   - True orphan dialogs (no matching intent)

4. **Offer template generation** — If uncovered intents exist:
   - A: Generate dialog templates for all uncovered
   - B: Let the user pick which ones
   - C: Just the report
   - If the user chooses A or B, run `parlay scaffold-dialogs @{feature}` or generate templates inline


---

## Skill: parlay-validate

# Validate

Validate a spec file against its schema.

## Arguments

- `type`: File type — `surface`, `buildfile`, `yaml`, or `analysis`
- `path`: Path to the file

## Steps

1. Run: `parlay validate --type {type} {path}`
2. Report OK or the validation errors.


---

## Skill: parlay-view-page

# View Page

Assemble and display a cross-feature page view.

## Arguments

- `page`: The page name (e.g., `dashboard`, `cluster-detail`)

## Steps

1. Run: `parlay view-page {page}`
2. If there are conflicts or unplaced fragments, help the user decide how to resolve them.


---

## CLI Utility Commands

- `parlay init` — Bootstrap project
- `parlay add-feature <name>` — Create feature folder
- `parlay create-dialogs @<feature>` — Scaffold dialog templates
- `parlay validate --type <type> <path>` — Validate a file
- `parlay parse --type <type> <path>` — Parse to JSON
- `parlay check-coverage @<feature>` — Coverage check (JSON)
- `parlay view-page <page>` — Assemble page view
- `parlay lock-page <page>` — Lock page layout
