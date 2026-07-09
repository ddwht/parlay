<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Capabilities Schema

File: `spec/intents/<feature>/capabilities.yaml`. The closed-vocabulary backend artifact declaring the operations a feature exposes — commands and queries against domain entities, with input, steps, output shape, and allowed errors.

Operation-shaped content lives here. Architectural prose for boundaries, probes, allowlists, and dependency pins lives in `infrastructure.md` — see `infrastructure.schema.md`. The two artifacts are co-equal and cover orthogonal concerns: `capabilities.yaml` answers "what does the backend do?" and `infrastructure.md` answers "what shape must the codebase hold for those operations to work safely?" Neither artifact is a stand-in for the other; many features have both.

## Structure

```yaml
schema_version: 1
feature: <feature-slug>

operations:
  - id: <feature-local id, e.g., task.create>
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

| Field | Required | Description |
|---|---|---|
| `schema_version` | Yes | Currently `1`. |
| `feature` | Yes | Feature slug; must match the directory name. |
| `operations` | Yes | List of capability operations. May be empty for presentation-only features. |
| `operations[].id` | Yes | Feature-local identifier (e.g., `task.create`). Normalized to `@<feature>/operation:<id>` on the way into the buildfile. |
| `operations[].kind` | Yes | One of the values in `operation-kinds.schema.md`. |
| `operations[].subject` | Yes | The primary entity the operation acts on. |
| `operations[].input` | No | Input contract; absent for parameterless queries. |
| `operations[].output` | No | Output contract — `shape` is `one`, `many`, or `empty`; `entity` names the returned entity for non-empty shapes. |
| `operations[].errors` | No | Errors the operation may emit. Every entry must come from the `errors.schema.md` closed set. |
| `operations[].policies` | No | Policies the operation enforces. Every entry must come from the `policies.schema.md` closed set. |
| `operations[].steps` | Yes | Ordered list of steps. Each step's `type:` must come from the `steps.schema.md` closed set. |

## Validation rules

The capabilities validator enforces:

| Code | When it fires |
|---|---|
| `capabilities-unknown-term` | A `kind`, `step.type`, `errors[]`, or `policies[]` entry falls outside its closed vocabulary. The fix message names v2-deferred terms (`subscription`, `job`) explicitly. |
| `capabilities-not-closed-form` | A capabilities document contains prose-only fragments instead of structured operations. Build mode fails; authoring mode warns. |
| `capabilities-duplicate-operation-id` | Two operations within one capabilities.yaml share the same id. |
| `capabilities-stub-unfilled` | An operation declares `kind: unknown` (the migrate-domain-operations stub kind). Build mode fails until the kind is set explicitly. |
| `buildfile-operation-ref-unnormalized` | A downstream buildfile references an operation by bare local id rather than the `@<feature>/operation:<id>` form. |

## Policy-step-error tie rules

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

## Relationship to blueprint's `authorization.policies`

Capabilities' `policies:` (this closed three-value enum) and `blueprint.yaml`'s `authorization.policies` (named, free-form per-resource rules — see `blueprint.schema.md` Section 3) are related but serve different consumers and are **not** currently tied by a shared identifier:

- Capabilities' `policies:` tells the **backend operation** which enforcement category applies (`auth-required`, `permission-required`, `transaction-required`) — it's what `build-feature` and the application-layer adapter need to know to wire the right guard around the operation's steps.
- Blueprint's `authorization.policies.<name>` tells the **frontend** which specific business rule governs an action's visibility (e.g., `task-deletion: { controls: "task deletion", rule: "owner or admin" }`) — it's consumed by generated components for show/hide decisions, per `blueprint.schema.md`'s Section 3.

A capability operation declaring `permission-required` says "this needs a permission check" without saying *which* blueprint policy supplies the concrete rule. Linking the two by name (e.g. an optional `policy-ref:` field on the operation) was considered and deferred here rather than added speculatively — nothing in the current pipeline consumes such a link (build-feature wires the backend guard from `policies:` alone; the frontend visibility rule is wired from the blueprint independently), so adding the field now would be an unused wire-contract addition. If a future feature needs the backend operation and the frontend visibility rule to provably reference the same policy, that field is the natural place to add it — this paragraph exists so that addition doesn't have to rediscover the reasoning.

## Normalization

`parser.NormalizeOperationID(feature, id) -> "@feature/operation:id"` is the single sanctioned path that turns the feature-local id into the buildfile-canonical reference. Validators reject any buildfile reference that didn't pass through this function.

## Backward compatibility

Features without `capabilities.yaml` continue to work — capability validation walks zero operations. Multi-target rules that consult `capabilities.yaml` short-circuit when the file is absent. The migration commands `parlay migrate-capabilities` and `parlay migrate-domain-operations` produce the file by moving operation-shaped content from `infrastructure.md` and `domain-model.operations` respectively; architectural-prose fragments in `infrastructure.md` are retained in place by design.
