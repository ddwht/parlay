# Status Feature Phases

> Extend `parlay status` to show, per feature, the furthest pipeline phase whose required artifacts already exist on disk, with an optional machine-readable JSON output for agent consumption.

---

## Show pipeline phase per feature in status

**Goal**: Let a designer or agent see, at a glance, how far each feature has progressed through the parlay pipeline without having to manually inspect each feature folder.
**Persona**: Designer or agent operating on a parlay project.
**Priority**: P0
**Context**: Running `parlay status` (or `/parlay-status`) inside a parlay project. Today it shows the active root and a flat list of feature identifiers; the user has no way to tell which features are still in early authoring versus which already have generated code.
**Action**: Compute a phase value per feature by reusing the file-existence checks already encoded in `core/internal/commands/check_readiness.go`, and add it as a new column in the existing feature listing.
**Objects**: feature, pipeline phase, intents.md, dialogs.md, surface.md, infrastructure.md, buildfile.yaml

**Constraints**:
- Phase vocabulary, in pipeline order: `intents | dialogs | artifacts | build | done`. Exactly these five values, lowercase, no others. The `code` phase is intentionally excluded — generated code is not tracked as a per-feature pipeline state in this command.
- Phase semantics: the furthest stage whose required on-disk artifact(s) exist for that feature. `done` means all four pre-terminal phases (intents, dialogs, artifacts, build) are complete on disk. The engineering spec under `spec/handoff/<feature>/specification.md` is NOT required for `done`.
- Reuse the file-existence checks from `core/internal/commands/check_readiness.go` rather than re-implementing path resolution or stage gating. If reuse requires a small refactor (e.g. extracting a helper that returns "what exists" without exit codes), prefer that over duplication.
- The phase column appears in BOTH the human listing and the `--json` output. In the human output it sits to the right of the feature identifier in each per-root section's tabwriter; in JSON it appears as a `phase` field on each feature entry.
- The human output is hierarchical when the active root is a parent: parent root header → parent features (with phase column) → for each registered child root, a child header → that child's features (with phase column). When the active root is a child, only that child's section is printed. See the "Aggregate status across parent and child roots" intent for the cross-root semantics.
- Must not error or skip features when an upstream artifact is missing — every listed feature gets exactly one phase value, even if it is still at `intents`.

**Verify**:
- A feature with only `intents.md` reports phase `intents`.
- A feature with `intents.md` and `dialogs.md` reports phase `dialogs`.
- A feature with `intents.md`, `dialogs.md`, and either `surface.md` or `infrastructure.md` (or both) reports phase `artifacts`.
- A feature that additionally has a buildfile under `.parlay/build/<feature>/buildfile.yaml` reports phase `build`.
- A feature whose intents, dialogs, artifacts, and buildfile all exist on disk reports phase `done`.
- Running `parlay status` against a bare-parent root (no features at parent, only child roots) prints the parent header with zero features, followed by each child root's header and its feature listing with the phase column populated.
- Running `parlay status` against a parent root with both parent-owned features and registered children prints the parent's features (with phase column) first, then each child root section in registration order.

**Questions**:
- (none)

---

## Emit machine-readable status with --json

**Goal**: Let agents consume `parlay status` output programmatically — current human-formatted output is not parseable.
**Persona**: Agent or scripted tooling driving parlay from outside Claude Code.
**Priority**: P1
**Context**: An agent needs to decide which feature to advance next, or wants to surface phase information in a UI. Parsing the human output is brittle and tabwriter-formatted.
**Action**: Add a `--json` flag to `parlay status` that emits a stable JSON object covering the same information as the human output, including the per-feature phase column.
**Objects**: status output, JSON envelope, feature, phase, root, child root

**Constraints**:
- `--json` must be additive and opt-in. Default behavior of `parlay status` (human tabular output) must be byte-for-byte unchanged for existing callers, except for the addition of the new phase column to the human listing and the new per-child sections in parent-root invocations.
- The JSON envelope must include a top-level `schema_version` integer field, starting at `1`. Any breaking change to field names, semantics, or removal bumps this integer. Additive fields (new optional keys) do not bump it. This is the conservative default; if a string-tag scheme like `"parlay.status/v1"` is preferred for ecosystem reasons, raise it back to the parent session before dialogs.
- The JSON envelope is hierarchical and mirrors the human output:
  - Top-level keys: `schema_version`, `root`, `children`.
  - `root` is an object with at least `path`, `kind` (`parent` | `child` | `standalone`), `source` (how the root was resolved), and `features` (an array of feature entries belonging to the root itself).
  - `children` is an array (possibly empty) of child-root objects, each with at least `name`, `path`, and `features`. Present only when the active root is a parent; for a child or standalone active root, `children` is an empty array.
  - When a child root is unavailable (missing/unreadable), its entry still appears in `children` with `name`, `path`, an empty `features` array, and an `unavailable` string field describing the reason. The overall command still exits zero.
- Each feature entry must include at minimum `id` and `phase`. Use snake_case for multi-word JSON keys.
- The JSON output must be valid JSON on stdout only — no human-readable preamble, no trailing log lines. Errors continue to go to stderr / non-zero exit.
- `--json` must work identically across parent, child, and bare-parent root topologies. For bare-parent, `root.features` is an empty array (not null) and `children` lists each registered child.

**Verify**:
- `parlay status --json` from a parent root returns an object with `schema_version: 1`, a `root` object whose `kind` is `parent` and whose `features` array reflects parent-owned features (possibly empty), and a `children` array with one entry per registered child root, each containing its own `features` array of `{id, phase}` entries.
- `parlay status --json` from a child root returns an object whose `root.kind` is `child` and whose `children` array is empty; `root.features` lists that child's features with phase.
- `parlay status --json | jq '.children[0].features[0].phase'` returns one of `intents`, `dialogs`, `artifacts`, `build`, `done`.
- `parlay status --json | jq .schema_version` returns `1`.
- `parlay status` without `--json` produces output identical to today's command for child/standalone roots, modulo the new phase column; for parent roots it additionally appends per-child sections in registration order.
- `parlay status --json` against a parent root where one child root is unreadable still exits zero and emits that child as a `children[]` entry with an `unavailable` field set.
- `parlay status --json` exits non-zero with stderr-only diagnostics when there is no active parlay root.

**Questions**:
- (none)

---

## Reuse readiness checks instead of reimplementing them

**Goal**: Keep a single source of truth for "what counts as having advanced through phase X" so that `status` and `check-readiness` can never disagree.
**Persona**: Parlay maintainer.
**Priority**: P1
**Context**: `core/internal/commands/check_readiness.go` already encodes file-existence checks per pipeline stage (`create-surface`, `build-feature`). Re-implementing those checks inline in `status.go` would create a drift hazard.
**Action**: Extract or expose the file-existence portions of the readiness checks as a helper that returns a structured per-feature phase value, and call that helper from both `status` and `check-readiness`.
**Objects**: check_readiness.go, status.go, phase computation helper

**Constraints**:
- The shared helper lives in `core/internal/commands/` alongside `check_readiness.go`, exported as a function (e.g. `ComputeFeaturePhase`) that both `status.go` and `check_readiness.go` call. This is the conservative choice: it reuses an existing package both call sites already import, avoids creating a new package boundary for a small amount of code, and sidesteps any risk of import cycles. If the helper grows or accretes additional callers outside `commands/`, extract a `core/internal/phases/` package as a follow-up — but not pre-emptively.
- The helper's input is a (root, feature) pair — concretely, a resolved root context (carrying that root's `spec/intents/`, `spec/handoff/`, and `.parlay/build/` paths) plus a feature identifier within that root. It is NOT a global feature ID. This shape lets the same helper serve a child invocation (called once per child feature) and a parent invocation that aggregates across the parent and each registered child (called once per (root, feature) pair).
- The shared helper must be pure: (root context, feature) in, phase value (and possibly a "what exists" struct) out. No side effects, no exit codes, no JSON marshalling, no I/O beyond the file-existence checks themselves.
- The existing `check-readiness` command's external contract (CLI flags, JSON output schema, exit codes) must not change. The refactor is purely internal.
- The shared helper must not hard-code the parent vs. child root path layout — it must derive all artifact paths from the supplied root context, mirroring how `check_readiness` already accepts `featurePath` but generalized so that the build-state lookup uses THAT root's `.parlay/build/`, not a project-wide singleton.
- The phase-value type must be a typed enum (Go const block or string-typed alias), not a free-form string, so that callers and tests cannot pass invalid values.

**Verify**:
- Both `parlay status` and `parlay check-readiness` resolve the existence of `intents.md`, `dialogs.md`, `surface.md`, `infrastructure.md`, and the buildfile through a single shared code path.
- Existing `check-readiness` tests continue to pass after the refactor with no test changes.
- The phase helper has its own unit tests covering each phase transition (intents-only, +dialogs, +surface/infrastructure, +buildfile, full = done).
- A unit test exercises the helper twice with the same feature name under two distinct root contexts (e.g. `core` and `studio`) and confirms each returns the phase computed against its own root's artifact tree, never a mix.

**Questions**:
- (none)

---

## Aggregate status across parent and child roots

**Goal**: Let a designer running `parlay status` at the parent root see, in one structured listing, the phase of every feature across the parent root and all of its registered child roots — without having to cd into each child and re-run.
**Persona**: Designer working in a multi-root parlay project (e.g. this repo with `core/` and `studio/`).
**Priority**: P1
**Context**: This project has a parent root at `/workspace/parlay-dev` and child roots `core/` and `studio/`. `parlay status` already resolves the active root via `--root` or cwd walk-up. Today the active-root listing stops at the parent's own features; the user has to invoke status separately under each child to see what's happening across the project.
**Action**: When the resolved active root is a parent, walk the parent's own `spec/intents/` AND each registered child root's `spec/intents/`, and emit a hierarchical listing: parent root header → parent features (with phase) → for each child root → child header → child features (with phase). When the resolved active root is a child, behavior is unchanged: only that child's features are listed; do not traverse upward to the parent or sideways to siblings.
**Objects**: parent root, child root, active-root resolution, hierarchical feature listing

**Constraints**:
- Each feature's phase is computed against ITS OWN root — the per-(root, feature) phase value is never mixed across roots. The helper from the "Reuse readiness checks" intent must accept a (root, feature) pair, not a global feature identifier, so the same call works for parent-owned and child-owned features.
- Aggregation is one level deep only: the parent walks its directly-registered children. It does not recurse further (children of children are not in scope; the project layout doesn't currently nest deeper).
- Aggregation is downward only. Running status under a child root never pulls in the parent or sibling children — that direction is intentionally one-way to keep child invocations cheap and scoped.
- A bare-parent root (no parent features, only children) is naturally handled: the parent header prints with zero features, then each child section prints normally. No special-case branch.
- If a registered child root is missing on disk, unreadable, or otherwise broken, the parent listing must still succeed — emit the child header with a clear inline diagnostic (e.g. `(unavailable: <reason>)`) and continue with the remaining children. One bad child must not abort the whole aggregated view.
- Cross-root phase computation must respect each root's own `spec/intents/`, `spec/handoff/`, and `.parlay/build/` trees. The helper must not assume a single project-wide build directory.
- The aggregated listing preserves a stable ordering: parent first, then child roots in the order they appear in the parent's child-root registration (not alphabetized post-hoc). Within each root, features keep whatever ordering the existing per-root listing already uses.

**Verify**:
- Running `parlay status` from `/workspace/parlay-dev` (the parent root) prints the parent header, the parent's own features (with phase) — or zero parent features — followed by a `core` section with every core feature and its phase, then a `studio` section with every studio feature and its phase.
- Running `parlay status --root core` (or from inside `core/`) prints only `core` features with their phases. No `studio` features. No parent header.
- Running `parlay status --root studio` (or from inside `studio/`) prints only `studio` features with their phases. No `core` features.
- A feature that exists with the same name under both `core/` and `studio/` is listed once per root with each root's independently-computed phase value — they do not collide or get deduplicated.
- Phase values for `core/<feature>` are computed against `core/.parlay/build/<feature>/` (not the parent's `.parlay/build/`), confirming the helper is per-root.
- If `studio/spec/intents/` is deleted or made unreadable, `parlay status` from the parent still prints parent features and the `core` section, then a `studio` header followed by an inline `(unavailable: …)` diagnostic, with a zero exit code.

**Questions**:
- (none)
