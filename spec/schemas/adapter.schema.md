# Framework Adapter Schema

File: `.parlay/adapters/<adapter-name>.adapter.yaml`
Registered via `/parlay register-adapter` or bundled during `/parlay init`.

A framework adapter maps abstract buildfile types to framework-specific widgets, patterns, and conventions. It teaches the tool how to generate buildfiles and code for a specific prototype framework.

## Structure

```yaml
name: <adapter name — e.g., go-cli, angular-clarity, react-mui>
framework: <human-readable framework name — e.g., "Angular + Clarity">
version: <adapter version>

component-types:
  <abstract-type>:
    widget: <framework-specific widget>
    description: <how this maps in this framework>
    import: <framework import path if applicable>

element-types:
  <abstract-type>:
    widget: <framework-specific widget>
    description: <how this renders in this framework>

action-types:
  <abstract-type>:
    widget: <framework-specific widget>
    description: <how this interaction works in this framework>

layout-patterns:
  <pattern-name>:
    description: <what this layout looks like>
    regions: [<region names this pattern provides>]
    implementation: <framework-specific approach>

file-conventions:
  source-root: <where generated code lives — e.g., "src/", "cmd/", "app/">
  component-pattern: <how components map to files — e.g., "one-file-per-component", "feature-modules">
  naming: <file naming convention — e.g., "kebab-case", "PascalCase">
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
    required-for: [<actions that need confirmation>]
    style: <prompt | dialog | inline>
  content:
    timestamps: <relative | absolute | both>
    empty-states: <message | hidden | placeholder>
```

The `patterns` section guides the agent's design decisions when generating components. Different frameworks have different idioms — a CLI prefers sequential prompts and concise output; a web app prefers progressive disclosure and visual feedback. By declaring patterns in the adapter, the agent makes consistent decisions across all features built with the same adapter.

## Example: go-cli adapter

```yaml
name: go-cli
framework: "Go CLI"
version: "1.0"

component-types:
  interactive-wizard:
    widget: sequential-prompts
    description: Series of bufio or survey prompts in sequence
  command-output:
    widget: cobra-command
    description: Cobra command with fmt.Print output
  interactive-prompt:
    widget: survey-prompt
    description: Single interactive prompt using survey or bufio
  data-display:
    widget: fmt-output
    description: Formatted terminal output
  report:
    widget: sectioned-output
    description: Multiple labeled sections printed to terminal

element-types:
  text-output:
    widget: fmt.Println
    description: Single line printed to stdout
  data-list:
    widget: bulleted-list
    description: "  - item" formatted lines
  data-table:
    widget: tabwriter
    description: Aligned columns using tabwriter
  status-indicator:
    widget: prefixed-message
    description: "[OK]", "[WARN]", "[ERR]" prefixed messages
  path-reference:
    widget: fmt.Println
    description: File path printed to stdout
  grouped-output:
    widget: headed-section
    description: "**heading**:" followed by indented items
  code-block:
    widget: indented-block
    description: Indented monospace output

action-types:
  selection:
    widget: select-prompt
    description: Lettered options (A/B/C) with bufio readline
  confirmation:
    widget: confirm-prompt
    description: Y/N prompt with bufio readline
  text-input:
    widget: text-prompt
    description: Free-form input with bufio readline
  navigation:
    widget: not-applicable
    description: CLI tools don't navigate — commands are entry points
  file-operation:
    widget: os-file-ops
    description: os.MkdirAll, os.WriteFile, os.ReadFile
  state-mutation:
    widget: in-memory-update
    description: Modify struct fields in memory

layout-patterns:
  sequential-prompts:
    description: One prompt after another, building up state
    regions: [prompts, confirmation]
    implementation: Sequential function calls with accumulated config
  report-output:
    description: Headed sections with lists and counts
    regions: [header, sections, footer]
    implementation: Print sections to stdout in order
  interactive-flow:
    description: Output followed by a prompt followed by more output
    regions: [display, prompt, result]
    implementation: Print, read input, print result

file-conventions:
  source-root: "cmd/parlay/"
  component-pattern: one-file-per-command
  naming: snake_case
  entry-point: "cmd/parlay/main.go"
  internal-packages: "internal/"

patterns:
  interaction:
    prefer: [sequential-prompts, lettered-options, immediate-feedback]
    avoid: [modal-dialogs, hover-interactions, drag-and-drop]
  information-density:
    default: high
    rationale: CLI users expect concise, scannable output without scrolling
  error-placement:
    default: inline
    rationale: Errors print immediately after the failing action; no separate error region
  confirmation:
    required-for: [destructive-operations, irreversible-changes]
    style: prompt
  content:
    timestamps: absolute
    empty-states: message
```

## Adapter validation

When an adapter is loaded, the tool verifies:
- All abstract component types from the buildfile schema have mappings
- All abstract element types have mappings
- All abstract action types have mappings
- At least one layout pattern is defined
- The patterns section is present (warning if missing — patterns are strongly recommended for consistent design decisions)

Missing mappings are warnings (the adapter may not support every type), not errors.

## Parsing

- YAML structure
- Type mappings: `<abstract-type>` keys under component-types, element-types, action-types
- Layout patterns: named patterns with region lists
- File conventions: string values for source-root, naming, entry-point
