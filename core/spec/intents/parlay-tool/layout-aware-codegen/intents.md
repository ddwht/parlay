# Layout-Aware Code Generation

> When a page artifact carries a `layout:` block, `parlay generate-code` consumes the typed tree as the structural source of truth and emits framework-specific code from it. The wiring (data sources, operation calls, presentation hooks) needed to bind layout nodes to domain entities and operations is **pre-resolved during the build phase** and encoded in the buildfile — codegen reads those bindings rather than inferring them. Pages without `layout:` fall back to the existing codegen behavior unchanged. This is the Core-side counterpart to Studio's Design Loop: Studio produces structure, the build phase resolves bindings, and codegen emits framework code from those resolved bindings.
>
> **Why three artifacts and a build phase.** Layout is intentionally wiring-free — its `clarity.datagrid` and `clarity.button` nodes do not encode which entity feeds the grid or which operation the button invokes. The **build phase** recovers that wiring by matching layout against the surface (`Shows` / `Actions`) and the domain model (entities / operations), and writes the resolved bindings into the buildfile. **Codegen** then reads the buildfile and the layout, resolves types against the adapter, and emits framework code. Both build and codegen use AI agents (build for binding inference, codegen for framework-text emission), but the binding *decisions* live only in build — codegen does not re-litigate them, and codegen has no interactive paths. Each artifact answers one independent question — *what is perceived and done* (surface), *what entities and operations exist* (domain), *what shape under this design system* (layout) — and the build phase is where they meet. Wiring is intentionally kept out of both the Design Loop (Studio is structural) and codegen (codegen consumes resolved bindings rather than producing them), which is what gives each phase a clean responsibility. Removing any of the three artifacts would force the others to absorb its job: layout would have to embed wiring (corrupting the round-trip), surface would have to commit to a design system (breaking adapter-agnosticism), or the adapter would have to encode entity-level semantics (breaking framework neutrality).

---

## Codegen Reads the Layout Block When Present

**Goal**: When a page has a `layout:` block, codegen treats it as the structural specification of the rendered output — node hierarchy, component types, layout parameters, presentation tokens — instead of inferring structure from surface and domain alone.
**Persona**: UX Designer
**Priority**: P0
**Context**: Studio writes layout into pages; the build phase resolves bindings into the buildfile; codegen reads both. The three meet at the page artifact and the buildfile: Studio writes structure, build resolves wiring, codegen emits framework code.
**Action**: After loading the buildfile, the agent reads the page's `layout:` block (if present), resolves each node's `type` against the adapter's component vocabulary, and emits framework-specific component instances matching the tree shape. The bindings on each node — which domain entity feeds it, which operation an action invokes — are read from the buildfile, not inferred at codegen time. Layout parameters and tokens flow through to the generated code unchanged in semantic.
**Objects**: layout, page, generate-code, framework-adapter, component-instance, buildfile

**Constraints**:
- Codegen reads the layout block but does not modify it — round-trip writes are Studio's job, not codegen's
- Generated code preserves the structural shape of the layout tree: a `clarity.region` with two `clarity.region` children produces a parent component containing two child components, in that order
- Token references in layout (`spacing-lg`, `spacing-xl`) are emitted as adapter-defined token references in the generated code, never as raw pixel values
- Generation is **behaviorally equivalent** across runs and across agents: the same `(layout, buildfile, adapter)` input produces output that passes the same testcases and exhibits the same component tree. Byte-identical output is **not** a goal — codegen's emission step is performed by an AI agent, so lexical details (variable names, comment phrasing, statement ordering) may differ between runs. Behavior, structure, and bindings are stable because the binding decisions live in the buildfile and the layout shape is fixed
- Layout validation against the adapter — vocabulary version, known component types, well-formed layout block — is owned by the layout-creation feature, not codegen. Codegen relies on the layout being already valid: every node's `type` is in the adapter's vocabulary, the declared vocabulary version matches the loaded adapter, and the block parses cleanly. If a layout reaches codegen having failed its precheck, codegen refuses to run for that page and points the author back to the layout-creation flow rather than describing the failure with codegen-internal vocabulary
- Activation is per-page — this intent only applies to pages whose page artifact carries a `layout:` block; layout-free pages take the backward-compatible path described in *Backward-Compatible Path for Layout-Free Pages*

**Verify**:
- A page with a layout block of three Clarity components produces three Angular component instances in the generated template, in the same order
- A page without a layout block produces the same generated output as it did before this feature shipped (regression-tested by re-generating an existing project)
- A layout that fails its precheck (unknown type, vocabulary version mismatch, malformed block) causes codegen to refuse to run for that page; the surfaced message points the author at layout creation rather than describing codegen internals
- Two AI agents reading the same page (with layout) plus the same buildfile and adapter produce generated code that passes the same testcases and emits the same component tree, even though lexical details may differ — the emitting agent is non-deterministic on text but consistent on behavior

---

## Codegen Consumes Resolved Bindings From the Buildfile

**Goal**: Codegen reads the resolved binding for each layout node from the buildfile and emits framework code that wires components to those bindings. The act of producing the bindings — matching layout against surface and domain, running rules, invoking AI for ambiguous cases, raising disambiguation prompts — is owned by the build phase and is out of scope for this feature.
**Persona**: UX Designer
**Priority**: P0
**Context**: Layout is intentionally wiring-free — it carries structure but no bindings. The buildfile is where resolved bindings live. By the time codegen runs, every consumable node has a recorded entity-and-field binding, every action node has a recorded operation binding, and every ambiguous case from the build phase has been resolved (interactively at build time, or escalated to a build-time error). Codegen does not re-infer; it consumes.
**Action**: For each layout node, codegen looks up the binding entry in the buildfile and emits the corresponding framework-specific wiring code — passing entity data into the component, attaching operation calls to action handlers, applying presentation hints from the binding to the component's render properties.
**Objects**: codegen, buildfile, binding, layout-node, generated-code, presentation-hint

**Constraints**:
- Codegen never invokes the rules engine, the AI matcher, or any disambiguation prompt — those live in the build phase, not in codegen
- Each binding in the buildfile carries a (layout-node, surface-fragment, domain-element) source triple; the generated code preserves this triple as a comment or annotation so traceability survives into the framework output
- If the buildfile lacks a binding for a layout node that codegen reaches, codegen treats it as a freshness-gate failure and refuses to proceed — see *Buildfile Freshness Gate*
- Codegen's promise is behavioral equivalence across runs and agents: same `(layout, buildfile, adapter)` produces output that passes the same testcases and emits the same component tree. Lexical variation in the emitted text is acceptable because the AI agent that produces framework code is non-deterministic on text but consistent on behavior; binding decisions are stable because they come from the buildfile, not from inference at codegen time
- Codegen does not maintain a cache because there is nothing to cache — there is no inference work happening at codegen time. Each run reads the buildfile, reads the layout, and emits
- Wiring inference applies per page that has a layout — pages without `layout:` follow the backward-compatible surface-driven path

**Verify**:
- A buildfile with a complete set of bindings produces generated code where every component has its expected entity-feed and operation-handler wiring — confirmed by the testcases the build phase recorded for the same source state
- A buildfile entry for the status column with a `presentation: badge` binding produces a Clarity badge in the generated template, with the entity field name flowing through unchanged
- A buildfile recording a (layout-node, surface-fragment, domain-element) triple for the create button produces generated code where that triple appears as a traceability annotation alongside the wired handler
- Re-running codegen twice on the identical `(layout, buildfile, adapter)` input produces output that passes the same testcases and emits the same component tree; lexical details may differ because the AI agent that emits framework code is non-deterministic on text, but the wired structure and observable behavior are stable

---

## Backward-Compatible Path for Layout-Free Pages

**Goal**: Projects that have not adopted Studio yet — or features that simply do not have a layout authored — must continue to codegen exactly as they do today.
**Persona**: UX Designer (existing parlay user not running Studio)
**Priority**: P0
**Context**: Studio is opt-in. Many projects will have pages without layout blocks for some time, possibly indefinitely. Those projects must see no behavior change from this Core release.
**Action**: When a page has no `## Layout` section, the codegen path falls through to the existing logic — surface + domain + adapter produce the prototype the same way they always have. The layout-aware path activates only when a layout block is present and parses successfully.
**Objects**: page, layout, generate-code, backward-compatibility

**Constraints**:
- The presence-of-layout decision is per-page, not per-project — one feature can have layouts and another can not, in the same codegen pass
- A page with an invalid layout never silently falls back to the layout-free path — the precheck described in *Codegen Reads the Layout Block When Present* refuses to proceed for that page and points the author back to layout creation. Silent fallback would mask a real authoring error
- Existing testcase generation continues to work for layout-free pages
- Documentation makes the opt-in nature visible — running `parlay generate-code` on a project with no layouts produces the same output as before

**Verify**:
- A project with no layouts at all generates output behaviorally equivalent to its pre-feature output, given the same adapter version and source state — the layout-free codegen path is unchanged by this feature, and any lexical variation arises only from the AI emission step that has always been there
- A project with one layout-bearing page and one layout-free page generates each correctly — layout-driven output for the first, surface-only output for the second
- A page whose layout fails the precheck does not regress into layout-free output for that page; layout-free pages in the same project still generate normally

---

## Buildfile Freshness Gate

**Goal**: Codegen runs only against a buildfile that is up-to-date with the source artifacts (intents, dialogs, surface, domain, layout, adapter version). If sources have changed since the buildfile was produced, codegen refuses to run and points the author at `parlay build-feature` to refresh.
**Persona**: UX Designer
**Priority**: P0
**Context**: Build resolves bindings; codegen consumes them. If the buildfile is stale, codegen would emit code with bindings that don't reflect the current sources — silently producing wrong output. The freshness gate prevents that. It is the only codegen-owned content-error category in this feature; binding-content errors (orphan layout nodes, ambiguous bindings, removed-field references) are owned by the build phase because that is where binding decisions are made.
**Action**: Before emitting code for any layout-bearing page, codegen compares the buildfile's recorded source-state signature against the current source state. On mismatch, codegen refuses to run and surfaces a `stale-buildfile` error pointing the author at `parlay build-feature`. On match, codegen proceeds.
**Objects**: buildfile, freshness, codegen, source-signature, error

**Constraints**:
- The buildfile records a content signature for each source artifact it consumed (intents, dialogs, surface, domain, layout, adapter version). Codegen recomputes those signatures from current sources at runtime and compares
- On signature mismatch, codegen emits `stale-buildfile at <feature>: buildfile reflects <prior-signature>; current sources are <current-signature>. To fix: run \`parlay build-feature <feature>\` to refresh the buildfile, then re-run codegen.` and exits non-zero
- The freshness gate runs **per feature**, not per page — buildfiles are feature-scoped, so a stale buildfile fails generation for every page in that feature
- The freshness signature is content-based, not timestamp-based — touching a source file without changing its content does not trigger a stale-buildfile error
- Layout-validation precheck refusals (sourced verbatim from the layout-creation feature) are surfaced separately from the freshness gate — they are not re-classified as buildfile staleness, and the precheck wins when both apply because the layout itself is invalid
- The freshness check is mechanical (signature comparison); it never invokes AI, never prompts, and is therefore safe in CI
- Errors never auto-edit the layout or the buildfile — codegen surfaces the failure and exits; the human (or `parlay build-feature`) makes the change

**Verify**:
- Running codegen on a feature whose buildfile predates a domain-model edit produces `stale-buildfile` and exits non-zero
- Running codegen immediately after `parlay build-feature` succeeds with no freshness-gate failure
- Touching a source file without changing its content (re-saving with no edits) does not trigger `stale-buildfile`; the signature is content-based
- A precheck refusal from the layout-creation feature is surfaced verbatim without a `stale-buildfile` wrapper, even when the buildfile is also stale (the precheck wins because the layout itself is invalid)
- A two-feature project where feature A has a stale buildfile and feature B is fresh still generates feature B successfully and reports the feature-A failure with code `stale-buildfile`

---

## Codegen Is Non-Interactive and CI-Safe

**Goal**: `parlay generate-code` runs correctly with no human at the terminal — in CI, in pre-commit hooks, in scripted batch runs — without prompts, hangs, or non-determinism. Because all inference now lives in the build phase, codegen has no interactive paths to suppress.
**Persona**: Build Engineer
**Priority**: P0
**Context**: The interactive concern (disambiguation, binding inference) is a build-phase responsibility. Codegen reads pre-resolved bindings from the buildfile and emits framework code; the emission step itself uses an AI agent, but it never prompts because there are no decisions left to make. CI's only codegen concern is that codegen exits cleanly with reliable exit codes, never prompts, and produces consistent output across workers — even though lexical text may vary.
**Action**: Codegen has no TTY-conditional code paths. It reads the buildfile, runs the freshness gate, runs the layout-validation precheck (sourced from the layout-creation feature), and emits code. Any failure produces a non-zero exit; success produces a zero exit. There is nothing for codegen to interact about.
**Objects**: codegen, ci, non-interactive, exit-code, buildfile

**Constraints**:
- Codegen never prompts — there are no interactive paths in codegen by construction. The `--non-interactive` flag is therefore not needed for codegen and is silently accepted for compatibility but has no effect
- Process exit code is non-zero on any error path (stale buildfile, layout precheck refusal); CI's pass/fail is derived from exit code, not from stdout pattern matching
- Generated output is behaviorally equivalent across runs given the same `(layout, buildfile, adapter)`, so concurrent CI runs on the same source state produce code that passes the same testcases and identical exit codes — lexical details may differ because the emitting AI agent is non-deterministic on text, but the CI pass/fail signal and the observable behavior are stable
- The non-interactive build phase, where actual ambiguity-resolution happens, is described separately as part of the build-feature pipeline, not in this feature
- Codegen never writes partial output: if any page fails, the run leaves the output directory in a state consistent with "no run produced these files" so a half-written prototype never reaches CI's verification step

**Verify**:
- Running `parlay generate-code` in a no-TTY container with a valid buildfile produces output identical to running it locally with a TTY on the same source state
- Running codegen on a feature with a stale buildfile in CI produces `stale-buildfile`, exits non-zero, writes no output
- Running codegen on a feature with an invalid layout in CI surfaces the precheck refusal verbatim, exits non-zero, writes no output
- Two CI workers running codegen against the same source state produce identical exit codes and behaviorally-equivalent generated code (passing the same testcases, emitting the same component tree; lexical text may differ between workers)
- Passing `--non-interactive` to codegen has no observable effect — codegen behaves the same way it does without the flag

---
