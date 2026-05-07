# Cross-cutting-target-paths — Dialogs

---

### Cross-cutting `target-files:` map to plan by entry kind

**Trigger**: `parlay validate --type buildfile --deep <path>` (or its in-process equivalent invoked by `parlay build-feature` / `parlay generate-code`) reaches the cross-cutting plan-cross-check step in `validatePlanSection`.

User: Submits a buildfile whose cross-cutting entry has `target-files: [src/registry/feature.go]`, where `src/registry/feature.go` does not exist on disk.
System (background): Walks the entry's `target-files:` paths, stat-checks each against the source root, classifies the entry as "purely-introducing" (every path is missing on disk and the entry has no `target-modifies:` companion field).
System (background): For every `target-files:` path, looks for a matching `plan.creates` row whose `sources:` cites this cross-cutting.
System (condition: every path appears in `plan.creates` with the cross-cutting as source): Validation passes for this entry.
System (condition: a path does not appear in `plan.creates`): Emits `cross-cutting-target-not-in-creates` naming the offending path, the cross-cutting id, and a fix message: `add plan.creates entry { path: <path>, sources: [cross-cutting/<id>] }`.

User: Submits a buildfile whose cross-cutting entry has `target-files: [routes/index.go]`, where `routes/index.go` exists on disk.
System (background): Stat-checks the path, finds it on disk, classifies the entry as "modifies-only".
System (background): For every `target-files:` path, looks for a matching `plan.modifies` row whose `sources:` cites this cross-cutting.
System (condition: every path appears in `plan.modifies` with the cross-cutting as source): Validation passes for this entry (today's behavior, no regression).
System (condition: a path does not appear in `plan.modifies`): Emits `cross-cutting-target-not-in-modifies` naming the offending path, the cross-cutting id, and a fix message citing `plan.modifies`.

#### Branch: Purely-introducing entry's path is incorrectly placed in `plan.modifies` (the legacy workaround attempt)

User: Submits a buildfile whose cross-cutting entry has `target-files: [src/new-thing.go]` (path missing on disk) AND a `plan.modifies` row pointing at `src/new-thing.go`.
System (background): The disk-shape check `plan-modify-target-missing` fires first: the path in `plan.modifies` does not exist in the source root.
System: Emits `plan-modify-target-missing` naming `src/new-thing.go` with the existing fix message: `either fix the path, or move the entry to plan.creates if this feature genuinely introduces the file`. The new routing rule does not silently rescue this case — the legacy error is still the right diagnosis.

#### Branch: Purely-introducing entry has no plan row at all

User: Submits a buildfile whose cross-cutting entry has `target-files: [src/new-thing.go]` (path missing on disk) and no `plan.creates` or `plan.modifies` row references this cross-cutting.
System: Emits `cross-cutting-target-not-in-plan` (the legacy entry-level code, retained) naming the cross-cutting id and the missing target, with a fix message naming `plan.creates`: `add plan.creates entry { path: src/new-thing.go, sources: [cross-cutting/<id>] }`.

#### Branch: Mixed-kind `target-files:` list (some on disk, some not)

User: Submits a cross-cutting entry with `target-files: [routes/index.go, routes/feature.go]` where `routes/index.go` exists on disk and `routes/feature.go` does not.
System (background): Stat-checks each path, observes mixed kinds in a single list, and refuses to apply either routing rule.
System: Emits `cross-cutting-mixed-target-kinds` naming both paths and their disk states, with a fix message offering two paths: `split into two cross-cutting entries (one per kind), or use the both-kinds shape: declare existing-on-disk paths in target-files: and not-yet-existing paths in target-creates:` (referencing the dialog-3 shape).

#### Branch: Schema clarification reads back during build

User: Runs `parlay build-feature` which writes a new buildfile with a purely-introducing cross-cutting.
System (background): The build-feature emitter consults the (now-amended) buildfile schema; its `Authoring rule for build-feature` paragraph explicitly states the routing — purely-introducing → `plan.creates`, modifies-only → `plan.modifies`. The emitter writes plan rows accordingly.
System: The emitted buildfile validates cleanly under the same rules at codegen time. No round-trip through `target-pattern:` is needed.

---

### `target-pattern:` resolves to concrete paths at validation time

**Trigger**: `validatePlanSection` reaches a cross-cutting entry whose `target-pattern:` field is non-empty.

User: Submits a cross-cutting entry with `target-pattern: "src/registry/*.go"` and no `target-files:`.
System (background): Resolves the pattern by globbing the union of (a) on-disk files under the source root and (b) the cross-pass `plannedCreates` set (when running in project-pass mode; see dialog 4). The resolved set is treated as a concrete `target-files:` list and routed by entry kind per dialog 1.
System (condition: resolved set is non-empty and routing succeeds): Validation passes for this entry.
System (condition: resolved set is empty): Emits `cross-cutting-pattern-empty` citing the pattern and the search roots that produced no match: `target-pattern '<pattern>' resolved to zero paths against <root> and <plannedCreates count> cross-pass creates — fix the glob or remove the entry`.

#### Branch: Pattern matches on-disk files only (modifies-only resolution)

User: Submits an entry with `target-pattern: "src/registry/*.go"` against a source root that contains three matching files (`a.go`, `b.go`, `c.go`).
System (background): Resolves to the three concrete paths; classifies the entry as modifies-only because every resolved path exists on disk.
System (condition: all three appear in `plan.modifies` with the cross-cutting as source): Validation passes.
System (condition: any path is missing from `plan.modifies`): Emits `cross-cutting-target-not-in-modifies` per resolved path, naming the path produced by pattern resolution.

#### Branch: Pattern matches a future-create from another feature (project-pass)

User: Submits an entry with `target-pattern: "src/generated/clients/*.ts"` in a project where another feature's buildfile has `plan.creates: [src/generated/clients/api.ts]`. No on-disk file matches.
System (background): Project-pass validation passes the cross-pass `plannedCreates` set into resolution. The pattern matches `src/generated/clients/api.ts` from the cross-pass set; the resolved set is one path; the entry is classified purely-introducing (the path is not on disk).
System (condition: the path appears in this feature's `plan.creates` with the cross-cutting as source): Validation passes.
System (condition: the path does not appear in this feature's `plan.creates`): Emits `cross-cutting-target-not-in-creates` per resolved path.

#### Branch: Single-feature validation cannot see cross-pass creates

User: Runs `parlay validate --type buildfile --deep <path>` against one buildfile (no `--project` flag).
System (background): `plannedCreates` is nil; pattern resolution sees the on-disk set only. A pattern that would have resolved through a sibling create in project-pass mode now resolves to zero paths.
System: Emits `cross-cutting-pattern-empty` with a hint that mentions project-pass mode: pattern resolved against on-disk files only; if a sibling feature is expected to create matching files, run `parlay validate --project` instead.

#### Branch: Determinism — same buildfile validated twice

User: Validates the same buildfile twice in a row against an unchanged source root and unchanged sibling buildfiles.
System: Returns identical resolved sets and identical verdicts byte-for-byte across both runs. Resolution is a pure function of `(pattern, on-disk-set, plannedCreates-set)`; no shell, no adapter, no AI calls.

#### Branch: Pattern includes a glob feature beyond `filepath.Match`

User: Submits an entry with `target-pattern: "src/**/registry.go"` (recursive `**`).
System (background): `filepath.Match` does not implement `**`. Resolution treats this as a literal-character match against the path, which produces zero matches.
System: Emits `cross-cutting-pattern-empty`. (Glob-feature creep is out of scope for this feature; recursive patterns remain unsupported until a separate feature adds them.)

---

### A cross-cutting entry may declare both create-targets and modify-targets

**Trigger**: `validatePlanSection` reaches a cross-cutting entry whose schema includes the new optional `target-creates:` field.

User: Submits a cross-cutting entry with `target-files: [routes/index.go]` (existing on disk) AND `target-creates: [routes/feature.go]` (not on disk), expressing the "introduce a new route file and register it in the index" pattern as one cross-cutting concern.
System (background): The presence of `target-creates:` overrides the entry-kind heuristic from dialog 1. `target-files:` is interpreted strictly as modifies-only — every path must exist on disk and appear in `plan.modifies`. `target-creates:` is interpreted strictly as introduce-targets — every path must NOT exist on disk and appear in `plan.creates`.
System (condition: routing succeeds for both fields): Validation passes for this entry; the buildfile may proceed to codegen.
System (condition: a `target-files:` path does not appear in `plan.modifies`): Emits `cross-cutting-target-not-in-modifies` naming the path.
System (condition: a `target-creates:` path does not appear in `plan.creates`): Emits `cross-cutting-target-creates-not-in-plan` naming the path and the cross-cutting id.

#### Branch: `target-files:` path is missing on disk in a two-kinded entry

User: Submits an entry with `target-files: [routes/feature.go]` (path missing on disk) AND `target-creates: [routes/other.go]`.
System (background): Because `target-creates:` is present, the heuristic is bypassed and `target-files:` is interpreted as modifies-only. The disk-shape check `plan-modify-target-missing` fires for `routes/feature.go`.
System: Emits `plan-modify-target-missing` for `routes/feature.go` with the legacy fix message. The two-kinded shape does not silently rescue an authoring error.

#### Branch: Path listed in both `target-files:` and `target-creates:`

User: Submits an entry where `routes/feature.go` appears in both `target-files:` and `target-creates:`.
System: Emits `cross-cutting-target-double-listed` naming the offending path and both fields, with a fix message: `pick one — list <path> in target-files: if it exists on disk and is being modified, or in target-creates: if it is being introduced`.

#### Branch: Two-kinded entry with `target-pattern:`

User: Submits an entry with `target-pattern: "routes/*.go"` and a `plan.creates` row for `routes/feature.go` (the pattern matches that future create) and `plan.modifies` rows for `routes/index.go` and `routes/auth.go` (existing on disk that the pattern also matches).
System (background): Resolves the pattern once; classifies each resolved path by on-disk vs cross-pass-creates; routes on-disk matches to `plan.modifies` and future-create matches to `plan.creates`. The `cross-cutting-mixed-target-kinds` error from dialog 1 does NOT fire here because the kinds came from pattern resolution, not from a mixed `target-files:` list.
System (condition: every resolved path is correctly placed): Validation passes.
System (condition: any placement is wrong): Emits the appropriate `cross-cutting-target-not-in-creates` or `cross-cutting-target-not-in-modifies` error per path.

#### Branch: Buildfile uses neither `target-creates:` nor `target-pattern:` two-kinded shape

User: Submits a buildfile with no `target-creates:` field anywhere.
System (background): Validation behaves identically to a buildfile authored before this feature shipped. No new error codes can fire on this entry shape.
System: Validation outcome matches today's outcome byte-for-byte. No regression.

---

### `plan.modifies` is satisfied by an on-disk file OR another feature's `plan.creates` in the same pass

**Trigger**: A caller invokes the validator in project-pass mode — either `parlay validate --project [--root <root>]` from the CLI, or the in-process equivalent invoked by `parlay generate-code` when planning a project-wide pass.

User: Runs `parlay validate --project --root core` against a project where feature A has `plan.creates: [router.go]` (path not on disk) and feature B has `plan.modifies: [router.go]`.
System (background): Walks every feature's buildfile under the resolved root, collects each `plan.creates` set into a project-wide map keyed by path with `feature/<slug>` source attribution. Then, for each feature in turn, validates the feature's buildfile with the union-minus-self as `plannedCreates`.
System (background): For feature B, the `plan-modify-target-missing` rule sees `router.go` is not on disk but appears in `plannedCreates` sourced from feature A. The rule treats the path as satisfied.
System (condition: every feature's plan rows have valid attribution and no cycles exist): Validation passes for the whole project. Emits one verdict per feature, all `ok`.
System (condition: a feature has a plan row whose modify-path is satisfied through a sibling create but lacks proper `sources:` attribution on its own row): Emits the existing source-attribution error for that feature; the relaxation is on existence, not provenance.

#### Branch: Single-feature validation rejects the same configuration

User: Runs `parlay validate --type buildfile --deep core/.parlay/build/feature-b/buildfile.yaml` against feature B alone, without `--project`.
System (background): `plannedCreates` is nil. The `plan-modify-target-missing` rule applies in legacy mode — on-disk presence only.
System: Emits `plan-modify-target-missing` for `router.go` with the legacy fix message and an additional hint: if router.go is created by another feature in the same project pass, validate the project as a whole with `parlay validate --project`.

#### Branch: Two-feature create-modify cycle

User: Runs `parlay validate --project` against a project where feature A has `plan.creates: [X]` AND `plan.modifies: [Y]`; feature B has `plan.creates: [Y]` AND `plan.modifies: [X]`. Neither X nor Y exists on disk.
System (background): Builds a directed dependency graph: feature A depends on feature B (because A modifies Y which B creates); feature B depends on feature A (because B modifies X which A creates). Detects the cycle.
System: Emits `plan-create-modify-cycle` naming both features and both paths: `cycle: feature-a creates X and modifies Y; feature-b creates Y and modifies X — neither feature can be ordered before the other`. The fix message points to splitting one cross-cutting concern across two passes or merging the two features.

#### Branch: Generate-code orders create-then-modify deterministically

User: Runs `parlay generate-code --project --root core` for a project where feature A creates `router.go` and feature B modifies it.
System (background): Validates project-pass first (same rules as above). Then walks the dependency graph in topological order, emitting feature A's creates before feature B's modifies regardless of feature-slug alphabetization.
System: Emitted source order has `router.go` written by feature A, then modified by feature B. Re-running the same project produces byte-identical output. The emission order is determined by the dependency graph, not by the order features appear in the source tree or in the CLI argument list.

#### Branch: A modify-path is satisfied through a sibling create but the source attribution on this feature's plan row is wrong

User: Runs project-pass validation on a project where feature B has `plan.modifies: [router.go]` with `sources: [component/<wrong-name>]` — citing a component that does not exist in feature B.
System (background): The relaxation makes the path pass the existence check (because feature A creates `router.go`). The independent source-attribution rule still fires: every plan row's `sources:` must reference a real entry in the buildfile.
System: Emits the existing source-attribution error for feature B's plan row — separate from the existence relaxation. The two rules are orthogonal: existence is relaxed in project-pass; provenance is unchanged.

#### Branch: CLI shape decision

User: Runs `parlay validate` and wonders how to invoke project-pass mode.
System (background): The pinned CLI shape is `parlay validate --project [--root <root>]`. No buildfile path argument; the command walks every feature in the resolved root, collects each `plan.creates` set, and validates each feature with the union-minus-self as `plannedCreates`. The flag composes with `--type buildfile --deep` (which is implied — project-pass is always deep buildfile validation).
System: When `--project` is set, any positional buildfile path argument is rejected with `validate-project-takes-no-path` — the project pass discovers buildfiles by walking the root, never by an argument. When `--project` is not set, behavior is unchanged from today.

#### Branch: Project-pass mode with no buildfiles

User: Runs `parlay validate --project --root core` against a fresh project where no feature has been built yet (no `.parlay/build/<feature>/buildfile.yaml` files exist).
System (background): The walker finds zero buildfiles; `plannedCreates` is empty.
System: Emits an informational result naming the resolved root and reporting zero buildfiles. Exit status is 0 (nothing to validate is not an error).

---
