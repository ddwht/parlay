# Framework Adapter Schema

File: `.parlay/adapters/<adapter-name>.adapter.yaml`
Registered via `/parlay register-adapter` or bundled during `/parlay init`.

A framework adapter maps the surface interaction vocabulary (Shows, Actions, Flows) to framework-specific widgets, patterns, and conventions. It teaches the tool how to generate buildfiles and code for a specific prototype framework.

The adapter is a **pure translation layer**: it maps generic user interactions to framework-specific implementations. It has no knowledge of the project's codebase, features, or domain. It answers one question: "how does this framework implement this interaction?"

## Structure

```yaml
name: <adapter name — e.g., go-cli, angular-clarity, react-mui, ios-uikit>
framework: <human-readable framework name — e.g., "Go CLI", "Angular + Clarity">
version: <adapter version>

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

file-conventions:
  source-root: <where generated code lives — e.g., "src/", "cmd/", "app/">
  component-pattern: <how components map to files — e.g., "one-file-per-component", "feature-modules">
  naming: <file naming convention — e.g., "kebab-case", "snake_case", "PascalCase">
  entry-point: <main file — e.g., "main.go", "main.ts", "App.tsx">

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

## Mapping rules

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

## Validation

When an adapter file is loaded, the tool verifies:
- Every Show type from the surface vocabulary has an entry in `shows:`
- Every Action type from the surface vocabulary has an entry in `actions:`
- Every Flow type from the surface vocabulary has an entry in `flows:`
- Missing entries are errors — the adapter must be comprehensive
- `widget: not-applicable` is allowed (with description explaining why)
- `requires: custom-implementation` is allowed (the agent is expected to write the implementation from scratch)
- The `file-conventions` section is complete

## Relationship to buildfile

The buildfile references widget names from the adapter, not surface vocabulary terms. When the agent generates a buildfile from a surface + adapter:

1. Read the surface fragment's Shows/Actions/Flow
2. Look up each term in the adapter to get the framework-specific widget
3. Write the widget name into the buildfile

The buildfile is fully framework-specific. The surface vocabulary does not appear in it. The adapter is the bridge between the two.
