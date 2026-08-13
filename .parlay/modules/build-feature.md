# build-feature

_Generate buildfile and testcases_

<!--
parlay-section: cross-cutting
parlay-extends: studio-support/layout-aware-build/disambiguation-prompt
parlay-extends: studio-support/layout-aware-build/two-pass-binding-resolution
parlay-extends: studio-support/layout-aware-build/starter-rule-set-and-project-extension
parlay-extends: studio-support/layout-aware-build/interactive-disambiguation-choice-recording
parlay-extends: studio-support/layout-aware-build/buildfile-bindings-section
parlay-extends: studio-support/layout-aware-build/headless-build-mode
-->

# Build Feature

Generate buildfile.yaml and testcases.yaml for a feature using the configured framework adapter.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Asking the user

This skill runs as a **phase module** — normally inside a parlay-loop subagent, where no interactive tool exists. A question asked there is written into a transcript nobody reads, and you then answer it yourself; that is not a confirmation, it is a decision made on the user's behalf. So do not prompt. **Stop and return a decision request** as your final output. The driver prompts and resumes you with the chosen `id`, with your context intact, so you continue exactly where you stopped.

````
```yaml parlay-decision
kind: phase-boundary        # phase-boundary | override | overwrite | failure | ambiguity | impasse
phase: <the phase you are in>
question: "<the one question, in the user's terms>"
context: |
  <what you found, and what is already on disk>
options:
  - id: <slug>
    label: "<what the user picks>"
    detail: "<the consequence, when it isn't obvious>"
default: <id>               # advancement kinds ONLY — see below
resume: "Re-enter with decision: <id>. <what is written so far>"
```
````

**The `default:` field.** It names the one option id a driver running `--non-interactive` may take without asking. It exists so an unattended run has a defined answer rather than an inferred one, and it must be an id from your own `options:` list.

Only the two advancement kinds may carry a default: `phase-boundary` (normally `proceed`) and `override` (your recommended set). Those are decisions where one answer is the recommendation and the others are the user electing to intervene — taking the recommendation unattended is what the user asked for by passing the flag.

The other four kinds must NOT carry one, and a driver must abort rather than invent one, because on each of them every available answer is wrong in a way the user would want to know about:

- `ambiguity` — the protocol already forbids resolving one by taking the cheapest reading. A flag must not become the exception that makes it allowed.
- `overwrite` — one answer destroys work that may have been hand-edited; the other ships a prototype that diverges from its spec. There is no safe default, only a choice about which loss is acceptable.
- `failure` — the safe-looking answer proceeds past a suite that did not pass, which is the one outcome a CI run exists to prevent.
- `impasse` — the pipeline cannot express what the spec asks for, and the offered way forward hands the work to a person permanently. Accepting that is a scope reduction nobody can consent to on the user's behalf.

So: when you raise one of those four, omit `default:`. Adding one does not make the run smoother; it makes an unattended run take an action nobody authorized.

**`impasse` vs `ambiguity`.** An ambiguity has two readings and you cannot pick between them; an impasse has none — the pipeline has no way to express what the spec asks for, whichever reading you take. They are separate kinds because their resolutions differ in kind: an ambiguity is settled by the user choosing a reading, an impasse by the user agreeing that this part of the system will be written by hand, declared as a unit, and never generated. Filing an impasse as an ambiguity offers the user a choice between readings that all fail.

Leave the filesystem coherent before you stop — a decision is a pause, not a half-write. If you genuinely cannot pause at that point, take the option that preserves the user's work, never the one that destroys it, and say so in your report.

Two things not to do: never narrow the options to spare the user a question, and never resolve an ambiguity by taking the reading that is cheapest to implement. Both turn a decision the user should own into one you made quietly.

## Recording what happened (feedback mode)

When feedback mode is on, this project records what actually happened during a run so the toolkit can be improved from evidence rather than recollection. It is **off by default**; when it is off every command below is a silent no-op, so call them unconditionally and never branch on whether it is enabled.

**The log is written to be sent.** A user turns this on, reproduces a problem, and forwards the file to whoever maintains the toolkit. So nothing you pass can be free text: every flag below takes a value from a closed vocabulary, and anything else is replaced with `redacted` before it reaches the file. Do not try to describe a situation in words — pick the closest vocabulary value and, if none fits, use `other`. How often `other` shows up is itself the signal that a vocabulary needs a new member.

The CLI already records its own half: every command's outcome and duration, and every diagnostic any validator produced. **Do not re-report those.** Record only what the CLI cannot see — what you did and why:

```
parlay internal feedback-record --kind <kind> --skill <this-skill> [--phase P] [--artifact A] [...]
```

| Kind | Record when | Flags |
|---|---|---|
| `phase` | You enter or leave a pipeline phase | `--phase intents\|dialogs\|artifacts\|build\|code` |
| `decision` | You raised a `parlay-decision` block, and again when it resolves. The CLI never sees these | `--decision <kind>` and, on resolution, `--option <id>` |
| `retry` | **The important one.** You authored something, had it refused, and tried a different shape | `--code <the error code>` and `--changed added-field\|removed-field\|changed-shape\|changed-version\|changed-artifact\|reordered\|other` |
| `improvised` | You proceeded without a rule you needed — invented a path, guessed a convention, weakened an assertion | `--needed schema-rule\|path-convention\|naming-convention\|adapter-capability\|example\|decision\|other` |
| `note` | Anything else worth a future reader knowing. Use sparingly | — |

`--subject` optionally names the feature, unit or operation concerned. Pass it in **plaintext**; the CLI hashes it on receipt with a per-project salt. Never hash it yourself.

**`retry` and `improvised` are the two the log exists for.** A validator that teaches by rejection looks exactly like one that teaches by documentation unless the retries are counted, and an agent that guessed a convention leaves no other trace at all — the run passes, and the guess surfaces later as an inconsistency nobody can date. Recording them is not an admission of failure; it is the only way the gap that forced them gets closed.

**Correlation is automatic — do not manage it.** Events are tied together by `PARLAY_RUN_ID`, which the loop driver sets once per pipeline run and every CLI call inherits from the environment. The CLI hashes it before writing, so the value never appears in the log. You do not need to read it, pass it, or thread it through; `--run` exists only to override it and is almost never the right thing to reach for.

## Steps

1. **Load schemas** — Read these files before generating:
   - `.parlay/schemas/buildfile.schema.md`
   - `.parlay/schemas/testcases.schema.md`
   - `.parlay/schemas/adapter.schema.md`
   - `.parlay/schemas/surface.schema.md`
   - `.parlay/schemas/intent.schema.md`
   - `.parlay/schemas/dialog.schema.md`
   - `.parlay/schemas/blueprint.schema.md`

2. **Load project config** — Read `.parlay/config.yaml` for project settings. Resolve the adapter from `.parlay/adapter-set.yaml` — the presentation slot for UI work, the kind-appropriate slot otherwise. `prototype-framework:` in config.yaml is deprecated (removed in v0.3); fall back to it only when no adapter-set exists.

3. **Load framework adapter** — Read `.parlay/adapters/{framework-slug}.adapter.yaml` for framework-specific vocabulary (component types, element types, action types, layout patterns, file conventions).

4. **Load application blueprint** (if exists) — Read `.parlay/blueprint.yaml` for app-level architectural decisions. Use it to:
   - If `authorization` defines roles/guards: include role-aware user data in fixtures (e.g., an admin user and a regular user). Add guard-related elements to components on guarded routes (e.g., an "unauthorized" redirect or message element with `visible-when: user not authenticated`).
   - If `authorization` defines policies: add policy-check conditions to action `enabled-when` fields where the policy's `controls` matches the action's domain (e.g., `enabled-when: policy:manage-discussions passes` for a delete button).
   - If `data` defines caching invalidation rules: use them to inform `effect:` targets on mutation actions (e.g., the invalidation scope tells you which queries/lists are affected by a create/delete).
   - The blueprint is optional — if it doesn't exist, proceed without it (the agent will invent these decisions during code generation, as it did before).

5. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/surface.md` or `surface.yaml` (if exists)
   - `spec/intents/{feature}/capabilities.yaml` (if exists)
   - `spec/intents/{feature}/infrastructure.md` (if exists)
   - `spec/intents/{feature}/domain-model.md` or `domain-model.yaml` (if exists)
   - `.parlay/build/{feature}/design-spec.yaml` (if exists — generated by `/parlay-reference-design-spec` from Figma)
   - A feature needs at least one of `surface.md` (or `surface.yaml`), `capabilities.yaml`, or `infrastructure.md` — the four spec artifacts are co-equal — `surface.yaml` (or legacy `surface.md`), `capabilities.yaml`, `infrastructure.md`, and the project's `domain-model.yaml` — none is a stand-in for another. Features that are purely user-facing have only `surface.md`; features that expose operation-shaped backend behavior have `capabilities.yaml`; features that introduce architectural prose (boundaries, probes, allowlists, dependency pins) have `infrastructure.md`; many features have several of these in combination.

6. **Check readiness** — Run: `parlay internal check-readiness @{feature} --stage build-feature`
   - This returns JSON with errors (blocking) and warnings (non-blocking)
   - If any errors are reported, present them to the user with the suggested fixes and stop — do not proceed to generation
   - If only warnings are reported (e.g., open questions), inform the user and ask whether to proceed

7. **Compute the diff** — Run: `parlay internal diff @{feature}` to find out what changed since the last build.
   - On the first build (`first_build: true`) or when the buildfile doesn't exist yet (`has_buildfile: false`), generate the entire buildfile from scratch.
   - On subsequent builds, the JSON output reports:
     - `components.stable[]` — components whose source dependencies (fragment, referenced intents, matching dialogs) all have unchanged hashes. **Preserve these entries verbatim** in the new buildfile — do not regenerate them.
     - `components.dirty[]` — components whose dependency chain has changes. Regenerate only these components, using `changed_sources` as a hint about what to focus on. Values prefixed with `design-spec:` indicate the component's design-spec entry changed (new Figma extraction, updated layout, changed tokens) — regenerate using the updated design-spec values, especially layout and widget refinements.
     - `components.removed[]` — components whose source fragment no longer exists. Drop these entries from the new buildfile.
     - `surface_fragments.new[]` — fragments in the current surface that aren't in any existing component. Decide whether to introduce new components for them.
   - Tell the user what's about to be regenerated before doing it (e.g., "Regenerating 2 components: upgrade-prompt, preflight-check. Keeping 5 stable components.").

7.5. **Integration discovery** — Before writing the buildfile, decide where each component lives in the source tree and which existing files each cross-cutting entry must touch. The buildfile carries this decision in its `plan:` section (see step 8); this step is the analysis that produces it.

   For each component, identify the file path the adapter's `file-conventions.component-pattern` and `naming` rules imply. New components produce new files (`plan.creates`). When the adapter's pattern is "extend a shared file" (e.g. one component contributes a function to a shared dispatcher), the file is shared and the component's plan entry is a `plan.modifies` whose `sources` lists every contributing component.

   For each `cross-cutting:` entry whose intent is to modify existing code:
   - Identify the canonical integration site by reading existing source. Use `parlay internal scan-generated <source-root>` to enumerate parlay-managed files, plus regular file reads to inspect surrounding code. The site is the file where similar entities are already wired (for example, the file that already registers existing entry-points, the file that owns the resolver, the file that owns the deployer registry).
   - Record the exact path in the entry's `target-files:`. Avoid `target-pattern:` for known-singleton sites.
   - When uncertain, raise an `ambiguity` decision request (see **Asking the user**): "This feature introduces a new <thing>. Where should it integrate?" with one option per candidate site from the scan, plus an option to specify a path.
   - Do NOT default to creating a new package when an existing file is the right home. The strict-target rule in `/parlay-generate-code` step 14.7 will refuse to invent paths under source-root anyway — it is cheaper to identify the site here than to discover the mismatch at code-generation time.

   For purely-introducing cross-cuttings (a new package is genuinely warranted), pick a path consistent with the adapter's `file-conventions.source-root` and naming rules, and list it in `plan.creates`. The agent is responsible for the choice; record it explicitly in `plan` rather than letting generate-code guess.

7.6. **Detect interactive vs headless mode** — Before any Pass-2 work that might prompt, decide once whether this run is interactive or headless. The skill MUST consult both signals:
   - **TTY check** on the controlling stdin.
   - **Explicit `--non-interactive` flag** on the `parlay build-feature` invocation.

   The flag wins over TTY detection in BOTH directions: with `--non-interactive` set, the skill runs as if headless even when a TTY is attached (so a developer can test CI behavior locally); without it, a missing TTY still triggers headless behavior automatically. Record the resolved mode for the rest of the run; per-node fallback is not allowed.

   **The loop threads its own `--non-interactive` into this phase**, and the two meanings agree rather than compete. This skill's flag decides whether Pass-2 ambiguity prompts or hard-errors; the loop's decides whether a decision request is auto-answered or aborts the run. Both resolve the same underlying question — is there a human here — and both answer an unresolvable case by refusing rather than guessing: headless ambiguity emits `ambiguous-binding` and fails, and the loop aborts on an `ambiguity` decision with exit 11. So treat the loop's flag as setting this one.

7.7. **Resolve layout bindings (two-pass)** — For every layout-bearing page in the active feature, after layouts are loaded and surface + domain are read, walk every layout node that consumes data or emits an action through two ordered passes.

   **Rule load.** Compute the active rule set: starter rules merged with project rules from the existing buildfile's `wiring.rules:` (if present). The starter rule set is the only other source; no rules are loaded from surface, domain, layout, or any other artifact. Apply rule-load-time checks before Pass 1 runs:

   - `rule-conflict` — two rules at the same precedence produce different bindings → build error naming both rule definitions.
   - `rule-precedence-error` — a project rule placed below a starter rule for the same match → `rule-precedence-error: project rule <name> attempts to silently disable starter rule <starter-name>. Projects can override starter rules at higher precedence, but cannot place rules below them.`
   - `rule-load-error` — a rule's `match` predicate references a non-existent domain field → `rule-load-error: rule <name> references domain field <Task.priority> that does not exist in the active feature's domain.`
   - `rule-termination-error` — a rule's `bind` output would re-trigger another rule (or itself) → `rule-termination-error: rule <name> produces a binding that re-triggers <other-rule-name>` — checked statically, not at match time.

   **Starter rule set vocabulary** — three families cover the common cases:
   - **structural-hint matches**: a layout-node property like `contentShape: badge` maps to a matching surface Show field.
   - **action-verb matches**: a button label or aria semantics combined with a surface `Action` maps to a domain operation.
   - **single-candidate matches**: when surface declares exactly one Action and the domain has exactly one operation of matching shape, the binding is unambiguous.

   Adding or removing a starter rule requires a build-feature schema bump.

   **Pass 1 — deterministic rules:**
   - Apply rules in precedence order (highest first).
   - **Single match** — record the binding with `confidence: rules` and the firing rule's qualified name (`starter/<name>` or `project/<name>`).
   - **Zero matches with a binding-expecting layout shape** (e.g. a `clarity.datagrid` with no Show fragment) — emit `orphan-layout-node` build-time error. Do NOT escalate to Pass 2; never invent a binding from nothing.
   - **Zero matches with a non-binding-expecting shape, OR multiple matches at distinct precedences** — escalate to Pass 2 with the Pass-1 candidate set (possibly empty for the orphan-eligible shape).

   **Pass 2 — AI matcher:**
   - Invoked ONLY on nodes that Pass 1 did not resolve unambiguously. Pass 2 must never run on a Pass-1-resolved node.
   - Receives exactly the candidate set Pass 1 produced (or the empty set for orphan-eligible shapes). The matcher never invents candidates outside this set.
   - **Inference is bounded to the active feature**: cross-feature surface fragments and domain elements are NEVER candidates.
   - **Single high-confidence pick** — record the binding with `confidence: ai`, the AI session/run identifier, and the candidate list at the moment of decision.
   - **Multiple candidates within the configurable confidence threshold**:
     - In **interactive mode**: transfer control to the disambiguation prompt (see step 7.8).
     - In **headless mode**: emit `ambiguous-binding` build-time error (see step 7.9). Never block waiting for input.

   **Determinism contract.** Re-running build on identical inputs records bindings whose source triples are stable. Lexical AI reasoning text may differ run-to-run; the recorded triple `(layout_node, surface_fragment, domain_element)` is what defines the decision deterministically.

   **AI participation rule.** AI participation in a binding decision happens at build time only. Codegen reads the buildfile's `bindings:` section and never invokes the rules engine, the AI matcher, or the disambiguation prompt.

7.8. **Interactive disambiguation prompt** (interactive mode only) — When Pass 2 leaves ambiguity in interactive mode (TTY present, `--non-interactive` not set), pause per-node and surface the prompt described by the `disambiguation-prompt` surface fragment. Multiple ambiguities in one run produce prompts one at a time in deterministic `(page-path, node-path)` lexicographic order — not as a batched multi-select.

   **Prompt format (fixed):**

   ```
   ambiguous binding at:
     <page-path> > <layout-node-path> ("<node-label>")
   candidates:
     [1] <domain-element-ref>  (ai-confidence: <value>)
     [2] <domain-element-ref>  (ai-confidence: <value>)
     ...
     [q] quit (abort build, exit non-zero)
     [s] skip (record as `unresolved`, continue, buildfile will be invalid)
   choose >
   ```

   The prompt component (slug `disambiguation-prompt`) is a fragment of this skill's interactive flow — it does NOT produce a standalone CLI file. Its emission point is this subsection of build-feature.

   **Three input shapes accepted:**

   - **Numeric digit** — selects a candidate. Record the chosen binding with `confidence: designer`, the source triple, the timestamp of the choice, and the candidate list as it existed at the moment of selection. Continue to the next ambiguity (or complete the build if none remain).
   - **`q` (quit)** — abort the build with a non-zero exit code. No partial buildfile is written; recorded choices made earlier in the same run are also discarded. The buildfile-output directory is left consistent with "no run produced these files."
   - **`s` (skip)** — record the binding as `unresolved` and continue the build. The resulting buildfile fails its own validity check (signaling that codegen should refuse to consume it), but lets the designer collect multiple decisions before re-running. End-of-run summary lists every `unresolved` binding.

   **Persistence and lifecycle.**

   - A recorded `confidence: designer` binding is read as authoritative on subsequent build runs as long as its **candidate list is unchanged** (same domain operations, same surface action shape, same layout node).
   - A change in the candidate list (operation rename, removal, addition; surface action shape change) invalidates the recorded choice and re-triggers the prompt with the updated list. The recorded candidate list is what determines invalidation, not the AI confidence values.
   - Recorded choices live in the buildfile only — never in surface, domain, or layout artifacts. Designer decisions are build-time state, not authoring artifacts.
   - Adding a new ambiguous node to the layout produces a fresh prompt for that node only; previously recorded choices for unchanged nodes are preserved.

7.9. **Headless ambiguity error** (headless mode only) — In headless mode (TTY absent OR `--non-interactive` set), Pass-2 ambiguity that would have surfaced the disambiguation prompt instead emits `ambiguous-binding` to stderr in the format:

   ```
   ambiguous-binding at <feature> > <page> > <node-path>:
     candidates:
       <domain-element-ref>  (ai-confidence: <value>)
       <domain-element-ref>  (ai-confidence: <value>)
     expected: exactly one match
     to fix: rename the layout-node label to disambiguate, narrow the surface Action that maps to this node,
             add a wiring rule under wiring.rules, or run `parlay build-feature` interactively
             to record a designer choice.
   ```

   Exit code: **non-zero**. The error never blocks waiting for input.

   **Mode-invariant errors.** `orphan-layout-node` and `removed-field-referenced` errors behave identically across modes — actionable error message plus non-zero exit. They never gate on the disambiguation prompt; they fire whether or not a TTY is attached.

   **Pass 2 AI inference is still allowed in headless mode** — only the escalation-to-prompt path is rejected. A node where Pass 2 returns a single high-confidence candidate records the binding with `confidence: ai` and continues.

   **Recorded designer choices** from prior interactive runs are read as authoritative in headless mode and are not re-prompted, as long as their candidate lists are unchanged.

   **Atomicity invariant.** The non-interactive path NEVER writes a partial buildfile. Either the buildfile is committed whole or nothing reaches the output directory. If any binding cannot be resolved on any layout-bearing page, the buildfile-output directory is left in a state consistent with "no run produced these files." Implementations have flexibility (atomic temp-file-rename, in-memory accumulation then single write, write-then-fsync-then-rename); the invariant is "either fully committed or not present," not a specific mechanism.

   **CI contract:**
   - Process exit code is the source of truth: zero on success, non-zero on any error path.
   - CI scripts MUST NOT pattern-match stdout/stderr text. Wording may change across versions; only the exit code is stable.
   - Two CI workers running against the same source state produce buildfiles whose recorded source triples are identical for every binding (including any AI-resolved ones). Lexical AI reasoning text may differ; the recorded triples are stable.
   - This rule governs build-feature's own output only. `create-domain-model`'s greenfield-stub message is a deliberate, narrow exception — its wording is pinned stable on purpose for `studio-cli-hooks` to pattern-match — see that skill's step 6 for why. Don't generalize this exception; don't "fix" that one to match this rule.

8. **Generate buildfile.yaml** at `.parlay/build/{feature}/buildfile.yaml` (tool-internal location — designer never sees this):
   - Set `feature:`, `schema_version: 1`, and `adapter:` (or `adapter-set:` for multi-target projects) fields
   - Do NOT populate `models:` — it's deprecated. Entities live in `domain-model.yaml`; a non-empty `models:` fails validation with `buildfile-models-deprecated`. Reference domain entities by name from `domain-model.yaml` wherever the buildfile needs to name one (fixtures, data.inputs.model).
   - Create `fixtures:` with representative sample data
   - **Mark exactly one fixture `composes: true`** — the one whose data the *running prototype* boots with. A feature's other fixtures are scenarios and are supposed to disagree with it (an empty state and a populated one are the same ids at different moments); the composing one is the feature's contribution to the single dataset every feature shares. Pick the fixture your `scope: route` suite uses — that suite already means "everything this route renders", which is the same question. See `testcases.schema.md` § The composing fixture.
   - Map each surface fragment to a `component:`:
     - Look up each Show in the fragment's `**Shows**:` field in the adapter's `shows:` section → write the adapter's widget name as the element's `widget:`
     - Look up each Action in the fragment's `**Actions**:` field in the adapter's `actions:` section → write the adapter's widget name as the action's `widget:`
     - Look up the fragment's `**Flow**:` (if present) in the adapter's `flows:` section → write the adapter's pattern name as the route's `flow-pattern:`
     - Set the component's primary `widget:` from the adapter's action mapping for the dominant action (e.g., `invoke` → the adapter's mapped widget for invocation)
     - Define `data:` inputs and computed values
     - Define `file-operations:` (file reads, writes, directory creation) — named `file-operations:`, not `operations:`, to avoid colliding with the top-level multi-target `operations:` block described below
     - The buildfile must NOT contain surface vocabulary terms. Only framework-specific widget names from the adapter.
   - **When design-spec.yaml exists** — for each fragment that has a corresponding entry in `design-spec.yaml.fragments`:
     - Use the `widget` field to refine the component's `widget:` value (more specific than the adapter's generic mapping)
     - Design-spec no longer carries structural layout properties (column widths, flex ratios, alignment) — that's `<page>.layout.yaml`'s job now (see step 7.7's binding resolution and `layout.schema.md`). If the feature has a layout for this page, its structure has already been resolved into `bindings:`; do not look for it in design-spec.
     - Use `tokens` to add token references to elements — including `motion` tokens, which design-spec is the only source for. The design-spec has already mapped values per the adapter's `design-system` source rules:
       - `source: figma` — use the design-spec values directly (authoritative Figma tokens).
       - `source: framework` — the design-spec values are already framework token references (mapped during extraction). Use them as-is. If a value does not look like a recognized framework token, discard it and fall back to the adapter's default guidance. Never write raw hex values, Figma token names, or custom CSS classes for framework-sourced categories.
       - `source: not-defined` — use the design-spec values as supplementary guidance.
     - Use `variants` to add state-specific elements (loading skeleton, error result, empty illustration) with appropriate `visible-when` conditions
     - Use `spacing` and `colors` to add style values to elements
     - Apply `shared` values from the design-spec to all fragments, unless a per-fragment value overrides
   - When design-spec.yaml does not exist: proceed exactly as before — adapter defaults apply, agent uses its judgment
   - Define `routes:` mapping commands to components. The buildfile does not need to know whether the project is greenfield or brownfield — mount resolution happens entirely at generate-code time based on whether target page files already exist in the source tree.
   - Use intent Priority to guide component ordering and emphasis (P0 intents produce primary components)
   - **When infrastructure.md exists** — translate each infrastructure fragment into a `cross-cutting:` entry via the **adapter bridge**. Infrastructure fragments are framework-agnostic (Affects / Behavior / Invariants); the buildfile entry is framework-specific. The translation is a resolution step, not a 1:1 field rename:
     - Read `Affects:` to determine the abstract scope of the codebase the capability touches (e.g., "feature resolution", "validation pipeline").
     - Consult the adapter's `file-conventions` and `coding-conventions` to know how that scope is organized in the current framework.
     - Scan the existing source tree to find concrete files matching the abstract scope. Emit them as `target-files:` (explicit paths) or, for fan-out changes, as `target-pattern:` (a grep pattern that resolves to a file list at generate-code time). If `Affects:` resolves to zero files, raise an `ambiguity` decision request asking which files are affected — never guess.
     - Read `Behavior:` to understand the capability, and emit a framework-specific `transform:` describing what the code must do (the basis for Tier 2 intelligent merge).
     - Infer `introduces:` (new functions/types/constants) from `Behavior:` plus the adapter's naming and structure conventions.
     - Carry `Source:` through verbatim as `source:`.
     - Each `Invariants:` bullet seeds one testcase for the resulting cross-cutting entry — feed them into testcases.yaml in step 9.
     - `Caching:`, `Backward-Compatible:`, and `Notes:` carry through as hints embedded in `transform:` or as separate buildfile fields.
     - Infrastructure fragments do NOT produce `components:` or `routes:` entries — only `cross-cutting:` entries.
     - If a feature has both `surface.md` and `infrastructure.md`, the buildfile has both `components:` (from surface) AND `cross-cutting:` (from infrastructure).
     - The `cross-cutting:` section follows the same diff lifecycle as `components:`. `parlay internal diff` reports each entry as `stable`, `dirty`, or `removed`. Generate-code preserves stable entries and re-applies dirty ones.
   - **Emit the `plan:` section** — derived deterministically from `components:` + `cross-cutting:` + the integration sites identified in step 7.5:
     - `plan.modifies` — for every cross-cutting entry whose `target-files:` names existing files, add one entry per file with `sources: [cross-cutting/<id>]`. Multi-component shared files merge entries (same `path`, multiple `sources`).
     - `plan.creates` — one entry per `components:` whose generated file path doesn't already exist (the typical case for new component files). One entry per cross-cutting whose `target-files:` is empty (purely-introducing). `sources` cites the producing component or cross-cutting id.
     - `plan.deletes` — one entry per id in `components.removed[]` from the diff. `sources` cites the removed component id.
     - **Multi-target projects** nest the plan under `plan.targets.<kind>.creates`/`.modifies`/`.deletes` — one sub-plan per filled adapter-set slot, each pathed under that slot's `root:`. The presentation slot's rows come from `targets.presentation.components:` as above; the `application` slot's rows are the per-feature backend files (module/controller/service) implied by the `operations:` block; the `persistence` slot's rows are the shared schema derived from the domain entities. Run `parlay internal scaffold-plan @{feature}` to derive these per-target rows mechanically from the adapters' `file-conventions.paths` and the adapter-set roots, and `--compare` to confirm the emitted `plan.targets` agrees with the derivation. See the "Multi-target buildfiles" section below.
     - The plan is required for new buildfiles. Do not emit a buildfile without `plan:` — `/parlay-generate-code` will reject it.
   - **Preserve the `wiring:` section verbatim.** Project-specific binding rules live in `wiring.rules:` (see buildfile.schema.md). Carry the existing section through unchanged from the prior buildfile (the rules are designer-authored intent, not build-time state). When no prior buildfile exists, omit the section — the build agent applies only the starter rule set.
   - **Emit the `bindings:` section** — finalize step. After every layout-node decision is final (rules / ai / designer) from steps 7.7–7.9 and before the buildfile is committed, write one entry per layout-bearing-page node. Each entry carries the source triple, presentation hints typed against the active adapter, the `confidence:` annotation (`rules` / `ai` / `designer`), and provenance (rule name / AI session / timestamp + recorded candidate list). Run the validity check before committing:
     - Every layout-bearing-page node has a `bindings:` entry.
     - Presentation hints are known to the active adapter — unknown hints raise `unknown-presentation-hint` at finalize time, not deferred to codegen.
     - `confidence:` is one of the three permitted values.
     - Refuse to commit a buildfile that fails any of these checks.
   - **Emit the `source-signatures:` section** — the last thing written before the buildfile is committed. Run `parlay internal scaffold-signatures @{feature}`. It hashes every source artifact this feature actually has and writes the block in schema order, omitting fields for artifacts that don't exist. Do not hand-write these values: they are sha256 sums, and a signature naming an artifact the feature lacks — or missing for one it has — makes the codegen freshness gate wrong in the direction that stays quiet, since a gate with bad recorded inputs passes rather than fails. See `buildfile.schema.md`'s Source-signatures section for what each field means.

9. **Generate testcases.yaml** at `.parlay/build/{feature}/testcases.yaml` (tool-internal — drives cross-validation and feeds spec generation, never handed off to engineering):
   - One test suite per component
   - **Set `file:` on each suite to the path the plan already derived for it.** `parlay internal scaffold-plan` expands the adapter's `file-conventions.paths.test` template for every component and emits the resulting path as a `plan.creates` row. That row is where the suite's code goes. Read it; do not decide a location here and do not leave the question open — an unanswered path question does not disappear, it gets answered further downstream by whoever hits it first, and `generate-code` hitting it has no adapter conventions in view and invents one. Two components' tests then land in two different places in the same project.
   - If the adapter declares no `file-conventions.paths.test` template, `scaffold-plan` reports it as undecidable. That is the signal to fix the adapter, not to guess a path here.
   - **Skip a suite whose invariant a hand-authored unit already satisfies.** Run `parlay internal check-coverage @{unit}` for each declared unit, or read the unit's `satisfies:` list directly. An invariant listed there is covered by a test a person maintains; generating a second suite for it produces either a duplicate or — more often, because the generated suite cannot see the unit's internals — a vacuous one that asserts nothing and passes forever. Record the citation on the suite that would have covered it rather than emitting the suite.
   - Set `intent:` on each suite to `@{feature}/{intent-slug}` for traceability
   - **Derive test assertions from the contract artifacts' `verify:` fields first** — the operation's `verify:` in capabilities.yaml for operation suites, the fragment's `verify:` in surface.yaml for component suites. Fall back to the intent's **Verify** bullets only for entries carrying no `verify:` (artifacts predating the field; `parlay migrate-verify` relocates them). When both exist, `verify:` wins — it is the reviewed contract; the intent bullets are its history.
   - Cover: rendering, element presence, visibility conditions, actions, state transitions
   - Reference fixtures from the buildfile
   - Follow the testcases schema exactly

10. **Validate** — Run all (use `--json` flag for structured error parsing):
   - `parlay internal check-buildfile @{feature}` — feature-ref-aware validation that auto-resolves the buildfile path and adapter, runs deep cross-reference + adapter-vocab + plan-integrity checks, and emits structured JSON. Errors block; warnings are advisory.
   - `parlay validate --type yaml --json .parlay/build/{feature}/testcases.yaml` — testcases YAML schema only (deep buildfile validation already happened above).
   - **Treat `check-buildfile` errors as blocking.** Common error codes and fixes:
     - `missing-plan` — buildfile lacks the plan: section. The buildfile was written before plan: was required; regenerate via `/parlay-build-feature` (this skill).
     - `component-not-in-plan` / `cross-cutting-not-in-plan` — a buildfile entry has no corresponding plan row. Add a plan.creates or plan.modifies entry whose sources cite the entry.
     - `cross-cutting-target-not-in-plan` — a cross-cutting target-files: path is not represented in plan.modifies. Add the missing plan.modifies row.
     - `plan-modify-target-missing` — plan.modifies names a file that doesn't exist in the source root. Either (a) the file path is wrong (correct it), (b) the entry should be plan.creates instead (this is genuinely a new file), or (c) the integration site was misidentified — re-do step 7.5 to find the real site.
     - `plan-create-collision` — plan.creates names a file that already exists. Surface to the user; either pick a different path or move the entry to plan.modifies.
   - `parlay internal check-composition` — cross-feature fixture coherence. This is the only check that looks at more than one feature, and nothing ran it before, which is how four features came to hold four contradicting versions of the same expense report while every per-feature gate passed. It is cheap and it is the last chance to catch a disagreement before it becomes a prototype that tells a different story on every screen.
     - `composition-fixture-contradiction` — two features' **composing** fixtures give the same entity id different values for the same field, so both values are in the running prototype at once. Resolve it with the designer: one of the two is wrong about the domain, and answering "which" is a design decision, not a merge. Do not reconcile silently.
     - `composition-scenario-fixture-divergence` — the same disagreement, but at least one side is a fixture that does not reach the composed seed, so the two states never coexist at runtime. Reported as a **note**, not a failure. Do **not** renumber or rewrite a scenario fixture to make it agree — a scenario fixture exists to describe a different state, and editing it to match destroys the case it covers. If a fixture is meant to reach the composed runtime, mark it `composes: true` instead.
     - `composition-seed-ambiguous` — no fixture is marked `composes: true` and the route suites do not settle it. Go back to step 8 and mark one.
     - `composition-flow-unsatisfiable` — a `scope: flow` suite asserts on domain state after crossing from one feature's route into another's, and the project has no shared runtime that could carry the write. Reported as an **error** when the adapter declares `file-conventions.paths.store` and a participating feature's plan does not wire it — add the store to that feature's `plan.creates` (or regenerate the plan, which derives the row). Reported as a **note** when the adapter declares no store at all: the framework may simply have no shared runtime, and no better code would satisfy the assertion. Do not weaken the assertion to make it pass — a weakened assertion that goes green is how a broken journey shipped last time, with the only record of it being a comment in generated code.
   - If `check-buildfile` reports errors, raise a `failure` decision request (Revise / Accept and document / Cancel) — do NOT silently commit a buildfile that fails validation.

11. **Report** — Confirm the build specification is ready, mention that the artifacts live under `.parlay/build/{feature}/` (tool internals), and tell the user to run `/parlay-generate-code @{feature}` next to produce the prototype code and run tests.

This skill stops at the build specification. **Do NOT save the baseline or any other build state from this skill.** The baseline (`.baseline.yaml`) and the code-hashes sidecar (`.code-hashes.yaml`) are committed atomically as a unit at the end of `/parlay-generate-code`, only after a successful end-to-end generation. Saving baseline here would commit source state without corresponding code state, breaking the consistency invariant and stranding the feature in a state where `parlay internal diff` reports everything stable but no code exists.

Code generation is a separate skill (`/parlay-generate-code`) so that the buildfile.yaml can serve as a clean context boundary — codegen reads only buildfile + adapter and never reaches back into `spec/intents/`. Do NOT extend this skill to write code or to save state.

## Error Handling

When `parlay internal check-readiness` returns errors:
- `no-intents` — intents.md is empty or missing. Tell user to author intents first.
- `missing-goal` / `missing-persona` — required field missing. Show which intent and ask user to fill it in.
- `no-surface` — surface.md doesn't exist. Suggest running `/parlay-create-artifacts @{feature}` first.
- `fragment-missing-page` — surface fragment has no Page target. Show which fragment and ask user to add it.
- `fragment-missing-source` — surface fragment has no Source. Likely a bug in surface generation; regenerate the surface.
- `no-config` / `no-prototype-framework` — project not initialized. Suggest running `parlay init`.

When `parlay internal check-buildfile @{feature} --json` returns errors:
- `missing-model-reference` — a component references a model that doesn't exist. Either add the model to `models:` or change the input. The error's `context` field shows the component path.
- `missing-component-reference` — a route references a component that doesn't exist. Either add the component or remove it from the route.
- `missing-child-reference` — a component's `children:` references a non-existent component. Add or remove.
- `missing-fixture-model` — a fixture references a model that doesn't exist. Add the model or remove the fixture data.
- `unknown-component-type` — a component uses a type not in the adapter. Either change the type to one supported by the adapter, or extend the adapter.
- `adapter-not-found` — the adapter file is missing. Verify `.parlay/adapters/{name}.adapter.yaml` exists.
- `invalid-yaml` / `invalid-adapter-yaml` — YAML syntax error. Show the error to the user and ask them to fix or regenerate.

For all errors: parse the JSON `errors` array, apply each error's `fix` automatically when possible (e.g., regenerating a section), or present the error and fix to the user when human input is required.

## Section: Multi-target buildfile awareness

<!-- parlay-extends: parlay-tool/multi-adapter/multi-target-buildfile-schema -->
<!-- parlay-extends: parlay-tool/multi-adapter/legacy-buildfile-normalization -->
<!-- parlay-extends: parlay-tool/multi-adapter/migration-command-family -->

When the project has a `.parlay/adapter-set.yaml` with more than the presentation slot filled, build-feature emits the multi-target buildfile shape:

- **`adapter-set:`** names the topology (instead of a top-level `adapter:`).
- **Top-level `operations:`** — canonical declarations keyed by the normalized `@<feature>/operation:<id>` form, projected once from `capabilities.yaml`. Canonical fields (`kind`, `subject`, `input`, `output`, `errors`, `policies`, `steps`) live here and only here — restating them under `targets.<kind>:` fails with `buildfile-target-restates-canonical`.
- **`targets:`** — one entry per filled slot. `targets.presentation` carries `components:` (and client-side `routes:`); `targets.application` / `targets.persistence` / `targets.transport` carry `operations:` as a keyed map (`"@feature/operation:id": { <projection metadata> }`) referencing the canonical declarations — every ref must resolve or `buildfile-target-operation-missing` fires.
- **`owns:` (step ownership)** — inside each backend target's `operations."@f/op:id"`, add an `owns:` list of the steps THIS layer implements, derived deterministically with `parlay internal scaffold-operations @{feature}` (owner = the layer whose adapter's `supports.steps` lists the step). The container MUST be named `owns:`, not `steps:` — `steps` is a canonical field and restating it under a target fails `buildfile-target-restates-canonical`. Ownership is what lets codegen split responsibility (each target implements its owned steps, delegating others across links) and what the cross-kind edge extractor reads.
- **`plan.targets.<kind>`** — the per-target plan (see the plan-emission step above); derive it with `parlay internal scaffold-plan @{feature}`.

`parlay validate --project` runs the multi-target deep rules on this shape: canonical-once, operation-ref resolution, per-slot `supports:` coverage, and cross-kind link enforcement (every UI→application→persistence edge implied by the buildfile must be authorized by the adapter-set `links:` block).

### Pre-codegen support gate

Before any AI invocation, run `parlay internal check-supports @{feature}`. The CLI checks every operation term by **union coverage** — a term passes if at least one filled backend adapter supports it — plus the per-adapter shape/vocabulary check. It emits structured JSON and exits non-zero on any failure. Skill MUST stop on non-zero — surface the `issues[]` array to the designer with the relevant `adapter-supports-missing-<kind|step|policy|error>` codes; do NOT proceed to emit the buildfile. Union coverage means each adapter need only support its own layer's terms (nest owns `validate-input`/`return-*`, prisma owns `create-one`/`read-many`); a term fails only when **no** backend layer owns it. The check is mechanical: signature comparison only, no AI invocation. Presentation-only projects (no `.parlay/adapter-set.yaml` with non-presentation slots) get `ready: true` automatically.

**When the gap is not fixable by rewording, offer the unit.** `adapter-supports-missing-*` has two very different causes and surfacing both as the same wall helps nobody. Sometimes the operation is expressible and the adapter is simply thin — the fix is to extend the adapter, and saying so is the right answer. Sometimes the operation is not expressible in the closed vocabulary at all: a numerical solve, a codec, a geometry transform. Rewording that into `create-one` and `read-many` produces an operation that validates and describes nothing.

For the second case, raise `kind: impasse` offering the hand-authored unit (`authored.schema.md`), pre-filled with the operations that resist expression, the invariants you would otherwise have generated suites for, and the paths the plan would have written. On acceptance, `parlay add-feature "<name>" --authored --sources "<glob>"`. No `default:` — see the decision protocol. Without this the signal dead-ends: `check-supports` stops the build and the designer is left with a code and no way forward that the pipeline recognizes.

### Legacy normalization

On first regeneration of a legacy buildfile, normalize:
- top-level `adapter:` → `targets.<kind>.adapter`
- top-level `components:` → `targets.presentation.components:`
- top-level `routes:` → `targets.presentation` (client-side) or `targets.transport` (HTTP) — disambiguate via designer prompt when both are plausible
- per-component `operations:` → per-component `file-operations:` (the pre-multi-target field name collided with the new top-level `operations:` block; see `buildfile.schema.md`'s "Why this was renamed")
- `plan.creates`/`plan.modifies` → `plan.targets.<kind>.creates`/`plan.targets.<kind>.modifies`
- non-empty `models:` → flagged as `buildfile-models-deprecated` (entities belong in `domain-model.yaml`)
- missing `schema_version:` → set to `1`

Surface the diff to the designer for review before any write. `wiring.rules` and `bindings` sections stay byte-equivalent through normalization.

### Migration entry points

For projects upgrading FROM the legacy single-adapter shape:
- `parlay migrate-config` — converts `prototype-framework:` into a single-target presentation adapter-set
- `parlay migrate-spec` — converts each feature's `surface.md` into `surface.yaml`
- `parlay migrate-capabilities` — extracts operation-shaped fragments from `infrastructure.md` into `capabilities.yaml`
- `parlay migrate-domain-operations` — migrates deprecated `domain-model.operations:` into per-feature capabilities stubs
