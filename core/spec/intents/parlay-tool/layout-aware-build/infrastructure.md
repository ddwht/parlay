# Layout-aware-build — Infrastructure

---

## Two-Pass Binding Resolution

**Affects**: build-feature binding resolution pipeline
**Behavior**: For every layout node on every layout-bearing page in the active feature, the build phase resolves a single binding consisting of a source triple `(layout-node, surface-fragment, domain-element)` plus presentation hints. Resolution proceeds in two ordered passes. Pass 1 evaluates the merged rule set (starter rules plus project rules from `wiring.rules:` in the buildfile); a single match records the binding with `confidence: rules`. Zero or multiple matches escalate to Pass 2, which invokes an AI matcher over the candidate set Pass 1 narrowed to (or an empty set, for orphan nodes); a single high-confidence pick records the binding with `confidence: ai`. If Pass 2 still leaves ambiguity (multiple candidates within the configurable confidence threshold), control transfers to the disambiguation prompt for interactive selection. Resolution is bounded to the active feature; cross-feature surface fragments and domain operations are never consulted.
**Invariants**:
- Pass 2 never executes against a node that Pass 1 resolved unambiguously.
- The rule set used for Pass 1 is exactly the union of the starter rule set and the project rules declared under `wiring.rules:` in the buildfile — no other rule sources are consulted.
- The Pass 2 candidate set is exactly the set Pass 1 produced; the AI matcher never invents candidates outside that set.
- A node with zero Pass 1 candidates only escalates to Pass 2 when the layout shape suggests a binding is expected; otherwise resolution emits an `orphan-layout-node` build-time error.
- Every recorded binding carries exactly one of `confidence: rules`, `confidence: ai`, or `confidence: designer`.
- Re-running build on identical inputs records bindings whose source triples are stable across runs, even when AI lexical reasoning text would differ.
- Inference is bounded to the active feature; no surface fragment or domain element from any other feature appears in any candidate set.
**Source**: @parlay-tool/layout-aware-build/run-a-two-pass-binding-resolution-during-build
**Caching**: per-process
**Backward-Compatible**: yes

**Notes**:
- Codegen never re-litigates a binding decision. The buildfile is the contract: build resolves, codegen consumes.
- The two-pass ordering is what makes determinism + AI assistance compose. Determinism comes first (Pass 1); AI fills gaps (Pass 2); designer judgment is the floor (prompt).

---

## Starter Rule Set and Project Extension

**Affects**: rule-engine vocabulary and rule loading
**Behavior**: A finite, enumerated starter rule set ships with the build phase, covering the common structural patterns that allow Pass 1 to resolve bindings without AI: structural-hint matches (a layout-node property like `contentShape: badge` maps to a matching surface Show field), action-verb matches (a button label or aria semantics combined with a surface `Action` maps to a domain operation), and single-candidate matches (when surface declares one Action and the domain has exactly one operation of that shape, the binding is unambiguous). Projects extend this set by declaring rules under `wiring.rules:` in the buildfile, using a closed-shape rule schema with the fields `match` (predicate over layout-node, surface fragment, and domain element), `bind` (the source triple to record), `precedence` (integer, higher wins), and `confidence: rules`. The build agent merges starter and project rules at run time and applies them in precedence order during Pass 1.
**Invariants**:
- The starter rule set is finite and enumerated. Adding a starter rule requires a build-feature schema bump, not a runtime change.
- The rule schema is a closed set of fields: `match`, `bind`, `precedence`, `confidence`. No other fields are accepted.
- Project rules live only under `wiring.rules:` in the buildfile. They are never declared inline in surface, domain, or layout files.
- Rule conflicts (two rules matching the same node with different bindings at the same precedence) produce a build-time error naming both rule definitions; resolution does not silently pick a winner.
- A project rule cannot place itself below a starter rule with the same match — projects can override starter rules at higher precedence but cannot silently disable them.
- Rule termination is checked statically at rule-load time; recursive references and rules that re-trigger themselves or each other are rejected.
- Every binding produced by Pass 1 records the rule name that fired (`starter/<name>` or `project/<name>`) so the choice is auditable from the buildfile alone.
- A rule whose `match` predicate references a non-existent domain field is rejected at rule-load time, not at match time.
**Source**: @parlay-tool/layout-aware-build/define-a-starter-rule-set-extensible-per-project
**Caching**: per-process
**Backward-Compatible**: yes

**Notes**:
- The starter rule set's value depends on it covering the common cases well enough that Pass 2 (AI) is reserved for genuinely ambiguous bindings. Too small a starter set forces every binding through AI; too opinionated a starter set blocks projects from expressing their own conventions.
- The starter set is identical across all projects. Project extensions are project-scoped.

---

## Interactive Disambiguation Choice Recording

**Affects**: build-feature binding-decision persistence
**Behavior**: When the disambiguation prompt collects a designer's selection, the build phase records the chosen binding with `confidence: designer`, the source triple, the timestamp of the choice, and the candidate list as it existed at the moment of selection. Recorded designer choices persist across subsequent build runs as long as the candidate list remains identical (same domain operations, same surface action shape, same layout node). When the candidate list changes (a domain operation is renamed or removed, a new operation appears, the surface action narrows or widens), the recorded choice is invalidated and the prompt re-surfaces with the updated candidate list. Choices recorded with the `[s] skip` option are stored as `unresolved` and cause the buildfile's own validity check to fail, signaling that codegen should refuse to consume the buildfile until the deferred decisions are made. Recorded choices are stored in the buildfile only — never in surface, domain, or layout files.
**Invariants**:
- A recorded designer choice is read as authoritative on subsequent build runs as long as its candidate list is unchanged. The prompt is not re-surfaced.
- A change in the candidate list (operation rename, removal, addition; surface action shape change) invalidates the recorded choice and re-triggers the prompt with the updated list.
- Recorded choices are written to the buildfile, never to layout, surface, or domain artifacts. Designer decisions are build-time state, not authoring artifacts.
- The `[q] quit` option aborts the build with non-zero exit and writes no partial buildfile; recorded choices made earlier in the same run are also discarded.
- The `[s] skip` option records `unresolved` and continues the build; the resulting buildfile fails its own validity check.
- Multiple ambiguities in one run produce prompts in deterministic `(page-path, node-path)` lexicographic order; the designer answers them one at a time.
- The recorded candidate list is what determines invalidation, not the AI confidence values; lexical AI reasoning text may differ across runs without invalidating a recorded choice.
**Source**: @parlay-tool/layout-aware-build/raise-an-interactive-disambiguation-prompt-when-both-passes-leave-ambiguity
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- This fragment is the persistence-and-lifecycle counterpart to the surface fragment "Disambiguation Prompt". The surface fragment describes what the designer sees and chooses; this fragment describes how that choice survives, when it expires, and where it is stored.
- The "candidate list at the moment of decision" is the durable record. Re-deriving the candidate list on each build run and comparing to the recorded one is what gives the system its rename-detection semantics.

---

## Buildfile Bindings Section

**Affects**: buildfile schema and finalization
**Behavior**: The buildfile gains a `bindings:` section as a peer to the existing top-level sections (`models`, `fixtures`, `routes`, `components`, `cross-cutting`, `source-signatures`). The section is keyed by feature, then page, then layout-node-path. Each entry records the source triple — `layout_node` (referenced by its stable `id`), `surface_fragment` (referenced as `@feature/fragment-slug`), and `domain_element` (referenced as `@feature/entity[.field]` or `@feature/operation`) — plus presentation hints typed against the active adapter's componentVocabulary and tokens, the `confidence` annotation (`rules`, `ai`, or `designer`), and provenance: rule name for `rules`, AI session/run identifier for `ai`, timestamp and recorded candidate list for `designer`. Buildfile validity requires every layout-bearing-page node to have an entry. Bindings are layout-derived: removing a layout node drops its entry on the next build; renaming a layout-node `id` drops the old entry and triggers fresh resolution for the new node rather than silently re-binding to the same target.
**Invariants**:
- The `bindings:` section is a peer to `models`, `fixtures`, `routes`, `components`, `cross-cutting`, and `source-signatures` — never nested inside any of them.
- Source triples reference artifacts only by stable identifier: layout-node by `id`, surface fragment by `@feature/fragment-slug`, domain element by `@feature/entity[.field]` or `@feature/operation`.
- Every binding entry has exactly one `confidence` value: `rules`, `ai`, or `designer`.
- Presentation hints unknown to the active adapter's componentVocabulary or tokens fail the build at finalize time, naming the offending hint and the active adapter version.
- Removing a layout node from the layout removes its binding entry on the next build run.
- Renaming a layout-node `id` drops the old entry and triggers fresh Pass-1-then-Pass-2-then-prompt-as-needed resolution; it never silently re-binds.
- Bindings are feature-scoped; one feature's buildfile carries entries only for that feature's pages.
- A buildfile produced for a feature whose pages contain layout nodes is invalid if any such node has no `bindings:` entry.
**Source**: @parlay-tool/layout-aware-build/record-resolved-bindings-in-the-buildfile-with-traceability-triples
**Caching**: none
**Backward-Compatible**: no

**Notes**:
- `Backward-Compatible: no` because the schema gains a new top-level section that older buildfiles will not have. The build-feature schema bump is the migration trigger; existing buildfiles without `bindings:` will need to be regenerated, not patched.
- The bindings section is the durable record that makes "why did this column render as a badge?" answerable from the buildfile alone. Codegen, freshness checks, and audit tools all read from it.

---

## Headless Build Mode

**Affects**: build-feature interactive vs non-interactive control flow
**Behavior**: The build phase detects non-interactive invocation (no TTY attached, or the explicit `--non-interactive` flag — the flag wins over TTY detection in either direction) and switches every interactive code path to its error variant. Pass-2 ambiguity that would have surfaced the disambiguation prompt instead emits an `ambiguous-binding` build-time error listing all candidates with their AI confidence values, an `expected: exactly one match` line, and a remediation hint enumerating the four ways to fix it. Other build-time errors (`orphan-layout-node`, `removed-field-referenced`) behave identically across interactive and non-interactive modes. Pass 2 AI inference itself remains allowed in non-interactive mode; only the escalation-to-prompt path is rejected. Recorded designer choices from prior interactive runs are read as authoritative in non-interactive mode and are not re-prompted. The non-interactive path commits the buildfile atomically: if any binding cannot be resolved, the buildfile-output directory is left in a state consistent with "no run produced these files," so a half-resolved buildfile never reaches downstream codegen.
**Invariants**:
- Non-interactive mode is detected by absence of TTY OR by the explicit `--non-interactive` flag; the flag overrides TTY detection in both directions.
- Pass-2 ambiguity in non-interactive mode produces an `ambiguous-binding` error and a non-zero exit code; it never blocks waiting for input.
- `orphan-layout-node` and `removed-field-referenced` errors behave identically interactive and non-interactive: actionable error message plus non-zero exit.
- Pass 2 AI inference is permitted in non-interactive mode; only the escalation-to-prompt path is rejected.
- Recorded designer choices from prior interactive runs are honored in non-interactive mode without re-prompting, as long as their candidate lists are unchanged.
- The non-interactive path never writes a partial buildfile. Either the buildfile is committed whole or nothing reaches the output directory.
- Two non-interactive runs against the same source state produce buildfiles whose recorded source triples are identical for every binding (including AI-resolved ones); only AI lexical reasoning text may differ.
- The process exit code is non-zero on any error path; CI's pass/fail derives from the exit code, not from stdout pattern matching.
**Source**: @parlay-tool/layout-aware-build/headless-build-for-ci
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The contract for CI is exit-code-driven: zero on success, non-zero on any error. Stdout/stderr text is for human consumption and may change wording across versions; CI scripts must not pattern-match it.
- The atomic-buildfile-commit invariant is what prevents downstream codegen from ever consuming a half-resolved buildfile. Implementations have flexibility (atomic temp-file-rename, in-memory accumulation then single write, write-then-fsync-then-rename) — the invariant is "either fully committed or not present", not a specific mechanism.
