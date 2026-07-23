# Domain-model-editor-validation

> Live validation for the domain-model editor: the same deep-validation rules Core's CLI enforces, run continuously against the in-memory draft and surfaced inline where the offending element is edited. Invalid states — orphan references, missing required companion keys, types outside the closed set — surface in the UI the moment they arise; the boundary is schema correctness only, no domain reasoning. The critical engineering constraint is parity: the editor must not grow a second validator that drifts from Core's, because a model the editor calls clean must never fail `parlay build-feature`. Builds on domain-model-editor-mvp (subsystem, forms, save flow); covers relationships when domain-model-editor-relationships ships.

---

## Validation parity with Core's deep validation

**Goal**: Make Core's domain-model deep validation the single source of truth for what the editor flags — one rule set, one closed error-code vocabulary, exercised identically by the CLI and the editor — so "clean in the editor" and "passes `parlay build-feature`" are the same statement.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: The domain-model schema pins a closed vocabulary of validation codes: `missing-schema-version`, `schema-version-newer-than-binary`, `schema-version-unreachable`, `field-type-outside-closed-set`, `enum-tone-outside-closed-set`, `undeclared-entity-reference`, `relationship-cardinality-unknown`, `operation-input-field-not-found`, and the authoring-mode warning `domain-operations-deprecated`. Core's CLI enforces these on its read paths. Studio depends on Core as a Go module, so the editor can invoke the same validation code rather than reimplement it — the only acceptable architecture, because a reimplementation drifts the first time the schema gains a rule (and the schema's versioning policy explicitly anticipates new primitives arriving with `schema_version` bumps). The editor's closed-vocabulary forms make several codes unrepresentable from form input (that was the MVP's point), but the validator still sees them: hand-edited files arrive through load, and cross-element states a form can't see locally (a ref whose target was deleted out from under it in a hand-edit) are exactly what deep validation exists to catch. Studio-side, the harness's `validation-failed` envelope carries `fields[]` — the natural transport, with each entry carrying the closed code and a path to the offending element.

**Action**: Expose Core's domain-model deep validation as an importable entry point (exporting it is in scope for this feature if the current function is internal to Core's CLI) taking an in-memory model and returning the full finding list — each finding carrying the closed code, an element path (e.g. `entities.Order.fields.status`, `relationships.customer-orders.to`), and the schema's actionable message. The Studio subsystem wraps it at `POST /api/domain-model/validate`, accepting a model draft and returning the findings as a `validation-failed`-shaped payload (200 with an empty list when clean — a finding list is a query result, not an error). Severity follows the schema's authoring-mode table: `domain-operations-deprecated` is a warning; everything else is an error.

**Objects**: core-deep-validation-entry-point, validation-finding, closed-error-code, element-path, validate-endpoint, severity-classification, parity-test-suite

**Constraints**:
- The editor invokes Core's validation code via the module dependency; no validation rule is reimplemented in Studio Go code or in the UI bundle's JavaScript — client-side form guards (closed dropdowns, pickers) are input affordances, not the validator, and the server-side finding list is always authoritative
- Findings carry exactly the schema's closed error codes, unchanged; the editor never invents codes, renames codes, or adds Studio-only rules to the deep-validation pass
- Every finding carries a machine-usable element path sufficient for the UI to anchor it to a specific entity, field, enum, enum value, or relationship; findings that apply to the whole model (e.g. `missing-schema-version`) use a distinguished top-level path
- Severity follows the schema's authoring-mode table: `domain-operations-deprecated` is a warning, all other codes are errors; the classification lives with the validator, not in the UI
- The validate endpoint is a pure function over the submitted draft: it reads nothing from disk, mutates nothing, and returns 200 with the finding list (empty when clean) — `validation-failed` at the HTTP-error level is reserved for malformed requests, not for a draft with findings
- A parity test suite in the studio tree runs a shared fixture corpus (one fixture per closed code, plus clean fixtures) through both Core's CLI-path validation and the editor's endpoint and asserts identical finding sets — this suite is the drift alarm and failing it blocks the build

**Questions**:
- Where should the exported entry point live in Core's package layout so both Core's CLI and Studio consume the identical code path — and does exporting it belong in a small Core-side feature (studio-support style) rather than being smuggled in through a Studio feature's implementation?

---

## Live in-editor validation surfacing

**Goal**: Surface validation findings while the designer edits — anchored inline at the offending element and aggregated in a model-wide panel — so invalid states are visible the moment they arise, not discovered at save or, worse, at build time.

**Persona**: Product designer

**Priority**: P1

**Context**: The MVP's forms make locally-invalid input unrepresentable, so live validation's real quarry is cross-element states: delete an enum via a hand-edit and load the file, and three fields now reference a ghost; a fixture migrated from an old project may carry `field-type-outside-closed-set` in places no form is currently open. That shapes the surfacing requirements: findings must be visible globally (a panel with counts), not only at controls the designer happens to have on screen, and each finding must navigate to its element. The finding messages should carry the schema's actionable phrasing — the schema documents fixes, not just failures (e.g. an inline-object rejection says to lift the nested shape into a separate entity joined by a `ref` field) — because the designer's next question after "what's wrong" is always "what do I do." Validation runs against the in-memory draft continuously; the draft never touches disk until save, so validating it is free of side effects by construction.

**Action**: The editor revalidates the draft on every committed mutation (debounced across rapid edits) via the validate endpoint. Findings render in two places: inline markers at the anchored element (form control, entity row, enum value row, relationship row, and — when the diagram ships — node/edge badges), and a validation panel listing all findings with severity, code, message, and element path, where clicking a finding navigates to and highlights its element. Errors and warnings are visually distinct; the shell shows a persistent count (e.g. in the header near the save bar) so a clean model reads as clean at a glance.

**Objects**: live-revalidation, debounced-validate-call, inline-finding-marker, validation-panel, finding-navigation, severity-styling, finding-count-indicator, actionable-fix-message

**Constraints**:
- Revalidation triggers on every committed draft mutation, debounced so rapid edits produce one trailing call; a stale response (superseded by a newer draft) is discarded, never rendered over fresher findings
- Every finding renders in the validation panel; findings whose element is representable in the current view also render an inline marker at that element — the panel is the complete list, inline markers are the subset in view
- Clicking a panel finding navigates to the owning editor surface (entity form, enum form, relationships panel, diagram) and highlights the element; whole-model findings highlight nothing but explain themselves in place
- Finding messages present the schema's actionable fix phrasing alongside the code, not the bare code alone
- Errors and warnings are visually distinct at every surfacing point (marker, panel row, count indicator), matching the validator's severity classification
- Validation state is presentation-only: it never blocks editing gestures, never mutates the draft, and is recomputed from scratch per validate response — no client-side finding cache to go stale
- A model with zero findings shows an explicit clean state at the count indicator, not merely the absence of markers

**Verify**:
- A test loads a hand-edited fixture where a field's `enum:` names a deleted enum and asserts an `undeclared-entity-reference`-class finding renders inline at the field row and in the panel with an error style
- A test performs five rapid field edits and asserts one trailing validate call fires and its findings render (debounce collapses the burst)
- A test clicks a panel finding for `relationships.customer-orders.to` and asserts the relationships panel opens with that relationship highlighted
- A test loads a fixture with a populated deprecated `operations:` block and asserts a `domain-operations-deprecated` finding renders with warning styling, visually distinct from errors in the panel and the count indicator
- A test fixes the last error in a draft and asserts the count indicator flips to the explicit clean state after the trailing revalidation
- A test races two validate responses (older arriving after newer) and asserts the stale finding set is discarded

---

## Save gating on validation findings

**Goal**: Pin what validation findings mean for the save flow: errors block the editor's save, warnings don't — so the editor never authors a file it knows would fail the build, while deprecation debt it didn't create doesn't hold edits hostage.

**Persona**: Product designer

**Priority**: P1

**Context**: The editor's whole warrant is schema correctness — an editor that writes a model it knows carries `field-type-outside-closed-set` has failed at the one thing it does. But two realities temper a naive "block on any finding." First, hand-editing is a first-class flow and files arrive invalid: a designer who loads a broken model, fixes half of it, and cannot save the improvement is worse off than with a text editor — yet allowing that save writes a file Core's build path rejects, and the schema gives no severity gradations among errors to distinguish "inherited breakage" from "new breakage." The resolution: block on errors uniformly, but make the load-time state visible immediately (the findings panel populates on load, before any edit), so a designer opening a broken file knows the repair scope up front rather than discovering a blocked save after an hour of work. Second, `domain-operations-deprecated` is authoring-mode warning severity precisely because the migration is a separate designer-paced workflow — it must never gate saves. The compare-and-swap conflict flow (409, reload-and-reapply) is orthogonal to this gate and unchanged by it.

**Action**: The save path validates the draft server-side before writing: error-severity findings fail the save with the harness's `validation-failed` envelope carrying the finding list, and nothing touches disk; warning-only findings save normally. The UI reflects the gate ahead of the attempt: while error findings exist, the save bar presents a disabled/blocked state naming the error count and linking to the validation panel, so the blocked save is never a surprise. Server-side enforcement is authoritative — a stale or bypassed client cannot write an invalid file.

**Objects**: server-side-save-gate, error-blocks-save-rule, warning-passes-save-rule, blocked-save-bar-state, validation-failed-save-envelope, load-time-finding-visibility

**Constraints**:
- The gate is enforced server-side on every save regardless of client state: a save request whose draft carries any error-severity finding is rejected with the `validation-failed` envelope listing the findings, and the on-disk file is untouched
- Warning-severity findings (`domain-operations-deprecated`) never block a save, alone or alongside the block message for other findings' errors
- The save bar reflects the gate continuously: with error findings present it shows a blocked state with the error count and a path to the validation panel, replacing — not disabling without explanation — the save affordance
- Findings for a freshly-loaded file populate before any edit, so a designer opening an already-invalid file sees the repair scope immediately, not at first save
- There is no force-save or override path through the editor; the escape hatch for deliberately writing an invalid file is the one that already exists — a text editor — and the editor's contract is not to be one
- The gate orders before the etag compare-and-swap: an invalid draft fails with `validation-failed` even when the etag is also stale (validity is designer-actionable first; conflict resolution matters only for a savable draft)

**Verify**:
- A test submits a save whose draft carries a `field-type-outside-closed-set` finding and asserts a `validation-failed` envelope with the finding listed and an unchanged on-disk file
- A test submits a save whose only finding is `domain-operations-deprecated` and asserts the save succeeds and the file is written
- A test loads a fixture with two errors and asserts the save bar is in the blocked state naming the count before any edit occurs
- A test fixes both errors and asserts the save bar transitions to the normal dirty/save state after the trailing revalidation, and the subsequent save succeeds
- A test submits an invalid draft with a stale etag and asserts the response is `validation-failed` (the gate), not `conflict` (the etag), and a follow-up with the draft fixed and the same stale etag then yields `conflict`
- A test attempts a save via direct API call (bypassing the UI) with an error-carrying draft and asserts the server-side gate rejects it identically

**Questions**:
- Is the no-force-save posture right for repair-in-progress workflows on large hand-broken models, or should a future revision allow saving when the error set is a strict subset of the load-time error set (strictly-improving saves)? Deferred until real usage shows the need

---
