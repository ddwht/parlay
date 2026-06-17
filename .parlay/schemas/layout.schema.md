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

<!--
parlay-feature: design-loop/design-loop
parlay-component: cross-cutting/on-disk-artifact-contract
-->

## Optional `figma:` block (Design Loop)

A layout MAY declare an optional top-level `figma:` block that records the Figma file the Parlay Studio Design Loop targets for this page. The block is **optional** — existing layout YAMLs without it continue to validate clean, so read-only Domain Model Editor use of layouts (which never invokes the Design Loop) stays unaffected.

```yaml
figma:
  file_url: <URL string>
```

| Field | Value type | Required | Description |
|---|---|---|---|
| `file_url` | URL string | required when `figma:` is present | The Figma file the Design Loop reads and writes for this page. Consumed by the `parlay-design-loop` skill (see `.claude/skills/parlay-design-loop/SKILL.md`) — its step 3 and step 6 `get_metadata` calls target this URL, and step 5's write tools (`use_figma`, `add_code_connect_map`, `send_code_connect_mappings`) push edits into the same file. |

The `figma:` block is the **per-feature** location for the Figma file URL. The URL is NOT stored in `studio-config.yaml`, NOT in any environment variable, and NOT at the page schema's root — different features routinely operate on different Figma files, so the URL is a per-feature concern, not a global one.

When `figma:` is absent (or omitted entirely) the layout is still a valid layout file — it just cannot be the target of a design-loop run, since the loop has no Figma file URL to call `get_metadata` against. Validators MUST accept layouts without the block; the block may be omitted whenever the page is read-only or has no design-loop integration.

### v1 contents

In v1 the `figma:` block declares only `file_url:`. A `team_url:` field and a per-node Figma node ID map were considered and **deferred** — pinning speculative fields before the first round-trip produces real data would force premature schema revision. Once `design-loop-result.yaml` actually carries node IDs from a real round-trip, a follow-up feature can extend the `figma:` block to persist them.

## Out of scope (for this stub)

The following layout-schema concerns are owned by `@studio-support/page-layout-field` and will be documented in subsequent revisions of this file:

- Top-level layout file structure (`layout:`, `vocabulary:`, `mode:`).
- Vocabulary pinning syntax (e.g., `vocabulary: clarity@17`) and the version-mismatch error path.
- Container node shape (`type:`, `children:`, `properties:`).
- Leaf and data-shape node shape.
- Per-mode value selection at the page level.
- Validation pass details (deferred to the validator implementation in `internal/agent/validate.go`).

This stub contributes only the universal-container-fields section so that the `@studio-support/adapter-vocabulary-extension` feature has a single source of truth to point at when describing where these fields live.
