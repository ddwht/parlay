<!--
parlay-section: cross-cutting
parlay-feature: studio-support/page-layout-field
-->

# Page Schema

File: `spec/pages/<page-name>.page.md`
Optional. Locks the layout of a page when the derived view from feature surfaces isn't enough. Created via `/parlay lock-page ==page-name==`.

By default, pages are derived views assembled on demand from feature surfaces. A manifest is only needed when cross-feature layout needs an explicit owner.

## Template

```
# <Page Name>

> <One-line description>

**Owner**: <Team or person responsible>
**Status**: <draft | reviewed | locked>

## <Region Name>

1. @feature-a/fragment-name
2. @feature-b/fragment-name

## <Region Name>

1. @feature-c/fragment-name
```

## Fields

| Field | Required | Parse rule |
|---|---|---|
| Page Name | Yes | `# ` heading. Must match Page values in feature surfaces. |
| Description | No | `> ` line after heading |
| Owner | No | `**Owner**:` line content |
| Status | No | `**Status**:` value — `draft`, `reviewed`, or `locked` |
| Region | Yes (at least one) | `## ` heading. Must match Region values in feature surfaces. |
| Fragment list | Yes | Numbered list (`1.`, `2.`, ...) of `@feature/fragment-name` references. Order overrides feature surface Order values. |
| Layout | No | `## Layout` heading with a fenced YAML block conforming to `layout.schema.md`. See the [Layout](#layout) section below. |

## Layout

The `## Layout` section is **optional**. A page artifact that omits it parses and behaves exactly as today — pages without a layout block are valid and produce a parser-visible result bit-for-bit equivalent to a page authored before this section existed. The Fields table row for Layout reads `No` in the Required column; nothing in this document marks the layout block as required at the page level.

When present, the body of the `## Layout` section is a fenced YAML code block whose top-level keys are:

- `componentVocabulary` (string, required) — e.g., `"clarity@17"`. Names the component vocabulary the layout tree is authored against.
- `schema_version` (integer, required) — the layout-tree schema version the block conforms to.
- `nodes` (list, required) — a recursive tree of layout nodes.

All three keys MUST be present when a `## Layout` section is included; the parser rejects layout blocks that omit any of them, naming the missing field.

### Position among top-level sections

The `## Layout` section MAY appear anywhere among the page's top-level body sections — order relative to other top-level sections is not significant. The parser matches by heading text, not by position. A page that places `## Layout` immediately after the frontmatter and a page that places it after other top-level sections produce identical parse trees.

### Node-tree shape

The shape of the `nodes` tree (universal container fields, recursive children, vocabulary-specific component types, token-vs-raw-value rules, no-wiring-in-layout invariant, and the full validation contract) is defined in the sibling [`layout.schema.md`](./layout.schema.md) document. This page-schema document names only the three top-level YAML keys above and forwards every further detail of the node tree to `layout.schema.md` — it deliberately does not restate the node-tree shape inline.

### Precedence when a per-feature layout also exists

A page name can be the target of fragments from multiple features, and any of those features may separately author its own `spec/intents/<feature>/<page>.layout.yaml` (see `layout.schema.md`) for the same page name. When a page manifest's `## Layout` section is ALSO present for that page, the manifest's embedded layout is authoritative — it is the cross-feature structural layout, the same role the manifest already plays for fragment ordering ("Manifest order overrides feature surface Order values" below). A per-feature standalone `<page>.layout.yaml` is the page's structural layout only in the absence of a manifest-level `## Layout` override; it's the derived/default the way surface `Order` values are the fragment-ordering default.

This mirrors the existing fragment-ordering precedence exactly: manifest-level structure (when present) wins over feature-level structure, for the same reason a manifest exists at all — cross-feature layout needs one explicit owner, and that owner is the manifest once one is locked. A project that never locks a page manifest, or locks one without a `## Layout` section, is unaffected — the per-feature layout (if any) governs, or the adapter's generic Show/Action-derived placement applies if there's no layout at all.

### Example

```
## Layout

​```yaml
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: root
    type: clarity.stack
    direction: vertical
    gap: spacing-md
    children:
      - id: header
        type: clarity.header
      - id: main-table
        type: clarity.datagrid
​```
```

(The leading zero-width characters in the inner fence above are decorative for the surrounding code block in this schema doc; an authored page omits them.)

## Versioning

The page manifest's own shape (the `# Page Name` / `**Owner**:` / `**Status**:` / region-and-fragment-list structure) has no `schema_version:` field (see `schema-versioning.schema.md` for the house rule) — don't confuse this with the embedded `## Layout` block's `schema_version`, which is a completely separate field on a completely separate structure (see `layout.schema.md`). The manifest format is simple, hand-reviewed, and hasn't changed shape since it was introduced; deferred for the same reason as the adapter and blueprint files — add it when the format's first breaking change actually happens.

## Behavior

- Manifest order overrides feature surface Order values
- Unlisted fragments targeting this page appear after manifest-ordered ones in `/parlay view-page`
- Tool warns on drift (new/removed fragments) but never auto-updates a locked manifest
- Does not define layout dimensions or styling — that's the prototype framework's job

## Error codes

`parlay validate --type page` resolves every reference in the manifest against
the surfaces that would produce it. Before this it resolved none of them: a
manifest naming a page no feature targets, a region nothing declares, and a
fragment no surface produces validated `OK`. A manifest *is* a set of
references, so a validator that never resolves them was only re-checking that
the parser had parsed.

| Code | When it fires |
|---|---|
| `page-fragment-unresolved` (warning) | A listed `@feature/fragment` reference matches no fragment any surface produces |
| `page-has-no-fragments` (warning) | No surface fragment carries `page: <this page>`, so the manifest orders nothing |

Both are warnings on purpose. A manifest listing a fragment its feature has not
written yet is the normal state of a page designed ahead of its features, and
blocking it would make the manifest unusable for the thing it exists to do.
`view-page` reports the same drift at assembly time; what was missing was any
report at all.

**Page identity is the filename stem** — `spec/pages/<page>.page.md` — not the
`# ` heading. That is what `view-page` looks up and what a surface fragment's
`page:` names; the heading is a display title and is usually capitalised.

## Parsing

- Page identity: `# ` heading
- Metadata: `**Field**:` pattern
- Regions: `## ` headings
- Fragment ordering: numbered list items
- Fragment references: `@feature/fragment-name` pattern
