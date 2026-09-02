---
amendment: terminal-rung-requires-both-founding-documents
date: 2026-09-01
trigger: "Review of 001 found its terminal-rung constraint said 'the founding documents' while the implementation checked only dialogs.md, so a feature with no intents.md could report done."
affects:
  - "@parlay-tool/status-feature-phases/surface:per-feature-phase-column-in-human-listing"
  - "@parlay-tool/status-feature-phases/surface:machine-readable-json-output-via-json"
supersedes:
  - planned-phase-and-content-aware-ladder
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
        - "The terminal rung measures completion evidence — artifacts plus both build outputs — and additionally requires BOTH founding documents to be present. That is a structural-integrity check, not a content one: a feature with no `intents.md` promises nothing, so there is nothing for the build outputs to be the completion of. It does not read dialog content and infers nothing from finding none; a backend feature legitimately has no dialog turns, and content validity at that altitude belongs to the build and readiness gates."
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
        - A feature with both founding documents present, artifacts, a buildfile and testcases reports `done` even when its `dialogs.md` carries zero parsed dialog turns.
        - A feature missing `intents.md` never reports `done`, whatever else exists.
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

Narrows one constraint carried by 001. The terminal rung requires BOTH
founding documents to be present, not `dialogs.md` alone.

Nothing else in 001 changes. This record restates both amended lineages in
full because `version:` is a snapshot rather than a patch, and a reader must
not have to reconstruct the JSON-envelope revision out of a superseded file
to learn whether it still stands.

## Why

001's own constraint said the terminal rung "checks the founding documents
for structural presence" — plural — while the implementation it described
checked only `dialogs.md`. A feature carrying dialogs, an artifact and both
build outputs but no `intents.md` therefore reported `done`.

Status enumeration hides this in practice, because features are discovered
through their `intents.md` in the first place. That is exactly why it is
worth pinning: an invariant that holds only because of the caller's
discovery order is not an invariant, and `ComputeFeaturePhase` is callable
directly.

This is a separate record rather than an edit to 001 because the ledger is
append-only and defines no draft state. 001 had been validated and reviewed
under its own name before the mismatch was found, and a rule that bends when
the record is young is not the integrity boundary the project relies on
elsewhere. The cost — two records for one evolving change — is the cost the
policy deliberately chooses.

## Acceptance

- A feature missing `intents.md` never reports `done`, whatever else exists.
- A feature with both founding documents present, artifacts, a buildfile and testcases reports `done` even when its `dialogs.md` carries zero parsed dialog turns.
