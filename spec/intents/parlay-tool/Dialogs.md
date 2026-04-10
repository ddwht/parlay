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
System (background): Creates project structure (three zones — designer input, engineering output, tool internals):
System (background): — `spec/intents/` folder for designer-authored feature inputs (intents, dialogs, surface)
System (background): — `spec/handoff/` folder for engineering handoff artifacts (specification.md per feature)
System (background): — `.parlay/config.yaml` with chosen tools (agent: Claude Code, sdd: GitHub SpecKit, prototype: Angular + Clarity)
System (background): — `.parlay/schemas/` with internal schema definitions
System (background): — `.parlay/adapters/angular-clarity.adapter.yaml` with framework adapter
System (background): — `.parlay/build/` folder reserved for internal build artifacts (populated on first build-feature)
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
System (background): Updates source file with the resolved decision.
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
System (background): Runs `parlay check-readiness @upgrade-plan-creation --stage build-feature` to verify preconditions.
System (condition: readiness check fails): I can't build yet. Found ==number== issues:
System: - ==issue description==
System: Fix these and run /parlay-build-feature again.
System (background): Runs `parlay diff @upgrade-plan-creation` to classify components as stable / dirty / removed.
System (condition: first build): No committed state yet — generating the full buildfile from scratch.
System (condition: incremental build): Diff report:
System: - ==N== stable components (will be preserved verbatim)
System: - ==M== dirty components (will be regenerated): ==list==
System: - ==K== removed components (will be dropped): ==list==
System: Building with framework adapter: go-cli
System (background): Reads intents.md, dialogs.md, surface.md, domain-model.md for @upgrade-plan-creation.
System (background): Generates buildfile.yaml — preserves stable components verbatim, regenerates dirty components from current sources, drops removed components.
System (background): Generates testcases.yaml from buildfile.yaml.
System (background): Runs `parlay validate --type buildfile --deep --json` to verify cross-references with structured error output.
System (background): Writes buildfile.yaml and testcases.yaml to .parlay/build/upgrade-plan-creation/ (tool internals — designer never edits these). Does NOT save baseline or code-hashes — that happens at the end of /parlay-generate-code, only after tests pass.
System: [OK] Build specification ready. Internal artifacts saved to .parlay/build/upgrade-plan-creation/. All cross-references validated.
System: Next: run /parlay-generate-code @upgrade-plan-creation to produce the prototype code, run tests, and commit the build state.

---

### Generate Prototype Code

**Trigger**: /parlay-generate-code @feature-name (agent skill)

User: /parlay-generate-code @upgrade-plan-creation
System (background): Reads skill instructions from .claude/skills/parlay-generate-code/SKILL.md.
System (background): Loads .parlay/schemas/buildfile.schema.md, adapter.schema.md.
System (background): Reads .parlay/build/upgrade-plan-creation/buildfile.yaml.
System (background): Reads .parlay/adapters/go-cli.adapter.yaml.
System (background): Does NOT read anything under spec/intents/upgrade-plan-creation/ — the buildfile is the only design source.
System (condition: buildfile missing): I can't generate code yet — no buildfile found at .parlay/build/upgrade-plan-creation/buildfile.yaml. Run /parlay-build-feature @upgrade-plan-creation first.
System (background): Runs `parlay diff @upgrade-plan-creation` to classify components as stable / dirty / removed.
System (background): Runs `parlay scan-generated cmd/upgrade-plan-creation/` to map existing files to their owning components via the `parlay-component:` marker. Files without a marker are user-owned and excluded.
System (background): Runs `parlay verify-generated @upgrade-plan-creation` to detect any hand-edits to files the diff considers stable.
System (condition: first generation): No code state committed yet (`has_hashes: false`) — treating every component as new and regenerating the full prototype.
System (condition: stable file modified): I noticed ==filename== is marked as a stable component but has been edited since the last generation. How would you like to proceed?
  A: Overwrite (lose my edits)
  B: Skip this file (keep my edits, possibly diverging from the buildfile)
  C: Show me the diff first
System (condition: incremental): Plan:
System: - ==N== stable component files (will be preserved)
System: - ==M== dirty component files (will be regenerated): ==list==
System: - ==K== removed component files (will be deleted): ==list==
System: Generating prototype code with adapter: go-cli
System: Source root (from adapter): cmd/upgrade-plan-creation/
System (background): For each dirty/new component in buildfile, generates a code file at the adapter's file-conventions location. Each generated file gets a two-line `parlay-feature:` / `parlay-component:` marker. Stable components are skipped (their existing files stay untouched). Removed components have their marker-tagged files deleted.
System (background): Generates test code from .parlay/build/upgrade-plan-creation/testcases.yaml and runs the test suite.
System (condition: tests fail): Tests failed:
System: - ==test name== — ==failure summary==
System: NOT committing build state. The previous committed state is preserved; you can fix the issue and re-run /parlay-generate-code.
  A: Show me the failures in detail
  B: Regenerate the failing components
  C: Stop, I'll investigate manually
System (condition: tests pass): All tests pass.
System (background): Runs `parlay save-build-state @upgrade-plan-creation --source-root cmd/upgrade-plan-creation/` — atomically writes both .baseline.yaml and .code-hashes.yaml using write-then-rename. This is the only sanctioned write path for either file.
System: [OK] Prototype generated and tests passing. Build state committed.
System: Generated ==number== code files:
System: - cmd/upgrade-plan-creation/main.go
System: - cmd/upgrade-plan-creation/preflight.go
System: - cmd/upgrade-plan-creation/upgrade_prompt.go
System (background): Reads .parlay/build/upgrade-plan-creation/testcases.yaml and generates test code.
System (background): Runs generated tests against the prototype.
System (condition: tests pass): [OK] All tests pass. Prototype is ready to run.
System (condition: tests fail): ==number== test(s) failed:
System: - ==test name== — ==failure summary==
System: How would you like to proceed?
  A: Show me the failures in detail
  B: Regenerate the failing components
  C: Stop, I'll investigate manually

#### Branch: No Agent (CLI fallback)

User: parlay generate-code @upgrade-plan-creation
System (background): No agent available. The CLI cannot generate code on its own.
System: generate-code requires an AI agent. Use the /parlay-generate-code skill in your AI agent (e.g., Claude Code).

---

### Generate Engineering Specification

**Trigger**: /parlay-generate-enggspec @feature-name

User: /parlay-generate-enggspec @upgrade-plan-creation
System: We're ready to hand over to engineering. Let me read through our specifications and generate an engineering spec in GitHub SpecKit format.
System (background): Reads intents, dialogs, surface from spec/intents/upgrade-plan-creation/ and buildfile.yaml + testcases.yaml from .parlay/build/upgrade-plan-creation/. Translates to GitHub SpecKit format.
System: The specification is ready: spec/handoff/upgrade-plan-creation/specification.md
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
System (background): Merges models with resolved decisions.
System: Domain model integrated and saved.

---

### Define Application Blueprint

**Trigger**: Team is ready to define app-level architecture before first code generation

User: I need to set up the blueprint for this project.
System: Let's define how the app is wired together. First, what navigation strategy does the app use?
  A: Browser routing (React, Angular, Vue web apps)
  B: Hash routing (SPAs without server-side routing)
  C: Native tab bar (iOS, Android)
  D: Native stack navigation (iOS, Android)
  E: CLI subcommands (Go CLI, Node CLI)
User: Selects A
System: Got it — browser routing. What's the default route (where "/" redirects to)?
User: /dashboard
System: Now let's define the layout shells. A shell is the persistent chrome (sidebar, header, footer) that wraps a set of pages.
System: How many distinct shells does the app have?
User: Two — one for the main app with a sidebar, and one for auth pages that's just centered content.
System (background): Scaffolds shell definitions from the user's description.
System: Here's what I've drafted:
System: ==shows shells YAML snippet==
System: Which pages does each shell wrap?
User: The app shell wraps dashboard, tasks, settings. The auth shell wraps login and register.
System (background): Maps pages to shells.
System: Do any routes require authentication or specific roles?
User: Everything under the app shell needs login. The users page also needs admin.
System (background): Creates authorization guards and attaches them to routes.
System (condition: features have surfaces with Pages): I can see Page targets from your feature surfaces: ==list==. I'll wire those into the route table.
System (background): Generates the full blueprint.yaml at .parlay/blueprint.yaml.
System (background): Runs `parlay validate --type blueprint --json .parlay/blueprint.yaml`.
System (condition: validation passes): [OK] Blueprint saved to .parlay/blueprint.yaml. All references validated.
System (condition: validation fails): Found ==number== issues:
System: - ==issue description==
System: Let me fix those and try again.
System: You can refine the data, errors, and state sections later. The blueprint is ready for code generation.

#### Branch: Minimal CLI Blueprint

User: This is a CLI app, I just need the basics.
System (background): Creates a minimal blueprint with `navigation.strategy: cli-subcommands` and no shells.
System: [OK] Minimal blueprint saved. CLI apps don't need shells or guards — the blueprint just records the navigation strategy for code generation.

---

### Sync Intents and Dialogs

**Trigger**: /parlay-sync @feature-name

User: /parlay-sync @upgrade-plan-creation
System (background): Runs `parlay check-coverage @upgrade-plan-creation` to get coverage and drift data.
System (background): Runs `parlay collect-questions @upgrade-plan-creation` to check for open questions.
System: Here's the sync report for @upgrade-plan-creation:
System: **Coverage:**
System: - ==intent A== — matched by dialog ==dialog title==
System: - ==intent B== — matched by dialog ==dialog title==
System (condition: uncovered intents exist): **Intents without dialogs:**
System: - ==intent C== — no matching dialog found
System (condition: orphan dialogs exist): **Orphan dialogs** (no matching intent):
System: - ==dialog title== — doesn't trace to any intent
System (condition: open questions exist): **Open questions:** ==number== unresolved across ==number== intents
System (condition: drift detected): **Drift detected:** ==number== intents changed since last build:
System: - ==intent title== — ==fields that changed==
System (condition: drift detected): Let me review the drifted intents against your downstream artifacts.
System (background): Reads surface.md, buildfile.yaml, testcases.yaml for drifted intents.
System (condition: meaningful drift found): I found ==number== downstream artifacts that may need updating:
System: - Surface fragment ==name==: Shows field no longer matches intent Goal
System: - Testcases for ==component==: Verify bullets changed, tests may be stale
System: Want me to help update them?
  A: Yes, walk me through each one
  B: Just flag them, I'll update manually
  C: No, the current artifacts are fine
System (condition: uncovered intents exist): Want me to generate dialog templates for the uncovered intents?
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
