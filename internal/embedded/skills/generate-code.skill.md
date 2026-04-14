# Generate Code

Translate ALL features' buildfiles into working prototype source code at the project level. Reads every feature's buildfile, merges cross-cutting concerns (models, routes), and generates code for the entire project incrementally.

## Arguments

None — this skill operates at the project level, not per-feature.

## Inputs (and the strict isolation rule)

This skill reads ONLY from these locations:

- `.parlay/schemas/buildfile.schema.md`
- `.parlay/schemas/adapter.schema.md`
- `.parlay/schemas/blueprint.schema.md`
- `.parlay/config.yaml`
- `.parlay/blueprint.yaml` — application blueprint (optional for CLIs, recommended for web/mobile)
- `.parlay/adapters/{framework}.adapter.yaml`
- `.parlay/build/*/buildfile.yaml` — ALL features' buildfiles
- The existing prototype source tree (for incremental updates)
- `.parlay/build/*/testcases.yaml` — read **only** at the test execution phase

**You must NOT read anything under `spec/intents/{feature}/`.** This includes intents.md, dialogs.md, surface.md, and domain-model.md. The buildfile is the deterministic intermediate; if you find yourself wanting to read source-of-truth design files to make a decision, the buildfile is leaking detail and the right fix is to enrich the buildfile schema, not to cross the boundary.

This isolation rule is the load-bearing test for whether the buildfile is doing its job. If a code generator can produce a working, test-passing prototype using only buildfile + adapter, the buildfile is correct.

## Steps

1. **Load schemas** — Read these before generating:
   - `.parlay/schemas/buildfile.schema.md`
   - `.parlay/schemas/adapter.schema.md`
   - `.parlay/schemas/blueprint.schema.md`

2. **Load project config** — Read `.parlay/config.yaml` to determine the prototype framework.

3. **Load framework adapter** — Read `.parlay/adapters/{framework-slug}.adapter.yaml` for framework-specific vocabulary, file conventions, and patterns.

4. **Load ALL buildfiles** — Read `.parlay/build/*/buildfile.yaml` for every feature that has been built. If no buildfiles exist, stop and tell the user to run `/parlay-build-feature @{feature}` for at least one feature.

5. **Compute the merged model and routes** — Across all features' buildfiles:
   - MERGE the `models:` sections: collect all entity definitions from all features. If the same entity appears in multiple features with different properties, take the UNION of properties. This produces the complete model layer.
   - MERGE the `routes:` sections: collect all routes from all features. This produces the complete entry point dispatch table.
   - These merged artifacts drive the cross-cutting files (model definitions, entry point).
   - **External type resolution** (brownfield): for each entity in the merged model set, grep the source tree (under `file-conventions.source-root`) for existing type/interface/struct definitions matching the entity name (e.g., `interface User`, `type User struct`, `export type User`).
     - If exactly **one match** is found: record it as an external type (entity name → import path). In step 14, generate an import statement for this entity instead of a type declaration.
     - If **multiple matches** are found: present disambiguation to the user via AskUserQuestion:
       ```
       Found multiple existing definitions for "User":
       A: src/types/user.ts (line 14) — interface User { id: string; name: string; }
       B: src/models/auth.ts (line 42) — interface User { id: number; email: string; }
       C: Generate a new type (ignore existing definitions)
       ```
     - If **no match** is found: proceed as before (generate the type declaration).
     - Store the external type map (`{ entityName: importPath }`) for use in step 14.

6. **Load and merge blueprint** — Read `.parlay/blueprint.yaml` if it exists. The blueprint provides app-level structural decisions that complement the per-feature buildfiles:
   - For each route in the merged route table (from step 5), join on `path` to the blueprint's `navigation.routes` to determine: which `shell` wraps it, which `guard` protects it, whether it's `lazy`-loaded. Routes not listed in the blueprint get the default shell (first shell in `shells:`) and no guard.
   - Record the `navigation.strategy` and `navigation.default-route` — these drive the router component and the root redirect.
   - Record `authorization.guards` — each guard becomes a wrapper component.
   - Record `errors.boundaries` — each boundary scope becomes an error boundary component.
   - Record `state.global` — each global state slice becomes a context provider.
   - Record `data` settings — the fetching strategy and caching config drive the data infrastructure setup.
   - If the blueprint doesn't exist, proceed without it — the agent uses its own judgment for these decisions (as it did before the blueprint existed). This is the backwards-compatible path.

7. **Determine source root** — From the adapter's `file-conventions.source-root`. All features share one source root since they compile into one project.

8. **Compute the project-level diff** — Run: `parlay diff` (no @feature) to get the unified change report. The JSON output has:
   - `features.<name>.components.stable/dirty/removed` — per-feature component status based on source changes. On `first_build: true` for a feature, treat all its components as new.
   - `sections` — `models`, `routes`, `fixtures` compared across ALL features' merged buildfile sections. Values: `"changed"`, `"stable"`, `"new"`. Used to determine which project-scoped cross-cutting files need regeneration.

9. **Scan generated files** — Run: `parlay scan-generated {source-root}` to map each file to its owner.
   - Files with `parlay-feature: X + parlay-component: Y` belong to feature X's component Y.
   - Files with `parlay-scope: project + parlay-section: Z` are project-scoped cross-cutting files.
   - Files with `parlay-artifact: test` are test files for their parent component.
   - Files without ANY parlay marker are user-owned; never modify or delete them.

10. **Verify stable files haven't been hand-edited** — Run: `parlay verify-generated` (no @feature, project-level) to compare each recorded generated file against its stored content hash. Returns JSON `{has_hashes, stable, modified, missing}`.
   - If `has_hashes` is `false`, this is the very first generation — treat everything as new and skip the modified-file check.
   - Otherwise, for each component the diff says is `stable`, check that its file is in `verify.stable[]`. If the file is in `verify.modified[]`, the user has hand-edited it — STOP and surface the situation:
     ```
     <file> is marked as a stable component but has been edited since the last generation.
     A: Overwrite (lose my edits)
     B: Skip this file (keep my edits, possibly diverging from the buildfile)
     C: Show me the diff first
     ```
   - If a stable file is in `verify.missing[]`, the user deleted it — ask whether to regenerate or to drop the component.

11. **Tell the user what's about to happen** — Before regenerating, summarize: "Regenerating N component files: ... . Keeping M stable files. Deleting K removed component files."

12. **Generate code per dirty/new component** — For each component the diff classifies as dirty or new:
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

13. **Delete removed-component files** — For each component in `components.removed[]`, look up the file path from the scan-generated output and delete the file. Only delete files that have a `parlay-component:` or `parlay-section:` marker — never touch user-owned files.

14. **Regenerate cross-cutting files (section-derived)** — Consult `diff.sections` to determine which cross-cutting files need regeneration:
    - If `sections.models` is `"changed"` or `"new"`: regenerate the models/types file from `buildfile.models`. For each entity in the merged model set, check the external type map (from step 5): if the entity is external, emit an import statement pointing to the existing file instead of a type declaration; if the entity is not external, generate the type declaration as before. The resulting models file may contain a mix of imports and declarations. Mark it with `parlay-section: models`.
    - If `sections.routes` is `"changed"` or `"new"`: regenerate the entry point from `buildfile.routes`. Mark it with `parlay-section: routes`.
    - If `sections.blueprint` is `"changed"` or `"new"`: regenerate the cross-cutting blueprint-derived files:
      - **Shell components**: One layout component per shell in `blueprint.shells`. Mark each with `parlay-section: shell-{name}`.
      - **Guard components**: One route guard per guard in `blueprint.authorization.guards`. Mark each with `parlay-section: guard-{name}`.
      - **Error boundaries**: Error boundary components per scope in `blueprint.errors.boundaries`. Mark with `parlay-section: errors`.
      - **State providers**: Context providers per global state slice in `blueprint.state.global`. Mark with `parlay-section: state`.
      - **Route wiring**: The entry point / router file must reflect `navigation.strategy`, `navigation.default-route`, `navigation.not-found`, and the shell→route→guard assignments. This file is also marked `parlay-section: routes`, so it is regenerated whenever routes OR blueprint changes.
    - If a section is `"stable"`: leave the corresponding file untouched (look it up via scan-generated by its `parlay-section:` marker).
    - If a section is `"removed"`: delete the corresponding file.
    - Cross-cutting files use a two-line marker:
      ```
      // parlay-scope: project
      // parlay-section: models
      ```

14.5. **Mount into existing files (brownfield)** — This step runs only when the adapter has a `mount-strategies:` section AND the project has existing source files that are not Parlay-generated (i.e., files without `parlay-component:` or `parlay-section:` markers).

   For each route in the merged route table that references a page:

   1. **Find the target file**: search the source tree for the file implementing the page component. Use the page name from the buildfile route. If the file has a `parlay-section:` marker, it is Parlay-owned — skip (step 14 already handles it). If the file is not found, skip (new page — step 14 creates it).

   2. **Read the file**: read the full content of the target file.

   3. **Match mount strategy**: scan each strategy in the adapter's `mount-strategies:` for a `detection` pattern that appears in the file content.
      - **1 match**: proceed with this strategy automatically.
      - **0 matches**: ask the user via AskUserQuestion:
        ```
        <file> uses widgets that don't match any mount strategy in the adapter.
        How should the new <Component> be added?
        A: Show me the file so I can describe the pattern
        B: Skip — I'll integrate manually
        C: Add as a new standalone route instead
        ```
      - **Multiple matches**: ask the user to choose:
        ```
        <file> has multiple integration points:
        A: New <strategy-1-name> (found <detection-1> on line N)
        B: New <strategy-2-name> (found <detection-2> on line M)
        C: Skip — I'll integrate manually
        ```

   4. **Find existing instances**: search the file for existing instances of the chosen strategy's template pattern. These serve as style examples for indentation, prop naming, and code conventions.

   5. **Generate mount diff**: using the template with placeholders filled from the buildfile component data, and existing instances as style guides, generate the insertion code.

   6. **Present diff for review**: show the user a unified diff of the target file:
      ```
      Proposed change to <file>:

      <unified diff showing the added lines>

      A: Apply this change
      B: Skip — I'll integrate manually
      C: Edit the proposed change
      ```

   7. **Apply or skip**: on approval, write the modified file. On skip, continue to the next route. On edit, accept the user's modification and apply it.

   Mount diffs are typically small (1-3 files, a few lines each). The files being modified are page components (adding tabs, panels, sections), route config files (adding route entries), and navigation menus (adding menu items).

15. **Generate test code** — Read `.parlay/build/{feature}/testcases.yaml` and translate each suite into framework-appropriate test code. Use the test framework specified in `testcases.yaml` `framework:` field. Tests live at the location the framework expects (e.g., `*_test.go` next to the source for Go).

16. **Run tests** — Execute the generated tests against the generated prototype. Capture the result.
    - **If any test fails, STOP.** Do not proceed to step 17. Report the failures and ask the user how to proceed (show details / regenerate failing components / stop). The build state must NOT be committed when tests are failing — see step 15.

17. **Commit the build state** — Only if all tests passed in step 16: run `parlay save-build-state --source-root {source-root}`. This atomically writes:
    - Per-feature baselines for ALL features (source hashes for per-feature diff)
    - Project-level baseline at `.parlay/build/_project/.baseline.yaml` (merged section hashes)
    - Project-level code-hashes at `.parlay/build/_project/.code-hashes.yaml` (all generated files)
    - This is the **only** sanctioned write path for these files. No @feature argument — the command operates at project level.

18. **Report** —
    - On success: list the generated files (one per component + cross-cutting files), confirm tests passed, confirm that `save-build-state` succeeded, and tell the user how to run the prototype.
    - On test failure (stopped at step 16): list the failing tests with summaries, and ask the user how to proceed. **Do not call `save-build-state` when tests have failed** — the whole point of running tests before committing state is to avoid committing a broken state.
    - On generation failure (stopped before step 16): report the underlying error and stop.

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
