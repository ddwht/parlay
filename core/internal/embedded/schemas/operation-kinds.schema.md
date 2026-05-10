<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Operation Kinds Schema

Closed vocabulary for the `kind:` field on a capability operation declared in `spec/intents/<feature>/capabilities.yaml`.

## Closed set

| Kind | Description |
|---|---|
| `command` | A state-changing operation. Validates input, mutates state, returns the result of the change. |
| `query` | A read-only operation. Reads state, returns the result without mutation. |

A `kind:` value outside this set fails capability validation with `capabilities-unknown-term` naming the offending value.

## v2-deferred terms

The following kinds are reserved for v2 and are NOT permitted in v1:

| Kind | Status |
|---|---|
| `subscription` | v2-deferred — streaming / long-lived operations are deferred to v2 |
| `job` | v2-deferred — background / asynchronous operations are deferred to v2 |

When a v1 capability declares `kind: subscription` or `kind: job`, validation fails with `capabilities-unknown-term`. The accompanying fix message names the term as v2-deferred so authors understand it is reserved rather than a typo.
