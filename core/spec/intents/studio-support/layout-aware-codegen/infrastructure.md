# Layout-Aware Code Generation — Infrastructure

---

## Layout-Block Reader

**Affects**: codegen pipeline
**Behavior**: For every page artifact a codegen run encounters, detect whether the artifact carries a `layout:` block. When present, read the typed tree and treat it as the structural source of truth for that page's emission. When absent, the page falls through to the existing surface-and-domain emission path. Activation is per-page, not per-project — pages with and without layout coexist in the same run.
**Invariants**:
- A page with a `layout:` block produces a generated component tree whose shape matches the layout tree (parent/child order preserved)
- A page without a `layout:` block generates the same output it did before this feature shipped (regression-confirmed against pre-feature output for the same adapter version and source state)
- The presence-of-layout decision is page-scoped — one feature can mix layout-bearing and layout-free pages
- Token references (`spacing-lg`, `spacing-xl`) flow through to generated code as adapter-defined token references, never as raw pixel values
**Source**: @studio-support/layout-aware-codegen/codegen-reads-the-layout-block-when-present, @studio-support/layout-aware-codegen/backward-compatible-path-for-layout-free-pages
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Layout validation against the adapter (vocabulary version, known component types, well-formed block) is owned by the **layout-creation feature**, not codegen. Codegen consumes a precheck verdict; it does not re-litigate validity.
- Behavioral equivalence is the contract — byte-identical output is not a goal because the emission step is performed by an AI agent. Binding decisions live in the buildfile and are stable across runs.

---

## Resolved-Binding Consumer

**Affects**: codegen wiring emission
**Behavior**: For each layout node, look up the corresponding binding entry in the buildfile and emit framework-specific wiring code — pass entity data into the component, attach operation calls to action handlers, apply presentation hints to render properties. Codegen does not invoke the rules engine, the AI matcher, or any disambiguation prompt — those operations live in the build phase and have already produced the bindings before codegen runs.
**Invariants**:
- A buildfile with a complete set of bindings produces generated code where every layout node has its expected entity-feed and operation-handler wiring
- A binding's `(layout-node, surface-fragment, domain-element)` source triple is preserved as a comment or annotation alongside the wired component in the generated output
- Codegen never invokes the rules engine, AI matcher, or disambiguation logic — those calls do not appear in any codegen path
- Re-running codegen against an unchanged `(layout, buildfile, adapter)` produces output that passes the same testcases and emits the same component tree (lexical text may differ; behavior does not)
**Source**: @studio-support/layout-aware-codegen/codegen-consumes-resolved-bindings-from-the-buildfile
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- No cache because there is no inference work to cache — every run reads the buildfile, reads the layout, and emits.
- Presentation hints (e.g., `presentation: badge` on a status column) come from the buildfile binding, not from codegen-time inference.

---

## Buildfile Freshness Gate

**Affects**: codegen entry guard
**Behavior**: Before emitting code for any layout-bearing page in a feature, recompute content signatures for every source artifact the buildfile consumed (intents, dialogs, surface, domain, layout, adapter version) and compare against the signatures recorded in the buildfile. On match, proceed. On mismatch, refuse to run for that feature and surface a `stale-buildfile` error pointing the author at `parlay build-feature` to refresh. The gate is mechanical, runs without AI, and is per-feature in scope.
**Invariants**:
- Running codegen on a feature whose buildfile predates a source edit produces a `stale-buildfile` error and a non-zero exit, with no files written
- Touching a source file without changing its content does not trigger `stale-buildfile` — signatures are content-based, not timestamp-based
- Running codegen immediately after `parlay build-feature` succeeds with no freshness-gate failure
- A precheck refusal from the layout-creation feature is surfaced verbatim and is **not** wrapped in a `stale-buildfile` envelope, even when the buildfile is also stale (precheck wins because the layout itself is invalid)
- A two-feature project where feature A is stale and feature B is fresh still generates feature B and reports feature A's failure with code `stale-buildfile`
**Source**: @studio-support/layout-aware-codegen/buildfile-freshness-gate, @studio-support/layout-aware-codegen/codegen-consumes-resolved-bindings-from-the-buildfile
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Error message format: `stale-buildfile at <feature>: buildfile reflects <prior-signature>; current sources are <current-signature>. To fix: run \`parlay build-feature <feature>\` to refresh the buildfile, then re-run codegen.`
- The freshness gate is the only codegen-owned content-error category in this feature. Binding-content errors (orphan layout nodes, ambiguous bindings, removed-field references) are owned by the build phase.
- A buildfile lacking a binding for a layout node codegen reaches is treated as a freshness-gate failure — there is no separate "missing-binding" error class in codegen.

---

## Layout-Validation Precheck Surfacer

**Affects**: codegen entry guard
**Behavior**: Before emitting code for a layout-bearing page, consult the layout-validation precheck owned by the layout-creation feature. If the precheck refuses (unknown component type, vocabulary version mismatch, malformed block), surface its refusal verbatim, refuse to run codegen for that page, and continue with other pages in the same project. Do not augment the message with codegen-internal vocabulary, do not silently fall back to the layout-free path, and do not re-classify the failure as `stale-buildfile`.
**Invariants**:
- A page whose layout fails the precheck causes codegen to refuse for that page; the surfaced message is the precheck's verbatim output
- Other pages in the same project (with valid or absent layouts) continue to generate normally
- A page with an invalid layout never silently falls back to the layout-free path
- When both precheck failure and stale buildfile apply to the same page, the precheck refusal is surfaced and `stale-buildfile` is suppressed for that page
**Source**: @studio-support/layout-aware-codegen/codegen-reads-the-layout-block-when-present, @studio-support/layout-aware-codegen/backward-compatible-path-for-layout-free-pages, @studio-support/layout-aware-codegen/buildfile-freshness-gate
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The precheck itself is implemented by the **layout-creation feature** (separate, future). This infrastructure entry only covers codegen's role: consult the precheck verdict, surface refusals verbatim, and gate emission accordingly.

---

## Non-Interactive Codegen Pipeline

**Affects**: codegen process behavior
**Behavior**: Codegen has no TTY-conditional code paths and no interactive prompts. It reads the buildfile, runs the freshness gate, runs the layout-validation precheck, and emits framework code via an AI agent — but the agent has no decisions left to make at codegen time, so it never prompts. Any failure produces a non-zero exit code; success produces zero. Output is written atomically: if any page fails, the output directory is left in a state consistent with "no run produced these files" so a half-written prototype never reaches CI's verification step.
**Invariants**:
- Running codegen in a no-TTY environment produces output behaviorally equivalent to running it locally with a TTY on the same source state
- Process exit code is non-zero on any error path (stale buildfile, layout precheck refusal); exit code is zero on success
- Passing `--non-interactive` to codegen has no observable effect — the flag is silently accepted for compatibility
- Codegen never writes partial output: on any per-page failure, no new files are written for the run
- Two concurrent runners executing codegen against the same source state produce identical exit codes and behaviorally-equivalent output (same testcases pass, same component tree; lexical text may vary because the emitting AI agent is non-deterministic on text)
**Source**: @studio-support/layout-aware-codegen/codegen-is-non-interactive-and-ci-safe
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The interactive concern (disambiguation, binding inference) is a **build-phase** responsibility — not addressed in this infrastructure entry. Codegen is non-interactive *by construction* because there are no decisions left to make.
- CI's pass/fail is derived from exit code, not from stdout pattern matching.
