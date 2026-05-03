# Status Feature Phases — Surface

---

## Per-feature phase column in human listing

**Shows**: data-table, data-value
**Actions**: (none)
**Source**: @parlay-tool/status-feature-phases/show-pipeline-phase-per-feature-in-status

**Page**: parlay-status (CLI)
**Region**: main
**Order**: 20

**Notes**:
- The existing per-root features section already renders a tabwriter table of feature identifiers. This fragment adds one new column to the right of the identifier: the pipeline phase, one of `intents | dialogs | artifacts | build | done`, lowercase.
- The column is right-padded by the existing tabwriter — no new layout primitive. Phase tokens are bare lowercase strings with no decoration (no brackets, no color codes, no progress bars).
- Every listed feature gets exactly one phase value. Never blank, never an error placeholder. The `code` phase is intentionally absent from the vocabulary.
- The column is unconditional — it appears whenever the features section is rendered, in both child-root and parent-root invocations. There is no flag to turn it off in human output.
- Existing callers see the addition as a new trailing column on each feature line. No other field shifts position. No other line changes.

---

## Hierarchical parent-and-children listing

**Shows**: data-tree, data-value, message
**Actions**: (none)
**Source**: @parlay-tool/status-feature-phases/show-pipeline-phase-per-feature-in-status, @parlay-tool/status-feature-phases/aggregate-status-across-parent-and-child-roots

**Page**: parlay-status (CLI)
**Region**: main
**Order**: 10

**Notes**:
- When the resolved active root is a parent, the human output is a hierarchical listing: parent root header → parent's own features (with phase column) → for each registered child root in registration order, a child section header (the child's name) → that child's features (with phase column).
- When the active root is a child or standalone, only that root's section is printed. No parent header is synthesized. No sibling sections are emitted. Aggregation is downward-only.
- Bare-parent case (parent has no parent-owned features, only registered children): parent header prints with `features: (none)` (or zero-count equivalent), then each child section prints normally. This is not a special-case branch — it falls out of the per-root walk.
- Same-name-different-root: a feature named `widget` appearing under both `core/` and `studio/` is listed once per root with each root's independently-computed phase. The two entries do not collide or get deduplicated.
- Ordering is stable: parent first, then children in the order they appear in the parent's `pctx.Index.Children` slice (the on-disk YAML order from `<parent>/.parlay/roots.yaml`). Status MUST NOT alphabetize. Within each root, features keep whatever ordering the existing per-root listing already uses.

---

## Unavailable child root inline diagnostic

**Shows**: message, status
**Actions**: (none)
**Source**: @parlay-tool/status-feature-phases/aggregate-status-across-parent-and-child-roots, @parlay-tool/status-feature-phases/emit-machine-readable-status-with-json

**Page**: parlay-status (CLI)
**Region**: main
**Order**: 15

**Notes**:
- When a registered child root is missing on disk, unreadable, or otherwise broken, the aggregated parent listing must NOT abort. The child's header still prints, followed by an inline diagnostic line of the form `(unavailable: <reason>)` (e.g. `(unavailable: spec/intents not readable: open /workspace/parlay-dev/studio/spec/intents: no such file or directory)`). No feature lines are emitted under that header. The remaining children continue to render in registration order.
- The command exits zero in this case — one bad child does not break the whole aggregated view. Stack traces are NOT emitted. The diagnostic is a single human-readable line.
- In `--json` output, the same condition surfaces as a `children[]` entry that includes the `name`, `path`, an empty `features` array, and an `unavailable` string field carrying the same reason text. The overall command still exits zero.

---

## Machine-readable JSON output via `--json`

**Shows**: code
**Actions**: (none)
**Source**: @parlay-tool/status-feature-phases/emit-machine-readable-status-with-json

**Page**: parlay-status (CLI)
**Region**: main
**Order**: 30

**Notes**:
- The `--json` flag is additive and opt-in. Default behavior of `parlay status` (human tabular output) is byte-for-byte unchanged for existing callers, except for the addition of the new phase column and the new per-child sections in parent-root invocations.
- When `--json` is set, a single JSON document is emitted on stdout. No human-readable preamble. No trailing log lines on stdout. All diagnostics go to stderr.
- Envelope shape (top-level keys):
  - `schema_version`: integer, starting at `1`. Bumped on breaking changes (field renames, semantic changes, removals). Additive fields (new optional keys) do NOT bump it. The value is the integer `1`, NOT a string tag like `"parlay.status/v1"`.
  - `root`: object with at least `path` (absolute string), `kind` (one of `parent`, `child`, `standalone`), `source` (how the root was resolved — e.g. `flag`, `cwd`, `env`), and `features` (an array of feature entries belonging to the root itself; `[]` if none, never `null`).
  - `children`: array of child-root objects. Present only when the active root is a parent. For a child or standalone active root, `children` is `[]` (always present, never omitted, never `null`). Each entry has at least `name` (string), `path` (absolute string), and `features` (array of feature entries, `[]` if none).
- Each feature entry includes at minimum `id` (the feature slug, e.g. `parlay-tool/status-feature-phases`) and `phase` (one of `intents`, `dialogs`, `artifacts`, `build`, `done`). Additional fields may be added later without bumping `schema_version`.
- Multi-word JSON keys use snake_case (`schema_version`, not `schemaVersion` or `SchemaVersion`).
- Unavailable child: that child's entry still appears in `children[]` with `name`, `path`, an empty `features` array, and an `unavailable` string field describing the reason. Command exits zero.
- Bare-parent case: `root.features` is `[]`, `children` lists each registered child with its own `features` array. No special-case envelope shape.
- Error path: with no active parlay root, the command prints a diagnostic on stderr (e.g. `no active parlay root`), prints nothing on stdout (no partial envelope, no empty object), and exits non-zero. The JSON contract is broken by absence, never by emitting an invalid envelope.
- Coexistence with `--ambiguity-as-signal`: when ambiguity is detected and that flag is set, the CLI exits with code 11 and emits the existing ambiguity JSON envelope on stderr (per active-root convention). Stdout stays empty. The success-path `--json` envelope is NOT emitted in that case — the two streams (stderr for ambiguity, stdout for status payload) and the two exit codes keep the schemas from conflating.
- Malformed flag (e.g. `--jsno`): cobra's default handling applies — `Error: unknown flag: --jsno` on stderr, usage printed, non-zero exit. We do NOT customize this path.

---

## Exit-code and stream discipline

**Shows**: status
**Actions**: (none)
**Source**: @parlay-tool/status-feature-phases/emit-machine-readable-status-with-json, @parlay-tool/status-feature-phases/aggregate-status-across-parent-and-child-roots

**Page**: parlay-status (CLI)
**Region**: main
**Order**: 40

**Notes**:
- Exit zero whenever the active root resolves and the listing renders, even if some children are unavailable.
- Exit non-zero only when the active root cannot be resolved (no parlay root in cwd-walk, no `--root`, no `PARLAY_ROOT`) or when ambiguity is detected with `--ambiguity-as-signal` (exit code 11, stderr JSON envelope per active-root convention).
- Stdout discipline: human output OR a single JSON document, depending on `--json`. Never both. Never a JSON document mixed with log lines.
- Stderr discipline: all diagnostics, ambiguity envelopes, and error messages. Never the success-path payload.
