<!--
parlay-section: cross-cutting
parlay-feature: studio-support/adapter-vocabulary-extension
parlay-cross-cutting-id: layout-schema-universal-container-fields
-->

# Layout Schema

File: `spec/intents/<feature>/<page>.layout.yaml` (and the in-buildfile layout regions emitted by `/parlay-build-feature`).

A **layout** describes the visual / structural composition of a page using components drawn from the active adapter's `componentVocabulary:` (see `adapter.schema.md` Section 8). Layouts pin themselves to a vocabulary version (e.g., `clarity@17`) so that a layout authored against one revision of a design system fails fast when the adapter advertises a different revision rather than silently rendering against an unintended vocabulary.

This file defines the **universal container fields** that every layout-author may use on every container node, regardless of which adapter is active. The full layout schema (node shape, child arrays, leaf-component properties, vocabulary pinning syntax) is owned by `@studio-support/page-layout-field` and will be added here as that feature lands. This stub establishes the universal-container-fields contract so the vocabulary-extension and layout-pipeline features can refer to a single source of truth for container chrome.

## Relationship to design-spec.schema.md

Layout and design-spec (`.parlay/build/<feature>/design-spec.yaml`) both describe a page's UI, with a deliberately disjoint scope: this file owns structural composition — which components exist, how they nest, `direction`/`gap`/`padding`/`alignment` — and design-spec owns non-layout enrichment on top of that structure — exact widget variant, state-specific visuals, and color/spacing/motion token values. Neither restates the other's fields. A layout node never carries a `variants:` map or a `motion:` token; a design-spec never declares `direction` or child ordering. See `design-spec.schema.md`'s "Relationship to layout.schema.md" section for the full statement and the migration note for design-specs authored before this split (their now-removed per-fragment `layout:` field predates this schema's node-tree ownership of structure).

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

<!--
parlay-section: cross-cutting
parlay-feature: studio-support/page-layout-field
parlay-cross-cutting-id: layout-schema-doc-tree-shape
-->

## Top-level structure

A layout block — whether embedded in a page artifact under the optional `## Layout` heading, or carried in a standalone `*.layout.yaml` file — declares three top-level keys at its root:

| Key | Value type | Required | Description |
|---|---|---|---|
| `componentVocabulary` | string | required | Name and version of the component vocabulary the layout targets (e.g., `clarity@17`). See [Vocabulary pinning](#vocabulary-pinning) below. |
| `schema_version` | integer | required | Layout schema version this block conforms to. Mismatched versions are rejected by the validator (see [Validation pass](#validation-pass)). |
| `nodes` | list of layout nodes | required | The recursive tree of layout nodes that compose the page. Container nodes carry `children:`; leaf and data-shape nodes do not. See [Container node shape](#container-node-shape) and [Leaf and data-shape nodes](#leaf-and-data-shape-nodes). |

The rule for embedding the same three keys inside a page Markdown body under the optional `## Layout` heading — including the heading-match semantics, the fenced YAML code-block convention, and the Fields-table row — is documented in `page.schema.md`. The layout schema owns the keys themselves; the page schema owns the embedding rule. This file does not restate the embedding rule inline.

**Versioning policy** (see `schema-versioning.schema.md` for the house rule): **regenerate, for now**. Layouts can be hand-edited — this is a real difference from buildfiles/testcases, which are purely tool-generated — but the feature is new enough (v1, freshly landed) that there's no installed base of hand-authored layouts a migrator chain would need to bridge. This is a deliberate, recorded exception to "hand-editable implies migrator chain": when this schema needs its first real v2 shape, that's the point to decide whether a migrator chain is worth building, weighed against how much hand-authored content exists by then to lose without one. Regenerating from Studio's layout pipeline (or hand-editing the small delta) is the interim policy — the same reasoning already applied to the `schemaVersion` → `schema_version` field rename in this revision, which was a hard cutover with no dual-key bridge for the same reason.

**Precedence when both forms exist for the same page.** A standalone `spec/intents/<feature>/<page>.layout.yaml` and a page manifest's embedded `## Layout` section can both exist for the same page name. When they do, the page manifest's embedded layout is authoritative — see `page.schema.md`'s "Precedence when a per-feature layout also exists" section for the full rule and rationale. This file does not restate it inline.

## Vocabulary pinning

The `componentVocabulary` key pins a layout to a specific vocabulary name and version using the syntax `<name>@<version>`:

```yaml
componentVocabulary: clarity@17
```

The name (`clarity`) identifies a component vocabulary; the version (`17`) identifies a specific revision of that vocabulary. At validation time the precheck cross-references this declaration against the active adapter's registered vocabulary version.

When a layout declares `clarity@17` but the active adapter is registered as `clarity@16` (or vice versa), validation fails with `vocabulary-version-mismatch`. The fix names **both** remediation paths:

- Re-register the adapter at the version the layout declares (e.g., upgrade or downgrade the adapter to `clarity@17`), or
- Change the layout's `componentVocabulary` declaration to match the registered adapter version (e.g., `clarity@16`).

Neither path is preferred over the other — the choice depends on whether the adapter or the layout is the source of truth for the intended vocabulary version in that project.

## Container node shape

A **container node** is a layout node whose `children:` list carries one or more nested layout nodes. Every container node declares the following universal node fields:

| Field | Value type | Required | Description |
|---|---|---|---|
| `id` | string | required | Stable identifier for the node. Preserved across read-edit-write round-trips per the round-trip-stability invariant — surviving nodes keep their ids; newly added nodes get fresh ids. |
| `type` | string | required | Vocabulary-qualified component type (e.g., `clarity.region`). Validated against the active adapter's `componentVocabulary` entry for the declared version (see [Vocabulary-binding rule](#vocabulary-binding-rule)). |
| `children` | list of layout nodes | optional | Recursive nesting to arbitrary depth. Each child is itself a layout node — container, leaf, or data-shape — and is validated by the same rules at every level. |

The four universal container fields (`direction`, `gap`, `padding`, `alignment`) documented in the [Universal container fields](#universal-container-fields) section above apply to every container node. They are not restated here — the table above is the single source of truth for their value types and rejection rules.

## Leaf and data-shape nodes

A **leaf node** is a layout node with no `children:` list (e.g., a button, a label, a single data-field display, a static icon). A **data-shape node** is a leaf node whose `type` is a vocabulary component that consumes a typed data binding (e.g., `clarity.datagrid`, `clarity.kpi-card`, `clarity.chart`). Both leaf and data-shape nodes carry the same universal node fields as container nodes — `id` (required) and `type` (required) — plus the universal container fields (`direction`, `gap`, `padding`, `alignment`) on whichever fields the component opts in to from the [Universal container fields](#universal-container-fields) set.

The distinction between a leaf node and a data-shape node is not encoded in the layout schema itself; it is a property of the vocabulary component named in `type`. The validator does not branch on "leaf vs. data-shape" — it walks every node uniformly and looks the type up in the adapter to determine what fields are valid.

Vocabulary-specific node fields (e.g., `headerLabel` on `clarity.region`, `density` on `clarity.datagrid`, `axisTitle` on `clarity.chart`) are validated per the [Vocabulary-binding rule](#vocabulary-binding-rule) below — not enumerated in this schema.

## Vocabulary-binding rule

Vocabulary-specific node fields are **not enumerated in this schema**. They are validated against the active `adapter`'s `componentVocabulary` entry for the node's declared `type`.

The flow:

1. The layout declares `componentVocabulary: <name>@<version>` at the root (see [Vocabulary pinning](#vocabulary-pinning)).
2. Each node declares `type: <vocabulary-qualified-name>` (e.g., `clarity.datagrid`).
3. At validation time, the validator looks up the `adapter` registered under the declared vocabulary name and version, finds the `componentVocabulary` entry for the declared `type`, and validates the node's vocabulary-specific fields against the property declarations on that entry.

Both `adapter` and `componentVocabulary` are named here by their exact string spelling so the rule is greppable from either side of the binding — a reader investigating either term will land on this section. Per-vocabulary variation (which components exist, which properties they declare, which property values are valid) is captured in the adapter, not in this schema.

This is why the layout schema owns only the universal fields and the top-level structure; the per-component shape is delegated to the adapter so that adding a new vocabulary component does not require editing this schema.

## Per-mode value selection

A layout MAY declare an optional `mode:` selector at the top level alongside `componentVocabulary`, `schema_version`, and `nodes`, and a node MAY declare per-mode field values via a `mode:` key on individual fields. This lets a single layout carry distinct values for different visual modes (e.g., dense vs. comfortable density, light vs. dark theme, compact vs. spacious spacing).

The `mode:` key is **optional**. A layout that does not declare it parses and validates exactly as before — there is no behavioral change for layouts that have no mode-aware fields, and the absence of `mode:` is bit-for-bit equivalent to today's layouts. Pages without per-mode fields stay simple.

When a node carries per-mode values for a field, the active `adapter`'s `componentVocabulary` entry for that `type` MUST declare the field as mode-aware. Otherwise validation fails with `missing-mode-emit-form` — the error code is owned by `@studio-support/adapter-vocabulary-extension` and surfaced through the same precheck verdict shape as the codes listed in [Validation pass](#validation-pass) below. The remediation is either to remove the per-mode declaration from the node or to update the adapter's `componentVocabulary` entry to mark the field as mode-aware.

## Validation pass

Validation of a layout block runs through two entry points:

- **The per-rule validator** — walks every node in the tree and applies the per-node checks (component-type membership, variant membership, property type, allowed-children, raw-value-vs-token, unknown-token, universal-field-redeclared, layout-schema-version-unsupported). Returns the full set of validation errors found.
- **The precheck contract** — the consumer-facing entry point. Calls the per-rule validator internally and aggregates the result into a closed-shape verdict that consumers (codegen, status, repair, sync) branch on uniformly.

The closed set of stable error codes the precheck registers for this feature:

- `malformed-layout-block` — block-level YAML parse failure.
- `missing-schema-version` — `schema_version` key absent.
- `vocabulary-version-mismatch` — page declares a vocabulary version that does not match the registered adapter version.
- `unknown-component-type` — node references a `type` outside the declared vocabulary.
- `raw-value-where-token-required` — a token-typed field (e.g., `gap`, `padding`) carries a raw value (e.g., `24px`, bare integer). **Same underlying violation** as the Design Loop vocabulary validator's `spacing-token-check` rule (`vocabulary.schema.md`) — see that schema's "Same violation, two reporting frameworks" note for why the two aren't literally one code today.
- `wiring-in-layout` — node carries a wiring field (`dataSource`, `binding`, expression-string fields) that does not belong in a layout block.

Adjacent error codes (`unknown-variant`, `unknown-token`, `universal-field-redeclared`, `missing-mode-emit-form`) are owned by sibling features and surface through the same precheck contract. The full verdict shape — `Code`, `File`, `NodePath`, `Found`, `Expected`, `Fix` — is owned by the precheck contract and is not restated here.
