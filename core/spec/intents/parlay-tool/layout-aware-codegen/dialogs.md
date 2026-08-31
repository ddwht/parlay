# Layout-aware-codegen — Dialogs

---

### Codegen Reads the Layout Block When Present

**Trigger**: A designer (or CI) runs `parlay generate-code` against a project where at least one page artifact carries a `layout:` block authored by Studio. The buildfile produced by the build phase contains the resolved bindings for that page.

User: `parlay generate-code`
System: Loads the buildfile, then for each page artifact reads the `layout:` block alongside the surface. For pages with a layout, resolves each node's `type` against the loaded adapter's component vocabulary and emits framework-specific component instances matching the tree shape. Bindings on each node are read from the buildfile, not inferred. Reports per-page progress: `pages/tasks.md: layout (clarity@17) -> Angular components (3 nodes)`.

#### Branch: Layout block parses cleanly, buildfile is fresh

User: (no further input — codegen runs end-to-end)
System: Emits the generated code. Structural shape of the layout tree is preserved one-for-one: a `clarity.region` with two `clarity.region` children produces a parent component containing two child components, in declaration order. Token references like `spacing-lg` are emitted as adapter-defined token references in the framework code, never as raw pixel values. Exits 0.

#### Branch: Layout fails its precheck — codegen refuses to run for that page

User: `parlay generate-code` (page's layout fails the validation precheck — could be unknown component type, vocabulary version mismatch, or malformed block; the precheck is owned by the layout-creation feature and runs before codegen)
System: Refuses to run codegen for that page. Surfaces the precheck failure verbatim from the validation layer; does not re-classify or augment the message with codegen-internal vocabulary. Points the author back to the layout-creation flow to fix the issue. Other pages continue. Process exits non-zero.

#### Branch: Two AI agents generate code from the same inputs

User: Agent A runs codegen; Agent B runs codegen on the same source state
System: Both produce generated code that passes the same testcases and emits the same component tree. The resolved bindings live in the buildfile produced by the build phase; codegen reads those bindings rather than re-inferring them, so binding decisions are stable across agents. The emission step itself is performed by an AI agent, so lexical details (variable names, comment text, statement ordering) may differ between agents — observable behavior does not.

---

### Codegen Consumes Resolved Bindings From the Buildfile

**Trigger**: A page has a layout block, and codegen needs to wire each layout node to its data source and operation handler. The bindings have already been resolved during the build phase and recorded in the buildfile.

User: `parlay generate-code` (Tasks page: `Task` entity with `status` enum, layout has a `clarity.datagrid` whose status column declares `contentShape: badge`, and a `clarity.button` labeled "Create task"; buildfile has bindings recorded for every layout node)
System: For each layout node, looks up the binding entry in the buildfile. The status column's binding maps to `Task.status` with `presentation: badge`; the create button's binding maps to `createTask`. Emits Angular + Clarity code that wires the datagrid to a `Task[]` data feed, renders the status column as a Clarity badge using the enum's presentation tones, and attaches the create button's click handler to `createTask`. No AI invocation, no prompt, no inference work in codegen.

#### Branch: Buildfile contains a (layout-node, surface-fragment, domain-element) traceability triple

User: `parlay generate-code`
System: Preserves the source triple in the generated code as a comment or annotation alongside each wired component, so a designer can later trace "why does this column render as a badge?" back to the surface-fragment and domain-element that informed the binding. The triple is read from the buildfile, not reconstructed.

#### Branch: Buildfile lacks a binding for a layout node codegen reaches

User: `parlay generate-code` (the layout has a node — say a freshly added column — that has no binding entry in the buildfile)
System: Refuses to proceed for that page. Treats the missing binding as a freshness-gate failure (see *Buildfile Freshness Gate*) — the buildfile must have been produced before this layout edit. Surfaces `stale-buildfile` and points the author at `parlay build-feature` to refresh.

#### Branch: Re-running codegen on the same buildfile and layout

User: `parlay generate-code` (no edits since the prior run; buildfile and layout unchanged)
System: Reads the same buildfile, reads the same layout, emits code that passes the same testcases. Binding decisions are read from the buildfile and so are stable across runs; the AI agent that emits framework text may produce different lexical output run-to-run, but the wired structure and observable behavior do not change.

#### Branch: A binding has a presentation hint

User: `parlay generate-code` (buildfile records `presentation: badge` for the status column with the field `Task.status`)
System: Emits a Clarity badge for the status column with the entity field name flowing through unchanged into the generated template. The presentation hint comes from the buildfile, not from AI inference at codegen time.

---

### Backward-Compatible Path for Layout-Free Pages

**Trigger**: A project — or a single page within a project — has no `## Layout` section in its page artifact. Codegen must produce the same output it did before this feature shipped.

User: `parlay generate-code` (project has zero layouts authored — designer has not adopted Studio)
System: For each page, checks for a layout block. None present anywhere. Falls through to the existing surface + domain + adapter codegen path. Output is behaviorally equivalent to pre-feature output for the same adapter version and source state — the layout-free path is unchanged by this feature, and any lexical variation arises only from the same AI emission step that has always been there. Existing testcase generation continues to apply.

#### Branch: Mixed project — one page with layout, one without

User: `parlay generate-code` (project has `pages/tasks.md` with a layout and `pages/settings.md` without)
System: Per-page activation. `tasks.md` takes the layout-aware path (reads the buildfile's bindings, emits structure-from-layout output). `settings.md` takes the layout-free path (surface-only output). Both succeed in the same run.

#### Branch: Page has a layout block but it is invalid

User: `parlay generate-code` (page has a `## Layout` section but the precheck rejects it — malformed YAML, vocabulary mismatch, or unknown type)
System: Does NOT silently fall back to the layout-free path — silent fallback would mask a real authoring error. The precheck (owned by the layout-creation feature) refuses, codegen reports the precheck refusal for that page, and other pages in the project still generate.

#### Branch: Designer asks "did anything change for me?"

User: Designer who has not adopted Studio runs `parlay generate-code` after upgrading parlay
System: Produces the same output as before. Documentation surface (CLI help, release notes) makes the opt-in nature visible: layout-aware codegen activates only when a layout block is present.

---

### Buildfile Freshness Gate

**Trigger**: Codegen is invoked on a feature with a layout-bearing page. Before emitting code, codegen verifies that the buildfile's recorded source-state signature matches the current source state.

User: `parlay generate-code`
System: For each layout-bearing feature, recomputes content signatures for every source artifact the buildfile consumed (intents, dialogs, surface, domain, layout, adapter version). Compares against the buildfile's recorded signatures. On match, proceeds with codegen. On mismatch, refuses to proceed and surfaces `stale-buildfile`.

#### Branch: Sources unchanged since the buildfile was produced

User: `parlay generate-code` (immediately after `parlay build-feature`)
System: Recomputed signatures match the buildfile's recorded signatures. Freshness gate passes. Codegen proceeds and emits code.

#### Branch: A source artifact has been edited since the last build

User: Designer edits the domain model, then runs `parlay generate-code` without re-building
System: `stale-buildfile at @studio-support/layout-aware-codegen: buildfile reflects domain-model:abc123…; current sources are domain-model:def456…. To fix: run \`parlay build-feature @studio-support/layout-aware-codegen\` to refresh the buildfile, then re-run codegen.` Exits non-zero. No files written.

#### Branch: Source file touched but content unchanged

User: User re-saves a source file with no actual changes (filesystem timestamp updates, content does not)
System: Recomputed content signature matches the recorded one. Freshness gate passes; no `stale-buildfile` triggered. Signatures are content-based, not timestamp-based.

#### Branch: Precheck failure and stale buildfile both apply

User: `parlay generate-code` (layout has been edited and is now invalid AND the domain model also changed since the last build)
System: Surfaces the precheck refusal verbatim from the layout-creation feature. Does NOT also report `stale-buildfile` for the same page — the precheck wins because the layout itself is invalid. Once the layout is fixed and re-validated, a subsequent codegen run would surface `stale-buildfile` if the buildfile is still out of date.

#### Branch: Two-feature project, one stale and one fresh

User: `parlay generate-code` (project has feature A with a stale buildfile, feature B with a fresh buildfile)
System: Reports `stale-buildfile` for feature A, generates feature B successfully, exits non-zero because at least one feature failed. Per-feature isolation: a stale buildfile in one feature does not block another feature's codegen.

#### Branch: Designer asks codegen to "fix it for me"

User: (implicit — designer hopes codegen will refresh the buildfile or rewrite the layout)
System: Never auto-runs `parlay build-feature` and never auto-edits the layout. The error message points at the offending feature and tells the human (or `parlay build-feature`) to refresh; the human (or Studio) makes the change.

---

### Codegen Is Non-Interactive and CI-Safe

**Trigger**: `parlay generate-code` is invoked without a human at the terminal — in CI, in a pre-commit hook, or via a scripted batch run.

User: CI runner: `parlay generate-code`
System: Codegen has no TTY-conditional behavior and no interactive paths. It reads the buildfile, runs the freshness gate, runs the layout-validation precheck (sourced from the layout-creation feature), and emits code via an AI agent — but with no decisions left to make at codegen time, the agent never prompts. No prompts, no hangs, no binding inference. Failures exit non-zero; success exits 0.

#### Branch: Clean run

User: `parlay generate-code` (project resolves cleanly: fresh buildfile, valid layouts)
System: Produces output behaviorally equivalent to a local run on the same source state — same testcases pass, same component tree. Lexical text may differ because the emitting AI agent is non-deterministic on text; the CI pass/fail signal does not depend on that. Exits 0.

#### Branch: Stale buildfile in CI

User: `parlay generate-code` in CI on a feature whose buildfile is older than its sources
System: `stale-buildfile at @studio-support/layout-aware-codegen: ... To fix: run \`parlay build-feature ...\` to refresh the buildfile, then re-run codegen.` Exits non-zero. No files written. The fix is for the upstream pipeline to run `parlay build-feature` before `parlay generate-code` (typically by making build a separate CI step).

#### Branch: Invalid layout in CI

User: `parlay generate-code` in CI on a feature whose layout fails the precheck
System: Surfaces the precheck refusal verbatim from the layout-creation feature. Exits non-zero. No files written. Identical message regardless of TTY presence.

#### Branch: --non-interactive flag

User: Developer or CI passes `--non-interactive` to codegen
System: Silently accepts the flag for compatibility, but it has no effect — codegen has no interactive paths anyway. Output and exit code are identical with or without the flag.

#### Branch: Concurrent CI runs

User: Two CI workers run `parlay generate-code` against the same source state
System: Identical exit codes. Generated code from both workers passes the same testcases and emits the same component tree; lexical details may vary because the AI agent that emits framework text is non-deterministic on text. Binding decisions come from the buildfile and so are stable across workers. The CI pass/fail signal stays consistent.

#### Branch: Partial-write avoidance

User: `parlay generate-code` (page A succeeds, page B fails the freshness gate)
System: Leaves the output directory in a state consistent with "no run produced these files" — either every page succeeded, or no new files were written. A half-written prototype never reaches CI's verification step.

#### Branch: Exit code is the source of truth

User: CI script branches on `parlay generate-code`'s exit code
System: Exit code is non-zero on any error path. CI's pass/fail is derived from the exit code, not from stdout pattern matching.

---
