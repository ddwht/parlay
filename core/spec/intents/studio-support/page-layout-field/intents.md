# Optional Layout Field on Page Artifacts

> Extend Core's existing page schema with an optional `layout:` YAML block that carries a typed tree of design-system components. Pages without `layout:` continue to behave exactly as today; pages with `layout:` become the canonical source of truth for what the page looks like, written by Studio's Design Loop and consumed by Core's codegen.

---

## Add Optional Layout Block to the Page Schema

**Goal**: Allow pages to carry a structured layout — a typed tree of design-system components — directly inside the existing `pages/<page-id>.md` artifact, without breaking projects whose pages do not have layouts yet.
**Persona**: UX Designer
**Priority**: P0
**Context**: Studio needs a place to write the canonical layout for a page after a Figma round-trip. The layout is embedded in the existing page artifact rather than introduced as a separate file, so designer-authored layout and developer-authored prose live in one artifact and parallel editing happens through different sections of the same Markdown file.
**Action**: Add a `## Layout` section to the page schema whose body is a YAML code block conforming to the layout schema. The block carries `componentVocabulary` (e.g., `clarity@17`), `schemaVersion`, and a recursive `nodes` tree; node properties are typed per component vocabulary. Pages without a `## Layout` section parse exactly as before.
**Objects**: page, layout, component-vocabulary, layout-node, schema-version

**Constraints**:
- The layout block is optional — pages without it must continue to load, validate, and codegen exactly as they do today
- The block is embedded in the existing `pages/<page-id>.md` file, not a separate `*.layout.yaml` — Studio extends the existing page artifact rather than introducing a parallel file, so designer and developer edits stay co-located
- `componentVocabulary` is declared explicitly at the top of the layout (e.g., `clarity@17`); a Studio binary configured for a different vocabulary fails fast on read rather than silently mis-rendering
- `schemaVersion` is present from day one
- Layout nodes carry only structural and presentation data — `id`, `type`, layout parameters (`direction`, `gap`, `padding`, `alignment`), variants, text, children. Wiring information (data sources, operation references, expressions) is **explicitly forbidden** in the layout block; wiring lives in the layout-aware codegen pass
- Spacing values are token names (e.g., `spacing-lg`), not raw values, so the same layout codegens correctly across adapter pixel scales
- Layout-node `id` is stable across round-trips — Studio uses it to match canonical nodes to Figma nodes during sync
- The `## Layout` section is a top-level body section in the page Markdown — sibling to other top-level sections, never nested under one. Order relative to other top-level sections is not significant for parsing; Studio-emitted output places it immediately after the frontmatter for readability, but a hand-edited page that orders it elsewhere parses identically

**Verify**:
- A page with `---` frontmatter, body prose, and a `## Layout` YAML block parses cleanly; `parlay status` reports the page as valid
- A page without a `## Layout` section parses cleanly and is treated identically to today's pages
- A layout block declaring a `componentVocabulary` not registered against any installed adapter fails parse with a clear error naming the vocabulary
- A layout block missing `schemaVersion` fails parse with an actionable error
- A layout block containing wiring fields (e.g., `dataSource:`, `binding:`, expression strings) fails parse with an error explaining wiring lives in codegen
- Round-tripping a page through Studio's design loop preserves all node `id`s
- A page that places `## Layout` immediately after the frontmatter and a page that places it after other top-level sections produce identical parse trees and identical codegen output

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

## Surface Layout Validation as a Precheck Contract for Codegen

**Goal**: Provide a single, callable validation entry point that runs every layout check (well-formedness, schema compliance, vocabulary membership, token correctness, no-wiring-in-layout) and returns a structured verdict that callers — codegen, `parlay status`, `parlay repair`, and Studio's sync flow — can consume uniformly. Codegen never re-implements validation logic; it asks the precheck and surfaces the answer.
**Persona**: UX Designer
**Priority**: P0
**Context**: The first two intents specify *what* makes a layout valid (parse-time schema checks; vocabulary and token cross-checks against the active adapter). They do not specify *who* runs those checks, *when*, or *what the verdict looks like* to a caller. Without that contract, every consumer (codegen, status, sync) implements its own version of validation and the rules drift. *Layout-Aware Code Generation* already references this precheck as the source of its precheck refusals; this intent makes that reference real.
**Action**: Define a `layout-precheck` function with a stable signature: input is a parsed page artifact plus the active adapter; output is a verdict — either `ok` or a structured failure record carrying an error code, the file path, the layout-node path inside the file, the offending value, the expected shape, and a "to fix" suggestion. The function aggregates every check defined in this feature and in `adapter-vocabulary-extension`. Callers receive the verdict and decide policy (codegen refuses to run; status colors the page red; sync rejects the round-trip).
**Objects**: layout-precheck, verdict, error-code, page-artifact, adapter, caller-policy

**Constraints**:
- The verdict is a closed shape across all error types — every failure carries exactly the same fields (code, file, node-path, found, expected, fix-hint) and no others. There is no `severity` field; every failure is an error. If a future requirement surfaces a soft case, it is added as a new closed-shape variant or a separate verdict kind, not by widening this one
- Error codes are stable identifiers and exhaustive: `malformed-layout-block`, `missing-schema-version`, `vocabulary-version-mismatch`, `unknown-component-type`, `unknown-variant`, `raw-value-where-token-required`, `unknown-token`, `wiring-in-layout`, `universal-field-redeclared`, `missing-mode-emit-form`. New codes are added in lockstep with new validation rules in this feature or in `adapter-vocabulary-extension`
- The precheck is invoked at page-load time, before any consumer attempts to use the layout — codegen, status, repair, and sync all hit this single entry point
- The precheck never auto-fixes — it returns a verdict; the human (or Studio) makes corrections. Callers may surface the verdict, refuse to proceed, or annotate state, but they never silently mutate the layout
- The precheck is deterministic: the same `(page-artifact, adapter)` input always produces the same verdict (same code, same wording, same hint). This is testable and CI-stable
- Aggregating verdicts across many pages is a list of per-page records, not a single global verdict — callers decide whether to fail-fast on the first failure or collect all failures
- The precheck produces no AI calls and uses no external state — it is a pure function over its inputs and runs in sub-millisecond time on common-case pages
- A passing verdict carries `code: ok` and no other fields — callers branch on `code == ok` versus everything else, so the failure shape never sneaks into success paths

**Verify**:
- A page with a malformed `## Layout` YAML block returns a verdict `{code: malformed-layout-block, file: ..., found: <parser error verbatim>, fix: <parser-emitted next step>}`
- A page declaring `clarity@17` evaluated against an adapter loaded as `clarity@16` returns `{code: vocabulary-version-mismatch, file: ..., found: clarity@17, expected: clarity@16, fix: re-register the clarity adapter at version 17, or change the page declaration to clarity@16}`
- A page using `gap: 24px` returns `{code: raw-value-where-token-required, file: ..., node-path: ..., found: 24px, expected: <list of valid spacing tokens>, fix: replace 24px with one of [...]}`
- A page with `type: clarity.kanban` against `clarity@17` returns `{code: unknown-component-type, file: ..., node-path: ..., found: clarity.kanban, expected: <vocabulary list>, fix: pick a known type from clarity@17 or upgrade the adapter}`
- A page that passes every check returns a verdict whose only field is `{code: ok}`
- The same page validated twice in a row returns identical verdicts byte-for-byte (the verdict struct is the deterministic part of the system, even though codegen's emitted code text downstream is not)
- Calling `parlay status` on a project with a mix of valid and invalid layout-bearing pages returns one verdict per page; the per-page results are the union of every check, not just the first failure

---
