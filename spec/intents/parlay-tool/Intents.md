# Intent Design Tool

> A toolkit that takes user intents and dialogues and parlays them into prototypes, surfaces, and engineering specifications — without requiring the designer to write code.

---

## Author Intents

**Goal**: Describe what users need using simple, human-readable documents that capture goals, context, and constraints.
**Persona**: UX Designer
**Context**: Starting a new feature — the designer needs to capture user goals before any dialogs, surfaces, or code exist.
**Action**: Write markdown documents following the intent schema.
**Objects**: intent, feature

**Constraints**:
- The designer must never need to touch generated code or final specifications
- The only documents the designer works with are intents, dialogs, and surfaces
- The format must support quick iteration on different ideas
- A new intent should be writable in under 5 minutes

**Hints**:
- What if the designer wants to revise intents after dialogs and surfaces already reference them?
- Should the format support inline comments or annotations for collaboration?
- How minimal can the format be while still enabling prototype generation?

---

## Scaffold Dialogs from Intents

**Goal**: Generate dialog templates from authored intents so the designer has a starting point for writing user-system conversations.
**Persona**: UX Designer
**Context**: The designer has finished authoring intents for a feature and is ready to describe the user-system interactions.
**Action**: Run a command that reads intents and generates one dialog template per intent, pre-filled from Goal and Action fields.
**Objects**: intent, dialog, dialog-template, feature

**Constraints**:
- Generated templates must follow the dialog schema and be immediately editable
- Templates are a starting point — the designer owns and rewrites them
- Must not overwrite existing dialogs if some have already been authored

**Hints**:
- Should the command generate one dialog per intent, or group related intents into a single dialog?
- What if some intents are too abstract to produce a useful dialog template?

---

## Configure Project Tools

**Goal**: Choose the AI agent, SDD framework, and prototype framework that will be used throughout the project.
**Persona**: UX Designer
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

**Hints**:
- What if the designer wants to change frameworks mid-project?
- Should configuration be per-project or per-feature?
- How are new framework extensions discovered and installed?
- How does the tool update internal schemas when a new version of parlay is available?

---

## Register Framework Adapter

**Goal**: Provide the tool with knowledge about a specific prototype framework — what components it offers, what patterns it uses, and how abstract design concepts map to concrete framework constructs.
**Persona**: Tool creator
**Context**: A new prototype framework (e.g., Angular + Clarity, React + MUI, Go CLI) needs to be supported. The adapter teaches the tool how to translate surface fragments into framework-specific buildfile entries.
**Action**: Define a framework adapter that maps abstract component types to framework-specific widgets, layout patterns, interaction styles, and file conventions.
**Objects**: framework-adapter, component-mapping, widget-vocabulary, layout-pattern

**Constraints**:
- The adapter must be loadable at build-feature time without modifying the core tool
- The buildfile schema defines the abstract structure — the adapter fills in the framework-specific vocabulary
- Each adapter must declare its supported component types and interaction patterns
- Adapters must be versionable — different versions of a framework may have different component sets

**Hints**:
- Should adapters be bundled with the tool or installed separately?
- Can adapters extend the buildfile schema with framework-specific sections?
- How does the tool validate that a buildfile uses only vocabulary from the loaded adapter?

---

## Generate Surface from Intents and Dialogs

**Goal**: Generate a surface that describes the UI fragments a feature contributes, based on its intents and dialogs.
**Persona**: UX Designer
**Context**: Intents and dialogs are authored — the designer needs to see what UI pieces this feature produces before moving to prototyping.
**Action**: AI agent reads intents and dialogs, proposes UI fragments with Shows/Actions, and presents them for review.
**Objects**: surface, fragment, intent, dialog, feature

**Constraints**:
- The surface is generated by the tool — the designer reviews and edits, never writes from scratch
- The output format must be simple enough to review and adjust without tooling
- Must not overwrite existing surface fragments if the surface file already exists
- Ambiguities in intents/dialogs must be resolved through questions before generating

**Hints**:
- Should the tool suggest page targets during generation, or leave that for later?
- What happens when intents are added after a surface already exists — incremental update or full regeneration?
- Should surface variants (e.g., mobile vs. desktop) be supported?

---

## Generate Surface from Figma

**Goal**: Use an existing Figma design to generate or update the surface document, so visual work done in Figma carries over into the same format.
**Persona**: UX Designer
**Context**: The designer has already created visual mockups in Figma and wants to use them as the basis for the surface rather than generating from intents alone.
**Action**: AI agent connects to Figma via MCP, extracts components and layout, and produces surface fragments.
**Objects**: surface, fragment, figma-design, feature

**Constraints**:
- Must integrate with Figma via MCP
- The output must be the same surface format as intent-generated surfaces
- The designer must review the generated surface before it is used
- When a Figma design covers multiple features, the tool must ask how to split fragments across features

**Hints**:
- How should conflicts between an existing surface and the Figma import be resolved?
- Should changes in Figma automatically trigger surface updates, or is it always a manual pull?
- Can the tool map Figma components to fragment names from existing intents?

---

## View Assembled Page

**Goal**: See the full layout of a page by assembling all feature fragments that target it, so the designer can review the cross-feature experience.
**Persona**: UX Designer
**Context**: Multiple features target the same page — the designer wants to see what the assembled screen looks like before locking or prototyping.
**Action**: Tool collects all fragments targeting the page from all feature surfaces, groups by region, sorts by order, and presents the assembled view.
**Objects**: page, fragment, surface, region

**Constraints**:
- Must show fragments from all features, not just the current one
- Must flag conflicts — fragments targeting the same region with the same order
- The assembled view is read-only — changes are made in individual feature surfaces

**Hints**:
- Should the view show a diff against a locked page manifest if one exists?
- Should unplaced fragments (no page target) be shown in a separate section?

---

## Lock Page Layout

**Goal**: Create a page manifest that freezes the arrangement of fragments on a page, giving the layout an explicit owner and a reviewable document.
**Persona**: UX Designer
**Context**: The assembled page view looks right, or the team needs to agree on a layout before handoff — the designer wants to lock it down.
**Action**: Tool generates a page manifest from the current assembled view, the designer reviews and adjusts, then sets the status.
**Objects**: page, page-manifest, fragment, region

**Constraints**:
- The manifest is generated from the current assembled state — not written from scratch
- The designer must review before the manifest is considered active
- A locked manifest must warn if features add or remove fragments targeting that page
- Must not block features from being prototyped in isolation

---

## Integrate with AI Agent via Skills

**Goal**: Provide AI-heavy capabilities as agent skills — markdown files the AI agent reads and executes natively — while keeping the CLI as a helper binary for mechanical operations.
**Persona**: Tool creator
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

**Hints**:
- Should skills fall back to CLI heuristics when no agent is available?
- How should skill versioning work when the tool updates?

---

## Resolve Ambiguities Through AI Dialogue

**Goal**: Have the AI agent identify and resolve ambiguities in intents, dialogs, and surfaces by asking the designer directly during specification creation.
**Persona**: UX Designer
**Context**: The designer has written intents and dialogs, but some details are ambiguous, incomplete, or contradictory — the AI agent needs to clarify before generating output.
**Action**: The agent analyzes documents, identifies ambiguities, presents each one to the designer as a conversational question with lettered options, waits for the response, then proceeds with generation.
**Objects**: intent, dialog, surface, disambiguation-record

**Constraints**:
- The agent talks directly to the designer — no CLI mediator, no YAML round-trip
- Each ambiguity is presented with lettered options (A/B/C) and an optional freeform choice
- The agent must ask permission before updating any human-owned file (intents, dialogs, surface)
- Answers must be incorporated back into the source documents, not just used silently
- Prior disambiguation decisions are stored in disambiguation.yaml and referenced to avoid re-asking

**Hints**:
- What if the designer doesn't know the answer — can they defer a decision?
- Should the agent present all ambiguities at once or one at a time?

---

## Generate Prototype

**Goal**: Translate intents, dialogs, and surfaces into a working interactive prototype without the designer writing any code.
**Persona**: UX Designer
**Context**: Surface is reviewed, framework is chosen — the designer wants a running prototype.
**Action**: Tool loads the framework adapter for the configured prototype framework, generates a buildfile using abstract structure filled with framework-specific vocabulary, generates test cases, then produces code and runs tests.
**Objects**: prototype, buildfile, test-case, framework-adapter, surface, intent, dialog

**Constraints**:
- The buildfile must be generated using the framework adapter — not hardcoded to any framework
- The same surface + different framework adapter must produce a structurally equivalent but framework-appropriate buildfile
- The buildfile must be deterministic — same inputs + same adapter = same buildfile
- Code generation reads the buildfile only — not the intents, dialogs, or surface directly
- The designer must never need to modify generated prototype code
- The prototype must be testable — the system generates both implementation and property-based tests

**Hints**:
- Should the prototype be regeneratable from scratch, or does it accumulate state between builds?
- How does the designer trigger a rebuild after changing an intent or dialog?
- What level of visual fidelity is expected — wireframe, design-system-accurate, or pixel-perfect?

---

## Generate Engineering Specification

**Goal**: Translate the design artifacts into a formal engineering specification that the development team can use to build the production system.
**Persona**: UX Designer working with engineers
**Context**: The prototype has been validated and the design is stable — it's time to hand off to engineering in their preferred specification format.
**Objects**: engineering-spec, sdd-framework, intent, dialog, surface

**Constraints**:
- Must support all popular SDD frameworks (GitHub SpecKit, Kiro, Tessl, etc.)
- Must provide extensibility points for new SDD formats in the future
- The generated specification must be reviewable by the designer before handoff

**Hints**:
- Should the engineering spec include the original intents/dialogs as context, or only the formal specification?
- How does the spec stay in sync if the designer makes changes after generation?
- What if the engineering team uses a format the tool doesn't support yet?

---

## Extract and Share Domain Models

**Goal**: Extract a domain model from the current project's specifications and share it with other designers or engineers working in the same domain.
**Persona**: UX Designer working with a team
**Context**: The project has matured enough that its domain entities and relationships are valuable to other team members working on related features.
**Action**: AI agent reads through all specifications to extract entities, relationships, and state machines into a portable model file.
**Objects**: domain-model, entity, relationship, state-machine

**Constraints**:
- The domain model must be packable into a portable format that can be loaded into another project
- Loading an external domain model must integrate it with the current project's existing model
- When integration is ambiguous, the AI agent must ask the designer how to resolve it

**Hints**:
- What if two domain models define the same entity differently?
- Should domain models be versioned?
- How does the designer know which parts of the model are relevant to share vs. internal implementation details?

---

## Sync Intents and Dialogs

**Goal**: Identify gaps between intents and dialogs — intents that have no corresponding dialog, and dialogs that don't trace back to any intent — and generate templates for the missing pieces.
**Persona**: UX Designer
**Context**: The designer has been authoring intents and dialogs independently and wants to check that everything is covered before generating a surface or prototype.
**Action**: AI agent scans all intents and dialogs in a feature, matches them by content and references, and produces a coverage report with ready-to-fill dialog templates for uncovered intents.
**Objects**: intent, dialog, coverage-report, dialog-template

**Constraints**:
- Generated dialog templates must follow the dialog schema and be immediately editable by the designer
- The sync must not modify existing human-authored files without permission
- The coverage report must clearly distinguish between missing dialogs and dialogs that exist but may not fully cover an intent

**Hints**:
- Should the sync also detect dialogs that cover functionality not captured in any intent (orphan dialogs)?
- Should it suggest splitting a dialog that covers too many intents?
- Can the AI pre-fill dialog templates with a best guess based on the intent's Goal and Action fields?
