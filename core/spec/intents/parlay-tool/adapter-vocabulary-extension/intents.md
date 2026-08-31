# Component and Token Vocabulary in Adapters

> Extend the adapter schema so each adapter declares its design-system component vocabulary (e.g., `clarity@17` — the named components, their valid variants, the per-component property shape) and its design tokens (spacing, color, typography). Core's layout-aware codegen consumes this to generate framework-specific code; Studio's Design Loop consumes it to validate Figma round-trips against an explicit allowlist. Without this declaration, neither side has a deterministic answer to "is this layout valid?"

---

## Declare a Named Component Vocabulary in the Adapter

**Goal**: Make each adapter explicit about which design-system components it supports, what variants those components accept, and what properties are required vs optional — so layouts referencing the vocabulary can be machine-validated and codegen has a typed source for component definitions.
**Persona**: UX Designer (using the adapter)
**Priority**: P0
**Context**: Today, an adapter knows how to emit code in a specific framework (Angular, React) but does not formally publish what design-system components it supports. The typed-tree layout format that downstream tooling consumes requires a registry of valid `type` values and their property shapes; without an adapter-side declaration, layout validation and codegen have no source of truth for "is this component allowed and what does it accept".
**Action**: Add a top-level `componentVocabulary:` section to the adapter schema with a versioned name (e.g., `clarity@17`) and a list of components. Each component declares its `type` (the string layout files reference), its valid `variants`, its required and optional properties (with types — string, token-reference, enum, boolean, child-list), and the component's category (container, leaf, data-shape).
**Objects**: adapter, component-vocabulary, component-definition, variant, property-shape

**Constraints**:
- The vocabulary name is versioned (e.g., `clarity@17`, not just `clarity`); a layout file pinned to a version must fail-fast against an adapter declaring a different version
- Property types are a closed set: `string`, `token-reference`, `enum` (with values listed), `boolean`, `int`, `child-list` (with allowed child types)
- A component's allowed children are declared explicitly — a `clarity.datagrid` declares it accepts `clarity.datagrid-column` children; a `clarity.region` declares it accepts any visual component. This is what makes "components not in the vocabulary" detectable at sync time
- Variants are a closed enum per component (e.g., `clarity.button` variants are `primary | secondary | tertiary | danger`)
- Component definitions are the same shape across adapters — what differs is the vocabulary content, not the schema. A `react-clarity` adapter declaring `clarity@17` carries the same component definitions as the Angular adapter, just with a different framework-mapping section
- The vocabulary lives in the adapter file itself (not a separate file) because the adapter already ships through `parlay register-adapter`
- Universal container properties — `direction`, `gap`, `padding`, `alignment` — are NOT declared in `componentVocabulary`. They live in the layout schema as fields available to every container node. Vocabulary declarations carry only the per-component overlay: variants and vocabulary-specific properties (e.g., `headerLabel`, `density`)
- Cross-adapter vocabulary sync is out of scope for this feature. Until a second adapter declares the same vocabulary version, two adapters that both ship `clarity@17` keep their `componentVocabulary:` blocks identical by hand; an include/import mechanism is deferred until a real second adapter exists to validate the design

**Verify**:
- An adapter declaring `clarity@17` with a representative component set (region, heading, button, datagrid, datagrid-column) parses cleanly
- A layout file referencing `clarity.datagrid-column` validates against the adapter's declaration; a layout file referencing `clarity.foobar` fails validation with the offending type and the adapter version named
- A layout file using a button variant not in the adapter's enum (`mega-button`) fails validation
- Two adapters declaring `clarity@17` (one Angular, one React) declare structurally identical `componentVocabulary:` sections — diff is empty across that section
- An adapter that declares a universal field (`direction`) inside a `componentVocabulary` component entry fails validation with an error pointing at the universal-fields rule
- Removing a component from the adapter's vocabulary causes existing layouts that reference it to fail validation on the next codegen pass

---

## Declare Design Tokens in the Adapter

**Goal**: Make each adapter explicit about its design tokens — spacing, color, typography — so layouts can use named tokens instead of raw values and Studio can validate Figma round-trips against the token set.
**Persona**: UX Designer (using the adapter)
**Priority**: P0
**Context**: Layout files reference tokens (`spacing-lg`, `color-primary`) rather than raw pixel/hex values. Without an adapter-side token registry, Studio cannot validate that a Figma edit using a non-token spacing value is out of vocabulary, and codegen cannot translate the token reference into a framework-appropriate emission.
**Action**: Add a `tokens:` section to the adapter schema with three subsections: `spacing` (named scale), `color` (named palette), `typography` (named text styles). Each token entry declares its name and a per-theme-mode emit-form (e.g., `light`, `dark`, named themes) for the adapter's framework — CSS variable, Sass variable, theme-object key, whatever the design system uses. Mode-invariant tokens declare a single emit-form that applies to all modes.
**Objects**: adapter, design-token, spacing, color, typography, theme-mode

**Constraints**:
- Token names are stable across same-design-system adapters: `clarity@17` for Angular and `clarity@17` for React must declare the same token *names*, even if the emit-form differs
- Spacing values are an ordered scale (e.g., `spacing-xs`, `spacing-sm`, `spacing-md`, `spacing-lg`, `spacing-xl`) — the ordering matters for sync-back warnings ("this Figma frame uses a tighter spacing than spacing-sm — did you mean spacing-xs?")
- The token list is closed within an adapter version. Adding a token is a vocabulary version bump
- Color tokens may carry semantic and presentation metadata — `tone: neutral | info | warning | danger | success`. This tone vocabulary is shared with the domain model's enum-tone metadata so a domain enum value tagged `danger` resolves to the same color token in every page that renders it; the alignment is intentional, not coincidental
- Typography tokens declare their use site (`heading-page`, `heading-section`, `body`, `caption`) rather than physical properties — physical mapping is an emit detail
- The token source-of-truth is the design system, not parlay. Adapters declare what the design system declares; an adapter cannot invent tokens that do not exist in the underlying system
- The adapter file is the runtime source of truth for tokens at codegen time. Studio reads tokens from the adapter, never from a live MCP fetch. An MCP fetch may be used as an authoring aid when the adapter author updates the declaration, but it is never on the codegen path — this keeps codegen deterministic and offline-capable
- Theme modes are part of the declaration from day one. Each adapter declares at least one mode (typically `light`) and may declare additional modes (`dark`, named themes). Tokens that vary by mode declare an emit-form per mode the adapter supports; tokens that are mode-invariant (most spacing and typography) declare a single emit-form
- Mode names are stable across same-design-system adapters: `clarity@17` for Angular and `clarity@17` for React both declare the same mode names. Adding or renaming a mode is a vocabulary version bump
- A page's selected mode (defaulting to the adapter's first declared mode) drives codegen emission. Generated code is per-mode-aware — switching the active mode at runtime does not require re-running codegen

**Verify**:
- An adapter declaring `clarity@17` includes spacing, color, and typography sections with a complete named scale (e.g., `spacing-xs` through `spacing-xl`, plus a baseline color palette and typography use-sites)
- A layout file using `gap: spacing-lg` validates against the adapter's token list
- A layout file using `gap: 24px` (raw value) fails validation with a clear error explaining tokens are required
- A layout file using `gap: spacing-mega` (unknown token) fails validation naming the unknown token and listing valid alternatives
- Codegen translates `spacing-lg` to the adapter's emit-form — a CSS variable for one adapter, a theme-object lookup for another — without the layout file changing
- An adapter declaring `light` and `dark` color modes for `color-surface` produces generated code that renders correctly in both modes; switching modes at runtime does not require re-running codegen
- A color token that declares an emit-form for `light` but omits `dark` in an adapter that supports both modes fails parse and the layout-precheck (defined in `page-layout-field`) surfaces the failure as `{code: missing-mode-emit-form, file: <adapter-path>, found: <token-name>, expected: dark, fix: declare a dark emit-form for <token-name> or remove dark from the adapter's mode list}`. This is the canonical Verify for the `missing-mode-emit-form` code; `page-layout-field` lists the code in its registry but defers the rule-Verify here

---
