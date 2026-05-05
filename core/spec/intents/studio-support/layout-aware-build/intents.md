# Layout-Aware Build

> When `parlay build-feature` runs against a feature whose pages carry `layout:` blocks, it resolves the binding for every layout node — which entity feeds the grid, which operation a button invokes, which presentation hint applies — and writes those resolved bindings into the buildfile. Codegen later consumes those bindings mechanically. This feature is the upstream counterpart to *Layout-Aware Code Generation*: the build phase is where AI inference, rule-matching, and disambiguation prompts live, so codegen never has to make those decisions.
>
> **Why binding resolution lives in build, not codegen.** Layout is intentionally wiring-free — its `clarity.datagrid` and `clarity.button` nodes do not encode which domain entity or operation they bind to. Three artifacts converge here: surface (`Shows` / `Actions`), domain (entities / operations), and layout (typed tree). The build phase is the single point in the pipeline where all three are read together; it is where binding decisions belong. Putting decisions here means: (a) the buildfile becomes the contract between authoring/build and codegen, (b) codegen has no AI-driven decisions, only AI-driven text emission, (c) re-running codegen against an unchanged buildfile produces behaviorally-equivalent output without re-prompting, (d) Studio's Design Loop stays purely structural — no inference creeps back into authoring.

---

## Run a Two-Pass Binding Resolution During Build

**Goal**: For every layout node on every layout-bearing page in a feature, resolve a single binding — the `(layout-node, surface-fragment, domain-element)` source triple plus any presentation hints — using two ordered passes: deterministic rules first, AI matcher second. Disambiguation prompts only when both passes leave ambiguity.
**Persona**: UX Designer
**Priority**: P0
**Context**: The build phase reads surface, domain, and layout together. Most bindings are reachable from structural hints in the layout (e.g., `contentShape: badge` on a column maps to the matching surface field) or from matching surface verbs to domain operations. The remaining cases — overlapping action verbs, multiple domain operations of the same shape — need either AI judgment or a designer's call.
**Action**: For each layout node that consumes data or emits an action, the build agent runs **Pass 1** against the rule set (declared in the buildfile under `wiring.rules`, with starter rules contributed by *Define a Starter Rule Set, Extensible Per Project*). If Pass 1 produces exactly one match, the binding is recorded. If Pass 1 produces zero or multiple matches, **Pass 2** invokes an AI matcher over `(layout-node, surface-fragment, domain-element)` candidates and proposes a single best match. If Pass 2 also leaves ambiguity, control transfers to *Raise an Interactive Disambiguation Prompt*. The resolved binding is written to the buildfile alongside its source triple; AI work happens only at build time, never at codegen time.
**Objects**: build-feature, binding, layout-node, surface-fragment, domain-element, rules-engine, ai-matcher, source-triple

**Constraints**:
- Pass 1 and Pass 2 are ordered — Pass 2 never runs on a node that Pass 1 resolved unambiguously
- The rule set used for Pass 1 is the union of the starter set plus any project-specific rules declared under `wiring.rules` in the buildfile; the build agent does not run rules from other sources
- Pass 2's AI matcher receives the full set of candidates that Pass 1 narrowed to (or, for orphan nodes, the empty set); it never invents candidates outside the candidate space derived from surface and domain
- Inference is bounded to the active feature — the build agent does not pull in surface fragments or domain operations from other features
- Every resolved binding includes a `confidence: rules | ai | designer` annotation so downstream tooling can distinguish deterministic from inferred bindings
- A node with no candidates after Pass 1 (zero matches) goes to Pass 2 only if the layout shape suggests a binding is expected — otherwise it is reported as an `orphan-layout-node` build-time error rather than asking AI to invent a binding
- Build-time inference is the only place AI participates in the binding decision; codegen reads the buildfile and never re-litigates

**Verify**:
- A Tasks page with a `Task` entity (status enum), a layout containing a `clarity.datagrid` whose status column declares `contentShape: badge`, and a `clarity.button` labeled "Create task" produces a buildfile with all bindings resolved by Pass 1 — no Pass 2 invocations recorded — because the structural-hint and action-verb starter rules cover both cases
- A page with a button labeled "Save" where two domain operations (`updateTask` and `saveDraft`) match the surface action triggers Pass 2; the buildfile records a `confidence: ai` annotation and the chosen domain element
- A page with a button labeled "Process" where Pass 2 returns two equally-likely candidates triggers the disambiguation prompt described in *Raise an Interactive Disambiguation Prompt*
- A layout node with no surface fragment to feed it produces an `orphan-layout-node` error during build, not a phantom binding
- Re-running `parlay build-feature` on identical inputs records bindings whose source triples are stable (the same layout-node and surface-fragment), even though Pass-2 lexical text in any AI call may differ — the recorded triple identifies the decision deterministically

---

## Define a Starter Rule Set, Extensible Per Project

**Goal**: Ship a small, named set of deterministic binding rules that handle the common cases out of the box, and let projects add their own rules without modifying core. The starter set is the floor; extensibility is the ceiling.
**Persona**: UX Designer
**Priority**: P0
**Context**: Pass 1's value depends on the rule set being meaningful enough that Pass 2 (AI) is reserved for genuinely ambiguous cases. Too small a starter set forces every binding through AI; too opinionated a starter set blocks projects from expressing their own conventions. The rule set is data, not code, so it sits in the buildfile and projects can extend it.
**Action**: Author a starter rule set covering the common cases: structural-hint matches (`contentShape` on a column → matching Show field), action-verb matches (button label or aria semantics + surface `Action` → domain operation), single-candidate matches (when surface declares one Action and domain has exactly one operation of that shape, the binding is unambiguous). Define a rule schema so projects can add rules of the same shape under `wiring.rules` in the buildfile. The build agent merges starter + project rules at run time.
**Objects**: rules-engine, starter-rules, project-rules, wiring-rules-section, rule-schema

**Constraints**:
- The starter rule set is finite and enumerated — *not* an open-ended pattern matcher. New starter rules are vocabulary changes (require a build-feature schema bump)
- The rule schema is a closed set of fields: `match` (predicate over layout-node properties + surface fragment fields + domain element shape), `bind` (the source triple to record), `precedence` (integer; higher wins on conflict), `confidence` (always `rules`)
- Projects add rules under `wiring.rules:` in the buildfile — never inline in surface, domain, or layout; the rule set is build-time concern, not authoring concern
- Rule conflicts (two rules matching the same node with different bindings) are resolved by `precedence`; ties are a build-time error so projects must disambiguate explicitly
- A project rule's precedence cannot lower-bound a starter rule below it — projects can override but not silently disable starter rules
- Rules must terminate — recursive rule references and rules that match their own output are detected and rejected at build-time
- The starter set is the same across all projects; project extensions are project-scoped

**Verify**:
- A project with no `wiring.rules:` block uses only the starter rules and resolves the Tasks-page example end-to-end via Pass 1
- A project that adds a rule mapping `tone: clarity.badge.tone` produces bindings where the new rule fires on eligible nodes; the buildfile records `confidence: rules` and the rule name
- Two project rules matching the same node with the same precedence cause a build-time error naming both rule definitions
- A starter rule and a project rule matching the same node with the project rule at higher precedence record the project rule's binding; the buildfile annotates which rule fired so the choice is auditable
- A rule whose `match` predicate references a domain field that does not exist fails build at rule-load time, not at match time, so authoring errors surface immediately

---

## Raise an Interactive Disambiguation Prompt When Both Passes Leave Ambiguity

**Goal**: When Pass 1 produces multiple candidates and Pass 2 cannot pick a single best match with high confidence, the build agent surfaces a disambiguation prompt to the designer and records their choice as the binding. The prompt is the only path that can introduce designer judgment into a binding; codegen never prompts.
**Persona**: UX Designer
**Priority**: P0
**Context**: Some bindings are genuinely ambiguous from the spec — two operations match the same button, two surface fragments could feed the same node. AI can guess, but a guess without designer confirmation creates silent risk. The interactive prompt is the contract: when no rule and no high-confidence AI match exists, the designer chooses, and the choice is recorded.
**Action**: When Pass 2 returns multiple candidates whose AI-assigned confidence values are within a configurable threshold, the build agent pauses, prints the page path, the layout-node path inside it, and the candidate list, then waits for the designer's selection. The selection is recorded in the buildfile with `confidence: designer` and a timestamp; the binding survives subsequent `parlay build-feature` runs as long as the underlying layout, surface, and domain do not change in a way that invalidates the choice.
**Objects**: disambiguation-prompt, designer-decision, candidate-list, recorded-choice, build-feature

**Constraints**:
- The prompt format names the file, the layout-node path inside that file (e.g., `pages/tasks.md > region/toolbar/button[2]`), the human label of the node (e.g., "Process"), and the candidate list with stable numeric selectors `[1]`, `[2]`, ...
- The prompt accepts numeric selection, a `[q] quit` escape (which aborts the build with a non-zero exit), and an optional `[s] skip` that records the binding as `unresolved` and continues the build (the resulting buildfile fails its own validity check, but lets the designer collect multiple decisions before re-running)
- A recorded designer choice persists across re-runs as long as the candidate list is identical; if the candidates change (a new domain operation appears, an existing one is renamed) the choice is invalidated and the prompt re-surfaces
- Recording a choice writes to the buildfile, never to the layout, surface, or domain — designer decisions are build-time state, not authoring artifacts
- The prompt is interactive only — headless build (CI) takes the path described in *Headless Build for CI* and never prompts
- Multiple ambiguities in one build run produce a sequence of prompts in deterministic order (page path, then node path) — the designer sees them one at a time, not as a batch

**Verify**:
- A page with one ambiguous binding triggers exactly one prompt; selecting `[1]` records the first candidate with `confidence: designer` and continues the build
- Re-running build immediately after a prompt selection completes without re-prompting; the recorded choice is read from the buildfile
- Editing the layout to add a node that has its own ambiguity produces a fresh prompt for the new node only — the previously recorded choice is preserved
- Renaming the chosen domain operation (e.g., `processTask` → `processTaskItem`) invalidates the recorded choice and re-triggers the prompt with the updated candidate list
- Pressing `[q]` during a prompt aborts the build with a non-zero exit code and writes no partial buildfile
- A build with three ambiguous bindings on three different pages produces three prompts in `(page-path, node-path)` lexicographic order

---

## Record Resolved Bindings in the Buildfile with Traceability Triples

**Goal**: The buildfile is the durable record of every binding decision the build phase made. Each binding carries enough information that codegen can emit it, downstream tools can audit it, and a designer can ask "why did this column render as a badge?" and get a complete answer.
**Persona**: UX Designer
**Priority**: P0
**Context**: Codegen reads the buildfile to wire components; freshness checks compare its source signatures; tooling traces back from generated code to the design intent that drove it. All of these depend on the buildfile carrying the binding data in a stable, parseable shape — and on every binding pointing back to the artifacts that justified it.
**Action**: Extend the buildfile schema with a `bindings:` section keyed by feature → page → layout-node-path. Each entry records: the source triple `(layout-node, surface-fragment, domain-element)`, the presentation hints (e.g., `presentation: badge`, `tone: status-color`), the `confidence: rules | ai | designer` annotation, and (when applicable) the rule name or the prompt session id that produced the choice. Buildfile validity requires every layout-bearing-page node to have an entry.
**Objects**: buildfile, bindings-section, source-triple, presentation-hint, confidence-annotation, rule-name, prompt-session-id

**Constraints**:
- The bindings section is a peer to the existing buildfile sections (`models`, `fixtures`, `routes`, `components`, `cross-cutting`, `source-signatures`) — not nested inside them
- Source triples reference artifacts by stable identifier: layout-node by its `id`, surface fragment by `@feature/fragment-slug`, domain element by `@feature/entity[.field]` or `@feature/operation`
- Presentation hints are vocabulary-typed against the adapter's componentVocabulary and tokens; an unknown hint at build-finalize time is a build error, not a deferred-to-codegen problem
- The confidence annotation is exhaustive — every binding has exactly one of `rules`, `ai`, or `designer`
- Removing a layout node from the layout removes its binding entry on the next build (the binding is layout-derived, not durable independent of layout)
- Renaming a layout node `id` does NOT silently re-bind — the build phase fails the affected node and re-triggers Pass 1 (which may then re-trigger Pass 2 / prompt) so the change is auditable
- Bindings are feature-scoped — the same buildfile can carry bindings for multiple pages within a feature, but never for pages in another feature

**Verify**:
- A buildfile produced for the Tasks-page example contains a `bindings:` section with entries for every layout node, each carrying a complete source triple and `confidence: rules`
- A binding produced by Pass 2 is recorded with `confidence: ai` and the AI session/run identifier so the choice is auditable
- A binding produced by an interactive prompt is recorded with `confidence: designer` and the timestamp of the choice
- Removing a layout node and re-running build produces a buildfile whose `bindings:` no longer has that node's entry
- Renaming a layout node `id` produces a build where the old entry is dropped, the new node triggers fresh resolution (rules → ai → prompt as needed), and the buildfile reflects the renamed node
- A binding with an unknown presentation hint (`presentation: clarity.foobar`) fails the build with a clear error naming the offending hint and the active adapter version

---

## Headless Build for CI

**Goal**: `parlay build-feature` runs correctly in CI and other headless contexts — no interactive prompts, no hangs, no silent guesses. Ambiguity becomes an explicit error; the build fails fast, CI sees a non-zero exit, and a developer re-runs locally to make the binding choice.
**Persona**: Build Engineer
**Priority**: P0
**Context**: CI invokes `parlay build-feature` after layout edits, after domain edits, and as part of pre-merge checks. The interactive disambiguation prompt described in *Raise an Interactive Disambiguation Prompt* requires a human; CI has none. Headless mode converts every interactive path into a deterministic-failure variant so binding decisions never silently default.
**Action**: The build agent detects non-interactive invocation (no TTY, or the explicit `--non-interactive` flag) and switches every interactive path to its error variant. Pass-2 ambiguity that would have prompted produces an `ambiguous-binding` build-time error listing all candidates with their AI confidence values. Other build-time errors (`orphan-layout-node`, `removed-field-referenced`) behave identically across modes. The process exits non-zero on any error path; no partial buildfile reaches disk.
**Objects**: build-feature, headless, non-interactive, error-code, ambiguous-binding, orphan-layout-node, removed-field-referenced

**Constraints**:
- Non-interactive detection: TTY check OR explicit `--non-interactive` flag; the flag overrides the TTY check in either direction (force interactive in tmux, force non-interactive locally for testing CI behavior)
- Ambiguous bindings produce `ambiguous-binding at <feature> > <page> > <node-path>: candidates [<list with confidences>] (expected: exactly one match). To fix: rename the layout-node label to disambiguate, narrow the surface Action that maps to this node, add a wiring rule under wiring.rules, or run \`parlay build-feature\` interactively to record a designer choice.`
- `orphan-layout-node` and `removed-field-referenced` errors behave identically interactive vs non-interactive — they always fail with an actionable error
- Process exit code is non-zero on any error path; CI's pass/fail is derived from exit code, not stdout pattern matching
- The non-interactive path never writes a partial buildfile: if any binding cannot be resolved, the run leaves the buildfile-output directory in a state consistent with "no run produced these files" so a half-resolved buildfile never reaches downstream codegen
- Pass-2 AI inference is allowed in non-interactive mode but never escalates to a prompt — if Pass 2 returns multiple equally-confident candidates, it errors out as `ambiguous-binding` instead
- Recorded designer choices from prior interactive runs are honored in CI — headless mode reads the buildfile's existing bindings as authoritative for decisions already made

**Verify**:
- Running `parlay build-feature --non-interactive` on a feature with one ambiguous binding exits non-zero with the binding-candidates listed in the error and no buildfile written
- Running the same command on a feature with no ambiguities produces a buildfile that passes the same testcases the build phase recorded for the same source state
- A simulated CI run (no TTY) on an `orphan-layout-node` condition produces the same error message and exit code as an interactive run on the same input
- `parlay build-feature --non-interactive` invoked locally with a TTY present uses the non-interactive code path — the flag wins over TTY detection
- A CI run on a feature whose buildfile already has a designer-recorded binding for what would otherwise be an ambiguous node succeeds — the existing recorded choice is honored, the prompt is not re-raised, and the binding stays `confidence: designer`
- Two CI workers running `parlay build-feature --non-interactive` against the same source state produce buildfiles whose bindings (including any AI-resolved bindings) reach the same source triples; lexical reasoning text from the AI matcher may differ but the recorded triples are stable

---
