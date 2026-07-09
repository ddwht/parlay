<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Steps Schema

Closed vocabulary for the `steps:` list on a capability operation declared in `spec/intents/<feature>/capabilities.yaml`. Each step's `type:` must come from the closed set defined here, grouped by purpose.

## Closed set

### Write group

Steps that mutate persistent state.

| Step | Description |
|---|---|
| `validate-input` | Validate the operation's input against schema and business rules. |
| `authorize` | Confirm the caller is permitted to perform the operation. |
| `create-one` | Persist a new entity instance. |
| `update-one` | Modify an existing entity instance. |
| `delete-one` | Remove an existing entity instance. |

### Read group

Steps that read persistent state without mutating it.

| Step | Description |
|---|---|
| `read-one` | Fetch a single entity by identifier. |
| `read-many` | Fetch a collection of entities. |
| `search` | Fetch entities matching a query. |

### Return group

Steps that produce the operation's output.

| Step | Description |
|---|---|
| `return-one` | Return a single entity instance. |
| `return-many` | Return a collection of entities. |
| `return-empty` | Return without a body (used for commands whose effect is the only output). |

A step `type:` outside the union of these three groups fails capability validation with `capabilities-unknown-term`. An adapter declaring `supports.steps: [...]` whose entries fall outside these groups fails validation with `adapter-supports-unknown-term`.

## v2-deferred terms

An operation's steps today can only read and write the operation's own subject entity — there is no step for calling an external service, emitting an event to another system, or running a pure computation over already-loaded data. The following step-family additions were considered for this closed set and are **deferred**, following the same posture as `operation-kinds.schema.md`'s `subscription`/`job` deferral:

| Step | Status |
|---|---|
| `call-external` | v2-deferred — invoking an external service or third-party API mid-operation is deferred to v2 |
| `emit-event` | v2-deferred — publishing a domain event for other systems to consume is deferred to v2 |
| `compute` | v2-deferred — a pure in-memory transform step with no read/write side effect is deferred to v2 |

When a v1 capability declares one of these three step types, validation fails with `capabilities-unknown-term` — the same code as any other out-of-vocabulary term. The accompanying fix message names the term as v2-deferred so authors understand it is reserved rather than a typo, exactly as `operation-kinds.schema.md` does for `subscription` and `job`. No adapter `supports.steps:` entry may declare these either, for the same reason.

**Why deferred rather than added now:** each of the three needs adapter-side work this schema doesn't yet specify — `call-external` needs a way to declare which external services an adapter can reach and how errors from them map to the closed `errors.schema.md` set; `emit-event` needs an event-bus vocabulary parallel to `steps`/`policies`/`errors` that doesn't exist yet; `compute` needs a decision on whether its transform logic is declarative (like `filter`/`transform` in `buildfile.schema.md`'s operation types) or prose-described. Adding any of the three without settling those questions would lock in an under-specified shape. Deferral keeps the v1 closed set honest about what it actually supports today.
