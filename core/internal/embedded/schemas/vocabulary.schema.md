<!--
parlay-section: cross-cutting
parlay-feature: design-loop/vocabulary-validation
parlay-cross-cutting-id: vocabulary-adapter-schema-docs
-->

# Vocabulary Schema

File: the `vocabulary:` block lives **inline** at the top level of `<adapter>.adapter.yaml`. It is NOT a sibling `<adapter>.vocabulary.yaml` file, and the validator does NOT read any sibling file. See "Inline vs separate file" below for the rationale.

The vocabulary block is the closed declaration of what components, tokens, and layout containers the validator considers admissible. It is consumed by:

- The Design Loop pre-flight validator (`parlay internal validate-vocabulary @<feature>`) — checks the canonical layout against this vocabulary before any Figma round-trip.
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

**Same violation, two reporting frameworks.** `spacing-token-check` firing on a raw `gap`/`padding` value and `layout.schema.md`'s precheck code `raw-value-where-token-required` describe the identical violation — a token-typed field carrying a raw value — caught by two different validators that serve two different consumers: the precheck contract (codegen, status, repair, sync) reports a flat `Code` string; the Design Loop vocabulary validator (`parlay internal validate-vocabulary`, the read-back classifier) reports a `Rule` enum value alongside `Severity`. Collapsing these into one literal string was considered for this consolidation and **not done**: `Rule` is a closed, six-value Go enum (`type Rule string` in `studio/pkg/vocabulary/report.go`) that this schema's own "determinism contract" pins as a wire contract — renaming `spacing-token-check` to `raw-value-where-token-required` would be a breaking change to that enum, made in a different module (`studio`) than this consolidation's territory (`core/internal/agent`). Documenting the equivalence here, in both schemas, is the safe version of "collapse to one code" available without crossing that boundary; an actual rename is a `studio`-side decision, flagged rather than made here.

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

## Relationship to `componentVocabulary:` and `tokens:` — authoritative source, and this block's redefined role

`adapter.schema.md`'s `componentVocabulary:` (Section 8) and `tokens:` (Section 9) are **the single hand-authored declaration** of what components, properties, variants, and token names an adapter admits. This schema's `vocabulary:` block is **redefined as a derivation target**, not an independent hand-authored artifact: an adapter author's source of truth is `componentVocabulary:`/`tokens:`; `vocabulary:` exists to serve the Design Loop's validator, which needs its own flatter shape, and it should be *kept consistent with* the authoritative blocks rather than edited as a second, parallel decision.

**This redefinition is a documentation-level change only** — it does not (yet) change what any code does. `vocabulary.LoadFromAdapterFile` still reads the `vocabulary:` block directly, exactly as it always has, and adapter authors still hand-edit it directly, exactly as they always have. What changes is the normative story: from here forward, `vocabulary:` drifting out of sync with `componentVocabulary:`/`tokens:` is a **bug** (the derivation target has drifted from its source), not two independent authored documents that happen to disagree. That framing is what justifies the parity check below — a consistency test only makes sense once one side is declared authoritative.

**Field-name equivalence.** The two blocks name the same underlying facts differently:

| `vocabulary:` (this schema) | `componentVocabulary:`/`tokens:` (`adapter.schema.md`) | Same fact? |
|---|---|---|
| `components[].name` | `componentVocabulary.components[].type` | Yes — both are the vocabulary-qualified component identifier (e.g. `clarity.button`). |
| `components[].properties` (flat `[]string`) | `componentVocabulary.components[].properties[].name` (typed records) | Partially — the flat list is a strict subset of information; typed metadata (`type`, `enum-values`, `required`) has no equivalent here. |
| `components[].variants` (per-axis enum) | `componentVocabulary.components[].variants` (closed enum) | Yes — same shape, same purpose. |
| `spacing_tokens` (flat `[]string`) | `tokens.spacing[].name` | Yes — names match; `tokens.spacing` additionally carries `order` and `emit-form`, absent here. |
| `color_tokens` (flat `[]string`) | `tokens.color[].name` | Yes — names match; `tokens.color` additionally carries `tone` and per-mode `emit-forms`, absent here. |
| `layout_containers[].container_type` | *(no equivalent)* | No — see below. |
| `layout_containers[].admissible_parameters` / `parameter_constraints` | *(no equivalent — closest is the universal container fields, owned by `layout.schema.md`, not `componentVocabulary`)* | No — this is the real mismatch, not just a naming difference. |

**Why full mechanical derivation isn't implemented yet.** The `layout_containers` row above is the blocker: `LayoutContainerSpec`'s `admissible_parameters`/`parameter_constraints` shape predates the universal container fields (`direction`, `gap`, `padding`, `alignment`) that `layout.schema.md` now owns exclusively — `componentVocabulary` entries are explicitly forbidden from re-declaring those fields (`universal-field-redeclared`; see "Universal-fields rule extends to the derivation path" below). There is no `componentVocabulary` shape today that cleanly maps to `admissible_parameters`/`parameter_constraints`. A mechanical derivation would either drop that subfield silently or require redesigning `LayoutContainerSpec` first. Separately, the derivation's natural home would read an adapter file (`core`-side concept) and populate a `studio`-package type (`vocabulary.Vocabulary`) — real code living on either side of the `core`↔`studio` module boundary is a bigger commitment than this documentation-consolidation pass should take on. Both reasons point the same direction: declare the authoritative side now (this section), implement the parity check as a cheap consistency net (below), and leave the full derivation for a follow-up that first resolves the `layout_containers` mismatch.

## Universal-fields rule extends to the derivation path

`layout.schema.md`'s `universal-field-redeclared` check (adapter parse time: a `componentVocabulary` entry may not re-declare `direction`/`gap`/`padding`/`alignment`) extends in spirit to this schema's `layout_containers[].admissible_parameters`: an `admissible_parameters` list that names one of the four universal fields is declaring, in the `vocabulary:` block, exactly the redeclaration `componentVocabulary` is forbidden from making in its own block. This isn't enforced by a Go check today (see the parity-check note below for what IS enforced), but it's the same rule wearing the other schema's clothes — an adapter author hand-editing `layout_containers` should apply it by hand until the parity check (or a future full derivation) catches it mechanically.

## Cross-block parity check

A new consistency test — `TestVocabularyBlockConsistentWithComponentVocabulary` in `internal/agent` — reads both blocks from the same adapter file and checks the parts of the equivalence table marked "Yes": every `componentVocabulary.components[].type` has a same-named entry in `vocabulary.components[].name` (and vice versa), and every `tokens.spacing[].name`/`tokens.color[].name` appears in `spacing_tokens`/`color_tokens` (and vice versa). This is NOT the full derivation — it's a drift detector, cheap enough to add now because it only reads data already parsed on the `core` side (the adapter YAML) plus the `vocabulary:` block's own flat lists, with no `studio` import required. It fails loudly the moment an adapter author edits one side and forgets the other, which is exactly the risk this whole section exists to name.

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
