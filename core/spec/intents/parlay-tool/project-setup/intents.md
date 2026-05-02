# Project Setup

> One-time project bootstrap — choosing tools, registering framework adapters, and defining the application blueprint.

---

## Configure Project Tools

**Goal**: Choose the AI agent, SDD framework, and prototype framework that will be used throughout the project.
**Persona**: UX Designer
**Priority**: P0
**Context**: Bootstrapping a new project — the designer needs to declare which tools the system will use for prototype generation and spec output.
**Action**: Select tools through an interactive setup wizard that presents available options.
**Objects**: project, ai-agent, sdd-framework, prototype-framework, extension

**Constraints**:
- Must support choosing from a list of installed/available extensions
- The tool model must be extensible to add new frameworks and design systems in the future
- Configuration must not require coding knowledge
- The tool must set up internal configuration (schemas, agent rules) that the designer never needs to see or manage
- Internal schemas must be loaded by the AI agent on-demand per command, not kept in agent context permanently
- The agent-specific config file must be lightweight — commands and pointers, not full schema content
- The project layout uses three zones with strict ownership: `spec/intents/` (designer-authored input), `spec/handoff/` (engineering-consumed output), and `.parlay/` (tool internals — never user-facing)

**Verify**:
- `.parlay/config.yaml` is created with the selected agent, SDD framework, and prototype framework
- `.parlay/schemas/` directory is populated with schema files
- `.parlay/build/` directory is created for internal build artifacts
- `spec/intents/` directory is created for designer-authored inputs
- `spec/handoff/` directory is created for engineering output artifacts
- The wizard presents only installed/available options

---

## Register Framework Adapter

**Goal**: Provide the tool with knowledge about a specific prototype framework — what components it offers, what patterns it uses, what design choices are preferred, and how abstract design concepts map to concrete framework constructs.
**Persona**: Tool creator
**Priority**: P1
**Context**: A new prototype framework needs to be supported. The adapter teaches the tool how to translate surface fragments into framework-specific buildfile entries and how to make design decisions that fit the framework.
**Action**: Define a framework adapter that maps abstract component types to framework-specific widgets, layout patterns, interaction styles, file conventions, and preferred design patterns.
**Objects**: framework-adapter, component-mapping, widget-vocabulary, layout-pattern, design-pattern

**Constraints**:
- The adapter must be loadable at build-feature time without modifying the core tool
- The buildfile schema defines the abstract structure — the adapter fills in the framework-specific vocabulary
- Each adapter must declare its supported component types and interaction patterns
- Each adapter must declare its preferred design patterns (interaction style, information density, error placement, confirmation style) so the agent makes consistent decisions across features
- Adapters must be versionable — different versions of a framework may have different component sets

**Verify**:
- Adapter file is saved to `.parlay/adapters/{name}.adapter.yaml`
- All abstract component types from the buildfile schema have a mapping in the adapter
- Adapter declares preferred patterns for interaction, information density, error placement, and confirmation
- Adapter is selectable during project configuration
- Build-feature reads the patterns section and applies them when generating components

---

## Define Application Blueprint

**Goal**: Declare the cross-cutting architectural decisions for an application — layout shells, navigation topology, authorization model, data strategy, error handling, state architecture, and platform integration — so that code generation produces a deterministic app structure.
**Persona**: Tech lead / Architect
**Priority**: P1
**Context**: The project has at least one feature with a reviewed surface, and the team is ready to describe how the app is wired together before generating code. The adapter describes framework conventions; the blueprint describes this specific app's structure.
**Action**: Author a YAML file at `.parlay/blueprint.yaml` that declares shells, routes with guards, authorization roles/policies, data strategy, error boundaries, global state, and (for native apps) platform integration.
**Objects**: blueprint, shell, navigation, guard, role, policy, error-boundary, state-slice

**Constraints**:
- The blueprint is a project-level singleton — one per app, not per feature
- The blueprint lives in `.parlay/` (tool zone) — it is team-authored, not designer-authored
- Every section is optional
- Shell names referenced in navigation routes must exist in the `shells:` section
- Guard names referenced in navigation routes must exist in the `authorization.guards:` section
- The blueprint must not duplicate what the adapter already provides (framework conventions) — it captures only per-app decisions
- Code generation reads the blueprint alongside the buildfile and adapter, preserving the codegen boundary (never reads `spec/intents/`)

**Verify**:
- `.parlay/blueprint.yaml` is created and passes `parlay validate --type blueprint`
- Shell references in `navigation.routes[].shell` resolve to entries in `shells:`
- Guard references in `navigation.routes[].guard` resolve to entries in `authorization.guards:`
- `parlay diff` includes `sections.blueprint` in project-level diff output
- `generate-code` uses the blueprint to produce deterministic shell components, route wiring, guards, error boundaries, and state providers

---
