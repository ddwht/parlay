<!--
parlay-section: cross-cutting
parlay-feature: design-loop/vocabulary-validation
parlay-cross-cutting-id: vocabulary-adapter-schema-docs
-->

# Vocabulary Schema

File: the `vocabulary:` block lives **inline** at the top level of `<adapter>.adapter.yaml`. It is NOT a sibling `<adapter>.vocabulary.yaml` file, and the validator does NOT read any sibling file. See "Inline vs separate file" below for the rationale.

The vocabulary block is the closed declaration of what components, tokens, and layout containers the validator considers admissible. It is consumed by:

- The Design Loop pre-flight validator (`parlay validate-vocabulary @<feature>`) — checks the canonical layout against this vocabulary before any Figma round-trip.
- The Design Loop read-back classifier — runs the same checks against designer-authored novelties, in single-node mode.
- Future in-process Studio callers (e.g. Domain Model Editor pre-saves) that load the vocabulary block directly, in-process.

The validator's internal representation — the `Vocabulary`, `ComponentSpec`, `LayoutContainerSpec`, and `ParameterConstraint` record shapes — mirrors the field set documented here one-for-one. Any rename or extra field on either side is a wire-contract break.

## Structure

```yaml
vocabulary:
  components:
    - name: <string referenced from layout files — e.g., clarity.button>
      properties: [<flat list of admissible property names>]
      variants:
        <axis-name>: [<closed enum of admissible variant values>]

  spacing_tokens: [<flat list of admissible spacing token names>]

  color_tokens: [<flat list of admissible color token names>]

  layout_containers:
    - container_type: <string referenced from layout files — e.g., clarity.region>
      admissible_parameters: [<flat list of admissible parameter names>]
      parameter_constraints:
        <parameter-name>:
          type: <one of: enum | string | int | boolean>
          allowed_values: [<closed enum when type is enum>]
```

## Subfield: components

`components` is a list of `ComponentSpec` records. Each record declares one design-system component and the closed sets the validator pins.

| Field | Required | Description |
|---|---|---|
| `name` | Yes | The string layout files reference. Must match the layout node's `type:` value verbatim. |
| `properties` | No | Flat list of admissible property names. Property references not in this list trigger `rule: property-check`, `severity: error`. |
| `variants` | No | Per-axis enum of admissible variant values. References to unknown axes OR values outside the axis enum trigger `rule: variant-check`, `severity: error`. |

Layout nodes whose `type:` does not match any `components[].name` trigger `rule: type-check`, `severity: error`. Checks 1-3 (type, property, variant) only ever emit `error` — never `warning`.

Example:

```yaml
components:
  - name: clarity.button
    properties: [label, disabled, icon]
    variants:
      kind: [primary, secondary, tertiary]
```

## Subfield: spacing_tokens

`spacing_tokens` is a flat `[]string` of admissible spacing token names. Spacing/padding/gap values in layouts must resolve to one of these names. Raw literals (e.g. `16`, `1.5rem`) trigger `rule: spacing-token-check`, `severity: error`. Token names that resolve through an alias to a non-canonical token trigger the same rule with `severity: warning`.

Example:

```yaml
spacing_tokens: [spacing-xs, spacing-sm, spacing-md, spacing-lg, spacing-xl]
```

## Subfield: color_tokens

`color_tokens` is a flat `[]string` of admissible color token names. Color values in layouts must resolve to one of these names. Raw hex literals (e.g. `#3B82F6`) trigger `rule: color-token-check`, `severity: error`. Alias-to-non-canonical resolutions trigger the same rule with `severity: warning`.

Example:

```yaml
color_tokens:
  - color-status-info
  - color-status-danger
  - color-status-success
```

## Subfield: layout_containers

`layout_containers` is a list of `LayoutContainerSpec` records. Each record pins one layout-container shape — e.g., `clarity.region` — and the parameter set the validator admits.

| Field | Required | Description |
|---|---|---|
| `container_type` | Yes | The container component type — must also exist in `components[].name`. |
| `admissible_parameters` | Yes | Flat list of admissible parameter names. Names outside this list trigger `rule: layout-container-check`, `severity: error`. |
| `parameter_constraints` | No | Per-parameter `ParameterConstraint`. Values outside the constraint trigger the same rule with `severity: error`. |

A `ParameterConstraint` has:

| Field | Required | Description |
|---|---|---|
| `type` | Yes | One of `{enum, string, int, boolean}`. |
| `allowed_values` | When type is enum | Closed list of admissible values for this parameter. |

Example:

```yaml
layout_containers:
  - container_type: clarity.region
    admissible_parameters: [direction, gap, padding]
    parameter_constraints:
      direction:
        type: enum
        allowed_values: [horizontal, vertical]
```

## componentVocabulary resolution

A layout pins its vocabulary by setting `componentVocabulary: <name>@<version>` on the top-level layout node (e.g. `componentVocabulary: clarity@17`). The validator resolves this value through the registered adapter set: it iterates the registered adapters, reads each adapter's `componentVocabulary.name` field, and matches against the layout's value. The first match's `vocabulary:` block becomes the admissible set.

When the value cannot be resolved against any registered adapter, the validator emits `vocabulary-unknown-adapter` (see "Resolution failures" below) — the error message names both the referenced componentVocabulary value AND the registered-adapter list, so the operator can see which adapters were tried and pick the missing one to register.

## Resolution failures

The validator emits exactly two stable error codes for vocabulary resolution failures. These strings are part of the wire contract — the design-loop skill matches them textually:

| Code | Trigger | Message template |
|---|---|---|
| `vocabulary-missing-from-adapter` | The resolved adapter file parses cleanly but has no `vocabulary:` block. | `vocabulary-missing-from-adapter: adapter file <path> has no vocabulary: block` |
| `vocabulary-unknown-adapter` | The layout's `componentVocabulary:` value does not match any registered adapter's `componentVocabulary.name`. | `vocabulary-unknown-adapter: referenced componentVocabulary "<value>" does not resolve against any registered adapter (registered: <list>)` |

Exactly two codes exist. No third resolution-failure code may be introduced without a schema-doc change. The Go sentinels are `vocabulary.ErrVocabularyMissingFromAdapter` and `vocabulary.ErrVocabularyUnknownAdapter`; both are exported package-level variables so callers can match via `errors.Is`.

## Inline vs separate file

The `vocabulary:` block lives **inline** at the top level of `<adapter>.adapter.yaml`. Adapter authors do NOT place vocabulary content in a sibling `<adapter>.vocabulary.yaml` file — the validator does not read any sibling file. A unit test plants a sibling `<adapter>.vocabulary.yaml` file and asserts the loader ignores it.

Rejected alternative (archaeology): an earlier proposal split the vocabulary into a separate `<adapter>.vocabulary.yaml` to keep adapter files small. The split was rejected because:

1. It doubled the file-IO contract surface — every consumer would need to know to read both.
2. It created a synchronization concern (adapter version vs vocabulary version drift) that inline placement avoids.
3. The vocabulary is intrinsically tied to a specific adapter's componentVocabulary version — separating them suggests an independence that does not exist.

Inline is the only supported placement. New adapters MUST place the block at the top level of the adapter YAML.

## Backward compatibility

Adapters without a `vocabulary:` block cannot drive Design Loop vocabulary validation — `LoadFromAdapterFile` returns `ErrVocabularyMissingFromAdapter`. Such adapters remain valid for non-Design-Loop adapter uses: codegen against the adapter still works, the adapter still registers cleanly, and `parlay validate --type adapter` still passes.

Drift detection between the declared vocabulary and the real Figma source of truth is out of scope for this schema. Keeping the declared vocabulary in sync with Figma is the adapter author's responsibility. A future integration-test feature may add automated drift detection; the validator does not do any such comparison and never calls Figma MCP.

## Vocabulary block contents intentionally exclude resolved values

The `spacing_tokens` and `color_tokens` lists are flat names — they do NOT carry pixel values for spacing or hex values for colors. The validator checks set membership only. The resolved value (e.g. `spacing-md` -> `16px` in the light mode of a Tailwind adapter) lives with the design system, not in the validator's contract. Two adapters declaring the same vocabulary version MUST produce identical name sets; resolved values may differ.

## Determinism contract

Any AI or codegen pass that reads this schema produces the same `Vocabulary`, `ComponentSpec`, `LayoutContainerSpec`, and `ParameterConstraint` field sets — exactly the four subfields and the four record shapes documented above. The validator's internal types are the runtime mirror of this schema; any divergence between the schema doc and the implementation is a bug in whichever was edited last.
