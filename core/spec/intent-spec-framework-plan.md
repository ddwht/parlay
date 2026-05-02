# Intent-Driven Specification Framework

## Vision

A toolkit and methodology that supports the full design-to-specification loop: **user intent → dialogue → interactive prototype → validation → living specification**. It enables fast iteration on UX ideas, keeps the focus on user workflows rather than architecture, and produces shareable, navigable prototypes that evolve into a comprehensive product specification.

## Core Principles

- **Intent-first**: Everything starts with what the user is trying to accomplish, not with data models or screens.
- **Dialogue as design artifact**: The user↔system conversation is the primary medium for exploring and documenting workflows.
- **Prototype as proof**: The interactive prototype exists to validate the spec, not the other way around. Prototype code is disposable; the spec persists.
- **Independent bootability**: Every scenario, every team's slice, every workflow must be runnable in isolation without dependencies on other parts being ready.
- **Spec as byproduct**: The specification assembles itself from artifacts teams create during their actual design work — it's not a separate documentation effort.

## Problem Context

Existing tools cover parts of the loop but none address it end-to-end:

- **SDD tools** (Kiro, GitHub Spec Kit, Tessl) go from spec → code but treat UX as an afterthought. They're engineering-focused and don't support experience validation through prototyping.
- **Intent Prototyping** (Yegor Gilyov / Smashing Magazine) goes from intent → prototype but lacks shareability, scenario management, team composition, and the bridge to formal specification.
- **Traditional prototyping tools** (Axure, UXPin, Figma) produce visual artifacts but don't generate structured specs, API contracts, or domain models.

This framework fills the gap between UX exploration and engineering specification.

---

## Phase 1 — Define the Intent Language

**Goal**: Establish a markdown-based format for expressing user intents and user↔system dialogues that is human-authored, human-readable, and machine-parseable.

**Why first**: This is the atomic unit. Every other phase depends on having a clear, consistent way to express what the user wants and how the system responds.

### Tasks

- [ ] **1.1 — Draft the intent schema**
  Define the structure for an intent file: goal (one sentence), persona, context/situation, and any constraints. Keep it minimal — a PM or UX writer should be able to author one in 5 minutes.

- [ ] **1.2 — Draft the dialogue format**
  Define how user↔system conversations are written. Each turn should capture: who's speaking (user or system), what they say/do, and optionally what data or state is implied. The format must support branching (unhappy paths) without becoming a flowchart DSL.

- [ ] **1.3 — Write 3–5 real intents with dialogues**
  Use actual VCF workflows (e.g., upgrade readiness, fleet overview, lifecycle operations) to validate the format. Does it capture the essential information? Is it pleasant to write? Can someone unfamiliar with the product understand the workflow from reading it?

- [ ] **1.4 — Define exploration hints convention**
  Alongside the primary dialogue, define a way to note "things worth trying" — edge cases, alternative paths, open design questions. These are prompts for stakeholders reviewing the prototype, not scripted steps.

- [ ] **1.5 — Validate parseability**
  Confirm that the format can be parsed programmatically to extract: data entities mentioned, operations/actions taken, states referenced, and navigation implied. This doesn't require building a parser yet — just confirming the format is structured enough that one could be built.

### Deliverable
A documented schema (as a markdown template) and 3–5 real examples.

---

## Phase 2 — Build the Domain Modeling Bridge

**Goal**: Create a process (and eventually lightweight tooling) to extract typed domain models from dialogues.

**Why second**: The domain model is the shared truth that feeds both the prototype's simulation and the eventual API specification. It must emerge from user needs, not from database design.

### Tasks

- [ ] **2.1 — Manual extraction exercise**
  Take the dialogues from Phase 1 and manually extract: entities (cluster, host, upgrade, eligibility), their properties, relationships, and state transitions. Document the extraction as a repeatable process.

- [ ] **2.2 — Define the domain model format**
  Choose a representation: TypeScript interfaces, Mermaid class diagrams, or both. The format should be usable as both documentation (in the spec) and code (in the prototype). TypeScript interfaces are the natural choice for an Angular/TS stack.

- [ ] **2.3 — Identify state machines**
  For entities with lifecycle states (e.g., an upgrade goes from pending → preflight → in-progress → complete → failed), define the state machine explicitly. This becomes both spec documentation and simulation logic.

- [ ] **2.4 — Map dialogue turns to operations**
  Each user action in a dialogue implies an operation. "User selects Upgrade Readiness action" → `getUpgradeReadiness(clusterId)`. Catalog these as a proto-API — not a formal contract yet, just a list of operations implied by the dialogues.

- [ ] **2.5 — Create a domain model template**
  A standardized file structure for domain models: entities, relationships, state machines, and the operation catalog. This template will be used by every team.

### Deliverable
Typed domain models for 3–5 workflows, a repeatable extraction process, and a file template.

---

## Phase 3 — Create the Simulation Engine Pattern

**Goal**: Build a lightweight, stateful mock layer that lets the prototype behave like a real (simplified) system rather than a slideshow.

**Why third**: Without simulation, scenarios can only show static screens. With simulation, stakeholders can explore freely — click the wrong thing, go back, try unexpected sequences — and get meaningful responses.

### Tasks

- [ ] **3.1 — Define the simulation contract**
  A simulation is: a typed world state, a set of mutation functions (actions), a set of query functions (reads), and rules that keep state coherent after mutations. Define this as a generic pattern/interface.

- [ ] **3.2 — Build an in-memory state store**
  A simple typed store that holds the world state, supports snapshots (for loading fixtures), and emits change notifications (so Angular components react). This can be as simple as a BehaviorSubject-backed service with immutable updates.

- [ ] **3.3 — Implement one real simulation**
  Take the domain model from Phase 2 (e.g., fleet upgrade workflow) and build a concrete simulation. It should handle: seeding from a fixture, responding to user actions with state changes, enforcing state machine transitions, and deriving computed state (e.g., fleet-level eligibility from per-cluster data).

- [ ] **3.4 — Define the fixture format**
  Fixtures are world snapshots — "save files" for the simulation. Define the format: a typed object that fully describes the world state at a point in time. Fixtures are the contract between scenarios (one scenario's exit state becomes another's entry fixture).

- [ ] **3.5 — Establish simulation complexity guidelines**
  Document what level of simulation fidelity is "enough." The simulation should handle the 3–4 main entity types in a workflow and their interactions. It should NOT try to replicate real backend logic. Define the boundary clearly.

### Deliverable
A simulation library/pattern, one working simulation, and fixture format documentation.

---

## Phase 4 — Build the Scenario Registry

**Goal**: Create the mechanism that lists available workflows, loads world presets, and makes specific scenarios shareable via URL.

**Why fourth**: This is what makes the prototype useful to anyone besides the person who built it. It turns "an Angular app" into "a navigable catalog of UX explorations."

### Tasks

- [ ] **4.1 — Define the scenario interface**
  The typed contract for a scenario: id, name, spec metadata (goal, persona, design questions, exploration hints), setup function (receives Angular Injector, hydrates simulation), optional teardown, entry route, and tags for grouping.

- [ ] **4.2 — Build the registry service**
  An Angular service that collects all registered scenarios, supports lookup by ID, and provides filtered/grouped lists (by tag, by team, by status).

- [ ] **4.3 — Implement URL-based activation**
  When the app loads with `?scenario=<id>`, the registry intercepts, runs the scenario's setup, and navigates to the entry route. Without the parameter, the app boots normally. Use `APP_INITIALIZER` or a top-level route guard.

- [ ] **4.4 — Build the overlay UI**
  A floating panel (Angular CDK overlay or fixed-position component) that lists all scenarios grouped by tags. Shows: scenario name, goal summary, exploration hints. Clicking a scenario runs teardown → setup → navigate. Toggled via a small fab button. Has its own styling scope so it doesn't contaminate the prototype.

- [ ] **4.5 — Implement scenario isolation**
  Ensure activating a new scenario fully cleans up the previous one. Teardown resets the simulation state, clears any component-level state, and returns the app to a neutral position before the new setup runs.

- [ ] **4.6 — Add journey support**
  A journey is an ordered list of scenario IDs for end-to-end walkthroughs. The overlay UI shows a stepped sequence — complete one scenario, click "next," the registry transitions to the next. Each step remains independently bootable.

- [ ] **4.7 — Support scenario variants**
  When two scenarios explore different design directions for the same intent (e.g., inline remediation vs. dialog-based), the registry should surface them as related variants. Different design → different feature folder and entry route. Different data → same feature folder, different setup.

### Deliverable
Working registry with overlay UI, URL activation, and journey support.

---

## Phase 5 — Establish the Team Boundary Model

**Goal**: Define conventions that let multiple teams contribute to the same system specification independently, while making integration visible early.

**Why fifth**: Only matters once more than one team is involved. But the conventions need to be established before teams diverge, or integration becomes painful.

### Tasks

- [ ] **5.1 — Define the three-zone model**
  Document the owned/published/integrated pattern:
  - **Owned**: Internal to a team — domain models, workflows, scenarios, prototype code. Full autonomy.
  - **Published**: Outward-facing contracts — the interfaces other teams can depend on. Changes through deliberate versioning.
  - **Integrated**: Where published contracts meet. Nobody owns exclusively.

- [ ] **5.2 — Create the team template repo structure**
  A standardized directory layout:
  ```
  teams/
    <team-name>/
      intents/           ← intent files and dialogues
      domain/            ← owned domain models
      simulation/        ← team's simulation logic
      scenarios/         ← team's scenarios with specs
      prototype/         ← UI code (disposable)
      published/         ← contracts other teams depend on
  integration/
    mappings/            ← how published contracts connect
    scenarios/           ← cross-team integration scenarios
  ```

- [ ] **5.3 — Define the published contract format**
  What a team exposes: entity interfaces (the external view of their domain objects), events (state changes others might care about), and dependency declarations (what they need from other teams). TypeScript interfaces with clear documentation.

- [ ] **5.4 — Create stub generation pattern**
  When team A depends on team B's published contract, team A uses a stub — a simple implementation that conforms to the contract but returns static/configurable data. Define how stubs are created, where they live, and how they're replaced with real implementations in integration scenarios.

- [ ] **5.5 — Define integration scenario conventions**
  Integration scenarios wire multiple teams' simulations together via their published contracts. Document how these are authored, who owns them, and how they surface in the scenario registry (separate "integration" tag group).

- [ ] **5.6 — Establish contract alignment checks**
  A lightweight CI step that verifies: do team A's dependency declarations match team B's published contracts? Are there breaking changes in published contracts that would affect other teams? Flag mismatches as early warnings.

### Deliverable
Documented conventions, template repo structure, and contract alignment tooling.

---

## Phase 6 — Build the Spec Generator

**Goal**: Automatically assemble a unified specification document from all existing artifacts.

**Why sixth**: This is the payoff — but it's also the easiest phase, because it's mostly reading and formatting things that already exist.

### Tasks

- [ ] **6.1 — Define the spec document structure**
  What the generated spec looks like: a table of contents organized by user intents, with each section containing the intent, dialogue, domain entities involved, implied operations, link to live scenario, design questions, and exploration findings.

- [ ] **6.2 — Build the artifact walker**
  A script that traverses the team directories and collects: intent files, dialogue files, domain model files, scenario metadata, published contracts, and integration mappings. Produces a structured intermediate representation.

- [ ] **6.3 — Generate the spec index**
  From the intermediate representation, produce a navigable document — a static site or markdown tree. Each workflow links to its live prototype scenario. Each domain entity links to every workflow that touches it. Published contracts appear as the formal interfaces between team sections.

- [ ] **6.4 — Add traceability links**
  Every API operation in the spec should trace back to a dialogue turn that implied it. Every entity property should trace back to a system response that displayed it. Make these links explicit and navigable.

- [ ] **6.5 — Support design decision records**
  When a scenario variant is rejected, capture the decision: which alternatives were explored, which was chosen, and why. These become part of the spec as design decision documentation.

- [ ] **6.6 — Automate spec generation in CI**
  The spec regenerates on every push. The live version is always up to date. Add diff detection — highlight what changed in the spec between commits.

### Deliverable
A generated, navigable specification document with traceability from user intent to API contract.

---

## Phase 7 — Create the Validation Loop

**Goal**: Ensure scenarios stay healthy, contracts stay aligned, and stakeholder feedback is captured.

### Tasks

- [ ] **7.1 — Build scenario smoke tests**
  For every registered scenario: instantiate setup, confirm it doesn't throw, confirm the entry route exists in the router config. Run in CI. Catches 80% of drift.

- [ ] **7.2 — Add contract alignment validation**
  Verify that published contracts across teams are compatible. Dependency declarations match available contracts. Type shapes are compatible. Run in CI alongside smoke tests.

- [ ] **7.3 — Implement feedback capture in overlay UI**
  Stakeholders reviewing a scenario can leave comments — tied to the specific scenario, timestamped. These feed back into the spec as annotations or open questions.

- [ ] **7.4 — Add optional scenario verification**
  Scenarios can optionally declare a `verify()` function — lightweight checks that run after setup + navigation to confirm the world looks right. "The upgrade banner is showing." "The fleet table has 3 rows." Not full E2E, just smoke signals.

- [ ] **7.5 — Track scenario coverage**
  Which intents have scenarios? Which have been reviewed by stakeholders? Which have stable specs vs. open design questions? Surface this in the spec index as a maturity/coverage dashboard.

### Deliverable
CI-integrated health checks, feedback mechanism, and coverage tracking.

---

## Phase 8 — Define the Handoff Format

**Goal**: Define what engineering receives when a workflow specification is "done."

### Tasks

- [ ] **8.1 — Define "done" criteria for a workflow spec**
  What must be true before a spec is handed off: intent is validated, dialogue is stable, domain model covers the workflow, at least one scenario is stakeholder-reviewed, design questions are resolved, API operations are cataloged.

- [ ] **8.2 — Generate user stories from intents**
  Map intent + dialogue to standard user story format with acceptance criteria. The dialogue steps become acceptance criteria. The exploration hints become test scenarios.

- [ ] **8.3 — Generate API contract stubs**
  From the operation catalog (Phase 2) and published contracts (Phase 5), produce formal API definitions — OpenAPI schemas or TypeScript interfaces. These are starting points for engineering, not final contracts.

- [ ] **8.4 — Package test fixtures**
  The scenario fixtures become test data for engineering. Package them in a format engineering can consume in their integration tests. The fixture format should be backend-agnostic.

- [ ] **8.5 — Produce the handoff bundle**
  For each workflow: the spec document section, API contract stubs, domain model definitions, test fixtures, a link to the live prototype scenario, and design decision records. One self-contained package per workflow.

### Deliverable
A documented handoff format and tooling to produce handoff bundles from the spec.

---

## Recommended Sequencing

```
Phase 1 (Intent Language)  ──┐
                              ├──→  Phase 3 (Simulation)  ──→  Phase 4 (Registry)
Phase 2 (Domain Bridge)   ──┘                                       │
                                                                     ├──→  Phase 6 (Spec Gen)
                                              Phase 5 (Team Model)  ─┘          │
                                                                                 ├──→  Phase 8 (Handoff)
                                                                 Phase 7 (Validation)
```

**Start immediately** (no tooling needed): Phases 1 and 2 — write real intents, dialogues, and extract domain models by hand for your current VCF work.

**Build next** (enables shareability): Phases 3 and 4 — the simulation and registry turn your prototype into something demoable and navigable.

**Add when teams scale**: Phase 5 — the boundary model only matters once multiple teams contribute.

**Build last** (they consume everything above): Phases 6, 7, 8 — spec generation, validation, and handoff are the payoff layer.
