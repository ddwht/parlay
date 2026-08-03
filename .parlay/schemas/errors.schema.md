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

## Error representation is a codegen-time concern

How a declared error is represented in generated code — `conflict` → `ConflictException` / HTTP 409, Prisma `P2002` → `conflict`, and so on — is handled by each adapter's error-mapping **conventions**, applied by codegen. It is not a pre-codegen structured gate: there is no `error-no-mapping` validator. The supports gate already ensures every declared error is one some filled backend layer lists (union coverage); mapping that error onto a framework construct is then the adapter's job at generation time.
