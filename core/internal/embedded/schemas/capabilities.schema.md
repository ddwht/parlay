<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Capabilities Schema

File: `spec/intents/<feature>/capabilities.yaml`. The closed-vocabulary backend artifact declaring the operations a feature exposes — commands and queries against domain entities, with input, steps, output shape, and allowed errors.

Operation-shaped content lives here. Architectural prose for boundaries, probes, allowlists, and dependency pins lives in `infrastructure.md` — see `infrastructure.schema.md`. The two artifacts are co-equal and cover orthogonal concerns: `capabilities.yaml` answers "what does the backend do?" and `infrastructure.md` answers "what shape must the codebase hold for those operations to work safely?" Neither artifact is a stand-in for the other; many features have both.

## Structure
<!-- parlay:normative -->



```yaml
schema_version: 1
feature: <feature-slug>

operations:
  - id: <feature-local id, e.g., task.create>
    source: <comma-separated @feature/intent-slug references>
    verify:
      - <acceptance criterion, one line>
    rationale: <one line of provenance prose>
    kind: <command | query>
    subject:
      entity: <EntityName from domain-model>
    input:
      type: <InputTypeName>
    output:
      shape: <one | many | empty>
      entity: <EntityName>
    errors:
      - <error from errors.schema.md closed set>
    policies:
      - <policy from policies.schema.md closed set>
    steps:
      - { type: <step from steps.schema.md closed set>, ... }
```

### Versioning

`schema_version` (see `schema-versioning.schema.md` for the house rule) is currently `1`. **Policy: regenerate.** `capabilities.yaml` is tool-derived from intents/dialogs via `/parlay-create-artifacts` (and populated by the migration commands `migrate-capabilities`/`migrate-domain-operations` for pre-existing content). A stale `schema_version` is a signal to re-run the producing command, not to migrate the file in place — there's no hand-authored state in a v1 capabilities file that a migrator would need to preserve beyond what regeneration already reconstructs from intents.

| Field | Required | Description |
|---|---|---|
| `schema_version` | Yes | Currently `1`. |
| `feature` | Yes | Feature slug; must match the directory name. |
| `operations` | Yes | List of capability operations. May be empty for presentation-only features. |
| `operations[].id` | Yes | Feature-local identifier (e.g., `task.create`). Normalized to `@<feature>/operation:<id>` on the way into the buildfile. |
| `operations[].source` | Required on generate, tolerated absent on read | Comma-separated `@feature/intent-slug` traceability references — the same shape `surface.yaml`'s `fragments[].source` uses. See "Why `source:` exists" below. |
| `operations[].verify` | No | Acceptance criteria, one line each — relocated from the owning intent's **Verify** bullets by `/parlay-create-artifacts` on generate and by `parlay migrate-verify` for pre-existing artifacts. Testcase derivation reads these; since v0.3 there is no intent-bullet fallback — a missing verify: means run `parlay migrate-verify`. |
| `operations[].rationale` | No | One line of provenance prose — why the operation exists. Never validated beyond being a string. |
| `operations[].kind` | Yes | One of the values in `operation-kinds.schema.md`. |
| `operations[].subject` | Yes | The primary entity the operation acts on. |
| `operations[].input` | No | Input contract; absent for parameterless queries. `input.type` names an ad-hoc input DTO — see "The `input.type` namespace" below. |
| `operations[].output` | No | Output contract — `shape` is `one`, `many`, or `empty`; `entity` names the returned entity for non-empty shapes. |
| `operations[].errors` | No | Errors the operation may emit. Every entry must come from the `errors.schema.md` closed set. |
| `operations[].policies` | No | Policies the operation enforces. Every entry must come from the `policies.schema.md` closed set. |
| `operations[].steps` | Yes | Ordered list of steps. Each step's `type:` must come from the `steps.schema.md` closed set. |

<!-- /parlay:normative -->

## Why `source:` exists

Every other artifact records where its content came from. `surface.yaml` fragments carry `source:`, `infrastructure.md` fragments carry `Source:`, and buildfile cross-cutting entries carry one too. `capabilities.yaml` was alone in having none, and the asymmetry was invisible for as long as traceability was only ever walked **forwards** — from intent, to artifact, to buildfile, to test.

The reverse walk is what needs it. Given a change described in a person's words — "the approval step should also notify the requester" — something has to answer *which artifact owns that*. For a surface change the fragment's `source:` answers it. For a backend change there was nothing to answer with, so a backend refinement could not be routed to the operation it belongs to, and the only remaining options were to guess by name-similarity or to re-derive the whole artifact from intents. The first blesses contradictions; the second discards a reviewed document to change one line of it.

**Required on generate, tolerated absent on read.** Every `capabilities.yaml` in existence was written before the field, and erroring on read would fail all of them at once over a fact none could have recorded — the same reasoning `testcases-file-missing` follows. `/parlay-create-artifacts` and the migration commands populate it going forward; a file without it still loads, and the reverse walk degrades to asking rather than failing.

`source:` is traceability, not derivation. It records which intent an operation came from. It does not license the build phase to read intents — the buildfile remains the executable contract, and this field travels into it as data like any other.

## The `input.type` namespace
<!-- parlay:normative -->



`input.type` (e.g., `CreateTaskInput`) is **not** a reference into a closed vocabulary the way `subject.entity`/`output.entity` are references into `domain-model.yaml`'s declared entities — those two are cross-checked against the resolved root's domain model and fail with `capabilities-entity-undeclared`, which `input.type` has no equivalent of. There is no `types:` registry anywhere in the parlay artifact set today — `input.type` is a free-form descriptive name, unvalidated by the capabilities validator, that exists purely for human and AI readability when reading the operation. The Go representation (`parser.CapabilityIO.Type`) is a plain string with no cross-reference check.

Concretely:

- **What may appear**: any string. Convention (not enforced) is `<Verb><Entity>Input` — `CreateTaskInput`, `UpdateTaskInput` — but a feature-local shorthand is equally valid.
- **Where the DTO's actual field shape is declared**: nowhere, structurally. Unlike a domain entity's fields (which `domain-model.schema.md` closes over `DomainField`'s type set), an input DTO's fields are inferred by whoever consumes the capability — `build-feature` when it wires the operation into a buildfile, or an application-layer adapter's codegen — from context: the named entity in `subject`, the operation's `kind`, and the intent that produced the capability. This is a real, current limitation, not an oversight papered over: input shapes are so often a proper *subset* of an entity's fields (a create input omits `id`/`created_at`; an update input might omit most fields the caller isn't changing) that reusing `DomainEntity` directly would either be wrong (claims fields that aren't actually accepted) or require a second, parallel "partial entity" concept this schema doesn't have.
- **A natural next step, not undertaken here**: a `types:` section paralleling `domain-model.yaml`'s entities — closed field lists per named input DTO — would give `input.type` the same closed-vocabulary treatment `subject.entity` already has. That's new schema surface, not a naming-namespace clarification, so it's out of scope for this consolidation; this section exists so the gap is documented rather than silently assumed away.

<!-- /parlay:normative -->

## Validation rules
<!-- parlay:normative -->



The capabilities validator enforces:

| Code | When it fires |
|---|---|
| `capabilities-unknown-term` | A `kind`, `step.type`, `errors[]`, or `policies[]` entry falls outside its closed vocabulary. The fix message names v2-deferred terms (`subscription`, `job`) explicitly. |
| `capabilities-not-closed-form` | A capabilities document contains prose-only fragments instead of structured operations. Build mode fails; authoring mode warns. |
| `capabilities-duplicate-operation-id` | Two operations within one capabilities.yaml share the same id. |
| `capabilities-stub-unfilled` | An operation declares `kind: unknown` (the migrate-domain-operations stub kind). Build mode fails until the kind is set explicitly. |
| `capabilities-subject-missing` | An operation declares no `subject.entity`. Required for every operation — the downstream wiring is derived from it. |
| `capabilities-source-missing` (warning) | An operation declares no `source:`, so nothing records which intent it came from and a change described in prose cannot be routed to it. Warning in both modes: the field is required on generate but tolerated absent on read, since every capabilities.yaml predates it. |
| `capabilities-entity-undeclared` | `subject.entity` or `output.entity` names an entity that `domain-model.yaml` does not declare **and no feature's contribution proposes**. Requires a resolvable domain model: with none, the cross-reference is skipped rather than failing every operation, since a project that has not authored a domain model yet is a normal state. |
| `capabilities-entity-pending` (warning) | The referenced entity is not in the root model yet, but a feature's `spec/intents/<feature>/domain-model.yaml` contribution proposes it. The finding names the proposing feature. This case used to be indistinguishable from a typo — both graded as errors — so a feature referencing an entity a sibling was about to introduce had to ship a placeholder. Accept the proposing contribution and the reference resolves. |
| `buildfile-operation-ref-unnormalized` | A downstream buildfile references an operation by bare local id rather than the `@<feature>/operation:<id>` form. |

<!-- /parlay:normative -->

## Policy-step-error tie rules
<!-- parlay:normative -->



A `policies:` entry is a claim that the operation enforces something; that claim is only coherent if the operation also has a step that performs the check and an error that reports the check failing. Two of the three closed policies tie to a specific step and error pair:

| Policy | Required step | Required error | Why |
|---|---|---|---|
| `auth-required` | `authorize` (from `steps.schema.md`'s write group) | `unauthorized` (from `errors.schema.md`) | The policy claims the caller must be authenticated; without an `authorize` step there's nothing performing that check, and without `unauthorized` in `errors:` the operation has no way to report the check failing. |
| `permission-required` | `authorize` | `forbidden` | Same shape as `auth-required`, one level stricter — the caller is authenticated but must also hold a specific grant. Both policies share the `authorize` step (steps.schema.md doesn't split "is authenticated" from "is permitted" into separate step types); they differ in which error reports failure. |
| `transaction-required` | — (no tie) | — (no tie) | Transactionality is a property of how the *existing* write steps execute (wrapped in a transaction boundary), not a discrete step of its own — there is no `begin-transaction` step in `steps.schema.md`'s closed set, and a transaction failure surfaces as whichever of `conflict` or `server-error` fits the underlying cause, not one fixed error. This policy has no fixed tie for that reason; declaring it does not require a specific step or error entry. |

An operation declaring `auth-required` or `permission-required` without the tied step or error fails capability validation:

| Code | When it fires |
|---|---|
| `capabilities-policy-missing-step` | Operation declares `auth-required` or `permission-required` but its `steps:` list has no `authorize` entry. |
| `capabilities-policy-missing-error` | Operation declares `auth-required` without `unauthorized` in `errors:`, or `permission-required` without `forbidden` in `errors:`. |

The reverse is NOT required — an operation may declare an `authorize` step or the `unauthorized`/`forbidden` errors without declaring the corresponding policy (a step or error can exist for reasons the closed policy vocabulary doesn't capture). The tie is one-directional: policy implies step-and-error, not the other way around.

<!-- /parlay:normative -->

## Relationship to blueprint's `authorization.policies`

Capabilities' `policies:` (this closed three-value enum) and `blueprint.yaml`'s `authorization.policies` (named, free-form per-resource rules — see `blueprint.schema.md` Section 3) are related but serve different consumers and are **not** currently tied by a shared identifier:

- Capabilities' `policies:` tells the **backend operation** which enforcement category applies (`auth-required`, `permission-required`, `transaction-required`) — it's what `build-feature` and the application-layer adapter need to know to wire the right guard around the operation's steps.
- Blueprint's `authorization.policies.<name>` tells the **frontend** which specific business rule governs an action's visibility (e.g., `task-deletion: { controls: "task deletion", rule: "owner or admin" }`) — it's consumed by generated components for show/hide decisions, per `blueprint.schema.md`'s Section 3.

A capability operation declaring `permission-required` says "this needs a permission check" without saying *which* blueprint policy supplies the concrete rule. Linking the two by name (e.g. an optional `policy-ref:` field on the operation) was considered and deferred here rather than added speculatively — nothing in the current pipeline consumes such a link (build-feature wires the backend guard from `policies:` alone; the frontend visibility rule is wired from the blueprint independently), so adding the field now would be an unused wire-contract addition. If a future feature needs the backend operation and the frontend visibility rule to provably reference the same policy, that field is the natural place to add it — this paragraph exists so that addition doesn't have to rediscover the reasoning.

## Normalization
<!-- parlay:normative -->



`parser.NormalizeOperationID(feature, id) -> "@feature/operation:id"` is the single sanctioned path that turns the feature-local id into the buildfile-canonical reference. Validators reject any buildfile reference that didn't pass through this function.

<!-- /parlay:normative -->

## Backward compatibility

Features without `capabilities.yaml` continue to work — capability validation walks zero operations. Multi-target rules that consult `capabilities.yaml` short-circuit when the file is absent. The migration commands `parlay migrate-capabilities` and `parlay migrate-domain-operations` produce the file by moving operation-shaped content from `infrastructure.md` and `domain-model.operations` respectively; architectural-prose fragments in `infrastructure.md` are retained in place by design.
