# Multi-adapter — Dialogs

---

### Adapter kinds and adapter-set topology

**Trigger**: `parlay validate` or `parlay build-feature` loads an adapter file or `.parlay/adapter-set.yaml`.

User: Authors `.parlay/adapters/prisma-postgres.adapter.yaml` with `kind: persistence` and registers it in `.parlay/adapter-set.yaml` under `targets.persistence: {adapter: prisma-postgres, root: apps/api}`.
System (background): Reads the adapter file, validates `kind:` against the closed set `{presentation, transport, application, persistence}`, then reads the adapter-set entry, resolves `prisma-postgres.adapter.yaml` under `.parlay/adapters/`, and confirms its declared kind matches the slot.
System: Validation passes. The persistence slot is now filled and downstream rules (link enforcement, supports contract, capabilities resolution) take it into account.

#### Branch: Adapter file missing `kind:` (legacy default)

User: Loads an existing presentation-only project whose `.parlay/adapters/react-antd.adapter.yaml` predates this feature and has no `kind:` field.
System (background): Treats the absence of `kind:` as the legacy default `kind: presentation`. No rewrite is performed at validate time.
System: Validation passes; the adapter is usable in `targets.presentation`. `parlay upgrade` later offers to write the explicit field, but does not require it.

#### Branch: Adapter file with unknown `kind:` value

User: Authors an adapter file with `kind: storage`.
System: Emits `adapter-kind-unknown` naming the offending value `storage` and the closed set `{presentation, transport, application, persistence}`. Fix message: `set kind: to one of the four closed values, or omit the field to default to presentation`.

#### Branch: Adapter set lists two adapters under the same kind

User: Authors `.parlay/adapter-set.yaml` with `targets.presentation: {adapter: react-antd, ...}` and a second `targets.presentation: {adapter: react-mui, ...}` (or a YAML map duplicate).
System (background): Detects the duplicate key during parse.
System: Emits `adapter-set-duplicate-kind` naming the kind `presentation` and the two adapter references, with a fix message: `v1 supports at most one target per adapter kind — pick one and remove the other`.

#### Branch: Adapter set references a missing adapter file

User: Authors `.parlay/adapter-set.yaml` with `targets.transport: {adapter: openapi-rest}` while no `.parlay/adapters/openapi-rest.adapter.yaml` exists.
System: Emits `adapter-set-adapter-missing` naming the unresolved path `.parlay/adapters/openapi-rest.adapter.yaml`. Fix message: `add the adapter file or change the targets.transport.adapter reference`.

#### Branch: Slot kind contradicts referenced adapter's declared kind

User: Authors `.parlay/adapter-set.yaml` with `targets.application: {adapter: nest-app}` while `nest-app.adapter.yaml` declares `kind: persistence`.
System: Emits `adapter-set-kind-mismatch` naming both the slot (`application`) and the adapter's actual kind (`persistence`). Fix message: `either move the adapter to its actual slot, or set its kind: field to match the slot it occupies`.

#### Branch: Presentation-only project with only one slot filled

User: Authors `.parlay/adapter-set.yaml` with `targets.presentation:` filled and no other slots.
System (background): Validates the adapter set against the topology rules; the unfilled slots impose no constraints on this set.
System: Validation passes. Downstream rules that key on non-presentation kinds (link enforcement, capabilities, supports for backend kinds) will short-circuit on this project (see "Presentation-only projects continue to work unchanged").

---

### Adapter-set links enforce cross-kind boundaries

**Trigger**: `parlay build-feature` walks the resolved buildfile's cross-kind references and consults `.parlay/adapter-set.yaml`'s `links:` block.

User: Authors `.parlay/adapter-set.yaml` with full four-kind topology and `links: [{from: presentation, relation: calls, to: transport}, {from: transport, relation: dispatches, to: application}, {from: application, relation: persists, to: persistence}]`. Builds a feature whose presentation actions call transport operations and whose application steps persist through persistence.
System (background): Walks each cross-kind edge in the buildfile's `targets:` (presentation `effect.operation` calling transport, application step `create-one` going through persistence). For each edge, looks up the `(from-kind, to-kind)` pair in `links:` and confirms a permitting relation exists.
System: Validation passes. The buildfile proceeds to codegen.

#### Branch: A presentation component references a persistence repository directly

User: Authors a presentation component whose action `effect.operation` resolves to a persistence-layer operation, bypassing transport and application.
System (background): Walks the edge `(from: presentation, to: persistence)`; finds no permitting relation in `links:`.
System: Emits `adapter-set-link-violated` naming the source slot (`presentation`), the target slot (`persistence`), and the missing relation. Fix message: `either route the call through transport + application (the standard relation is calls → dispatches → persists), or — if the project genuinely needs presentation-to-persistence — add an explicit links: entry, but reconsider whether the boundary is intentional`.

#### Branch: `links:` block is omitted

User: Authors `.parlay/adapter-set.yaml` with full topology but no `links:` block.
System (background): Treats the absence of `links:` as "no cross-kind edges permitted"; intra-kind edges (a presentation component referencing another presentation fragment) are still legal.
System (condition: any cross-kind edge exists in the buildfile): Emits `adapter-set-link-missing` for the first such edge encountered, naming the source and target kinds and a fix message: `declare cross-kind relations explicitly in adapter-set.yaml's links: block; without it, all cross-kind access is rejected`.

#### Branch: A `links:` entry references an unfilled slot

User: Authors `.parlay/adapter-set.yaml` with `targets.presentation:` filled, no transport slot, but `links: [{from: presentation, relation: calls, to: transport}]`.
System (background): Resolves the link's `to: transport`; finds no slot for it.
System: Emits `adapter-set-link-unfilled-slot` naming the offending link and the missing slot `transport`. Fix message: `links: may not anticipate kinds the project did not register — either fill the transport slot, or remove the link`.

#### Branch: Authoring mode warns instead of failing

User: Runs `parlay validate` (authoring mode) on a project mid-edit whose adapter-set lists targets but has not yet authored `links:`.
System (background): Surfaces would-be link violations as warnings rather than errors so the project remains editable.
System: Returns "valid with warnings" listing each cross-kind edge that lacks a permitting relation. The same project under `parlay build-feature` (build mode) would fail.

#### Branch: Presentation-only project has no cross-kind edges

User: Builds a feature in a project whose adapter-set fills only `targets.presentation:`.
System (background): The buildfile contains no cross-kind references; link enforcement walks an empty edge set.
System: Validation passes regardless of whether `links:` is present or absent. The presentation-only short-circuit applies.

---

### Adapter `supports` contract gates codegen pre-AI

**Trigger**: `parlay build-feature` resolves a feature's `capabilities.yaml` into the buildfile and checks each operation's terms by union coverage across the filled backend adapters' `supports:` declarations.

User: Authors `nestjs-application.adapter.yaml` with the terms the application layer owns — `supports: {operation_kinds: [command, query], steps: [validate-input, authorize, return-one], policies: [auth-required], errors: [validation-failed, unauthorized]}` — and `prisma-postgres.adapter.yaml` with the terms the persistence layer owns — `supports: {operation_kinds: [command, query], steps: [create-one, read-one], policies: [transaction-required], errors: [conflict, not-found]}`. Builds a feature whose `task.create` operation uses `validate-input`, `create-one`, `return-one`.
System (background): Runs the per-adapter shape/vocabulary check on each backend adapter, then checks each operation term by **union coverage**: `validate-input` and `return-one` are owned by the application adapter, `create-one` by the persistence adapter — every term is owned by some filled backend layer.
System: Validation passes. Build-feature emits the buildfile with the operation projected through the application and persistence targets, each target's `operations."@f/op:id".owns:` listing the steps it implements.

#### Branch: Feature uses a step no backend layer supports

User: Adds a step `read-tree` to `task.search`; no filled backend adapter lists `read-tree` in its `supports.steps`.
System (background): Union coverage: `read-tree` appears in no filled backend adapter's `supports.steps`.
System: Emits `adapter-supports-missing-step` naming the operation `@task-list/operation:task.search`, the term `read-tree`, and the filled backend slots. Fix message: `no configured backend layer implements this step — add or swap in an adapter whose layer supports it, or remove the step`. Generation does not proceed; the AI is not invoked.

#### Branch: Adapter declares a term not in the closed vocabulary

User: Authors an adapter with `supports.steps: [validate-input, telepathy]`.
System (background): Walks the supports declaration; looks up `telepathy` in the v1 steps vocabulary.
System: Emits `adapter-supports-unknown-term` naming `telepathy`, the term kind `steps`, and the adapter. Fix message: `supports: may only enumerate terms that exist in the closed vocabulary — adapters opt into subsets, they do not invent terms`.

#### Branch: Presentation adapter without `supports:` block

User: Loads a presentation adapter that has the existing surface-vocabulary contract but no `supports:` block.
System (background): Recognizes the kind as `presentation`; the `supports:` requirement applies only to non-presentation kinds.
System: Validation passes. Presentation-vocabulary checks run as before; the new `supports:` rules are inapplicable for this kind.

#### Branch: Pattern descriptions present but unmatched

User: Authors an application adapter with `supports.steps: [validate-input, create-one]` and pattern descriptions for `validate-input`, `create-one`, and `read-tree`. The feature uses only `validate-input` and `create-one`.
System (background): Validation walks `supports`, not pattern descriptions. The `read-tree` description is unused but does no harm.
System: Validation passes. Pattern descriptions feed the AI prompt at codegen time only when their term is used; an unused description has zero validation footprint.

#### Branch: Feature uses a policy no backend layer owns

User: Adds `policies: [transaction-required]` to an operation in a project whose only filled backend slot is `application` (no persistence adapter). `transaction-required` is persistence-owned, so no filled backend adapter lists it.
System: Emits `adapter-supports-missing-policy` naming the operation, the policy `transaction-required`, and the filled backend slots. Generation does not proceed. (Add a persistence adapter that owns `transaction-required` and union coverage passes.)

---

### V1 closed vocabularies and v2 deferrals

**Trigger**: Any validator path that looks up a term — adapter `supports`, capabilities operation `kind` / step `type` / errors / policies — consults the closed-vocabulary schema files under `.parlay/schemas/`.

User: Authors an operation in `capabilities.yaml` with `kind: command`, steps `[validate-input, create-one, return-one]`, errors `[validation-failed, conflict]`, policies `[transaction-required]`.
System (background): Resolves each term against the relevant v1 closed vocabulary file (`operation-kinds.schema.md`, `steps.schema.md`, `errors.schema.md`, `policies.schema.md`). All terms appear.
System: Validation passes. The operation enters the buildfile under the normalized id `@<feature>/operation:<id>`.

#### Branch: Designer reaches for a v2-deferred operation kind

User: Authors `kind: subscription` on a capability operation.
System (background): Looks up `subscription` in `operation-kinds.schema.md`; finds it absent. Consults the v2-deferral list.
System: Emits `capabilities-unknown-term` naming `subscription`, the field `kind`, and the v1 list `[command, query]`. Fix message: `subscription is deferred to v2; use kind: command or kind: query for v1, or watch the v2 milestone for subscription support`.

#### Branch: Adapter declares a term that is in v1 vocabulary but unrelated to its kind

User: Authors a presentation adapter with `supports.steps: [create-one]`.
System (background): The presentation adapter has the existing surface-vocabulary contract; backend `supports` shape applies only to non-presentation kinds.
System: Emits `adapter-supports-shape-mismatch` naming the kind `presentation` and the offending field `supports.steps`. Fix message: `presentation adapters use the surface-vocabulary supports shape; remove backend supports declarations or change the adapter's kind`.

#### Branch: Unknown step name in capabilities

User: Authors a step `type: synchronize-orbit`.
System: Emits `capabilities-unknown-term` naming the step type and the v1 step list (write/read/return groups). Fix message: lists the closed terms grouped by write/read/return.

#### Branch: Future-version extension simulation (schema PR adds a term, no adapter declares support)

User: Lands a schema PR that adds `read-tree` to `steps.schema.md` but no shipped adapter declares `read-tree` in `supports.steps`.
System (background): The two-gate extension rule applies — schema gains the term, then at least one adapter must declare support before any feature may use it.
System: A feature using `read-tree` fails with `adapter-supports-missing-step` because no filled backend adapter supports it; the schema-extension test in CI flags `read-tree` as "added but unimplemented" until an adapter ships support.

#### Branch: Schema docs build catches drift between intent and schema files

User: Lands a change that edits `steps.schema.md` to remove `delete-one` without updating the v1 vocabulary intent.
System (background): The docs build compares the schema files to the intent's enumeration of v1 contents.
System: Fails with `v1-vocabulary-drift` naming the missing term and the intent that pinned it. Fix message: `keep steps.schema.md and the multi-adapter v1-vocabulary intent in sync — either restore the term, or amend the intent to drop it`.

---

### Bundled adapter-set presets

**Trigger**: `parlay init` (or the equivalent project-setup entry point) prompts the user to choose a preset.

User: Runs `parlay init my-app` and is presented with the choice list: `react-antd-only`, `angular-clarity-only`, `react-nest-prisma`, `angular-nest-prisma`, `custom`.
User: Picks `react-nest-prisma` (the v1 first preset).
System (background): Copies the bundled `.parlay/adapter-set.yaml` template (full four-kind topology — react-antd presentation, openapi-rest transport, nestjs-application application, prisma-postgres persistence) and the four corresponding adapter files into the new project.
System: Reports the preset chosen and the files written. The project is ready for `parlay add-feature`.

#### Branch: Designer picks a presentation-only preset

User: Picks `react-antd-only`.
System (background): Copies a `.parlay/adapter-set.yaml` with only `targets.presentation:` filled and a single adapter file (`react-antd.adapter.yaml`).
System: Reports the preset chosen. Multi-target rules will short-circuit on this project until/unless the user adds non-presentation slots.

#### Branch: Designer picks `custom`

User: Picks `custom`.
System: Skips the preset copy; leaves `.parlay/adapter-set.yaml` for the user to author from scratch. Prints a pointer to the schema doc and the closed kind vocabulary.

#### Branch: A preset's adapter-set references an adapter file the tool no longer bundles

User: Lands a change that renames a bundled adapter file but forgets to update the preset that referenced it.
System (background): The preset-completeness check runs at build time, walking each preset's `targets.<kind>.adapter` references against the bundled adapter templates.
System: Fails with `preset-adapter-missing` naming the preset and the missing adapter file. Fix message: `every preset's adapter-set must reference adapter files the tool also bundles; update the preset or restore the adapter`.

#### Branch: Project diverges from its starting preset over time

User: A project initialized from `react-nest-prisma` later replaces its persistence adapter with a custom `mongo-application.adapter.yaml` and removes `prisma-postgres.adapter.yaml`.
System (background): The adapter-set is editable like any other file; presets are not enforced after setup. No "preset compliance" check fires.
System: Validation passes against the project's current adapter-set, regardless of which preset it started from. The setup-time `parlay init` choice is not remembered as a constraint.

#### Branch: First preset exercised in CI

User: Lands a change that breaks codegen for one of the v1-supported kinds.
System (background): CI runs an end-to-end integration test against a project initialized from `react-nest-prisma` — `add-feature` → `build-feature` → `generate-code`. Other presets get smaller smoke tests.
System: The integration test fails on the first preset, surfacing the regression before merge. Smoke tests on other presets may also fail depending on how the change is scoped.

---

### capabilities.yaml replaces infrastructure as the closed-vocabulary backend artifact

**Trigger**: `parlay build-feature` loads `spec/intents/<feature>/capabilities.yaml`.

User: Authors `spec/intents/task-list/capabilities.yaml` with `schema_version: 1`, `feature: task-list`, and an `operations:` list whose first entry has `id: task.create`, `kind: command`, `subject: {entity: Task}`, `input: {type: CreateTaskInput}`, `output: {shape: one, entity: Task}`, `errors: [validation-failed, conflict]`, `policies: [transaction-required]`, and steps `[{type: validate-input}, {type: create-one, entity: Task, identity: generated}, {type: return-one, entity: Task}]`.
System (background): Parses the file, validates every term against the closed vocabularies, normalizes the operation id to `@task-list/operation:task.create`, and emits the canonical operation into the buildfile's `operations:` block.
System: The buildfile gains the canonical operation. Downstream target sections, testcases, and bindings reference it by the normalized id.

#### Branch: Operation uses a step outside the closed vocabulary

User: Adds a step `type: telepathy` to `task.create`.
System: Emits `capabilities-unknown-term` naming the field `steps[1].type`, the term `telepathy`, and the v1 step list. Fix message: lists the closed terms; cites the relevant adapter's `supports.steps` if the project has a non-presentation adapter that does not implement the term.

#### Branch: Prose-only fragment in build mode

User: Authors a `capabilities.yaml` with prose paragraphs describing operations rather than the closed schema.
System (background): The parser recognizes the file shape but cannot resolve closed-vocabulary terms.
System (build mode): Emits `capabilities-not-closed-form` with line ranges for the offending paragraphs. Fix message: `convert each prose description to the closed operation shape (id/kind/subject/input/output/errors/policies/steps), or run parlay migrate-capabilities to scaffold operation entries from operation-shaped fragments`.
System (authoring mode): Emits the same code as a warning; `parlay validate` returns "valid with warnings" so the file remains editable.

#### Branch: Two operations share an id within one feature

User: Authors two `operations:` entries with `id: task.create`.
System: Emits `capabilities-duplicate-operation-id` naming the offending id. Fix message: `operation ids are unique within a feature; rename one entry or merge them`.

#### Branch: Feature without backend behavior

User: Builds a feature whose `spec/intents/<feature>/` contains no `capabilities.yaml`.
System (background): The absence is treated as "no backend operations"; the buildfile's `operations:` block is empty.
System: Build proceeds. Validation does not require operation suites, supports checks against backend kinds, or coverage-review entries for operations.

#### Branch: Operation id normalization is consistent everywhere downstream

User: References the operation in a presentation target's `effect.operation`, in a testcase's `act.operation`, and in a binding rule.
System (background): Every reference uses the normalized form `@task-list/operation:task.create`. The validator confirms each reference resolves to the same key under `operations:`.
System: Validation passes. A reference that uses the bare local id (`task.create`, without the `@feature/operation:` prefix) fails with `buildfile-operation-ref-unnormalized` and a fix message: `references downstream of capabilities use the normalized form`.

---

### domain-model.yaml `operations:` field is deprecated in favor of capabilities

**Trigger**: `parlay validate` or `parlay build-feature` parses `domain-model.yaml` and finds a non-empty `operations:` field.

User: Loads a project whose `domain-model.yaml` lists three entries under `operations:` from before the multi-target migration.
System (background, authoring mode): Parses the field, treats it as deprecated, does not feed it into routing or codegen.
System (authoring mode): Emits `domain-operations-deprecated` as a warning. Fix message: `move these entries to per-feature capabilities.yaml — run parlay migrate-domain-operations to scaffold stubs`.
System (build mode): Same code, error severity. The build fails until the field is empty or removed.

#### Branch: Designer runs the migrator

User: Runs `parlay migrate-domain-operations`.
System (background): Walks each entry under `domain-model.operations[*]`. When the target feature is unambiguous (only one feature owns the relevant entity), writes a stub directly. When ambiguous, prompts the user.
System: For each entry, asks "which feature owns this operation?" via an AskUserQuestion list of candidate features. After answers, writes one stub per entry into the chosen feature's `capabilities.yaml` with `kind: unknown`, the prose carried over verbatim under `notes:`, and a TODO marker for designer review. Empties (or removes) the legacy `operations:` field in `domain-model.yaml`.

#### Branch: Build-feature ignores the legacy field for routing

User: Has a project whose `domain-model.yaml` still has populated `operations:` (migration not yet run) and an empty `capabilities.yaml`.
System (background): Build-feature does not read `domain-model.operations` for routing or codegen. The buildfile's `operations:` block is empty.
System: In authoring mode the build emits the deprecation warning and produces a valid (presentation-only) buildfile. In build mode the build fails because of the deprecated field even though the rest of the project is well-formed.

#### Branch: Domain-model retains its true scope

User: Edits `domain-model.yaml` to add a new entity, a relationship, and a state machine, with the legacy `operations:` field empty.
System: Validation passes. Entities/relationships/states/enums/value objects continue to drive entity resolution in the buildfile unchanged. Only the `operations:` field is affected by this intent.

#### Branch: Stub-then-author flow

User: Migrates legacy operations to stubs, then opens a stub in `capabilities.yaml` to author the closed-vocabulary content.
System (background): Stubs ship with `kind: unknown` deliberately so the designer must choose a real kind; the validator rejects `kind: unknown` in build mode with `capabilities-stub-unfilled`.
System: In authoring mode, the stub validates as a draft. Once the designer fills in the kind/steps/errors/policies, the entry is treated as a normal capability operation. Build mode requires no stubs to remain.

---

### surface.yaml replaces surface.md as the closed presentation artifact format

**Trigger**: `parlay build-feature` loads the surface artifact under `spec/intents/<feature>/`.

User: Authors `spec/intents/task-list/surface.yaml` (no `surface.md`) with the closed presentation vocabulary — fragments, actions, flows.
System (background): Parses the YAML directly into the in-memory surface model, validates against the closed schema, and feeds it into build-feature.
System: Build proceeds; the buildfile's presentation target reflects the surface contract verbatim.

#### Branch: Project has only legacy `surface.md`

User: Loads a project that has `surface.md` and no `surface.yaml`.
System (background): Parses the markdown via the legacy parser into the same in-memory representation as the YAML form.
System: Build proceeds. The legacy parser remains active in v1 for back-compat. A note is logged: `surface.md is legacy migration input — run parlay migrate-spec to convert to surface.yaml`.

#### Branch: Both files present

User: Has both `surface.yaml` (recently authored) and `surface.md` (the legacy markdown the designer forgot to delete).
System (background): Detects both files, prefers the YAML form.
System: Emits `surface-md-superseded` as a warning naming both files. Fix message: `delete surface.md once you have confirmed surface.yaml carries the same content; the markdown is no longer consulted by validators`. Build proceeds against the YAML.

#### Branch: Designer runs the migrator

User: Runs `parlay migrate-spec` on a project with `surface.md` files in multiple features.
System (background): Walks each feature's `surface.md`, parses it via the legacy parser, emits `surface.yaml` alongside it, and writes a per-feature migration report listing any free-text content that did not have a closed-schema destination.
System: Reports the count of features migrated, files written, and unrouted free-text fragments. The original `surface.md` files are left in place; deletion is the designer's call after reviewing the migration report.

#### Branch: Migrator is idempotent

User: Runs `parlay migrate-spec` a second time on a project that has already been migrated.
System (background): Walks the source tree; finds `surface.yaml` next to each `surface.md`. The migrator's idempotency rule short-circuits when the YAML is up to date with the markdown's parsed model.
System: Reports "no changes needed" and exits 0. No file is rewritten.

#### Branch: Free-text content has no schema destination

User: Has a `surface.md` with a paragraph describing internal design rationale that does not map to any closed-schema field.
System (background): The migrator emits `surface.yaml` with the closed-schema content and writes an entry in the migration report citing the paragraph's line range and a "no destination" classification.
System: Reports the unrouted content. Designer decides whether to delete, move to a separate design-notes file, or keep as a comment in the `surface.md` (which becomes inert documentation).

---

### Blueprint: scope, override precedence, and strategy selection

**Trigger**: `parlay build-feature` reads `blueprint.yaml` and resolves layered settings against the adapter-set and per-adapter defaults.

User: Authors `blueprint.yaml` with `data: {fetching: stale-while-revalidate, caching: per-route}`, `auth: {strategy: jwt}`, `errors: {retry: writes}`. Each strategy is supported by the relevant adapter.
System (background): For each setting, walks the precedence chain `blueprint > adapter-set > adapter default`, resolves the effective value, and validates that the chosen strategy appears in the corresponding adapter's `supports` (or strategy-equivalent declaration).
System: Build proceeds. The resolved-value report attributes each setting to its source layer.

#### Branch: Blueprint chooses an unsupported strategy

User: Sets `data.fetching: stale-while-revalidate` against a presentation adapter that supports only `[on-mount, prefetch]`.
System: Emits `blueprint-strategy-unsupported` naming the choice (`stale-while-revalidate`), the adapter (`react-antd`), and the adapter's declared support list. Fix message: `pick a strategy the adapter supports, or use an adapter that implements stale-while-revalidate`.

#### Branch: Blueprint sets a value outside the closed strategy vocabulary

User: Sets `data.fetching: telepathy`.
System: Emits `blueprint-strategy-unknown` naming the field (`data.fetching`) and the closed list (`on-mount | prefetch | stale-while-revalidate | graphql`). Fix message: `data.fetching uses a closed vocabulary; pick from the listed values`.

#### Branch: Blueprint attempts to declare topology

User: Adds a `targets:` block to `blueprint.yaml`.
System: Emits `blueprint-topology-not-allowed`. Fix message: `target topology is owned by .parlay/adapter-set.yaml; blueprint declares cross-cutting policy only`.

#### Branch: Canonical error has no mapping at any level

User: Authors an operation declaring `errors: [server-error]`. Neither the application adapter, the adapter-set, nor blueprint maps `server-error` to a transport response or a presentation surface.
System (background): Walks the error-mapping precedence chain; finds no mapping at any level.
System: Emits `error-no-mapping` naming the operation, the error, and the missing layer (transport or presentation, depending on which mapping is absent). Fix message: `add a mapping in adapter-set or blueprint, or implement it in the relevant adapter's defaults`.

#### Branch: No blueprint at all

User: Has a project without `blueprint.yaml`.
System (background): The precedence chain falls through to adapter-set, then to adapter defaults. Every setting resolves to a default.
System: Validation passes. The resolved-value report attributes every setting to the adapter (the default layer).

#### Branch: Blueprint key outside the owned scope

User: Sets a setting under `blueprint.deployment.region: us-west-2`.
System (background): `deployment` is not in blueprint's owned scope (data/auth/errors/state/navigation/platform).
System: Emits `blueprint-scope-violation` naming the offending path. Fix message: `blueprint owns only the listed scopes; deployment-related concerns belong elsewhere`.

---

### Multi-target buildfile: operations, targets, and target-aware plan

**Trigger**: `parlay build-feature` emits or re-validates a buildfile.

User: Builds a feature with `capabilities.yaml` declaring `task.create` and a project with the four-kind adapter-set. The emitted buildfile has `operations: {'@task-list/operation:task.create': {kind: command, subject: ..., input: ..., output: ..., errors: [...], policies: [...], steps: [...]}}` declared once, plus `targets.presentation`, `targets.transport`, `targets.application`, `targets.persistence` each carrying only projection metadata.
System (background): Validates that canonical fields appear under `operations:` only, that target sections carry only their kind's projection shape, that every operation reference under `targets:` resolves to a key under `operations:`, and that every layout `wiring`/`bindings` ref likewise resolves.
System: Validation passes; the buildfile is ready for codegen.

#### Branch: Target restates a canonical field

User: Authors `targets.application` with an `errors:` field repeating the canonical errors from `operations:`.
System: Emits `buildfile-target-restates-canonical` naming the target `application`, the operation, and the offending field `errors`. Fix message: `canonical fields appear under operations: only — delete the duplicate from the target section`.

#### Branch: Binding references an unknown operation

User: A `bindings:` rule names `@task-list/operation:task.archive`, but `operations:` has no key by that name.
System: Emits `buildfile-binding-operation-missing` naming the binding and the missing ref. Fix message: `every binding ref must resolve to a key under operations: — fix the ref or add the operation to capabilities.yaml`.

#### Branch: Target effect references an unknown operation

User: A presentation `effect.operation` names `@task-list/operation:nonexistent`.
System: Emits `buildfile-target-operation-missing` naming the target, the component, and the missing ref.

#### Branch: Plan is target-aware

User: Builds a feature whose plan creates `apps/web/src/.../TaskCreateForm.tsx` (presentation) and modifies `apps/api/prisma/schema.prisma` (persistence).
System (background): Emits `plan.targets.presentation.creates: [...]` and `plan.targets.persistence.modifies: [...]` rather than a single top-level `plan.creates`/`plan.modifies`.
System: Validation passes. Codegen routes each plan row to the correct target during emission.

#### Branch: Bindings and targets share canonical refs

User: Renames `@task-list/operation:task.create` to `@task-list/operation:task.add` in `operations:` but forgets to update a `bindings:` rule.
System: Emits `buildfile-binding-operation-missing` naming the stale binding ref. Fix message: `bindings and targets share canonical operation refs; renaming a key under operations: requires updating every consumer`.

---

### Legacy buildfile fields: stay, deprecate, or repurpose

**Trigger**: `parlay build-feature` loads an existing buildfile and detects pre-multi-target shape during normalization.

User: Has a buildfile from before this feature with top-level `adapter: react-antd`, top-level `components: {...}`, top-level `routes: [...]`, and top-level `plan.creates: [...]`.
System (background): Walks the buildfile, identifies legacy fields, applies the disposition table:
- `adapter: react-antd` → emits an `adapter-set` reference plus `targets.presentation.adapter: react-antd`
- `components: {...}` → relocates under `targets.presentation.components`
- `routes: [...]` → relocates under `targets.presentation.routes`
- `plan.creates: [...]` → relocates under `plan.targets.presentation.creates`
System: Surfaces the diff for designer review. After confirmation, writes the normalized buildfile back to disk. The result is functionally equivalent to the original for a presentation-only project.

#### Branch: Components are double-declared

User: Has a buildfile with both top-level `components:` and `targets.presentation.components:` populated.
System: Emits `buildfile-components-double-declared`. Fix message: `pick one — top-level components: is legacy and should be removed once targets.presentation.components: is populated`. Build halts until resolved.

#### Branch: Legacy `models:` block is populated

User: Has a buildfile with non-empty `models: {...}`.
System (authoring mode): Warns with `buildfile-models-deprecated` and points at `domain-model.yaml`. Build-feature does not consume the entries for routing or codegen.
System (build mode): Errors with the same code. Fix message: `move entity declarations to domain-model.yaml; per-feature model duplication is dropped`.

#### Branch: Routes are ambiguous between presentation and transport

User: Has a buildfile with top-level `routes: [{path: /tasks}]` while the project's adapter-set declares a transport target with explicit HTTP exposure for `/tasks`.
System (background): Cannot mechanically choose between `targets.presentation.routes` and `targets.transport.routes` without designer input.
System: Emits `buildfile-routes-ambiguous` naming the path and the candidate targets. Fix message: `pick the target explicitly — declare the route under targets.presentation.routes for client-side routing, or under targets.transport for HTTP exposure`.

#### Branch: Legacy adapter field still parses (sunset path)

User: Has a buildfile with top-level `adapter: react-antd` and no `adapter-set` reference.
System (background): The legacy field is still parseable in v1 (its outright removal is owned by a separate deprecation feature scheduled for a later version). The normalization branch in build-feature converts the legacy field to a single-target presentation adapter set.
System: After normalization, the buildfile carries an `adapter-set` reference and `targets.presentation.adapter: react-antd`. The legacy `adapter:` line is removed from the rewritten file.

#### Branch: Wiring/bindings are preserved unchanged

User: Has a buildfile with `wiring.rules:` and `bindings:` sections from the layout-aware-build feature.
System (background): These fields are explicitly out of scope for migration churn; the normalizer leaves them byte-equivalent.
System: A diff between the pre-normalization and post-normalization buildfile shows zero changes inside `wiring.rules:` and `bindings:`. Their canonical operation refs continue to resolve against `operations:`.

---

### testcases.yaml v2: discriminated suite kinds and source_refs

**Trigger**: `parlay build-feature` emits or re-validates `testcases.yaml`.

User: Has a feature with `capabilities.yaml` declaring `task.create` and a presentation surface fragment `TaskCreateForm`. The emitted `testcases.yaml` is `schema_version: 2` with one `kind: presentation` suite (rendering the form) and one `kind: operation` suite (asserting `task.create` succeeds and rejects empty input).
System (background): Validates each suite's kind, confirms the operation suite's `act.operation` resolves to a canonical key, and walks every suite's `source_refs` to confirm they point at real surface fragments and capability operations.
System: Validation passes. The buildfile and testcases are ready for the coverage-review gate.

#### Branch: Operation has no covering suite

User: Adds `task.delete` to `capabilities.yaml` but does not author a covering testcase suite.
System: Emits `testcases-operation-uncovered` naming the operation `@task-list/operation:task.delete`. Fix message: `every canonical operation requires at least one kind: operation suite — add one, or grant an exemption in coverage-review.yaml`.

#### Branch: New v2 suite missing `source_refs`

User: Authors a new `kind: operation` suite without `source_refs:`.
System: Emits `testcases-source-refs-missing`. Fix message: `every new v2 suite cites at least one source_refs entry tying it to surface or capabilities; operation suites always require source_refs in build mode`.

#### Branch: Unknown suite kind

User: Authors a suite with `kind: integration`.
System: Emits `testcases-suite-kind-unknown` naming `integration` and the v2 set `[presentation, operation]`. Fix message: `v1 supports two suite kinds; integration is not a recognized value`.

#### Branch: Legacy v1 suite loaded as v2 presentation

User: Has a `testcases.yaml` with `schema_version: 1` and a presentation suite carrying an `intent:` string.
System (background): Loads the file as v2; treats the suite as `kind: presentation`; auto-populates `source_refs[0]` from the legacy `intent` string so the migrated suite has provenance from day one.
System: In authoring mode, missing additional `source_refs` warn with `testcases-source-refs-missing-legacy` until the project regenerates v2 testcases. In build mode, the warning persists rather than failing the build, so the project's existing test surface remains intact.

#### Branch: Operation suite asserts a shape that contradicts the canonical operation

User: Authors a `kind: operation` suite asserting `output.entity: Project` while the canonical operation declares `output.entity: Task`.
System: Emits `testcases-operation-shape-mismatch` naming the suite, the field, and both shapes. Fix message: `operation-suite assertions must match the canonical output/error/persistence shapes — fix the suite or amend the canonical operation in capabilities.yaml`.

#### Branch: Presentation-only project requires no operation suites

User: Builds a feature without `capabilities.yaml` and with only `kind: presentation` suites.
System (background): The "every canonical operation has a covering suite" rule walks an empty operation set.
System: Validation passes. Operation-suite coverage rules are inapplicable.

---

### coverage-review.yaml gates codegen on human approval

**Trigger**: `parlay generate-code` reads `.parlay/build/<feature>/coverage-review.yaml`.

User: Has just built a feature whose `buildfile.yaml` and `testcases.yaml` are stable. Runs `parlay review-coverage` (or the equivalent designer-facing entry point) which presents the testcases, asks for approval, and on confirmation writes `.parlay/build/<feature>/coverage-review.yaml` with the current `buildfile_hash`, `testcases_hash`, the list of `approved_suites`, and an empty `exemptions:` block (since every required term has covering suites).
System (background): Computes the canonical-form hashes, records the reviewer identity, and writes the file.
User: Runs `parlay generate-code` for the same feature.
System (background): Reads the review file, recomputes both hashes against the current artifacts, confirms they match, and walks every required approval against `approved_suites`. All checks pass.
System: Codegen proceeds. The feature is generated through the layer pipeline.

#### Branch: Review file is missing

User: Runs `parlay generate-code` on a feature that has never been reviewed.
System: Emits `coverage-review-missing`. Fix message: `run parlay review-coverage to record approval before generating code; this is a workflow integrity gate, not a security boundary`.

#### Branch: Buildfile changed after review (stale hash)

User: Edits `buildfile.yaml` and runs `parlay generate-code` without re-reviewing.
System (background): Recomputes `buildfile_hash`; mismatch with the value recorded in the review file.
System: Emits `coverage-review-stale` naming `buildfile_hash` as the drifted hash. Fix message: `re-run parlay review-coverage — the buildfile changed since the last approval`.

#### Branch: A suite is unapproved

User: Adds a new operation suite, re-runs build-feature (which updates `testcases.yaml` and bumps `testcases_hash`), and then re-runs review but skips the new suite.
System: Emits `coverage-review-suite-unapproved` naming the unapproved suite. Fix message: `every suite present in testcases.yaml must be approved; either approve or remove the suite`.

#### Branch: A required term has no covering suite and no exemption

User: Has an operation declaring `errors: [server-error]` but neither a covering case nor an exemption.
System: Emits `coverage-review-uncovered` naming the operation, the term `error:server-error`, and the term kind `error`. Fix message: `add a case that asserts the error path, or grant an explicit exemption in exemptions: with a free-text reason`.

#### Branch: Exemption resolves a missing-coverage entry

User: Adds an `exemptions:` entry naming the suite, the item `error:server-error`, and the reason "framework-level 500 handling is covered by the transport adapter test suite".
System (background): The exemption is recorded against the testcases hash; if the testcases file changes, the exemption persists only as long as the hash matches.
System: Codegen proceeds.

#### Branch: Re-running build invalidates review

User: Runs `parlay build-feature` (perhaps after editing `surface.yaml`).
System (background): The new buildfile and testcases get fresh canonical hashes that do not match the review file.
System: The next `parlay generate-code` fails with `coverage-review-stale` until the review is re-recorded. Re-running build is the correct trigger for re-review.

---

### Codegen flow: ordered layer generation and fixed read-set

**Trigger**: `parlay generate-code <feature>` is invoked.

User: Runs `parlay generate-code task-list` on a project with the four-kind adapter-set, a passing review file, and matching hashes.
System (background): Computes the allowed input set: `.parlay/build/task-list/{buildfile, testcases, coverage-review}`, `.parlay/adapter-set.yaml`, the four referenced adapter files, `blueprint.yaml`, `config.yaml`, `domain-model.yaml`, and the source tree under each target's root. Refuses to read anything outside this set.
System (background): Begins emission in the default order — persistence first. After persistence completes, application generation runs with the freshly-emitted persistence schema in its prompt context. Transport then runs against the application handlers; presentation runs against transport.
System: Reports the layers generated, the files written/modified, and a hash summary per generated path. Re-running the same command produces source bytes that may differ but pass the same testcase suite.

#### Branch: Codegen attempts to read from spec/intents

User: Has a custom adapter that, in its prompt assembly, attempts to read `spec/intents/<feature>/capabilities.yaml`.
System (background): Instrumentation traps the read attempt.
System: Fails the run with `codegen-spec-read-forbidden` naming the path. Fix message: `spec/intents is off-limits at codegen time; the buildfile is the executable contract — anything codegen needs must be there`.

#### Branch: Codegen attempts to read outside an adapter's declared root

User: Has an adapter whose prompt assembly reads `apps/web/...` while the presentation target's declared root is `apps/dashboard/...`.
System: Fails with `codegen-input-out-of-scope` naming the path. Fix message: `each adapter reads only under its target's root; either fix the path or update the target's root in adapter-set.yaml`.

#### Branch: Layer-informs-next visible in prompt context

User: Edits a persistence shape (e.g. adds a `priority` column to `Task`) and re-runs generate-code.
System (background): Persistence regenerates with the new column. Application's prompt context now includes the new persistence schema; the application handler generation responds (the AI sees the `priority` column and emits handler code that reads/writes it).
System: The application layer's output reflects the persistence change without the designer touching the application target. Trace inspection of the prompt assembly confirms the persistence outputs are present in the application prompt's context section.

#### Branch: Missing review halts codegen before reading anything else

User: Runs `parlay generate-code` on a feature with no `coverage-review.yaml`.
System (background): The review-file check runs before any input read.
System: Fails with `coverage-review-missing` and emits zero files. The "fixed read-set" rule is effectively shorter when codegen exits before the layered pipeline begins.

#### Branch: Behavioral conformance verified, source bytes differ

User: Runs `parlay generate-code` twice on the same buildfile.
System (background): Two independent runs produce two different source-file byte streams (AI variation), but both pass the same testcase suite when run.
System: Both runs report success. CI runs the suite twice on independent regenerations to catch prompt-context flakiness; suite divergence is the failure signal, not byte divergence.

#### Branch: Generated-code drift detection sees byte changes

User: Runs `parlay generate-code` on a known buildfile, then again, and compares per-path file hashes.
System (background): Generated-code hashes are tracked separately; bytes drift run-to-run; suite still passes.
System: Reports the drift in a separate ownership/drift surface. The drift alone is not a failure when the suite passes, but the report is available for reviewers who care about determinism for a given path.

---

### Validation modes: authoring vs build

**Trigger**: A validator entry point is invoked. The mode is determined by the entry point, not by a flag.

User: Runs `parlay validate` on a project mid-edit (authoring mode).
System (background): Walks every multi-target rule. For rules whose schema severity declares "authoring: warning, build: error", emits warnings on violations and continues.
System: Returns "valid with warnings" with one entry per violation. The project remains editable; the designer sees what the build will reject.

User: Runs `parlay build-feature` on the same project (build mode).
System (background): Walks the same rules with the same logic but uses the build-mode severity from each rule's schema.
System: Fails with errors for every violation that the previous `parlay validate` flagged as a warning. The error codes are identical; only severity differs.

#### Branch: A rule with no per-mode severity declaration

User: Lands a new rule and forgets to declare its per-mode severity.
System (background): The rule defaults to error in both modes (no "warnings-only" rule exists).
System: Both authoring and build mode error on violations. The schema-extension test in CI flags rules without explicit severity declarations so they get explicit handling before the next ship.

#### Branch: Authoring mode does not silently pass a build-mode failure

User: Has a project with a `domain-model.operations` populated.
System (authoring): Returns "valid with warnings" naming `domain-operations-deprecated`.
System (build): Errors with the same code.
System: The same designer running `parlay validate` locally sees every issue that build will reject; nothing is hidden by mode.

#### Branch: Build mode never downgrades errors

User: Runs `parlay build-feature` against a feature with a missing `source_refs` on a new v2 suite.
System (background): The rule's build-mode severity is "error".
System: Errors with `testcases-source-refs-missing`. Build does not produce a partial buildfile; partial success is not a build outcome.

#### Branch: Same code, both modes

User: Compares the warning surface of `parlay validate` against the error surface of `parlay build-feature`.
System: For every rule whose severity differs by mode, the error code is the same; only severity differs. Designer-facing tools can dedupe by error code without worrying about mode-specific code variants.

---

### Migration of legacy artifacts to the new shape

**Trigger**: Designer runs one or more of `parlay migrate-config`, `parlay migrate-spec`, `parlay migrate-capabilities`, `parlay migrate-domain-operations`, or `parlay build-feature` (which performs in-process buildfile normalization).

User: Has a pre-multi-target project with `prototype-framework: react`, `surface.md` and `infrastructure.md` per feature, populated `domain-model.operations`, and existing buildfiles using top-level `adapter:`/`components:`/`routes:`/`plan.creates`.
User: Runs `parlay migrate-config`.
System (background): Reads `prototype-framework: react`; emits `.parlay/adapter-set.yaml` with only the presentation slot filled, citing the matching presentation adapter (`react-antd` or whichever was implied).
System: Reports the conversion. The legacy `prototype-framework` field is left in place with a `prototype-framework-deprecated` warning until the dedicated deprecation feature removes it.

User: Runs `parlay migrate-spec`.
System (background): Walks each feature's `surface.md`, parses via the legacy parser, emits `surface.yaml`, writes a per-feature migration report listing any unrouted free text.
System: Reports the migrated features.

User: Runs `parlay migrate-capabilities`.
System (background): Walks each feature's `infrastructure.md`. Auto-converts operation-shaped fragments into closed operations under `kind: <best-guess>` (interactive prompt when ambiguous). Pattern-shaped fragments get classified into the migration report with suggested destinations (see "Pattern-fragment decomposition during capabilities migration").
System: Reports operations migrated, fragments routed, fragments left for designer review.

User: Runs `parlay migrate-domain-operations`.
System (background): Walks `domain-model.operations[*]`; for each entry, asks "which feature owns this operation?" via AskUserQuestion when ambiguous; writes stubs with `kind: unknown` into the chosen feature's `capabilities.yaml`; empties the legacy field.
System: Reports stubs written.

User: Runs `parlay build-feature task-list` (which normalizes the buildfile in-process).
System (background): Walks the legacy buildfile shape; emits the new target-aware shape; surfaces a designer-visible diff for each non-trivial relocation.
System: After designer confirmation, writes the normalized buildfile.

#### Branch: Adapter file has no `kind:` (no migration needed at file-level)

User: Has an adapter file from before this feature with no `kind:` field. Does not run any migrator yet.
System (background): The validator treats the missing field as `kind: presentation` at parse time. No file rewrite happens.
System: Validation passes; the project is functionally migrated for this artifact. `parlay upgrade` later offers to write the explicit field on opt-in.

#### Branch: Legacy v1 testcases load with auto-populated source_refs[0]

User: Has a `testcases.yaml` with `schema_version: 1` and presentation suites carrying `intent:` strings.
System (background): Loads each suite as v2 presentation; auto-populates `source_refs[0]` from the legacy `intent` string. Missing additional `source_refs` warn until the project regenerates v2 testcases.
System: The suites work without manual edits; the warning is the prompt to regenerate.

#### Branch: A migrator is idempotent

User: Runs `parlay migrate-spec` a second time on a project that has already been migrated.
System: Reports "no changes needed". No file is rewritten. The same applies to every other migrator: re-running on already-migrated input is a no-op.

#### Branch: Designer rejects a normalization decision

User: Runs `parlay build-feature` on a project whose legacy `routes:` block ambiguously could land in `targets.presentation` or `targets.transport` (the path collides with a transport HTTP exposure).
System (background): The normalizer emits the diff and asks the designer to disambiguate via AskUserQuestion.
System: If the designer cancels, the buildfile is left untouched and the build fails with `buildfile-routes-ambiguous`. If they pick a target, normalization proceeds.

#### Branch: Some migrations are scheduled separately

User: Asks "when does `prototype-framework` actually disappear?"
System: The field stays parseable in v1 (with deprecation warnings). Outright removal is owned by a separate deprecation feature scheduled for a later version. The same applies to the legacy top-level `adapter:` field on buildfiles, the `models:` field, and `domain-model.operations` — all are deprecated in v1 and removed by their respective deprecation features.

---

### Pattern-fragment decomposition during capabilities migration

**Trigger**: `parlay migrate-capabilities` walks legacy `infrastructure.md` content and the operation-shaped extractor leaves residue: paragraphs that describe engineering patterns rather than operations.

User: Runs `parlay migrate-capabilities` on a project whose `infrastructure.md` includes a paragraph describing a "router registry" (a slot to register handlers, a list, a lookup function), a paragraph describing a "validate-then-trim-then-uniqueness" pipeline, and a paragraph describing a "persistence cache" that warms up on startup.
System (background): The operation-shaped extractor consumes operation-shaped paragraphs and writes them to `capabilities.yaml`. The remaining pattern-shaped paragraphs go to the pattern-fragment classifier.
System (background): Classifier identifies:
- The router registry as **registry-shaped** with suggested destination "domain entity plus register/list/lookup operations"
- The pipeline as **pipeline-shaped** with suggested destination "command operation in capabilities"
- The cache as **cache-shaped** with suggested destination "cacheable policy plus blueprint.data.caching"
System: Writes a per-feature migration report listing each fragment by source line range, detected shape, and suggested destination. Does not write to `capabilities.yaml`, `domain-model.yaml`, or `blueprint.yaml` for any of these — designer review is required.

#### Branch: Ambiguous fragment falls into "designer review"

User: Has a paragraph describing "a thing that watches the database and notifies the UI when something changes" — could be a subscription (v2-deferred), a hook, or a polling loop.
System (background): The classifier is conservative; it cannot confidently route the fragment.
System: The migration report quotes the fragment verbatim under "unrouted; designer review" with a note: `the classifier could not confidently determine the shape — manual review required`. No suggested destination is offered.

#### Branch: V2-deferred fragment is preserved

User: Has a paragraph describing a "subscription" that the system maintains.
System (background): The classifier identifies hook-shaped or subscription-shaped fragments and notes them as v2-deferred.
System: The report preserves the fragment verbatim and notes the v2 deferral. Nothing is written to `capabilities.yaml`. The project can revisit the fragment when v2 ships subscription support.

#### Branch: Helper/utility fragment routed to "adapter-level pattern"

User: Has a paragraph describing a "string trimmer that removes leading/trailing whitespace and collapses internal runs".
System (background): The classifier identifies the fragment as helper-shaped.
System: The report lists the fragment with destination "adapter-level pattern" and notes: `helper utilities are not spec-layer concerns; if this is genuinely useful, it lives in the adapter's code as a utility — do not wedge it into capabilities.yaml`.

#### Branch: The migrator does not write to suggested destinations

User: Has a registry-shaped fragment classified with suggested destination "domain entity plus register/list/lookup operations". The destination is a hint, not an action.
System (background): The migrator never edits `domain-model.yaml`, `capabilities.yaml`, or `blueprint.yaml` based on suggestions; it only writes the report.
System: The designer reads the report, edits the relevant artifacts manually (or scaffolds via other parlay commands), and runs the regular validation pipeline. The migrator's responsibility ends at the report.

#### Branch: Re-running on an unchanged input

User: Runs `parlay migrate-capabilities` a second time on the same `infrastructure.md`.
System: Produces an identical report. No `capabilities.yaml` was written by the previous pattern-fragment branch (operation-shaped fragments may have been written, but those are an idempotency concern of the operation-shaped extractor, not the pattern classifier).

---

### Presentation-only projects continue to work unchanged

**Trigger**: Any pipeline command runs against a project whose `.parlay/adapter-set.yaml` fills only the `presentation` slot (or a project that has not yet adopted multi-target machinery).

User: Has a presentation-only project (single React app). Runs `parlay add-feature task-list`, `parlay scaffold-dialogs`, `parlay create-artifacts`, `parlay build-feature`, `parlay generate-code` end-to-end.
System (background): Every multi-target rule short-circuits on filled-slot composition: `capabilities.yaml` is not required, backend `supports` checks have no adapters to consult, `links` enforcement walks zero edges, `coverage-review.yaml` is required only once the project has migrated to v2 testcases.
System: The pipeline runs to completion. The output is byte-equivalent to running the same pipeline before this feature shipped (modulo unrelated changes).

#### Branch: No `capabilities.yaml`, no operation suites required

User: Has a presentation-only feature with `surface.yaml` (or `surface.md`) and no `capabilities.yaml`.
System (background): The buildfile's `operations:` block is empty; the testcases-coverage rule walks zero canonical operations.
System: Validation and codegen pass. The project never authors an operation suite.

#### Branch: Legacy v1 testcases warn but do not block

User: Has a presentation-only project on `testcases.yaml` v1 without `source_refs:`.
System (background): Legacy suites load as v2 presentation; the auto-populated `source_refs[0]` provides minimal provenance; missing additional `source_refs` warn but do not fail.
System: Build succeeds with warnings until the project regenerates v2 testcases. The build-mode error severity from the testcases-v2 intent applies only to new or regenerated v2 suites.

#### Branch: No `coverage-review.yaml` required for legacy v1 presentation-only

User: Has a presentation-only project that has not migrated to v2 testcases. Runs `parlay generate-code`.
System (background): The coverage-review gate is required for v2 testcases; legacy v1 presentation-only projects start with warnings until they regenerate.
System: Codegen succeeds. The warning surface includes `coverage-review-recommended` (informational) so the project knows what changes once it regenerates v2 testcases.

#### Branch: Adding the first non-presentation slot transitions the project to multi-target

User: Edits `.parlay/adapter-set.yaml` to add `targets.persistence: {adapter: prisma-postgres, root: apps/api}`.
System (background): The next `parlay build-feature` sees a non-presentation slot filled; multi-target rules now apply.
System: Build now requires `capabilities.yaml` (or proceeds if absent — that's still legal, but the build's surface widens), runs `supports` checks against the persistence adapter, enforces `links` for any cross-kind edges, and gates codegen on `coverage-review.yaml`. The transition is automatic; no explicit "migrate to multi-target" step is required.

#### Branch: Backwards compatibility is mechanical, not promised

User: Audits each multi-target error code introduced by this feature.
System (background): Every error code's rule definition includes a "presentation-only short-circuit" check — the rule consults the adapter-set's slot composition before applying.
System: A reviewer can mechanically confirm the back-compat contract by walking the rule definitions; nothing relies on a human-promised "we'll keep it working".

---
