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

6. **Compute the diff** — Run: `parlay diff @{feature}` to find out what changed since the last build.
   - On the first generation (`has_buildfile: true` but no prior generated code), regenerate every component.
   - On subsequent generations, the JSON output reports:
     - `components.stable[]` — components whose buildfile entry derives from unchanged sources. **Skip code regeneration for these components.** Their existing code files (identified by the `parlay-component:` marker comment) are still valid.
     - `components.dirty[]` — components that need code regeneration. Regenerate only these.
     - `components.removed[]` — components whose source fragment no longer exists. Delete their code files (identified by the `parlay-component:` marker).
   - Tell the user what's about to be regenerated before doing it (e.g., "Regenerating 2 component files: upgrade_prompt.go, preflight_check.go. Keeping 5 stable files.").
   - If `parlay diff` reports `first_build: true`, treat the entire feature as new and regenerate everything.

7. **Generate code per component** — For each component in the buildfile:
   - Map the component to a file path using the adapter's `component-pattern` and `naming` conventions
   - Translate the component's abstract `type`, `elements`, `actions`, and `operations` into framework-specific code using the adapter's widget mappings
   - Honor the adapter's `patterns:` section (interaction style, information density, error placement, confirmation style, content rules)
   - Add a marker comment at the top of each generated file linking back to the buildfile component, e.g.:
     ```
     // parlay-component: <component-name>
     // Generated from .parlay/build/{feature}/buildfile.yaml — do not edit by hand
     ```
     (For non-comment-supporting languages, use the closest equivalent: frontmatter, sidecar file, etc.)

8. **Generate cross-cutting files** — From the buildfile's `models:`, `routes:`, and any framework-required entry points (per the adapter's `entry-point` field), produce the supporting files: data models, routing/main, fixtures used as runtime data sources, etc.

9. **Generate test code** — Read `.parlay/build/{feature}/testcases.yaml` and translate each suite into framework-appropriate test code. Use the test framework specified in `testcases.yaml` `framework:` field. Tests live at the location the framework expects (e.g., `*_test.go` next to the source for Go).

10. **Run tests** — Execute the generated tests against the generated prototype. Capture the result.

11. **Report** —
    - On success: list the generated files (one per component + cross-cutting files), confirm tests passed, and tell the user how to run the prototype.
    - On test failure: list the failing tests with summaries, and ask the user how to proceed (show details / regenerate failing components / stop).
    - On generation failure: report the underlying error and stop.

## Determinism contract

Two AI agents reading the same buildfile + adapter must produce code that passes the same testcases. The code itself does NOT need to be byte-equivalent or even structurally identical — the contract is functional determinism, measured at the testcase boundary. Agents have latitude on naming, file organization, idiomatic style, and framework-specific helpers, as long as observable behavior matches.

If two agents produce code that diverges on testcase pass/fail, that is either:
- A buildfile schema bug (missing detail) — fix the schema
- A testcase observability bug (testing implementation details) — fix the testcases.yaml generation in build-feature
- An agent bug (not following the buildfile faithfully) — fix the skill instructions

It is never a "minor difference" to be ignored.

## Incremental regeneration

Incremental rebuilds work via `parlay diff @{feature}`, which compares current sources to the baseline saved at the last build. The diff output classifies each component as `stable`, `dirty`, or `removed`. The skill regenerates only dirty components, leaves stable components untouched, and deletes code files for removed components (identified by the `parlay-component:` marker).

For the very first generation of a feature (no prior code, or `first_build: true` from diff), full regen is the only option — there are no stable components to preserve.

If you encounter a stable component but its file has been hand-edited or moved by the user, **do not** silently overwrite it. Surface the situation and ask the user how to proceed. The `parlay-component:` marker is the source of truth for "this file is generated"; absence of the marker means the file is user-owned.

## Error handling

- `buildfile-not-found` — `.parlay/build/{feature}/buildfile.yaml` does not exist. Tell the user to run `/parlay-build-feature @{feature}` first.
- `adapter-not-found` — `.parlay/adapters/{framework}.adapter.yaml` does not exist. Tell the user to run `/parlay-register-adapter` or `parlay init`.
- `invalid-buildfile-yaml` — YAML parse error. Show the error and ask the user to regenerate via `/parlay-build-feature`.
- `unknown-component-type` — buildfile uses a component type not in the adapter. Either the buildfile is stale (regenerate it) or the adapter needs extending.
- `source-root-collision` — adapter's source root conflicts with existing non-generated files. Ask the user how to proceed.
- `test-execution-failed` — generated tests don't pass. Show summaries and offer the menu (show details / regenerate failing components / stop).
- `spec-leak` — if you (the agent) find yourself wanting to read a file under `spec/intents/`, **do not**. Stop and report which buildfile field is missing the information you need. This is a buildfile schema bug, not an excuse to cross the boundary.
