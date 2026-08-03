# Multi-adapter

> Extend Parlay's adapter system from presentation-only to a multi-target, multi-kind model. A project registers adapters of four closed kinds — `presentation`, `transport`, `application`, `persistence` — through `.parlay/adapter-set.yaml`, with link rules enforcing cross-kind boundaries and per-adapter `supports` contracts that fail validation before the AI is invoked. Backend behavior gains a closed-vocabulary YAML artifact (`capabilities.yaml`) that supersedes and reframes the legacy `infrastructure.md`. The buildfile becomes multi-target: canonical operations are declared once and projected through each registered adapter, plan rows split by target, and existing layout `wiring`/`bindings` continue to coexist by sharing canonical operation refs. A `coverage-review.yaml` gate hash-binds human approval of testcases to codegen so the AI cannot grade its own homework. Presentation-only projects continue to validate and generate without any change.

---

## Adapter kinds and adapter-set topology

**Goal**: Establish a closed adapter-kind vocabulary (`presentation`, `transport`, `application`, `persistence`) and a project-level `.parlay/adapter-set.yaml` that registers at most one adapter per kind, each bound to a source root. Every adapter file declares its `kind:`; an existing adapter without that field defaults to `presentation` so legacy projects keep validating.

**Persona**: Parlay tool maintainer

**Priority**: P0

**Context**: Today an adapter is implicitly UI-only and a project binds to one adapter through legacy fields. To express backend codegen cleanly, an adapter has to identify *what kind of work it does* — UI rendering, API boundary, business logic, or persistence — and the project needs a topology file that says which kinds are filled and which source root each fills. A v1 cap of one target per kind keeps validation, routing, and codegen simple while leaving room for future multi-transport or multi-database projects.

**Action**: Add a required `kind:` field to the adapter-file schema, with the closed set `{presentation, transport, application, persistence}`. Add `.parlay/adapter-set.yaml` with required keys `name` and `targets`, where `targets` is a map keyed by kind whose entries are objects `{adapter, root}`. Treat an adapter file missing `kind:` as `kind: presentation`. Validation rejects (a) an adapter file whose `kind:` is outside the closed set, (b) an adapter set whose `targets:` keys an unknown kind, (c) an adapter set with more than one adapter under the same kind, (d) an adapter set whose `targets.<kind>.adapter` does not resolve under `.parlay/adapters/`, and (e) an adapter set whose declared slot kind does not match the referenced adapter's own `kind:` field.

**Objects**: adapter-kind, adapter-file, adapter-set-yaml, target-slot, source-root, kind-default-presentation

**Constraints**:
- The kind vocabulary is closed at `{presentation, transport, application, persistence}` for v1 — adding kinds is a schema change in a later version, not an authoring extension point
- Multiple adapters per kind are rejected in v1 with `adapter-set-duplicate-kind`; relaxing to multi-target is deferred to a later version
- An adapter file without an explicit `kind:` is treated as `kind: presentation` and validates clean — this is the migration default for already-shipped adapters
- A reserved `kind: <unknown>` value (anything outside the closed set) fails with `adapter-kind-unknown` naming the offending value and the closed set
- An adapter set entry that references a missing adapter file fails with `adapter-set-adapter-missing` naming the unresolved path under `.parlay/adapters/`
- An adapter set entry whose slot kind contradicts the referenced adapter's own `kind:` fails with `adapter-set-kind-mismatch` naming both the slot and the adapter's actual kind
- A project may declare any subset of the four kinds in its adapter set; missing slots are valid and impose no further constraints on the adapter set itself

**Verify**:
- An adapter file with `kind: persistence` validates; an adapter file with `kind: storage` fails with `adapter-kind-unknown`
- An adapter set with `targets.presentation = {adapter: react-antd, root: apps/web}` and `targets.persistence = {adapter: prisma-postgres, root: apps/api}` validates when both files exist with matching kinds
- An adapter set listing two different adapters under `targets.presentation` fails with `adapter-set-duplicate-kind`
- An adapter set referencing `targets.application = {adapter: nest-app}` while `nest-app.adapter.yaml` declares `kind: persistence` fails with `adapter-set-kind-mismatch` naming both `application` and `persistence`
- An adapter file with no `kind:` field validates and is treated as `kind: presentation` — confirmed by it being acceptable in `targets.presentation`
- An adapter set with only the `presentation` slot filled validates clean

---

## Adapter-set links enforce cross-kind boundaries

**Goal**: Make the adapter set's `links:` block enforceable rather than decorative. Cross-kind access outside the declared link relations fails validation. Presentation may not call application or persistence directly; transport may not access persistence; an unfilled slot imposes no rule on its absent edges.

**Persona**: Parlay tool maintainer

**Priority**: P0

**Context**: Without enforced links, the multi-kind topology degrades into prose. Linkable relations such as `calls`, `dispatches`, and `persists` define the legitimate code paths between layers; cross-kind drift (a presentation component importing a persistence repository) is exactly the architectural failure this feature exists to prevent. The buildfile already encodes target-to-target invocations through canonical operation refs, so the validator can mechanically check edges against the declared link set.

**Action**: Extend `.parlay/adapter-set.yaml` with an optional `links:` list whose entries are `{from: <kind>, relation: <verb>, to: <kind>}`. Define a closed v1 link-relation vocabulary `{calls, dispatches, persists}`. The buildfile validator walks every cross-kind reference recorded in `targets:` (presentation actions invoking transport operations, transport handlers dispatching to application, application steps persisting through persistence, etc.) and rejects any edge whose `(from-kind, to-kind)` pair is not present in `links:` with a permitting relation. Link rules apply only to filled slots; an edge whose `to` slot is unfilled fails earlier as an unresolved target.

**Objects**: adapter-set-links, link-relation, cross-kind-edge, target-reference, layer-boundary

**Constraints**:
- The link-relation vocabulary `{calls, dispatches, persists}` is closed in v1 — additional verbs require a schema change
- A `links:` block is optional; without it, the validator treats only edges within a single kind as legal and rejects every cross-kind edge with `adapter-set-link-missing`
- A cross-kind edge that violates the declared links fails with `adapter-set-link-violated` naming the source slot, the target slot, and the relation that would have permitted the edge
- A `links:` entry whose `from` or `to` kind is not declared in `targets:` fails with `adapter-set-link-unfilled-slot` (links may not anticipate kinds the project did not register)
- Link enforcement runs in build mode; authoring mode warns instead of failing so partially-authored adapter sets remain editable
- An edge whose `to:` is not the recipient of any operation reference (i.e. nothing actually invokes it) does not fail link validation — links rule on observed edges, not theoretical ones
- Presentation-only projects with no other slots filled have no cross-kind edges and therefore no link rules to enforce

**Verify**:
- An adapter set with full four-kind topology and `links: [{from: presentation, relation: calls, to: transport}, {from: transport, relation: dispatches, to: application}, {from: application, relation: persists, to: persistence}]` validates a buildfile whose presentation actions call transport operations
- A buildfile in the same project where a presentation component directly references a persistence repository fails with `adapter-set-link-violated` naming `presentation`, `persistence`, and the missing relation
- An adapter set without `links:` rejects any cross-kind edge with `adapter-set-link-missing`
- A `links:` entry naming `to: transport` in a project whose `targets.transport` is unfilled fails with `adapter-set-link-unfilled-slot`
- A presentation-only adapter set generates a buildfile with no cross-kind edges and validates clean even with `links:` absent

---

## Adapter `supports` contract gates codegen pre-AI

**Goal**: Each non-presentation adapter declares the closed-vocabulary terms **its own layer** implements — operation kinds, steps, errors, and policies — so build validation can fail *before the AI is invoked* when a feature requires a term **no filled backend layer** supports. Coverage is a **union** across the filled backend slots, not an intersection: an adapter lists only its layer's terms (persistence owns the data steps; application owns validation/authorization/return shaping), and a term passes if any backend adapter owns it. Presentation adapters keep their existing surface-vocabulary contract; backend adapters add the new declarations.

**Persona**: Parlay tool maintainer

**Priority**: P0

**Context**: AI-driven codegen has no useful failure mode for "no layer in your stack implements `read-tree`" — it will improvise something that compiles and passes shallow tests. The cure is to refuse generation up front. Each adapter therefore enumerates which terms *its layer* implements; build-feature fails when the resolved feature uses a term that **no filled backend adapter** owns. Listing per-layer (honest listings) is what makes the union gate correct: because each step is owned by exactly one layer, a term no filled layer owns is supported by nobody and fails, while a term one layer legitimately owns passes even though a sibling backend adapter does not list it. The pattern descriptions for each supported term live alongside `supports:` and feed the AI prompt at generation time, but the *gate itself* is mechanical.

**Action**: Add a `supports:` block to the adapter-file schema with sub-keys `operation_kinds`, `steps`, `policies`, and `errors`, each a list drawn from the closed backend vocabulary. During build-feature the validator runs (a) a per-adapter shape/vocabulary check on each filled slot, then (b) a union coverage check: for every operation in the resolved `capabilities.yaml`, each term (operation kind, every step type, every policy, every error) must appear in the `supports:` of **at least one** filled backend adapter. A term supported by no backend adapter fails build with a stable error code naming the term, the term kind, the filled backend slots, and the operation that surfaced it. Pattern descriptions for supported terms are authored alongside `supports:` in the same adapter file; they are AI prompt material, not validator input.

**Objects**: adapter-supports, operation-kinds, steps-vocabulary, policies-vocabulary, errors-vocabulary, pattern-descriptions, pre-codegen-gate

**Constraints**:
- The four closed vocabularies (`operation_kinds`, `steps`, `policies`, `errors`) are owned by Parlay; v1 fixes their starter sets and adapters declare which terms they implement, never which terms exist
- An adapter that declares a term not in its kind's vocabulary fails with `adapter-supports-unknown-term` naming the term and the closed list — this prevents adapter authors from inventing private vocabulary that codegen will silently honor
- A feature whose resolved operations use a term not declared in the relevant adapter's `supports` fails with one of `adapter-supports-missing-operation-kind`, `adapter-supports-missing-step`, `adapter-supports-missing-policy`, `adapter-supports-missing-error`, naming the offending operation, the term, and the adapter
- Validation runs at build-feature time (after `capabilities.yaml` is resolved into the buildfile, before generate-code is invoked) — failures here block both build and codegen
- Pattern descriptions are stored next to `supports:` but are not consumed by validation; they are only read at codegen time as part of the prompt assembly
- Presentation adapters retain their existing surface-vocabulary contract; the new `supports:` shape applies to non-presentation kinds (`transport`, `application`, `persistence`) — a presentation adapter may omit it
- The closed vocabulary lists themselves live in adapter-side schema files (the Parlay tool ships these as the source of truth); adapters cannot extend the lists, only opt in to subsets

**Verify**:
- An application adapter declaring `supports.steps: [validate-input, create-one, return-one]` validates; declaring `supports.steps: [foo-bar]` fails with `adapter-supports-unknown-term`
- A feature whose `capabilities.yaml` operation uses step `read-tree` against an application adapter that omits `read-tree` from `supports.steps` fails build with `adapter-supports-missing-step` naming the operation, the step, and the adapter
- The same project, after the operation is rewritten to use only supported steps, builds clean
- A feature using policy `transaction-required` against an adapter whose `supports.policies` omits it fails with `adapter-supports-missing-policy`
- A presentation-only project with no non-presentation adapters skips backend `supports` checks entirely (no `capabilities.yaml`, nothing to validate against)
- Pattern descriptions present in an adapter file but never matched by an operation cause no validation activity (they are AI prompt material, not contracts)

---

## V1 closed vocabularies and v2 deferrals

**Goal**: Pin the v1 contents of every closed vocabulary the supports contract and `capabilities.yaml` reference, so adapters know exactly which terms they may opt into and designers know exactly which terms they may use. Make the v2-deferred set equally explicit: subscriptions and jobs (as operation kinds) and any additional steps, errors, or policies are out of v1 scope and require both a schema bump and at least one adapter declaring support to ship later.

**Persona**: Parlay tool maintainer

**Priority**: P0

**Context**: The supports contract validates feature usage against adapter-declared subsets of four closed vocabularies — operation kinds, steps, errors, and policies — but those vocabularies are only useful if their *contents* are pinned. Without a single source of truth for what's in v1, schema docs, adapter authors, designer-facing tooling, and the validator each have to re-derive the lists, and they will drift. A separate intent fixes the v1 lists so every downstream rule shares one definition. Subscriptions and jobs are explicitly deferred so a designer who reaches for them gets a clear "v2-deferred" message instead of an unexplained "unknown term".

**Action**: Define the v1 closed-vocabulary contents:
- **operation_kinds** — `command`, `query`
- **steps (write group)** — `validate-input`, `authorize`, `create-one`, `update-one`, `delete-one`
- **steps (read group)** — `read-one`, `read-many`, `search`
- **steps (return group)** — `return-one`, `return-many`, `return-empty`
- **errors** — `validation-failed`, `unauthorized`, `forbidden`, `not-found`, `conflict`, `server-error`
- **policies** — `auth-required`, `permission-required`, `transaction-required`

These lists live in dedicated schema files under `.parlay/schemas/` (one per vocabulary), are owned by Parlay, and are the single source of truth referenced by the supports contract, capabilities validation, and adapter authoring. Subscriptions and jobs are explicitly excluded from `operation_kinds` in v1; using either fails with `capabilities-unknown-term` whose fix message names them as v2-deferred. Additional terms in any vocabulary follow the same gate in later versions: the schema gains the term first, then at least one adapter must declare support before any feature may use it.

**Objects**: v1-vocabulary-contents, operation-kinds-v1, steps-v1, errors-v1, policies-v1, v2-deferred, term-extension-gate

**Constraints**:
- Each closed vocabulary lives in its own schema file (`.parlay/schemas/operation-kinds.schema.md`, `.parlay/schemas/steps.schema.md`, `.parlay/schemas/errors.schema.md`, `.parlay/schemas/policies.schema.md`) so adapter authors and validators consult one canonical list per vocabulary
- Adapters cannot extend the lists; they only opt into subsets via `supports`
- A `capabilities.yaml` operation with `kind: subscription` or `kind: job` fails with `capabilities-unknown-term` whose fix message explicitly says "deferred to v2" rather than just "not in the closed list"
- Adding a term to any vocabulary in a later version is a two-gate change: schema PR adds the term, then at least one adapter declares support before any feature may use it; the second gate prevents shipping a vocabulary entry no adapter implements
- Step partitioning into write/read/return groups is informational (helps adapter authors organize support declarations); the validator treats all step terms as a flat namespace
- A schema doc that ships with v1 must enumerate exactly these terms — drift between the v1 schema files and this intent is itself a failure
- The list above is the v1 contract; later versions bump the closed-vocabulary schema versions when they extend the lists

**Verify**:
- Each of the four closed-vocabulary schema files enumerates exactly the terms named above and no others at the moment v1 ships
- An adapter declaring `supports.steps: [validate-input, create-one, return-one]` validates against the v1 vocabulary; declaring `supports.steps: [telepathy]` fails with `adapter-supports-unknown-term` naming `telepathy` and the v1 step list
- A `capabilities.yaml` operation with `kind: subscription` fails with `capabilities-unknown-term` and a fix message that names subscription as v2-deferred (not just "not in the list")
- An operation declaring `policies: [transaction-required]` validates; declaring `policies: [eventual-consistency]` fails with `capabilities-unknown-term`
- A schema-docs build that omits any v1 term from its closed-vocabulary tables fails the docs check that compares schema files to this intent's vocabulary list
- A simulated v2 extension PR that adds a new step term but ships zero adapters declaring support for it fails the schema-extension gate

---

## Bundled adapter-set presets

**Goal**: Ship a small set of bundled `.parlay/adapter-set.yaml` presets covering common stacks, plus name the v1 first preset that the tool exercises end-to-end in CI. Custom adapter-sets remain fully supported; presets are starting templates, not gatekeepers.

**Persona**: Project owner setting up a new Parlay project

**Priority**: P1

**Context**: Authoring an adapter-set from scratch is a small but real friction for new projects, especially because kinds, links, and adapter-file references are all first contact at once. Bundled presets reduce setup to "pick a name" for common stacks. Naming the v1 first preset specifically pins the stack the tool's reference implementation, examples, and integration tests target — the rest ship alongside but only one is exercised end-to-end in v1.

**Action**: Ship four bundled adapter-set presets as starting templates inside the tool, each consisting of a `.parlay/adapter-set.yaml` plus the adapter files it references:
- **`react-antd-only`** — presentation only (react-antd)
- **`angular-clarity-only`** — presentation only (angular-clarity)
- **`react-nest-prisma`** — full four-kind stack (react-antd + openapi-rest + nestjs-application + prisma-postgres)
- **`angular-nest-prisma`** — full four-kind stack (angular-clarity + openapi-rest + nestjs-application + prisma-postgres)

Designate `react-nest-prisma` as the **v1 first preset**: the stack the tool's reference implementation, examples, and integration tests target end-to-end. The other three presets ship but are exercised by smaller test surfaces in v1.

`parlay init` (or the equivalent project-setup entry point) prompts the user with the four named presets plus a `custom` option. Choosing a preset copies the corresponding adapter-set and adapter files into the project. Choosing `custom` leaves the project to author its own adapter-set from scratch. After setup, the project's adapter-set is editable like any other file; presets are not enforced and projects may diverge over time without re-validation churn.

**Objects**: bundled-presets, react-nest-prisma-first-preset, adapter-set-template, custom-mode, parlay-init

**Constraints**:
- Presets are templates copied at setup time; they are not enforced afterward
- Preset names are a public contract — renaming requires a deprecation/alias path, not a silent change
- Each preset's `.parlay/adapter-set.yaml` references only adapter files that the tool also bundles; no preset references a file that does not ship
- The v1 first preset (`react-nest-prisma`) is exercised by full end-to-end integration tests in v1 CI; other presets are exercised by smaller smoke tests
- `custom` is always offered alongside the named presets; the setup flow does not force preset adoption
- A project that diverges from its starting preset is fully supported; there is no "preset compliance" check after setup
- Adding a new preset later requires shipping both the adapter-set template and any new adapter templates it references in the same change

**Verify**:
- `parlay init` lists exactly `react-antd-only`, `angular-clarity-only`, `react-nest-prisma`, `angular-nest-prisma`, and `custom` at v1 ship
- Choosing `react-nest-prisma` produces a `.parlay/adapter-set.yaml` with all four kinds filled and four corresponding adapter files copied into `.parlay/adapters/`
- Choosing `react-antd-only` produces a `.parlay/adapter-set.yaml` with only the presentation slot filled and one adapter file
- A new project created from `react-nest-prisma` runs a feature end-to-end through `add-feature` → `build-feature` → `generate-code` in v1 CI without manual edits
- Choosing `custom` skips the preset copy and leaves the user to author `.parlay/adapter-set.yaml`
- A preset adapter-set referencing an adapter file the tool does not bundle fails the preset-completeness check at build time

---

## capabilities.yaml replaces infrastructure as the closed-vocabulary backend artifact

**Goal**: Replace the legacy `infrastructure.md` artifact with `capabilities.yaml` — a per-feature, YAML, closed-vocabulary description of backend operations. Capabilities is to backend code generation what `surface.yaml` is to UI generation: the feature-local contract that downstream adapters consume. Operation IDs are feature-local and normalized to a globally-unique form `@<feature>/operation:<id>` during build.

**Persona**: UX Designer authoring backend behavior

**Priority**: P0

**Context**: The original purpose of `infrastructure` was to describe what the system does behind the user's actions — load, validate, persist, return. In practice, existing `infrastructure.md` files drifted into describing engineering patterns (registries, dispatchers, pipelines), which downstream codegen could not consume reliably. Renaming to `capabilities` recovers the original intent, gives the artifact a clearer name, and avoids adding a fourth spec artifact. YAML lets validators check terms against closed vocabularies without prose-parsing.

**Action**: Define `spec/intents/<feature>/capabilities.yaml` as the closed-vocabulary backend artifact. Schema: top-level `schema_version`, `feature`, and `operations:`. Each operation has `id` (feature-local), `kind` (drawn from `operation_kinds`), `subject`, `input`, `output`, `errors` (from the closed `errors` vocabulary), `policies` (from the closed `policies` vocabulary), and `steps` (each entry's `type` from the closed `steps` vocabulary, with kind-specific extra keys such as `entity` and `identity`). Build-feature normalizes operation IDs to `@<feature>/operation:<id>` and uses that form everywhere downstream — buildfile `operations:` keys, target references, testcase `act.operation`, and `source_refs`. Build-mode validation rejects prose-only fragments and any term outside its closed vocabulary.

**Objects**: capabilities-yaml, capability-operation, operation-id, operation-id-normalization, closed-vocabulary, infrastructure-rename

**Constraints**:
- `capabilities.yaml` is per-feature and lives next to `surface.yaml` and the legacy `dialogs.md` under `spec/intents/<feature>/`
- Operation IDs in the source file are feature-local strings (e.g. `task.create`); they are normalized to `@<feature>/operation:<id>` only on the way into the buildfile, so designer-facing files stay terse
- Every term used in `kind`, `errors`, `policies`, and `steps[*].type` must be present in the corresponding closed vocabulary; an unknown term fails with `capabilities-unknown-term` naming the field, the term, and the closed list
- Prose-only or partially-shaped operations are rejected in build mode with `capabilities-not-closed-form`; authoring mode permits them with a warning so designer drafts are editable
- A feature without backend behavior simply omits `capabilities.yaml`; that is valid and produces no operation entries in the buildfile
- The artifact is the *successor* of `infrastructure.md`, not an alias — `infrastructure.md` is a legacy migration input only (handled in the migration intent), not a parallel artifact
- Operation IDs are unique within their feature; `capabilities-duplicate-operation-id` fires on collisions inside one file

**Verify**:
- A feature with `capabilities.yaml` declaring `task.create` (kind `command`, steps validate-input + create-one + return-one) builds clean and shows up in the buildfile under key `@task-list/operation:task.create`
- The same file with a step `type: telepathy` fails build with `capabilities-unknown-term` naming the closed `steps` vocabulary
- A feature whose `capabilities.yaml` declares two operations with the same `id` fails with `capabilities-duplicate-operation-id`
- A feature with no `capabilities.yaml` builds clean and produces a buildfile whose `operations:` block is empty
- A `capabilities.yaml` consisting entirely of prose paragraphs (no closed shape) fails in build mode with `capabilities-not-closed-form` and warns in authoring mode
- Every reference to the operation downstream — buildfile target sections, testcases `act.operation`, coverage-review approvals — uses the normalized `@<feature>/operation:<id>` form, not the bare local id

---

## domain-model.yaml `operations:` field is deprecated in favor of capabilities

**Goal**: The `operations:` field in `domain-model.yaml` no longer carries authoritative behavior. It is marked deprecated, validation flags its use, and a migrator lifts existing entries into capability stubs for designer authoring. Domain-model retains its true scope: entities, relationships, states, enums, and value objects — what the data *is*, not what the system *does*.

**Persona**: UX Designer / Parlay tool maintainer

**Priority**: P1

**Context**: Existing domain-model files conflate the data model with system behavior, partly because there was no other home for operation-shaped fragments. With `capabilities.yaml` as the dedicated backend artifact, `domain-model.operations` is redundant and an invitation to drift between the two homes. The cleanest cut is to deprecate the field, surface the deprecation in validation, and migrate existing entries to capability stubs that designers can fill in.

**Action**: Mark `operations:` in the `domain-model.yaml` schema as deprecated. `parlay validate` emits `domain-operations-deprecated` for any project whose `domain-model.yaml` populates the field, with a fix message pointing at `capabilities.yaml`. Add a migrator entry point that lifts each `domain-model.operations[*]` into a stub entry inside the appropriate feature's `capabilities.yaml` with `kind: unknown` and prose carried over verbatim, so designer review can re-classify it under the closed vocabulary. The field itself remains parseable (so existing files still load) but build-feature stops consuming it for routing or codegen.

**Objects**: domain-model-operations-deprecated, domain-model-yaml, capability-stub, kind-unknown, migrate-domain-operations

**Constraints**:
- The field stays parseable in v1 — removing it is deferred to a later version so existing projects can validate while they migrate
- `parlay validate` returns `domain-operations-deprecated` as a warning in authoring mode and as an error in build mode; the build-mode error blocks regeneration until the field is empty or removed
- The migrator `parlay migrate-domain-operations` writes one stub per entry into a target feature's `capabilities.yaml`; choosing the target feature is interactive (prompted) when ambiguous and explicit (named) when only one feature owns the relevant entity
- Stubs land with `kind: unknown` and a prose-only `notes:` field so designer review can decide the real kind, steps, policies, and errors — the migrator does not fabricate closed-vocabulary terms
- Build-feature ignores `domain-model.operations` entirely for routing and codegen; the only path from those entries to the buildfile is via the migration step
- Domain-model retains full schema and behavior for entities, relationships, states, enums, and value objects — those are the model's actual scope and unaffected by this intent

**Verify**:
- A project whose `domain-model.yaml` lists three entries under `operations:` warns under authoring `parlay validate` and errors under build `parlay validate`
- After running `parlay migrate-domain-operations`, the same three entries appear as `kind: unknown` stubs in the chosen feature's `capabilities.yaml`, and the original `domain-model.yaml` either has its `operations:` cleared or carries a deprecation marker comment
- Build-feature on a project with non-empty `domain-model.operations` and an empty `capabilities.yaml` does not pull the legacy operations into the buildfile (the routing path is broken on purpose)
- Domain-model entities and relationships continue to drive entity resolution in the buildfile unchanged

---

## surface.yaml replaces surface.md as the closed presentation artifact format

**Goal**: Make YAML the target format for the presentation contract. `surface.yaml` is the closed presentation vocabulary; `surface.md` becomes a legacy migration input only. The artifact's content (closed-vocabulary fragments, actions, flows) is unchanged; only the serialization shifts.

**Persona**: UX Designer

**Priority**: P1

**Context**: Markdown was the historical author format because designers wrote prose-flavored fragments, but the closed vocabulary that build-feature consumes is structural, not narrative. YAML lets the schema be enforced directly without lossy parsing, and it aligns `surface` with the other two spec artifacts (`capabilities.yaml`, `domain-model.yaml`) that are also moving to YAML. Markdown remains appropriate for narrative documents (intents, dialogs, agent instructions, design notes), just not for machine-validated spec artifacts.

**Action**: Add `surface.yaml` to the spec layer as the target format. Authoring tools, validators, and build-feature consume `surface.yaml` directly. A `parlay migrate-spec` step converts existing `surface.md` files into `surface.yaml` by parsing the established markdown shape into the closed schema. Until migration runs, build-feature parses `surface.md` into the same in-memory representation so projects that have not yet migrated still build. After migration, the `.md` form is treated as documentation and is not consulted by validators.

**Objects**: surface-yaml, surface-markdown-legacy, migrate-spec, in-memory-surface-model

**Constraints**:
- `surface.yaml` is the canonical artifact going forward; `surface.md` is legacy migration input
- Both forms parse to the same in-memory model; the build pipeline does not branch on serialization, only on which file is present
- A feature with both `surface.yaml` and `surface.md` present prefers the YAML form and warns with `surface-md-superseded` so the designer can delete the stale markdown
- The migrator preserves all closed-vocabulary content and discards markdown decoration that was never part of the schema (free-text descriptions outside known fields land in a per-feature migration report, not in the YAML output)
- Build-mode validation accepts either form in v1; a later version may make YAML mandatory once migration is complete project-wide
- The migrator is idempotent — running it twice on the same project produces no diff after the first run

**Verify**:
- A feature with `surface.yaml` (and no `surface.md`) builds and codegens identically to the same feature represented in `surface.md`
- `parlay migrate-spec` on a project with `surface.md` files produces `surface.yaml` files alongside, preserving every closed-vocabulary fragment, action, and flow
- A feature with both files present builds from the YAML and warns about the markdown
- Free-text content in `surface.md` that has no closed-vocabulary destination appears in the migration report and is not silently dropped
- Re-running `parlay migrate-spec` on a project that has already migrated produces no further changes

---

## Blueprint: scope, override precedence, and strategy selection

**Goal**: Pin down what `blueprint.yaml` owns and what it does not in the multi-target world. Blueprint declares cross-cutting choices (data, auth, errors, state, navigation, platform), may configure declared targets, but cannot add, remove, replace, or relink them — target topology belongs to `adapter-set.yaml`. Override precedence is fixed at `blueprint > adapter-set > adapter default`. Strategy selection draws from closed adapter-supplied vocabularies; an unsupported choice fails build.

**Persona**: Parlay tool maintainer / Project owner authoring blueprint

**Priority**: P1

**Context**: Blueprint exists today but its scope was never crisp; the multi-target model makes the distinction between "topology" (adapter-set) and "policy" (blueprint) load-bearing. Settings that legitimately layer — error mappings, retry policies, caching strategy — need a declared precedence so the validator can resolve the effective value deterministically. Strategy choices like `data.fetching: stale-while-revalidate` only make sense if the relevant adapter implements that strategy; the build must refuse otherwise rather than producing code that silently downgrades.

**Action**: Define blueprint's owned scope as `data`, `auth`, `errors`, `state`, `navigation`, `platform`. Forbid blueprint from declaring or modifying `targets:` (slot composition is owned by `adapter-set.yaml`); a blueprint that attempts target topology fails with `blueprint-topology-not-allowed`. Resolve any layered setting through the precedence `blueprint > adapter-set > adapter default`. Define a closed strategy vocabulary for the standard cross-cutting choices (e.g. `data.fetching: {on-mount, prefetch, stale-while-revalidate, graphql}`, `data.caching: {none, per-route, shared}`, `auth.strategy: {none, session, jwt, oauth2}`, `errors.retry: {none, reads, writes, all}`). Each strategy chosen in blueprint must appear in the corresponding adapter's `supports` (or its strategy-specific equivalent); otherwise build fails with `blueprint-strategy-unsupported` naming the choice and the adapter. A canonical error left without a mapping at any level fails with `error-no-mapping` naming the operation, the error, and the missing layer (transport or presentation).

**Objects**: blueprint-scope, blueprint-precedence, strategy-selection, error-mapping, topology-ownership

**Constraints**:
- Blueprint may not contain a `targets:` block; topology is owned by `adapter-set.yaml` exclusively
- The override precedence is fixed: blueprint wins, then adapter-set, then adapter default; ties or contradictions inside one level fail with `blueprint-override-conflict`
- Strategy vocabularies are closed at the schema level; a value outside the closed set fails with `blueprint-strategy-unknown` naming the field and the closed list
- A strategy that the relevant adapter does not declare support for fails with `blueprint-strategy-unsupported` even if the value is in the closed set
- A canonical operation error that no layer maps to a transport response or a presentation surface fails with `error-no-mapping` — silence is not a default
- Blueprint remains optional; a project with no `blueprint.yaml` uses adapter defaults at every layer
- Settings outside the owned scope (data/auth/errors/state/navigation/platform) are rejected with `blueprint-scope-violation` — blueprint is not a free-form override sheet

**Verify**:
- A blueprint setting `data.fetching: stale-while-revalidate` against a presentation adapter that supports it builds clean
- The same setting against an adapter that does not declare support for `stale-while-revalidate` fails with `blueprint-strategy-unsupported`
- A blueprint setting `data.fetching: telepathy` (outside the closed set) fails with `blueprint-strategy-unknown`
- A canonical error declared in `capabilities.yaml` with no mapping at adapter, adapter-set, or blueprint level fails with `error-no-mapping` naming the operation and the error
- A blueprint that declares an entry under `targets:` fails with `blueprint-topology-not-allowed`
- A project with no blueprint still validates and uses adapter defaults end-to-end
- An override at adapter-set level is ignored when blueprint sets the same key; the resolved-value report attributes the value to blueprint and not to adapter-set

---

## Multi-target buildfile: operations, targets, and target-aware plan

**Goal**: Extend the buildfile with `operations:` (canonical, declared once) and `targets:` (one entry per registered adapter) blocks. Canonical fields — `steps`, `errors`, `policies`, `input`, `output`, `kind`, `subject` — appear under `operations:` only; target sections carry projection metadata only. The `plan:` block becomes target-aware: each target's plan rows are scoped under `plan.targets.<kind>`. Existing layout `wiring`/`bindings` continue to coexist, sharing canonical operation refs without duplication.

**Persona**: Parlay tool maintainer

**Priority**: P0

**Context**: Codegen needs every adapter to receive a coherent slice of the same canonical operation. If steps or errors were restated under each target, a generation pass would face two prompt sources for the same fact and the AI's interpretation would diverge. Declaring canonical fields once and projecting through targets keeps the prompt surface small and the validator strict. The plan block already names files codegen will create or modify; adding target awareness lets each adapter own its slice of the plan without colliding with siblings. Layout `wiring`/`bindings` answer "what does this layout button invoke?"; `targets:` answer "how is that invocation implemented across the stack?" — different concerns, shared canonical refs.

**Action**: Add an `operations:` block at the buildfile's top level whose keys are normalized operation IDs (`@<feature>/operation:<id>`) and whose values carry every canonical field exactly once. Add a `targets:` block whose keys are kinds (`presentation`, `transport`, `application`, `persistence`) and whose values carry per-target projection metadata only (component shapes for presentation, exposure metadata for transport, handler shape for application, repository shape for persistence). Forbid restating canonical fields inside any target section; the validator fails such restatements with `buildfile-target-restates-canonical` naming the target, the operation, and the offending field. Extend `plan:` with a `targets:` sub-block: existing `plan.creates` / `plan.modifies` arrays move under `plan.targets.<kind>`. Existing `wiring.rules` and `bindings` sections stay in place; their operation refs must resolve to keys present under `operations:` (cross-layer drift fails with `buildfile-binding-target-mismatch`).

**Objects**: buildfile-operations, buildfile-targets, canonical-once-rule, plan-targets, wiring-bindings-coexistence, operation-ref-resolution

**Constraints**:
- Canonical fields (`kind`, `subject`, `input`, `output`, `errors`, `policies`, `steps`) appear once under `operations:` and never inside `targets:`
- A target section restating a canonical field fails with `buildfile-target-restates-canonical` — the cure is to delete the duplicate, not to "make them agree"
- Per-target projection metadata is per-kind (presentation has `components`; transport has `exposure`/`method`/`path`; application has `handler`; persistence has `repositories.<name>.{entity, supports}`); each kind's shape is fixed by its adapter's schema
- `plan.targets.<kind>.creates` and `plan.targets.<kind>.modifies` carry the file lists previously held by `plan.creates`/`plan.modifies`; legacy top-level `plan.creates`/`plan.modifies` are still parsed for back-compat (handled in the legacy-fields intent) but new authoring uses the target-aware shape
- `wiring.rules` and `bindings` continue to use the same canonical operation refs they always did; every ref must resolve to a key under `operations:`
- A binding referencing an operation that exists under no key in `operations:` fails with `buildfile-binding-operation-missing` naming the binding and the missing ref
- Operation refs in target sections (e.g. presentation `effect.operation`) likewise must resolve to canonical keys; otherwise fails with `buildfile-target-operation-missing`
- The buildfile remains the executable contract for codegen; nothing in this intent changes that role, only its shape

**Verify**:
- A buildfile declaring `@task-list/operation:task.create` once under `operations:` and projecting it through presentation, transport, application, and persistence target sections validates clean
- The same buildfile with `errors: [validation-failed]` re-declared inside the application target fails with `buildfile-target-restates-canonical` naming the target and the field
- A binding referencing an operation absent from the `operations:` block fails with `buildfile-binding-operation-missing`
- A target's `effect.operation` referencing an unknown operation fails with `buildfile-target-operation-missing`
- A buildfile with `plan.targets.presentation.creates: [TaskCreateForm.tsx]` and `plan.targets.persistence.modifies: [schema.prisma]` validates and routes plan rows to the correct target during codegen
- Layout `wiring.rules` referencing `@task-list/operation:task.create` and a presentation target referencing the same key both resolve to the same canonical entry; renaming the canonical entry without updating both consumers is rejected

---

## Legacy buildfile fields: stay, deprecate, or repurpose

**Goal**: Pin a stay/deprecate/repurpose decision on every pre-existing buildfile field so build-feature can normalize old buildfiles into the new shape on first regeneration without ambiguity. The fields under audit: `adapter`, `components`, `routes`, `models`, `fixtures`, `cross-cutting`, `plan`, `wiring.rules`, `bindings`.

**Persona**: Parlay tool maintainer

**Priority**: P0

**Context**: Existing buildfiles in this repo and downstream projects already encode useful information that the multi-target shape needs to absorb without dropping data. Each pre-existing field has to be classified — kept as-is, repurposed under a new path, or deprecated — so the migration is mechanical and the validator can speak clearly when it encounters a legacy field that should have been moved.

**Action**: Apply the following per-field decisions in build-feature normalization and validation:
- `adapter` (top level): repurposed. Replaced by `adapter-set` reference plus per-target `adapter:` declarations. Legacy buildfiles map a top-level `adapter:` to a single-target presentation adapter set during normalization. Removal scheduled for a future minor version (separate deprecation feature).
- `components`: kept, relocated. Now lives under `targets.presentation.components`. Legacy top-level `components:` normalize there on first regeneration.
- `routes`: kept, relocated. Routes belong under `targets.presentation` (client-side) or `targets.transport` (HTTP). Legacy top-level `routes:` normalize to `targets.presentation` unless the project's adapter-set has a transport target with explicit HTTP exposure for the same path.
- `models`: deprecated. `domain-model.yaml` is the canonical home for entities. Per-feature model duplication is dropped at build-feature time; the buildfile resolves entities from `domain-model.yaml` during normalization.
- `fixtures`: kept. Per-feature fixture data continues to feed `testcases.yaml` and adapter-generated test scaffolding.
- `cross-cutting`: kept, mostly empty post-rename. Pattern fragments that previously lived in `infrastructure.md` decompose elsewhere (per the pattern-fragment intent). The section retains adapter-level cross-cutting metadata that does not fit `operations`, `domain-model`, blueprint, or adapter-set.
- `plan`: kept, extended to be target-aware. Per-target `plan.targets.<kind>` is the new authoritative shape; legacy top-level `plan.creates`/`plan.modifies` parse and normalize to the appropriate target on first regeneration.
- `wiring.rules` and `bindings`: kept unchanged. They coexist with `operations:` and `targets:`, sharing canonical operation refs.

**Objects**: legacy-buildfile-fields, build-feature-normalization, deprecation-warnings, target-relocation, models-deprecated

**Constraints**:
- Normalization happens on first regeneration through build-feature; reading a legacy buildfile with no regeneration does not silently rewrite it
- A buildfile with both legacy `components:` at top level and `targets.presentation.components` populated fails with `buildfile-components-double-declared` (pick one)
- A buildfile with legacy `models:` populated emits `buildfile-models-deprecated` (warning in authoring, error in build) directing the author to `domain-model.yaml`
- A buildfile with legacy `routes:` whose paths overlap with `targets.transport` HTTP exposures fails with `buildfile-routes-ambiguous` so the migrator does not silently choose the wrong target
- A buildfile with a legacy top-level `adapter:` field still parses and is treated as a single-target presentation adapter set during this version; the field is not removed yet (separate deprecation feature owns removal)
- Designer review during build-feature surfaces the deprecation of `models:` and any ambiguous routing decisions, so the designer is aware before the buildfile is written back to disk
- `wiring.rules` and `bindings` are explicitly out of scope for migration churn — they continue to work as-is

**Verify**:
- A legacy buildfile with top-level `components: {TaskCreateForm: ...}` and `adapter: react-antd` produces, after build-feature, `targets.presentation.components.TaskCreateForm: ...` and an `adapter-set` reference covering only the presentation target
- A legacy buildfile with both top-level `components:` and `targets.presentation.components:` fails with `buildfile-components-double-declared`
- A legacy buildfile with non-empty `models:` warns in authoring mode and errors in build mode, with a fix message naming `domain-model.yaml`
- A legacy buildfile whose `routes: [{path: /tasks}]` collides with a transport target's `path: /tasks` exposure fails with `buildfile-routes-ambiguous`
- A buildfile with legacy `plan.creates: [...]` (no `plan.targets`) normalizes to `plan.targets.presentation.creates: [...]` for a presentation-only project
- `wiring.rules` and `bindings` survive build-feature normalization byte-equivalent (no rewriting of these sections)

---

## testcases.yaml v2: discriminated suite kinds and source_refs

**Goal**: Extend `testcases.yaml` from presentation-only to a v2 schema with discriminated suite kinds (`presentation`, `operation`) in one file. Every canonical operation needs at least one `kind: operation` suite. Every new or regenerated v2 suite cites at least one `source_refs` entry tying the suite to its origin in surface or capabilities. Legacy v1 suites continue to load.

**Persona**: Parlay tool maintainer / UX Designer

**Priority**: P0

**Context**: With `capabilities.yaml` providing closed-vocabulary backend operations, those operations need testable contracts. A discriminated suite kind keeps presentation tests (rendering, click, verify) and operation tests (input/output/error/persistence assertions) in one file without forcing the validator to guess. `source_refs` give every test a traceable origin in the spec layer; without them, tests drift away from the artifacts they are supposed to verify.

**Action**: Bump `testcases.yaml` to `schema_version: 2`. Each suite carries `kind: presentation` (rendering/interaction) or `kind: operation` (input/output/persistence assertions against a canonical operation). Operation suites reference the canonical operation key (`@<feature>/operation:<id>`) and assert over `output`, `error`, and `persistence` shapes. Every canonical operation declared in `capabilities.yaml` must have at least one operation suite; missing coverage fails build with `testcases-operation-uncovered` naming the operation. Every new v2 suite (presentation or operation) must declare at least one `source_refs` entry pointing at the surface fragment or capability operation it verifies; build-mode failure code is `testcases-source-refs-missing`. Legacy v1 presentation suites load as v2 presentation suites; missing `source_refs` on legacy-loaded suites are warnings until the project regenerates v2 testcases.

**Objects**: testcases-v2, suite-kind-presentation, suite-kind-operation, source-refs, operation-coverage

**Constraints**:
- `schema_version: 2` is the new authoring shape; v1 files load and are accepted for legacy compatibility
- Each suite has `kind: presentation` or `kind: operation`; an unknown kind fails with `testcases-suite-kind-unknown`
- Every canonical operation in `capabilities.yaml` requires at least one `kind: operation` suite, OR an explicit exemption in the coverage-review file (separate intent)
- Presentation-only projects (no `capabilities.yaml`) require no operation suites and are unaffected by this rule
- New v2 suites must declare `source_refs:` (at least one entry); operation suites *always* require `source_refs` even in legacy-loaded projects
- Legacy v1 presentation suites without `source_refs` warn (`testcases-source-refs-missing-legacy`) but do not block build until the project regenerates v2 testcases
- Source refs use stable selectors (e.g. `surface.fragments['TaskCreateForm']`, `capabilities.operations['task.create']`) so the validator can mechanically check resolution
- Operation suites assert over the closed shapes defined by the operation's `output` / `errors` / `persistence` projection — the validator rejects assertions that name fields outside those shapes

**Verify**:
- A `testcases.yaml` with `schema_version: 2` and one `kind: presentation` suite plus one `kind: operation` suite (covering the only canonical operation) builds clean
- The same file with the operation suite removed fails build with `testcases-operation-uncovered` naming the missing operation
- A new v2 suite without any `source_refs` entry fails with `testcases-source-refs-missing` in build mode
- A legacy v1 suite (loaded as v2 presentation) without `source_refs` warns rather than fails, until regenerated
- An operation suite asserting `output.entity: Project` when the canonical operation's `output.entity: Task` fails with `testcases-operation-shape-mismatch`
- A presentation-only project with no `capabilities.yaml` builds clean with only presentation suites

---

## coverage-review.yaml gates codegen on human approval

**Goal**: Introduce a `coverage-review.yaml` artifact that hash-binds human approval to the buildfile and the testcases. `parlay generate-code` refuses to run without a matching review. Re-running build-feature changes the hashes and forces re-approval. This is a workflow integrity mechanism: it prevents the AI from authoring the operation, authoring the tests, and grading its own homework in a single uninterrupted pass.

**Persona**: UX Designer reviewing testcases / Parlay tool maintainer

**Priority**: P0

**Context**: AI-driven codegen produces both the implementation and the tests in adjacent passes. With nothing between testcase authoring and codegen, vacuous tests that cite real contracts but verify nothing can slip through; `source_refs` alone do not catch this. A small, hash-bound review file gives a designer one explicit checkpoint to read the suites and approve them. The cost is one prompt per regeneration cycle; the benefit is a hard stop on AI self-grading.

**Action**: Add `.parlay/build/<feature>/coverage-review.yaml` with required keys `feature`, `reviewed_at`, `reviewed_by`, `review_method`, `buildfile_hash`, `testcases_hash`, `approved_suites`, and optional `exemptions`. Compute `buildfile_hash` over the canonical-form `buildfile.yaml` and `testcases_hash` over the canonical-form `testcases.yaml`. `parlay generate-code` refuses to run if (a) the file is missing, (b) either hash does not match the on-disk artifact, or (c) any required approval (every operation suite, every presentation suite the feature declares) is absent from `approved_suites`. Re-running `build-feature` recomputes the hashes; any mismatch invalidates the review. Required-but-missing coverage (a step, error, or policy without a covering testcase) requires an explicit `exemptions:` entry naming the suite, the missing item, and a free-text `reason`; absent exemption fails generate-code with `coverage-review-uncovered` even if the review file otherwise validates.

**Objects**: coverage-review-yaml, buildfile-hash, testcases-hash, approved-suites, exemptions, codegen-gate

**Constraints**:
- The review file lives alongside `buildfile.yaml` and `testcases.yaml` under `.parlay/build/<feature>/`
- `buildfile_hash` and `testcases_hash` are SHA-256 over a canonical (deterministic) serialization of the artifact, so cosmetic diffs (whitespace, key order) do not invalidate review
- A missing review file fails generate-code with `coverage-review-missing`; a hash mismatch fails with `coverage-review-stale` naming which hash drifted
- Every suite present in `testcases.yaml` must be listed in `approved_suites` for codegen to proceed; an unapproved suite fails with `coverage-review-suite-unapproved` naming the suite
- A required step / error / policy with no covering testcase requires an `exemptions:` entry; without one, generate-code fails with `coverage-review-uncovered` naming the operation, the term, and the term kind
- The review file is a workflow integrity mechanism, not a cryptographic security boundary — it records local human review intent and may be replaced with signed records in a later version
- V1 binds review to `buildfile_hash` and `testcases_hash`; a later version may add a normalized `review_subject_hash` covering adapter-set, referenced adapters, blueprint, and domain-model if the review surface needs to widen
- No separate exemption schema and no closed exemption marker vocabulary exist in v1; the review file's `exemptions:` block is the entire mechanism

**Verify**:
- A feature with matching hashes and every suite listed in `approved_suites` codegens successfully
- The same feature after editing `testcases.yaml` (changing the hash) fails generate-code with `coverage-review-stale` naming the testcases hash
- Re-running build-feature changes both hashes and re-running generate-code without re-approval fails with `coverage-review-stale`
- A feature whose `capabilities.yaml` declares an error `server-error` with no covering operation-suite case AND no exemptions entry for it fails generate-code with `coverage-review-uncovered` naming the operation and the error
- The same feature with an `exemptions:` entry naming the suite, `error:server-error`, and a reason codegens successfully
- A presentation-only project that has migrated to v2 testcases requires the same review file as a backend project; a presentation-only project still on v1 testcases warns until it regenerates (see migration intent)
- A missing review file fails with `coverage-review-missing`, not with a downstream error

---

## Codegen flow: ordered layer generation and fixed read-set

**Goal**: Pin codegen's read-set and emission order. `parlay generate-code` reads only the buildfile, testcases, coverage review, adapter-set, referenced adapters, blueprint, config, domain-model, and the source tree; it does *not* read `spec/intents/**`. Default generation order across kinds is `persistence → application → transport → presentation`. Regeneration conformance is verified through tests, not byte-equality of source files.

**Persona**: Parlay tool maintainer

**Priority**: P0

**Context**: AI-driven codegen has to be insulated from the spec layer for two reasons. First, the spec layer is designer-owned and changes asynchronously; reading it at generate-code time would re-introduce all the prompt drift that build-feature exists to remove. Second, the buildfile is the executable contract — anything codegen needs should already be there. Pinning read-set makes that contract testable. Pinning emission order across kinds avoids "presentation generated against an application slice that does not exist yet" failure modes during initial-pass generation.

**Action**: Define generate-code's allowed inputs as the union of `.parlay/build/<feature>/{buildfile.yaml, testcases.yaml, coverage-review.yaml}`, `.parlay/adapter-set.yaml`, the referenced adapter files under `.parlay/adapters/`, `blueprint.yaml`, `config.yaml`, `domain-model.yaml`, and the source tree under each adapter's declared root. Forbid reads of `spec/intents/**`; instrumentation logs any attempt and fails generate-code with `codegen-spec-read-forbidden`. Default emission order is `persistence → application → transport → presentation`; each layer generates fully before the next starts. Each layer's freshly-generated outputs feed the next layer's prompt — application generation sees the persistence schema just produced, transport generation sees the application handlers, presentation generation sees the transport endpoints — so the prompt context for each layer already reflects the in-progress codegen state. Regeneration conformance is the testcase suite, not source-file equality — two regeneration runs of the same buildfile are required to pass the same suite but may produce different source bytes. Generated-code hashes are still tracked for ownership and drift detection but are not behavioral proof.

**Objects**: codegen-read-set, spec-read-forbidden, emission-order, regeneration-conformance, behavioral-vs-byte-equality

**Constraints**:
- `spec/intents/**` is off-limits at codegen time; the build-feature step is the only stage that bridges spec to buildfile
- Reading any path outside the allowed set fails with `codegen-input-out-of-scope` naming the path
- Generation order across kinds is `persistence → application → transport → presentation` by default; an adapter set may declare an override only with explicit reason in adapter-set.yaml (`generation_order:` field, also closed-vocab list of permutations) — out of scope for v1
- Each layer's freshly-generated outputs are visible to the next layer's prompt; reordering layers therefore changes which outputs feed which prompt and is a behavioral change, not a mechanical reshuffle
- Regeneration conformance is the test suite; CI runs the suite twice on independent regenerations to catch prompt-context flakiness
- Generated-code drift detection (file hashes per generated path) is a separate concern from behavioral conformance; both run, but a hash drift alone is not a failure when the suite still passes
- Each layer fully completes before the next begins — partial transports cannot be generated against a half-finished application layer in v1
- Codegen invocation that lacks a passing `coverage-review.yaml` (per the review-gate intent) is rejected before any file is read for actual generation

**Verify**:
- A generate-code run that attempts to read `spec/intents/<feature>/capabilities.yaml` fails with `codegen-spec-read-forbidden` and does not produce any output
- A generate-code run that reads `domain-model.yaml`, the buildfile, the testcases, and the source tree under each target's root succeeds and writes files in `persistence → application → transport → presentation` order
- Two independent regeneration runs of the same buildfile both pass the same testcase suite even though their source bytes differ
- An application-layer generation run after a persistence-layer regeneration includes the new persistence schema in its prompt context — verifiable by changing a persistence shape and observing application generation respond to it (or by trace inspection of the layer's prompt assembly)
- A generate-code run with a missing or stale `coverage-review.yaml` fails before any file is read for generation
- An attempt to read `apps/web/src/...` from a project whose presentation target's root is `apps/dashboard/src/...` fails with `codegen-input-out-of-scope`

---

## Validation modes: authoring vs build

**Goal**: Make explicit that Parlay validation runs in two modes — authoring (permissive, warning-rich, accepts drafts and migration stubs) and build (strict, used by build-feature and generate-code). The same rules apply in both modes; their severity differs. Tooling chooses the mode by the entry point, never by a flag the designer has to remember.

**Persona**: Parlay tool maintainer

**Priority**: P1

**Context**: Designer authoring tools need to surface drift and missing pieces without blocking work; the build pipeline needs to refuse to ship anything broken. Splitting validation into two modes — and making the entry point determine which one applies — gives both audiences what they need without a flag-driven UX. It also makes deprecation paths livable: a deprecated field warns in authoring mode and fails in build mode, so designers see the warning long before the build pipeline rejects it.

**Action**: Define two validation modes formally:
- **authoring**: invoked by interactive validate calls and editor integrations; deprecation warnings, missing-source_refs on legacy suites, prose-only fragments in spec layer, and similar non-fatal issues surface as warnings; the validator returns "valid with warnings" rather than failing
- **build**: invoked by `parlay build-feature` and `parlay generate-code` (and CI entry points); the same rules return errors instead of warnings; the validator fails the build on any violation

The mode is set by the entry point, not by a CLI flag the designer toggles. Every error code in the multi-target validation surface declares its severity in each mode (in the schema files); rules without an explicit declaration default to error in both modes (there is no "warnings-only" rule).

**Objects**: validation-modes, authoring-mode, build-mode, severity-by-mode, entry-point-driven

**Constraints**:
- Mode is determined by the validator entry point, not by a flag — `parlay validate` (default) runs in authoring mode; `parlay build-feature`, `parlay generate-code`, and equivalent CI hooks run in build mode
- Each rule declares its severity per mode in the schema; rules with no declaration default to error in both modes
- Authoring mode never silently passes a build-mode failure; it surfaces every error-in-build-mode rule as a warning at minimum, so a designer running validate locally sees what build will reject
- Build mode never downgrades an error to a warning; partial success is not a build outcome
- The split applies to all multi-target rules introduced by this feature (capabilities, supports, links, coverage-review, testcases, blueprint, etc.) — no rule is exempt
- A future "lint mode" or similar is out of scope; v1 has exactly two modes

**Verify**:
- `parlay validate` on a project whose `domain-model.yaml` populates `operations:` returns "valid with warnings" naming `domain-operations-deprecated`
- `parlay build-feature` on the same project fails with `domain-operations-deprecated` as an error
- `parlay validate` on a project missing `coverage-review.yaml` returns warnings (no codegen attempted)
- `parlay generate-code` on the same project fails with `coverage-review-missing`
- A new rule introduced without an explicit per-mode severity entry defaults to error in both modes (verified by the rule schema test)
- A rule fired in authoring mode and in build mode produces the same error code; only severity differs

---

## Migration of legacy artifacts to the new shape

**Goal**: Provide a complete migration path from the pre-multi-target world to the new shape, covering adapter files (kind default), buildfiles (target relocation), spec artifacts (`surface.md` → `surface.yaml`, `infrastructure.md` → `capabilities.yaml`), domain-model operations, and the legacy `prototype-framework` field. Existing projects continue to validate while they migrate; build-feature normalizes on first regeneration.

**Persona**: UX Designer / Parlay tool maintainer

**Priority**: P0

**Context**: Multi-target lands across an existing user base with shipped projects. A clean migration requires every legacy artifact to have a defined path forward and a tool that walks it. The principle is that no legacy file is silently rewritten without designer review; migration steps surface their decisions and let the designer reject ambiguous cases. Some legacy fields scheduled for outright removal (e.g. `prototype-framework`, the top-level `adapter:` field) are marked deprecated here but their *removal* is owned by separate deprecation features that ship in a later version.

**Action**: Implement migration steps:
- **Adapter files**: an adapter file without `kind:` is treated as `kind: presentation` (default applied at validator time, no rewrite required); designer review during `parlay upgrade` offers to write the explicit field
- **Legacy buildfile fields**: `parlay build-feature` normalizes top-level `adapter:`, `components:`, `routes:`, and `plan.creates`/`plan.modifies` into the new target-aware shape on first regeneration (per the legacy-fields intent), with designer-visible diffs
- **Legacy v1 testcases**: `schema_version: 1` testcase files load as v2 presentation suites. Each legacy suite's `intent` string auto-populates as `source_refs[0]` so the migrated suite carries provenance from day one; the entry can be expanded manually or by regeneration. Missing `source_refs` on legacy-loaded suites are warnings (not errors) until the project regenerates v2 testcases — the build-mode error severity from the testcases-v2 intent applies only to new or regenerated v2 suites
- **`surface.md` → `surface.yaml`**: `parlay migrate-spec` parses existing markdown into the closed-schema YAML form. Free-text content with no schema destination lands in a per-feature migration report rather than being silently dropped
- **`infrastructure.md` → `capabilities.yaml`**: `parlay migrate-capabilities` converts operation-shaped fragments into closed operations under `kind: <best-guess>` (interactive when ambiguous). Pattern-shaped fragments are out of scope for auto-conversion (handled in the pattern-fragment intent)
- **`domain-model.operations` → capability stubs**: `parlay migrate-domain-operations` lifts entries into stubs with `kind: unknown` (per the domain-model deprecation intent)
- **`prototype-framework`**: `parlay migrate-config` converts the legacy field to a single-target adapter set (presentation slot only). The field stays parseable in v1 with a `prototype-framework-deprecated` warning; outright removal is owned by a separate deprecation feature scheduled for a later version

Existing projects that have not run any migration step continue to validate (with deprecation warnings) and continue to build (with field-by-field normalization happening in-memory). Migration is opt-in but recommended.

**Objects**: migrate-spec, migrate-capabilities, migrate-domain-operations, migrate-config, kind-default-presentation, normalization-on-regeneration

**Constraints**:
- No migration step silently overwrites a designer-authored file; each surfaces its diff and waits for confirmation
- `parlay migrate-capabilities` only auto-converts operation-shaped fragments; pattern-shaped fragments are reported and routed by the designer (separate intent)
- `parlay migrate-spec` is idempotent — running it twice produces no further changes after the first run
- `parlay migrate-domain-operations` writes stubs with `kind: unknown`; it does not fabricate closed-vocabulary terms
- `parlay migrate-config` covers `prototype-framework` and any other top-level legacy adapter binding; the resulting adapter-set has only the presentation slot filled
- Legacy fields removal (full deletion of `prototype-framework`, `adapter:`, `models:`) is *out of scope* for this feature; each is tracked in its own deprecation feature so the schedule is visible and the changes are reviewable separately
- A project that has not migrated still validates and builds in v1; deprecation warnings accumulate but nothing breaks
- Each migration step is independently runnable — `migrate-spec` does not depend on `migrate-capabilities`, etc.

**Verify**:
- An adapter file without `kind:` validates clean and is treated as `kind: presentation`; `parlay upgrade` offers to write the explicit field but does not require it
- A project with `infrastructure.md` containing operation-shaped fragments produces, after `parlay migrate-capabilities`, a `capabilities.yaml` with closed-vocabulary operations and a migration report listing pattern-shaped fragments left for designer review
- A project with `surface.md` produces, after `parlay migrate-spec`, a `surface.yaml` byte-equivalent to running build-feature against the original markdown (same in-memory model in, same closed-schema YAML out)
- A project with `prototype-framework: react` produces, after `parlay migrate-config`, a `.parlay/adapter-set.yaml` with only the presentation slot filled and the relevant adapter
- A project that has not run any migration step builds successfully under v1 with deprecation warnings
- Re-running any migration step on an already-migrated project produces no diff
- Removal of legacy fields is not attempted by this feature; the warnings continue to fire until a separate deprecation feature ships

---

## Pattern-fragment decomposition during capabilities migration

**Goal**: When `parlay migrate-capabilities` encounters pattern-shaped fragments in a legacy `infrastructure.md` (registries, pipelines, dispatchers, resolvers, validators, and similar engineering patterns), it does *not* auto-convert them into capability operations. Instead, it emits a migration report grouping each fragment with a suggested destination — domain-model state, adapter-level pattern, blueprint setting, transport routing, capability operation, or v2-deferred subscription/job. The designer routes the fragments; the migrator never invents closed-vocabulary content for pattern fragments.

**Persona**: UX Designer / Parlay tool maintainer

**Priority**: P1

**Context**: Existing `infrastructure.md` files drifted toward describing engineering patterns, not operations. Auto-converting those fragments would either fabricate closed-vocabulary content (lying about what the system does) or silently drop them (losing intent). The third path is to surface each fragment with a suggested destination so the designer makes the call. The closed list of suggested destinations matches the architectural surfaces that *can* legitimately host each kind of pattern.

**Action**: Extend `parlay migrate-capabilities` with a pattern-fragment classifier that produces a per-feature migration report. The report groups fragments by detected shape and pairs each with a suggested destination from the closed list:
- pipeline-shaped fragment → command operation in `capabilities` (or a v2-deferred job)
- registry-shaped fragment → domain entity plus register/list/lookup operations
- dispatcher-shaped fragment → transport-adapter routing logic
- traversal-shaped fragment → query operation with `read-tree` if the relevant adapter supports it
- resolver-shaped fragment → query operation
- validator-shaped fragment → `validate-input` step or shared step
- aspect-shaped fragment → operation policy
- cache-shaped fragment → cacheable policy plus blueprint `data.caching` setting
- migrator-shaped fragment → domain-model state machine plus a migration command
- hook-shaped fragment → v2-deferred subscription, or adapter lifecycle config
- helper / utility fragment → adapter-level pattern (not spec-layer)
- otherwise → "unrouted; designer review" with the fragment quoted verbatim

The migrator never auto-applies a suggestion. Designer review is required to actually move each fragment to its destination.

**Objects**: pattern-fragment-classifier, migration-report, suggested-destinations, designer-routing, no-auto-conversion

**Constraints**:
- The classifier is conservative; ambiguous fragments fall into the "designer review" bucket rather than being misclassified
- The closed list of suggested destinations is owned by Parlay and lives in a schema file; new pattern shapes (and their suggested destinations) extend the list in later versions
- The migration report names each fragment by source line range, the detected shape, and the suggested destination — enough context for the designer to find and re-author the fragment
- The migrator does not write to any destination on the designer's behalf; it only writes the report
- A fragment whose suggested destination is "v2-deferred" (subscription, job) is preserved verbatim in the report so the project can revisit it when v2 lands
- The pattern-fragment classifier is independent of the operation-shaped extractor; both run on the same `infrastructure.md` input but produce disjoint outputs (operations into `capabilities.yaml`, patterns into the report)
- Helper / utility fragments routed to "adapter-level pattern" are explicitly *not* spec-layer concerns and the report says so, so designers do not try to wedge them into `capabilities.yaml`

**Verify**:
- A fragment describing "router registry that lists registered handlers" classifies as registry-shaped and the report suggests "domain entity plus register/list/lookup operations"
- A fragment describing "validate text length, then trim whitespace, then enforce uniqueness" classifies as pipeline-shaped and the report suggests "command operation in capabilities"
- A fragment that the classifier cannot confidently route lands in "unrouted; designer review" with the fragment quoted verbatim — never silently dropped
- A fragment classified as "v2-deferred subscription" is preserved in the report, not in `capabilities.yaml`
- The migrator never writes to the suggested destination on its own — `capabilities.yaml`, `domain-model.yaml`, and `blueprint.yaml` are unchanged by this step regardless of the suggestions
- Re-running the migration on an unchanged `infrastructure.md` produces an identical report

---

## Presentation-only projects continue to work unchanged

**Goal**: Projects that register only a presentation adapter — no transport, application, or persistence — continue to validate, build, and codegen exactly as they did before this feature shipped. Multi-target machinery (capabilities, links, supports for backend kinds, coverage-review for backend operations) is strictly opt-in: presentation-only projects feel no friction.

**Persona**: Maintainer of a presentation-only project

**Priority**: P0

**Context**: The vast majority of existing Parlay projects are presentation-only. The architectural shift to multi-target must not impose ceremony on those projects. The signal for "this project is multi-target" is the presence of non-presentation slots in `.parlay/adapter-set.yaml`; their absence keeps every existing code path live. This is the explicit backwards-compatibility contract for the rollout — it is what distinguishes "additive feature" from "schema migration".

**Action**: Make every multi-target validation rule check first whether the project's adapter set has any non-presentation slot filled before applying. If not:
- `capabilities.yaml` is not required (its absence is not a failure)
- Backend `supports` validation is skipped (no backend adapters to consult)
- Adapter-set `links` validation has no cross-kind edges to enforce
- Blueprint backend strategy validation runs only on settings that target presentation
- `testcases.yaml` requires no operation suites; legacy v1 presentation suites continue to work without `source_refs` (warning-only until v2 regeneration)
- `coverage-review.yaml` is required once the project has migrated to v2 testcases, but legacy v1 presentation-only projects start with warnings (not errors) until they regenerate
- Codegen reads only the presentation slice and emits in presentation order

A presentation-only project run end-to-end through the full pipeline (`add-feature` → `scaffold-dialogs` → `create-artifacts` → `build-feature` → `generate-code`) produces the same output before and after this feature ships, modulo unrelated changes.

**Objects**: presentation-only-projects, opt-in-multi-target, back-compat-contract, single-slot-adapter-set

**Constraints**:
- A project's "multi-target-ness" is determined by the count of filled slots in `.parlay/adapter-set.yaml`, not by a flag
- Every backend rule (capabilities, supports, links, blueprint backend strategies, operation suites, coverage-review for operations) checks the slot composition before applying
- A presentation-only project that adds a transport, application, or persistence slot transitions into multi-target mode automatically; no explicit "migrate to multi-target" step is required, though the migration intent's `migrate-spec` and `migrate-capabilities` steps remain available
- Legacy v1 testcase suites in presentation-only projects emit warnings (not errors) for missing `source_refs` until the project regenerates v2 testcases
- A presentation-only project does not require `coverage-review.yaml` until it migrates to v2 testcases; once migrated, the gate applies uniformly
- No existing buildfile, intents, dialogs, surface, or infrastructure file in a presentation-only project needs to be regenerated solely on account of this feature shipping
- Every error code introduced by multi-target rules has a "presentation-only short-circuit" check documented in its rule definition, so the back-compat contract is mechanical, not promised in prose

**Verify**:
- A presentation-only project run through the full pipeline before this feature ships and after it ships produces byte-identical output (modulo unrelated changes)
- A presentation-only project has no `capabilities.yaml`; build-feature succeeds and produces a buildfile whose `operations:` block is empty and whose `targets:` block contains only the presentation entry
- A presentation-only project's legacy v1 testcase suites without `source_refs` warn but do not block build
- A presentation-only project with no `coverage-review.yaml` and only legacy v1 testcases codegens successfully (gate not yet active)
- A presentation-only project that adds a transport target to its adapter-set transitions into multi-target mode and starts requiring `capabilities.yaml`, operation suites, and `coverage-review.yaml` on the next build
- An adapter-set link rule referencing only `presentation` validates clean even with no other slots filled

---
