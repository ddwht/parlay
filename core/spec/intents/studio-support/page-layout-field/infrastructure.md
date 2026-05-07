# Page-layout-field — Infrastructure

---

## Optional Layout Block in Page Artifacts

**Affects**: page-artifact loading and page-schema validation
**Behavior**: Page artifacts gain an optional layout-bearing top-level body section. When present, the section's body is decoded as a typed layout tree carrying a component-vocabulary identifier, a schema-version, and a recursive nodes structure; the parsed page exposes a layout field alongside its existing fields. When absent, the page parses identically to today's pages and downstream consumers see no layout field. The section is recognized by its heading regardless of position among siblings, so a page that places it first and a page that places it after other top-level sections produce identical parse trees. The block is forbidden from carrying wiring information (data sources, bindings, expression strings); wiring lives in the layout-aware codegen pass.
**Invariants**:
- A page with no layout-bearing section parses successfully and exposes no layout field
- A page with a well-formed layout-bearing section parses successfully and exposes a layout field carrying component-vocabulary, schema-version, and nodes
- A page declaring a component-vocabulary that no installed adapter registers fails parse with an error naming the offending vocabulary identifier and the page path
- A page whose layout omits schema-version fails parse with an error naming the missing field and the page path
- A page whose layout contains any wiring field fails parse with an error naming the offending field, the offending node identifier, and the page path
- A page whose layout uses a raw spacing or padding value where a token reference is required fails parse with an error naming the offending node identifier, the offending field, and the page path
- A hand-edited page that places the layout-bearing section after other top-level sections parses to the same tree as a page that places it immediately after the frontmatter
- A page round-tripped through Studio's design loop preserves every layout-node identifier across the read-edit-write cycle, except for newly added nodes which are minted fresh identifiers
**Source**: @studio-support/page-layout-field/add-optional-layout-block-to-the-page-schema
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Pages without a layout-bearing section must remain bit-for-bit equivalent to today's pages in all parser-visible respects; this is the load-bearing guarantee that lets existing projects adopt the change without migration.
- The layout block is embedded in the existing page artifact rather than introduced as a parallel file. Designer-authored layout and developer-authored prose live in one artifact; parallel editing happens through different sections of the same Markdown file.
- Stability of layout-node identifiers across round-trips is the contract Studio relies on to match canonical nodes to Figma nodes during sync.

---

## Layout Tree Schema

**Affects**: layout-tree validation
**Behavior**: A layout schema declares the typed-tree shape that writers (Studio's design-loop) and readers (codegen, validation tooling) agree on. The schema enumerates the universal node fields that apply to every node regardless of vocabulary (identifier, type, children) and the universal layout-container fields (direction, gap, padding, alignment). Vocabulary-specific node fields (per-component properties such as a header label or a density variant) are not enumerated in the schema directly; instead the schema declares the rule that those fields are validated against the adapter's component definition for the node's declared type. The schema declares which fields are required versus optional, and which fields expect token references rather than raw values. Validation walks the tree recursively to arbitrary depth and reports every offending node by identifier and field.
**Invariants**:
- A tree using only universal fields validates successfully regardless of which component-vocabulary is declared
- A tree referencing a component-type not defined in the declared vocabulary fails validation with an error naming the offending node identifier and the offending type
- A tree using a raw value where a token reference is required fails validation with an error naming the offending node identifier and the offending field
- A tree nested to arbitrary depth validates each level against the same rules; depth carries no special cap
- The layout-tree schema-version is pinned alongside the page-schema-version; a layout block declaring a schema-version not supported by the current build fails validation with an error naming the version found and the version expected
- Every validation error names the offending node identifier and the offending field, so a designer fixing a Figma sync rejection knows exactly where to look
**Source**: @studio-support/page-layout-field/define-the-layout-tree-schema
**Caching**: per-process
**Backward-Compatible**: yes

**Notes**:
- The vocabulary itself comes from adapter registrations (see @studio-support/adapter-vocabulary-extension); the tree shape — how vocabulary nodes nest and what shared layout properties are allowed — is owned here.
- Validation messages that name the offending node identifier and the offending field are load-bearing: they let Studio's sync flow point a designer directly at the offending element on the canvas.

---

## Layout Precheck Contract

**Affects**: layout validation surfaced uniformly to consumers (codegen, status, repair, sync)
**Behavior**: A single callable validation entry point — the layout-precheck — runs every layout check (well-formedness, schema compliance, vocabulary membership, token correctness, no-wiring-in-layout) and returns a structured verdict. Input is a parsed page artifact plus the active adapter; output is either a passing verdict carrying only an ok code, or a structured failure record carrying a stable error code, the page-artifact path, the layout-node path inside the file, the offending value, the expected shape, and a fix hint. Every failure record carries exactly these fields and no others. The precheck aggregates every check defined in this feature and in @studio-support/adapter-vocabulary-extension; new error codes are added in lockstep with new validation rules. Consumers (codegen, parlay-status, parlay-repair, Studio sync) call the precheck and decide policy on the verdict — the precheck never auto-fixes and never enforces policy. The precheck is a pure function over its inputs: same input always produces byte-identical output, no AI calls, no external state, sub-millisecond on common-case pages.
**Invariants**:
- A passing verdict carries exactly one field, the ok code; no other fields appear on success paths
- A failing verdict carries exactly six fields — code, file, node-path, found, expected, fix — across every failure code; the failure shape is closed
- A page with a malformed layout-bearing block returns a verdict with the malformed-layout-block code, the page path, the parser's verbatim error message in the found field, and the parser-emitted next step in the fix field
- A page declaring a component-vocabulary version that does not match the adapter's registered version returns a verdict with the vocabulary-version-mismatch code, the page path, the declared version in found, the registered version in expected, and a fix naming both remediation paths
- A page using a raw value where a token reference is required returns a verdict with the raw-value-where-token-required code, the page path, the offending node path, the raw value in found, the list of valid tokens in expected, and a fix naming the substitution
- A page whose layout references a component-type outside the declared vocabulary returns a verdict with the unknown-component-type code, the page path, the offending node path, the offending type in found, the vocabulary's known type list in expected, and a fix naming both remediation paths
- The same page artifact and adapter pair produces byte-identical verdicts across repeated calls
- The precheck performs no AI calls and reads no external state beyond its inputs
- A walk of multiple pages produces a list of per-page verdicts; the list is the union of every check across every page, not a single global verdict, and not just the first failure
- Adding a new validation rule in this feature or in @studio-support/adapter-vocabulary-extension adds a new stable error code in lockstep; the verdict shape stays closed
**Source**: @studio-support/page-layout-field/surface-layout-validation-as-a-precheck-contract-for-codegen
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The closed verdict shape is the load-bearing design choice: every failure carries exactly the same fields so callers branch on code-equals-ok versus everything else, and the failure shape never sneaks into success paths. There is no severity field; every failure is an error. If a future requirement surfaces a soft case, it is added as a new closed-shape variant or a separate verdict kind, not by widening this one.
- Stable error codes registered by this feature: malformed-layout-block, missing-schema-version, vocabulary-version-mismatch, unknown-component-type, raw-value-where-token-required, wiring-in-layout. Additional codes (unknown-variant, unknown-token, universal-field-redeclared, missing-mode-emit-form) are listed in this feature's intents for completeness but are owned and Verify-cased by sibling features (@studio-support/adapter-vocabulary-extension and the page-schema feature).
- Determinism is testable and CI-stable: the verdict struct is the deterministic part of the system even though codegen's emitted code text downstream is not.
- Aggregating verdicts across many pages is a list of per-page records, not a single global verdict — callers decide whether to fail-fast on the first failure or collect all failures.

---
