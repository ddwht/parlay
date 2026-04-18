# Project Setup — Dialogs

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
System (background): Creates project structure (three zones — designer input, engineering output, tool internals).
System: The project is bootstrapped and ready. All operations are available as /parlay-* skills.

---

### Register Framework Adapter

**Trigger**: /parlay-register-adapter ==path-to-adapter==

User: /parlay-register-adapter ./adapters/go-cli.adapter.yaml
System (background): Reads and validates the adapter definition.
System: Registered framework adapter "go-cli":
System: - Component types: ==number==
System: - Layout patterns: ==number==
System: - File conventions: ==source-root==
System (background): Copies adapter to .parlay/adapters/
System: The adapter is now available for build-feature.

---

### Define Application Blueprint

**Trigger**: Team is ready to define app-level architecture before first code generation

User: I need to set up the blueprint for this project.
System: Let's define how the app is wired together. First, what navigation strategy does the app use?
  A: Browser routing (web apps)
  B: Hash routing (SPAs without server-side routing)
  C: Native tab bar (mobile)
  D: Native stack navigation (mobile)
  E: CLI subcommands
User: Selects A
System: Got it — browser routing. What's the default route?
User: /dashboard
System: Now let's define the layout shells. A shell is the persistent chrome that wraps a set of pages.
System: How many distinct shells does the app have?
User: Two — one for the main app with a sidebar, and one for auth pages.
System (background): Scaffolds shell definitions, maps pages, creates guards.
System (background): Generates .parlay/blueprint.yaml and validates it.
System: [OK] Blueprint saved. You can refine the data, errors, and state sections later.

#### Branch: Minimal CLI Blueprint

User: This is a CLI app, I just need the basics.
System (background): Creates a minimal blueprint with `navigation.strategy: cli-subcommands`.
System: [OK] Minimal blueprint saved.

---
