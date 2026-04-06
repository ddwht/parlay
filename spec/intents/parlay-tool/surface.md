# Intent Design Tool — Surface

---

## Project Setup Wizard

**Shows**: Step-by-step selection flow — AI agent choice, SDD framework choice, prototype framework choice — each with a list of available options.
**Actions**: Select option from list, confirm selection, skip optional steps
**Source**: @parlay-tool/configure-project-tools

---

## Feature Scaffold Confirmation

**Shows**: Created feature folder path, list of generated files (intents.md, dialogs.md), and the next step command (/parlay create-dialogs).
**Actions**: Copy next-step command
**Source**: @parlay-tool/author-intents

---

## Dialog Template Report

**Shows**: Number of intents found, number of dialog templates generated, list of template titles with placeholder status.
**Actions**: Open dialogs.md for editing
**Source**: @parlay-tool/scaffold-dialogs-from-intents

---

## Disambiguation Prompt

**Shows**: Quoted excerpt of the ambiguous dialog or intent, description of the ambiguity, lettered options (A/B/C) with descriptions, and a freeform input option.
**Actions**: Select an option, provide custom input, approve or reject proposed updates to source files
**Source**: @parlay-tool/resolve-ambiguities-through-ai-dialogue

**Notes**:
- Reusable pattern — appears during create-surface, load-domain-model, and any command that encounters ambiguity
- Always asks permission before modifying human-owned files

---

## Surface Fragment List

**Shows**: Number of generated fragments, each with name and brief description of what it shows.
**Actions**: Open surface.md for review, add page/region targets
**Source**: @parlay-tool/generate-surface-from-intents-and-dialogs

---

## Figma Import Split

**Shows**: Extracted Figma components grouped by detected feature, with component names and descriptions.
**Actions**: Confirm suggested feature assignments, reassign manually, assign all to a single feature
**Source**: @parlay-tool/generate-surface-from-figma

**Notes**:
- Only shown when a Figma design covers multiple features
- Single-feature designs skip directly to Surface Fragment List

---

## Page Assembly View

**Shows**: Assembled page layout organized by region, with ordered fragment references from all contributing features. Flags conflicts (same region + same order) and lists unplaced fragments separately.
**Actions**: Resolve ordering conflicts, assign unplaced fragments to page/region
**Source**: @parlay-tool/view-assembled-page

---

## Page Lock Confirmation

**Shows**: The assembled page layout to be locked, target file path for the page manifest.
**Actions**: Confirm lock, assign page owner, set status (draft/reviewed/locked)
**Source**: @parlay-tool/lock-page-layout

---

## Adapter Registration Confirmation

**Shows**: Adapter name, number of component types, layout patterns, file conventions. Confirmation that the adapter was saved to .parlay/adapters/.
**Actions**: Set as project framework
**Source**: @parlay-tool/register-framework-adapter

---

## Build Progress Report

**Shows**: Active framework adapter name, created devspec files (buildfile.yaml, testcases.yaml) with paths, prototype generation status, test execution results.
**Actions**: Review generated prototype, review buildfile, re-run build
**Source**: @parlay-tool/generate-prototype

---

## Engineering Spec Output

**Shows**: Path to generated engineering specification, SDD framework used for generation.
**Actions**: Open specification for review, hand off to engineering team
**Source**: @parlay-tool/generate-engineering-specification

---

## Domain Model Export

**Shows**: Path to saved domain model file, summary of extracted entities, relationships, and state machines.
**Actions**: Share model file with team, load into another project
**Source**: @parlay-tool/extract-and-share-domain-models

---

## Domain Model Integration

**Shows**: Integration result — either clean confirmation or list of conflicts requiring disambiguation. For conflicts: the conflicting object name, integration options with descriptions.
**Actions**: Select integration strategy, provide custom mapping
**Source**: @parlay-tool/extract-and-share-domain-models

**Notes**:
- Uses Disambiguation Prompt pattern when conflicts exist
- Clean integrations show a simple confirmation with no interaction needed

---

## Coverage Report

**Shows**: Three lists — covered intents (with matched dialog), uncovered intents (no dialog), and orphan dialogs (no matching intent). Counts for each category.
**Actions**: Generate dialog templates for all uncovered intents, select specific intents for template generation, dismiss report
**Source**: @parlay-tool/sync-intents-and-dialogs
