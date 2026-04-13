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
```

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

When `source: framework`, the agent uses the framework's token system and never hardcodes values. When `source: figma`, the agent reads values from the per-fragment design-spec. When `source: not-defined`, the agent uses its judgment — the design-spec may provide values later, or the agent picks reasonable defaults.

Teams can add custom categories beyond the standard set (e.g., `z-index`, `breakpoints`, `opacity`).

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
