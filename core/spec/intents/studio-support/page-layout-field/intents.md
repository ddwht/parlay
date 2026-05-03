# Optional Layout Field on Page Artifacts

> Extend Core's existing page schema with an optional `layout:` YAML block that carries a typed tree of design-system components. Pages without `layout:` continue to behave exactly as today; pages with `layout:` become the canonical source of truth for what the page looks like, written by Studio's Design Loop and consumed by Core's codegen.

---

## Add Optional Layout Block to the Page Schema

**Goal**: Allow pages to carry a structured layout — a typed tree of design-system components — directly inside the existing `pages/<page-id>.md` artifact, without breaking projects whose pages do not have layouts yet.
**Persona**: UX Designer
**Priority**: P0
**Context**: Studio needs a place to write the canonical layout for a page after a Figma round-trip. The architecture (§4.3) commits to embedding layout in the existing page artifact rather than introducing a separate file, so designer/developer parallel editing happens through different sections of the same Markdown file.
**Action**: Add a `## Layout` section to the page schema whose body is a YAML code block conforming to the layout schema. The block carries `componentVocabulary` (e.g., `clarity@17`), `schemaVersion`, and a recursive `nodes` tree; node properties are typed per component vocabulary. Pages without a `## Layout` section parse exactly as before.
**Objects**: page, layout, component-vocabulary, layout-node, schema-version

**Constraints**:
- The layout block is optional — pages without it must continue to load, validate, and codegen exactly as they do today
- The block is embedded in the existing `pages/<page-id>.md` file, not a separate `*.layout.yaml` — this is a deliberate design choice (P5: Studio extends Core's existing artifacts, doesn't introduce new ones)
- `componentVocabulary` is declared explicitly at the top of the layout (e.g., `clarity@17`); a Studio binary configured for a different vocabulary fails fast on read rather than silently mis-rendering
- `schemaVersion` is present from day one
- Layout nodes carry only structural and presentation data — `id`, `type`, layout parameters (`direction`, `gap`, `padding`, `alignment`), variants, text, children. Wiring information (data sources, operation references, expressions) is **explicitly forbidden** in the layout block; wiring lives in the layout-aware codegen pass
- Spacing values are token names (e.g., `spacing-lg`), not raw values, so the same layout codegens correctly across adapter pixel scales
- Layout-node `id` is stable across round-trips — Studio uses it to match canonical nodes to Figma nodes during sync

**Verify**:
- A page with `---` frontmatter, body prose, and a `## Layout` YAML block parses cleanly; `parlay status` reports the page as valid
- A page without a `## Layout` section parses cleanly and is treated identically to today's pages
- A layout block declaring a `componentVocabulary` not registered against any installed adapter fails parse with a clear error naming the vocabulary
- A layout block missing `schemaVersion` fails parse with an actionable error
- A layout block containing wiring fields (e.g., `dataSource:`, `binding:`, expression strings) fails parse with an error explaining wiring lives in codegen
- Round-tripping a page through Studio's design loop preserves all node `id`s

**Questions**:
- Where exactly in the page Markdown does `## Layout` live — top-level under the frontmatter, nested under another section, or order-agnostic? §4.3's example puts it at top level. Confirm during dialog authoring.

---

## Define the Layout Tree Schema

**Goal**: Pin the typed-tree shape so Studio (writer) and Core's codegen (reader) agree on what valid layout structures look like.
**Persona**: UX Designer
**Priority**: P0
**Context**: A typed tree only works if the type rules are explicit. The vocabulary itself comes from adapters (see `adapter-vocabulary-extension`); the tree shape — how vocabulary nodes nest and what shared layout properties are allowed — is defined here.
**Action**: Author a layout schema (in the project's schemas directory) that defines: the recursive `nodes` array; the universal node fields (`id`, `type`, `children`); the universal layout container fields (`direction`, `gap`, `padding`, `alignment`); the rule that vocabulary-specific fields (e.g., `headerLabel`, `density`) are validated against the adapter's component definition for that `type`.
**Objects**: layout-schema, layout-node, container, vocabulary-binding

**Constraints**:
- Universal fields apply to all nodes regardless of vocabulary; vocabulary-specific fields are validated against the adapter
- The schema declares which fields are required vs optional, and which expect token references vs raw values
- The schema is versioned alongside the page schema (same `schemaVersion`)
- Validation errors name the offending node `id` and the offending field, so a designer fixing a Figma sync rejection knows exactly where to look

**Verify**:
- A layout tree with universal fields only (no vocabulary-specific properties) validates against the schema regardless of `componentVocabulary`
- A layout tree referencing a `type` not in the declared `componentVocabulary` fails validation with the offending `type`
- A layout tree using a raw pixel value where a token is expected fails validation
- A nested layout tree (containers within containers) validates correctly to arbitrary depth

---
