# Multi-adapter — Infrastructure

---

## Adapter Kind Discriminator and Adapter Set Topology

**Affects**: adapter file schema, adapter-set schema, project topology validator
**Behavior**: Add a closed-vocabulary `kind:` field to the adapter file schema with the closed set `{presentation, transport, application, persistence}`, treating an absent field as the legacy `presentation` default. Define `.parlay/adapter-set.yaml` with required keys `name` and `targets` — where `targets` is a map keyed by kind, each entry being `{adapter, root}` — plus an optional `links` block (handled in the next fragment). Validate that adapter-set entries reference adapter files that exist on disk under `.parlay/adapters/`, that each entry's slot kind matches the referenced adapter's declared kind, and that no two entries share a kind.
**Invariants**:
- An adapter file with no `kind:` is treated as `kind: presentation` and validates clean
- An adapter file with a `kind:` value outside the closed set fails validation with `adapter-kind-unknown`
- An adapter-set listing two adapters under the same kind fails with `adapter-set-duplicate-kind`
- An adapter-set entry whose `targets.<kind>.adapter` does not resolve under `.parlay/adapters/` fails with `adapter-set-adapter-missing`
- An adapter-set entry whose declared slot kind contradicts the referenced adapter's `kind:` fails with `adapter-set-kind-mismatch`
- An adapter-set with only the presentation slot filled validates clean and imposes no rules on absent slots
**Source**: @multi-adapter/adapter-kinds-and-adapter-set-topology
**Backward-Compatible**: yes

**Notes**:
- The kind vocabulary is owned by Parlay and lives in `.parlay/schemas/adapter.schema.md`; extending the closed set is a schema change in a later version
- v1 caps each kind at one adapter; multi-target-per-kind is deferred to a later version with explicit schema bump
- The legacy default (missing `kind:` → `presentation`) is applied at parse time, not by file rewrite — `parlay upgrade` later offers to write the explicit field on opt-in

---

## Adapter Set Link Enforcement

**Affects**: adapter-set schema, buildfile validator (cross-kind edge walking)
**Behavior**: Define a closed link-relation vocabulary `{calls, dispatches, persists}`. Walk every cross-kind reference recorded in the buildfile's `targets:` block (presentation effect.operation calls into transport, transport handlers dispatch to application, application steps persist through persistence) and reject any edge whose `(from-kind, to-kind)` pair is not present in `.parlay/adapter-set.yaml`'s `links:` with a permitting relation. Authoring mode emits warnings rather than errors so partially-authored adapter-sets remain editable; build mode fails. Edges referring to unfilled slots fail earlier as unresolved targets.
**Invariants**:
- A cross-kind edge whose `(from-kind, to-kind)` pair is absent from `links:` fails with `adapter-set-link-violated` naming the source slot, the target slot, and the missing relation
- An adapter-set with no `links:` block rejects any cross-kind edge with `adapter-set-link-missing`
- A `links:` entry whose `from` or `to` kind is not declared in `targets:` fails with `adapter-set-link-unfilled-slot`
- A presentation-only adapter-set with no cross-kind edges in the buildfile validates clean regardless of `links:` presence
- Link enforcement is build-mode strict, authoring-mode warning
**Source**: @multi-adapter/adapter-set-links-enforce-cross-kind-boundaries
**Backward-Compatible**: yes

**Notes**:
- The link-relation vocabulary is owned by Parlay; adapter authors cannot extend it
- Link enforcement runs only on observed edges, not theoretical ones — an adapter that hosts no actual references does not need a link to it

---

## Adapter Supports Contract

**Affects**: adapter file schema (non-presentation kinds), build-feature term-walking validator
**Behavior**: Add a `supports:` block to the adapter file schema for non-presentation kinds, with sub-keys `operation_kinds`, `steps`, `policies`, and `errors` — each a list drawn from the corresponding closed vocabulary. During build-feature, for every operation declared in the resolved `capabilities.yaml`, walk the operation's kind, every step type, every policy, and every error against the `supports:` of the adapter occupying the relevant slot. Fail the build before any AI invocation when a feature requires a term the adapter does not declare. Pattern descriptions live alongside `supports:` in the adapter file but are AI prompt material, not validator input.
**Invariants**:
- An adapter declaring a `supports.*` term that is not in the corresponding closed vocabulary fails with `adapter-supports-unknown-term`
- A presentation adapter declaring a non-presentation `supports:` shape (e.g. `supports.steps`) fails with `adapter-supports-shape-mismatch`
- A feature whose resolved operation uses an unsupported operation kind, step, policy, or error fails build with one of `adapter-supports-missing-operation-kind`, `adapter-supports-missing-step`, `adapter-supports-missing-policy`, `adapter-supports-missing-error`
- Validation runs in build-feature, before generate-code is invoked — failures here block both build and codegen
- An unused pattern description (term not used by any operation) causes no validation activity
**Source**: @multi-adapter/adapter-supports-contract-gates-codegen-pre-ai
**Backward-Compatible**: yes

**Notes**:
- Pattern descriptions feed the AI prompt at codegen time only when their term is used
- The "fail before generation" property is the value proposition: a missing term should never reach the AI
- Authoring guidelines for pattern descriptions belong in a separate adapter-author guide; out of scope for this feature

---

## Closed Vocabulary Schema Files

**Affects**: schema directory at `.parlay/schemas/`, validator term lookup
**Behavior**: Maintain four closed-vocabulary schema files — one per closed list — that enumerate the v1 contents:
- operation_kinds: `command`, `query`
- steps (write group): `validate-input`, `authorize`, `create-one`, `update-one`, `delete-one`
- steps (read group): `read-one`, `read-many`, `search`
- steps (return group): `return-one`, `return-many`, `return-empty`
- errors: `validation-failed`, `unauthorized`, `forbidden`, `not-found`, `conflict`, `server-error`
- policies: `auth-required`, `permission-required`, `transaction-required`

Every term lookup (adapter `supports`, capability operation kind/step/error/policy validation, blueprint strategy validation against an adapter that opts in) consults these schema files. Subscriptions and jobs are explicitly excluded from `operation_kinds` in v1 with a "deferred to v2" annotation so the validator can produce a kind-aware fix message.
**Invariants**:
- Each closed-vocabulary schema file enumerates exactly the terms above at v1 ship time
- A term outside its vocabulary's enumerated set produces `*-unknown-term` (the prefix differs per consumer: `adapter-supports-`, `capabilities-`, `blueprint-strategy-`)
- A v2-deferred term (`subscription`, `job`) produces a fix message that says "deferred to v2" rather than just "not in the list"
- Adapters cannot extend the vocabulary lists; they only opt into subsets via `supports`
- A future-version PR that adds a term to a closed list AND no shipped adapter declares support for it triggers a CI flag (the schema-extension gate) so the term is not used before any implementation exists
**Source**: @multi-adapter/v1-closed-vocabularies-and-v2-deferrals
**Backward-Compatible**: yes

**Notes**:
- Step partitioning into write/read/return groups is informational for adapter authors; the validator treats step terms as a flat namespace
- A docs-build check compares each closed-vocabulary schema file's terms to this feature's intent enumeration so drift is caught at PR time

---

## Bundled Adapter Set Presets and Installation

**Affects**: project setup command (`parlay init`), bundled template directory
**Behavior**: Ship four bundled adapter-set templates inside the tool — `react-antd-only`, `angular-clarity-only`, `react-nest-prisma`, `angular-nest-prisma` — each consisting of a `.parlay/adapter-set.yaml` plus the adapter files it references. Designate `react-nest-prisma` (react-antd + openapi-rest + nestjs-application + prisma-postgres) as the v1 first preset exercised end-to-end in CI. Extend `parlay init` to prompt for a preset by name (or `custom`) and copy the chosen preset's files into the project. Custom mode skips the copy and leaves the user to author `.parlay/adapter-set.yaml` from scratch.
**Invariants**:
- `parlay init` lists exactly the four named presets plus `custom` at v1 ship time
- A preset's adapter-set references only adapter files the tool also bundles (the preset-completeness check fails build with `preset-adapter-missing` otherwise)
- Choosing `react-nest-prisma` produces a four-kind adapter-set with all four corresponding adapter files copied into `.parlay/adapters/`
- Choosing `react-antd-only` produces a presentation-only adapter-set with one adapter file
- Choosing `custom` skips the copy entirely
- Renaming a preset is a public-contract change; an alias path is required (preset names are not silently changed)
- A project that diverges from its starting preset over time is fully supported; no "preset compliance" check fires after setup
**Source**: @multi-adapter/bundled-adapter-set-presets
**Backward-Compatible**: yes

**Notes**:
- The v1 first preset is exercised in v1 CI as the canonical full-stack project shape; other presets get smaller smoke tests
- Adding a new preset later requires shipping both the adapter-set template and any new adapter templates it references in the same change

---

## Capabilities Artifact Schema, Parser, and Operation ID Normalization

**Affects**: spec layer schema, build-feature parser, downstream operation reference resolution
**Behavior**: Define `spec/intents/<feature>/capabilities.yaml` with top-level `schema_version`, `feature`, and `operations:` keys. Each operation has `id` (feature-local), `kind`, `subject`, `input`, `output`, `errors`, `policies`, and `steps`. Parse the file, validate every term against the closed vocabularies, and normalize the operation id to `@<feature>/operation:<id>` on the way into the buildfile — the normalized form is used everywhere downstream (buildfile keys, target references, testcase `act.operation`, `source_refs`). Reject prose-only fragments in build mode; warn in authoring mode.
**Invariants**:
- A capabilities file with all closed terms parses and emits canonical operations into the buildfile under the normalized id
- A term outside its closed vocabulary fails with `capabilities-unknown-term` naming the field, the term, and the vocabulary list
- Two operations sharing an `id` within one feature fail with `capabilities-duplicate-operation-id`
- A prose-only fragment fails build mode with `capabilities-not-closed-form`; warns in authoring mode
- A reference downstream that uses the bare local id (without the `@feature/operation:` prefix) fails with `buildfile-operation-ref-unnormalized`
- A feature without backend behavior simply omits `capabilities.yaml`; the buildfile's `operations:` block is empty and downstream operation-coverage rules walk an empty set
**Source**: @multi-adapter/capabilities-yaml-replaces-infrastructure-as-the-closed-vocabulary-backend-artifact
**Backward-Compatible**: yes

**Notes**:
- The artifact succeeds (does not coexist with) `infrastructure.md`; the legacy markdown is migration input only after this feature ships
- Op-id normalization happens in build-feature, not at authoring time — designer-facing files stay terse with feature-local ids

---

## Domain Model Operations Deprecation and Migration

**Affects**: domain-model schema (deprecation marker), `parlay validate` rule set, `parlay migrate-domain-operations` command
**Behavior**: Mark `operations:` in the `domain-model.yaml` schema as deprecated. Validation emits `domain-operations-deprecated` as a warning in authoring mode and an error in build mode for any project whose `domain-model.yaml` populates the field. Build-feature stops consuming the field for routing or codegen, regardless of mode. Add a migrator entry point `parlay migrate-domain-operations` that walks each entry under `domain-model.operations[*]`, asks the designer which feature owns it (interactive when ambiguous), and writes a stub into the chosen feature's `capabilities.yaml` with `kind: unknown` and prose carried over verbatim under `notes:`. After all entries are processed, the legacy `operations:` field is emptied or removed.
**Invariants**:
- `parlay validate` returns `domain-operations-deprecated` as a warning in authoring mode and an error in build mode when `domain-model.operations` is non-empty
- `parlay migrate-domain-operations` writes one stub per legacy entry; ambiguous target feature is resolved by interactive prompt, never by guess
- Stubs land with `kind: unknown`; `kind: unknown` is rejected in build mode with `capabilities-stub-unfilled` so unfilled stubs cannot reach codegen
- Build-feature does not read `domain-model.operations` for routing or codegen; the only path from those entries to the buildfile is via the migration step
- Domain-model retains its scope for entities, relationships, states, enums, and value objects unchanged
- Re-running the migrator on a project with no legacy field is idempotent (no-op)
**Source**: @multi-adapter/domain-model-yaml-operations-field-is-deprecated-in-favor-of-capabilities, @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape
**Backward-Compatible**: yes

**Notes**:
- Outright removal of the field is owned by a separate deprecation feature scheduled for a later version
- The migrator never fabricates closed-vocabulary terms — that is designer authoring work after migration

---

## Surface YAML Schema, Parser, and Migration

**Affects**: spec layer schema (surface artifact), surface parser, `parlay migrate-spec` command
**Behavior**: Add `surface.yaml` to the spec layer as the target format alongside the existing `surface.md` legacy form. Both formats parse to the same in-memory surface model, so the build pipeline does not branch on serialization. `parlay migrate-spec` parses each feature's `surface.md` via the legacy parser and emits `surface.yaml` alongside, writing a per-feature migration report listing any free-text content that did not map to a closed-schema field. The migrator is idempotent — re-running on already-migrated input produces no further changes.
**Invariants**:
- A feature with `surface.yaml` (and no `surface.md`) builds and codegens identically to a feature with the same content represented as `surface.md`
- A feature with both forms present prefers the YAML and emits `surface-md-superseded` (warning) so the designer can delete the stale markdown
- Free-text content with no closed-schema destination lands in the per-feature migration report rather than being silently dropped
- The migrator is idempotent across re-runs
- Legacy `surface.md` is retained after migration as inert documentation; deletion is the designer's call
**Source**: @multi-adapter/surface-yaml-replaces-surface-md-as-the-closed-presentation-artifact-format, @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape
**Backward-Compatible**: yes

**Notes**:
- v1 accepts either form; a later version may make YAML mandatory once project-wide migration is complete
- Markdown remains appropriate for narrative documents (intents, dialogs, proposals); only the machine-validated spec artifacts move to YAML

---

## Blueprint Scope, Override Precedence, and Strategy Selection

**Affects**: blueprint schema, layered-setting resolver, error-mapping resolver
**Behavior**: Pin blueprint's owned scope to `data`, `auth`, `errors`, `state`, `navigation`, and `platform`. Reject any `targets:` block in blueprint (topology is owned by `.parlay/adapter-set.yaml`). Resolve any layered setting through the precedence `blueprint > adapter-set > adapter default`. Validate strategy choices against the relevant adapter's declared support — `data.fetching: {on-mount, prefetch, stale-while-revalidate, graphql}`, `data.caching: {none, per-route, shared}`, `auth.strategy: {none, session, jwt, oauth2}`, `errors.retry: {none, reads, writes, all}`. Walk every canonical operation error and confirm a mapping exists at adapter, adapter-set, or blueprint level; fail with `error-no-mapping` otherwise.
**Invariants**:
- A blueprint declaring a `targets:` block fails with `blueprint-topology-not-allowed`
- A strategy value outside its closed vocabulary fails with `blueprint-strategy-unknown`
- A strategy value not declared in the relevant adapter's support fails with `blueprint-strategy-unsupported`
- A canonical operation error with no mapping at any level fails with `error-no-mapping` naming the operation, the error, and the missing layer (transport or presentation)
- A setting outside the owned scope (e.g. `blueprint.deployment.region`) fails with `blueprint-scope-violation`
- Override conflicts within a single layer fail with `blueprint-override-conflict`
- A project with no `blueprint.yaml` resolves every setting to the adapter default and validates clean
**Source**: @multi-adapter/blueprint-scope-override-precedence-and-strategy-selection
**Backward-Compatible**: yes

**Notes**:
- The strategy vocabularies are closed at the schema level; v1 fixes the lists shown above
- The resolved-value report (which layer wins for each setting) is exposed through `parlay validate` so designers can audit precedence decisions

---

## Multi-Target Buildfile Schema and Validator

**Affects**: buildfile schema, build-feature emitter, buildfile validator (canonical-once rule, target operation ref resolution, plan target routing)
**Behavior**: Extend `buildfile.yaml` with two top-level blocks: `operations:` (canonical operation declarations keyed by normalized id, carrying every canonical field exactly once) and `targets:` (one entry per registered adapter, keyed by kind, carrying per-target projection metadata only). Forbid restating canonical fields under any target — `kind`, `subject`, `input`, `output`, `errors`, `policies`, and `steps` appear under `operations:` exclusively. Extend `plan:` with a `targets:` sub-block where each target's plan rows live under `plan.targets.<kind>.creates/modifies`. Existing `wiring.rules` and `bindings` sections coexist unchanged; their operation refs must resolve to keys under `operations:`.
**Invariants**:
- A buildfile declaring an operation under `operations:` and projecting it through target sections without restating canonical fields validates clean
- A target section restating a canonical field fails with `buildfile-target-restates-canonical` naming the target, the operation, and the field
- A binding referencing an operation absent from `operations:` fails with `buildfile-binding-operation-missing`
- A target's `effect.operation` (or other op-ref) referencing an absent operation fails with `buildfile-target-operation-missing`
- Plan rows scoped under `plan.targets.<kind>` route to the correct target during codegen emission
- `wiring.rules` and `bindings` sections survive build-feature normalization byte-equivalent (no rewriting of layout-aware-build content)
**Source**: @multi-adapter/multi-target-buildfile-operations-targets-and-target-aware-plan
**Backward-Compatible**: yes

**Notes**:
- Per-target projection shapes are kind-specific (presentation → components; transport → exposure/method/path; application → handler; persistence → repositories) and live in each adapter's schema
- The canonical-once rule is the central drift prevention: two prompt sources for the same fact give the AI ambiguous structural input

---

## Legacy Buildfile Field Normalization

**Affects**: build-feature normalization pass, designer-review prompt flow
**Behavior**: On first regeneration of a legacy buildfile, walk the pre-multi-target shape and normalize:
- top-level `adapter:` → `adapter-set` reference plus `targets.<kind>.adapter` declarations (single-target presentation set when no other slots are involved)
- top-level `components:` → `targets.presentation.components`
- top-level `routes:` → `targets.presentation` (client-side) or `targets.transport` (HTTP) — disambiguate via designer prompt when the path collides with a transport HTTP exposure
- top-level `plan.creates`/`plan.modifies` → `plan.targets.<kind>.creates/modifies`
- non-empty `models:` → emit `buildfile-models-deprecated` (warning in authoring, error in build) directing to `domain-model.yaml`

Surface the diff to the designer for review before any write; abort if the designer cancels. Reject double-declared shapes (legacy + new under the same content).
**Invariants**:
- A buildfile with both top-level `components:` and `targets.presentation.components:` populated fails with `buildfile-components-double-declared`
- A buildfile with non-empty `models:` warns in authoring mode and errors in build mode
- A buildfile with `routes:` whose path collides with a transport HTTP exposure fails with `buildfile-routes-ambiguous` if the designer cancels disambiguation
- `wiring.rules` and `bindings` are out of scope for normalization; the diff inside them is empty
- Normalization is idempotent — re-running on an already-normalized buildfile produces no changes
**Source**: @multi-adapter/legacy-buildfile-fields-stay-deprecate-or-repurpose
**Backward-Compatible**: yes

**Notes**:
- Outright removal of legacy fields (`adapter:`, `models:`) is owned by separate deprecation features scheduled for a later version
- Designer-visible diff is a hard requirement: silent rewrites are rejected

---

## Testcases V2 Schema with Discriminated Suite Kinds

**Affects**: testcases schema (`schema_version: 2`), suite parser, source_refs resolver, operation-coverage walker
**Behavior**: Bump `testcases.yaml` to schema version 2. Each suite carries a `kind` discriminator with the closed set `{presentation, operation}`. Operation suites reference the canonical operation key and assert over `output`, `error`, and `persistence` shapes. Walk every canonical operation in `capabilities.yaml` and confirm at least one `kind: operation` suite covers it. Validate every new v2 suite's `source_refs` entries against real surface fragments and capability operations. Load legacy v1 suites as v2 presentation suites; auto-populate `source_refs[0]` from each legacy suite's `intent` string so the migrated suite carries provenance from day one.
**Invariants**:
- A canonical operation with no covering operation suite (and no exemption in coverage-review) fails with `testcases-operation-uncovered`
- A new v2 suite without `source_refs` fails with `testcases-source-refs-missing`
- A legacy v1 suite without additional `source_refs` (beyond auto-populated `[0]`) emits `testcases-source-refs-missing-legacy` as a warning until v2 regeneration
- A suite with a `kind` value outside `{presentation, operation}` fails with `testcases-suite-kind-unknown`
- An operation suite asserting a shape outside the canonical operation's `output`/`error`/`persistence` projection fails with `testcases-operation-shape-mismatch`
- A presentation-only project (no `capabilities.yaml`) requires no operation suites
**Source**: @multi-adapter/testcases-yaml-v2-discriminated-suite-kinds-and-source-refs
**Backward-Compatible**: yes

**Notes**:
- Source ref selectors use stable forms: `surface.fragments['<id>']`, `capabilities.operations['<id>']`, `surface.fragments['<id>'].actions['<action>']`
- Auto-populated `source_refs[0]` from legacy intent gives migrated suites minimal provenance; designers expand with regeneration

---

## Coverage Review Hash Computation and Codegen Gate

**Affects**: review-coverage command, generate-code gate, canonical-form serialization
**Behavior**: Add `.parlay/build/<feature>/coverage-review.yaml` with required keys `feature`, `reviewed_at`, `reviewed_by`, `review_method`, `buildfile_hash`, `testcases_hash`, `approved_suites`, plus optional `exemptions`. Compute `buildfile_hash` over a deterministic canonical-form serialization of `buildfile.yaml` and `testcases_hash` over the equivalent serialization of `testcases.yaml`. Add `parlay review-coverage <feature>` to walk suites, record approvals, collect exemptions for missing coverage, and write the review file. Make `parlay generate-code` refuse to run if the review file is missing, either hash does not match the on-disk artifact, any required suite is unapproved, or any required term (operation kind, step, error, policy) lacks both a covering case and an explicit exemption.
**Invariants**:
- A missing review file fails generate-code with `coverage-review-missing` before any other read
- A drifted hash fails with `coverage-review-stale` naming which hash drifted (`buildfile_hash` or `testcases_hash`)
- A suite present in `testcases.yaml` but absent from `approved_suites:` fails with `coverage-review-suite-unapproved`
- A required term with no covering case and no exemption fails with `coverage-review-uncovered` naming the operation, the term, and the term kind
- An exemption entry binds the suite, the missing item, and a free-text reason; no exemption schema or closed marker vocabulary exists in v1
- Re-running build-feature changes the canonical-form hashes and forces re-approval
- Cosmetic edits (whitespace, key order) do not invalidate the review because hashes are computed over canonical form
**Source**: @multi-adapter/coverage-review-yaml-gates-codegen-on-human-approval
**Backward-Compatible**: yes

**Notes**:
- The gate is a workflow integrity mechanism, not a cryptographic security boundary; it records local human review intent
- A future version may add signed review records or a `review_subject_hash` covering adapter-set/adapters/blueprint/domain-model if review surface needs to widen

---

## Codegen Read-Set and Layer Pipeline

**Affects**: generate-code input enforcement, layer ordering, prompt-context plumbing
**Behavior**: Pin generate-code's allowed input set to `.parlay/build/<feature>/{buildfile.yaml, testcases.yaml, coverage-review.yaml}`, `.parlay/adapter-set.yaml`, the referenced adapter files under `.parlay/adapters/`, `blueprint.yaml`, `config.yaml`, `domain-model.yaml`, and the source tree under each adapter's declared root. Forbid reads of `spec/intents/**` — instrumentation traps any attempt and aborts the run. Emit kinds in the order persistence → application → transport → presentation, fully completing each layer before starting the next. Feed each layer's freshly-generated outputs into the next layer's prompt context so the in-progress codegen state informs subsequent generation.
**Invariants**:
- A read attempt against `spec/intents/**` aborts generate-code with `codegen-spec-read-forbidden` before any output is written
- A read attempt against a path outside any target's declared root aborts with `codegen-input-out-of-scope`
- Layer order is `persistence → application → transport → presentation` by default; an override requires explicit declaration in adapter-set
- Each layer's freshly-emitted outputs are visible in the next layer's prompt-context section
- Two regeneration runs of the same buildfile pass the same testcase suite even when their source bytes differ
- Generated-code hashes are tracked but byte drift alone is not a failure when the suite passes
**Source**: @multi-adapter/codegen-flow-ordered-layer-generation-and-fixed-read-set
**Backward-Compatible**: yes

**Caching**: per-feature-per-run

**Notes**:
- The fixed read-set is the testable form of "the buildfile is the executable contract"
- Layer ordering reordering is a behavioral change, not a mechanical reshuffle, because of the inform-the-next plumbing

---

## Validation Mode Dispatch

**Affects**: validator entry-point dispatch, per-rule severity table
**Behavior**: Define two validation modes: authoring (permissive, warning-rich; invoked by `parlay validate` and editor integrations) and build (strict; invoked by `parlay build-feature`, `parlay generate-code`, and CI hooks). Determine the mode from the entry point — never from a CLI flag the designer toggles. Each rule declares its severity per mode in its schema; rules without an explicit declaration default to error in both modes. Authoring mode surfaces every error-in-build-mode rule as a warning at minimum so the designer running validate locally sees what build will reject. Build mode never downgrades errors.
**Invariants**:
- `parlay validate` runs in authoring mode; `parlay build-feature` and `parlay generate-code` run in build mode
- Every rule introduced by this feature has an explicit per-mode severity in its schema
- A rule without an explicit per-mode severity defaults to error in both modes (verified by the rule-schema test in CI)
- An error code fired in both modes is identical; only severity differs
- Authoring mode never silently passes a build-mode failure
**Source**: @multi-adapter/validation-modes-authoring-vs-build
**Backward-Compatible**: yes

**Notes**:
- A future "lint mode" or similar is out of scope; v1 has exactly two modes
- The mode-by-entry-point rule keeps designers from having to remember a flag during day-to-day work

---

## Migration Command Family and Sequencing

**Affects**: CLI command registration (`parlay migrate-config`, `parlay migrate-spec`, `parlay migrate-capabilities`, `parlay migrate-domain-operations`), build-feature in-process buildfile normalization
**Behavior**: Implement the migration commands as independent entry points, each running on its respective legacy artifact:
- `parlay migrate-config`: `prototype-framework: <value>` → single-target presentation adapter-set
- `parlay migrate-spec`: `surface.md` → `surface.yaml` (per-feature)
- `parlay migrate-capabilities`: `infrastructure.md` operation-shaped fragments → `capabilities.yaml`; pattern-shaped fragments to migration report
- `parlay migrate-domain-operations`: `domain-model.operations[*]` → per-feature `capabilities.yaml` stubs
- Build-feature in-process: legacy buildfile shape → multi-target shape with designer-visible diffs

Each migration is independent (no implicit ordering) and idempotent (re-running on migrated input is a no-op). No migration step silently overwrites a designer-authored file; each surfaces its diff or report.
**Invariants**:
- Each migration command can be invoked independently; ordering between them is a designer choice, not a tool constraint
- Re-running any migration on already-migrated input produces no diff (no-op)
- No migration silently overwrites existing content; ambiguous decisions prompt the designer
- A project that has not run any migration step continues to validate and build (with deprecation warnings) under v1
- Each migration step's failure modes leave the legacy input in place — partial migration is rolled back
**Source**: @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape
**Backward-Compatible**: yes

**Notes**:
- Outright removal of legacy fields (`prototype-framework`, top-level `adapter:`, `models:`, `domain-model.operations`) is owned by separate deprecation features scheduled for a later version
- This fragment owns the introduction of the migration commands; their no-op buffers and removal sequencing are owned by the deprecation features

---

## Pattern Fragment Classifier

**Affects**: `parlay migrate-capabilities` second pass, migration report writer
**Behavior**: Inside `parlay migrate-capabilities`, after the operation-shaped extractor consumes operation-shaped fragments, walk the residual paragraphs in legacy `infrastructure.md` and classify by detected shape. Pair each classified fragment with a suggested destination drawn from a closed list:
- pipeline → command operation in capabilities (or v2-deferred job)
- registry → domain entity plus register/list/lookup operations
- dispatcher → transport-adapter routing logic
- traversal → query operation with `read-tree` (when supported)
- resolver → query operation
- validator → `validate-input` step or shared step
- aspect → operation policy
- cache → cacheable policy plus `blueprint.data.caching`
- migrator → domain-model state machine plus migration command
- hook → v2-deferred subscription, or adapter lifecycle config
- helper / utility → adapter-level pattern, not spec-layer
- otherwise → "unrouted; designer review"

Write a per-feature migration report listing each fragment by source line range, detected shape, and suggested destination. Never auto-apply a suggestion — destination changes are designer authoring work.
**Invariants**:
- The classifier is conservative; ambiguous fragments fall into "unrouted; designer review" rather than risking misclassification
- The migrator never edits `capabilities.yaml`, `domain-model.yaml`, or `blueprint.yaml` based on classifier suggestions; it only writes the report
- v2-deferred fragments (subscription/job/hook-with-subscription-shape) are preserved verbatim with the deferral annotation
- Helper / utility fragments are explicitly tagged as adapter-level (not spec-layer) to discourage wedging them into capabilities
- Re-running the classifier on unchanged input produces an identical report
**Source**: @multi-adapter/pattern-fragment-decomposition-during-capabilities-migration
**Backward-Compatible**: yes

**Notes**:
- The closed list of suggested destinations is owned by Parlay; new pattern shapes extend the list in later versions
- This fragment is the second pass of migrate-capabilities; the first pass is handled by the operation-shaped extractor inside the Capabilities Artifact fragment's parser

---

## Presentation-Only Short-Circuit

**Affects**: every multi-target validation rule, slot-composition introspection
**Behavior**: Make every multi-target validation rule consult the project's `.parlay/adapter-set.yaml` slot composition before applying. When only the presentation slot is filled (or no adapter-set exists), short-circuit backend rules — `capabilities.yaml` is not required, backend `supports` validation is skipped, link enforcement walks zero edges, blueprint backend strategies do not validate, operation-suite coverage walks zero operations, and `coverage-review.yaml` is required only once the project has migrated to v2 testcases. Adding the first non-presentation slot transitions the project into multi-target mode automatically; no explicit "migrate to multi-target" step is required.
**Invariants**:
- A presentation-only project run end-to-end produces byte-equivalent output before and after this feature ships (modulo unrelated changes)
- A presentation-only project with no `capabilities.yaml` builds clean and produces a buildfile whose `operations:` block is empty
- Legacy v1 testcase suites in presentation-only projects warn (not error) for missing `source_refs` until v2 regeneration
- A presentation-only project on legacy v1 testcases does not require `coverage-review.yaml`; gate activates on v2 regeneration
- Adding a non-presentation slot to `.parlay/adapter-set.yaml` activates multi-target rules on the next build with no explicit migration step
- Every error code introduced by this feature has a documented presentation-only short-circuit check in its rule definition
**Source**: @multi-adapter/presentation-only-projects-continue-to-work-unchanged
**Backward-Compatible**: yes

**Notes**:
- The short-circuit is mechanical (a rule's first check is slot composition), not promised in prose — auditable by walking the rule definitions
- This fragment is what distinguishes "additive feature" from "schema migration" for the user-facing rollout

---

## CLI Command Registration and Deployer Integration

**Affects**: CLI command registration (`core/cmd/root.go` or equivalent), deployer hardcoded command lists, CLAUDE.md template for skill exposure
**Behavior**: Register the new commands introduced by this feature — `parlay migrate-config`, `parlay migrate-spec`, `parlay migrate-capabilities`, `parlay migrate-domain-operations`, `parlay review-coverage` — in the CLI command registry. Update the generic deployer's hardcoded command list (per the project blueprint's deployer cross-cutting rules) so the generic agent surface advertises the new commands. Add per-skill files to the embedded skills list so the Claude and Cursor deployers materialize them on `parlay init` / `parlay upgrade`.
**Invariants**:
- Each new command is registered in the CLI's standard handler and appears in `parlay --help`
- The generic deployer's hardcoded command list contains every new command at the moment this feature ships
- The embedded skills list contains the corresponding `parlay-migrate-config`, `parlay-migrate-spec`, `parlay-migrate-capabilities`, `parlay-migrate-domain-operations`, `parlay-review-coverage` skill sources
- `parlay upgrade` deploys the new skills to the agent surface (`.claude/skills/`, `.cursor/skills/`, `AGENT_INSTRUCTIONS.md`)
- Renaming any command requires updating both the CLI registration and the generic deployer's list in the same change (per the blueprint's cross-cutting rule)
**Source**: @multi-adapter/migration-of-legacy-artifacts-to-the-new-shape, @multi-adapter/coverage-review-yaml-gates-codegen-on-human-approval
**Backward-Compatible**: yes

**Notes**:
- This fragment is the integration surface for the deployer architecture documented in `core/.parlay/blueprint.yaml`
- Adapter files (`.parlay/adapters/`) are NOT covered by this fragment — per the dogfooding rules, adapters are project-owned and `parlay upgrade` deliberately leaves them alone

---
