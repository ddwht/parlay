# Adapter-vocabulary-extension — Infrastructure

---

## Component Vocabulary Declaration in Adapter

**Affects**: adapter schema (top-level `componentVocabulary` section), adapter parser, adapter validator, adapter registry
**Behavior**: Each adapter declares a versioned, named component vocabulary (e.g., `clarity@17`) listing the design-system components it supports. Each component declaration carries its `type` string (the value layout files reference), its `category` (`container`, `leaf`, `data-shape`), a closed enum of allowed `variants`, a list of required and optional properties (with types drawn from a closed type set), and — for containers — the explicit list of allowed child component types. The adapter parser loads this section, the adapter validator enforces shape and closed-set rules, and the adapter registry exposes the parsed vocabulary to downstream consumers (layout validation, codegen, Studio sync).
**Invariants**:
- Vocabulary names are versioned: a bare name without `@<version>` fails parse with an error naming the offending vocabulary
- Property types are restricted to a closed set: `string`, `token-reference`, `enum` (with values listed), `boolean`, `int`, `child-list` (with allowed child types). Any other type fails validation naming the component, the property, and the disallowed type
- Variants are a closed enum per component; any layout reference to a variant outside the enum fails validation listing the offending variant and the allowed alternatives
- Universal container fields (`direction`, `gap`, `padding`, `alignment`) are NOT declared inside `componentVocabulary` component entries. Declaring any of them inside a component entry fails validation with an error pointing at the universal-fields rule
- Container components declare allowed child component types explicitly; layouts placing a disallowed child fail validation naming the parent type, the allowed child set, and the offending child
- Vocabulary content (component definitions) is structurally identical across adapters declaring the same vocabulary version — only the framework-mapping section differs. Cross-adapter parity is held by hand until a shared-include mechanism lands
- Removing a component from the vocabulary causes any layout still referencing the removed type to fail at the next validation pass naming the layout file, the component, and the vocabulary version
**Source**: @adapter-vocabulary-extension/declare-a-named-component-vocabulary-in-the-adapter
**Caching**: per-process — the parsed vocabulary is cached after first parse of an adapter file within a CLI invocation
**Backward-Compatible**: yes — adapters that omit `componentVocabulary:` continue to parse; the field is optional but, when absent, layout validation against vocabulary references is skipped with a warning rather than failing

**Notes**:
- The vocabulary lives in the adapter file itself (not a separate file) because the adapter already ships through `parlay register-adapter`
- Cross-adapter parity is a manual sync today; an include/import mechanism is deferred until a real second adapter ships the same vocabulary version and validates the design need
- Layout pinning to a vocabulary version (e.g., `clarity@17`) is enforced fail-fast: a layout pinned to one version against an adapter declaring another version fails validation immediately, naming both versions

---

## Universal Container Fields in Layout Schema

**Affects**: layout schema (universal container fields available on every container node)
**Behavior**: Universal container fields — `direction`, `gap`, `padding`, `alignment` — live in the layout schema and are available on every container node in any vocabulary, regardless of which adapter is active. Vocabulary declarations carry only the per-component overlay (variants and vocabulary-specific properties such as `headerLabel`, `density`); they never re-declare a universal field. The layout parser recognizes these fields on every container and emits a single source of truth for their value types.
**Invariants**:
- The set of universal container fields is fixed: `direction`, `gap`, `padding`, `alignment`. Adding a field to the universal set is a schema change, not a vocabulary change
- Universal field value types are uniform across vocabularies (e.g., `gap` always takes a token-reference; `direction` always takes one of a fixed enum)
- A vocabulary entry that re-declares a universal field is rejected at adapter parse time, not at layout-validate time, so the offending adapter cannot register
**Source**: @adapter-vocabulary-extension/declare-a-named-component-vocabulary-in-the-adapter
**Caching**: none — universal-field rules are static and resolved inline during layout parse
**Backward-Compatible**: yes for layouts authored before this change (which used the legacy per-component re-declaration); a one-time migration step strips re-declarations from existing adapter files. New adapter files reject re-declarations from the start

**Notes**:
- This separation keeps the per-component overlay small and uniform, and lets layout authoring assume container chrome is always available without consulting the vocabulary
- The list is intentionally short; new universal fields require a schema change and migration of every adapter

---

## Design Token Declaration in Adapter

**Affects**: adapter schema (top-level `tokens` section with `spacing`, `color`, `typography` subsections), adapter parser, token validator, token registry
**Behavior**: Each adapter declares its design tokens grouped into spacing (an ordered named scale), color (a named palette with semantic tone metadata), and typography (named text styles keyed by use-site). Token names are part of the adapter's vocabulary version and are stable across adapters declaring the same vocabulary version (e.g., `clarity@17` for Angular and React both declare the same token names; only the per-mode emit-form differs by framework). The adapter file is the runtime source of truth at codegen time — Studio reads tokens from the adapter, never from a live design-system fetch.
**Invariants**:
- Token names are stable across same-design-system adapters: any drift in token *names* between two adapters declaring the same vocabulary version is reported as a parity violation
- The token list is closed within an adapter version; adding a token requires a vocabulary version bump
- Spacing tokens carry an ordered position within the scale; the ordering is preserved by the parser and exposed to consumers (it drives sync-back warnings such as "this Figma frame uses a tighter spacing than `spacing-sm` — did you mean `spacing-xs`?")
- Color tokens may carry a `tone` drawn from a fixed enum: `neutral | info | warning | danger | success`. This tone vocabulary is shared with the domain model's enum-tone metadata so a domain enum value tagged `danger` resolves to the same color token in every page that renders it
- Typography tokens declare a use-site (`heading-page`, `heading-section`, `body`, `caption`) rather than physical properties; physical mapping is a per-emit-form detail
- Adapters cannot invent tokens absent from the underlying design system; an authoring-time check warns when a token name does not appear in the upstream system
- The adapter file is the runtime source of truth at codegen time; codegen never reads from MCP. MCP fetches are an authoring aid only, used during adapter editing to diff against the upstream design system
**Source**: @adapter-vocabulary-extension/declare-design-tokens-in-the-adapter
**Caching**: per-process — the parsed token set is cached alongside the parsed component vocabulary for the same adapter
**Backward-Compatible**: yes — adapters that omit `tokens:` continue to parse; layout validation against token references is skipped with a warning until a `tokens:` block is added

**Notes**:
- Color-token `tone` enum alignment with the domain model's enum-tone metadata is intentional, not coincidental — this is an explicit cross-feature coupling with `domain-model-yaml-migration`
- The MCP-as-authoring-aid path produces a diff surface during adapter editing; the runtime codegen path never touches it

---

## Theme Modes in Token Declaration

**Affects**: adapter schema (theme-mode declaration at top of `tokens` section), token parser, token validator, codegen mode resolution
**Behavior**: Theme modes are part of the adapter declaration from day one. Each adapter declares at least one mode (typically `light`) and may declare additional modes (`dark`, named themes). Tokens that vary by mode declare an emit-form per mode the adapter supports; tokens that are mode-invariant (most spacing, all typography) declare a single emit-form. The token parser validates mode coverage; codegen resolves a page's selected mode (defaulting to the adapter's first declared mode) and emits per-mode-aware code so switching the active mode at runtime does not require re-running codegen.
**Invariants**:
- Every adapter declares at least one mode; an adapter with an empty or missing mode list fails parse
- Mode names are stable across same-design-system adapters declaring the same vocabulary version; drift is reported as a parity violation
- Adding or renaming a mode within a vocabulary version is rejected; the change requires a vocabulary version bump (e.g., `clarity@17` → `clarity@18`)
- Every mode-varying token declares an emit-form for every mode the adapter supports; a missing emit-form fails parse naming the token and the missing mode
- A page's selected mode defaults to the adapter's first declared mode; codegen output carries every mode's emission so runtime mode-switching does not invalidate generated code
**Source**: @adapter-vocabulary-extension/declare-design-tokens-in-the-adapter
**Caching**: per-process — declared-mode list is cached with the token set
**Backward-Compatible**: no — every adapter declaration must include a mode list from this change forward. Existing adapter files without a mode list need a one-line migration adding `modes: [light]`

**Notes**:
- Per-mode emission keeps codegen offline-capable and deterministic; runtime mode-switching is a presentation concern, not a regeneration trigger
- Mode-invariant tokens (most spacing, all typography) declare a single emit-form regardless of how many modes the adapter supports

---

## Layout Validation Against Vocabulary and Tokens

**Affects**: layout validation pipeline, vocabulary lookup, token lookup, error reporting
**Behavior**: When a layout file is validated, the pipeline resolves the layout's pinned vocabulary version against the active adapter, then looks up every component reference (`type`), every variant reference, every property name, every container child relationship, and every token reference. Mismatches produce errors that name the offending value, the vocabulary version, and the set of allowed alternatives. Raw values (e.g., `gap: 24px`) supplied where a token reference is required are flagged with the same shape of error and a list of available tokens.
**Invariants**:
- Validation errors name the offending value, the vocabulary version, and the set of allowed alternatives
- Layout pinning to a vocabulary version is enforced fail-fast: a version mismatch between layout and adapter halts validation before any component lookup
- A reference to a component, variant, property, or token absent from the adapter declaration fails validation with the offending value and the vocabulary version named
- A raw value where a token reference is required fails validation with a list of available tokens, not a generic type error
- A disallowed child type in a container fails validation naming the parent type, the allowed child set, and the offending child
**Source**: @adapter-vocabulary-extension/declare-a-named-component-vocabulary-in-the-adapter, @adapter-vocabulary-extension/declare-design-tokens-in-the-adapter
**Caching**: per-process — vocabulary and token lookups reuse the parsed adapter cache
**Backward-Compatible**: yes — layouts that do not pin a vocabulary version skip vocabulary validation with a warning; layouts that do pin one are validated strictly

**Notes**:
- This fragment is the consumer of the two declaration fragments above (component vocabulary and tokens) — it has no source-of-truth role; it only reads
- Downstream consumers of the same lookups (codegen, Studio Design Loop sync) reuse the same cache and the same lookup primitives

---

## Token-Aware Per-Mode Codegen Emission

**Affects**: codegen pipeline, token-to-emit-form translation, per-mode emission
**Behavior**: When codegen runs for a page against an active adapter, every token reference in the layout is translated into the adapter's emit-form for the page's selected mode. The same layout file produces different emitted code under different adapters (e.g., `gap: spacing-lg` becomes `gap: var(--spacing-lg)` for one adapter and `theme.spacing.lg` for another) without the layout file changing. Generated code is per-mode-aware: switching the active mode at runtime does not require re-running codegen, because the emission carries every mode's value for every mode-varying token referenced by the page.
**Invariants**:
- A token reference in a layout is always translated into the adapter's emit-form, never emitted as the literal token name
- The same layout produces structurally different emitted code under different adapters declaring the same vocabulary version, with no layout file change required
- Per-mode emission carries every supported mode's value for every mode-varying token referenced by the page; runtime mode-switching never triggers regeneration
- Mode-invariant tokens emit a single value regardless of mode count
- Codegen never reads tokens from a live design-system fetch; the adapter file is the only runtime source of truth at codegen time
**Source**: @adapter-vocabulary-extension/declare-design-tokens-in-the-adapter
**Caching**: per-process — emit-form lookups reuse the parsed adapter cache; codegen output is written, not cached in-process
**Backward-Compatible**: yes for adapters that previously emitted raw values — those continue to work; new adapters declaring `tokens:` opt in to token-aware emission

**Notes**:
- Per-mode emission is the reason codegen is deterministic and offline-capable: every value is in the adapter file at codegen time
- This fragment is the downstream dependency for `layout-aware-codegen`; the contract surfaced here is what that feature will consume
- `page-layout-field` is the upstream-of-page-layouts dependency: page layouts reference the same `componentVocabulary` and `tokens` declared here. The component-vocabulary and token-declaration fragments above are the source of truth that page layouts read against
