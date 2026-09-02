---
amendment: planned-phase-and-content-aware-ladder
date: 2026-09-01
trigger: "Backlog support (docs/plans/backlog-support-proposal.md §3) needs a phase that distinguishes an untouched placeholder from authored work; implementing it exposed that PhaseIntents was unreachable for every feature add-feature creates."
affects:
  - "@parlay-tool/status-feature-phases/surface:per-feature-phase-column-in-human-listing"
  - "@parlay-tool/status-feature-phases/surface:machine-readable-json-output-via-json"
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
amends_intents:
  - intent: show-pipeline-phase-per-feature-in-status
    mode: extend
    version:
      title: Show pipeline phase per feature in status
      goal: Let a designer or agent see, at a glance, how far each feature has progressed through the parlay pipeline without having to manually inspect each feature folder.
      persona: Designer or agent operating on a parlay project.
      priority: P0
      context: Running `parlay status` inside a parlay project. It shows the active root and a list of feature identifiers; without a phase the user cannot tell which features are still in early authoring versus which already have generated code.
      action: Compute a phase value per feature and add it as a new column in the existing feature listing.
      objects:
        - feature
        - pipeline phase
        - intents.md
        - dialogs.md
        - surface.yaml
        - infrastructure.md
        - capabilities.yaml
        - buildfile.yaml
      constraints:
        - "Phase vocabulary, in pipeline order: `planned | intents | dialogs | artifacts | build | done`. Exactly these six values, lowercase, no others, plus the off-ladder value `hand-authored` for a declared unit, which is not a rung and does not participate in the ordering. The `code` phase is intentionally excluded."
        - "`planned` is a feature that exists but promises nothing yet: its `intents.md` parses to zero intents. Every feature is born here, because `add-feature` writes both founding documents eagerly and the scaffolded `intents.md` is commented out precisely so it parses empty."
        - "Phase semantics below the artifacts rung are CONTENT-based, not existence-based: `intents` requires at least one parsed intent, `dialogs` at least one parsed dialog. File presence cannot answer this question, because `add-feature` creates both files at feature birth."
        - "The terminal rung measures completion evidence — artifacts plus both build outputs — and checks the founding documents for structural presence only. It does not read dialog content and infers nothing from finding none; a backend feature legitimately has no dialog turns, and content validity at that altitude belongs to the build and readiness gates."
        - "A founding document that cannot be read or parsed is treated as present-and-authored, never as empty. An unreadable file is the weakest possible evidence, and must never be the reason a feature is reported as emptier than it is."
        - "The phase column appears in BOTH the human listing and the `--json` output."
        - "The human output is hierarchical when the active root is a parent: parent root header, parent features, then each registered child root's header and features."
        - "Must not error or skip features when an upstream artifact is missing — every listed feature gets exactly one phase value, even if it is still at `planned`."
      verify:
        - A feature created by `parlay add-feature` and not yet edited reports phase `planned`.
        - A feature whose `intents.md` carries at least one parsed intent, with only the scaffolded `dialogs.md`, reports phase `intents`.
        - A feature with at least one parsed intent and at least one parsed dialog reports phase `dialogs`.
        - A feature that additionally has `surface.yaml`, `capabilities.yaml` or `infrastructure.md` reports phase `artifacts`.
        - A feature that additionally has a buildfile under `.parlay/build/<feature>/buildfile.yaml` reports phase `build`.
        - A feature with artifacts, a buildfile and testcases reports `done` even when its `dialogs.md` carries zero parsed dialog turns.
        - A feature whose `intents.md` cannot be read is not reported as `planned`.
  - intent: emit-machine-readable-status-with-json
    mode: revise
    version:
      title: Emit machine-readable status with --json
      goal: Let an agent or script consume `parlay status` output structurally, without parsing a human-formatted listing.
      persona: Designer or agent operating on a parlay project.
      priority: P0
      context: Skills and CI steps need the active root, its features and their phases as data. Screen-scraping a tabwriter listing breaks on every cosmetic change.
      action: Add a `--json` flag that suppresses the human listing and writes one JSON document to stdout.
      objects:
        - status envelope
        - schema_version
        - root
        - children
        - feature entry
      constraints:
        - "The JSON envelope must include a top-level `schema_version` integer field. It is `2`. Any breaking change to field names, semantics, or removal bumps this integer. Adding a VALUE to a documented enum is such a change and bumps it; adding a new optional FIELD is not and does not."
        - "Top-level keys: `schema_version`, `root`, `children`."
        - "`--json` suppresses the human listing entirely — one document on stdout, never both, never interleaved."
        - "Each feature entry includes at minimum `id` and `phase`. Additional optional fields may be added later without bumping `schema_version`."
        - "Multi-word JSON keys use snake_case (`schema_version`, not `schemaVersion`)."
      verify:
        - "`parlay status --json | jq .schema_version` returns `2`."
        - "`parlay status --json` from a parent root returns an object with `schema_version: 2`, a `root` object whose `kind` is `parent`, and a `children` array with one entry per registered child root."
        - 'A feature created by `parlay add-feature` and not yet edited appears with `"phase": "planned"`.'

---

## Change

The phase vocabulary gains a sixth value, `planned`, and the two rungs below
`artifacts` stop asking whether a founding document EXISTS and start asking
whether it carries parsed content. The status JSON envelope's `schema_version`
moves from `1` to `2`.

Three consequences, in the order they matter:

1. `planned` is now the floor of the ladder — a feature that exists but promises
   nothing yet.
2. `intents` becomes reachable. It was not before: `add-feature` writes
   `dialogs.md` at feature birth, and the ladder asked only whether that file was
   present, so every feature reported `dialogs` from the moment it was created.
3. The terminal rung is unchanged in behaviour but explicit in intent: it
   measures completion evidence and checks founding-document presence only.

## Why

The founding constraint said "exactly these five values", and the code now emits
six. That contradiction is the reason this record exists rather than a code-only
change: the contract is what a reader trusts, and a contract that mechanically
rejects a token the tool emits is worse than one that never mentioned it.

`PhaseIntents` was unreachable. Measured on this repository before the change, of
46 features in `core`, **zero** reported `intents` — not because none were at that
stage, but because no feature made by `add-feature` could ever report it. A rung
nothing can occupy is not a ladder rung, and a brand-new empty folder claiming to
have authored dialogs is a worse answer than no answer.

The version bump is forced by the new token, not by taste. A consumer switching
exhaustively on `phase` breaks on a value it has never seen, while one ignoring an
unknown key does not — and `schema-versioning.schema.md` treats exactly this class
as a bump, recording `.code-hashes.yaml` as "currently at 2; v2 added the
`hand-authored` provenance, changing the domain of an existing field".
`PhaseHandAuthored` added a phase value without bumping this envelope, which looks
like precedent for leaving it alone; it is not, because that omission and the
`.code-hashes` bump landed in the same commit (`e159f47`), one bumping and one not,
for the same kind of change. An inconsistency inside a single commit is not a
policy to repeat.

The terminal rung keeps presence semantics for a reason found by measurement, not
argued from taste: making it content-aware demoted
`parlay-tool/structured-domain-model-validation` from `done` to `build`. That
feature is fully built and its `dialogs.md` reads "CLI/backend feature — no
interactive dialog turns." Requiring parsed turns there punishes a backend feature
for correctly declaring it has none. The rung is therefore defined as completion
evidence rather than design-document completeness, and it claims nothing about
whether an empty `dialogs.md` was deliberate — distinguishing a declared no-dialog
feature from an untouched stub would need an explicit declaration in the dialog
schema, and inference cannot substitute for one.

## Acceptance

- A feature created by `parlay add-feature` and not yet edited reports phase `planned`.
- A feature with at least one parsed intent and only the scaffolded `dialogs.md` reports `intents`.
- A feature with at least one parsed intent and at least one parsed dialog reports `dialogs`.
- A feature with artifacts, a buildfile and testcases reports `done` even when its `dialogs.md` carries zero parsed dialog turns.
- A founding document that cannot be read does not demote the feature.
- `parlay status --json | jq .schema_version` returns `2`.
- `parlay status --json` reports `"phase": "planned"` for a freshly added, unedited feature.
