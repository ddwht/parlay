# Page-layout-field — Dialogs

---

### Add Optional Layout Block to the Page Schema

**Trigger**: A page artifact at `pages/<page-id>.md` is loaded by Core's page loader — invoked by `parlay status`, codegen, Studio's design-loop writer, or any tool that reads pages.

User: Loads a page artifact whose body contains a `## Layout` section with a YAML code block.
System (background): Parses frontmatter, walks top-level body sections, recognizes `## Layout` as the layout-bearing section, and decodes its YAML body against the layout schema.
System: Returns a parsed page that carries a `layout` field alongside the existing page fields.

User: Loads a page artifact with no `## Layout` section.
System (background): Parses the page exactly as today — no layout-bearing branch is taken.
System: Returns a parsed page whose `layout` field is absent. Downstream tools treat the page identically to today's pages.

#### Branch: Layout block declares a vocabulary the active adapter does not recognize

User: Loads a page whose layout declares `componentVocabulary: clarity@17`, but no installed adapter registers `clarity@17`.
System: Refuses the parse. Emits an error naming the vocabulary string and the page path: `unknown component vocabulary 'clarity@17' in pages/dashboard.md — register an adapter for this vocabulary or change the page declaration`.

#### Branch: Layout block omits schemaVersion

User: Loads a page whose layout YAML has `componentVocabulary` and `nodes` but no `schemaVersion` key.
System: Refuses the parse. Emits an actionable error: `pages/dashboard.md: layout block missing required field 'schemaVersion' — add 'schemaVersion: <n>' at the top of the ## Layout block`.

#### Branch: Layout block contains forbidden wiring fields

User: Loads a page whose layout includes a node with `dataSource:`, `binding:`, or an expression-string field.
System: Refuses the parse. Emits an error explaining the separation: `pages/dashboard.md: layout block contains wiring field 'dataSource' on node 'main-table' — wiring lives in the layout-aware codegen pass, not in the layout block`.

#### Branch: Spacing or layout values use raw units instead of tokens

User: Loads a page whose layout sets `gap: 24px` or `padding: 16` on a container node.
System: Refuses the parse. Emits an error naming the offending node and the expected token namespace: `pages/dashboard.md, node 'header-row': field 'gap' must be a spacing token (e.g., 'spacing-lg'), got raw value '24px'`.

#### Branch: Page places `## Layout` after other top-level sections

User: Loads a hand-edited page whose `## Layout` section appears after `## Notes` and `## Behavior` rather than immediately after the frontmatter.
System (background): Parses top-level sections by heading match, not position; recognizes `## Layout` regardless of where it appears among siblings.
System: Returns a parsed page byte-identical to one whose `## Layout` was placed first. Codegen output is identical between the two orderings.

#### Branch: Round-tripping a page through Studio's design loop

User: Studio reads a page, edits the layout in the canvas, and writes the page back.
System (background): On read, captures every layout-node `id`. On write, preserves those `id`s for nodes that survived the edit and only mints new `id`s for nodes the designer added.
System: Returns a written page whose surviving nodes carry the same `id`s they had on read. Subsequent Figma sync matches canonical nodes to Figma nodes by these stable `id`s.

---

### Define the Layout Tree Schema

**Trigger**: A layout block is being validated — either at page-load time, at Studio sync time, or when a developer runs `parlay validate` on a page artifact.

User: Submits a layout tree for validation.
System (background): Walks the tree depth-first; for each node, checks universal fields against the universal schema and vocabulary-specific fields against the adapter's component definition for that node's `type`.
System: Returns a validation result — `ok` if every node passes, otherwise a list of structured errors each naming an offending node `id` and field.

#### Branch: Tree uses universal fields only

User: Submits a layout tree whose nodes use only universal fields (`id`, `type`, `children`, `direction`, `gap`, `padding`, `alignment`) — no vocabulary-specific properties.
System: Validates the tree as `ok` regardless of which `componentVocabulary` is declared, because no vocabulary-specific validation is exercised.

#### Branch: Tree references a `type` outside the declared vocabulary

User: Submits a layout tree declaring `componentVocabulary: clarity@17` and containing a node `type: clarity.kanban`, but `clarity@17` does not define `clarity.kanban`.
System: Rejects the tree with an error naming the offending node `id` and the offending `type`: `node 'board-1': type 'clarity.kanban' is not defined in clarity@17 — pick a known type or upgrade the vocabulary`.

#### Branch: Tree uses a raw value where a token is required

User: Submits a layout tree where a node sets `padding: 16` (raw integer) on a field that expects a spacing-token reference.
System: Rejects the tree with an error naming the node `id` and the field: `node 'main-card': field 'padding' expects a spacing-token reference, got raw value '16'`.

#### Branch: Deeply nested tree

User: Submits a layout tree with containers nested several levels deep — a vertical stack containing horizontal rows containing card grids.
System: Validates each level recursively against the same rules; depth has no special cap. Returns `ok` if every node at every depth passes.

#### Branch: Schema-version mismatch between page and layout schema

User: Submits a layout block whose `schemaVersion` does not match the version this Core build targets.
System: Rejects the tree with an error naming the version found and the version expected: `layout schemaVersion '2' is not supported by this build (expects '1') — upgrade Core or change the page's schemaVersion`.

---

### Surface Layout Validation as a Precheck Contract for Codegen

**Trigger**: A consumer (codegen, `parlay status`, `parlay repair`, Studio sync) needs to know whether a page's layout is valid before acting on it. Each consumer calls `layout-precheck(page-artifact, adapter)` and branches on the verdict.

User: Calls `layout-precheck` with a parsed page artifact and the active adapter.
System (background): Aggregates every check — well-formedness, schema compliance, vocabulary membership, token correctness, no-wiring-in-layout — runs them in deterministic order, and produces a single closed-shape verdict. No AI calls; no external state; pure function over its inputs.
System: Returns a verdict. If everything passes, the verdict carries `{code: ok}` and no other fields. Otherwise the verdict carries `{code, file, node-path, found, expected, fix}` — every failure shape carries exactly these fields, no others.

#### Branch: Page with a malformed `## Layout` YAML block

User: Calls precheck on a page whose YAML in the `## Layout` block fails to parse (bad indent, unterminated string, etc.).
System: Returns `{code: malformed-layout-block, file: pages/dashboard.md, node-path: <empty — block-level failure>, found: <verbatim parser error message>, expected: well-formed YAML conforming to the layout schema, fix: <next step from the parser, e.g. "fix indent at line 14">}`.

#### Branch: Vocabulary version mismatch

User: Calls precheck on a page declaring `clarity@17` against an adapter loaded as `clarity@16`.
System: Returns `{code: vocabulary-version-mismatch, file: pages/dashboard.md, node-path: <empty — block-level>, found: clarity@17, expected: clarity@16, fix: re-register the clarity adapter at version 17, or change the page declaration to clarity@16}`.

#### Branch: Raw value where a token is required

User: Calls precheck on a page whose layout uses `gap: 24px` on a container.
System: Returns `{code: raw-value-where-token-required, file: pages/dashboard.md, node-path: nodes[0].children[2], found: 24px, expected: <list of valid spacing tokens registered by the adapter>, fix: replace 24px with one of [spacing-sm, spacing-md, spacing-lg, ...]}`.

#### Branch: Unknown component type

User: Calls precheck on a page with `type: clarity.kanban` against `clarity@17` which does not define that type.
System: Returns `{code: unknown-component-type, file: pages/dashboard.md, node-path: nodes[0].children[0], found: clarity.kanban, expected: <list of types in clarity@17>, fix: pick a known type from clarity@17 or upgrade the adapter}`.

#### Branch: Page passes every check

User: Calls precheck on a page whose layout is well-formed, schema-compliant, vocabulary-correct, token-correct, and wiring-free.
System: Returns `{code: ok}` and no other fields. Callers branch on `code == ok` versus everything else.

#### Branch: Determinism check — same input twice

User: Calls precheck twice on the same `(page-artifact, adapter)` pair.
System: Returns identical verdicts byte-for-byte across both calls — same code, same wording, same hint, same node-path. The verdict is the deterministic part of the system even though codegen output downstream is not.

#### Branch: `parlay status` walks a project with a mix of valid and invalid pages

User: Runs `parlay status` against a project containing several layout-bearing pages, some valid, some not.
System (background): Calls precheck once per page; collects results into a list of per-page records.
System: Returns one verdict per page — the per-page result is the union of every check, not just the first failure within that page. Whether to fail-fast or render all results is the caller's policy, not the precheck's.

#### Branch: New error code added in lockstep with a new validation rule

User: A future feature introduces a new validation rule (e.g., a new constraint in @adapter-vocabulary-extension) and a corresponding stable error code.
System (background): The precheck registers the new code in its exhaustive list and runs the new check alongside existing checks; the verdict shape stays closed.
System: Existing callers continue to work — they already branch on `code == ok` versus everything else, so a new failure code surfaces as "not ok" without code changes. Callers that want to specialize on the new code update at their leisure.

#### Branch: Caller wants to surface the verdict but not block

User: Studio sync receives a non-`ok` verdict from precheck and wants to annotate the page in the UI without blocking the round-trip.
System: The precheck does not auto-fix and does not enforce policy — it returns the verdict. The caller decides what to do: refuse to proceed, annotate state, or surface to the human. The precheck never silently mutates the layout.

---
