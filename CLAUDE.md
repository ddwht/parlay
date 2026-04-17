# Parlay Project

This project uses the Parlay intent-driven design toolkit.
All operations are available as /parlay-* slash commands.

## Available Commands

- `/parlay-add-feature` — Create a new feature
- `/parlay-build-feature` — Generate buildfile and testcases
- `/parlay-collect-questions` — collect-questions
- `/parlay-create-surface` — Generate surface from intents and dialogs
- `/parlay-extract-domain-model` — Extract domain model from all features
- `/parlay-generate-code` — Generate prototype code from buildfile
- `/parlay-generate-enggspec` — Generate engineering specification
- `/parlay-load-domain-model` — Load and integrate external domain model
- `/parlay-lock-page` — Lock a page layout into a manifest
- `/parlay-onboard` — Onboard existing codebase and draft adapter
- `/parlay-reference-design-spec` — reference-design-spec
- `/parlay-register-adapter` — Register a framework adapter
- `/parlay-scaffold-dialogs` — Scaffold dialog templates from intents
- `/parlay-sync` — Check intent-dialog coverage
- `/parlay-view-page` — Assemble and display a page view

## Schema Loading

Skills load schemas on-demand from .parlay/schemas/. Do not keep schema content in memory across commands.

## Interactive Questions

When a skill step says to "ask the user", "present options", or "wait for the user's response", you MUST use the AskUserQuestion tool to pause execution and collect the user's input before proceeding to the next step. Do not output the question as plain text and continue — the skill requires the user's answer to decide what to do next.

## File Ownership

Three-zone layout — strict ownership:
- **spec/intents/<feature>/** (designer-authored): intents.md, dialogs.md — ask permission before modifying
- **spec/intents/<feature>/** (generated, human-reviewed): surface.md, domain-model.md, *.page.md
- **spec/handoff/<feature>/** (engineering output): specification.md
- **.parlay/build/<feature>/** (tool internals): buildfile.yaml, testcases.yaml, .baseline.yaml — never user-facing
