# Parlay Studio — Architectural Proposal (v4)

> **Status (2026-07-23): historical design proposal.** This document records the v4 Studio design; it is not a description of the shipped system. Since superseded: the doc's references to `surface.md` and `domain-model.md` reflect the markdown-era spec formats. Both have since migrated to YAML — `surface.yaml` (via `parlay migrate-spec`) and `domain-model.yaml` (via `parlay migrate-domain-model`). Where the body says `surface.md` or treats `domain-model.md` migration as an open question, read `surface.yaml` / `domain-model.yaml` as the current formats. Read the body below as design rationale, not current behavior.

A round-trip prototyping toolchain built as an extension on top of Parlay Core.

## What's new in v4

v3 conflated Core and Studio into one product and treated Figma as a state-bearing collaborator. v4 separates them cleanly: Parlay Core stays the developer-facing engine, Parlay Studio becomes a designer-facing extension that adds two tools (Design Loop and Domain Model Editor) on top of Core. Figma is a disposable editor — the source of truth lives in Parlay-side artifacts. Wiring inference is deferred to Core's existing codegen, removing it from the layout iteration loop.

## TL;DR

Parlay Core is the existing engine and stays unchanged in spirit: intents, dialogs, surface, domain model, pages, code generation. Parlay Studio is a separate binary that depends on Core and adds:

- A **Design Loop Tool** that round-trips a page's layout between Parlay's canonical format and Figma, with Figma treated as a disposable editor
- A **Domain Model Editor** that provides a visual, web-based UI for editing the domain model

Studio's two tools share a small ephemeral web server that spins up on demand and shuts down when the work is done. Layout is a typed tree of design-system components, embedded directly in Core's existing page artifact. Wiring is not part of the layout loop — it happens during Core's codegen pass, as it does today.

The product claim:

> Designers iterate on layouts in Figma, edit the domain model in a visual editor, and hand off to Core for code generation. The toolchain stays out of the way — Figma is disposable, the canonical artifacts live in Parlay, and wiring is Core's job.

---

## 1. Scope and Personas

**Parlay Core's persona is the developer.** Spec-driven scaffolding, intent-driven design, generated code. This is unchanged.

**Parlay Studio's persona is the UX designer.** Visual layout editing in Figma, structured domain modeling in a web UI, no CLI fluency required beyond starting Studio's tools. Designer hands off to a developer (or the same person in a different mode) who runs Core for code generation.

Studio is opt-in. Projects without Studio installed see no behavior change in Core. Projects with Studio installed get additional tools but Core's commands work the same.

---

## 2. Architecture Overview

```
                        Parlay Core (binary)
   ┌──────────────────────────────────────────────────────────┐
   │  intents → dialogs → surface → pages → code generation   │
   │                                  ▲                        │
   │                                  │                        │
   │                          domain-model.yaml                │
   └──────────────────────────────────────────────────────────┘
                                  ▲
                                  │ depends on
                                  │
                        Parlay Studio (binary)
   ┌──────────────────────────────────────────────────────────┐
   │                                                          │
   │   Design Loop Tool ◄──► Figma (via MCP)                  │
   │   (round-trips layout)                                   │
   │                                                          │
   │   Domain Model Editor                                    │
   │   (visual editor over domain-model.yaml)                 │
   │                                                          │
   │   Shared web server harness (ephemeral, on-demand)       │
   │                                                          │
   └──────────────────────────────────────────────────────────┘
```

Studio is invoked through hooks on Core's CLI. When the user is at a point in Core's workflow where layout or domain-model work is appropriate, Core prompts "open editor?" If yes, Studio's binary takes over, the web server starts (if needed), the designer works, the result is written to disk, control returns to Core.

The artifacts on disk are owned by Core. Studio reads and writes them but doesn't own a separate format. This means:

- A non-Studio user opens a Parlay project authored with Studio: it works, the page artifacts and domain model are valid Core artifacts
- A non-Studio user edits the domain model by hand: it works, Studio reads the same file later
- A Studio user works on a Parlay project that wasn't authored with Studio: it works, Studio generates the additional content (Figma-managed layouts) on demand

---

## 3. Core Principles

**P1. Core/Studio separation is hard.** Core is a binary, Studio is a separate binary that depends on Core. They share schemas via Core's published types but ship and version independently. Studio has no privileged access to Core internals.

**P2. Source of truth is Parlay-side, always.** Figma is a disposable editor. The canonical layout for a page lives in Core's page artifact. Figma frames are generated from that canonical state and read back into it; Figma never holds state Parlay doesn't already have.

**P3. Wiring is not part of the layout loop.** Layout iteration in Figma should be fast. AI-driven wiring inference happens at codegen time, not on every Figma round-trip. This is the most important separation in the architecture.

**P4. Design-system-bound layout vocabulary.** Layout artifacts reference a specific design-system component vocabulary (e.g., Clarity v17). Cross-design-system migration is not a goal. Cross-framework migration within the same design system works because the vocabulary is shared.

**P5. Studio extends Core's existing artifacts, doesn't introduce new ones.** Layout content is embedded in Core's page schema. Domain model is Core's existing domain model. Studio adds tooling, not new file formats.

**P6. Ephemeral, hook-driven UI.** Studio's web server starts when needed, serves the editor, shuts down when work is complete. No always-on daemon. No installed UI requiring updates.

---

## 4. Artifacts

### 4.1 Reused from Parlay Core (mostly unchanged)

- `intents.md`, `dialogs.md`, `surface.md` — read as-is by both Core and Studio
- `domain-model.yaml` — Core's existing artifact. Format moves to YAML (machine-friendly), versioned schema, edited primarily by Studio's Domain Model Editor
- `pages/<page-id>.md` — Core's existing page schema, with one optional addition (§4.3)

### 4.2 Domain Model — `domain-model.yaml`

Machine-friendly YAML. Edited primarily through Studio's editor. Hand-editing is supported but not the primary workflow.

```yaml
schemaVersion: 0.1

enums:
  TaskStatus:
    values:
      todo:         { label: To do,        tone: neutral }
      in_progress:  { label: In progress,  tone: info    }
      blocked:      { label: Blocked,      tone: danger  }
      done:         { label: Done,         tone: success }
  Priority:
    values:
      low:    { label: Low,    tone: neutral }
      medium: { label: Medium, tone: neutral }
      high:   { label: High,   tone: warning }

entities:
  Task:
    fields:
      id:         { type: uuid, required: true }
      title:      { type: string, required: true }
      status:     { type: TaskStatus, required: true }
      priority:   { type: Priority, required: true }
      assigneeId: { type: ref, target: User, required: false }
      createdAt:  { type: datetime, required: true }
  User:
    fields:
      id:   { type: uuid, required: true }
      name: { type: string, required: true }

relationships:
  Task.assignee:
    type: many-to-one
    from: Task.assigneeId
    to:   User.id

operations:
  createTask:
    label: Create task
    input:  [title, priority, assigneeId]
    effects: [create Task with status = todo]
  updateTaskStatus:
    label: Update status
    input:  [taskId, status]
    effects: [set Task.status = input.status]
```

Enum presentation metadata (`label`, `tone`) lives here as a deliberate exception so it isn't redeclared per page.

### 4.3 Pages — `pages/<page-id>.md` (Core's existing schema, extended)

Core already has page artifacts. Studio extends them with an optional `layout:` field carrying the canonical layout for that page. Pages without `layout:` work in Core exactly as they do today; pages with `layout:` get used by Core's codegen.

```markdown
---
schemaVersion: 0.1
id: task-list
route: /tasks
surface: tasks/list-tasks
---

# Task list

## Layout

```yaml
componentVocabulary: clarity@17
schemaVersion: 0.1

nodes:
  - id: page-region
    type: clarity.region
    direction: vertical
    gap: spacing-lg
    padding: spacing-xl
    alignment: stretch
    children:
      - id: header-region
        type: clarity.region
        direction: horizontal
        gap: spacing-md
        alignment: space-between
        children:
          - id: page-title
            type: clarity.heading
            level: page
            text: Tasks

          - id: create-button
            type: clarity.button
            variant: primary
            label: Create task
            icon: plus

      - id: task-grid
        type: clarity.datagrid
        density: normal
        selectionMode: none
        children:
          - id: col-title
            type: clarity.datagrid-column
            headerLabel: Title
            contentShape: text-long
            width: fill
            sortable: true

          - id: col-status
            type: clarity.datagrid-column
            headerLabel: Status
            contentShape: badge
            width: md
            sortable: true

          - id: col-priority
            type: clarity.datagrid-column
            headerLabel: Priority
            contentShape: badge
            width: sm
            sortable: true

          - id: col-assignee
            type: clarity.datagrid-column
            headerLabel: Assignee
            contentShape: text-short
            width: md
            sortable: true
```
```

Three things to note about the layout format:

**Typed tree of design-system components.** Each node has a `type` from the design-system vocabulary declared at the top. Properties are typed per component. Layout containers (`clarity.region`) carry layout parameters (gap, padding, alignment, direction). Spacing values are token names (`spacing-lg`), not raw values.

**Component vocabulary is declared explicitly.** `componentVocabulary: clarity@17` makes the dependency on a specific design system version explicit. A Studio binary configured for a different vocabulary fails fast on read.

**Embedded in the page artifact, not a separate file.** Designer/developer parallel editing happens through different sections of the same Markdown file. This is consistent with how Core's pages already work.

**Schema version present from day one.** Future format changes can be migrated mechanically.

**No wiring information in layout.** No data sources, no operation references, no expressions. Wiring is Core's job at codegen time, derived from surface + domain + layout.

### 4.4 What's Not An Artifact

Things that are *not* persistent artifacts in v4:

- **Figma frames.** Generated on demand for editing, deleted when work is complete. No URL stored anywhere on the Parlay side.
- **Session state for in-flight Figma edits.** No `.parlay-studio/sessions.yaml` or equivalent. Each edit is a single round-trip: generate, edit, sync back. Resumability across sessions is deferred (see Open Questions).
- **Component identity mappings.** During a single round-trip, Studio holds the Figma-node-ID-to-canonical-node-ID mapping in memory. After sync-back, the mapping is discarded. New round-trips re-derive identity.

---

## 5. The Design Loop Tool

This is Studio's primary contribution. It manages the round-trip between Parlay's canonical layout and Figma.

### 5.1 The flow

```
User runs `parlay-studio design-loop edit <page-id>`
        │
        ▼
Studio reads canonical layout from page artifact
        │
        ▼
Studio walks the typed tree, emits MCP commands to instantiate
Clarity components in a new Figma frame, recording Figma-node-ID
↔ canonical-node-ID mapping in memory
        │
        ▼
Studio outputs Figma URL to terminal, opens it in browser
        │
        ▼
Designer edits in Figma — moves nodes, changes variants, edits text,
adjusts spacing tokens, adds/removes components from the vocabulary
        │
        ▼
User runs `parlay-studio design-loop sync <page-id>`
        │
        ▼
Studio reads frame back via MCP, gets structured component data
        │
        ▼
Studio matches Figma nodes to canonical nodes using the in-memory
mapping (or structural matching for new nodes), translates back to
canonical format
        │
        ▼
Studio writes updated layout into page artifact
        │
        ▼
Studio deletes the Figma frame (or marks it for deletion)
        │
        ▼
Steady state: canonical layout is updated; no Figma-side state remains
```

The "in-memory" qualifier matters: the mapping is held by the running Studio process. If the process exits between edit and sync, the mapping is lost and sync falls back to structural matching. This is the trade-off accepted in §4.4: simpler architecture, no resumability across process boundaries.

### 5.2 Constraints on what Figma editing can express

The format is design-system-bound and uses tokens, not raw values. This means Figma edits that violate the vocabulary fail at sync time with a clear error. Specifically:

**Allowed Figma edits:**
- Move nodes within and between containers
- Change component variants (Clarity provides them)
- Change spacing tokens for gaps, padding
- Edit text content
- Add new instances of components in the vocabulary
- Remove components (with warning if they were in canonical)
- Reorder children

**Not allowed (sync refuses with warning):**
- Components not in the vocabulary (free-form vector layers, custom non-Clarity components)
- Absolute positioning that doesn't map to container semantics
- Raw pixel spacing values that don't match a token
- Complex grid layouts that don't decompose into rows-of-columns

The "refuse and warn" behavior is by design. The whole point of design-system binding is that the format constrains what's expressible. A designer wanting capabilities outside the vocabulary is asking for a design-system change, which is a different conversation.

### 5.3 Why no HTML in the pipeline

Figma's MCP supports an HTML-rendering path (send HTML, Figma creates editable design layers). v4 does *not* use this path because:

- HTML-rendered design layers in Figma are not Code Connect-bound component instances. Read-back loses component identity.
- Multi-iteration round-trips become unstable because Figma replaces or duplicates rendered HTML rather than updating component instances in place.
- The HTML representation loses semantic information that the typed tree carries (e.g., variants become CSS classes, which Figma can't introspect back to variants).

Instead, Studio drives MCP's component-instantiation commands directly. Each canonical node becomes a real Clarity component instance in Figma, with Code Connect bindings preserved. This is more dependent on MCP's component-write capabilities being mature, which is why §10 lists MCP write API stability as a Phase 0 spike target.

### 5.4 Fallback if MCP component-instantiation isn't viable

If the Phase 0 spike shows MCP's component-write API isn't mature enough to drive layouts at scale, the fallback is:

- Designers author Figma layouts directly using the design system's pre-built Code Connect-mapped library (no Studio-driven generation)
- Studio reads layouts via MCP `get_design_context` and `get_code_connect_map`, no writing
- Initial round-trip becomes "designer creates frame, runs `parlay-studio design-loop sync --from <url>`, Studio creates canonical layout"
- Subsequent rounds don't auto-generate frames; designer manages frames manually

This is a smaller product but still useful. The fallback is documented so the architecture has a recovery path.

---

## 6. Domain Model Editor

A web-based visual editor for `domain-model.yaml`. Spins up via `parlay-studio domain-edit`, opens browser, edits in place, shuts down on idle.

### 6.1 Capabilities

- **Entity editor:** create/edit/delete entities, manage typed fields, mark required/optional, add mock generation hints
- **Enum editor:** define enums, edit values with presentation metadata (label, tone)
- **Relationship editor:** visual ER diagram showing relationships between entities; create relationships by drawing connections
- **Operation editor:** define operations with input fields and effects
- **Live validation:** invalid states (orphan refs, missing required fields, type errors) surface in the UI

### 6.2 What it does *not* do

- No domain reasoning beyond schema correctness
- No domain inference from existing code (Core's `/parlay-extract-domain-model` already handles that and stays in Core)
- No mock data generation (Core generates mocks at codegen time from the model)

### 6.3 Lifecycle

```
User runs `parlay-studio domain-edit`
        │
        ▼
Studio's web server starts on a free local port
        │
        ▼
Browser opens to the editor UI, loads current domain-model.yaml
        │
        ▼
Designer edits, clicks save (or auto-save fires)
        │
        ▼
Server writes updated YAML to disk
        │
        ▼
Server idle for N minutes → shuts down
        │
        ▼
Browser tab shows "session ended, run `parlay-studio domain-edit` to resume"
```

Idle timeout is the right trade-off for a designer persona. Explicit start/stop commands are too CLI-native; always-on daemon is too heavyweight.

---

## 7. The Web Server Harness

Both tools share infrastructure:

- HTTP server bound to a free local port, opened in browser automatically
- File I/O abstraction that writes to Parlay project paths
- MCP client used by Design Loop Tool
- Common authentication-against-Figma flow (OAuth or token-based, depending on MCP server requirements)
- Common UI shell (header, save indicator, error display)

The harness is built once and serves both tools. Future Studio tools (Phase 4+) plug into the same harness.

The server is *ephemeral*: it spins up on tool invocation, shuts down on idle or explicit close. There's no daemon to install, no port to keep open, no service to monitor. This matters for designer onboarding — installing Studio doesn't add a long-running process to the system.

---

## 8. Wiring Inference (Recap)

Wiring lives in Core's codegen, not in Studio. v4's separation is:

- **Studio's job:** produce a canonical layout (typed tree of design-system components) and a domain model
- **Core's job:** at codegen, read intents/dialogs/surface, domain model, page layout, and emit framework-specific code with bindings inferred

This means Studio doesn't need a wiring cache, doesn't need expression languages, doesn't need any AI in its inner loop. The Design Loop is purely structural editing.

Codegen-time wiring is a Core concern. It can use AI inference (matching surface Shows/Actions to layout nodes, matching domain fields to columns), it can use rules (column with `contentShape: badge` and a domain field of an enum type → render as Clarity badge), or both. v4 doesn't prescribe an implementation; it just confirms that Studio is not part of it.

---

## 9. AI Surface

AI participates in Studio at two specific points, both bounded:

- **Initial layout proposal** (Phase 4+): given a page's surface and domain, AI proposes an initial canonical layout. Designer edits in Figma. AI is at authoring time, not in the loop.
- **Sync-back classification** (Phase 1, only if needed): when MCP returns Figma data with components that aren't cleanly mapped, AI may suggest classifications. Designer confirms.

AI does *not* participate in:
- The Design Loop's round-trip mechanics
- Figma frame generation (deterministic from canonical layout)
- Sync-back of in-vocabulary edits (mechanical)
- Domain model editing (pure UI)
- Any always-on cache or inference loop

This is dramatically smaller than v3's AI surface. Most of v3's caching infrastructure was solving problems v4 doesn't have because wiring isn't in Studio's loop.

---

## 10. Phased Plan

### Phase 0 — MCP Spike (~2–3 weeks)

Validate the assumptions v4 builds on. Specifically:

- Can Studio drive MCP component-instantiation reliably to create a real Clarity Figma frame from a typed tree?
- What metadata can Studio attach to component instances (plugin data, naming conventions, etc.) to support read-back identity?
- What's the round-trip fidelity on a real screen (e.g., Tasks list)?
- What rate limits and pricing apply at sustained use?

Output: a go/no-go on the typed-tree round-trip. If go, Phase 1 proceeds. If no-go, Phase 1 implements the fallback in §5.4.

### Phase 1 — Studio MVP (~10–12 weeks)

The smallest version of Studio that exercises both tools end-to-end:

- Studio binary scaffolding, depends on Core
- Hook integration with Core CLI (`parlay-studio` invokable from Core's prompts)
- Web server harness
- Domain Model Editor (entity, enum, field editing — relationships and operations in Phase 2)
- Design Loop Tool (round-trip with one design system, Clarity)
- One adapter (Angular + Clarity)
- Page schema extension to carry `layout:` field
- Sync error reporting (refuse-and-warn for out-of-vocabulary edits)

Phase 1 ships with no resumability for in-flight Figma edits and no AI inference. Both are deferred.

### Phase 2 — Domain Model Editor depth (~3–4 weeks)

Relationship editor (ER diagram), operation editor, mock generation hints in the UI.

### Phase 3 — Multi-adapter validation (~3 weeks)

Second adapter (e.g., react-clarity) to validate the same-design-system framework swap. Then a third adapter with a different design system (e.g., react-antd) to validate the cross-design-system path. Confirm the typed-tree format generalizes across adapters.

### Phase 4 — AI authoring assistance (open scope)

Initial layout proposal from surface + domain. Editor still owns the artifact; AI is upstream.

### Phase 5+ — Deferred features

Items from the open questions list, prioritized by real designer feedback:

- Resumability for in-flight Figma edits (Q21)
- Multi-screen / multi-frame management
- Collaboration patterns (multiple designers, conflict resolution)

---

## 11. What Studio Does and Doesn't Promise

**Promises:**

- Round-trip layout editing in Figma where the canonical artifact lives in Parlay
- Visual domain model editing without leaving the design workflow
- Ephemeral tooling: no daemons, no services, no UI installs requiring updates
- Compatibility with Core's existing artifacts; non-Studio users see no behavior change
- Studio's failures are recoverable: deleting `.parlay-studio/` returns the project to a Core-only state

**Does not promise:**

- Cross-session resumability of in-flight Figma edits (Phase 1 explicitly defers this)
- Pixel-perfect design fidelity (the format is design-system-bound, not pixel-bound)
- Free-form designer creativity in Figma (the vocabulary constrains what's expressible)
- Cross-design-system layout migration (that's a re-author, not a translation)
- Production-grade code generation (that's Core's existing scope, not Studio's)

These are the right trade-offs for a UX prototyping toolchain bolted onto a developer-facing engine.

---

## 12. Reuse and Departure from v3

**Reuses from v3:**
- MCP-based Figma integration (no plugin)
- Design-system-bound layout vocabulary
- Stability-via-disposability principle (canonical truth on Parlay side)
- Code Connect-aware component identity

**Departs from v3:**
- Two binaries (Core, Studio) instead of one product
- Layout embedded in page artifact, not a separate spec file
- No `screens.yaml` registry — pages already exist in Core
- No wiring artifacts, no expression language, no wiring cache
- No `wiring-overrides.yaml` — wiring is Core's codegen concern
- Figma is purely disposable; no session state, no URL persistence
- Round-trip is one edit at a time, in-process; no cross-session resumption in Phase 1

The simplifications come from accepting that wiring is Core's job, that Figma is an editor not a collaborator, and that Studio extends Core rather than replacing it.

---

## 13. Open Questions

The active list. Priority 1 items are Phase 0 / Phase 1 blockers. Priority 2 items are Phase 1+ deferrals.

### Priority 1 — must answer before or during Phase 1

**Q1. Phase 0 spike outcome.** Can MCP drive Clarity component instantiation reliably enough for the typed-tree round-trip? Falls back to §5.4 if no.

**Q2. Component identity persistence in Figma.** What survives copy/paste, regroup, rename? Plugin data, layer naming, attached comments — pick one or more after the spike. (Note: identity mapping is in-memory during a single round-trip; this is about supporting reliable read-back even when designers do unexpected operations.)

**Q3. Spacing/token vocabulary source.** Tokens come from the design system. Where does Studio get them at runtime — adapter config, MCP variables fetch, or hardcoded? Adapter config is simplest; MCP fetch is most accurate. Pick during Phase 1 implementation.

**Q4. MCP plan and pricing.** Phase 1 needs at least one Dev/Full seat on a paid Figma plan. Confirm that's acceptable for the target users (VMware/Broadcom: yes; broader open-source positioning: gate).

**Q5. Hook trigger points.** When does Core prompt "open Domain Model Editor" or "open Design Loop"? After `/parlay-create-domain-model`, after `/parlay-add-feature`, both, neither? Decide during Phase 1 design.

**Q6. Idle timeout duration.** How long does the web server idle before shutting down? Default 30 minutes is a reasonable starting point but should be configurable.

### Priority 2 — defer to Phase 1+ retro

**Q7. Resumability for in-flight Figma edits.** Studio's current model treats each round-trip as atomic and in-process. Real designers may need to step away mid-edit. If feedback shows this is blocking, add ephemeral session state (`.parlay-studio/sessions.yaml`) recording Figma URL, timestamp, and Figma-node-ID-to-canonical-node-ID mapping. Keep separate from canonical truth.

**Q8. Multi-screen management.** When a feature has many screens, does Design Loop generate frames into one Figma file or many? How does Studio refer to them? Defer until Phase 1 surfaces real multi-screen workflows.

**Q9. Collaboration patterns.** What happens when two designers edit the same canonical layout? Studio in Phase 1 is single-user; conflict resolution is git's job. Real teams may need richer behavior.

**Q10. Domain Model Editor migration from existing markdown.** Core's existing `domain-model.md` is markdown. v4 moves it to YAML. Migration path: one-shot conversion script, run during Studio install? Or grace period with both formats supported? Decide during Phase 1.

**Q11. Adapter version migration.** A project pinned to Clarity v17 today and Clarity v18 tomorrow has layout files referencing the old vocabulary. How does Studio handle the upgrade? Manual review with migration warnings is acceptable for Phase 1.

**Q12. Studio UI design system.** The Domain Model Editor needs its own UI components (forms, ER diagram, etc.). Use Clarity? Use a separate library to avoid coupling Studio to Clarity's release cycle? Defer until Phase 1 implementation begins.

**Q13. Studio configuration.** Where does Studio's config live (`.parlay-studio/config.yaml`?), what's in it (Figma file/team URL, MCP server, idle timeout, etc.)? Define during Phase 1.

**Q14. Internationalization of generated layouts.** Static strings in layout (`label: Tasks`) are inlined. i18n is acknowledged as a known constraint. Real i18n migration path is deferred.

---

## 14. What This Architecture Is and Isn't

**Is:**
- An extension architecture that adds designer-facing tools without changing Core's contract
- A round-trip layout editor that uses Figma without depending on Figma for state
- A canonical-truth-on-our-side product where deleting Studio doesn't break the project

**Isn't:**
- A replacement for Parlay Core
- A general-purpose Figma-to-code product
- A pixel-perfect design tool
- A free-form layout editor (the vocabulary constrains expression deliberately)
- Compatible with arbitrary Figma authoring (it's compatible with Figma authoring against a design-system-bound vocabulary)

These are the right trade-offs for a designer-facing extension on a developer-facing engine. They are the wrong trade-offs for a standalone designer tool.

---

## Appendix — Concrete Example: Tasks Screen End-to-End

To make the abstractions concrete, here's the Tasks screen as actual artifacts.

**Domain model** (excerpt from `domain-model.yaml`):

```yaml
schemaVersion: 0.1
entities:
  Task:
    fields:
      id: { type: uuid, required: true }
      title: { type: string, required: true }
      status: { type: TaskStatus, required: true }
```

**Surface** (from Core's existing `surface.md`, unchanged):

```
## Task List
Shows: data-list, empty-state
Actions: navigate-drill, invoke-create
Source: @tasks/list-tasks
```

**Page with embedded layout** (`pages/task-list.md`):

```markdown
---
schemaVersion: 0.1
id: task-list
route: /tasks
surface: tasks/list-tasks
---

# Task list

## Layout

```yaml
componentVocabulary: clarity@17
schemaVersion: 0.1
nodes:
  - id: page-region
    type: clarity.region
    direction: vertical
    children:
      - id: task-grid
        type: clarity.datagrid
        children:
          - id: col-title
            type: clarity.datagrid-column
            headerLabel: Title
            contentShape: text-long
          - id: col-status
            type: clarity.datagrid-column
            headerLabel: Status
            contentShape: badge
```
```

**Design Loop round-trip:**
1. `parlay-studio design-loop edit task-list` → Studio reads page, instantiates Clarity Datagrid + 2 columns in a fresh Figma frame, opens browser
2. Designer adjusts column widths, changes density, adds a "Priority" column
3. `parlay-studio design-loop sync task-list` → Studio reads back, validates against Clarity vocabulary, writes updated layout into page artifact, deletes Figma frame

**Codegen** (Core, unchanged):
- `parlay generate-code` reads page (with layout), surface, domain model
- Infers wiring (col-title → Task.title, col-status → Task.status with TaskStatus presentation, etc.)
- Emits Angular + Clarity component code

This is the entire loop. No `screens.yaml`, no `wiring-spec.yaml`, no `layout-spec.yaml` separate from the page, no expression language, no cache directory, no AI in the round-trip. Studio extends; Core does what it already does.
