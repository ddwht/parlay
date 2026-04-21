# Parlay

An intent-driven specification framework that turns user goals into working prototypes through a structured pipeline. Describe what users need, not how to code it.

```
intent → dialog → artifacts (surface and/or infrastructure) → [design-spec] → buildfile → code
                                                                   ↑ optional
                                                                (from Figma)
```

Parlay bridges the gap between design and code generation by providing:

- A **closed interaction vocabulary** (Shows, Actions, Flows) that describes UI without naming any framework
- A **framework adapter** that translates that vocabulary into specific widgets (React + Ant Design, Go CLI, Angular + Clarity, etc.)
- **Infrastructure fragments** for behind-the-scenes changes (helpers, resolvers, cross-cutting patterns) that coexist with user-facing surfaces
- An **application blueprint** that captures per-app architectural decisions (shells, routing, auth, data strategy, error handling)
- A **structured buildfile** that agents read at codegen time rather than re-inferring everything from design — the goal is repeatable, behavior-level output, not byte-identical code
- **Initiatives** for grouping related features and treating them as a cohesive unit
- **Incremental regeneration** at the component level — stable component files are preserved verbatim; cross-cutting entries and brownfield merges are re-applied on each run
- An **end-to-end orchestrator** (`/parlay-loop`) that walks a feature through the whole pipeline with phase-group subagents and confirmation checkpoints

## Quick Start

### Install

```bash
# Homebrew (macOS / Linux)
brew install ddwht/parlay/parlay

# Shell script (macOS / Linux)
curl -sSfL https://raw.githubusercontent.com/ddwht/parlay/main/install.sh | sh

# Go (if you have Go installed)
go install github.com/ddwht/parlay/cmd/parlay@latest
```

### Bootstrap a Project

```bash
parlay init
```

`parlay init` and `parlay upgrade` are the main CLI commands humans run directly (init for a new project, upgrade after the binary version changes). A few repair/inspection utilities are also human-facing; everything else is invoked by skills. The init prompt asks for your AI agent (Claude Code, Cursor, or Generic), SDD framework (GitHub SpecKit, Kiro, etc.), and prototype framework (React + Ant Design, Go CLI, etc.). It creates:

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

Skills AND subagents are deployed for your chosen AI agent:
- Claude Code: `.claude/skills/parlay-*/` + `.claude/agents/parlay-*.md`
- Cursor: `.cursor/skills/parlay-*/` + `.cursor/agents/parlay-*.md`
- Generic: `AGENT_INSTRUCTIONS.md` (a single concatenated file)

From this point on, everything happens through skills in your AI agent.

### Already have a project? Onboard instead of init

```
/parlay-onboard
```

Analyzes an existing codebase, drafts an adapter that matches its conventions, and plugs parlay in without requiring a greenfield rewrite.

### The end-to-end path: `/parlay-loop`

If you want to walk a new feature through the whole pipeline in one continuous session:

```
/parlay-loop task-list
```

`parlay-loop` orchestrates every downstream skill — intents → dialogs → artifacts → build → code — with mandatory confirmations at each phase boundary, gap analysis at the intents and dialogs stages, and a fresh phase-group subagent between the designer, build, and code phases. Pass `--from <phase>` to start mid-pipeline after editing an upstream artifact by hand. See the [skill cheat sheet](#skills-reference) for the individual skills you can also invoke manually.

### Add a Feature

```
/parlay-add-feature task list
```

Creates `spec/intents/task-list/` with empty `intents.md` and `dialogs.md`. Pass `--initiative <name>` to nest the new feature inside an initiative:

```
/parlay-add-feature reset password --initiative auth-overhaul
```

creates `spec/intents/auth-overhaul/reset-password/`.

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
```

Each intent needs only **Goal** and **Persona** — everything else is optional. Write one in under 5 minutes.

### Scaffold Dialogs

```
/parlay-scaffold-dialogs @task-list
```

This generates complete dialog flows from your intents — triggers, happy paths with user/system turns, and branches for each constraint or verify edge case. The agent asks about any ambiguities in your intents before generating, then presents the result for review. On re-runs against intents that have changed, it proposes complete update diffs for meaningful changes (new constraints → new branches, new verify items → new flows) and skips cosmetic rewrites.

### Decide and Create Artifacts

```
/parlay-create-artifacts @task-list
```

Parlay classifies each intent as **surface** (user-visible output), **infrastructure** (internal code changes with no visible output), or **ambiguous**, and creates the right artifacts:

- **surface.md** — framework-agnostic UI fragments (Shows, Actions, Flows)
- **infrastructure.md** — cross-cutting code changes (affected scope, behavior, invariants)
- **both** when a feature has visible output plus plumbing

You review the per-intent classification before any file is written. Generated surfaces look like:

```markdown
## Add Task Command

**Shows**: status, message
**Actions**: provide-text, provide-value, invoke
**Source**: @task-list/add-task
**Page**: cli
**Region**: command
**Order**: 1

**Notes**:
- Errors use status with [ERR] prefix
- Success uses status with [OK] prefix
```

The surface vocabulary is closed — `data-list`, `status`, `provide-text`, `empty-state`, `invoke` are defined terms, not free text.

Generated infrastructure fragments look like:

```markdown
## Duplicate-Slug Detection

**Affects**: path resolution, feature enumeration
**Behavior**: Add a defensive check during path resolution and feature enumeration that detects when two or more directories under the same parent slugify to the same identifier. When a collision is detected, fail loudly rather than silently picking one.
**Invariants**:
- Duplicate-slug errors list all conflicting paths.
- The check is per-parent-directory, not whole-tree.
**Source**: @initiatives/group-features-under-an-initiative
**Backward-Compatible**: yes
```

### Enrich with Figma (Optional)

If a Figma design exists for the feature, enrich the surface with visual details:

```
/parlay-reference-design-spec @task-list <figma-link>
```

This reads Figma via MCP, maps Figma components to surface fragments, and generates `.parlay/build/task-list/design-spec.yaml` with per-fragment widget specifics, layout, tokens, variants, spacing, and colors. Build-feature uses it automatically when present.

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
    - path: /login
      shell: public
  not-found: render-404

authorization:
  strategy: role-based
  roles:
    - name: user
    - name: admin
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
```

Every section is optional. Cross-reference checks (shell refs, guard refs, duplicate routes, strategy validation) run automatically when skills load the blueprint.

### Build the Feature Specification

```
/parlay-build-feature @task-list
```

Loads your intents, dialogs, surface, infrastructure (if present), adapter, and blueprint, then generates:

- `.parlay/build/task-list/buildfile.yaml` — structured intermediate representation with framework-specific widgets. Surface fragments become `components:`; infrastructure fragments become `cross-cutting:`.
- `.parlay/build/task-list/testcases.yaml` — verification tests (element-visibility and action-enabled assertions)

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
5. Processes `cross-cutting:` entries — modifies existing files via Tier 2 intelligent merge, creates new ones as needed
6. Mounts components into existing pages where applicable (brownfield integration)
7. Runs tests — must pass before state is committed
8. Saves build state atomically via `parlay save-build-state`

On subsequent runs, only dirty components (and cross-cutting entries) are regenerated. Stable component files are preserved byte-for-byte; cross-cutting merges into shared files are re-applied on each run. Hand-edits to any tracked file are detected and surfaced before overwriting.

### Hand Off to Engineering

```
/parlay-generate-enggspec @task-list
```

Generates `spec/handoff/task-list/specification.md` in your configured SDD format (GitHub SpecKit, Kiro, etc.).

## Initiatives

An initiative groups related features and gives them a shared folder. Example layout:

```
spec/intents/auth-overhaul/
  login/
    intents.md
    dialogs.md
  reset-password/
    intents.md
    dialogs.md
  session-management/
    intents.md
```

Create one with:

```
/parlay-new-initiative auth overhaul
```

and then add features inside it:

```
/parlay-add-feature login --initiative auth-overhaul
```

Features inside initiatives are referenced as `@auth-overhaul/login` throughout parlay skills and CLI commands. An initiative is a folder that contains feature folders — no more, no less. Tools that walk the three parallel trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`) keep initiatives in lockstep; `/parlay-repair` reconciles if they drift.

Move a feature between initiatives (or in/out of orphan state) with `parlay move-feature`:

```
parlay move-feature @login --to auth-overhaul
parlay move-feature @auth-overhaul/login --out
```

## Project Layout

```
spec/
  intents/                    Designer-authored (per feature)
    <feature>/                  (or) <initiative>/<feature>/
      intents.md                Human-authored goals
      dialogs.md                Human-authored conversations
      surface.md                Generated, human-reviewed UI fragments
      infrastructure.md         Generated, human-reviewed cross-cutting changes
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
    <feature>/                  (or) <initiative>/<feature>/
      buildfile.yaml              Structured build spec
      testcases.yaml              Verification tests
      design-spec.yaml            Visual details from Figma (optional)
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

Parlay has 11 schemas defining every artifact:

| Schema | What it defines |
|---|---|
| `intent.schema.md` | Goal, Persona, Priority, Context, Action, Objects, Constraints, Verify, Questions |
| `dialog.schema.md` | User/System/Background/Conditional turns, Options, Branches |
| `surface.schema.md` | Shows, Actions, Flows — closed interaction vocabulary |
| `infrastructure.schema.md` | Affects, Behavior, Invariants, Source — framework-agnostic cross-cutting changes |
| `adapter.schema.md` | Widget mappings, design system, compositions, conventions, patterns |
| `blueprint.schema.md` | Shells, navigation, authorization, data, errors, state, platform |
| `design-spec.schema.md` | Per-fragment visual details from Figma (widget, layout, tokens, variants) |
| `buildfile.schema.md` | Models, fixtures, routes, components, cross-cutting entries |
| `testcases.schema.md` | Suites, cases, steps, verification types |
| `page.schema.md` | Cross-feature page manifests with region ordering |
| `feature-structure.schema.md` | Three-zone project layout, initiative nesting, ownership rules |

Schemas are loaded on-demand by the AI agent per command — never kept in agent context permanently.

## Skills Reference

After `parlay init`, all workflow operations happen through AI agent skills. The skills load schemas on-demand, call the `parlay` helper binary for parsing and validation, and handle ambiguity resolution conversationally.

| Skill | Description |
|---|---|
| `/parlay-loop <feature> [--from <phase>]` | Walk a feature end-to-end through intents → dialogs → artifacts → build → code with phase-group subagents and confirmations |
| `/parlay-add-feature <name> [--initiative <name>]` | Create a feature folder with intents.md and dialogs.md |
| `/parlay-new-initiative <name>` | Create an empty initiative directory across the three trees |
| `/parlay-onboard` | Onboard an existing codebase and draft a matching adapter |
| `/parlay-scaffold-dialogs @<feature>` | Generate complete dialog flows from authored intents |
| `/parlay-create-artifacts @<feature>` | Decide between surface, infrastructure, or both — then generate |
| `/parlay-reference-design-spec @<feature> <figma-link>` | Extract visual details from Figma into design-spec.yaml |
| `/parlay-build-feature @<feature>` | Generate buildfile.yaml + testcases.yaml |
| `/parlay-generate-code` | Generate prototype code (project-level, all features) |
| `/parlay-generate-enggspec @<feature>` | Generate engineering specification for handoff |
| `/parlay-extract-domain-model` | Extract domain model from all features |
| `/parlay-load-domain-model <path>` | Load and integrate an external domain model |
| `/parlay-sync @<feature>` | Check coverage, traceability, and drift |
| `/parlay-collect-questions @<feature>` | Collect unresolved design questions |
| `/parlay-repair` | Validate and reconcile the three parallel trees (spec/intents, spec/handoff, .parlay/build) |
| `/parlay-register-adapter <path>` | Register a framework adapter |
| `/parlay-view-page <page>` | Assemble cross-feature page view |
| `/parlay-lock-page <page>` | Lock page layout into a manifest |

## CLI Reference

The `parlay` binary is a helper that skills call internally for parsing, validation, diffing, and state management. You only run `parlay init` (and a handful of bookkeeping commands) directly — the rest are invoked by the agent through skills.

| Command | Description |
|---|---|
| `parlay init` | Bootstrap a new project (user-facing) |
| `parlay upgrade` | Re-deploy schemas, skills, subagents, and agent config from the binary (user-facing) |
| `parlay loop <@feature> [--from <phase>]` | Skill-pointer for the end-to-end orchestrator |
| `parlay add-feature <name> [--initiative <name>]` | Create a feature folder |
| `parlay new-initiative <name>` | Create an empty initiative directory |
| `parlay move-feature @<feature> --to <initiative> \| --out` | Move a feature between initiatives |
| `parlay create-dialogs @<feature>` | Scaffold dialog templates |
| `parlay create-artifacts @<feature>` | Skill-pointer for artifact decision |
| `parlay build-feature @<feature>` | Skill-pointer for buildfile generation |
| `parlay generate-code` | Skill-pointer for code generation |
| `parlay generate-enggspec @<feature>` | Skill-pointer for handoff spec |
| `parlay extract-domain-model` | Skill-pointer for domain-model extraction |
| `parlay load-domain-model <path>` | Skill-pointer for external domain-model load |
| `parlay repair [--dry-run] [--yes]` | Validate and reconcile the three parallel trees |
| `parlay simplify` | Detect duplicated helpers across generated files and propose extractions |
| `parlay register-adapter <path>` | Register a framework adapter |
| `parlay view-page <page>` | Assemble cross-feature page view |
| `parlay lock-page <page>` | Lock page layout into a manifest |
| `parlay validate --type <type> <path>` | Validate file against its schema |
| `parlay parse --type <type> <path>` | Parse file to JSON |
| `parlay check-coverage @<feature>` | Check intent-dialog coverage (JSON) |
| `parlay check-drift @<feature>` | Check if intents changed since last build (JSON) |
| `parlay check-readiness @<feature> --stage <stage>` | Pre-flight checks before generation (JSON) |
| `parlay collect-questions @<feature>` | Collect open questions from intents (JSON) |
| `parlay diff [@<feature>]` | Show what changed since last build (JSON) |
| `parlay scan-generated <source-root>` | Map generated files to their owners (JSON) |
| `parlay verify-generated [@<feature>]` | Detect hand-edits to generated files (JSON) |
| `parlay save-build-state --source-root <root>` | Atomically commit build state (called by generate-code) |

JSON-output commands are for agent consumption; human-facing commands print plain text.

## Adapters

The adapter is the bridge between framework-agnostic surface vocabulary and framework-specific code. Parlay ships four starter adapters; teams are expected to customize them to match their codebase conventions.

### What an Adapter Contains

| Section | Purpose | Customized by |
|---|---|---|
| `shows:` | Maps each Show type to framework widgets | Rarely (framework upgrade) |
| `actions:` | Maps each Action type to framework widgets | Rarely |
| `flows:` | Maps Flow patterns to composite patterns | Rarely |
| `design-system:` | Inventories design tokens per category (colors, spacing, typography, etc.) with `source: framework / figma / not-defined` | Team |
| `compositions:` | Runtime recipes (e.g., crud-table-with-drawer) | Team |
| `conventions:` | Structured rules (state management, naming, error handling) | Team |
| `file-conventions:` | Source root, component pattern, naming, entry point | Team |
| `mount-strategies:` | Templates for mounting into existing files (brownfield) | Team |
| `patterns:` | Interaction style, info density, error placement | Team |
| `agent:` | How the same vocabulary renders in an AI agent session | Rarely |

### Bundled Adapters

- **go-cli** — Go + Cobra CLI (commands, prompts, tabwriter tables)
- **react-antd** — React + Ant Design (Table, Modal, Form, Steps, etc.)
- **angular-material** — Angular + Material (MatTable, MatDialog, MatStepper, M3 tokens)
- **angular-clarity** — Angular + Clarity (ClrDatagrid, ClrWizard, ClrSidePanel, CDS tokens)

### Creating an Adapter

Use `/parlay-register-adapter <path>` to plug in a custom adapter. Onboarding an existing codebase? `/parlay-onboard` drafts one by analyzing your source tree and conventions.

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
| `deployers` | Internal: which project-owned systems cross multiple features |

Every section is optional. Cross-reference checks (shell refs, guard refs, duplicate routes, strategy validation) run automatically when skills load the blueprint.

## Agent Support

Parlay is designed to be portable across AI agents. Skills are plain markdown instructions; they expect the host agent to support slash-command dispatch and interactive prompting (AskUserQuestion or an equivalent). Agent-specific deployers package them into the right on-disk layout alongside any subagent definitions.

| Agent | Deployer | Skills Location | Subagents Location | Project Config |
|---|---|---|---|---|
| Claude Code | `ClaudeDeployer` | `.claude/skills/parlay-*/SKILL.md` | `.claude/agents/parlay-*.md` | `CLAUDE.md` (marker-preserved region) |
| Cursor | `CursorDeployer` | `.cursor/skills/parlay-*/SKILL.md` | `.cursor/agents/parlay-*.md` | `.cursor/rules/parlay.mdc` (always-apply) |
| Generic | `GenericDeployer` | `AGENT_INSTRUCTIONS.md` (concatenated) | Embedded in the same file | — |

Adding a new agent requires only a new deployer — zero changes to skills, subagents, or schemas.

Subagents (`parlay-designer`, `parlay-build`, `parlay-code`) are pre-defined phase-group agents used by `/parlay-loop` to hold intents/dialogs/artifacts in one context, then switch to a fresh context for build and again for code generation. The Generic adapter has no native subagent primitive; `/parlay-loop` falls back to a fresh-session handoff (printing `/parlay-loop <feature> --from <phase>` for the user to re-invoke in a new session).

## Key Design Principles

**Intent-first.** User goals are the atomic unit. Everything derives from them.

**Closed vocabulary.** The surface interaction vocabulary (Shows, Actions, Flows) is a finite, defined set — not free text. This makes adapter mapping exhaustive and validatable.

**Surface and infrastructure coexist.** Not all feature work is user-visible. Infrastructure fragments capture internal code changes (helpers, resolvers, cross-cutting patterns) alongside surface fragments in the same feature. Both feed the buildfile.

**Behavior-level contract (aspirational).** The buildfile + adapter + blueprint are structured enough that an agent can regenerate code without re-reading design. The goal is that two agents following the same spec produce functionally equivalent output — observable behavior, not byte-identical code — with latitude on naming and style. This is a design target, not a verified invariant; treat it as an expectation the tool is built around rather than something it proves automatically.

**Strict codegen boundary.** By convention, `/parlay-generate-code` reads only from `.parlay/` (buildfile, adapter, blueprint) and never from `spec/intents/`. The skill enforces this rule; if the agent finds it needs design-level detail, that's a signal the buildfile schema is leaking information.

**Three-zone ownership.** Designer-authored input, engineering handoff, and tool internals are strictly separated. Cross-zone writes are errors.

**Initiatives group, never block.** Features inside an initiative are still driven independently through the pipeline. Initiatives are folders for organization and naming, not collective-processing units.

**Incremental by default.** Stable component files are preserved byte-for-byte; only components with changed upstream sources are regenerated. Cross-cutting entries and brownfield merges are re-applied on every run. Hand-edits to any tracked file are detected and surfaced before overwriting.

**Portable across agents.** Skills are plain English markdown. The helper binary handles parsing, validation, and state management. Adding support for a new AI agent requires only a deployer, not skill rewrites — provided the agent supports slash-command dispatch and interactive prompts. `/parlay-loop` additionally needs native subagent support; without it the loop degrades to fresh-session handoff.

**One continuous loop, many checkpoints.** `/parlay-loop` lets you walk a feature end-to-end, but you confirm at every phase boundary. No auto-advance. Mid-loop edits and resumption via `--from` are first-class.
