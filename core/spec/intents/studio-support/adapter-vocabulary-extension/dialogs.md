# Adapter-vocabulary-extension — Dialogs

---

### Declare a Named Component Vocabulary in the Adapter

**Trigger**: Adapter author edits an adapter file under `.parlay/adapters/` to add or revise the `componentVocabulary:` section.

User: Opens `.parlay/adapters/angular-clarity.adapter.md` and adds a top-level `componentVocabulary:` section with `name: clarity@17` and a list of components (region, heading, button, datagrid, datagrid-column).
User: For each component, declares `type`, `category` (container | leaf | data-shape), `variants` (closed enum), and required/optional properties with types drawn from the closed set: `string | token-reference | enum | boolean | int | child-list`.
User: Saves the file and runs `/parlay register-adapter angular-clarity`.
System (background): Parses `componentVocabulary:`, validates each component declaration against the adapter schema, and registers the vocabulary version `clarity@17`.
System: Adapter `angular-clarity` registered. Vocabulary `clarity@17` declares 5 components (1 container, 3 leaves, 1 data-shape).

#### Branch: Property type outside the closed set

User: Declares a component property with `type: object` (not in the closed set).
System: Validation failed for component `clarity.foo` property `config`: type `object` is not allowed. Closed set is: `string`, `token-reference`, `enum`, `boolean`, `int`, `child-list`. Re-declare using one of these or split the property into typed sub-properties.

#### Branch: Universal container field declared inside the vocabulary

User: Declares `direction` (or `gap`, `padding`, `alignment`) as a property inside a `componentVocabulary` component entry.
System: Validation failed for component `clarity.region` property `direction`: universal container fields (`direction`, `gap`, `padding`, `alignment`) live in the layout schema, not in `componentVocabulary`. Every container node already accepts these. Remove the entry from the vocabulary; the field stays available on the node automatically.

#### Branch: Variant declared outside the enum

User: Authors a layout that references `clarity.button` with `variant: mega-button`.
System (background): Resolves the layout's pinned vocabulary (`clarity@17`) and looks up the adapter's variant enum for `clarity.button` — `primary | secondary | tertiary | danger`.
System: Validation failed: `clarity.button` does not declare variant `mega-button` in `clarity@17`. Allowed variants: `primary`, `secondary`, `tertiary`, `danger`. Either pick an allowed variant in the layout, or bump the vocabulary version and add `mega-button` to the adapter declaration.

#### Branch: Component referenced is not in the vocabulary

User: Authors a layout that references type `clarity.foobar`.
System: Validation failed: `clarity.foobar` is not a declared component in `clarity@17`. Declared types are: `clarity.region`, `clarity.heading`, `clarity.button`, `clarity.datagrid`, `clarity.datagrid-column`. Either correct the type or add `clarity.foobar` to the adapter vocabulary (which requires a vocabulary version bump).

#### Branch: Vocabulary version mismatch

User: Layout file is pinned to `clarity@17` but the active adapter declares `clarity@18`.
System: Validation failed fast: layout pins vocabulary `clarity@17` but adapter `angular-clarity` declares `clarity@18`. Either re-pin the layout to `clarity@18` (re-validating component and variant references) or install an adapter that declares `clarity@17`.

#### Branch: Disallowed child type

User: Authors a layout placing a `clarity.button` directly inside `clarity.datagrid` (which only accepts `clarity.datagrid-column` children).
System: Validation failed: `clarity.datagrid` declares allowed children `[clarity.datagrid-column]`. Found child `clarity.button`. Wrap the button in a `clarity.datagrid-column` cell renderer, or move it outside the datagrid.

#### Branch: Component removed from vocabulary

User: Removes `clarity.callout` from the adapter's `componentVocabulary:` and re-runs codegen on a layout that references it.
System: Codegen aborted: layout `pages/dashboard.page.md` references `clarity.callout`, which is no longer declared in `clarity@17`. Either restore the component to the adapter (and bump the vocabulary version), or remove the reference from the layout.

#### Branch: Cross-adapter parity check

User: Registers a second adapter `react-clarity` also declaring `clarity@17`.
System (background): Diffs the `componentVocabulary:` sections of `angular-clarity` and `react-clarity` for vocabulary version `clarity@17`.
System (condition: identical): Both adapters declare structurally identical `componentVocabulary:` blocks for `clarity@17`. Cross-adapter parity is held by hand for now — when the two declarations drift, re-sync manually until a shared-include mechanism lands.
System (condition: drift detected): Vocabulary `clarity@17` differs across adapters. ==list of differing components/variants/properties==. Reconcile by hand: pick one adapter as the source and copy the vocabulary block verbatim into the other.

---

### Declare Design Tokens in the Adapter

**Trigger**: Adapter author edits an adapter file to add or revise the `tokens:` section, or a layout author references a token that needs to be resolved at codegen time.

User: Opens `.parlay/adapters/angular-clarity.adapter.md` and adds a `tokens:` section with three subsections: `spacing`, `color`, `typography`.
User: Declares the adapter's theme modes — at minimum `light`, optionally `dark` or named themes — at the top of the `tokens:` section.
User: For each spacing token (`spacing-xs` through `spacing-xl`), declares an ordered position and a single emit-form (mode-invariant).
User: For each color token, declares a `tone` (`neutral | info | warning | danger | success`) and a per-mode emit-form covering every declared mode.
User: For each typography token, declares its use-site name (`heading-page`, `heading-section`, `body`, `caption`) and a single emit-form.
User: Saves the file and runs `/parlay register-adapter angular-clarity`.
System (background): Parses `tokens:`, validates that every color token covers every declared mode, validates the spacing scale ordering, and registers the token set under vocabulary version `clarity@17`.
System: Adapter `angular-clarity` registered. Tokens for `clarity@17`: 5 spacing, 12 color (across modes `light`, `dark`), 4 typography.

#### Branch: Layout uses a raw value where a token is required

User: Authors a layout with `gap: 24px`.
System: Validation failed: `gap` requires a token reference, not a raw value. Replace `24px` with a declared spacing token. Available: `spacing-xs`, `spacing-sm`, `spacing-md`, `spacing-lg`, `spacing-xl`.

#### Branch: Layout references an unknown token

User: Authors a layout with `gap: spacing-mega`.
System: Validation failed: `spacing-mega` is not declared in `clarity@17`. Available spacing tokens: `spacing-xs`, `spacing-sm`, `spacing-md`, `spacing-lg`, `spacing-xl`. Either pick an allowed token or add `spacing-mega` to the adapter (which requires a vocabulary version bump).

#### Branch: Color token missing an emit-form for a declared mode

User: Adapter declares modes `light` and `dark`. Author declares `color-surface` with an emit-form for `light` only.
System: Parse failed for token `color-surface`: adapter declares mode `dark` but the token omits an emit-form for it. Either add a `dark` emit-form to `color-surface`, or remove `dark` from the adapter's declared modes (which is a vocabulary version bump).

#### Branch: Mode name renamed or added

User: Renames mode `dark` to `night` (or adds a new mode `high-contrast`) without bumping the vocabulary version.
System: Parse failed: mode names are stable across same-design-system adapters. Renaming `dark` -> `night` or adding `high-contrast` to `clarity@17` requires a vocabulary version bump (e.g., `clarity@18`). Confirm the bump and update every adapter that ships this vocabulary, or revert the change.

#### Branch: Codegen translates a token into the framework emit-form

User: Runs codegen for a page using `gap: spacing-lg` against `angular-clarity`.
System (background): Resolves `spacing-lg` against the adapter's emit-form for `spacing-lg` (a CSS variable in this adapter), substitutes it into the generated template, and writes the output.
System: Generated `dashboard.component.html`. `gap: spacing-lg` emitted as `gap: var(--spacing-lg)`.
User: Switches the active adapter to `react-clarity` (which emits `spacing-lg` as a theme-object key) and re-runs codegen on the same layout.
System: Generated `Dashboard.tsx`. `gap: spacing-lg` emitted as `theme.spacing.lg`. Layout file unchanged.

#### Branch: Page selects a non-default mode for codegen

User: Sets the page's active mode to `dark` (the adapter's first declared mode is `light`).
System (background): For each color token referenced by the page, picks the `dark` emit-form; for mode-invariant tokens, picks the single declared form.
System: Generated code is per-mode-aware for `dark`. Switching the active mode at runtime does not require re-running codegen; the emission carries both modes' values.

#### Branch: Token tone aligned with domain enum

User: Domain enum value `OrderStatus.cancelled` is tagged `tone: danger` in the domain model. Page renders the value via the adapter.
System (background): Resolves the enum's tone to the matching color token in the adapter (e.g., `color-status-danger`).
System: Renders `OrderStatus.cancelled` with `color-status-danger` from `clarity@17`. Tone-to-token alignment is intentional — the domain's `danger` and the adapter's `danger` color resolve through the same vocabulary.

#### Branch: Author tries to invent a token absent from the design system

User: Adds `color-brand-special` to the adapter that does not exist in the underlying Clarity design system.
System: Adapters declare what the design system declares — they do not invent tokens. Either add `color-brand-special` to the upstream Clarity definition first, or pick an existing color token. The adapter file is the runtime source of truth at codegen time, but its contents must mirror the design system.

#### Branch: MCP fetch used as authoring aid (never on codegen path)

User: Runs an MCP fetch against the Clarity design system to re-pull the canonical token list during adapter authoring.
System (background): Fetches the latest token names and values from the design system MCP and surfaces a diff against the adapter file.
System: MCP returned ==N== tokens; ==M== differ from the adapter file. Apply the diff into the adapter manually if the upstream change is intended. Note: codegen never reads from MCP — only from the adapter file. This fetch is an authoring aid only.

---
