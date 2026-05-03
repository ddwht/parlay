<!--
parlay-section: cross-cutting
parlay-feature: studio-support/adapter-vocabulary-extension
parlay-cross-cutting-id: layout-schema-universal-container-fields
-->

# Layout Schema

File: `spec/intents/<feature>/<page>.layout.yaml` (and the in-buildfile layout regions emitted by `/parlay-build-feature`).

A **layout** describes the visual / structural composition of a page using components drawn from the active adapter's `componentVocabulary:` (see `adapter.schema.md` Section 8). Layouts pin themselves to a vocabulary version (e.g., `clarity@17`) so that a layout authored against one revision of a design system fails fast when the adapter advertises a different revision rather than silently rendering against an unintended vocabulary.

This file defines the **universal container fields** that every layout-author may use on every container node, regardless of which adapter is active. The full layout schema (node shape, child arrays, leaf-component properties, vocabulary pinning syntax) is owned by `@studio-support/page-layout-field` and will be added here as that feature lands. This stub establishes the universal-container-fields contract so the vocabulary-extension and layout-pipeline features can refer to a single source of truth for container chrome.

## Universal container fields

The following four fields are available on **every** container node in **every** vocabulary, regardless of which adapter is active:

| Field | Value type | Description |
|---|---|---|
| `direction` | enum: `{horizontal, vertical}` | Layout axis along which children flow. |
| `gap` | token-reference (spacing token) | Inter-child spacing — references a token from the active adapter's `tokens.spacing` (e.g., `spacing-sm`, `spacing-lg`). Raw values like `24px` are rejected with `raw-value-where-token-required` listing the available spacing tokens. |
| `padding` | token-reference (spacing token) | Internal padding around the container's children — references a spacing token. Same raw-value rejection rule as `gap`. |
| `alignment` | enum: `{start, center, end, stretch}` | Cross-axis alignment of children within the container. |

These fields live in the **layout schema** — NOT in adapter `componentVocabulary` entries. The set is fixed at `{direction, gap, padding, alignment}`.

## Uniformity contract

The value type for each universal field is **uniform across vocabularies**:

- `gap` is always a `token-reference` (specifically into `tokens.spacing`). It is never a raw number, never a string literal, and never an enum, regardless of which design system the active adapter implements.
- `direction` is always one of the fixed enum `{horizontal, vertical}`.
- `padding` is always a `token-reference` (into `tokens.spacing`).
- `alignment` is always one of the fixed enum `{start, center, end, stretch}`.

This uniformity is what lets the layout schema own these fields — they have one shape across every adapter, so there is no per-vocabulary variation to capture inside `componentVocabulary`.

## Universal-fields rule

An adapter MUST NOT re-declare any universal field inside its `componentVocabulary` entries. An adapter that declares (e.g.) `direction` as a property of `clarity.region` fails parse with `universal-field-redeclared` naming the offending component and field.

This is enforced at **adapter parse time**, not at layout-validate time — the failure surfaces when the adapter file is loaded via `parlay register-adapter`, before any layout has a chance to reference it.

## Migration posture

**Existing adapter files** that re-declared a universal field inside a `componentVocabulary` entry need a one-time migration step to strip the re-declaration. The migration is mechanical: locate the offending property entry inside the component, delete it, and re-register the adapter. After the strip, the universal field is still available on every container node — the only difference is that the value type is now sourced from the layout schema rather than from the adapter.

**New adapter files** reject the re-declaration at parse time, so the violation cannot reach disk in a freshly-authored adapter.

**Adding a new universal field** is a layout-schema change — not a vocabulary change. It requires migrating every adapter to confirm none of them re-declare the new field, and it requires bumping the layout schema version. This is rare; the four-field set above is intentionally conservative.

## Out of scope (for this stub)

The following layout-schema concerns are owned by `@studio-support/page-layout-field` and will be documented in subsequent revisions of this file:

- Top-level layout file structure (`layout:`, `vocabulary:`, `mode:`).
- Vocabulary pinning syntax (e.g., `vocabulary: clarity@17`) and the version-mismatch error path.
- Container node shape (`type:`, `children:`, `properties:`).
- Leaf and data-shape node shape.
- Per-mode value selection at the page level.
- Validation pass details (deferred to the validator implementation in `internal/agent/validate.go`).

This stub contributes only the universal-container-fields section so that the `@studio-support/adapter-vocabulary-extension` feature has a single source of truth to point at when describing where these fields live.
