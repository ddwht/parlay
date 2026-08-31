# Layout-aware-build — Dialogs

---

### Run a Two-Pass Binding Resolution During Build

**Trigger**: A designer has finished editing a feature's surface, domain, layout, or pages and runs `parlay build-feature @<feature>` to refresh the buildfile.

#### Happy path — all bindings resolved by Pass 1

User: Runs `parlay build-feature @studio-support/tasks` from a terminal.
System: Reads `surface.md`, `domain-model.md`, and every `*.page.md` for the feature. Loads the merged rule set (starter rules plus `wiring.rules:` from any prior buildfile). Walks every layout-bearing page in source order; for each layout node that consumes data or emits an action, runs Pass 1 against the rule set.
System: Reports `tasks: pass-1 resolved 7/7 nodes (rules)` for each page where every node matches a single rule.
System: Writes the buildfile with a `bindings:` section keyed `feature → page → layout-node-path`. Every entry carries `confidence: rules` and the name of the rule that fired.
System: Exits 0. The designer sees a one-line summary per page plus the buildfile path.

#### Branch — Pass 1 leaves ambiguity, Pass 2 picks a winner

User: Same invocation; the layout has a `clarity.button` labeled "Save" and the domain has both `updateTask` and `saveDraft`.
System: Pass 1 returns two candidates for that node. Pass 2 invokes the AI matcher over the candidate set narrowed by Pass 1.
System: Pass 2 returns `updateTask` with confidence well above the threshold. Records the binding with `confidence: ai`, the AI session/run identifier, and the candidate list at the moment of decision.
System: Reports `tasks: pass-2 resolved 1 node (ai); candidate list recorded`. Exits 0.

#### Branch — Pass 2 leaves ambiguity → hand off to disambiguation prompt

User: Layout has a `clarity.button` labeled "Process" and the domain has `processTask` and `processBatch` with similar shapes.
System: Pass 2 returns two candidates whose confidences are within the configured threshold. Control transfers to *Raise an Interactive Disambiguation Prompt* (see below).

#### Branch — orphan layout node

User: Layout declares a node that has no matching surface fragment to feed it.
System: Pass 1 produces zero candidates. The layout shape (e.g., a `clarity.datagrid` with no Show fragment in surface) does not suggest a binding is expected to be invented. The build aborts with `orphan-layout-node at <feature> > <page> > <node-path>: no surface fragment found that this node can consume. To fix: add a Shows entry to surface, remove the layout node, or wire it to an existing fragment via wiring.rules.`
System: Exits non-zero. No partial buildfile is written.

#### Branch — re-run on identical inputs

User: Runs `parlay build-feature` a second time without changing anything.
System: Reads cached bindings from the existing buildfile. The recorded source triples are stable across runs even if Pass-2 lexical reasoning text would differ. Re-prompts no one. Exits 0 with a `bindings unchanged` summary.

---

### Define a Starter Rule Set, Extensible Per Project

**Trigger**: A designer wants to understand which structural patterns the build phase resolves automatically, or wants to add a project-specific binding convention.

#### Happy path — starter rules cover the common cases

User: Has a fresh project with no `wiring.rules:` block in the buildfile. Runs `parlay build-feature @<feature>`.
System: Loads the starter rule set (structural-hint matches like `contentShape: badge` → matching Show field; action-verb matches like a button labeled "Create task" + surface `Action: createTask` → `domain.createTask`; single-candidate matches when surface declares exactly one Action and the domain has exactly one operation of matching shape).
System: Pass 1 resolves every node on the Tasks-page example end-to-end. Buildfile records each binding with `confidence: rules` and the starter-rule name.

#### Branch — adding a project rule

User: Adds a rule to `wiring.rules:` in the buildfile mapping `tone` on a layout column to `clarity.badge.tone`. Schema: `match` (predicate over layout-node + surface fragment + domain element shape), `bind` (the source triple to record), `precedence` (integer), `confidence: rules`.
System: On next build, merges starter + project rules. The new rule fires on eligible nodes and the buildfile records the rule name in the binding entry — auditable from the buildfile alone.

#### Branch — rule conflict at the same precedence

User: Adds two project rules that both match the same node with different bindings and the same `precedence` value.
System: Build aborts with `rule-conflict: rules <ruleA> and <ruleB> both match <feature> > <page> > <node-path> at precedence <n> with different bindings. To fix: raise one rule's precedence above the other, or narrow one rule's match predicate.`
System: Exits non-zero.

#### Branch — project rule overrides a starter rule

User: Adds a project rule at higher precedence than a starter rule that matches the same node.
System: The project rule fires; the starter rule does not. Buildfile annotates which rule fired (`rule: project/<name>` vs `rule: starter/<name>`) so the override is auditable.

#### Branch — project rule attempts to lower a starter rule's precedence

User: Adds a project rule whose precedence would silently disable a starter rule (e.g., by setting precedence below the starter and configuring the same match).
System: Rejects at rule-load time: `rule-precedence-error: project rule <name> attempts to silently disable starter rule <starter-name>. Projects can override starter rules at higher precedence, but cannot place rules below them.`

#### Branch — rule references a non-existent domain field

User: Adds a project rule whose `match` predicate references `Task.priority` but the domain entity has no `priority` field.
System: Rejects at rule-load time (not at match time): `rule-load-error: rule <name> references domain field <Task.priority> that does not exist in the active feature's domain.` This surfaces authoring errors immediately rather than at the moment of an unrelated build.

#### Branch — recursive or self-matching rule

User: Adds a rule whose `bind` output would cause a second rule (or the same rule) to fire on the produced binding.
System: Rejects at rule-load time with `rule-termination-error: rule <name> produces a binding that re-triggers <other-rule-name>` — rule termination is checked statically.

---

### Raise an Interactive Disambiguation Prompt When Both Passes Leave Ambiguity

**Trigger**: Pass 2 returned multiple candidates within the confidence threshold for a layout node, and the build is running interactively (TTY present, `--non-interactive` not set).

#### Happy path — single ambiguity, designer picks

User: Runs `parlay build-feature @studio-support/tasks` after adding a "Process" button.
System: Pass 1 narrows to two operations; Pass 2 returns both within threshold. The build pauses and prints:
```
ambiguous binding at:
  pages/tasks.md > region/toolbar/button[2] ("Process")
candidates:
  [1] @studio-support/tasks/operation/processTask  (ai-confidence: 0.62)
  [2] @studio-support/tasks/operation/processBatch (ai-confidence: 0.58)
  [q] quit (abort build, exit non-zero)
  [s] skip (record as `unresolved`, continue, buildfile will be invalid)
choose >
```
User: Types `1` and presses Enter.
System: Records the binding with `confidence: designer`, the chosen source triple, the timestamp of the choice, and the candidate list as it was at the moment of decision. Continues the build. Exits 0.

#### Branch — re-run after a recorded choice

User: Re-runs `parlay build-feature` immediately, no changes elsewhere.
System: Reads the recorded `confidence: designer` binding from the existing buildfile. Verifies the candidate list has not changed (same domain operations, same surface action, same layout node). Skips the prompt. Exits 0 with `bindings unchanged`.

#### Branch — candidate list changes invalidate the choice

User: Renames `processTask` to `processTaskItem` in the domain and re-runs build.
System: Detects that the chosen domain operation no longer exists (or that the candidate list has changed in a way that invalidates the prior choice). Drops the prior recorded binding. Re-runs Pass 1, then Pass 2, then re-surfaces the prompt with the new candidate list (now including `processTaskItem`).

#### Branch — multiple ambiguities in one run

User: Three pages each have one ambiguous binding.
System: Surfaces prompts one at a time in `(page-path, node-path)` lexicographic order. Each prompt waits for input before the next is shown. After all three are answered, the build completes and writes a single buildfile with all three `confidence: designer` entries.

#### Branch — designer aborts with `[q]`

User: At a prompt, types `q` and presses Enter.
System: Aborts the build with a non-zero exit code. No partial buildfile is written. Recorded choices made earlier in this same run are also discarded — the buildfile-output directory is left consistent with "no run produced these files."

#### Branch — designer skips with `[s]`

User: At a prompt, types `s` to defer the decision.
System: Records the binding as `unresolved` and continues the build. The resulting buildfile fails its own validity check (so codegen will refuse to consume it), but the designer can collect multiple decisions before re-running. A clear summary at end-of-run lists every `unresolved` binding.

#### Branch — designer adds a new ambiguous node, prior choice preserved

User: Edits the layout to add a new button with its own ambiguity, then re-runs build.
System: The previously recorded choice for the original node is preserved (its candidate list is unchanged). A fresh prompt appears for the new node only.

---

### Record Resolved Bindings in the Buildfile with Traceability Triples

**Trigger**: A build run is finalizing the buildfile — every binding decision (rules, ai, designer) needs a durable record.

#### Happy path — bindings section emitted

User: Runs `parlay build-feature` and the run completes successfully.
System: Writes a `bindings:` section as a peer to `models`, `fixtures`, `routes`, `components`, `cross-cutting`, and `source-signatures` (not nested inside any of them). Keys: `<feature> → <page> → <layout-node-path>`. Each entry carries:
- the source triple: `layout_node` (by `id`), `surface_fragment` (`@feature/fragment-slug`), `domain_element` (`@feature/entity[.field]` or `@feature/operation`)
- presentation hints (e.g., `presentation: badge`, `tone: status-color`) typed against the active adapter's componentVocabulary and tokens
- the `confidence` annotation: exactly one of `rules`, `ai`, `designer`
- when `confidence: rules`, the rule name (`starter/contentShape-badge` or `project/<name>`)
- when `confidence: ai`, the AI session/run identifier
- when `confidence: designer`, the timestamp of the choice and the candidate list at the time

#### Branch — designer asks "why did this column render as a badge?"

User: Greps the buildfile or runs a future tool that walks the bindings section.
System: The binding entry shows the layout-node id, the surface fragment, the domain field, `presentation: badge`, `confidence: rules`, and `rule: starter/contentShape-badge`. Designer has a complete answer with no inference required.

#### Branch — unknown presentation hint

User: A layout node carries `presentation: clarity.foobar` and `clarity.foobar` is not in the adapter's componentVocabulary.
System: Build aborts at finalize time with `unknown-presentation-hint: clarity.foobar at <feature> > <page> > <node-path>. Active adapter: <name>@<version>. Known hints: [...]. To fix: choose a known hint, upgrade the adapter, or add the hint to the adapter's componentVocabulary.`
System: This is a build error, not a deferred-to-codegen problem.

#### Branch — layout node removed

User: Removes a layout node from a `*.page.md` file and re-runs build.
System: The bindings section on the next build no longer carries that node's entry. Bindings are layout-derived, never durable independent of layout.

#### Branch — layout-node `id` renamed

User: Renames a layout node's `id` from `task-status-column` to `status-col` without otherwise changing the node's shape.
System: Drops the old entry. Treats the new node as fresh: re-runs Pass 1, then Pass 2 if needed, then prompt if needed. The change is auditable because the buildfile's bindings section reflects the renamed node, not a silent re-bind to the same operation.

#### Branch — bindings cross-feature isolation

User: Has two features `tasks` and `projects`, each with its own buildfile. The tasks feature has a layout node whose surface action could plausibly map to a domain operation in `projects`.
System: Inference is bounded to the active feature. The build agent only considers candidates from `tasks`'s own surface and domain. Cross-feature operations are never auto-bound — if a designer wants that wiring, they must declare it explicitly via cross-cutting (covered by a different feature's intents).

---

### Headless Build for CI

**Trigger**: A CI worker runs `parlay build-feature @<feature>` after a layout/domain edit, or a developer runs `parlay build-feature --non-interactive` locally to test CI behavior.

#### Happy path — CI build with no ambiguities

User (CI): Invokes `parlay build-feature @studio-support/tasks`. No TTY is attached.
System: Detects non-interactive context (no TTY). Runs Pass 1 and Pass 2 normally. Every binding resolves unambiguously. Writes the buildfile. Exits 0. CI sees the zero exit and proceeds.

#### Branch — CI build hits an ambiguous binding

User (CI): Same invocation, but a layout edit introduced an ambiguous "Process" button.
System: Pass 2 returns multiple candidates within threshold. Instead of prompting, emits:
```
ambiguous-binding at studio-support/tasks > pages/tasks.md > region/toolbar/button[2]:
  candidates:
    @studio-support/tasks/operation/processTask  (ai-confidence: 0.62)
    @studio-support/tasks/operation/processBatch (ai-confidence: 0.58)
  expected: exactly one match
  to fix: rename the layout-node label to disambiguate, narrow the surface Action that maps to this node,
          add a wiring rule under wiring.rules, or run `parlay build-feature` interactively
          to record a designer choice.
```
System: Exits non-zero. No partial buildfile is written. The buildfile-output directory is left consistent with "no run produced these files" so downstream codegen never consumes a half-resolved buildfile.

#### Branch — CI honors prior designer choices

User (CI): The repository already contains a buildfile with a `confidence: designer` binding for what would otherwise be an ambiguous node, recorded in a prior interactive run.
System: Reads the existing binding as authoritative. Verifies the candidate list is unchanged. Does not re-prompt, does not error. The binding stays `confidence: designer`. Exits 0.

#### Branch — `--non-interactive` overrides TTY detection

User: Runs `parlay build-feature --non-interactive` locally with a TTY attached, to test CI behavior before pushing.
System: The flag wins over TTY detection. Behaves exactly as in CI: ambiguous bindings produce `ambiguous-binding` errors instead of prompts.

#### Branch — `orphan-layout-node` and `removed-field-referenced` errors

User (CI or local): The layout has a node with no surface fragment, or surface references a domain field that has been removed.
System: These errors behave identically interactive and non-interactive — they always fail with an actionable error and a non-zero exit code. They never gate on the disambiguation prompt.

#### Branch — Pass-2 AI inference allowed in non-interactive mode

User (CI): A layout node hits Pass 2 and Pass 2 returns a single high-confidence candidate.
System: Records the binding with `confidence: ai` and continues. AI inference itself is not the boundary; AI inference that *escalates to a prompt* is. The headless mode rejects only the escalation, not the inference.

#### Branch — two CI workers, same source state, stable buildfiles

User: Two CI workers run `parlay build-feature --non-interactive` against the same source state in parallel pipelines.
System: Both produce buildfiles whose recorded source triples are identical for every binding (including any AI-resolved ones). The lexical reasoning text the AI matcher used internally may differ run-to-run, but the recorded triples — what gets written to the buildfile — are stable.

---
