<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Errors Schema

Closed vocabulary for the `errors:` list on a capability operation declared in `spec/intents/<feature>/capabilities.yaml`.

## Closed set

| Error | Description |
|---|---|
| `validation-failed` | Input did not satisfy the operation's input schema or business rules. |
| `unauthorized` | The caller is not authenticated. |
| `forbidden` | The caller is authenticated but lacks permission for the operation. |
| `not-found` | The operation references an entity that does not exist. |
| `conflict` | The operation cannot proceed because of a state conflict (duplicate identifier, version mismatch, concurrent edit). |
| `server-error` | An unexpected error occurred on the server. |

An `errors:` entry outside this set fails capability validation with `capabilities-unknown-term`. An adapter declaring `supports.errors: [...]` whose entries fall outside this set fails validation with `adapter-supports-unknown-term`.

## Mapping requirement

For every operation, every declared error MUST have a mapping at one of the layers — adapter, adapter-set, or blueprint. The blueprint resolver walks the layered settings and confirms a mapping exists. Unmapped errors fail validation with `error-no-mapping` naming the missing layer.
