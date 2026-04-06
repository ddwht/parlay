# Intent Design Tool — Dialogs

---

### Bootstrap Project

**Trigger**: User starts a new project

User: I need to bootstrap a project using Intent Design Tool.
System: What AI agent would you like to use? ==List of supported AI agents==
User: Claude Code
System: Do you need to be integrated with any SDD framework?
User: Yes, GitHub SpecKit
System: Great, noted. What framework do you want to use for the prototype? ==List of installed extensions==
User: Angular + Clarity
System: Noted. Setting up the project now.
System (background): Creates project structure:
System (background): — `spec/intents/` folder for features
System (background): — `.parlay/config.yaml` with chosen tools (agent: Claude Code, sdd: GitHub SpecKit, prototype: Angular + Clarity)
System (background): — `.parlay/schemas/` with internal schema definitions
System (background): — `.parlay/adapters/angular-clarity.adapter.yaml` with framework adapter
System (background): — `.claude/skills/parlay-*/SKILL.md` — 12 skills deployed for Claude Code
System (background): — `CLAUDE.md` with skill list and schema loading rules
System: The project is bootstrapped and ready. All operations are available as /parlay-* skills.

---

### Add Feature

**Trigger**: /parlay-add-feature ==feature-name==

User: /parlay-add-feature upgrade plan creation
System (background): Runs `parlay add-feature upgrade plan creation`.
System: I've added the upgrade-plan-creation feature to /spec/intents/upgrade-plan-creation/.
System: Start by filling out intents.md. When your intents are ready, use /parlay-scaffold-dialogs.

---

### Scaffold Dialogs from Intents

**Trigger**: /parlay-scaffold-dialogs @feature-name

User: /parlay-scaffold-dialogs @upgrade-plan-creation
System (background): Runs `parlay create-dialogs @upgrade-plan-creation`.
System: I found ==number== intents. Generating dialog templates.
System (background): For each intent, generates a dialog template pre-filled from the Goal and Action fields.
System: I've added ==number== dialog templates to dialogs.md. Each one has a title, trigger, and placeholder turns based on your intents.
System: Review and rewrite them to capture the real user-system conversation. The templates are a starting point — make them yours.

---

### Generate Surface from Intents and Dialogs

**Trigger**: /parlay-create-surface @feature-name (agent skill)

User: /parlay-create-surface @upgrade-plan-creation
System (background): Reads skill instructions from .claude/skills/parlay-create-surface/SKILL.md.
System (background): Loads .parlay/schemas/surface.schema.md, intent.schema.md, dialog.schema.md.
System (background): Reads intents.md and dialogs.md for @upgrade-plan-creation.
System (background): Checks disambiguation.yaml for prior decisions.
System (background): Analyzes intents and dialogs for ambiguities.
System (condition: ambiguities found): I found some things that need your input before I can generate the surface:
System: ==context excerpt==
System: ==description of ambiguity==
  A: ==option A description==
  B: ==option B description== (recommended)
  C: ==custom input==
User: B
System: Got it. Should I update ==affected-file== to reflect this?
User: Yes
System (background): Updates source file and saves decision to disambiguation.yaml.
System (background): Generates surface.md following the surface schema.
System (background): Runs `parlay validate --type surface` to verify the output.
System: I've generated ==number== fragments in surface.md:
System: - ==Fragment A== — ==brief description==
System: - ==Fragment B== — ==brief description==
System: Review the fragments. Add Page and Region targets to place them on screens.

#### Branch: No Agent (CLI fallback)

User: parlay create-surface @upgrade-plan-creation
System (background): No agent available. Runs basic heuristic generation.
System (background): Creates one fragment per intent with Shows from Goal field.
System: Generated ==number== fragments. For better results, use the /parlay-create-surface skill with an AI agent.

---

### Generate Surface from Figma

**Trigger**: /parlay-create-surface-by-figma ==figma-link==

User: /parlay-create-surface-by-figma ==link to Figma file==
System: I see the Figma link. Let me use Figma MCP to read the design.
System (background): Connects to Figma via MCP, extracts components, layout structure, and content.
System (condition: design maps to single feature): I've generated ==number== fragments in surface.md from the Figma design. Please review.
System (condition: design covers multiple features): This Figma design seems to cover multiple features. I found components that could belong to:
System: - @upgrade-plan — ==list of components==
System: - @fleet-overview — ==list of components==
System: How should I split these?
  A: Assign to the features I listed
  B: Put everything in @upgrade-plan-creation for now
  C: Let me assign them manually
User: Selects A
System (background): Generates surface fragments and distributes them to the appropriate feature surface files.
System: Updated surface.md in ==number== features. Review each one.

---

### View Assembled Page

**Trigger**: /parlay-view-page ==page-name==

User: /parlay-view-page dashboard
System (background): Collects all fragments targeting "dashboard" from all feature surfaces.
System: Here's the assembled dashboard view:
System: **header**:
System: 1. @fleet-overview/health-summary
System: **main**:
System: 1. @fleet-overview/cluster-table
System: 2. @upgrade-plan/readiness-card
System: **sidebar**:
System: 1. @alerts/notification-bar
System (condition: conflicts exist): I found ==number== conflicts:
System: - ==Fragment A== and ==Fragment B== both target ==region== with the same order. Which should come first?
System (condition: unplaced fragments exist): ==number== fragments have no page target yet:
System: - ==Fragment name== from @feature — ==Shows description==
System: Want to assign them to a page?
  A: Yes, let me assign them
  B: No, leave them unplaced for now

---

### Lock Page Layout

**Trigger**: /parlay-lock-page ==page-name==

User: /parlay-lock-page dashboard
System (background): Assembles the current page view for "dashboard".
System: Here's the layout I'll lock:
System: ==shows assembled view, same as view-page==
System: This will create spec/pages/dashboard.page.md. Who should own this page?
User: Platform UX Team
System (background): Creates dashboard.page.md with Owner: Platform UX Team, Status: draft.
System: The page manifest is ready for review at spec/pages/dashboard.page.md. Set the status to "reviewed" or "locked" when you're satisfied with the layout.

---

### Register Framework Adapter

**Trigger**: /parlay-register-adapter ==path-to-adapter==

User: /parlay-register-adapter ./adapters/go-cli.adapter.yaml
System (background): Reads and validates the adapter definition.
System: Registered framework adapter "go-cli":
System: - Component types: 12 (command, prompt, output, list, table, confirmation, ...)
System: - Layout patterns: 3 (sequential-prompts, report-output, interactive-flow)
System: - File conventions: cmd/, internal/
System (background): Copies adapter to .parlay/adapters/go-cli.adapter.yaml
System: The adapter is now available for build-feature. Set it as the prototype framework with: parlay init or update .parlay/config.yaml.

---

### Build Feature

**Trigger**: /parlay-build-feature @feature-name (agent skill)

User: /parlay-build-feature @upgrade-plan-creation
System (background): Reads skill instructions from .claude/skills/parlay-build-feature/SKILL.md.
System (background): Loads .parlay/schemas/buildfile.schema.md, testcases.schema.md, adapter.schema.md.
System (background): Loads framework adapter from .parlay/adapters/go-cli.adapter.yaml.
System: Building with framework adapter: go-cli
System (background): Reads intents.md, dialogs.md, surface.md, domain-model.md for @upgrade-plan-creation.
System (background): Generates buildfile.yaml — maps surface fragments to adapter component types, actions to adapter interaction patterns.
System (background): Generates testcases.yaml from buildfile.yaml.
System (background): Runs `parlay validate --type buildfile` and `parlay validate --type yaml` to verify.
System: Created /spec/intents/upgrade-plan-creation/devspec/buildfile.yaml
System: Created /spec/intents/upgrade-plan-creation/devspec/testcases.yaml
System: [OK] Feature built. Ready for review.

---

### Generate Engineering Specification

**Trigger**: /parlay-generate-enggspec @feature-name

User: /parlay-generate-enggspec @upgrade-plan-creation
System: We're ready to hand over to engineering. Let me read through our specifications and generate an engineering spec in GitHub SpecKit format.
System (background): Reads intents, dialogs, surface, and devspec. Translates to GitHub SpecKit format.
System: The specification is ready: /spec/intents/upgrade-plan-creation/enggspec/specification.md
System: Review it and hand it over to the engineering team.

---

### Extract Domain Model

**Trigger**: /parlay-extract-domain-model

User: /parlay-extract-domain-model
System (background): Reads through all specifications to extract entities, relationships, and state machines.
System: The domain model is saved at ==path to domain model file== and can be shared with others.

---

### Load Domain Model

**Trigger**: /parlay-load-domain-model ==path==

User: /parlay-load-domain-model ./external-model.md
System (background): Reads skill instructions. Loads external model and current project model.
System (background): Compares entity definitions between both models.
System (condition: no conflicts): No conflicts found. Merging models now.
System: I've integrated the model into the current project. Everything mapped cleanly.
System (condition: conflicts found): I found ==number== conflicts that need your input:
System: Entity "==name==" exists in both models with different definitions.
System: How should it be integrated?
  A: Keep current project definition
  B: Use external definition
  C: Merge properties from both
  D: ==custom mapping==
User: C
System: Got it. Should I update spec/domain-model.md to reflect this?
User: Yes
System (background): Saves decision to disambiguation.yaml. Merges models with resolved decisions.
System: Domain model integrated and saved.

---

### Sync Intents and Dialogs

**Trigger**: /parlay-sync @feature-name

User: /parlay-sync @upgrade-plan-creation
System (background): Scans intents.md and dialogs.md for @upgrade-plan-creation.
System: Here's the coverage report:
System: **Covered intents:**
System: - ==intent A== — matched by dialog ==dialog title==
System: - ==intent B== — matched by dialog ==dialog title==
System: **Intents without dialogs:**
System: - ==intent C== — no matching dialog found
System: - ==intent D== — no matching dialog found
System: **Orphan dialogs** (no matching intent):
System: - ==dialog title== — doesn't trace to any intent
System: Want me to generate dialog templates for the uncovered intents?
  A: Yes, generate templates for all
  B: Let me pick which ones
  C: No, just the report is enough
User: Selects A
System (background): Generates dialog templates based on each intent's Goal and Action fields.
System: I've added ==number== dialog templates to dialogs.md. They're marked with ==placeholder turns== for you to fill in.
System: The templates are a starting point — edit them to capture the real conversation.

#### Branch: User Picks Specific Intents

User: Selects B
System: Which intents should I generate templates for?
  ==Checkbox list of uncovered intents==
User: ==Selects specific intents==
System (background): Generates dialog templates for selected intents only.
System: Added ==number== templates to dialogs.md. Ready for your review.
