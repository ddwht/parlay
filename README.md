# Parlay

An intent-driven specification framework that turns user goals into working prototypes through a deterministic pipeline. Describe what users need, not how to code it.

```
intent → dialog → surface → buildfile → code
```

Parlay bridges the gap between design and code generation by providing:

- A **closed interaction vocabulary** (15 Shows, 31 Actions, 10 Flows) that describes UI without naming any framework
- A **framework adapter** that translates that vocabulary into specific widgets (React + Ant Design, Go CLI, Angular + Clarity, etc.)
- An **application blueprint** that captures per-app architectural decisions (shells, routing, auth, data strategy, error handling)
- A **deterministic buildfile** that two independent AI agents must honor identically — the contract is observable behavior, not byte-equivalent code
- **Incremental regeneration** at the component level — stable components are preserved verbatim, only dirty ones are rebuilt

## Quick Start

### Install

```bash
# Homebrew (macOS / Linux)
brew tap ddwht/parlay
brew install parlay

# Shell script (macOS / Linux)
curl -sSfL https://raw.githubusercontent.com/ddwht/parlay/main/install.sh | sh

# Go (if you have Go installed)
go install github.com/ddwht/parlay/cmd/parlay@latest
```

### Bootstrap a Project

```bash
parlay init
```

This is the only CLI command you run directly. It prompts for your AI agent (Claude Code, Cursor, etc.), SDD framework (GitHub SpecKit, Kiro, etc.), and prototype framework (React + Ant Design, Go CLI, etc.). It creates:

```
.parlay/
  config.yaml              # tool configuration
  blueprint.yaml           # app-level architecture (edit this)
  schemas/                 # internal schema definitions
  adapters/                # framework adapter(s)
  build/                   # internal build artifacts (per feature)
spec/
  intents/                 # designer-authored feature inputs
  handoff/                 # engineering handoff output
```

Skills are deployed for your chosen AI agent (e.g., `.claude/skills/parlay-*/` for Claude Code). From this point on, everything happens through skills in your AI agent.

### Add a Feature

```
/parlay-add-feature task list
```

Creates `spec/intents/task-list/` with empty `intents.md` and `dialogs.md`.

### Author Intents

Edit `spec/intents/task-list/intents.md`:

```markdown
# Task List

> A small CLI for capturing and reviewing tasks with priorities.

---

## Add Task

**Goal**: Quickly capture a new task with text and a priority level so it can be reviewed later.
**Persona**: CLI User
**Priority**: P0
**Context**: The user is working in a terminal and wants to quickly record something they need to do.
**Action**: Run `task-list add` with the task text and a priority flag.
**Objects**: task, priority

**Constraints**:
- Adding a task must require text content (empty tasks are rejected)
- Task text must be 200 characters or fewer
- Priority must be one of: high, medium, low
- Default priority is medium when no flag is given
- Tasks are persisted to a local JSON file
- The user must see confirmation including the assigned ID

**Verify**:
- `task-list add "buy milk"` creates a task with priority medium
- `task-list add "ship release" --priority high` creates a high-priority task
- `task-list add ""` rejects with an error
- `task-list add "x" --priority urgent` rejects with unknown-priority error

---

## List Tasks

**Goal**: See all current tasks ordered by priority so the user knows what to work on next.
**Persona**: CLI User
**Priority**: P0
**Context**: The user has added tasks and wants an overview with urgent items at the top.
**Action**: Run `task-list list` to print tasks sorted by priority.
**Objects**: task, priority

**Constraints**:
- Tasks sorted by priority: high first, then medium, then low
- Within same priority, preserve insertion order
- Each line shows task ID, priority, and text
- An empty list shows a friendly message

**Verify**:
- Three tasks (high, medium, low) listed in that order
- Same-priority tasks retain insertion order
- Empty list prints a single message and exits zero
```

Each intent needs only **Goal** and **Persona** — everything else is optional. Write one in under 5 minutes.

### Scaffold Dialogs

```
/parlay-scaffold-dialogs @task-list
```

This generates dialog templates from your intents. Edit them to capture the real user-system conversation:

```markdown
### Add Task

**Trigger**: `task-list add "<text>" [--priority <high|medium|low>]`

User: task-list add "buy milk"
System (background): Loads tasks from local store.
System (background): Validates task text is non-empty and within 200-char limit.
System (background): Assigns the next available task ID and uses default priority (medium).
System (background): Appends the task to the store and persists.
System: [OK] Task #3 added (medium): buy milk

#### Branch: Empty Text

User: task-list add ""
System: [ERR] Task text cannot be empty.

#### Branch: Unknown Priority

User: task-list add "x" --priority urgent
System: [ERR] Unknown priority "urgent". Use one of: high, medium, low.

---

### List Tasks

**Trigger**: `task-list list`

User: task-list list
System (background): Loads tasks from local store.
System (background): Sorts tasks by priority (high, medium, low), preserving insertion order within each bucket.
System (condition: empty list): Nothing to do! Add a task with `task-list add "<text>"`.
System (condition: tasks exist): Tasks:
System:   #2 [high  ] ship release
System:   #1 [medium] buy milk
System:   #3 [low   ] read book
```

### Generate Surface

Use the AI skill:

```
/parlay-create-surface @task-list
```

This reads your intents and dialogs, resolves ambiguities conversationally, and generates `surface.md` — a framework-agnostic description of what the UI shows and what the user can do:

```markdown
## Add Task Command

**Shows**: status, message
**Actions**: provide-text, provide-value, invoke
**Source**: @task-list/add-task
**Page**: cli
**Region**: command
**Order**: 1

**Notes**:
- Errors (empty text, too long, unknown priority) use status with [ERR] prefix
- Success uses status with [OK] prefix

---

## List Tasks Command

**Shows**: data-list, empty-state, message
**Actions**: invoke
**Source**: @task-list/list-tasks
**Page**: cli
**Region**: command
**Order**: 2

**Notes**:
- data-list renders sorted tasks with ID, priority label, and text
- empty-state shows a friendly message when no tasks exist
```

The vocabulary is closed — `data-list`, `status`, `provide-text`, `empty-state`, `invoke` are defined terms, not free text.

### Define the Blueprint

Edit `.parlay/blueprint.yaml` to describe how your app is wired together. For a CLI app this is minimal:

```yaml
app: task-list

navigation:
  strategy: cli-subcommands
```

For a web app, the blueprint captures layout shells, routing, auth, and more:

```yaml
app: project-tracker

shells:
  main:
    description: Sidebar navigation with header
    chrome:
      - region: sidebar
        widget: Sider
        content: primary navigation menu
      - region: header
        widget: Header
        content: breadcrumbs, user menu
    wraps: [dashboard, tasks, settings]
  public:
    description: Centered layout for login
    chrome: []
    wraps: [login]

navigation:
  strategy: browser
  default-route: /dashboard
  routes:
    - path: /dashboard
      shell: main
      guard: require-auth
    - path: /tasks
      shell: main
      guard: require-auth
      lazy: true
    - path: /settings
      shell: main
      guard: require-auth
    - path: /login
      shell: public
  not-found: render-404

authorization:
  strategy: role-based
  roles:
    - name: user
      description: Standard authenticated user
    - name: admin
      description: Can manage users and settings
  guards:
    require-auth:
      requires: user
      redirect: /login

data:
  fetching: stale-while-revalidate
  caching:
    strategy: in-memory
    invalidation:
      - trigger: mutation on Task
        scope: task-list, dashboard-metrics

errors:
  boundaries:
    - scope: app
      fallback: error page
    - scope: route
      fallback: inline retry
  http:
    "401": redirect:/login
    "5xx": error-boundary with retry

state:
  global:
    - name: currentUser
      type: User
      source: auth-flow
  propagation: context
```

Every section is optional. The blueprint is validated automatically during `/parlay-build-feature` and `/parlay-generate-code`.

### Build the Feature Specification

Use the AI skill:

```
/parlay-build-feature @task-list
```

This loads your intents, dialogs, surface, adapter, and blueprint, then generates:

- `.parlay/build/task-list/buildfile.yaml` — deterministic intermediate representation with framework-specific widgets
- `.parlay/build/task-list/testcases.yaml` — property-based tests

The buildfile is tool-internal — you never edit it. It's the contract between design and code.

### Generate Prototype Code

```
/parlay-generate-code
```

This operates at the project level (all features). It:

1. Reads all buildfiles + adapter + blueprint (never reads `spec/intents/`)
2. Merges models and routes across features
3. Generates shell components from the blueprint
4. Generates per-feature component code
5. Runs tests — must pass before state is committed
6. Saves build state atomically

On subsequent runs, only dirty components are regenerated. Stable components are preserved verbatim.

### Hand Off to Engineering

```
/parlay-generate-enggspec @task-list
```

Generates `spec/handoff/task-list/specification.md` in your configured SDD format (GitHub SpecKit, Kiro, etc.).

## Project Layout

```
spec/
  intents/                    Designer-authored (per feature)
    <feature>/
      intents.md                Human-authored goals
      dialogs.md                Human-authored conversations
      surface.md                Generated, human-reviewed UI fragments
      domain-model.md           Generated, human-reviewed entities
  handoff/                    Engineering output (per feature)
    <feature>/
      specification.md          Generated engineering spec
  pages/                      Optional cross-feature page manifests
    <page>.page.md

.parlay/                      Tool internals (never user-facing)
  config.yaml                   Project configuration
  blueprint.yaml                Application architecture
  schemas/                      Schema definitions
  adapters/                     Framework adapters
  build/
    <feature>/
      buildfile.yaml              Deterministic build spec
      testcases.yaml              Property-based tests
      .baseline.yaml              Source hash baseline
    _project/
      .baseline.yaml              Merged section hashes
      .code-hashes.yaml           Generated file hashes
```

Three zones with strict ownership:

| Zone | Audience | Rule |
|---|---|---|
| `spec/intents/` | Designer authors and reviews | Source of truth for design |
| `spec/handoff/` | Engineering consumes | Generated, human-reviewed before handoff |
| `.parlay/` | Tool only | Never user-facing; never edit manually |

## Schemas

Parlay has 9 schemas defining every artifact:

| Schema | What it defines |
|---|---|
| `intent.schema.md` | Goal, Persona, Priority, Context, Action, Objects, Constraints, Verify |
| `dialog.schema.md` | User/System/Background/Conditional turns, Options, Branches |
| `surface.schema.md` | 15 Shows, 31 Actions, 10 Flows — closed interaction vocabulary |
| `adapter.schema.md` | Widget mappings, compositions, conventions, file conventions, patterns |
| `blueprint.schema.md` | Shells, navigation, authorization, data, errors, state, platform |
| `buildfile.schema.md` | Models, fixtures, routes, components (elements, actions, operations) |
| `testcases.schema.md` | Suites, cases, steps (render/click/input/select), 15 verification types |
| `page.schema.md` | Cross-feature page manifests with region ordering |
| `feature-structure.schema.md` | Three-zone project layout and ownership rules |

Schemas are loaded on-demand by the AI agent per command — never kept in agent context permanently.

## Skills Reference

After `parlay init`, all workflow operations happen through AI agent skills. The skills load schemas on-demand, call the `parlay` helper binary for parsing and validation, and handle ambiguity resolution conversationally.

| Skill | Description |
|---|---|
| `/parlay-add-feature <name>` | Create a feature folder with intents.md and dialogs.md |
| `/parlay-scaffold-dialogs @<feature>` | Generate dialog templates from authored intents |
| `/parlay-create-surface @<feature>` | Generate surface.md with ambiguity resolution |
| `/parlay-build-feature @<feature>` | Generate buildfile.yaml + testcases.yaml |
| `/parlay-generate-code` | Generate prototype code (project-level, all features) |
| `/parlay-generate-enggspec @<feature>` | Generate engineering specification for handoff |
| `/parlay-extract-domain-model` | Extract domain model from all features |
| `/parlay-load-domain-model <path>` | Load and merge external domain model |
| `/parlay-sync @<feature>` | Check coverage, traceability, and drift |
| `/parlay-collect-questions @<feature>` | Collect unresolved design questions |
| `/parlay-register-adapter <path>` | Register a framework adapter |
| `/parlay-view-page <page>` | Assemble cross-feature page view |
| `/parlay-lock-page <page>` | Lock page layout into a manifest |

## CLI Reference

The `parlay` binary is a helper that skills call internally for parsing, validation, diffing, and state management. You only run `parlay init` directly — everything else is invoked by the agent through skills.

| Command | Description |
|---|---|
| `parlay init` | Bootstrap a new project (the only user-facing CLI command) |
| `parlay add-feature <name>` | Create a feature folder |
| `parlay create-dialogs @<feature>` | Scaffold dialog templates |
| `parlay create-surface @<feature>` | Generate surface (basic mode) |
| `parlay register-adapter <path>` | Register a framework adapter |
| `parlay view-page <page>` | Assemble cross-feature page view |
| `parlay lock-page <page>` | Lock page layout into a manifest |
| `parlay validate --type <type> <path>` | Validate file (surface, buildfile, blueprint, yaml) |
| `parlay parse --type <type> <path>` | Parse file to JSON (intents, dialogs, surface) |
| `parlay check-coverage @<feature>` | Check intent-dialog coverage |
| `parlay check-readiness @<feature> --stage <stage>` | Pre-flight checks before generation |
| `parlay diff [@<feature>]` | Show what changed since last build |
| `parlay scan-generated <source-root>` | Map generated files to their owners |
| `parlay verify-generated [@<feature>]` | Detect hand-edits to generated files |
| `parlay save-build-state --source-root <root>` | Atomically commit build state |

## Adapters

The adapter is the bridge between framework-agnostic surface vocabulary and framework-specific code. Parlay ships templates; teams customize to match their codebase.

### What an Adapter Contains

| Section | Purpose | Customized by |
|---|---|---|
| `shows:` | Maps 15 Show types to framework widgets | Rarely (framework upgrade) |
| `actions:` | Maps 31 Action types to framework widgets | Rarely |
| `flows:` | Maps 10 Flow patterns to composite patterns | Rarely |
| `compositions:` | Runtime recipes (e.g., crud-table-with-drawer) | Team |
| `conventions:` | Structured rules (state management, naming, error handling) | Team |
| `file-conventions:` | Source root, component pattern, naming, entry point | Team |
| `patterns:` | Interaction style, info density, error placement | Team |

### Bundled Adapters

- **go-cli** — Go + Cobra CLI (commands, prompts, tabwriter tables)
- **react-antd** — React + Ant Design (Table, Modal, Form, Steps, etc.)
- **angular-material** — Angular + Material (MatTable, MatDialog, MatStepper, M3 tokens)

### Creating an Adapter

```yaml
name: react-antd
framework: React + Ant Design
version: "1.0"

shows:
  data-value:
    widget: Statistic
    description: Single metric display
  data-list:
    widget: List
    description: Vertical list with List.Item
  data-table:
    widget: Table
    description: Ant Design Table with sorting, filtering, pagination
  # ... all 15 Show types

actions:
  provide-text:
    widget: Input
    description: Single-line text input
  confirm:
    widget: Modal.confirm
    description: Confirmation dialog
  invoke-destructive:
    widget: Button.danger+Popconfirm
    description: Danger button with confirmation
    requires-confirmation: true
  # ... all 31 Action types

flows:
  crud-collection:
    pattern: Table+Modal+Form+Popconfirm
    description: CRUD table with modal form for create/edit
    regions: [toolbar, main, modal]
  # ... all 10 Flow types

compositions:
  crud-table-with-drawer:
    trigger: "component has data-table + navigate-drill + inspect"
    state: [selectedItem, drawerOpen]
    wiring: "row-click sets selectedItem and opens drawer"

conventions:
  state-management:
    rule: "useState for local, React Context for shared. No Redux."
    applies-to: all components

file-conventions:
  source-root: "src/"
  component-pattern: feature-modules
  naming: PascalCase
  entry-point: "src/App.tsx"
```

Register with:

```
/parlay-register-adapter ./adapters/react-antd.adapter.yaml
```

## Blueprint

The blueprint captures per-app architectural decisions that are too app-specific for the adapter and too cross-cutting for any single feature. It lives at `.parlay/blueprint.yaml`.

| Section | What it describes |
|---|---|
| `shells` | Layout hierarchy — which routes share which chrome |
| `navigation` | Route tree, strategy, guards, lazy loading, deep links |
| `authorization` | Roles, guards, resource-level policies |
| `data` | Fetching strategy, caching, invalidation graph, offline support |
| `errors` | Error boundary scopes, HTTP error handling, retry strategy |
| `state` | Global state slices, propagation pattern, URL-driven state |
| `platform` | Native only: push notifications, background tasks, widgets |

Every section is optional. Cross-reference checks (shell refs, guard refs, duplicate routes, strategy validation) run automatically when skills load the blueprint.

## Agent Support

Parlay is agent-agnostic. Skills are plain markdown instructions any AI can follow. Agent-specific deployers package them into the right format:

| Agent | Deployer | Skills Location | Config File |
|---|---|---|---|
| Claude Code | ClaudeDeployer | `.claude/skills/parlay-*/SKILL.md` | `CLAUDE.md` |
| Cursor | CursorDeployer | `.cursor/rules/parlay-*.mdc` | `.cursor/rules/parlay-project.mdc` |
| Other | GenericDeployer | `AGENT_INSTRUCTIONS.md` | — |

Adding a new agent requires only a new deployer — zero changes to skills or schemas.

## Key Design Principles

**Intent-first**: User goals are the atomic unit. Everything derives from them.

**Closed vocabulary**: The surface interaction vocabulary (Shows, Actions, Flows) is a finite, defined set — not free text. This makes adapter mapping exhaustive and validatable.

**Deterministic contract**: Same buildfile + adapter + blueprint → same testcases pass. The contract is observable behavior, not code structure. Two agents have latitude on naming and style.

**Strict codegen boundary**: Code generation reads only from `.parlay/` (buildfile, adapter, blueprint). It never reads `spec/intents/`. If it needs to, the buildfile schema is leaking detail.

**Three-zone ownership**: Designer-authored input, engineering handoff, and tool internals are strictly separated. Cross-zone writes are errors.

**Incremental by default**: Stable components are preserved verbatim. Only dirty components (with changed upstream sources) are regenerated. Hand-edits to stable files are detected and surfaced.

**Agent-agnostic**: Skills are plain English markdown. The helper binary handles parsing, validation, and state management. Adding a new AI agent requires only a deployer, not skill rewrites.
