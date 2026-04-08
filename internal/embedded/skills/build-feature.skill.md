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

5. **Check readiness** — Run: `parlay check-readiness @{feature} --stage build-feature`
   - This returns JSON with errors (blocking) and warnings (non-blocking)
   - If any errors are reported, present them to the user with the suggested fixes and stop — do not proceed to generation
   - If only warnings are reported (e.g., open questions), inform the user and ask whether to proceed

6. **Compute the diff** — Run: `parlay diff @{feature}` to find out what changed since the last build.
   - On the first build (`first_build: true`) or when the buildfile doesn't exist yet (`has_buildfile: false`), generate the entire buildfile from scratch.
   - On subsequent builds, the JSON output reports:
     - `components.stable[]` — components whose source dependencies (fragment, referenced intents, matching dialogs) all have unchanged hashes. **Preserve these entries verbatim** in the new buildfile — do not regenerate them.
     - `components.dirty[]` — components whose dependency chain has changes. Regenerate only these components, using `changed_sources` as a hint about what to focus on.
     - `components.removed[]` — components whose source fragment no longer exists. Drop these entries from the new buildfile.
     - `surface_fragments.new[]` — fragments in the current surface that aren't in any existing component. Decide whether to introduce new components for them.
   - Tell the user what's about to be regenerated before doing it (e.g., "Regenerating 2 components: upgrade-prompt, preflight-check. Keeping 5 stable components.").

7. **Generate buildfile.yaml** at `.parlay/build/{feature}/buildfile.yaml` (tool-internal location — designer never sees this):
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
   - Use intent Priority to guide component ordering and emphasis (P0 intents produce primary components)

8. **Generate testcases.yaml** at `.parlay/build/{feature}/testcases.yaml` (tool-internal — drives cross-validation and feeds spec generation, never handed off to engineering):
   - One test suite per component
   - Set `intent:` on each suite to `@{feature}/{intent-slug}` for traceability
   - Use the intent's **Verify** bullets as the basis for test assertions
   - Cover: rendering, element presence, visibility conditions, actions, state transitions
   - Reference fixtures from the buildfile
   - Follow the testcases schema exactly

9. **Validate** — Run all (use `--json` flag for structured error parsing):
   - `parlay validate --type buildfile --deep --adapter .parlay/adapters/{adapter}.adapter.yaml --json .parlay/build/{feature}/buildfile.yaml`
   - `parlay validate --type yaml --json .parlay/build/{feature}/testcases.yaml`
   - Deep validation checks: model references, component references in routes, fixture-model alignment, children references, and adapter vocabulary
   - If validation fails, parse the JSON error output and apply the fix from each error's `fix` field, then retry

10. **Report** — Confirm the build specification is ready, mention that the artifacts live under `.parlay/build/{feature}/` (tool internals), and tell the user to run `/parlay-generate-code @{feature}` next to produce the prototype code and run tests.

This skill stops at the build specification. **Do NOT save the baseline or any other build state from this skill.** The baseline (`.baseline.yaml`) and the code-hashes sidecar (`.code-hashes.yaml`) are committed atomically as a unit at the end of `/parlay-generate-code`, only after a successful end-to-end generation. Saving baseline here would commit source state without corresponding code state, breaking the consistency invariant and stranding the feature in a state where `parlay diff` reports everything stable but no code exists.

Code generation is a separate skill (`/parlay-generate-code`) so that the buildfile.yaml can serve as a clean context boundary — codegen reads only buildfile + adapter and never reaches back into `spec/intents/`. Do NOT extend this skill to write code or to save state.

## Error Handling

When `parlay check-readiness` returns errors:
- `no-intents` — intents.md is empty or missing. Tell user to author intents first.
- `missing-goal` / `missing-persona` — required field missing. Show which intent and ask user to fill it in.
- `no-surface` — surface.md doesn't exist. Suggest running `/parlay-create-surface @{feature}` first.
- `fragment-missing-page` — surface fragment has no Page target. Show which fragment and ask user to add it.
- `fragment-missing-source` — surface fragment has no Source. Likely a bug in surface generation; regenerate the surface.
- `no-config` / `no-prototype-framework` — project not initialized. Suggest running `parlay init`.

When `parlay validate --type buildfile --deep --json` returns errors:
- `missing-model-reference` — a component references a model that doesn't exist. Either add the model to `models:` or change the input. The error's `context` field shows the component path.
- `missing-component-reference` — a route references a component that doesn't exist. Either add the component or remove it from the route.
- `missing-child-reference` — a component's `children:` references a non-existent component. Add or remove.
- `missing-fixture-model` — a fixture references a model that doesn't exist. Add the model or remove the fixture data.
- `unknown-component-type` — a component uses a type not in the adapter. Either change the type to one supported by the adapter, or extend the adapter.
- `adapter-not-found` — the adapter file is missing. Verify `.parlay/adapters/{name}.adapter.yaml` exists.
- `invalid-yaml` / `invalid-adapter-yaml` — YAML syntax error. Show the error to the user and ask them to fix or regenerate.

For all errors: parse the JSON `errors` array, apply each error's `fix` automatically when possible (e.g., regenerating a section), or present the error and fix to the user when human input is required.
