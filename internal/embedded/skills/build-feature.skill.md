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
