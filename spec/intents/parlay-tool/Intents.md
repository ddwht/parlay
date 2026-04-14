# Intent Design Tool

> A toolkit that takes user intents and dialogues and parlays them into prototypes, surfaces, and engineering specifications — without requiring the designer to write code.

---

## Author Intents

**Goal**: Describe what users need using simple, human-readable documents that capture goals, context, and constraints.
**Persona**: UX Designer
**Priority**: P0
**Context**: Starting a new feature — the designer needs to capture user goals before any dialogs, surfaces, or code exist.
**Action**: Write markdown documents following the intent schema.
**Objects**: intent, feature

**Constraints**:
- The designer must never need to touch generated code or final specifications
- The only documents the designer works with are intents, dialogs, and surfaces
- The format must support quick iteration on different ideas
- A new intent should be writable in under 5 minutes

**Verify**:
- An intent with only Goal and Persona fields is valid
- An intent with all fields (Goal, Persona, Priority, Context, Action, Objects, Constraints, Verify, Questions) is valid
- Intents are separated by `---` and each starts with `## `

**Questions**:
- What if the designer wants to revise intents after dialogs and surfaces already reference them?
- Should the format support inline comments or annotations for collaboration?

---

## Scaffold Dialogs from Intents

**Goal**: Generate dialog templates from authored intents so the designer has a starting point for writing user-system conversations.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer has finished authoring intents for a feature and is ready to describe the user-system interactions.
**Action**: Run a command that reads intents and generates one dialog template per intent, pre-filled from Goal and Action fields.
**Objects**: intent, dialog, dialog-template, feature

**Constraints**:
- Generated templates must follow the dialog schema and be immediately editable
- Templates are a starting point — the designer owns and rewrites them
- Must not overwrite existing dialogs if some have already been authored

**Verify**:
- Each generated template has a `### ` heading and `**Trigger**:` field
- Existing dialogs in the file are preserved when new templates are appended
- Templates pre-fill `User:` and `System:` turns from the intent's Goal and Action

**Questions**:
- Should the command generate one dialog per intent, or group related intents into a single dialog?
- What if some intents are too abstract to produce a useful dialog template?

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
- The agent-specific config file (e.g., CLAUDE.md for Claude Code) must be lightweight — commands and pointers, not full schema content
- The project layout uses three zones with strict ownership: `spec/intents/` (designer-authored input — only intents.md, dialogs.md, surface.md are user-facing), `spec/handoff/` (engineering-consumed output), and `.parlay/` (tool internals — never user-facing)

**Verify**:
- `.parlay/config.yaml` is created with the selected agent, SDD framework, and prototype framework
- `.parlay/schemas/` directory is populated with schema files
- `.parlay/build/` directory is created for internal build artifacts
- `spec/intents/` directory is created for designer-authored inputs
- `spec/handoff/` directory is created for engineering output artifacts
- The wizard presents only installed/available options

**Questions**:
- What if the designer wants to change frameworks mid-project?
- Should configuration be per-project or per-feature?
- How are new framework extensions discovered and installed?
- How does the tool update internal schemas when a new version of parlay is available?

---

## Register Framework Adapter

**Goal**: Provide the tool with knowledge about a specific prototype framework — what components it offers, what patterns it uses, what design choices are preferred, and how abstract design concepts map to concrete framework constructs.
**Persona**: Tool creator
**Priority**: P1
**Context**: A new prototype framework (e.g., Angular + Clarity, React + MUI, Go CLI) needs to be supported. The adapter teaches the tool how to translate surface fragments into framework-specific buildfile entries and how to make design decisions that fit the framework.
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

**Questions**:
- Should adapters be bundled with the tool or installed separately?
- Can adapters extend the buildfile schema with framework-specific sections?
- How does the tool validate that a buildfile uses only vocabulary from the loaded adapter?

---

## Generate Surface from Intents and Dialogs

**Goal**: Generate a surface that describes the UI fragments a feature contributes, based on its intents and dialogs.
**Persona**: UX Designer
**Priority**: P0
**Context**: Intents and dialogs are authored — the designer needs to see what UI pieces this feature produces before moving to prototyping.
**Action**: AI agent reads intents and dialogs, proposes UI fragments with Shows/Actions, and presents them for review.
**Objects**: surface, fragment, intent, dialog, feature

**Constraints**:
- The surface is generated by the tool — the designer reviews and edits, never writes from scratch
- The output format must be simple enough to review and adjust without tooling
- Must not overwrite existing surface fragments if the surface file already exists
- Ambiguities in intents/dialogs must be resolved through questions before generating

**Verify**:
- Each generated fragment has `## ` heading, `**Shows**:`, and `**Source**:` fields
- `**Source**:` references trace back to existing intents via `@feature/intent-slug`
- Existing fragments are preserved when new ones are added
- Ambiguities are surfaced as questions before generation proceeds

**Questions**:
- Should the tool suggest page targets during generation, or leave that for later?
- What happens when intents are added after a surface already exists — incremental update or full regeneration?
- Should surface variants (e.g., mobile vs. desktop) be supported?

---

## Reference Design Spec from Figma

**Goal**: Enrich an existing surface with per-fragment visual design details extracted from a Figma file, producing a design-spec.yaml that captures widget specifics, layout, tokens, variants, spacing, and colors that the surface deliberately omits.
**Persona**: UX Designer
**Priority**: P2
**Context**: The surface is authored and reviewed, a Figma design exists for the feature, and the team wants higher visual fidelity in the generated prototype than adapter defaults provide.
**Action**: AI agent connects to Figma via MCP, maps Figma components to existing surface fragments, and generates a design-spec.yaml with per-fragment visual annotations.
**Objects**: design-spec, surface, fragment, figma-design, design-token, feature

**Constraints**:
- The surface must already exist — this skill enriches, it does not create the surface
- Fragment names in design-spec.yaml must match fragment names in surface.md exactly
- The design-spec is optional — the pipeline must work without it
- Requires Figma MCP — if unavailable, the skill must inform the user and stop
- The design-spec references the adapter's design-system categories — token category names must match
- The design-spec is a tool internal at `.parlay/build/<feature>/design-spec.yaml`, same visibility as buildfile.yaml
- Build-feature reads the design-spec IF it exists; if not, adapter defaults apply unchanged

**Verify**:
- design-spec.yaml is generated at `.parlay/build/<feature>/design-spec.yaml`
- Every fragment key in design-spec.yaml matches a fragment name in surface.md
- Token category references match categories declared in the adapter's design-system section
- build-feature produces a richer buildfile when design-spec.yaml exists compared to when it does not
- The pipeline works identically when design-spec.yaml does not exist (no regression)
- Figma MCP unavailability produces a clear error message, not a crash

**Questions**:
- Should the design-spec support incremental updates when the Figma design changes?
- Should the tool warn when design-system categories marked `source: figma` in the adapter have no corresponding values in the design-spec?

---

## View Assembled Page

**Goal**: See the full layout of a page by assembling all feature fragments that target it, so the designer can review the cross-feature experience.
**Persona**: UX Designer
**Priority**: P1
**Context**: Multiple features target the same page — the designer wants to see what the assembled screen looks like before locking or prototyping.
**Action**: Tool collects all fragments targeting the page from all feature surfaces, groups by region, sorts by order, and presents the assembled view.
**Objects**: page, fragment, surface, region

**Constraints**:
- Must show fragments from all features, not just the current one
- Must flag conflicts — fragments targeting the same region with the same order
- The assembled view is read-only — changes are made in individual feature surfaces

**Verify**:
- Fragments from multiple features targeting the same page are assembled together
- Fragments are grouped by region and sorted by order within each region
- Conflicting fragments (same region + same order) are flagged with a warning
- The output is read-only — no modifications to source surfaces

**Questions**:
- Should the view show a diff against a locked page manifest if one exists?
- Should unplaced fragments (no page target) be shown in a separate section?

---

## Lock Page Layout

**Goal**: Create a page manifest that freezes the arrangement of fragments on a page, giving the layout an explicit owner and a reviewable document.
**Persona**: UX Designer
**Priority**: P2
**Context**: The assembled page view looks right, or the team needs to agree on a layout before handoff — the designer wants to lock it down.
**Action**: Tool generates a page manifest from the current assembled view, the designer reviews and adjusts, then sets the status.
**Objects**: page, page-manifest, fragment, region

**Constraints**:
- The manifest is generated from the current assembled state — not written from scratch
- The designer must review before the manifest is considered active
- A locked manifest must warn if features add or remove fragments targeting that page
- Must not block features from being prototyped in isolation

**Verify**:
- Page manifest file is created at `spec/pages/{page-name}.page.md`
- Manifest lists all fragments in their current region and order
- Warnings are emitted when fragments are added or removed after locking
- Features can still be prototyped independently even when the page is locked

---

## Integrate with AI Agent via Skills

**Goal**: Provide AI-heavy capabilities as agent skills — markdown files the AI agent reads and executes natively — while keeping the CLI as a helper binary for mechanical operations.
**Persona**: Tool creator
**Priority**: P0
**Context**: Commands like create-surface, build-feature, and extract-domain-model need intelligence the CLI cannot provide. Instead of the CLI calling the agent as a subprocess, the agent should orchestrate the workflow and call the CLI for parsing, validation, and scaffolding.
**Action**: Each AI-heavy command is defined as a skill file (plain English markdown) that the agent reads. The skill instructs the agent what schemas to load, what files to read, what analysis to do, and what to generate. The agent calls the parlay binary for validation and structured parsing.
**Objects**: skill, agent-deployer, schema, framework-adapter

**Constraints**:
- Skills are authored once as agent-agnostic markdown — plain English instructions any AI can follow
- Agent-specific deployers package skills into the right format per agent (Claude Code: .claude/skills/, Cursor: .cursor/rules/, etc.)
- Adding a new agent requires only a new deployer — zero changes to skill content or schemas
- The helper binary exposes parsing, validation, and coverage checking as JSON-output subcommands the agent can call
- Skills reference schemas from .parlay/schemas/ for on-demand loading — not embedded in the skill itself
- Disambiguation is handled conversationally by the agent — no YAML round-trip needed

**Verify**:
- Skill files are plain markdown readable by any AI agent
- The same skill file works across different agents (Claude Code, Cursor, etc.) via deployers
- Adding a new agent requires only a new deployer, not skill or schema changes
- The parlay binary responds to subcommands with JSON output

**Questions**:
- Should skills fall back to CLI heuristics when no agent is available?
- How should skill versioning work when the tool updates?

---

## Resolve Ambiguities Through AI Dialogue

**Goal**: Have the AI agent identify and resolve ambiguities in intents, dialogs, and surfaces by asking the designer directly during specification creation.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer has written intents and dialogs, but some details are ambiguous, incomplete, or contradictory — the AI agent needs to clarify before generating output.
**Action**: The agent analyzes documents, identifies ambiguities, presents each one to the designer as a conversational question with lettered options, waits for the response, then proceeds with generation.
**Objects**: intent, dialog, surface

**Constraints**:
- The agent talks directly to the designer — no CLI mediator, no YAML round-trip, no side-channel cache
- Each ambiguity is presented with lettered options (A/B/C) and an optional freeform choice
- The agent must ask permission before updating any human-owned file (intents, dialogs, surface)
- Resolved decisions are incorporated back into the source documents (intents.md, dialogs.md, surface.md) — once the source is updated, the ambiguity is gone, so there is nothing to "remember"
- Deferred decisions are added to the affected intent's `**Questions**:` field, never stored separately. The intent's own schema already has a home for open questions.

**Verify**:
- Ambiguities are presented one at a time with lettered options
- The designer's choice is incorporated into the source document
- Deferred decisions land in the relevant intent's `**Questions**:` field
- No human-owned file is modified without explicit permission

**Questions**:
- Should the agent present all ambiguities at once or one at a time?

---

## Build Feature Specification

**Goal**: Generate a deterministic build specification (buildfile.yaml + testcases.yaml) that captures the prototype's structure, components, and observable behaviors — without yet writing any code.
**Persona**: UX Designer
**Priority**: P0
**Context**: Surface is reviewed, framework is chosen — the designer is ready to lock down the prototype's structural spec before code generation.
**Action**: Tool loads the framework adapter, reads intents/dialogs/surface/domain-model, generates a buildfile using abstract structure filled with framework-specific vocabulary, generates testcases.yaml from the buildfile, and saves a content baseline for drift detection. Code generation is a separate step.
**Objects**: buildfile, testcase, framework-adapter, surface, intent, dialog, baseline

**Constraints**:
- The buildfile must be generated using the framework adapter — not hardcoded to any framework
- The same surface + different framework adapter must produce a structurally equivalent but framework-appropriate buildfile
- The buildfile is the deterministic intermediate — it must contain enough detail that two AI agents reading it produce code that passes the same testcases (functional determinism, not byte equivalence)
- The designer must never need to read or edit the generated buildfile or testcases — they live in `.parlay/build/{feature}/` as tool internals
- Generated artifacts must pass deep validation — all cross-references (models, components, routes, adapter vocabulary) must resolve
- Buildfile operations must use the formal operations grammar — a closed set of typed operations, not free-form pseudo-code
- Before generation, readiness checks must pass — all preconditions for the build-feature stage are satisfied
- Component design choices must follow the patterns declared in the framework adapter
- Build artifacts (buildfile.yaml, testcases.yaml) are tool internals — they live in `.parlay/build/{feature}/`, never under `spec/intents/`. The designer never sees them.
- testcases.yaml is internal — it drives cross-validation and feeds spec generation, but is not handed off to engineering. Engineering writes their own real tests from `specification.md`.
- This intent **must not commit any build state** — neither `.baseline.yaml` nor `.code-hashes.yaml` is written here. State commit happens atomically at the end of **Generate Prototype Code**, only after tests pass. Saving baseline here would commit source state without corresponding code state, breaking the consistency invariant and stranding the feature in a state where `parlay diff` reports everything stable but no code exists.
- Rebuilds are incremental at the component level: once a build state has been committed by a previous successful end-to-end run, `parlay diff @{feature}` reports stable / dirty / removed components based on per-element source hashes; the agent regenerates only dirty components and preserves stable ones verbatim. Brownfield adoption is the limiting case (no committed state yet, everything starts new).
- This intent stops at the build specification. Code generation is handled by **Generate Prototype Code** as a separate step, with the buildfile as the context boundary.

**Verify**:
- `buildfile.yaml` is generated at `.parlay/build/{feature}/buildfile.yaml`
- `testcases.yaml` is generated at `.parlay/build/{feature}/testcases.yaml`
- **No `.baseline.yaml` is written by this intent** — state commit is the responsibility of Generate Prototype Code
- **No `.code-hashes.yaml` is written by this intent** — same reason
- The buildfile uses only vocabulary from the loaded framework adapter
- Deep validation passes: all model references, component references, fixture data, and adapter types resolve
- All buildfile operations conform to the formal grammar (read-file, write-file, create-directory, copy, filter, for-each, transform)
- Readiness check passes before generation begins
- The final report tells the designer to run `/parlay-generate-code @{feature}` next

**Questions**:
- What level of visual fidelity is expected — wireframe, design-system-accurate, or pixel-perfect? (Adapter concern.)

---

## Generate Prototype Code

**Goal**: Translate the build specification into working prototype code that runs and passes the generated tests.
**Persona**: UX Designer
**Priority**: P0
**Context**: Build Feature Specification has produced buildfile.yaml + testcases.yaml. The designer wants a runnable prototype to demo or validate.
**Action**: Tool loads the buildfile and the framework adapter, generates code files following the adapter's file conventions, runs the testcases against the prototype, and reports pass/fail.
**Objects**: prototype, buildfile, testcase, framework-adapter, code-file

**Constraints**:
- Code generation reads ONLY `.parlay/build/{feature}/buildfile.yaml`, `.parlay/adapters/{framework}.adapter.yaml`, and the existing prototype source tree (for incremental updates)
- Code generation MUST NOT read anything under `spec/intents/{feature}/` — if it needs to, the buildfile schema is leaking detail and must be tightened
- `.parlay/build/{feature}/testcases.yaml` is read only at the test execution phase, not during code generation itself
- Two AI agents reading the same buildfile must produce code that passes the same testcases (functional determinism — the contract is observable behavior, not code structure)
- Generated code lives outside `spec/` and `.parlay/` — at the location specified by the adapter's `file-conventions.source-root` (e.g., `src/`, `cmd/`, `app/`)
- The designer must never need to modify generated prototype code
- Incremental regeneration is driven by three read helpers: `parlay diff @{feature}` classifies components as stable/dirty/removed based on source changes AND reports `sections` (models/routes/fixtures) as changed/stable for cross-cutting files; `parlay scan-generated {source-root}` maps existing files to their markers; `parlay verify-generated @{feature}` detects hand-edits via stored content hashes.
- The very first generation of a feature is detected by `parlay verify-generated` returning `has_hashes: false`. In that case, every component and cross-cutting file is treated as new.
- Three marker variants identify generated files: `parlay-component: {name}` for component implementation files, `parlay-component: {name}` + `parlay-artifact: test` for component test files (ride the same component's dirty status), and `parlay-section: {section}` for cross-cutting files derived from a buildfile section (models, routes, fixtures). Files without any marker are user-owned; the tool must never modify or delete them.
- Cross-cutting file tracking: when `parlay diff` reports `sections.models: "changed"`, files marked with `parlay-section: models` are regenerated. When `sections.routes: "changed"`, files marked with `parlay-section: routes` are regenerated. Stable sections leave their files untouched.
- If `parlay verify-generated` reports a file as `modified` for a stable component or section, the user has hand-edited generated code. The agent must NOT silently overwrite it — surface the situation and offer to overwrite, skip, or show the diff.
- After writing generated files, the agent runs the generated tests. **Tests must pass** before any state is committed.
- Only on test success, the agent runs `parlay save-build-state @{feature} --source-root {root}` as the final step. This atomically commits both `.baseline.yaml` and `.code-hashes.yaml` using the write-then-rename pattern. **This is the only place either file is written** — there are no separate `save-baseline` or `save-code-hashes` CLI commands. The atomicity and the single-write-point are intentional: they enforce the consistency invariant that the two files always describe the same point in time (the last successful end-to-end generation).
- If tests fail, `save-build-state` MUST NOT be called. The previous committed state remains in place; the next attempt starts from the same diff as before. Failure recovery is just "fix the issue and re-run."

**Verify**:
- Prototype code is generated at the location specified by the adapter's `file-conventions.source-root`
- Generated tests pass against the generated prototype
- Code generation does not access any file under `spec/intents/{feature}/` (this is the load-bearing isolation rule)
- Re-running with no source changes produces functionally equivalent output (same testcases pass)
- Each generated file is traceable back to a buildfile component

**Questions**:
- Should the agent generate test runners as well, or assume the framework's default?
- What's the right error UX when tests fail — show failures, ask the user what to do?
- When should incremental regen kick in vs. full regen? (Driven by per-component source hashes — see component-memoization design.)

---

## Generate Engineering Specification

**Goal**: Translate the design artifacts into a formal engineering specification that the development team can use to build the production system.
**Persona**: UX Designer working with engineers
**Priority**: P1
**Context**: The prototype has been validated and the design is stable — it's time to hand off to engineering in their preferred specification format.
**Objects**: engineering-spec, sdd-framework, intent, dialog, surface

**Constraints**:
- Must support all popular SDD frameworks (GitHub SpecKit, Kiro, Tessl, etc.)
- Must provide extensibility points for new SDD formats in the future
- The generated specification must be reviewable by the designer before handoff
- The engineering spec lives in `spec/handoff/{feature}/`, separate from designer-facing inputs in `spec/intents/{feature}/`. This is the only handoff zone.
- `specification.md` is currently the only handoff artifact — internal artifacts (buildfile.yaml, testcases.yaml) stay in `.parlay/build/` and are not handed off

**Verify**:
- Engineering spec is generated in the format matching the configured SDD framework
- The generated spec is written to `spec/handoff/{feature}/specification.md`
- The designer can review the spec before it is shared with engineering

**Questions**:
- Should the engineering spec include the original intents/dialogs as context, or only the formal specification?
- How does the spec stay in sync if the designer makes changes after generation?
- What if the engineering team uses a format the tool doesn't support yet?

---

## Extract and Share Domain Models

**Goal**: Extract a domain model from the current project's specifications and share it with other designers or engineers working in the same domain.
**Persona**: UX Designer working with a team
**Priority**: P2
**Context**: The project has matured enough that its domain entities and relationships are valuable to other team members working on related features.
**Action**: AI agent reads through all specifications to extract entities, relationships, and state machines into a portable model file.
**Objects**: domain-model, entity, relationship, state-machine

**Constraints**:
- The domain model must be packable into a portable format that can be loaded into another project
- Loading an external domain model must integrate it with the current project's existing model
- When integration is ambiguous, the AI agent must ask the designer how to resolve it

**Verify**:
- Domain model is written to `spec/intents/{feature}/domain-model.md`
- Entities, relationships, and state machines are extracted from intents and dialogs
- Loading an external model into a project with an existing model triggers disambiguation
- Conflicting entity definitions are flagged for designer resolution

**Questions**:
- What if two domain models define the same entity differently?
- Should domain models be versioned?
- How does the designer know which parts of the model are relevant to share vs. internal implementation details?

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
- Every section is optional — a CLI app may only declare `navigation.strategy: cli-subcommands`
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
- Two agents reading the same buildfile + blueprint + adapter produce structurally equivalent app shells

**Questions**:
- Should `parlay init` scaffold a minimal blueprint, or should it be a separate command?
- Can the blueprint be generated from an existing codebase via an extraction skill?

---

## Sync Intents and Dialogs

**Goal**: Identify gaps and drift across the full artifact chain — intents without dialogs, stale downstream artifacts, broken references — and help the designer bring everything back in sync.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer has been authoring or editing intents and dialogs and wants to check that everything is covered and consistent before generating a surface or prototype.
**Action**: AI agent scans all intents and dialogs in a feature, checks coverage and full-chain traceability, detects content drift in intents that changed since the last build, and produces a report with actionable next steps.
**Objects**: intent, dialog, coverage-report, dialog-template, baseline

**Constraints**:
- Generated dialog templates must follow the dialog schema and be immediately editable by the designer
- The sync must not modify existing human-authored files without permission
- The coverage report must clearly distinguish between missing dialogs and dialogs that exist but may not fully cover an intent
- Content drift detection must compare current intents against a stored baseline from the last build
- The agent must review drifted intents against downstream artifacts and flag meaningful mismatches

**Verify**:
- Covered intents are correctly identified (structural + semantic matching)
- Uncovered intents are listed with an option to generate dialog templates
- Orphan dialogs (no matching intent) are identified and reported
- Intents modified since the last build are flagged with the specific fields that changed
- The agent reviews drifted intents against surface/buildfile/testcases and suggests updates
- Existing human-authored dialogs are never modified without permission

**Questions**:
- Should the sync also detect dialogs that cover functionality not captured in any intent (orphan dialogs)?
- Should it suggest splitting a dialog that covers too many intents?
- Can the AI pre-fill dialog templates with a best guess based on the intent's Goal and Action fields?

---

## Onboard Existing Codebase

**Goal**: Analyze an existing codebase and draft a framework adapter with mount-strategies, file conventions, and coding conventions populated from the project's actual structure and patterns.
**Persona**: Tech Lead
**Priority**: P1
**Context**: A team has an existing project and wants to use Parlay to add new features to it. They need an adapter that reflects their codebase conventions and declares how new components can be integrated into existing pages.
**Action**: AI agent reads representative source files, detects the framework and UI library, identifies widget patterns (Tabs, Routes, Menus), extracts coding conventions, and drafts a complete adapter YAML for review.
**Objects**: adapter, mount-strategy, codebase, framework-detection

**Constraints**:
- Must produce a reviewable draft adapter, not auto-register it — the team reviews and adjusts before registering
- Must not create persistent project-level indexes or appmaps — all codebase knowledge is captured in the adapter (reusable, framework-level) or read from live source code at generation time
- Must work for any supported framework (React, Angular, Go CLI, etc.)
- Must detect common widget patterns (Tabs, Routes, Menus, Collapse, etc.) and generate mount-strategy entries with detection patterns and code templates
- The adapter remains framework-level and reusable — project-specific file paths do not belong in it
- Mount-strategy templates use `{{placeholder}}` syntax for values that vary per integration

**Verify**:
- Adapter YAML is generated with shows, actions, flows, compositions, conventions, file-conventions, and mount-strategies sections
- Detected mount-strategy patterns match widgets that actually exist in the codebase
- The adapter is framework-level — another project using the same framework could use it with convention adjustments
- Team can review and edit the draft before registering via `/parlay-register-adapter`

**Questions**:
- Should the onboard skill also draft a blueprint from the existing route config and layout components?
- How should conventions be handled when the codebase is inconsistent (e.g., some modules use Redux, others use Context)?

---

## Mount Generated Feature into Existing Pages

**Goal**: When generating code for a new feature that targets an existing page, produce a reviewable diff showing exactly how the new component integrates into the existing file, rather than regenerating the entire file.
**Persona**: UX Designer
**Priority**: P0
**Context**: Brownfield project — the Settings page already exists with tabs, and a new feature adds a tab to it. The agent must not overwrite the existing page but instead propose a small change.
**Action**: During generate-code, the agent reads the target page file, matches a mount strategy from the adapter, finds existing instances of the pattern as style examples, generates a new instance, and presents a diff for user confirmation.
**Objects**: mount-strategy, mount-point, diff, existing-file, component

**Constraints**:
- Must never silently modify existing files — all changes to non-Parlay files are presented as reviewable diffs
- Must read the target file first, then match a mount strategy from the adapter's `mount-strategies:` section by looking for the `detection` pattern in the file content — the code determines the strategy, not the other way around
- Must use the existing disambiguation pattern (A/B/C lettered options) when zero or multiple mount strategies match
- When zero strategies match: ask the user how to integrate (show file / skip / add as new route)
- When multiple strategies match: present each with the line number where the detection pattern was found, let user choose
- When exactly one strategy matches: proceed automatically — find existing instances of the template pattern in the file, use them as style examples, generate a new instance
- Generated component files are still created automatically with markers (no diff review needed for new files)
- Files with `parlay-section:` markers are Parlay-owned and handled by the existing step 14 — mount diffs apply only to user-owned files
- Mount diffs are typically small: 1-3 files, a few lines each (an import + a component reference)

**Verify**:
- Agent produces a diff for each existing file that needs modification
- Diff is small and reviewable (a few lines, not a full file rewrite)
- User can approve, skip, or edit each proposed diff
- New component files are created automatically without requiring diff review
- Greenfield behavior is unchanged when no existing files are found or adapter has no mount-strategies
- Existing file content outside the mount point is never modified

**Questions**:
- Should mount diffs be batched (show all diffs at once) or presented one at a time?
- What happens when a feature is regenerated — should the mount diff be re-applied or is it a one-time operation?

---

## Resolve External Types During Code Generation

**Goal**: When generating code that declares a model type, detect if that type already exists in the source tree and import it instead of re-declaring it, avoiding namespace collisions in brownfield projects.
**Persona**: Developer
**Priority**: P1
**Context**: Brownfield project has existing TypeScript interfaces or Go structs (e.g., `User`, `Settings`) that new Parlay features reference in their intents' Objects fields. Without detection, generate-code would create a duplicate type declaration that conflicts with the existing one.
**Action**: At generate-code time, before writing the models/types cross-cutting file, grep the source tree for existing type definitions matching each entity name. If found, emit an import statement instead of a type declaration.
**Objects**: model, type, import, external-type

**Constraints**:
- Must not require a persistent project-level type index or domain model annotation — detection happens by grepping source code at generation time
- Must search the source tree (under adapter's `file-conventions.source-root`) for type/interface/struct definitions matching the entity name
- Must handle framework-specific type declaration patterns: `interface X` (TypeScript), `type X struct` (Go), `class X` (Python/Java)
- When exactly one match is found: record as external, generate import instead of declaration
- When multiple matches are found: present disambiguation with file paths and snippets, let user choose or opt to generate new
- When no match is found: generate the type declaration as before
- Must not modify existing type files — only the generated models file changes (mix of imports for external types and declarations for new types)
- The buildfile schema does not change — it continues to reference entity names without knowing whether they're external or Parlay-owned

**Verify**:
- When an existing type is found in the source tree, the generated code imports it from its actual location
- When no existing type is found, a new type declaration is generated (current behavior preserved)
- Multiple matches trigger disambiguation with file paths and type snippets
- Existing type files are never modified by Parlay
- The buildfile contains no external/import annotations — the resolution is fully at generate-code time
