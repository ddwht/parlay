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
