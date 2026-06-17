<!--
parlay-section: cross-cutting
parlay-extends: studio-support/adapter-vocabulary-extension/adapter-schema-component-vocabulary-section
parlay-extends: studio-support/adapter-vocabulary-extension/adapter-schema-tokens-section
parlay-extends: studio-support/adapter-vocabulary-extension/adapter-schema-theme-modes
parlay-extends: parlay-tool/multi-adapter/adapter-kind-discriminator
parlay-extends: parlay-tool/multi-adapter/adapter-supports-contract
-->

# Framework Adapter Schema

File: `.parlay/adapters/<adapter-name>.adapter.yaml`
Registered via `/parlay register-adapter` or bundled during `/parlay init`.

A framework adapter is a **two-level artifact**:

1. **Framework vocabulary** — maps the surface interaction vocabulary (Shows, Actions, Flows) to framework-specific widgets. This is the baseline, shared across teams using the same framework.
2. **Team implementation patterns** — composition recipes, conventions, and coding standards that define HOW generated code should be structured. This is team-owned and frequently customized.

Parlay ships adapter TEMPLATES with the widget mappings pre-filled. Teams customize the compositions, conventions, and patterns sections to match their codebase standards. The adapter is the team's "coding standards for generated code" — they own it, version it, and evolve it.

The adapter has no knowledge of the project's domain, features, or data. It answers two questions: "what framework widget implements this interaction?" and "how does our team structure the generated code?"

## Structure

```yaml
name: <adapter name — e.g., go-cli, react-antd, angular-clarity, ios-uikit>
framework: <human-readable framework name — e.g., "Go CLI", "React + Ant Design">
version: <adapter version>
kind: <presentation | transport | application | persistence>   # optional; absent means presentation

# --- Section 0: Kind discriminator ---
# Declared once per adapter file. Closed set:
#   presentation — UI-rendering adapters (react-antd, angular-clarity, go-cli)
#   transport    — protocol/transport adapters (openapi-rest, grpc)
#   application  — application-layer adapters (nestjs-application, fastapi)
#   persistence  — persistence-layer adapters (prisma-postgres, typeorm-postgres)
# A missing kind: field is treated as the legacy presentation default.

# --- Section 0.5: Supports contract (non-presentation kinds only) ---
# Required when kind is transport, application, or persistence.
# Forbidden when kind is presentation (or absent).
supports:
  operation_kinds: [<entries from operation-kinds.schema.md closed set>]
  steps:           [<entries from steps.schema.md closed set>]
  policies:        [<entries from policies.schema.md closed set>]
  errors:          [<entries from errors.schema.md closed set>]

# --- Section 1: Framework vocabulary (shared baseline) ---

shows:
  <surface-show-type>:
    widget: <framework-specific widget or "not-applicable">
    description: <how this renders in this framework>
    import: <framework import path, if applicable>
    requires: <"custom-implementation" if no built-in primitive exists>

actions:
  <surface-action-type>:
    widget: <framework-specific widget or "not-applicable">
    description: <how this interaction works in this framework>
    import: <framework import path, if applicable>
    requires: <"custom-implementation" if no built-in primitive exists>
    requires-confirmation: <true — only for invoke-destructive>

flows:
  <surface-flow-type>:
    pattern: <framework-specific composite pattern name>
    description: <how this flow is implemented in this framework>
    regions: [<region names this pattern provides>]

# --- Section 2: Composition recipes (team-customizable) ---

compositions:
  <recipe-name>:
    trigger: <when to use this recipe — surface vocabulary conditions>
    state: [<runtime state variables this composition needs>]
    wiring: <how components/widgets connect — event flow description>
    description: <human-readable explanation of the pattern>

# --- Section 3: Conventions (team-customizable) ---

conventions:
  <convention-name>:
    rule: <structured rule the agent must follow>
    applies-to: <scope — "all components", specific surface terms, or conditions>

# --- Section 4: File conventions (team-customizable) ---

file-conventions:
  source-root: <where generated code lives — e.g., "src/", "cmd/", "app/">
  component-pattern: <how components map to files — e.g., "one-file-per-component", "feature-modules">
  naming: <file naming convention — e.g., "kebab-case", "snake_case", "PascalCase">
  entry-point: <main file — e.g., "main.go", "main.ts", "App.tsx">

# --- Section 5: Design system inventory ---

design-system:
  <category-name>:
    source: <framework | figma | not-defined>
    format: <how to use it — token names, component props, API>
    usage: <rules for the agent — what to do and what to avoid>

# --- Section 6: Design patterns (team-customizable) ---

patterns:
  interaction:
    prefer: [<preferred interaction patterns>]
    avoid: [<discouraged interaction patterns>]
  information-density:
    default: <low | medium | high>
    rationale: <why this density fits the framework>
  error-placement:
    default: <inline | toast | dialog | console>
    rationale: <why this fits the framework>
  confirmation:
    required-for: [<action types that need confirmation>]
    style: <prompt | dialog | inline>
  content:
    timestamps: <relative | absolute | both>
    empty-states: <message | hidden | placeholder>

# --- Section 7: Mount strategies (team-customizable) ---

mount-strategies:
  <strategy-name>:
    detection: <widget/pattern to grep for in source code>
    template: |
      <code template with {{placeholders}}>
    description: <when this strategy applies>

# --- Section 8: Component vocabulary (design-system-specific) ---

componentVocabulary:
  name: <versioned vocabulary identifier — e.g., clarity@17 (bare names without @<version> are rejected)>
  components:
    - type: <string referenced from layout files — e.g., clarity.button>
      category: <one of: container | leaf | data-shape>
      variants: [<closed enum of allowed variant values>]
      properties:
        - name: <property name as referenced in layout files>
          type: <one of: string | token-reference | enum | boolean | int | child-list>
          enum-values: [<allowed values when type is enum>]
          child-types: [<allowed child types when type is child-list>]
          required: <boolean>
      allowed-children: [<for containers only — explicit list of allowed child component types>]

# --- Section 9: Design tokens (design-system-specific) ---

tokens:
  modes: [<at least one mode — typically light; may include dark or named themes>]
  spacing:
    - name: <token name — e.g., spacing-md>
      order: <integer position within the ordered scale>
      emit-form: <single mode-invariant emit form — e.g., var(--spacing-md)>
  color:
    - name: <token name — e.g., color-status-danger>
      tone: <one of: neutral | info | warning | danger | success — shared with the domain enum-tone>
      emit-forms: [<per mode: e.g., "light:var(--color-danger-light)", "dark:var(--color-danger-dark)">]
  typography:
    - name: <use-site name — e.g., heading-page>
      use-site: <one of: heading-page | heading-section | body | caption>
      emit-form: <single mode-invariant emit form>
```

## Section 0: Kind discriminator

Every adapter declares which slot it occupies in the adapter-set topology via the top-level `kind:` field. The closed set is:

| Kind | Purpose | Example adapters |
|---|---|---|
| `presentation` | UI rendering — translates Shows/Actions/Flows into framework widgets | `react-antd`, `angular-clarity`, `go-cli` |
| `transport` | Protocol/transport — translates capability operations into wire-level calls | `openapi-rest`, `grpc` |
| `application` | Application layer — orchestrates operation steps, policies, transactions | `nestjs-application`, `fastapi` |
| `persistence` | Persistence layer — translates persistence steps into ORM/database calls | `prisma-postgres`, `typeorm-postgres` |

A missing `kind:` field is treated as the legacy `presentation` default — pre-feature adapter files continue to load. `parlay upgrade` offers an opt-in prompt to make the default explicit.

A `kind:` value outside the closed set fails validation with `adapter-kind-unknown` naming the offending value.

## Section 0.5: Supports contract

Adapters whose kind is transport, application, or persistence MUST declare a `supports:` block. The block has four sub-keys; each is a list drawn from a closed vocabulary:

| Sub-key | Closed vocabulary file |
|---|---|
| `operation_kinds` | `operation-kinds.schema.md` |
| `steps` | `steps.schema.md` |
| `policies` | `policies.schema.md` |
| `errors` | `errors.schema.md` |

The `supports:` block declares which terms the adapter can fulfill at codegen time. During `parlay build-feature`, every operation in the resolved capabilities.yaml is walked against the supports block of the adapter occupying the relevant slot. The build fails before any AI invocation when a feature requires a term the adapter does not declare.

| Code | When it fires |
|---|---|
| `adapter-supports-missing-operation-kind` | Operation declares a `kind:` value not in the adapter's `supports.operation_kinds`. |
| `adapter-supports-missing-step` | Operation declares a `step.type` not in the adapter's `supports.steps`. |
| `adapter-supports-missing-policy` | Operation declares a policy not in the adapter's `supports.policies`. |
| `adapter-supports-missing-error` | Operation declares an error not in the adapter's `supports.errors`. |
| `adapter-supports-unknown-term` | The adapter declares an entry that falls outside the closed vocabulary file. |
| `adapter-supports-shape-mismatch` | A `presentation` adapter declares a `supports:` block (forbidden), or a non-presentation adapter omits it (required). |

Pattern descriptions for non-presentation kinds (e.g., describing how an application adapter wires steps to NestJS controllers) live alongside `supports:` but are AI prompt material, not validator input.

## Presentation-only vocabulary

The `shows:`, `actions:`, and `flows:` sections are required ONLY for presentation adapters. Non-presentation adapters (transport, application, persistence) MAY omit them — those vocabularies don't apply to backend layers. The validation rules in section "Validation" below treat presence as required only when `kind:` is `presentation` (or absent).

## Section 1: Framework vocabulary

### Shows mapping

Every Show type from the surface vocabulary must appear in the `shows:` section. The adapter specifies which framework widget renders each information type.

| Surface Show | What to map |
|---|---|
| `data-value` | How a single value is displayed (label, badge, chip, fmt.Println) |
| `data-list` | How an ordered/unordered collection renders (ul/ol, bulleted-list, List component) |
| `data-table` | How rows × columns render (HTML table, tabwriter, DataGrid) |
| `data-tree` | How nested hierarchy renders (TreeView, indented-list, collapsible outline) |
| `data-chart` | How data visualization renders (Chart.js, D3, not-applicable for CLI) |
| `status` | How lifecycle state renders (badge color, icon, [OK]/[ERR] prefix) |
| `progress` | How completion renders (progress bar, percentage text, spinner) |
| `message` | How informational text renders (paragraph, alert box, fmt.Println) |
| `media` | How non-text content renders (img tag, video player, not-applicable for CLI) |
| `empty-state` | How absence renders (placeholder, illustration, simple message) |
| `summary` | How aggregated metrics render (card grid, stat line, headed-section) |
| `diff` | How state comparison renders (unified diff, side-by-side, colored +/- lines) |
| `timeline` | How chronological sequence renders (vertical timeline, activity log, bulleted dates) |
| `code` | How structured/formatted content renders (pre/code block, syntax-highlighted, indented) |

### Actions mapping

Every Action type from the surface vocabulary must appear in the `actions:` section. The adapter specifies which framework widget implements each interaction.

For actions that don't apply to the framework (e.g., `reorder` via drag-and-drop in a CLI), use `widget: not-applicable` with a description explaining why.

For actions that are conceptually supported but lack a built-in framework primitive (e.g., `undo`/`redo` in most frameworks), use `requires: custom-implementation`.

### Flows mapping

Every Flow type from the surface vocabulary must appear in the `flows:` section. The adapter specifies which composite pattern implements each flow and what layout regions it provides.

Flows are higher-level than Shows and Actions — they describe how multiple widgets and interactions compose into a coherent user experience. The adapter pattern name should be specific enough that two agents reading it produce structurally similar code.

## Section 2: Composition recipes

Compositions describe HOW common widget combinations work together at runtime. They capture the state management and event wiring patterns that the buildfile deliberately does not specify.

The agent uses compositions as implementation recipes: when it sees a component with matching surface terms (the `trigger`), it follows the recipe's state and wiring patterns.

```yaml
compositions:
  crud-table-with-drawer:
    trigger: "component has data-table + navigate-drill + inspect"
    state: [selectedItem, drawerOpen]
    wiring: "row-click sets selectedItem and opens drawer; drawer-close clears selectedItem"
    description: "Table with row selection opening a side panel for detail view"

  form-in-modal:
    trigger: "component has provide-structured-input + dismiss"
    state: [modalOpen, formInstance]
    wiring: "button opens modal; form-submit validates, saves, closes; cancel discards and closes"
    description: "Modal dialog containing a form for create/edit operations"

  multi-select-toolbar:
    trigger: "component has select-many + invoke-batch"
    state: [selectedRowKeys]
    wiring: "table rowSelection feeds toolbar badge count; bulk action executes and clears selection"
    description: "Table with checkboxes and a toolbar that appears when items are selected"

  wizard-steps:
    trigger: "fragment has flow: guided-flow or flow: onboarding"
    state: [currentStep, formData]
    wiring: "next validates current step then advances; back preserves data and decrements; complete submits all"
    description: "Multi-step form with progress indicator and back/next navigation"
```

Compositions are optional. If no composition matches, the agent uses its own judgment — the testcases will verify the resulting behavior regardless. Compositions improve consistency between agents, not correctness.

Teams customize compositions to match their codebase patterns. A team that uses Redux would write different state/wiring than a team using React Context. Both are valid — the adapter captures the team's choice so every generated component follows the same pattern.

## Section 3: Conventions

Conventions are structured rules that constrain the agent's implementation choices. They reduce variance between agents without requiring a DSL. The agent MUST follow conventions when generating code.

```yaml
conventions:
  state-management:
    rule: "useState for component-local state. React Context for page-level shared state. No external state libraries."
    applies-to: all components

  event-naming:
    rule: "Events use the format on{Action}{Target} — e.g., onOpenDrawer, onCreateTask, onCloseModal."
    applies-to: all emit effects

  data-fetching:
    rule: "Custom hooks (useQuery pattern) for API calls. Loading state renders Spin. Error state renders Result with retry button."
    applies-to: components with api-fetch data sources

  error-handling:
    rule: "notification.error() for async operation failures. Form.Item rules for synchronous field validation. Never use alert()."
    applies-to: all components

  file-structure:
    rule: "One file per component. Shared hooks in src/hooks/. Shared types in src/types/. No barrel exports."
    applies-to: file generation
```

Conventions are the most frequently customized section. Teams should review and adjust them during adapter setup. Conventions that are too generic ("write clean code") are useless — each convention should make a SPECIFIC choice that eliminates a decision point for the agent.

## Section 5: Design system inventory

The design system section is a structured inventory of where each category of design decisions comes from. It tells the agent: for colors, use framework tokens; for motion, check the design-spec; for icons, the framework doesn't define them.

Each category has three fields:

| Field | Required | Description |
|---|---|---|
| `source` | Yes | Where values come from: `framework` (use built-in tokens), `figma` (extract from design-spec), or `not-defined` (agent uses sensible defaults) |
| `format` | Yes (when source is `framework` or `figma`) | How to use the values — token names, component APIs, import paths |
| `usage` | No | Constraints for the agent — what to do and what to avoid |

### Standard categories

Every adapter should declare these categories. Use `source: not-defined` for categories the framework doesn't cover.

| Category | What it covers |
|---|---|
| `colors` | Brand, semantic (success/error/warning), text, background, border |
| `spacing` | Padding, margin, gaps — the spatial rhythm |
| `border-radius` | Corner rounding |
| `typography` | Font families, sizes, weights, line heights |
| `shadows` | Elevation and depth |
| `icons` | Icon set, import pattern, sizing |
| `motion` | Transitions, animations, timing functions |
| `layout` | Grid system, flex/flow utilities, responsive primitives |

When `source: framework`, the agent uses the framework's token system and never hardcodes values. When `source: figma`, the agent reads values from `.parlay/build/<feature>/design-spec.yaml` — a per-feature file generated by `/parlay-reference-design-spec` from a Figma design. If design-spec.yaml does not exist for a feature, the agent treats the category as `not-defined` and uses sensible defaults. When `source: not-defined`, the agent uses its judgment.

Teams can add custom categories beyond the standard set (e.g., `z-index`, `breakpoints`, `opacity`).

## Section 7: Mount strategies

Mount strategies describe HOW to integrate a new component into an existing file. They are used in brownfield projects where pages, routes, and navigation already exist in the source tree.

Each strategy has three fields:

| Field | Required | Description |
|---|---|---|
| `detection` | Yes | A string pattern the agent greps for in existing source files to identify this integration point |
| `template` | Yes | A code template with `{{placeholder}}` markers showing the shape of code to insert |
| `description` | Yes | When this strategy applies — helps the agent and the onboard skill decide which strategy to use |

### How mount strategies are used

Mount strategies are consumed by `generate-code` (step 14.5) when a surface fragment targets a Page that already exists in the source tree as a non-Parlay file (no `parlay-section:` marker). The agent:

1. Reads the surface fragment's `Page:` and `Region:` fields from the buildfile route
2. Finds the file implementing that page (by searching the source tree for the component name)
3. Reads the file content
4. Scans the adapter's `mount-strategies:` for strategies whose `detection` pattern appears in the file
5. Applies disambiguation:
   - **1 match**: proceeds automatically
   - **0 matches**: asks the user via AskUserQuestion ("file doesn't match any mount strategy — how should the component be added?")
   - **Multiple matches**: asks the user to choose which integration point, showing each match with its line number
6. Finds existing instances of the matched template pattern in the file — these serve as style examples for indentation, prop naming, and code conventions
7. Generates a new instance following the template with placeholders filled from the buildfile component data
8. Produces a reviewable diff for the user (apply / skip / edit)

Mount strategies are **optional**. If no `mount-strategies:` section exists, `generate-code` falls back to generating standalone files and the entry point as before (greenfield behavior). This ensures full backward compatibility.

### Relationship to compositions

Mount strategies and compositions serve different purposes:
- **Compositions** describe HOW widgets wire together **within** a component (state + events)
- **Mount strategies** describe WHERE a new component slots **into** an existing file (insertion point + template)

A component may use both: a composition for its internal wiring, and a mount strategy for its integration point in an existing page.

### Template placeholders

Templates use double-brace syntax: `{{key}}`, `{{label}}`, `{{Component}}`, `{{path}}`, etc. Placeholder names are freeform — the agent fills them from the buildfile component data (component name, route path, page name) and adapter conventions (naming, import style).

## Section 8: Component vocabulary

The `componentVocabulary:` section declares the closed list of design-system components an adapter exposes to layouts. It is the runtime source of truth for "what components exist, what variants they have, what properties they accept, and what children they allow." Layouts (and Studio's layout pipeline) validate every component reference, variant, property, and child relationship against this vocabulary.

### Versioned vocabulary name

The vocabulary `name` is a versioned identifier — e.g., `clarity@17`. Bare names without `@<version>` are rejected at parse time. The version suffix lets layouts pin themselves to a specific vocabulary revision and lets validation fail fast (with a `version-mismatch` error) before any component lookup runs when a layout pins a version the active adapter does not declare.

### Component declaration

Each entry in `components:` declares one design-system component:

| Field | Required | Description |
|---|---|---|
| `type` | Yes | The string layout files reference (e.g., `clarity.button`, `clarity.datagrid`). |
| `category` | Yes | One of the closed set `{container, leaf, data-shape}`. Containers hold children. Leaves are terminal. Data-shapes carry data without rendering chrome (e.g., a datagrid column descriptor). |
| `variants` | No | Closed enum of allowed variant values. References to variants outside this enum fail with `unknown-variant`. |
| `properties` | No | Per-component overlay properties (NOT universal container fields — see below). |
| `allowed-children` | Container only | Explicit list of allowed child component types. Required for containers; absent or empty for leaves and data-shapes. References to disallowed children fail with `disallowed-child`. |

Each property has its own shape:

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Property name as referenced in layout files. |
| `type` | Yes | One of the closed set `{string, token-reference, enum, boolean, int, child-list}`. Any other type fails parse with `unknown-property`. |
| `enum-values` | When type is enum | Allowed values for this property. |
| `child-types` | When type is child-list | Allowed child component types for this slot. |
| `required` | Yes | Whether layouts must supply a value. |

### Universal container fields (NOT here)

The fields `direction`, `gap`, `padding`, `alignment` are **universal**: they live on every container node in any vocabulary, regardless of which adapter is active. They are declared once in the layout schema and are NEVER repeated inside `componentVocabulary` component entries. An adapter that re-declares a universal field inside a component fails parse with `universal-field-redeclared`. See `internal/embedded/schemas/layout.schema.md` for the canonical definition.

### Cross-adapter parity

Two adapters declaring the same vocabulary version (e.g., `angular-clarity` and `react-clarity` both declaring `clarity@17`) MUST produce structurally identical `componentVocabulary` blocks. Drift between them is a parity violation reported by the cross-adapter parity check. Until a shared-include mechanism lands, parity is held by hand — a regression test compares the two blocks at registration time.

### Optional section

The `componentVocabulary:` section is optional. Adapters that omit it continue to parse and register cleanly. When a layout references a vocabulary against an adapter without one, vocabulary-reference validation is skipped with a warning rather than failing the build.

### Companion top-level `vocabulary:` block

Adapters that drive Design Loop vocabulary validation declare an additional optional top-level block `vocabulary:` alongside `componentVocabulary:`. The `vocabulary:` block carries the closed admissible sets the validator pins — `components`, `spacing_tokens`, `color_tokens`, and `layout_containers` — and is read by `parlay validate-vocabulary` and the in-process `studio/internal/vocabulary` library. The full shape, the per-subfield record schemas, and the two stable resolution-failure error codes (`vocabulary-missing-from-adapter`, `vocabulary-unknown-adapter`) are documented in `vocabulary.schema.md`. Adapters MAY omit the block; in that case Design Loop vocabulary validation surfaces `vocabulary-missing-from-adapter` and other adapter uses (codegen, registration) continue to work.

## Section 9: Design tokens

The `tokens:` section declares the design-system tokens an adapter emits during codegen. Tokens are referenced by name from layouts (e.g., `gap: spacing-lg`, `color: color-status-danger`) and translated to per-framework emit-forms (CSS variables, theme-object key paths, etc.) when code is generated.

### Theme modes

Every adapter declaring `tokens:` MUST declare at least one theme mode (typically `light`). Adapters MAY declare additional modes such as `dark` or named themes. An empty or missing `modes:` list fails parse with `at least one mode` is required.

Mode names are stable across same-design-system adapters declaring the same vocabulary version. Renaming a mode (e.g., `dark` → `night`) or adding a new mode within an existing vocabulary version is rejected — it requires a vocabulary version bump (e.g., `clarity@17` → `clarity@18`). This is the breaking-change posture: every adapter declaration must include a mode list from this change forward; existing adapters without one need a one-line migration adding `modes: [light]`.

Codegen output is **per-mode-aware**: for each token that varies by mode, emission carries every supported mode's value side-by-side. The page's selected mode defaults to the adapter's first declared mode and a runtime mode-switch never requires re-running codegen — the mode-varying emit-forms are all already present in the generated output. This is the contract that makes runtime theme switching free.

### Spacing tokens

Spacing tokens form an ordered named scale (e.g., `spacing-xs`, `spacing-sm`, `spacing-md`, `spacing-lg`, `spacing-xl`). Each token declares:

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Token name as referenced from layouts. |
| `order` | Yes | Position within the ordered scale; the parser preserves order and uses it for sync-back warnings if Studio re-orders. |
| `emit-form` | Yes | Single mode-invariant emit-form (most spacing is mode-invariant). |

### Color tokens

Color tokens form a named palette. Each token declares:

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Token name as referenced from layouts (e.g., `color-surface`, `color-status-danger`). |
| `tone` | No | One of the closed set `{neutral, info, warning, danger, success}` — shared with the domain model's enum-tone metadata so a status-bearing enum can map directly to its semantic color. |
| `emit-forms` | Yes | Per-mode emit-forms covering every mode the adapter declares. A missing form fails parse naming the offending token AND the missing mode (e.g., `color-surface missing emit-form for dark`). |

### Typography tokens

Typography tokens are named text styles keyed by use-site. Each token declares:

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Use-site name (typically the same as `use-site`). |
| `use-site` | Yes | One of the closed set `{heading-page, heading-section, body, caption}`. The logical role; the physical mapping (font, size, line-height) is an emit detail. |
| `emit-form` | Yes | Single mode-invariant emit-form (typography is mode-invariant in the supported design system). |

### Vocabulary-version-locked token list

The set of tokens within a vocabulary version is **closed**. Adding or removing a token requires a vocabulary version bump. This is the same lock as the component list — it lets layouts pin themselves to a token set with the same fail-fast guarantees they get for components.

### MCP fetches are an authoring aid only

The adapter file is the **runtime source of truth** at codegen time. Studio MAY use a live MCP fetch from the upstream design system as an authoring aid (to suggest tokens to add, to flag drift between the upstream system and the local adapter), but codegen NEVER fetches at generation time. This keeps codegen offline and reproducible. Adapters cannot invent tokens absent from the upstream design system; the authoring tool's job is to keep the adapter in sync.

### Optional section

The `tokens:` section is optional. Adapters that omit it continue to parse and register cleanly. When a layout uses a token-reference against an adapter without `tokens:`, token validation is skipped with a warning rather than failing the build.

## Validation

When an adapter file is loaded, the tool verifies:
- Every Show type from the surface vocabulary has an entry in `shows:`
- Every Action type from the surface vocabulary has an entry in `actions:`
- Every Flow type from the surface vocabulary has an entry in `flows:`
- Missing vocabulary entries are errors — the adapter must be comprehensive
- `widget: not-applicable` is allowed (with description explaining why)
- `requires: custom-implementation` is allowed (the agent writes the implementation)
- The `file-conventions` section is complete
- `compositions:`, `conventions:`, and `design-system:` sections are optional but recommended
- If `design-system:` is present, each category must have a `source:` field with value `framework`, `figma`, or `not-defined`
- If `mount-strategies:` is present, each strategy must have `detection:`, `template:`, and `description:` fields. `detection:` must be a non-empty string. `template:` must contain at least one `{{placeholder}}`
- If `componentVocabulary:` is present, the `name:` field MUST include `@<version>` (bare names are rejected). Every property `type:` must be drawn from the closed set `{string, token-reference, enum, boolean, int, child-list}`. Every component `category:` must be one of `{container, leaf, data-shape}`. Universal container fields (`direction`, `gap`, `padding`, `alignment`) MUST NOT appear inside any component's `properties:` — they live in the layout schema and re-declaring them fails parse with `universal-field-redeclared`.
- If `tokens:` is present, the `modes:` list MUST contain at least one mode. Every color token's `emit-forms:` must cover every declared mode (a missing per-mode form fails parse naming the token and the missing mode). Color token `tone:` (when present) must be one of `{neutral, info, warning, danger, success}`. Typography token `use-site:` must be one of `{heading-page, heading-section, body, caption}`.

## Relationship to buildfile

The buildfile references widget names from the adapter, not surface vocabulary terms. When the agent generates a buildfile from a surface + adapter:

1. Read the surface fragment's Shows/Actions/Flow
2. Look up each term in the adapter to get the framework-specific widget
3. Write the widget name into the buildfile

The buildfile is fully framework-specific. The surface vocabulary does not appear in it. The adapter is the bridge between the two.

When the agent generates CODE from a buildfile:

1. Read the buildfile's components, elements, and actions (framework-specific widgets)
2. Check if a composition recipe matches the component's surface terms — if so, follow the recipe's state/wiring pattern
3. Follow the conventions for all implementation decisions (state management, naming, data flow, error handling)
4. Write code files following the file-conventions

The buildfile stays small (it describes WHAT). The adapter carries the implementation knowledge (HOW). The testcases verify behavior (CORRECT).

## Ownership model

| Section | Authored by | Customized by | Changes when |
|---|---|---|---|
| Shows/Actions/Flows | Parlay (shipped with adapter template) | Rarely — only if team uses different widgets | Framework version upgrade |
| Compositions | Parlay (ships defaults) | Team (adapts to their patterns) | Team discovers a better pattern |
| Conventions | Parlay (ships defaults) | Team (enforces their standards) | Team standards evolve |
| Design system | Parlay (ships defaults for known frameworks) | Team (marks source per category) | Framework upgrade or Figma integration |
| File conventions | Parlay (ships defaults) | Team (matches their project structure) | Project restructure |
| Patterns | Parlay (ships defaults) | Team (matches their UX preferences) | Design system changes |
| Mount strategies | Parlay (ships defaults for known frameworks) | Team (adapts to their codebase integration patterns) | Team discovers new integration patterns or changes page structure |
| Component vocabulary | Design-system owner (mirrored into the adapter) | Rarely — vocabulary content is intended to be identical across same-design-system adapters | Vocabulary version bump (e.g., clarity@17 → clarity@18) |
| Design tokens | Design-system owner (mirrored into the adapter) | Per-framework emit-forms; the token set itself is closed by vocabulary version | Vocabulary version bump or per-framework emit-form change |
