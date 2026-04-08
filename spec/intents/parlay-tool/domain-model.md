# Intent Design Tool — Domain Model

Extracted from intents.md, dialogs.md, and surface.md.

---

## Entities

### Project

The top-level container for all design work.

**Properties**:
- name: string
- path: string (root directory)

**Relationships**:
- has one ProjectConfig
- has many Feature
- has many Page (derived)
- has many PageManifest (optional)

---

### ProjectConfig

Tool configuration chosen during bootstrap.

**Properties**:
- ai-agent: string (e.g., "Claude Code")
- sdd-framework: string (e.g., "GitHub SpecKit")
- prototype-framework: string (e.g., "Angular + Clarity")

**Relationships**:
- belongs to Project
- stored at `.parlay/config.yaml`

---

### Extension

A pluggable framework or tool integration.

**Properties**:
- name: string
- type: enum (ai-agent, sdd-framework, prototype-framework)
- installed: boolean

**Relationships**:
- selectable during ProjectConfig setup
- prototype-framework extensions have a FrameworkAdapter

---

### FrameworkAdapter

Maps abstract component concepts to framework-specific widgets, patterns, and conventions. Loaded during build-feature to generate framework-appropriate buildfiles.

**Properties**:
- name: string (e.g., "go-cli", "angular-clarity", "react-mui")
- framework: string (matches ProjectConfig.prototype-framework)
- component-types: ComponentMapping[] (abstract type → framework widget)
- layout-patterns: LayoutPattern[] (available layout strategies)
- interaction-patterns: InteractionPattern[] (how user actions are handled)
- file-conventions: FileConvention (where generated code goes, naming rules)

**Relationships**:
- belongs to Extension (one adapter per prototype-framework)
- consumed by build-feature to generate buildfile.yaml
- consumed by code generator to produce prototype code
- stored at `.parlay/adapters/<name>.adapter.yaml`

---

### ComponentMapping

A single mapping from abstract component type to framework-specific widget.

**Properties**:
- abstract-type: string (e.g., "data-display", "selection", "confirmation", "navigation", "form-input")
- widget: string (framework-specific — e.g., "clr-datagrid" for Clarity, "cobra.Command" for Go CLI)
- description: string
- supports: string[] (what the widget can do — "sort", "filter", "paginate", etc.)

**Relationships**:
- belongs to FrameworkAdapter

---

### LayoutPattern

A reusable layout strategy provided by the framework.

**Properties**:
- name: string (e.g., "page-with-sidebar", "sequential-prompts", "tabbed-view")
- regions: string[] (named areas the pattern provides)
- description: string

**Relationships**:
- belongs to FrameworkAdapter

---

### InteractionPattern

How user actions are handled in this framework.

**Properties**:
- name: string (e.g., "select-from-list", "confirm-action", "text-input", "multi-step-wizard")
- trigger: string (what the user does)
- implementation: string (framework-specific approach)

**Relationships**:
- belongs to FrameworkAdapter

---

### Feature

A self-contained design unit with its own intents, dialogs, and surface.

**Properties**:
- name: string (display name, e.g., "upgrade plan creation")
- slug: string (folder name, e.g., "upgrade-plan-creation")
- path: string (spec/intents/<slug>/)

**Relationships**:
- belongs to Project
- has many Intent
- has many Dialog
- has one Surface (optional, generated)
- has one Buildfile (optional, generated)
- has one TestCase (optional, generated)
- has one EngineeringSpec (optional, generated)

**State machine**:
```
created → intents-authored → dialogs-scaffolded → dialogs-authored → surface-generated → prototype-built → enggspec-generated
```

Transitions:
- created → intents-authored: designer writes intents.md
- intents-authored → dialogs-scaffolded: /parlay create-dialogs
- dialogs-scaffolded → dialogs-authored: designer rewrites dialog templates
- dialogs-authored → surface-generated: /parlay create-surface (or create-surface-by-figma)
- surface-generated → prototype-built: /parlay build-feature
- prototype-built → enggspec-generated: /parlay generate-enggspec

Notes: states can be revisited — designer can edit intents after surface exists. State tracks the furthest completed step.

---

### Intent

An atomic user goal within a feature.

**Properties**:
- title: string (required)
- slug: string (derived from title)
- goal: string (required)
- persona: string (required)
- context: string (optional)
- action: string (optional)
- objects: string[] (optional, comma-separated entity names)
- constraints: string[] (optional, bullet list)
- hints: string[] (optional, bullet list)

**Relationships**:
- belongs to Feature
- referenced by Fragment (via Source)
- matched to Dialog (via /parlay sync)

---

### Dialog

A user-system conversation segment within a feature.

**Properties**:
- title: string (optional)
- slug: string (derived from title, or positional: dialog-1, dialog-2)
- trigger: string (optional — command, action, or event)

**Relationships**:
- belongs to Feature
- contains many Turn
- referenced by Fragment (via Source)
- matched to Intent (via /parlay sync)

---

### Turn

A single line in a dialog.

**Properties**:
- speaker: enum (user, system)
- type: enum (regular, background, conditional)
- condition: string (when type is conditional)
- content: string

**Relationships**:
- belongs to Dialog
- may contain Options

---

### Option

A choice presented to the user within a system turn.

**Properties**:
- letter: string (A, B, C, ...)
- description: string
- is-freeform: boolean (true when description is user input placeholder)

**Relationships**:
- belongs to Turn

---

### DialogTemplate

A scaffolded dialog skeleton generated from an intent.

**Properties**:
- title: string (derived from intent title)
- trigger: string (derived from intent action)
- placeholder-turns: Turn[] (generated from intent Goal and Action)

**Relationships**:
- generated from Intent
- becomes Dialog after human editing

---

### Surface

Collection of UI fragments for a feature.

**Properties**:
- feature-name: string (from file header)
- path: string (spec/intents/<feature>/surface.md)

**Relationships**:
- belongs to Feature
- contains many Fragment

---

### Fragment

A discrete UI piece that a feature contributes.

**Properties**:
- name: string (required, unique within feature)
- shows: string (required — what the user sees)
- actions: string (optional — what the user can do)
- source: string[] (optional — @feature/slug references to intents/dialogs)
- page: string (optional — target page name)
- region: string (optional — target region name)
- order: integer (optional — position within region)
- notes: string[] (optional — bullet list)

**Relationships**:
- belongs to Surface (and thus Feature)
- targets Page + Region (optional)
- references Intent and/or Dialog (via source)

---

### Page

A derived view assembling fragments from multiple features.

**Properties**:
- name: string (e.g., "dashboard", "cluster-detail")

**Relationships**:
- derived from Fragment page targets across all features
- contains many Region
- may have one PageManifest (optional)

Notes: Pages are virtual by default — assembled on demand by `/parlay view-page`. They become concrete only when a PageManifest is created.

---

### Region

A named area within a page.

**Properties**:
- name: string (e.g., main, sidebar, header, toolbar, footer, dialog)

**Relationships**:
- belongs to Page
- contains many Fragment (ordered)

---

### PageManifest

Locked layout of a page, created when cross-feature arrangement needs an owner.

**Properties**:
- name: string (matches Page name)
- description: string (optional)
- owner: string (optional — team or person)
- status: enum (draft, reviewed, locked)
- path: string (spec/pages/<name>.page.md)

**Relationships**:
- locks one Page
- contains Region references with ordered Fragment references
- overrides Fragment order values from individual surfaces

**State machine**:
```
draft → reviewed → locked
```

Transitions:
- draft → reviewed: design lead approves the layout
- reviewed → locked: layout frozen for handoff
- locked → draft: explicit unlock for changes (requires discussion)

---

### Buildfile

Prototype build specification — tool internal, never user-facing.

**Properties**:
- path: string (.parlay/build/<feature>/buildfile.yaml)

**Relationships**:
- belongs to Feature
- generated from Intent + Dialog + Surface
- consumed by prototype generation

---

### TestCase

Property-based test specification — tool internal, drives cross-validation and feeds spec generation, never handed off to engineering.

**Properties**:
- path: string (.parlay/build/<feature>/testcases.yaml)

**Relationships**:
- belongs to Feature
- generated alongside Buildfile
- consumed by test generation

---

### Prototype

Generated interactive prototype.

**Properties**:
- framework: string (from ProjectConfig)

**Relationships**:
- belongs to Feature
- generated from Buildfile
- tested by TestCase
- lives outside spec/ (in src/, app/, etc.)

---

### EngineeringSpec

Formal engineering specification for handoff — currently the only handoff artifact in spec/handoff/.

**Properties**:
- path: string (spec/handoff/<feature>/specification.md)
- format: string (from ProjectConfig sdd-framework)

**Relationships**:
- belongs to Feature
- generated from Intent + Dialog + Surface + Buildfile

---

### DomainModel

Portable domain description extracted from project specs.

**Properties**:
- path: string
- entities: Entity[]
- relationships: Relationship[]
- state-machines: StateMachine[]

**Relationships**:
- extracted from Project (all features)
- can be loaded into another Project

---

### Skill

An agent-agnostic instruction file that teaches the AI agent how to perform a Parlay operation.

**Properties**:
- name: string (e.g., "create-surface", "build-feature")
- content: string (markdown instructions — steps, schema references, validation calls)

**Relationships**:
- embedded in the parlay binary
- deployed by Deployer to agent-specific locations
- references schemas from .parlay/schemas/
- may call parlay binary for validation and parsing

---

### Deployer

Agent-specific adapter that packages skills into the format a particular AI agent expects.

**Properties**:
- name: string (e.g., "Claude Code", "Cursor", "Generic")

**Relationships**:
- reads Skills
- writes agent-specific files (e.g., .claude/skills/, .cursor/skills/, AGENT_INSTRUCTIONS.md)
- writes agent config (CLAUDE.md, .cursorrules)
- invoked by `parlay init`

---

### CoverageReport

Analysis result from /parlay sync.

**Properties**:
- covered-intents: {intent: string, dialog: string}[]
- uncovered-intents: string[]
- orphan-dialogs: string[]

**Relationships**:
- generated from Feature (comparing intents vs. dialogs)
- may trigger DialogTemplate generation for uncovered intents

---

## Operation Catalog

Operations implied by the dialogs, mapped to the commands that trigger them.

| Operation | Command | Input | Output |
|---|---|---|---|
| bootstrapProject | (initial setup) | ai-agent, sdd-framework, prototype-framework | Project, ProjectConfig, FrameworkAdapter, .parlay/ |
| registerAdapter | /parlay register-adapter | adapter path | FrameworkAdapter (saved to .parlay/adapters/) |
| addFeature | /parlay add-feature | feature name | Feature (with empty intents.md, dialogs.md) | CLI |
| scaffoldDialogs | /parlay create-dialogs | @feature | DialogTemplate[] → dialogs.md | CLI |
| generateSurface | /parlay create-surface | @feature | Surface (with Fragments) | **Agent** (fallback: CLI heuristics) |
| generateSurfaceFromFigma | /parlay create-surface-by-figma | figma link | Surface (with Fragments) | **Agent** |
| viewPage | /parlay view-page | page name | Page (assembled view) | CLI |
| lockPage | /parlay lock-page | page name, owner | PageManifest | CLI |
| buildFeature | /parlay build-feature | @feature + FrameworkAdapter | Buildfile, TestCase | **Agent** |
| generateEnggSpec | /parlay generate-enggspec | @feature | EngineeringSpec | **Agent** |
| extractDomainModel | /parlay extract-domain-model | (all features) | DomainModel | **Agent** |
| loadDomainModel | /parlay load-domain-model | model path | DomainModel (integrated) | **Agent** |
| syncCoverage | /parlay sync | @feature | CoverageReport | CLI |

---

## Entity Relationship Summary

```
Project
  ├── ProjectConfig (ai-agent, sdd-framework, prototype-framework)
  ├── FrameworkAdapter[]
  │     ├── ComponentMapping[]
  │     ├── LayoutPattern[]
  │     └── InteractionPattern[]
  ├── Feature[]
  │     ├── Intent[]
  │     ├── Dialog[]
  │     │     └── Turn[]
  │     │           └── Option[]
  │     ├── Surface
  │     │     └── Fragment[]  ──targets──→  Page + Region
  │     ├── Buildfile  ←──generated using──  FrameworkAdapter
  │     ├── TestCase
  │     └── EngineeringSpec
  ├── Page[] (derived from Fragments)
  │     ├── Region[]
  │     └── PageManifest (optional)
  └── DomainModel (extracted)
```
