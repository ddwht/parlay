<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Policies Schema

Closed vocabulary for the `policies:` list on a capability operation declared in `spec/intents/<feature>/capabilities.yaml`.

## Closed set

| Policy | Description |
|---|---|
| `auth-required` | The operation may only be invoked by an authenticated caller. |
| `permission-required` | The operation requires a specific permission grant; the granting authority depends on the project's authorization model. |
| `transaction-required` | The operation's steps must execute inside a transactional boundary; partial completion is not a valid outcome. |

A `policies:` entry outside this set fails capability validation with `capabilities-unknown-term`. An adapter declaring `supports.policies: [...]` whose entries fall outside this set fails validation with `adapter-supports-unknown-term`.
