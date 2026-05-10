# Multi-adapter — Surface

---

## Project Setup Preset Selection

**Shows**: message, data-list, data-value
**Actions**: select-one, invoke
**Flow**: onboarding
**Source**: @multi-adapter/bundled-adapter-set-presets

**Page**: init
**Region**: main
**Order**: 1

**Notes**:
- Triggered by `parlay init <project>`. The setup flow lists the four bundled presets plus a `custom` option.
- `data-list` enumerates the choices — `react-antd-only`, `angular-clarity-only`, `react-nest-prisma`, `angular-nest-prisma`, `custom` — each with a one-line description of the kinds it fills.
- `react-nest-prisma` is annotated as the v1 first preset (the stack exercised end-to-end in v1 CI). The annotation is informational; it does not steer the choice.
- `select-one` collects the user's pick. For a named preset, the system copies the preset's `.parlay/adapter-set.yaml` plus the corresponding adapter files into the project; for `custom`, the system skips the copy.
- `data-value` reports the files written (or "no files written — author .parlay/adapter-set.yaml from scratch" for custom).
- Follow-up `invoke` directs to `parlay add-feature` as the natural next step.

---

## Adapter Set Validation Failure

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/adapter-kinds-and-adapter-set-topology, @multi-adapter/adapter-set-links-enforce-cross-kind-boundaries

**Page**: validate
**Region**: main
**Order**: 1

**Notes**:
- Output for `parlay validate` (or any pipeline command) when `.parlay/adapter-set.yaml` or a referenced adapter file is malformed.
- `status` carries the failure type: kind unknown, adapter file missing, kind mismatch, duplicate kind, link violated, link missing, link to unfilled slot.
- `code` is the stable error code (`adapter-kind-unknown`, `adapter-set-adapter-missing`, `adapter-set-kind-mismatch`, `adapter-set-duplicate-kind`, `adapter-set-link-violated`, `adapter-set-link-missing`, `adapter-set-link-unfilled-slot`). Codes are stable across versions so editor integrations and external tools can match them.
- `message` names the offending value/path/edge and a fix hint citing the relevant artifact (adapter file, adapter-set, schema doc).
- In authoring mode link violations are warnings rather than errors so the project stays editable mid-edit.

---

## Pre-Codegen Support Gate Failure

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/adapter-supports-contract-gates-codegen-pre-ai

**Page**: build-feature
**Region**: main
**Order**: 1

**Notes**:
- Output for `parlay build-feature` when a feature's resolved capability operations use a term that is not in the relevant adapter's `supports:` block. Fails before any AI invocation.
- `status` names the missing-term kind (operation kind, step, policy, error) or the unknown-term-in-supports case.
- `code` is one of `adapter-supports-missing-operation-kind`, `adapter-supports-missing-step`, `adapter-supports-missing-policy`, `adapter-supports-missing-error`, `adapter-supports-unknown-term`, `adapter-supports-shape-mismatch`.
- `message` names the operation, the term, the term kind, and the adapter — and offers two fix paths: remove the term from this operation, or use an adapter that supports it.
- This is a "fail before generation" surface: the AI is never invoked, no source files are touched.

---

## Capabilities Validation Output

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/capabilities-yaml-replaces-infrastructure-as-the-closed-vocabulary-backend-artifact, @multi-adapter/v1-closed-vocabularies-and-v2-deferrals

**Page**: validate
**Region**: main
**Order**: 2

**Notes**:
- Output when validating `spec/intents/<feature>/capabilities.yaml`.
- `code` is one of `capabilities-unknown-term`, `capabilities-not-closed-form`, `capabilities-duplicate-operation-id`, `capabilities-stub-unfilled`, `buildfile-operation-ref-unnormalized`.
- A v2-deferred term (e.g. `kind: subscription` or `kind: job`) produces `capabilities-unknown-term` whose fix message says "deferred to v2" rather than just "not in the list" — designers know to wait rather than search for a missing term.
- Prose-only fragments produce `capabilities-not-closed-form` with line ranges. In authoring mode this is a warning; in build mode an error.
- `message` for build-mode failures names the failing field and quotes the offending content; cites the closed-vocabulary schema files for term lookups.

---

## Domain Operations Migration Prompt

**Shows**: message, data-list, data-value
**Actions**: select-one, invoke
**Flow**: configure
**Source**: @multi-adapter/domain-model-yaml-operations-field-is-deprecated-in-favor-of-capabilities, @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape

**Page**: migrate-domain-operations
**Region**: main
**Order**: 1

**Notes**:
- Output for `parlay migrate-domain-operations`. Walks each entry under `domain-model.operations[*]` and writes stubs into the appropriate feature's `capabilities.yaml`.
- For each legacy entry, `message` quotes the entry verbatim. When the target feature is unambiguous, the system writes the stub and reports it through `data-value`.
- When the target feature is ambiguous, `data-list` lists candidate features (each annotated with which entity makes it a candidate), and `select-one` collects the designer's choice.
- Stubs are written with `kind: unknown` and prose carried over verbatim under `notes:` so designer review can re-classify each one. The migrator never fabricates closed-vocabulary terms.
- After all entries are processed, the legacy `operations:` block is emptied (or removed) from `domain-model.yaml` and `data-value` reports the per-entry destinations and the cleared field.
- Final `invoke` chains to `parlay validate --type capabilities <feature>` for each feature touched.

---

## Spec Migration Report

**Shows**: summary, message, data-list
**Actions**: invoke
**Source**: @multi-adapter/surface-yaml-replaces-surface-md-as-the-closed-presentation-artifact-format, @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape

**Page**: migrate-spec
**Region**: main
**Order**: 1

**Notes**:
- Output for `parlay migrate-spec`. Walks each feature's `surface.md` and writes `surface.yaml` alongside.
- `summary` reports counts: features migrated, files written, features already migrated (idempotent no-op), features with unrouted free-text content.
- `data-list` enumerates per-feature outcomes; for features with unrouted content, the entry cites line ranges from `surface.md` that did not map to any closed-schema field.
- `message` reminds the designer that `surface.md` is left in place — deletion is the designer's call after reviewing the YAML and the unrouted-content report.
- `invoke` directs to delete `surface.md` once content is verified, or to `parlay validate --type surface` for the new YAML.

---

## Blueprint Validation Output

**Shows**: status, message, code, data-value
**Actions**: invoke
**Source**: @multi-adapter/blueprint-scope-override-precedence-and-strategy-selection

**Page**: validate
**Region**: main
**Order**: 3

**Notes**:
- Output when validating `blueprint.yaml` (or implicit blueprint defaults when no file is present).
- `code` is one of `blueprint-strategy-unsupported`, `blueprint-strategy-unknown`, `blueprint-topology-not-allowed`, `blueprint-scope-violation`, `blueprint-override-conflict`, `error-no-mapping`.
- For successful validation, `data-value` shows the resolved-value report — for each layered setting (data.fetching, data.caching, auth.strategy, errors.retry), the effective value and the source layer (blueprint, adapter-set, adapter default). This makes the precedence chain auditable.
- `error-no-mapping` is special — names the operation, the unmapped error, and the missing layer (transport or presentation) so the designer knows where to add the mapping. Fix message points at adapter-set or blueprint depending on which layer should own the mapping.
- For unsupported strategies, `message` lists the closed values the relevant adapter does support so the designer can pick a working alternative.

---

## Buildfile Normalization Review

**Shows**: diff, summary, message
**Actions**: confirm, dismiss
**Flow**: review-and-approve
**Source**: @multi-adapter/legacy-buildfile-fields-stay-deprecate-or-repurpose, @multi-adapter/multi-target-buildfile-operations-targets-and-target-aware-plan

**Page**: build-feature
**Region**: main
**Order**: 2

**Notes**:
- Output for `parlay build-feature` when a legacy buildfile is being normalized into the multi-target shape.
- `summary` lists relocations: top-level `adapter:` → `adapter-set` ref + `targets.<kind>.adapter`; top-level `components:` → `targets.presentation.components`; top-level `routes:` → presentation or transport (if disambiguated); top-level `plan.creates`/`plan.modifies` → `plan.targets.<kind>.creates/modifies`.
- `diff` shows the proposed file changes side-by-side so the designer can verify before any write.
- `message` flags ambiguous decisions explicitly — most prominently, a legacy `routes:` path colliding with a transport HTTP exposure (`buildfile-routes-ambiguous`) and any non-empty `models:` field (`buildfile-models-deprecated`).
- `confirm` writes the normalized buildfile; `dismiss` aborts and leaves the buildfile untouched, in which case the build fails with the relevant legacy-field error.
- `wiring.rules:` and `bindings:` sections are explicitly out of scope for normalization churn — the diff shows zero changes inside them.

---

## Routes Disambiguation Prompt

**Shows**: message, data-list
**Actions**: select-one
**Source**: @multi-adapter/legacy-buildfile-fields-stay-deprecate-or-repurpose

**Page**: build-feature
**Region**: dialog
**Order**: 1

**Notes**:
- Sub-flow of Buildfile Normalization Review when a legacy `routes:` path collides with a transport target's HTTP exposure on the same path.
- `message` explains the collision — both targets could legitimately own this path — and asks the designer to disambiguate.
- `data-list` lists the candidate targets (presentation for client-side routing, transport for HTTP exposure) with one-line descriptions tied to this project's adapter-set composition.
- `select-one` collects the choice; normalization writes the route under the chosen target.
- If the designer cancels (no choice made), build fails with `buildfile-routes-ambiguous` and the legacy `routes:` block remains in place.

---

## Testcases Validation Output

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/testcases-yaml-v2-discriminated-suite-kinds-and-source-refs

**Page**: validate
**Region**: main
**Order**: 4

**Notes**:
- Output when validating `testcases.yaml`.
- `code` is one of `testcases-operation-uncovered`, `testcases-source-refs-missing`, `testcases-source-refs-missing-legacy` (warning), `testcases-suite-kind-unknown`, `testcases-operation-shape-mismatch`.
- Legacy v1 suites loaded as v2 produce warnings (not errors) for missing `source_refs` — `source_refs[0]` is auto-populated from the legacy `intent` string so suites carry minimal provenance from the start.
- New v2 suites (presentation or operation) require explicit `source_refs`; build-mode failure code is `testcases-source-refs-missing`.
- For shape mismatches between operation suites and canonical operations, `message` names the suite, the field, and both shapes — the fix is to amend either the suite's assertion or the canonical operation in `capabilities.yaml`.

---

## Coverage Review Authoring

**Shows**: data-list, summary, message
**Actions**: confirm, dismiss, provide-text
**Flow**: review-and-approve
**Source**: @multi-adapter/coverage-review-yaml-gates-codegen-on-human-approval

**Page**: review-coverage
**Region**: main
**Order**: 1

**Notes**:
- Output for `parlay review-coverage <feature>`. Designer reads each suite, approves or rejects, optionally records exemptions for missing coverage.
- `data-list` enumerates suites — id, kind (presentation/operation), name, source_refs, and a brief summary of what it asserts.
- `summary` reports per-feature coverage status: how many canonical operations are declared, how many have covering suites, how many require exemptions, and the current `buildfile_hash` and `testcases_hash` that will be recorded.
- For each suite, `confirm` adds it to `approved_suites:`. `dismiss` halts review (no file written; the previous review file, if any, stays in place — codegen will fail on stale hashes until updated).
- For each missing-coverage item flagged by the system, the designer either adds a covering suite (exits to edit `testcases.yaml`, re-runs build, re-enters review) or grants an exemption — `provide-text` collects the free-text reason recorded under `exemptions:` along with the suite id and the missing item (e.g. `error:server-error`).
- On approval, the system writes `.parlay/build/<feature>/coverage-review.yaml` with current canonical-form hashes, the reviewer identity, the timestamp, and the approved-suites list.

---

## Coverage Review Gate Failure

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/coverage-review-yaml-gates-codegen-on-human-approval

**Page**: generate-code
**Region**: main
**Order**: 1

**Notes**:
- Output for `parlay generate-code` when the coverage-review gate refuses the run.
- `code` is one of `coverage-review-missing`, `coverage-review-stale`, `coverage-review-suite-unapproved`, `coverage-review-uncovered`.
- For `coverage-review-stale`, `message` names which hash drifted (`buildfile_hash` or `testcases_hash`) so the designer knows what changed.
- For `coverage-review-uncovered`, `message` names the operation, the term, and the term kind (step/error/policy) so the designer knows what to cover or exempt.
- Gate runs before any file is read for actual generation — the failure halts the run before the layered pipeline begins.
- `invoke` directs to `parlay review-coverage <feature>`.

---

## Codegen Pipeline Progress

**Shows**: progress, status, message, summary
**Flow**: monitor
**Source**: @multi-adapter/codegen-flow-ordered-layer-generation-and-fixed-read-set

**Page**: generate-code
**Region**: main
**Order**: 2

**Notes**:
- Streaming output for `parlay generate-code` after the coverage-review gate passes.
- `progress` reports layer-by-layer completion in the default order — persistence → application → transport → presentation. Each layer's start and completion is announced; presentation-only projects skip the first three layers entirely.
- `status` carries per-layer outcome — succeeded, failed, skipped.
- `message` reports files written or modified per target. On failure, names the layer, the path, and the underlying error from the adapter or the AI generator.
- Read-attempt violations (`codegen-spec-read-forbidden`, `codegen-input-out-of-scope`) abort the run with a `status: failed` message naming the offending path and the rule that rejected it.
- On success, final `summary` reports the total files emitted, hash drift versus the previous run (informational, not failure-inducing), and a pointer to the testcase suite for behavioral verification.

---

## Capabilities Migration Operations Extraction

**Shows**: summary, status, message, data-list
**Actions**: invoke
**Source**: @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape

**Page**: migrate-capabilities
**Region**: main
**Order**: 1

**Notes**:
- First-pass output of `parlay migrate-capabilities` for operation-shaped fragments — paragraphs in legacy `infrastructure.md` that match the operation pattern (input, steps, output).
- `summary` reports the count of operations extracted per feature and written into `capabilities.yaml`.
- `data-list` enumerates per-feature: the operation IDs created, their `kind` (or `kind: unknown` when the extractor could not infer kind), and the source line ranges they were extracted from.
- For ambiguous kind cases, `message` flags the operation and prompts the designer to set the kind manually before build mode rejects it.
- The pattern-shaped residue is handled separately by the Pattern Fragment Migration Report fragment.
- `invoke` directs to `capabilities.yaml` for review and to `parlay validate --type capabilities <feature>`.

---

## Pattern Fragment Migration Report

**Shows**: data-list, code, message
**Actions**: invoke
**Source**: @multi-adapter/pattern-fragment-decomposition-during-capabilities-migration

**Page**: migrate-capabilities
**Region**: main
**Order**: 2

**Notes**:
- Per-feature report from `parlay migrate-capabilities` for fragments the operation-shaped extractor did NOT consume — engineering-pattern paragraphs (registries, pipelines, dispatchers, resolvers, validators, aspects, caches, migrators, hooks, helpers).
- `data-list` enumerates each fragment by source line range, detected shape, and suggested destination drawn from the closed list.
- `code` blocks quote each fragment verbatim from `infrastructure.md` so the designer sees what is being routed without opening the source file.
- For ambiguous fragments, the entry says "unrouted; designer review" with no suggested destination — the classifier does not guess.
- For v2-deferred fragments (subscription-shaped, hook-shaped that maps to subscription), the entry preserves the fragment verbatim and tags it "v2-deferred".
- `message` reminds the designer that the migrator only writes the report — `capabilities.yaml`, `domain-model.yaml`, and `blueprint.yaml` are unchanged. Designer routes each fragment manually.
- `invoke` chains to the relevant artifact for each fragment's suggested destination.

---

## Config Migration Result

**Shows**: summary, status, message
**Actions**: invoke
**Source**: @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape

**Page**: migrate-config
**Region**: main
**Order**: 1

**Notes**:
- Output for `parlay migrate-config`. Converts the legacy `prototype-framework: <value>` field into a single-target presentation adapter-set.
- `summary` reports the conversion: the legacy field detected, the adapter-set written, and the adapter file referenced.
- For projects with no legacy field, `status: no-op` and the message says "no legacy fields detected; nothing to migrate" — keeps scripted callers from crashing.
- The legacy `prototype-framework` field stays parseable in v1 with a `prototype-framework-deprecated` warning; outright removal is owned by a separate deprecation feature scheduled for a later version.
- `invoke` directs to the next migration step (`parlay migrate-spec` or `parlay migrate-capabilities`).

---

## Domain-Model Validation Output

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/domain-model-yaml-operations-field-is-deprecated-in-favor-of-capabilities, @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape

**Page**: validate
**Region**: main
**Order**: 5

**Notes**:
- Output when validating `domain-model.yaml`.
- Covers `domain-operations-deprecated` (warning in authoring mode, error in build mode) for any project whose `domain-model.yaml` populates the legacy `operations:` field. The fix message names `capabilities.yaml` as the canonical home and points at `parlay migrate-domain-operations` for a one-time scaffolded migration.
- Also covers general domain-model schema validation — malformed entity declarations, missing required fields on relationships, unknown enum values — through the existing domain-model schema rules.
- For projects whose `domain-model.yaml` includes `kind: unknown` capability stubs (post-migration but pre-authoring), build mode emits `capabilities-stub-unfilled` directing the designer to fill in the operation kind before codegen can run.
- `invoke` directs to `domain-model.yaml` for direct edits, to `parlay migrate-domain-operations` for legacy lift, or to the relevant feature's `capabilities.yaml` for stub completion.

---

## Surface Validation Output

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/surface-yaml-replaces-surface-md-as-the-closed-presentation-artifact-format

**Page**: validate
**Region**: main
**Order**: 6

**Notes**:
- Output when validating a feature's surface artifact — `surface.yaml` (preferred) or `surface.md` (legacy migration input).
- Covers `surface-md-superseded` (warning) when both files coexist, naming both paths and recommending deletion of the legacy markdown after the designer confirms the YAML carries equivalent content.
- Covers general surface closed-schema validation — unknown Show, Action, or Flow values; fragments missing required `Shows:` or `Source:`; duplicate fragment names within a feature — through the existing surface schema rules. The rules apply identically to both serializations because both forms parse to the same in-memory model.
- For legacy `surface.md` files that have not been migrated, the validation output includes an informational `surface-md-legacy-format` note pointing at `parlay migrate-spec` — informational, not blocking, since v1 accepts both forms.
- `invoke` directs to the surface file for direct edits or to `parlay migrate-spec` for the markdown-to-YAML conversion.

---

## Buildfile Canonical Validation Output

**Shows**: status, message, code
**Actions**: invoke
**Source**: @multi-adapter/multi-target-buildfile-operations-targets-and-target-aware-plan

**Page**: validate
**Region**: main
**Order**: 7

**Notes**:
- Output when validating `.parlay/build/<feature>/buildfile.yaml` against the multi-target canonical shape. Distinct from the Buildfile Normalization Review fragment, which handles the legacy-shape migration path; this fragment handles canonical-shape validation after migration (or for greenfield buildfiles).
- `code` is one of:
  - `buildfile-target-restates-canonical` — a target section restates a canonical field (kind, subject, input, output, errors, policies, steps) that belongs only under `operations:`. Names the target, the operation, and the offending field. Fix: delete the duplicate from the target section.
  - `buildfile-binding-operation-missing` — a `bindings:` rule references an operation absent from `operations:`. Names the binding and the missing ref. Fix: correct the ref or add the operation to `capabilities.yaml`.
  - `buildfile-target-operation-missing` — a target's `effect.operation` (or other op-ref) references an absent operation. Names the target, the component, and the missing ref.
  - `buildfile-components-double-declared` — both top-level `components:` and `targets.presentation.components:` populated simultaneously. Fix: keep the target-scoped form, remove the legacy top-level form.
- All four codes are build-mode errors; authoring mode surfaces them as warnings so the buildfile remains editable.
- `invoke` directs to `buildfile.yaml` for direct edits, to `capabilities.yaml` for op-ref additions, or to `parlay build-feature --regenerate` to rebuild from spec.

---

## Adapter Kind Field Opt-In Prompt

**Shows**: message, data-list, data-value
**Actions**: confirm, dismiss, select-many
**Flow**: configure
**Source**: @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape, @multi-adapter/adapter-kinds-and-adapter-set-topology

**Page**: upgrade
**Region**: main
**Order**: 1

**Notes**:
- Triggered during `parlay upgrade` when one or more adapter files in `.parlay/adapters/` lack an explicit `kind:` field. Detection is mechanical — the upgrade walker scans each adapter file and counts missing `kind:` declarations.
- `message` explains the situation: these adapter files predate the multi-adapter feature; the validator already treats them as `kind: presentation` at parse time; writing the explicit field is purely a clarity improvement, not a correctness fix.
- `data-list` enumerates the affected adapter files with the inferred default (`kind: presentation`) for each.
- `select-many` lets the designer pick which files to update — all, none, or a subset. `confirm` writes the explicit field to the chosen files; `dismiss` skips this step entirely.
- The prompt is opt-in: the legacy default continues to apply at parse time regardless of whether the explicit field is written. Designers who skip the prompt do not break anything; the warning resurfaces on the next `parlay upgrade` run.
- After confirmation, `data-value` reports the files updated and the literal change (`kind: presentation` added at top level).
- This prompt does not run for adapter files that already declare `kind:` explicitly — including those declaring non-presentation kinds — and does not run on `.parlay/adapter-set.yaml` itself, which has its own validation surface.

---
