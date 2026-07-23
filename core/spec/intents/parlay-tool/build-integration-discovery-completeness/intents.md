# Build integration-discovery completeness

> The build phase's integration-discovery step (build-feature skill, step 7.5) decides where each component lives and which existing files each cross-cutting entry touches, then records the decision in the buildfile `plan:`. Two classes of dependency it does not discover, so codegen improvises them by hand. First, when a buildfile enumerates deletes, it does not trace the reverse dependencies of the symbols defined in the to-be-deleted files — a same-package caller (notably a test file) of a deleted helper is left out of `plan.modifies`, and the code phase has to add the modify entry on the fly. Second, package placement ignores language-level visibility rules: a library placed under a restricted-visibility path (Go's `internal/` tree) that is imported across a module boundary is unbuildable, and the code phase had to relocate it. This feature makes integration discovery trace reverse dependencies of deletions and defer package-placement legality to the adapter.

---

## Integration discovery traces reverse dependencies of deleted symbols

**Goal**: When a buildfile plans deletions, have integration discovery find the symbols defined in the to-be-deleted files and add every same-package caller of those symbols to `plan.modifies`, so the code phase does not have to discover a broken reference (especially in a same-package test file) and improvise a modify entry that the plan never authorized.
**Persona**: Parlay tool maintainer
**Priority**: P2
**Context**: `core/internal/embedded/skills/build-feature.skill.md` step 7.5 ("Integration discovery") resolves component homes and cross-cutting integration sites, and step 8 derives `plan.deletes` as "one entry per id in `components.removed[]`," but nothing walks the callers of what those deletes remove. A concrete case: a buildfile enumerated a file for deletion; a test file in the same package called a helper defined in that file. The delete left the test referencing an undefined symbol. The plan listed the delete but not the test file, so `plan.modifies` had no authorization to touch it — and generate-code's strict-target rule (step 14.7) refuses to write any path not in `plan.creates`/`plan.modifies`. The code phase had to improvise an extra `plan.modifies` entry for the test file to keep the package compiling. The discovery that should have happened at build time — "who references the symbols I am about to delete?" — happened at code time instead.
**Action**: Extend integration discovery so that, for every file slated for deletion, it enumerates the symbols that file defines (functions, types, constants) and greps the same package for callers, adding each caller to `plan.modifies` with `sources` citing the removed component/entry that motivated the delete. Same-package test files are in scope — a `_test.go`-style sibling that calls a deleted helper is exactly the case that bit. The scope is the deletion's own package by default; broader cross-package reference tracing can be a follow-up. The symbol enumeration + grep is mechanical; how to enumerate symbols is an adapter/language concern the step consults, not hard-coded.
**Objects**: integration-discovery, plan-deletes, deleted-symbol, same-package-caller, test-file-dependency, plan-modifies, strict-target-rule

**Constraints**:
- The reverse-dependency scan is same-package by default; if a deleted symbol has callers outside its package, the step surfaces them rather than silently ignoring (broader tracing may be deferred, but the author must not be left with an unbuildable tree and no signal).
- Every caller the scan adds to `plan.modifies` carries `sources` attribution tracing back to the delete that caused it, preserving the plan's provenance rule (no orphan modify rows).
- The scan is deterministic — the same to-be-deleted set and the same package source produce the same set of added `plan.modifies` entries, run to run.
- Language-specific symbol enumeration (what counts as a definition, how to find callers) is resolved through the adapter's conventions, not embedded as language-specific logic in the framework-agnostic skill; the skill describes the obligation ("find same-package callers of deleted symbols"), the adapter supplies the how.
- A buildfile with no deletes is unaffected — the scan runs only when `plan.deletes` is non-empty.

**Verify**:
- A buildfile that deletes a file defining a helper called by a same-package test file produces a `plan.modifies` entry for that test file at build time, with `sources` citing the delete — the code phase no longer improvises it.
- The strict-target rule in generate-code no longer blocks the (now-authorized) test-file edit, because the plan already lists it.
- A deleted symbol with a caller outside its own package is surfaced to the author rather than silently dropped.
- A buildfile with no `plan.deletes` produces byte-identical plan output to today.

---

## Package placement respects the adapter's visibility and layout rules

**Goal**: Stop integration discovery from placing a new library at a path whose language-level visibility rules make it unbuildable for its intended importers — specifically, routing a library that is imported across a module boundary out of a restricted-visibility tree (Go's `internal/`) and into a shareable location (`pkg/`) — by consulting the adapter's package-layout rules when it picks the path.
**Persona**: Parlay tool maintainer
**Priority**: P2
**Context**: Integration discovery (build-feature skill step 7.5) picks a new package's path from "the adapter's `file-conventions.source-root` and naming rules" but has no notion of language-enforced visibility. A concrete case: a buildfile placed a library under `studio/internal/`, but `core` imports that library across a module boundary. Go's `internal/` rule is path-enforced regardless of `replace` directives — a package under `internal/` is importable only within the subtree rooted at `internal/`'s parent — so the placement was unbuildable, and the code phase relocated the library to `studio/pkg/`. A related detail from the same case: a buildfile `replace` directive path must be relative to the importing module's `go.mod` location (`./studio`, not `../studio`, when the parent `go.mod` is at the repo root) — another placement/wiring rule the step did not account for. The right home for a cross-module-imported package is knowable at build time from who imports it; discovering it at code time is a wasted round-trip.
**Action**: Teach integration discovery to determine a new package's importers and route placement accordingly: a package imported only within its own module/subtree may live in a restricted-visibility location, but a package imported across a module boundary must be placed where those importers can legally reach it (e.g. `pkg/` rather than `internal/`). Encode the internal-vs-shareable choice, and the `replace`-directive path-relativity rule, in the adapter's package-layout conventions so the framework-agnostic skill defers the language specifics to the adapter and only enforces the general obligation ("place a cross-boundary-imported package where its importers can reach it").
**Objects**: package-placement, integration-discovery, cross-module-import, internal-visibility-tree, pkg-tree, replace-directive, adapter-package-layout-rule

**Constraints**:
- The visibility/layout specifics (Go's `internal/` path rule, `pkg/` convention, `replace`-directive relativity) live in the adapter's conventions, not in the embedded framework-agnostic skill — the skill states the obligation, the adapter supplies the language rule. This keeps bundled skills framework-agnostic while capturing the Go case as the motivating example.
- Placement is driven by discovered importers: a package with no cross-boundary importer keeps today's placement; only a cross-module-imported package is re-routed, so single-module projects are unaffected.
- When the step cannot determine importers unambiguously, it asks the designer (consistent with step 7.5's existing "when uncertain, ask via AskUserQuestion" behavior) rather than guessing a path that may be unbuildable.
- Any `replace` directive the buildfile records uses a path relative to the importing module's `go.mod` location per the adapter's rule, so the emitted wiring is buildable as written.
- The choice is recorded explicitly in the buildfile `plan:` (the create path reflects the legal location), not left for generate-code to guess and relocate.

**Verify**:
- A library imported across a module boundary is placed by integration discovery at a shareable path (e.g. `studio/pkg/...`), not inside a restricted-visibility `internal/` tree — the code phase no longer relocates it.
- A library imported only within its own subtree keeps today's placement (no regression for single-module or same-subtree cases).
- A recorded `replace` directive uses the correct relativity for the importing module's `go.mod` location (`./studio` when the parent `go.mod` is at the repo root).
- When importers are ambiguous, the step prompts the designer rather than emitting a placement that may not build.
