# Code Generation

> The build-and-generate pipeline — building deterministic feature specifications, generating prototype code, mounting into existing pages, and resolving external types.

---

## Build Feature Specification

**Goal**: Generate a deterministic build specification (buildfile.yaml + testcases.yaml) that captures the prototype's structure, components, and observable behaviors — without yet writing any code.
**Persona**: UX Designer
**Priority**: P0
**Context**: Surface is reviewed, framework is chosen — the designer is ready to lock down the prototype's structural spec before code generation.
**Action**: Tool loads the framework adapter, reads intents/dialogs/surface/domain-model, generates a buildfile using abstract structure filled with framework-specific vocabulary, generates testcases.yaml from the buildfile. Code generation is a separate step.
**Objects**: buildfile, testcase, framework-adapter, surface, intent, dialog, baseline

**Constraints**:
- The buildfile must be generated using the framework adapter — not hardcoded to any framework
- The same surface + different framework adapter must produce a structurally equivalent but framework-appropriate buildfile
- The buildfile is the deterministic intermediate — it must contain enough detail that two AI agents reading it produce code that passes the same testcases
- The designer must never need to read or edit the generated buildfile or testcases
- Generated artifacts must pass deep validation — all cross-references must resolve
- Buildfile operations must use the formal operations grammar — a closed set of typed operations, not free-form pseudo-code
- Build artifacts are tool internals at `.parlay/build/{feature}/`
- This intent must not commit any build state — that happens at the end of Generate Prototype Code
- Rebuilds are incremental at the component level via `parlay diff`

**Verify**:
- `buildfile.yaml` is generated at `.parlay/build/{feature}/buildfile.yaml`
- `testcases.yaml` is generated at `.parlay/build/{feature}/testcases.yaml`
- No `.baseline.yaml` or `.code-hashes.yaml` is written by this intent
- The buildfile uses only vocabulary from the loaded framework adapter
- Deep validation passes: all model references, component references, fixture data, and adapter types resolve

---

## Generate Prototype Code

**Goal**: Translate the build specification into working prototype code that runs and passes the generated tests.
**Persona**: UX Designer
**Priority**: P0
**Context**: Build Feature Specification has produced buildfile.yaml + testcases.yaml. The designer wants a runnable prototype.
**Action**: Tool loads the buildfile and the framework adapter, generates code files following the adapter's file conventions, runs the testcases against the prototype, and reports pass/fail.
**Objects**: prototype, buildfile, testcase, framework-adapter, code-file

**Constraints**:
- Code generation reads ONLY the buildfile, adapter, and existing source tree — MUST NOT read anything under `spec/intents/{feature}/`
- Two AI agents reading the same buildfile must produce code that passes the same testcases
- Generated code lives at the location specified by the adapter's `file-conventions.source-root`
- Incremental regeneration is driven by `parlay diff`, `parlay scan-generated`, and `parlay verify-generated`
- Generated files are marked with `parlay-component:` / `parlay-section:` markers for ownership tracking
- If `parlay verify-generated` reports a hand-edited stable file, the agent must NOT silently overwrite it
- Tests must pass before build state is committed via `parlay save-build-state`
- If tests fail, `save-build-state` MUST NOT be called

**Verify**:
- Prototype code is generated at the adapter's source root
- Generated tests pass against the generated prototype
- Code generation does not access any file under `spec/intents/{feature}/`
- Each generated file is traceable back to a buildfile component

---

## Mount Generated Feature into Existing Pages

**Goal**: When generating code for a new feature that targets an existing page, produce a reviewable diff showing exactly how the new component integrates into the existing file, rather than regenerating the entire file.
**Persona**: UX Designer
**Priority**: P0
**Context**: Brownfield project — an existing page already has content, and a new feature adds to it. The agent must not overwrite the existing page but instead propose a small change.
**Action**: During generate-code, the agent reads the target page file, matches a mount strategy from the adapter, finds existing instances of the pattern as style examples, generates a new instance, and presents a diff for user confirmation.
**Objects**: mount-strategy, mount-point, diff, existing-file, component

**Constraints**:
- Must never silently modify existing files — all changes to non-Parlay files are presented as reviewable diffs
- Must read the target file first, then match a mount strategy by looking for the `detection` pattern in the file content
- When zero strategies match: ask the user how to integrate
- When multiple strategies match: present each with the line number, let user choose
- When exactly one matches: proceed automatically
- Files with `parlay-section:` markers are Parlay-owned and handled separately
- Mount diffs are typically small: a few lines each

**Verify**:
- Agent produces a diff for each existing file that needs modification
- User can approve, skip, or edit each proposed diff
- Existing file content outside the mount point is never modified
- Greenfield behavior is unchanged when no existing files are found

---

## Resolve External Types During Code Generation

**Goal**: When generating code that declares a model type, detect if that type already exists in the source tree and import it instead of re-declaring it, avoiding namespace collisions in brownfield projects.
**Persona**: Developer
**Priority**: P1
**Context**: Brownfield project has existing type definitions that new features reference. Without detection, generate-code would create duplicate type declarations.
**Action**: At generate-code time, before writing the models cross-cutting file, grep the source tree for existing type definitions matching each entity name. If found, emit an import instead of a type declaration.
**Objects**: model, type, import, external-type

**Constraints**:
- Detection happens by grepping source code at generation time — no persistent type index required
- Must handle framework-specific type declaration patterns
- When exactly one match is found: record as external, generate import
- When multiple matches: present disambiguation with file paths and snippets
- When no match: generate the type declaration as before
- Must not modify existing type files — only the generated models file changes
- The buildfile schema does not change — resolution is fully at generate-code time

**Verify**:
- When an existing type is found, the generated code imports it from its actual location
- When no existing type is found, a new type declaration is generated
- Multiple matches trigger disambiguation
- Existing type files are never modified by Parlay

---
