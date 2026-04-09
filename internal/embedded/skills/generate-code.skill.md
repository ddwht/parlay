# Generate Code

Translate a buildfile into working prototype source code that runs and passes the generated tests.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Inputs (and the strict isolation rule)

This skill reads ONLY from these locations:

- `.parlay/schemas/buildfile.schema.md`
- `.parlay/schemas/adapter.schema.md`
- `.parlay/config.yaml`
- `.parlay/adapters/{framework}.adapter.yaml`
- `.parlay/build/{feature}/buildfile.yaml`
- The existing prototype source tree (only when doing incremental updates — for now, full regen)
- `.parlay/build/{feature}/testcases.yaml` — read **only** at the test execution phase, not during code generation

**You must NOT read anything under `spec/intents/{feature}/`.** This includes intents.md, dialogs.md, surface.md, and domain-model.md. The buildfile is the deterministic intermediate; if you find yourself wanting to read source-of-truth design files to make a decision, the buildfile is leaking detail and the right fix is to enrich the buildfile schema, not to cross the boundary.

This isolation rule is the load-bearing test for whether the buildfile is doing its job. If a code generator can produce a working, test-passing prototype using only buildfile + adapter, the buildfile is correct.

## Steps

1. **Load schemas** — Read these before generating:
   - `.parlay/schemas/buildfile.schema.md`
   - `.parlay/schemas/adapter.schema.md`

2. **Load project config** — Read `.parlay/config.yaml` to determine the prototype framework.

3. **Load framework adapter** — Read `.parlay/adapters/{framework-slug}.adapter.yaml` for framework-specific vocabulary, file conventions, and patterns.

4. **Load buildfile** — Read `.parlay/build/{feature}/buildfile.yaml`.
   - If the file does not exist: stop and tell the user to run `/parlay-build-feature @{feature}` first.
   - If the file is malformed: stop and report the YAML error.

5. **Determine source root** — From the adapter's `file-conventions.source-root` (e.g., `cmd/{feature}/`, `src/{feature}/`, `app/{feature}/`). This is where generated code will live. It must be outside `spec/` and `.parlay/`.

6. **Compute the diff** — Run: `parlay diff @{feature}` to find out what changed since the last successful end-to-end generation. The JSON output reports:
   - `components.stable[]`, `components.dirty[]`, `components.removed[]` — per-component status based on source changes (intents/dialogs/surface). On `first_build: true`, treat every component as new.
   - `sections` — per-section status for the buildfile's major sections (`models`, `routes`, `fixtures`). Values: `"changed"`, `"stable"`, `"new"`, `"removed"`. Used to determine which **cross-cutting files** (model definitions, entry points) need regeneration. Section changes don't affect per-component files — only files marked with `parlay-section:`.

7. **Scan generated files** — Run: `parlay scan-generated {source-root}` to map each file in the source root to its owning marker. Returns JSON `{source_root, files: [{feature, component, section, artifact, path}]}`.
   - Files with `parlay-component:` belong to that component. Files with `parlay-section:` belong to that buildfile section. Files with `parlay-artifact: test` are test files for their component.
   - Use this map to find file paths for dirty/removed components AND for changed sections without re-deriving filenames from the adapter naming convention.
   - Files without ANY parlay marker are user-owned; never modify or delete them.

8. **Verify stable files haven't been hand-edited** — Run: `parlay verify-generated @{feature}` to compare each recorded generated file against its stored content hash. Returns JSON `{has_hashes, stable, modified, missing}`.
   - If `has_hashes` is `false`, this is the very first generation for the feature — there is no committed code state yet. Treat every component as new regardless of what `parlay diff` reported, and skip the modified-file check entirely.
   - Otherwise, for each component the diff says is `stable`, check that its file is also in `verify.stable[]`. If the file is in `verify.modified[]`, the user has hand-edited it — STOP and surface the situation:
     ```
     <file> is marked as a stable component but has been edited since the last generation.
     A: Overwrite (lose my edits)
     B: Skip this file (keep my edits, possibly diverging from the buildfile)
     C: Show me the diff first
     ```
   - If a stable file is in `verify.missing[]`, the user deleted it — ask whether to regenerate or to drop the component.

9. **Tell the user what's about to happen** — Before regenerating, summarize: "Regenerating N component files: ... . Keeping M stable files. Deleting K removed component files."

10. **Generate code per dirty/new component** — For each component the diff classifies as dirty or new:
    - Map the component to a file path using the adapter's `component-pattern` and `naming` conventions (or, if the file already exists with a marker, reuse its path)
    - Translate the component's abstract `type`, `elements`, `actions`, and `operations` into framework-specific code using the adapter's widget mappings
    - Honor the adapter's `patterns:` section (interaction style, information density, error placement, confirmation style, content rules)
    - Add the marker at the top of every generated file. Use the comment style appropriate for the file type (`//` for Go/TS/JS, `#` for YAML/Python/shell).
    - **Component implementation files** get a two-line marker:
      ```
      // parlay-feature: {feature}
      // parlay-component: {component-name}
      ```
    - **Component test files** get a three-line marker:
      ```
      // parlay-feature: {feature}
      // parlay-component: {component-name}
      // parlay-artifact: test
      ```
      Test files ride the same component's dirty/stable status. When a component is dirty, regenerate BOTH its implementation and its test file.

11. **Delete removed-component files** — For each component in `components.removed[]`, look up the file path from the scan-generated output and delete the file. Only delete files that have a `parlay-component:` or `parlay-section:` marker — never touch user-owned files.

12. **Regenerate cross-cutting files (section-derived)** — Consult `diff.sections` to determine which cross-cutting files need regeneration:
    - If `sections.models` is `"changed"` or `"new"`: regenerate the models/types file from `buildfile.models`. Mark it with `parlay-section: models`.
    - If `sections.routes` is `"changed"` or `"new"`: regenerate the entry point from `buildfile.routes`. Mark it with `parlay-section: routes`.
    - If a section is `"stable"`: leave the corresponding file untouched (look it up via scan-generated by its `parlay-section:` marker).
    - If a section is `"removed"`: delete the corresponding file.
    - Cross-cutting files use a two-line marker:
      ```
      // parlay-feature: {feature}
      // parlay-section: models
      ```

13. **Generate test code** — Read `.parlay/build/{feature}/testcases.yaml` and translate each suite into framework-appropriate test code. Use the test framework specified in `testcases.yaml` `framework:` field. Tests live at the location the framework expects (e.g., `*_test.go` next to the source for Go).

14. **Run tests** — Execute the generated tests against the generated prototype. Capture the result.
    - **If any test fails, STOP.** Do not proceed to step 15. Report the failures and ask the user how to proceed (show details / regenerate failing components / stop). The build state must NOT be committed when tests are failing — see step 15.

15. **Commit the build state** — Only if all tests passed in step 14: run `parlay save-build-state @{feature} --source-root {source-root}`. This atomically writes both `.parlay/build/{feature}/.baseline.yaml` (source state for the next `parlay diff`) and `.parlay/build/{feature}/.code-hashes.yaml` (file hashes for the next `parlay verify-generated`).
    - This is the **only** sanctioned write path for either file. Both represent the same point in time: the end of a successful end-to-end generation. They are written together so the consistency invariant holds: the next time the user runs anything, the diff and the verify reports describe the same state.
    - If this command fails partway, the error message tells you to re-run it. The skill should propagate the error to the user.
    - Do not invoke any other save-* command. There is intentionally no `parlay save-baseline` or `parlay save-code-hashes` in the CLI — both have been folded into `save-build-state` to enforce the consistency invariant at the API level.

16. **Report** —
    - On success: list the generated files (one per component + cross-cutting files), confirm tests passed, confirm that `save-build-state` succeeded, and tell the user how to run the prototype.
    - On test failure (stopped at step 14): list the failing tests with summaries, and ask the user how to proceed. **Do not call `save-build-state` when tests have failed** — the whole point of running tests before committing state is to avoid committing a broken state.
    - On generation failure (stopped before step 14): report the underlying error and stop.

## Determinism contract

Two AI agents reading the same buildfile + adapter must produce code that passes the same testcases. The code itself does NOT need to be byte-equivalent or even structurally identical — the contract is functional determinism, measured at the testcase boundary. Agents have latitude on naming, file organization, idiomatic style, and framework-specific helpers, as long as observable behavior matches.

If two agents produce code that diverges on testcase pass/fail, that is either:
- A buildfile schema bug (missing detail) — fix the schema
- A testcase observability bug (testing implementation details) — fix the testcases.yaml generation in build-feature
- An agent bug (not following the buildfile faithfully) — fix the skill instructions

It is never a "minor difference" to be ignored.

## Incremental regeneration

Three read helpers and one write helper cooperate to make incremental rebuilds safe:

- **`parlay diff @{feature}`** — compares current sources to the saved baseline and classifies each buildfile component as `stable`, `dirty`, or `removed`. Source-of-truth for "what changed in design land."
- **`parlay scan-generated {source-root}`** — walks the source tree, finds every file with a `parlay-component:` marker, returns `path → component` map. Source-of-truth for "which file belongs to which component." Files without a marker are user-owned and excluded.
- **`parlay verify-generated @{feature}`** — compares each recorded generated file against its stored content hash from `.parlay/build/{feature}/.code-hashes.yaml`. Classifies as `stable`, `modified`, or `missing`. Source-of-truth for "did the user hand-edit a generated file."
- **`parlay save-build-state @{feature} --source-root {source-root}`** — atomically commits both the source baseline and the code hashes after a successful end-to-end generation. This is the **only** sanctioned write path for either file.

The skill calls the three read helpers before regenerating, then `parlay save-build-state` after writing files AND running tests successfully. The saves happen exactly once per successful e2e run and represent the state at that point in time.

**The very first generation** of a feature is detected by `parlay verify-generated` returning `has_hashes: false`. In that case there are no stable components to preserve and nothing to verify — treat every component as new and regenerate everything. `parlay diff` may report components as `stable` on a first run (if `parlay build-feature` left a baseline behind, which it shouldn't anymore but might from older runs) — `verify-generated`'s `has_hashes` field is the authoritative signal for "is there committed code state?"

If a stable component's file is reported as `modified` by verify-generated, the user has hand-edited it. **Do not** silently overwrite it. Surface the situation and let the user choose: overwrite, skip, or diff. The `parlay-component:` marker is the source of truth for "this file is generated"; absence of the marker means the file is user-owned and must never be touched.

## Why save-build-state is at the end (and only at the end)

The baseline (`.baseline.yaml`) and the code-hashes sidecar (`.code-hashes.yaml`) have a **consistency invariant**: they must always represent the same point in time — the end of a successful end-to-end generation. If either file is updated independently of the other, subsequent `parlay diff` and `parlay verify-generated` calls describe inconsistent states and the agent gets stuck (e.g., diff says "stable" but no code exists).

Earlier versions of the skill saved the baseline at the end of `build-feature`, before code generation. That broke the invariant: after build-feature ran but before generate-code ran, the baseline said "this source state is committed" but no code state existed for that source state. The next run would see all components as stable and skip everything.

The fix is structural: the baseline and code-hashes are written together by a single command (`parlay save-build-state`) at the end of `generate-code`, only after tests pass. The two underlying writes use the write-then-rename pattern for atomicity, so a partial failure leaves the previous state intact. If tests fail, neither file is written — the next run starts from the same state as before, so retrying is safe and deterministic.

## Error handling

- `buildfile-not-found` — `.parlay/build/{feature}/buildfile.yaml` does not exist. Tell the user to run `/parlay-build-feature @{feature}` first.
- `adapter-not-found` — `.parlay/adapters/{framework}.adapter.yaml` does not exist. Tell the user to run `/parlay-register-adapter` or `parlay init`.
- `invalid-buildfile-yaml` — YAML parse error. Show the error and ask the user to regenerate via `/parlay-build-feature`.
- `unknown-component-type` — buildfile uses a component type not in the adapter. Either the buildfile is stale (regenerate it) or the adapter needs extending.
- `source-root-collision` — adapter's source root conflicts with existing non-generated files. Ask the user how to proceed.
- `test-execution-failed` — generated tests don't pass. Show summaries and offer the menu (show details / regenerate failing components / stop).
- `spec-leak` — if you (the agent) find yourself wanting to read a file under `spec/intents/`, **do not**. Stop and report which buildfile field is missing the information you need. This is a buildfile schema bug, not an excuse to cross the boundary.
