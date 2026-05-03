# Status-feature-phases — Dialogs

---

### Show pipeline phase per feature in status

**Trigger**: Designer or agent runs `parlay status` (or `/parlay-status`) inside a parlay project to see how far each feature has progressed through the pipeline.

#### Scene: typical multi-feature parent root with mixed phases

User: `parlay status` (cwd is the parent root `/workspace/parlay-dev`, which has zero parent-owned features and two registered children `core` and `studio`)
System:
```
root:     /workspace/parlay-dev
kind:     parent
source:   cwd
features: (none)
child roots:
  - core    (core/)    — Parlay core engine
  - studio  (studio/)  — Parlay studio UI

core
features: 21
  - parlay-tool/status-feature-phases   dialogs
  - parlay-tool/parlay-loop             done
  - parlay-tool/artifact-generation     build
  - studio-support/page-layout-field    artifacts
  - studio-support/studio-cli-hooks     intents
  ...

studio
features: 7
  - shell                               artifacts
  - command-palette                     dialogs
  ...
```
Note: phase column is right-padded by the existing tabwriter, sits to the right of the feature identifier, and is one of `intents | dialogs | artifacts | build | done`.

#### Branch: feature with only intents.md on disk

User: `parlay status` against a root containing one feature `foo/bar` with `spec/intents/foo/bar/intents.md` and nothing else.
System: lists `foo/bar   intents`. The phase value is `intents`, never blank, never an error.

#### Branch: feature with intents + dialogs

User: as above, but `dialogs.md` also exists.
System: lists `foo/bar   dialogs`.

#### Branch: feature with intents + dialogs + artifacts (surface only, infrastructure only, or both)

User: as above, but additionally either `surface.md`, `infrastructure.md`, or both exist.
System: lists `foo/bar   artifacts`. Presence of EITHER artifact is sufficient — the helper does not require both.

#### Branch: feature with all four pre-terminal artifacts plus a buildfile — done

User: `parlay status` on a feature that has `intents.md`, `dialogs.md`, an artifact (`surface.md` and/or `infrastructure.md`), AND `.parlay/build/<feature>/buildfile.yaml`.
System: lists `<feature>   done`. The engineering spec under `spec/handoff/<feature>/specification.md` is NOT consulted — `done` is reached at buildfile presence.

#### Branch: bare-parent invocation (parent has no features, only children)

User: `parlay status` from the parent root, where the parent's `spec/intents/` is empty or missing.
System:
```
root:     /workspace/parlay-dev
kind:     parent
source:   cwd
features: (none)
child roots:
  - core    (core/)
  - studio  (studio/)

core
features: N
  - ...   <phase>
  ...

studio
features: M
  - ...   <phase>
  ...
```
The parent header prints with zero features (no error, no warning), then each child section prints normally. This is not a special-case branch in the code — it falls out of the per-root walk.

#### Branch: child-of-parent invocation

User: `parlay status --root core` (or `cd core && parlay status`).
System: prints only the `core` root header and its features with phase column. No parent header. No `studio` section. Aggregation is downward-only — child invocations never traverse upward or sideways.

#### Branch: parent root with both parent-owned features AND children

User: `parlay status` from a parent that has its own `spec/intents/` populated AND has registered children.
System: prints parent header, then parent's own features with phase column, then each child section in registration order (the order they appear in the parent's child-root registration — see "Aggregate status" dialog for the source of truth).

---

### Emit machine-readable status with --json

**Trigger**: An agent or scripted tool needs to consume `parlay status` programmatically and runs `parlay status --json`.

#### Scene: default (no --json flag) invocation — backward compatibility

User: `parlay status` (no flags) against a child root with two features.
System: emits the existing human tabular output, byte-for-byte unchanged from today, EXCEPT for the new phase column appended to each feature line. No JSON. No preamble. No log lines on stdout.

#### Scene: --json from a parent root with two children

User: `parlay status --json` from `/workspace/parlay-dev`.
System: emits a single JSON document on stdout, no other output:
```json
{
  "schema_version": 1,
  "root": {
    "path": "/workspace/parlay-dev",
    "kind": "parent",
    "source": "cwd",
    "features": []
  },
  "children": [
    {
      "name": "core",
      "path": "/workspace/parlay-dev/core",
      "features": [
        {"id": "parlay-tool/status-feature-phases", "phase": "dialogs"},
        {"id": "parlay-tool/parlay-loop",           "phase": "done"}
      ]
    },
    {
      "name": "studio",
      "path": "/workspace/parlay-dev/studio",
      "features": [
        {"id": "shell", "phase": "artifacts"}
      ]
    }
  ]
}
```
Constraint pin (schema_version style): the value is the **integer** `1`, NOT the string `"parlay.status/v1"`. We commit to numeric `schema_version` for this command and cross-command parlay JSON envelopes generally. Bumped on breaking changes only; additive fields do not bump.

Constraint pin (key casing): multi-word JSON keys use snake_case (`schema_version`, not `schemaVersion` or `SchemaVersion`).

#### Branch: --json from a child root

User: `parlay status --json --root core`.
System:
```json
{
  "schema_version": 1,
  "root": {
    "path": "/workspace/parlay-dev/core",
    "kind": "child",
    "source": "flag",
    "features": [
      {"id": "parlay-tool/status-feature-phases", "phase": "dialogs"}
    ]
  },
  "children": []
}
```
`children` is always present and is `[]` (not `null`, not omitted) for child or standalone active roots — agents can `.children[]` over it without a null check.

#### Branch: --json from a bare-parent root

User: `parlay status --json` against a parent with no parent-owned features.
System: `root.features` is `[]` (not null), `children` lists each registered child with its own `features` array. No special-case envelope shape.

#### Branch: --json with one unavailable child root

User: `parlay status --json` against a parent where `studio/spec/intents/` was deleted (or the directory is unreadable) but `studio` is still registered in the parent's roots index.
System: command exits zero. `children[]` includes the studio entry with an `unavailable` field describing the reason; its `features` array is empty:
```json
{
  "schema_version": 1,
  "root": { "kind": "parent", "features": [], ... },
  "children": [
    { "name": "core",   "path": "...", "features": [ ... ] },
    { "name": "studio", "path": "/workspace/parlay-dev/studio",
      "features": [], "unavailable": "spec/intents not readable: open /workspace/parlay-dev/studio/spec/intents: no such file or directory" }
  ]
}
```

#### Branch: malformed flag (cobra default)

User: `parlay status --jsno` (typo).
System: cobra prints its default `Error: unknown flag: --jsno` to stderr, prints command usage, exits non-zero. We do not customize this — cobra's default is sufficient.

#### Branch: --json with no active parlay root

User: `parlay status --json` from a directory that is neither a parlay root nor inside one, with no `--root` and no `PARLAY_ROOT`.
System: prints `no active parlay root` (or equivalent diagnostic) on stderr, prints nothing on stdout, exits non-zero. JSON output is stdout-only; errors stay stderr-only and break the JSON contract by absence rather than by emitting a partial envelope.

#### Branch: future interaction with --ambiguity-as-signal

User: `parlay status --json --ambiguity-as-signal` from a directory that resolves to multiple candidate roots.
System: when ambiguity is detected and `--ambiguity-as-signal` is set, the CLI exits with code 11 and emits the existing ambiguity JSON envelope on stderr (per the active-root convention). Stdout stays empty. The success-path `--json` envelope above is NOT emitted in this case — it would conflate two distinct schemas. The two flags coexist cleanly because they target different streams (stderr for ambiguity, stdout for the status payload) and different exit codes.

---

### Reuse readiness checks instead of reimplementing them

**Trigger**: Maintainer adds the phase column to status and notices that `check_readiness.go` already encodes the same file-existence checks. Rather than duplicate the logic, extract a shared helper.

#### Scene: helper signature and contract

System (post-refactor): `core/internal/commands/` exposes a function approximately:
```go
// FeaturePhase is the typed enum for pipeline-phase values.
type FeaturePhase string

const (
    PhaseIntents   FeaturePhase = "intents"
    PhaseDialogs   FeaturePhase = "dialogs"
    PhaseArtifacts FeaturePhase = "artifacts"
    PhaseBuild     FeaturePhase = "build"
    PhaseDone      FeaturePhase = "done"
)

// ComputeFeaturePhase returns the furthest pipeline phase whose required
// on-disk artifacts exist for the given feature under the given root
// context. Pure: no side effects, no I/O beyond stat calls, no exit
// codes, no marshalling.
func ComputeFeaturePhase(rootCtx *config.Context, featureSlug string) FeaturePhase
```
Constraint pins (from intents):
- Lives in `core/internal/commands/` alongside `check_readiness.go` (NOT a new `core/internal/phases/` package — extract later if call sites multiply).
- Input is `(rootCtx, featureSlug)`, NOT a global feature ID — so the same helper serves both child invocations and parent-aggregated invocations.
- Returns the typed `FeaturePhase` enum, NOT a free-form string. Callers and tests cannot construct an invalid value.
- All artifact paths derive from `rootCtx` — including `.parlay/build/<feature>/buildfile.yaml` (the build-state lookup uses THAT root's `.parlay/build/`, never a project-wide singleton).

#### Branch: call site #1 — status.go

User: `parlay status` for a child root with three features.
System: `runStatus` resolves features from `rootCtx.IntentsRoot()`, then for each feature calls `ComputeFeaturePhase(rootCtx, slug)` and renders the returned phase in the tabwriter column. No file-existence logic in `status.go` itself — it delegates entirely to the helper.

#### Branch: call site #2 — check_readiness.go

User: `parlay check-readiness @feature --stage build-feature` for a feature missing its buildfile.
System: `checkBuildFeatureReadiness` uses the same `ComputeFeaturePhase` helper to determine what exists on disk, then layers the existing readiness-specific validation (intent parsing, surface fragment validation, adapter checks, open-question collection) on top. The external contract of `check-readiness` (CLI flags, JSON output schema `{feature, stage, ready, issues[]}`, exit codes) is unchanged. Existing `check_readiness_test.go` cases pass without test modification.

#### Branch: same feature name under two distinct roots

User: a unit test calls `ComputeFeaturePhase(coreCtx, "shared/widget")` then `ComputeFeaturePhase(studioCtx, "shared/widget")`, where `core/spec/intents/shared/widget/` has only intents.md and `studio/spec/intents/shared/widget/` has the full pipeline through buildfile.
System: returns `PhaseIntents` for the first call, `PhaseDone` for the second. The two contexts never cross-contaminate. Each phase is computed against its own root's `spec/intents/`, `spec/handoff/`, and `.parlay/build/` trees.

#### Branch: helper is pure — no exits, no JSON, no logs

User: a test imports `commands.ComputeFeaturePhase` and calls it 1000 times in a loop.
System: no `os.Exit` is reachable, no `fmt.Println` fires, no JSON marshalling happens. The helper does only `os.Stat` calls (or equivalent) on a fixed set of paths derived from `rootCtx` and `featureSlug`, and returns the enum. All side-effecting concerns (formatting, exit codes) live in the call sites.

#### Branch: phase-transition unit tests

User: a test fixture creates a temporary root, populates artifacts incrementally, and asserts the helper returns each successive phase.
System:
- only `intents.md` → `PhaseIntents`
- + `dialogs.md` → `PhaseDialogs`
- + `surface.md` (or `infrastructure.md`, or both) → `PhaseArtifacts`
- + `.parlay/build/<feature>/buildfile.yaml` → `PhaseBuild`
- all four → `PhaseDone`
Each transition is a separate test case. The `code` phase is intentionally NOT exercised — generated code is not tracked here.

---

### Aggregate status across parent and child roots

**Trigger**: Designer at the parent root of a multi-root project (e.g. `/workspace/parlay-dev` with `core/` and `studio/` as children) wants one structured listing of every feature's phase across the whole project, without cd-ing into each child.

#### Scene: parent invocation showing parent + N children

User: `parlay status` from `/workspace/parlay-dev`.
System: walks (a) the parent's own `spec/intents/` and (b) each entry in `pctx.Index.Children` in order, emitting per-root sections.

Constraint pin (child-root registration source of truth — verified against existing code at `core/internal/config/roots_index.go`): the registered children come from `<parent>/.parlay/roots.yaml`, loaded via `config.LoadRootsIndex`, exposed on the runtime as `pctx.Index.Children`. This is a slice, NOT a map — its on-disk YAML order IS the registration order, and the aggregated listing preserves it without re-sorting. Status MUST consume `pctx.Index.Children` directly; it must not re-read `roots.yaml` itself, and it must not alphabetize.

For each child, the per-root walk uses a freshly-resolved `rootCtx` for that child (so `ComputeFeaturePhase` consults `core/.parlay/build/`, not the parent's `.parlay/build/`).

#### Branch: child invocation — downward-only

User: `parlay status --root studio` (or `cd studio && parlay status`).
System: prints only the `studio` section. NO parent header is synthesized. NO sibling `core` section is emitted. The aggregation is one-way (parent → children); child invocations are scoped to themselves. This keeps child invocations cheap and predictable.

#### Branch: unavailable child — diagnostic, not abort

User: `parlay status` from the parent root, but `studio/spec/intents/` was deleted (or `studio/` itself is missing, or its `spec/intents/` is unreadable due to permissions).
System (human output):
```
root:     /workspace/parlay-dev
kind:     parent
features: (none)

core
features: 21
  - ...   <phase>
  ...

studio
(unavailable: spec/intents not readable: open /workspace/parlay-dev/studio/spec/intents: no such file or directory)
```
The `core` section emits in full BEFORE the failing `studio` is encountered — a bad child does not abort the run. The command exits zero. The diagnostic is inline under the child's own header so the user sees which child failed and why. No stack trace. No partial feature listing for the failing child.

#### Branch: aggregation is one level deep

User: `parlay status` from a parent root whose children are themselves marked as parents in their own `roots.yaml` (a hypothetical nested layout — not currently used in this project).
System: walks the parent's directly-registered children only. Does NOT recurse into grandchildren. This is intentional — the project layout doesn't currently nest deeper, and recursing would change the output shape unpredictably for users on flat layouts. If nested aggregation is ever needed, it lands as a separate intent.

#### Branch: same feature name under both parent and a child (or two children)

User: a feature `widget` exists at the parent's `spec/intents/widget/` AND at `core/spec/intents/widget/`.
System: the parent section lists `widget   <parent-phase>` and the `core` section lists `widget   <core-phase>`. They are listed once per root, with each root's independently-computed phase value, and they do NOT collide or get deduplicated. The same applies to a feature appearing in two children — each child section lists it under its own phase.

#### Branch: stable ordering — registration order, not alphabetical

User: parent's `roots.yaml` lists children in the order `[studio, core]` (deliberately non-alphabetical).
System: the aggregated listing emits the `studio` section before `core`. Within each section, features keep whatever order the existing per-root listing already uses (currently directory-walk order from `config.ScanFeatureTree`). Status does NOT re-sort either dimension.

#### Branch: phase computation is per-root — no project-wide singletons

User: a unit test creates two roots, each with a `.parlay/build/widget/buildfile.yaml`, but only one of them has the corresponding `spec/intents/widget/intents.md`.
System: `ComputeFeaturePhase(rootA, "widget")` returns `PhaseDone` (intents + dialogs + artifacts + build all under rootA). `ComputeFeaturePhase(rootB, "widget")` is never called because `widget` doesn't appear in rootB's `spec/intents/`. The buildfile under rootB does NOT leak into rootA's listing — each root's `.parlay/build/` is consulted only for THAT root's features.

---
