# Status Feature Phases — Infrastructure

---

## Shared ComputeFeaturePhase helper and typed phase enum

**Affects**: the feature-phase computation helper shared across `parlay status` and `parlay check-readiness`, located alongside the existing readiness command in the CLI commands package.
**Behavior**: Introduce a single shared helper that returns the furthest pipeline phase whose required on-disk artifacts exist for a given (root, feature) pair. The helper is the single source of truth for "what counts as having advanced through phase X" across the parlay CLI. It is consumed by both `parlay status` (new caller, this feature) and `parlay check-readiness` (existing caller, refactored to delegate).

  **Location**: alongside `check_readiness.go` in the commands package. NOT a new `core/internal/phases/` package — both call sites already import that package, the helper is small, and a fresh package boundary creates needless churn and a risk of import cycles. If additional callers outside the commands package ever appear, extract a dedicated `phases` package as a follow-up; do not pre-emptively split.

  **Signature**: takes a (root context, feature slug) pair and returns a typed phase value. The input is never a global feature ID. The root context carries that root's `spec/intents/`, `spec/handoff/`, and `.parlay/build/` paths, so the same helper serves both child invocations (called once per child feature) and parent-aggregated invocations (called once per (root, feature) pair across the parent and each registered child).

  **Return type**: a string-typed alias with exported constants for each phase value (intents, dialogs, artifacts, build, done). Callers and tests cannot construct an invalid value through the public API. The string forms are exactly `intents`, `dialogs`, `artifacts`, `build`, `done` so that downstream JSON serialization and tabwriter rendering use the constant directly.

  **Phase semantics** (file-existence rules — these are the single source of truth):
  - intents: `<root>/spec/intents/<feature>/intents.md` exists.
  - dialogs: previous + `<root>/spec/intents/<feature>/dialogs.md` exists.
  - artifacts: previous + EITHER `<root>/spec/intents/<feature>/surface.md` OR `<root>/spec/intents/<feature>/infrastructure.md` (or both) exists. Presence of either is sufficient.
  - build: previous + `<root>/.parlay/build/<feature>/buildfile.yaml` exists.
  - done: all four pre-terminal artifacts exist. The engineering spec under `<root>/spec/handoff/<feature>/specification.md` is NOT consulted — `done` is reached at buildfile presence. Generated code is also not tracked here.

  **Purity contract**: pure function — input in, phase value out. No process exit is reachable. No stdout/stderr writes. No JSON marshalling. No logging. The only I/O is file-existence checks (stat or equivalent) on a fixed set of paths derived from the root context and feature slug. All side-effecting concerns (formatting, exit codes, JSON output) live in the call sites.

  **Per-root path derivation**: every artifact path is derived from the root context. The build-state lookup uses THAT root's `.parlay/build/<feature>/buildfile.yaml`, never a project-wide singleton. This is the critical property that makes cross-root aggregation correct: invocations against different roots consult disjoint trees and return independently-computed values that never cross-contaminate.

  **check-readiness refactor**: the existing file-existence portions of the build-feature readiness check (and any sibling stage check that duplicates path resolution) delegate to the new helper, then layer the readiness-specific validation on top (intent parsing, surface fragment validation, adapter checks, open-question collection). The external contract of `parlay check-readiness` — CLI flags, JSON output schema `{feature, stage, ready, issues[]}`, exit codes — is unchanged. Existing readiness tests pass without test modification.

**Invariants**:
- The helper is pure: invoking it twice with the same on-disk state returns the same enum value, with no observable side effects.
- The helper performs no I/O beyond stat-equivalent file-existence checks (no file reads, no network, no process exits, no logging, no marshalling).
- Every artifact path consulted by the helper is derived from the supplied root context — no project-wide singleton path is ever used for the build-state lookup.
- Invoking the helper with the same feature slug under two distinct root contexts produces two independently-computed phase values that never share state.
- The return value is always one of the five exported phase constants; no other string value is reachable through the public API.
- The external contract of `parlay check-readiness` (CLI flags, JSON output schema, exit codes) is byte-identical before and after the helper extraction.

**Source**: @parlay-tool/status-feature-phases/reuse-readiness-checks-instead-of-reimplementing-them, @parlay-tool/status-feature-phases/show-pipeline-phase-per-feature-in-status
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The helper is the dependency that makes the surface-side phase column safe to compute on every status invocation. It is also the dependency that prevents `status` and `check-readiness` from drifting on phase semantics.
- "Backward-Compatible: yes" applies to `check-readiness` — its CLI surface, JSON schema, and exit codes are unchanged. The refactor is purely internal.
- The phase-value type is a typed enum (string-typed alias + const block), NOT a free-form string. This is enforced at the API boundary so that callers and tests cannot pass invalid values.

---

## Status command call site — phase column and per-root rendering

**Affects**: the `parlay status` command's per-feature rendering loop and its flag surface, in the existing status command call site.
**Behavior**: Wire the shared phase helper into the status command so that every feature line in the human output and every feature entry in the JSON output carries a phase value. Status owns formatting and stream discipline; it MUST NOT contain any file-existence logic for phase computation — that delegates entirely to the helper.

  **Per-feature loop**: for each feature resolved from the active root's intents tree (using whichever existing helper enumerates a root's features today), call the phase helper and pass the returned value into the renderer. In the human path, the phase string is appended as a new tabwriter column to the right of the feature identifier. In the JSON path, the phase string is set as the `phase` field on the feature entry object alongside `id`.

  **`--json` flag**: register a new boolean flag `--json` on the existing `parlay status` command. Default false. When true, suppress the human tabular output entirely and emit a single JSON document on stdout (see surface.md for envelope shape). Diagnostics continue to go to stderr.

  **Stream discipline**: stdout carries either the human table OR the JSON document, never both, never interleaved with log lines. Stderr carries diagnostics, ambiguity envelopes, and error messages. Exit zero on successful render (even with unavailable children); non-zero only when the active root cannot be resolved or when `--ambiguity-as-signal` triggers (exit code 11 per active-root convention).

**Invariants**:
- Every feature line emitted by `parlay status` (in either human or JSON mode) carries exactly one phase value drawn from the typed phase enum.
- The status command performs no file-existence logic of its own for phase computation; all phase values come from the shared helper.
- When `--json` is unset, stdout contains the human tabular output only — never JSON, never interleaved log lines.
- When `--json` is set, stdout contains a single JSON document only — no human preamble, no trailing log lines.
- Diagnostics, ambiguity envelopes, and error messages are written to stderr only and never appear on stdout in either mode.
- Exit code is zero on a successful render even when one or more child roots are unavailable; non-zero exit is reserved for unresolved active root and ambiguity-as-signal triggers.

**Source**: @parlay-tool/status-feature-phases/show-pipeline-phase-per-feature-in-status, @parlay-tool/status-feature-phases/emit-machine-readable-status-with-json
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- "Backward-Compatible: yes" with one acknowledged delta: the human output gains a new trailing phase column on every feature line. No existing column shifts. The `--json` flag is opt-in.
- The JSON document shape is owned by the surface fragment "Machine-readable JSON output via `--json`"; this fragment owns only the wiring (flag registration, branching on the flag, calling the helper, marshalling the document).
- Cobra's default handling for unknown flags is sufficient — no customization.

---

## Cross-root walk — parent walks parent + registered children

**Affects**: the cross-root aggregation logic inside the `parlay status` command, scoped privately to that command (no exported API surface).
**Behavior**: When the resolved active root is a parent, the status command walks (a) the parent's own `spec/intents/` and (b) each registered child entry in on-disk YAML order, emitting one section per root. When the active root is a child or standalone, behavior is unchanged: only that root's features are rendered. The aggregation is one level deep (no recursion into grandchildren) and downward-only (child invocations never traverse upward to the parent or sideways to siblings).

  **Source of truth for child-root registration**: the parent's roots index, populated from `<parent>/.parlay/roots.yaml`. This is a slice — its on-disk YAML order IS the registration order. Status consumes the slice DIRECTLY; it MUST NOT re-read `roots.yaml` itself, MUST NOT alphabetize, MUST NOT re-sort by any other criterion. The aggregated listing preserves the slice order verbatim.

  **Per-child root context**: for each child entry, the walker resolves a freshly-instantiated root context for that child (so the phase helper consults the child's `.parlay/build/`, not the parent's `.parlay/build/`). This is the property that keeps per-root phase computation correct under aggregation.

  **One level deep**: the walker does NOT recurse into grandchildren even if a child root is itself marked as a parent in its own `roots.yaml`. The project layout doesn't currently nest deeper, and recursing would change the output shape unpredictably. If nested aggregation is ever needed, it lands as a separate intent.

  **Downward-only**: a child invocation never synthesizes a parent header, never enumerates sibling children. This keeps child invocations cheap, predictable, and scoped to themselves.

  **Bare-parent case**: when the parent's `spec/intents/` is empty or missing, the parent header still prints with `features: (none)` (or zero-count equivalent), then the per-child loop runs normally. There is no special-case branch — it falls out of the same code path.

  **Unavailable-child handling**: when a registered child root is missing on disk, unreadable (e.g. `spec/intents/` not readable due to permissions or missing directory), or otherwise broken, the walker:
  1. Catches the error at the per-child boundary (does NOT propagate up to abort the run).
  2. Emits the child header followed by an inline `(unavailable: <reason>)` diagnostic line in the human path, OR a `children[]` entry with `name`, `path`, empty `features`, and an `unavailable` string field in the JSON path.
  3. Continues to the next child in the slice.
  4. Lets the command exit zero overall.

  **Same-name-different-root**: a feature `widget` appearing under both `core/spec/intents/widget/` and `studio/spec/intents/widget/` is enumerated once per root by the walker. No deduplication. Each entry carries its own root-context-derived phase.

**Invariants**:
- When `parlay status` runs from a parent root, the output lists the parent's own features first, then each child in the order they appear in the parent's roots index slice — never alphabetized or otherwise re-sorted.
- When `parlay status` runs from a child or standalone root, only that root's features are rendered; no parent header is synthesized and no sibling children are enumerated.
- Aggregation is exactly one level deep: even if a child is itself a parent in its own roots index, grandchildren are never walked.
- An unreadable or missing child root never aborts the walk; the child appears under its own header with an `(unavailable: <reason>)` diagnostic (or `unavailable` JSON field) and the overall exit code stays zero.
- Each feature's phase value is computed against its own root's `spec/intents/`, `spec/handoff/`, and `.parlay/build/` trees — phase values never cross-contaminate between roots.
- A feature name appearing under two different roots is rendered once per root with each root's independently-computed phase value, never deduplicated.
- The cross-root walker is not exported from the status package; no external package can depend on it.

**Source**: @parlay-tool/status-feature-phases/aggregate-status-across-parent-and-child-roots
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- "Backward-Compatible: yes" applies to the per-root rendering of child and standalone roots — those paths are unchanged modulo the new phase column. The change for parent-root invocations is a strict superset: today the parent listing stops at parent-owned features; after this change it appends child sections in registration order.
- The walker is intentionally not exported. If a future caller needs cross-root aggregation outside `status`, factor it then — not pre-emptively.
- "Caching: none" — each `parlay status` invocation re-stats the on-disk artifacts. The status command is not in a hot path; correctness over micro-optimization. If performance becomes a concern under very large repos, add per-process caching as a follow-up; do not pre-emptively cache.
