# Parlay Multi-Adapter Architecture

> **Status (2026-07-23): historical design proposal.** This document records the thinking that led to the multi-adapter work; it is not a description of the shipped system. The main point since superseded: this doc frames the spec layer as **exactly three artifacts** (`surface`, `capabilities`, `domain-model`) and describes `infrastructure` as *renamed* to `capabilities`. The shipped model is **four co-equal artifacts** — `infrastructure.md` was *retained* alongside `capabilities.yaml`, not renamed away. Operation-shaped content lives in `capabilities.yaml`; architectural prose (boundaries, probes, allowlists, dependency pins) stays in `infrastructure.md`. The two are co-equal and cover orthogonal concerns. Read the body below as design rationale, not current behavior.

## Status

Architecture proposal, simplified revision.

This revision keeps the original multi-adapter idea but removes unnecessary
technical load from the earlier drafts. Closed-vocabulary details and exhaustive
validation rules belong in the corresponding schema files when they are authored.
This document defines the artifact boundaries, adapter model, buildfile model,
codegen workflow, and migration path.

Two simplifying decisions are now explicit:

1. The existing `infrastructure` artifact is renamed and reframed as
   `capabilities` — the closed-vocabulary home for backend operations triggered
   by surface actions or other events. No new spec artifact is introduced.
2. The spec layer converges on YAML. `surface.yaml`, `capabilities.yaml`, and
   `domain-model.yaml` are the target formats. Existing markdown files are
   treated as legacy migration input only.

This revision also fills two gaps from the prior pass: the blueprint's role and
override precedence in the multi-adapter model are now explicit (§5), and each
existing buildfile field is audited against the new operations/targets shape
with stay/deprecate/repurpose decisions stated (§9.1).

---

## 1. The Change in One Paragraph

Parlay's adapter today is presentation-only: it maps a closed surface vocabulary
(`Shows`, `Actions`, `Flows`) to framework-specific UI. Backend work has no
equivalent contract, so generation outside the UI relies on prose. This proposal
extends the adapter system so a project can register multiple adapters of
different kinds — presentation, transport, application, persistence — each
closed against its own vocabulary. The existing `infrastructure` artifact is
renamed to `capabilities` and reframed as the closed-vocabulary home for backend
operations that non-presentation adapters consume. The buildfile becomes
multi-target, projecting each canonical operation through every registered
adapter. Existing presentation-only projects continue to work unchanged.

---

## 2. Principles

- Intent stays framework-agnostic.
- The spec layer has three artifacts: `surface`, `capabilities`, and
  `domain-model`.
- `surface` and `capabilities` are YAML going forward.
- Backend behavior is modeled as closed-vocabulary operations, not prose.
- Adapters declare what they implement; validation refuses anything they do not.
- Buildfiles remain the executable contract for codegen.
- Codegen is AI-driven; behavioral conformance is verified through tests, not
  byte comparison.

---

## 3. Spec Layer

The spec layer consists of exactly three artifacts:

```text
spec/intents/<feature>/surface.yaml          What the user touches.
spec/intents/<feature>/capabilities.yaml     What the system does.
<activeRoot>/domain-model.yaml               What the data is.
```

`surface.yaml` is the closed presentation vocabulary. It replaces the legacy
`surface.md` format.

`capabilities.yaml` is the renamed and reframed `infrastructure` artifact. It
is the closed backend-operation vocabulary. It replaces the legacy
`infrastructure.md` format.

`domain-model.yaml` remains the canonical home for domain nouns: entities,
relationships, states, enums, and value objects. Its existing `operations` field
is deprecated in favor of `capabilities.yaml`.

Markdown remains appropriate for narrative documents such as README files,
agent instructions, proposals, and design notes. It is no longer the target
format for Parlay's machine-validated spec artifacts.

---

## 4. Adapters and Adapter Sets

## 4.1 Adapter Kinds

Every adapter declares a `kind`. Four kinds are supported in v1:

```text
presentation    UI rendering and interaction.
transport       API, RPC, GraphQL, queue, or other boundary layer.
application     Backend handlers, services, use cases, policies.
persistence     Repositories, migrations, storage access, fixtures.
```

Existing adapters default to `kind: presentation` and remain valid.

## 4.2 Adapter Set

A project registers adapters through `.parlay/adapter-set.yaml`.

V1 supports at most one target per adapter kind. This keeps validation and
codegen simple while leaving room for future multi-transport or multi-database
projects.

```yaml
name: default

targets:
  presentation: { adapter: react-antd,          root: apps/web }
  transport:    { adapter: openapi-rest,        root: apps/api }
  application:  { adapter: nestjs-application,  root: apps/api }
  persistence:  { adapter: prisma-postgres,     root: apps/api }

links:
  - { from: presentation, relation: calls,      to: transport }
  - { from: transport,    relation: dispatches, to: application }
  - { from: application,  relation: persists,   to: persistence }
```

Links are enforceable, not decorative. Presentation may not call application or
persistence directly; transport may not access persistence; violations fail
validation.

Presentation-only projects register only a presentation adapter. The other
slots are unfilled and the corresponding validation rules do not apply.

Bundled presets cover common stacks, for example:

```text
react-antd-only
angular-clarity-only
react-nest-prisma
angular-nest-prisma
```

Custom mode lets teams compose their own adapter set.

## 4.3 Adapter Support Contract

Non-presentation adapters declare what they implement. This is the basis for
"fail before generation" rather than "best-effort generate".

```yaml
name: nestjs-application
kind: application

supports:
  operation_kinds: [command, query]
  steps:
    - validate-input
    - authorize
    - create-one
    - update-one
    - delete-one
    - read-one
    - read-many
    - search
    - return-one
    - return-many
    - return-empty
  policies:
    - auth-required
    - permission-required
    - transaction-required
  errors:
    - validation-failed
    - unauthorized
    - forbidden
    - not-found
    - conflict
    - server-error
```

If a feature requires a step, policy, error, or operation kind that no selected
adapter supports, build validation fails before the AI is invoked.

Pattern descriptions describing how the AI should implement each supported
concept live alongside `supports` in the adapter file. They are AI prompt
material, not human-only documentation. Authoring guidelines for pattern
descriptions belong in a separate adapter-author guide.

---

---

## 5. Blueprint

The blueprint is the project-level cross-cutting decision document. It is not
new to this proposal — it exists in Parlay today — but the multi-adapter model
clarifies its scope and precedence.

## 5.1 What Blueprint Owns

Blueprint declares cross-cutting choices that apply across features:

```text
data         fetching strategy, caching strategy, contract source
auth         authentication strategy, authorization model
errors       project-level error mapping overrides
state        global state management policy
navigation   app-level routing strategy
platform     deployment assumptions, runtime environment
```

Blueprint may configure declared targets but cannot add, remove, replace, or
relink them — target topology is owned by `adapter-set.yaml`.

## 5.2 Separation of Concerns

```text
capabilities.yaml   what the system does
domain-model.yaml   what the data is
surface.yaml        what the user touches
adapter-set.yaml    which targets and adapters are active
adapters/*.yaml     how each adapter generates code
blueprint.yaml      project-level cross-cutting choices and overrides
```

## 5.3 Override Precedence

Settings that can be defined at multiple levels — error mappings, retry
policies, caching strategy — follow this precedence:

```text
blueprint > adapter-set > adapter default
```

Adapters provide defaults. Adapter-set may override per-project. Blueprint
takes final precedence. A canonical error without any mapping at any level
fails build validation with `error-no-mapping` naming the operation, the
error, and the missing layer (transport or presentation).

## 5.4 Strategy Selection

Blueprint selects from closed-vocabulary strategies that adapters implement.
Each strategy must be supported by the relevant adapter; build validation
fails if blueprint selects an unsupported strategy.

```text
data.fetching:    on-mount | prefetch | stale-while-revalidate | graphql
data.caching:     none | per-route | shared
auth.strategy:    none | session | jwt | oauth2
errors.retry:     none | reads | writes | all
```

---

## 6. Capabilities: The Backend Contract

`capabilities.yaml` is to backend generation what `surface.yaml` is to UI
generation: the feature-local closed-vocabulary contract for system behavior.

```yaml
schema_version: 1
feature: task-list

operations:
  - id: task.create
    kind: command
    subject: { entity: Task }
    input:  { type: CreateTaskInput }
    output: { shape: one, entity: Task }
    errors: [validation-failed, conflict]
    policies: [transaction-required]
    steps:
      - { type: validate-input }
      - { type: create-one, entity: Task, identity: generated }
      - { type: return-one, entity: Task }
```

Operation ids are feature-local. The buildfile normalizes them to the form:

```text
@<feature>/operation:<id>
```

Example:

```text
@task-list/operation:task.create
```

Regardless of serialization during migration, capabilities must normalize to
this closed operation model. Prose-only capability fragments are invalid in
build mode.

---

## 7. Why Rename Infrastructure Instead of Adding an Artifact

The original purpose of `infrastructure` was to capture events that happen
behind the scenes of the presentation level: the system's response to user
actions, off-screen.

For example:

```text
User: task-list add "buy milk"
System (background): Loads tasks from local store.
System (background): Validates task text.
System (background): Assigns the next available task ID.
System (background): Appends the task to the store and persists.
System: [OK] Task #3 added (medium): buy milk
```

Those background turns are exactly what the closed-vocabulary backend contract
should formalize.

In practice, existing `infrastructure.md` files drifted toward describing
engineering patterns such as registries, pipelines, and dispatchers rather than
operation-level system behavior. This proposal restores the original purpose,
gives the artifact a clearer name, and avoids adding a fourth spec artifact.

The spec layer remains:

```text
surface       what the user touches
capabilities  what the system does
domain-model  what the data is
```

---

## 8. Closed Vocabularies

Backend behavior uses closed lists at four points:

```text
operation kinds
steps
errors
policies
```

The full vocabularies belong in schema files. V1 implementations need only the
subset listed in §14. Adapters declare support per term; vocabularies can grow
without breaking existing projects.

V1 scope:

```text
Operation kinds: command, query
Steps:           partitioned into write / read / return groups
Errors:          validation-failed, unauthorized, forbidden, not-found,
                 conflict, server-error
Policies:        auth-required, permission-required, transaction-required
```

Subscriptions and jobs are deferred to v2. Their lifecycle schemas are not part
of the v1 implementation. They should be added when the first adapter declares
support for them.

---

## 9. Multi-Target Buildfile

The buildfile is the resolved per-feature contract that codegen reads. It gains
a `targets:` block — one entry per registered adapter — alongside its existing
sections.

```yaml
feature: task-list
adapter-set: default

operations:
  '@task-list/operation:task.create':
    kind: command
    subject: { entity: Task }
    input:  { type: CreateTaskInput }
    output: { shape: one, entity: Task }
    errors: [validation-failed, conflict]
    policies: [transaction-required]
    steps:
      - { type: validate-input }
      - { type: create-one, entity: Task, identity: generated }
      - { type: return-one, entity: Task }

targets:
  presentation:
    adapter: react-antd
    components:
      TaskCreateForm:
        source: '@task-list/add-task'
        actions:
          - name: submit
            effect:
              type: call
              operation: '@task-list/operation:task.create'

  transport:
    adapter: openapi-rest
    operations:
      '@task-list/operation:task.create':
        exposure: rest-endpoint
        method: POST
        path: /tasks

  application:
    adapter: nestjs-application
    operations:
      '@task-list/operation:task.create':
        handler: command-handler

  persistence:
    adapter: prisma-postgres
    repositories:
      TaskRepository:
        entity: Task
        supports: [create-one, read-one, read-many]

plan:
  targets:
    presentation:
      creates:
        - path: apps/web/src/features/tasks/TaskCreateForm.tsx
    transport:
      creates:
        - path: apps/api/src/tasks/tasks.controller.ts
    application:
      creates:
        - path: apps/api/src/tasks/tasks.service.ts
    persistence:
      modifies:
        - path: apps/api/prisma/schema.prisma
```

Canonical fields such as `steps`, `errors`, `policies`, `input`, and `output`
are declared once in `operations:` and are not restated in target sections.
Restating canonical fields fails validation because two prompt sources for the
same fact give the AI ambiguous structural input.

The buildfile's existing `wiring.rules` and `bindings` sections continue to
handle layout-node ↔ surface-fragment ↔ domain-element resolution. They coexist
with `operations` and `targets`:

```text
bindings  answer: what does this layout button invoke?
targets   answer: how is that invocation implemented across the stack?
```

Both layers reference the same canonical operation refs; cross-layer drift fails
validation.

## 9.1 Existing Buildfile Fields

The buildfile already contains fields predating this proposal. The multi-target
extension adds `operations:` and `targets:`; existing fields stay, are
deprecated, or are repurposed:

```text
adapter           Replaced by adapter-set + per-target adapter declarations.
                  Legacy buildfiles map adapter → adapter-set with a
                  single-target presentation set; field removed in v0.3.

components        Stays. Now lives under targets.presentation.components.
                  Legacy top-level components normalize to targets.presentation
                  on first regeneration.

routes            Stays. Routes belong under targets.presentation
                  (client-side routes) or targets.transport (HTTP routes).
                  Legacy top-level routes normalize to targets.presentation
                  unless adapter-set has a transport target with explicit
                  HTTP exposure for the same path.

models            Deprecated. domain-model.yaml is the canonical home for
                  entities. Per-feature model duplication is dropped at
                  build-feature time; the buildfile resolves entities from
                  domain-model.yaml during normalization.

fixtures          Stays. Per-feature fixture data continues to feed
                  testcases.yaml and adapter-generated test scaffolding.

cross-cutting     Mostly empty after the rename. Pattern fragments that
                  previously lived in infrastructure decompose elsewhere
                  (§13.1). The cross-cutting section retains adapter-level
                  cross-cutting metadata that does not fit operations,
                  domain-model, blueprint, or adapter-set.

plan              Stays, extended to be target-aware. Each target's plan
                  entries name the files that target generates.

wiring.rules /    Stay unchanged. The layout-aware-build feature added these;
bindings          they coexist with operations and targets, sharing canonical
                  operation refs.
```

Build-feature normalizes legacy buildfiles into the new shape on first
regeneration. Designer review surfaces the deprecation of `models:` and any
ambiguous routing decisions.

---

## 10. Testcases and the Coverage Review Gate

The repository already has a `testcases.yaml` schema for presentation tests
(rendering, click, verify). It extends to backend tests through a discriminated
suite kind in one file.

```yaml
schema_version: 2
feature: task-list

suites:
  - id: task-create-form-empty-render
    kind: presentation
    name: TaskCreateForm empty render
    component: TaskCreateForm
    fixture: empty-tasks
    source_refs:
      - surface.fragments['TaskCreateForm']
    cases:
      - name: renders form
        steps:
          - { action: render, target: TaskCreateForm }

  - id: task-create-operation
    kind: operation
    name: task.create
    operation: '@task-list/operation:task.create'
    source_refs:
      - capabilities.operations['task.create']
      - surface.fragments['TaskCreateForm'].actions['submit']
    cases:
      - name: creates task with valid input
        type: success
        act:
          operation: '@task-list/operation:task.create'
          input: { text: Buy milk, priority: medium }
        assert:
          output: { shape: one, entity: Task }
          persistence:
            created:
              entity: Task
              fields: { text: Buy milk, priority: medium }

      - name: rejects empty text
        type: error
        act:
          operation: '@task-list/operation:task.create'
          input: { text: '' }
        assert:
          error: validation-failed
```

Every canonical operation declared in `capabilities.yaml` requires at least one
`kind: operation` suite. Presentation-only projects with no `capabilities.yaml`
require no operation suites.

Every new or regenerated v2 suite must cite at least one `source_refs` entry.
Legacy schema v1 presentation suites load as presentation suites; missing
`source_refs` are warnings until the project regenerates v2 testcases.
Operation suites always require `source_refs` in build mode.

## 10.1 Why a Coverage Review Gate Exists

Codegen produces both the implementation and the tests. Without a human
checkpoint between testcase authoring and codegen, the AI authors the operation,
authors the tests, and runs the tests — grading its own homework. Source refs
alone do not prevent vacuous tests that cite real contracts and verify nothing.

The gate is a small artifact:

```text
.parlay/build/<feature>/coverage-review.yaml
```

It records human approval with hash-binding:

```yaml
feature: task-list
reviewed_at: 2026-05-09T14:32:00Z
reviewed_by: designer@example.com
review_method: local-user

buildfile_hash: sha256:a3f8c2...
testcases_hash: sha256:9c1ed4...

approved_suites:
  - task-create-form-empty-render
  - task-create-operation

exemptions:
  - suite: task-create-operation
    item: error:server-error
    reason: Framework-level 500 handling is covered by the transport adapter test suite.
```

`parlay generate-code` refuses to run if the file is missing, the hashes do not
match, or any required approval is absent. Re-running `build-feature` changes
the hashes and invalidates the review, forcing re-approval.

A declared step, error, or policy without case coverage requires an explicit
exemption in `coverage-review.yaml`. No separate exemption schema and no closed
exemption marker vocabulary are required in v1; the review file is the exemption
mechanism.

`coverage-review.yaml` is a workflow integrity mechanism, not a cryptographic
security boundary. It records local human review intent. A future version may
add signed review records if stronger guarantees are needed.

V1 binds review to `buildfile_hash` and `testcases_hash`. A later version may
add a normalized `review_subject_hash` covering adapter-set, referenced
adapters, blueprint, and domain-model if the review surface needs to become
stricter.

---

## 11. Codegen Flow

```text
parlay generate-code may read:
  buildfile.yaml, testcases.yaml, coverage-review.yaml,
  adapter-set.yaml, adapters/*.adapter.yaml,
  blueprint.yaml, config.yaml, domain-model.yaml, source tree

parlay generate-code must not read:
  spec/intents/**
```

Generation is AI-driven. The contract is testcase-defined behavioral
conformance, not byte-equivalent output. Two regeneration runs of the same
buildfile must pass the same conformance suite; they need not produce identical
source files.

Generated-code hashes track ownership and drift, not behavioral proof.

Default generation order:

```text
persistence → application → transport → presentation
```

Each layer informs the next.

---

## 12. Validation

Validation runs in two modes:

```text
authoring  permits draft/migration stubs and reports warnings
build      strict mode used by build-feature and generate-code
```

The substantive rules:

- Every adapter referenced by an adapter set exists, with matching kind.
- Adapter `supports` declarations cover everything used by registered features.
  Unsupported behavior fails before codegen.
- Adapter-set links are enforced; cross-layer access outside declared relations
  fails.
- Every term used in `capabilities.yaml` is in its closed vocabulary.
- `capabilities.yaml` must parse to closed operations; prose-only fragments fail
  build mode.
- Every operation reference in `targets` and `bindings` resolves to the same
  canonical operation in `operations`.
- Every canonical operation has at least one operation-suite testcase, or is
  exempted with human approval in `coverage-review.yaml`.
- New v2 presentation suites and all operation suites require `source_refs` in
  build mode.
- Codegen refuses to run when `coverage-review.yaml` is missing, stale, or
  hash-mismatched.

The full rule list belongs in the schema files when they are written.

---

## 13. Migration

Existing projects continue to work without modification.

```text
Existing adapters
  missing kind defaults to presentation.

prototype-framework
  legacy field; parlay migrate-config converts to a single-target adapter set;
  removal scheduled v0.3.

Existing buildfiles
  adapter/components/routes load as targets.presentation.

Existing testcases
  suite kind defaults to presentation; legacy intent becomes source_refs[0];
  schema_version 1 loads as v2. Missing source_refs are warnings until v2
  regeneration.

surface.md
  parlay migrate-spec converts to surface.yaml. Until migration, markdown is
  parsed into the same internal representation. Build/codegen consume the
  normalized model.

infrastructure.md
  renamed to capabilities.yaml. parlay migrate-capabilities converts
  operation-shaped fragments into closed operations. Prose-only fragments are
  invalid in build mode after migration.

domain-model operations
  parlay validate flags free-text effects as domain-operations-deprecated;
  parlay migrate-domain-operations lifts them into capabilities stubs with
  kind: unknown for designer authoring.
```

## 13.1 Pattern-Fragment Decomposition

Existing `infrastructure.md` files often contain pattern-flavored fragments
such as registries, pipelines, dispatchers, and helpers. These do not directly
migrate into capabilities.

The migrator is intentionally conservative:

```text
parlay migrate-capabilities auto-converts only operation-shaped fragments.
Pattern-shaped fragments are grouped into a migration report with suggested
  destinations.
Designers decide whether and where to move them.
```

Suggested destinations:

```text
Pipeline fragments       → command operations in capabilities; jobs in v2.
Registry fragments       → domain entity + register/list/lookup operations.
Dispatcher fragments     → transport adapter routing logic.
Traversal fragments      → query operation with read-tree when supported.
Resolver fragments       → query operation.
Validator fragments      → validate-input step or shared step.
Aspect fragments         → operation policies.
Cache fragments          → cacheable policy + blueprint.data.caching.
Migrator fragments       → domain-model state machines + migration command.
Hook fragments           → subscription in v2, or adapter lifecycle config.
Helper/utility fragments → adapter-level patterns; not spec-layer.
```

Fragments that do not clearly fit surface as warnings for designer review.

---

## 14. V1 Scope

The first implementation should cover:

```text
Adapter kinds      presentation, transport, application, persistence
Operation kinds    command, query
Steps              validate-input, authorize, create-one, update-one,
                   delete-one, read-one, read-many, search,
                   return-one, return-many, return-empty
Errors             validation-failed, unauthorized, forbidden, not-found,
                   conflict, server-error
Policies           transaction-required, auth-required, permission-required
First preset       react-antd + openapi-rest + nestjs-application + prisma-postgres
Coverage review    required before codegen for new or regenerated v2 testcases;
                   legacy presentation-only projects start with warnings until
                   they migrate/regenerate v2 testcases
```

Subscriptions and jobs are deferred to v2. Additional steps, errors, and
policies extend the closed lists in later versions; adapter `supports`
declarations mark which terms each adapter implements.

---

## 15. Key Risks

1. **Vocabulary overreach** — start with the v1 subset; extend only when
   adapters need it.
2. **Adapter coupling** — link enforcement prevents cross-layer access outside
   declared relations.
3. **Buildfile complexity** — canonical fields declared once; targets carry
   projection metadata only. Restating canonical fields fails validation.
4. **Hollow testcases** — coverage review gate requires human approval before
   codegen runs; AI cannot grade its own homework.
5. **Behavioral drift across regenerations** — verified by passing the testcase
   suite, not by source comparison. Closed vocabularies and review-approved
   testcases minimize the prompt-context surface where AI interpretation can
   diverge.
6. **Migration ambiguity** — pattern-shaped legacy infrastructure fragments are
   reported for designer routing instead of being aggressively auto-converted.

---

## 16. Architecture Summary

```text
spec/intents/<feature>/surface.yaml          Closed UI vocabulary.
spec/intents/<feature>/capabilities.yaml     Closed backend-operation vocabulary;
                                             renamed and reframed from infrastructure.
<activeRoot>/domain-model.yaml               Domain nouns; operations field deprecated.
.parlay/adapter-set.yaml                     Active multi-adapter topology.
.parlay/adapters/*.adapter.yaml              Per-kind adapters with kind + supports + patterns.
.parlay/build/<feature>/buildfile.yaml        Multi-target resolved contract.
.parlay/build/<feature>/testcases.yaml        Unified suite: presentation + operation.
.parlay/build/<feature>/coverage-review.yaml  Human approval gate before codegen.
```

The spec layer is three artifacts:

```text
surface       what the user touches
capabilities  what the system does
domain-model  what the data is
```

The core principles:

- Model backend work as typed operations, not prose.
- Use multiple adapters of different kinds, each closed against its own
  vocabulary, composed through an adapter set.
- Keep `wiring/bindings` and `operations/targets` as separate concerns in the
  buildfile; they share canonical operation refs.
- Verify behavior through tests, not byte comparison.
- Gate codegen on human approval of the testcases.

This extends Parlay's existing nature without changing it: human-reviewed intent
flows into typed tool-owned buildfiles, and adapters translate closed
vocabularies into implementation.

