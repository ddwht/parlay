# Capabilities Schema — authoring digest

Derived from `capabilities.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

```yaml
schema_version: 2
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

`schema_version` (see `schema-versioning.schema.md` for the house rule) is currently `2`. Version 2 is the shape carrying `source:` on every operation, and declaring it makes `capabilities-source-missing` an **error** rather than a warning — a file at the current shape is one where the provenance could have been recorded. **Policy: regenerate.** `capabilities.yaml` is tool-derived from intents/dialogs via `/parlay-create-artifacts` (and populated by the migration commands `migrate-capabilities`/`migrate-domain-operations` for pre-existing content). A stale `schema_version` is a signal to re-run the producing command, not to migrate the file in place — there's no hand-authored state in a v1 capabilities file that a migrator would need to preserve beyond what regeneration already reconstructs from intents.

| Field | Required | Description |
|---|---|---|
| `schema_version` | Yes | Currently `2`. |
| `feature` | Yes | Feature slug; must match the directory name. |
| `operations` | Yes | List of capability operations. May be empty for presentation-only features. |
| `operations[].id` | Yes | Feature-local identifier (e.g., `task.create`). Normalized to `@<feature>/operation:<id>` on the way into the buildfile. |
| `operations[].source` | Required on generate, tolerated absent on read | Comma-separated `@feature/intent-slug` traceability references — the same shape `surface.yaml`'s `fragments[].source` uses. See "Why `source:` exists" below. |
| `operations[].verify` | No | Acceptance criteria, one line each — the owning intent's **contract claims**: input validation, state change, output shape, allowed errors. Transport-independent by construction; a claim about what the user *sees* belongs on the surface fragment that shows it (`surface.schema.md` § Verify), not here. Relocated by `/parlay-create-artifacts` on generate and by `parlay migrate-verify` for pre-existing artifacts. Testcase derivation reads these; since v0.3 there is no intent-bullet fallback — a missing verify: means run `parlay migrate-verify`. |
| `operations[].rationale` | No | One line of provenance prose — why the operation exists. Never validated beyond being a string. |
| `operations[].kind` | Yes | One of the values in `operation-kinds.schema.md`. |
| `operations[].subject` | Yes | The primary entity the operation acts on. |
| `operations[].input` | No | Input contract; absent for parameterless queries. `input.type` names an ad-hoc input DTO — see "The `input.type` namespace" below. |
| `operations[].output` | No | Output contract — `shape` is `one`, `many`, or `empty`; `entity` names the returned entity for non-empty shapes. |
| `operations[].errors` | No | Errors the operation may emit. Every entry must come from the `errors.schema.md` closed set. |
| `operations[].policies` | No | Policies the operation enforces. Every entry must come from the `policies.schema.md` closed set. |
| `operations[].steps` | Yes | Ordered list of steps. Each step's `type:` must come from the `steps.schema.md` closed set. |

---

`input.type` (e.g., `CreateTaskInput`) is **not** a reference into a closed vocabulary the way `subject.entity`/`output.entity` are references into `domain-model.yaml`'s declared entities — those two are cross-checked against the resolved root's domain model and fail with `capabilities-entity-undeclared`, which `input.type` has no equivalent of. There is no `types:` registry anywhere in the parlay artifact set today — `input.type` is a free-form descriptive name, unvalidated by the capabilities validator, that exists purely for human and AI readability when reading the operation. The Go representation (`parser.CapabilityIO.Type`) is a plain string with no cross-reference check.

Concretely:

- **What may appear**: any string. Convention (not enforced) is `<Verb><Entity>Input` — `CreateTaskInput`, `UpdateTaskInput` — but a feature-local shorthand is equally valid.
- **Where the DTO's actual field shape is declared**: nowhere, structurally. Unlike a domain entity's fields (which `domain-model.schema.md` closes over `DomainField`'s type set), an input DTO's fields are inferred by whoever consumes the capability — `build-feature` when it wires the operation into a buildfile, or an application-layer adapter's codegen — from context: the named entity in `subject`, the operation's `kind`, and the intent that produced the capability. This is a real, current limitation, not an oversight papered over: input shapes are so often a proper *subset* of an entity's fields (a create input omits `id`/`created_at`; an update input might omit most fields the caller isn't changing) that reusing `DomainEntity` directly would either be wrong (claims fields that aren't actually accepted) or require a second, parallel "partial entity" concept this schema doesn't have.
- **A natural next step, not undertaken here**: a `types:` section paralleling `domain-model.yaml`'s entities — closed field lists per named input DTO — would give `input.type` the same closed-vocabulary treatment `subject.entity` already has. That's new schema surface, not a naming-namespace clarification, so it's out of scope for this consolidation; this section exists so the gap is documented rather than silently assumed away.

---

The capabilities validator enforces:

| Code | When it fires |
|---|---|
| `capabilities-unknown-term` | A `kind`, `step.type`, `errors[]`, or `policies[]` entry falls outside its closed vocabulary. The fix message names v2-deferred terms (`subscription`, `job`) explicitly. |
| `capabilities-not-closed-form` | A capabilities document contains prose-only fragments instead of structured operations. Build mode fails; authoring mode warns. |
| `capabilities-duplicate-operation-id` | Two operations within one capabilities.yaml share the same id. |
| `capabilities-stub-unfilled` | An operation declares `kind: unknown` (the migrate-domain-operations stub kind). Build mode fails until the kind is set explicitly. |
| `capabilities-subject-missing` | An operation declares no `subject.entity`. Required for every operation — the downstream wiring is derived from it. |
| `capabilities-source-missing` | An operation declares no `source:`, so nothing records which intent it came from and a change described in prose cannot be routed to it. **Warning below `schema_version: 2`, error at 2 or above** — every capabilities.yaml predates the field, and a file declaring the current shape is one that could have recorded it. |
| `capabilities-entity-undeclared` | `subject.entity` or `output.entity` names an entity that `domain-model.yaml` does not declare **and no feature's contribution proposes**. Requires a resolvable domain model: with none, the cross-reference is skipped rather than failing every operation, since a project that has not authored a domain model yet is a normal state. |
| `capabilities-entity-pending` (warning) | The referenced entity is not in the root model yet, but a feature's `spec/intents/<feature>/domain-model.yaml` contribution proposes it. The finding names the proposing feature. This case used to be indistinguishable from a typo — both graded as errors — so a feature referencing an entity a sibling was about to introduce had to ship a placeholder. Accept the proposing contribution and the reference resolves. |
| `buildfile-operation-ref-unnormalized` | A downstream buildfile references an operation by bare local id rather than the `@<feature>/operation:<id>` form. |

---

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

---

`parser.NormalizeOperationID(feature, id) -> "@feature/operation:id"` is the single sanctioned path that turns the feature-local id into the buildfile-canonical reference. Validators reject any buildfile reference that didn't pass through this function.
