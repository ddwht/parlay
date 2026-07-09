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

## Normalization

`parser.NormalizeOperationID(feature, id) -> "@feature/operation:id"` is the single sanctioned path that turns the feature-local id into the buildfile-canonical reference. Validators reject any buildfile reference that didn't pass through this function.

## Backward compatibility

Features without `capabilities.yaml` continue to work — capability validation walks zero operations. Multi-target rules that consult `capabilities.yaml` short-circuit when the file is absent. The migration commands `parlay migrate-capabilities` and `parlay migrate-domain-operations` produce the file by moving operation-shaped content from `infrastructure.md` and `domain-model.operations` respectively; architectural-prose fragments in `infrastructure.md` are retained in place by design.
