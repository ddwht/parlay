---
name: parlay-loop
description: "Parlay: Walk a feature end-to-end through the parlay design pipeline"
---

# Loop

Walk a feature end-to-end through the parlay design pipeline — intents → dialogs → artifacts → build → code — as one continuous guided process. The loop is the **driver**: it owns every interaction with the user, and delegates each phase-group to a subagent that does the work and reports back. Confirmations are mandatory at every phase boundary.

## Arguments

- `feature`: The feature reference in standard parlay form — `{feature}` for a top-level feature, `@{initiative}/{feature}` for a feature nested inside an initiative.
- `--from {phase}` (optional): Starting phase. Valid values: `intents`, `dialogs`, `artifacts`, `build`, `code`. Default: `intents`.
- `--authorize-criteria=machine` (optional): Advance boundaries without a person
  having approved the criteria this feature is graded against. Requires the
  project to have set `parlay.criterion-authority.allow-machine` — both, or
  neither counts. **Forward it to every phase-group and to every
  `parlay internal gate` call**; a phase that does not receive it will stop at
  the criteria decision, which is the correct behaviour for a run that was not
  authorized and the wrong one for a run that was. Recorded at the code boundary
  as a waiver: the separation between authoring a standard and grading against
  it is not provided for that run, and the record says so rather than claiming
  approval.
- `--non-interactive` (optional): Run unattended — take the declared default on advancement decisions, and abort on decisions that have no safe default. See **Non-interactive mode**. Interactive is the default; the flag never makes a run quieter than this, only less attended.

## Correlating the run (feedback mode)

Before the first phase, mint one correlation id and export it for the whole run:

```
export PARLAY_RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
```

Every CLI call and every phase subagent inherits it from the environment, which is what ties a diagnostic to the retry it caused across separate processes. Do this unconditionally — it is not only for feedback mode.

**It is also what makes one pipeline audit as one run.** A machine-authorized crossing (`--authorize-criteria=machine`) records an audit event, and the code and done boundaries are separate processes. Without a shared id the tool cannot tell one pipeline crossing two boundaries from two separate runs, and it will not guess: it records both. With this id exported, done inherits the code boundary's event and the pipeline logs once. Matching is on the id AND the criteria — an earlier run against the same standard is not this run, and never satisfies this one's audit.

**Do not put the feature name in it.** The id lands on every line of a log written to be sent upstream to the maintainers. The CLI hashes it before writing, so a feature name would not survive anyway — but building the id out of something that needs hashing invites someone to read the value back expecting it to mean something. A timestamp and the shell's PID identify a run without describing it.

You are the only participant that can do this. A pipeline run is the unit worth correlating — one feature, one loop, a dozen CLI calls — and nothing downstream knows where it started.

## Subcommands

The pipeline is the default. Four operations sit alongside it rather than inside it — they are short, they need the user, and none of them belongs at a phase boundary. The driver runs them in its own context:

| Invocation | What it does | Module |
|---|---|---|
| `handoff @{feature}` | Generate the engineering specification into `spec/handoff/{feature}/` | `.parlay/modules/generate-enggspec.md` |
| `design-spec` | Extract a design spec from Figma into the project | `.parlay/modules/reference-design-spec.md` |
| `domain-model` | Create the project domain model from existing features | `.parlay/modules/create-domain-model.md` |
| `domain-model --from {path}` | Load and integrate an externally-authored domain model | `.parlay/modules/load-domain-model.md` |

Read the named module and follow it. Because you are the driver, `AskUserQuestion` works here — prompt directly; there is no decision request to round-trip.

`domain-model` picks its module from whether `--from` is present. Both write the same artifact under the same conflict-resolution rules; the only difference is where the entities come from.

## Phase modules

The five phases are not menu entries. Their instructions live at `.parlay/modules/{create-intents,add-feature,scaffold-dialogs,create-artifacts,build-feature,generate-code}.md`, and the subagent that owns a phase reads the module it needs. `create-intents` is the intents phase's own module: `add-feature` creates the folder, and `create-intents` governs what goes in it. Nobody has to know which of five skills comes next — the driver knows the sequence, and each phase-group agent loads its own instructions.

## Prerequisites

The loop invokes three pre-defined subagents — one per phase-group — shipped by parlay and deployed by each agent adapter:

- `parlay-designer` — runs intents, dialogs, artifacts in one context
- `parlay-build` — runs the build phase (produces buildfile + testcases) in a fresh context
- `parlay-code` — runs the code phase (generates prototype + runs tests) in a fresh context

These subagents live at `.claude/agents/parlay-{name}.md` (Claude Code) or `.cursor/agents/parlay-{name}.md` (Cursor). On adapters without native subagent support (Generic CLI), the loop degrades to a **fresh-session handoff**: at each phase-group boundary it prints the exact resume command and exits, and the user re-invokes in a fresh session.

## Phase-groups

The five phases are organized into three phase-groups, each mapping to one of the pre-defined subagents:

- **designer** (intents + dialogs + artifacts) → `parlay-designer` subagent
- **build** (build) → `parlay-build` subagent
- **code** (code) → `parlay-code` subagent

Within a phase-group, phases run inline in one context so that, e.g., the dialogs phase sees the intents authored moments earlier. Between phase-groups, the loop ends the current subagent and invokes the next one — context clears as a side effect of the subagent boundary; no separate "clear context" primitive is required.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## The driver owns every user interaction

**A subagent cannot ask the user anything.** On the Claude Code adapter `AskUserQuestion` does not exist inside a subagent; a subagent that "asks" is writing into a transcript nobody reads, and then picking an answer for itself. Prompts authored inside a phase are not merely unreliable there — they are silently skipped, which is how three phase boundaries can be crossed with zero confirmations and an override menu can never appear.

So: **phases do not prompt. Phases stop and ask the driver to prompt.**

When a phase reaches a point that needs a human decision — a phase boundary, an artifact-set override, a file about to be overwritten, a failed test suite, an ambiguity it cannot resolve from the spec — it stops work and returns a **decision request** as its final output:

````
```yaml parlay-decision
kind: phase-boundary        # phase-boundary | override | overwrite | failure | ambiguity
phase: artifacts            # the phase that raised it
question: "Artifacts phase complete. Advance to build?"
context: |
  Created surface.yaml and capabilities.yaml.
  Gap analysis: 1 critical (intent "Reject with reason" has no dialog).
options:
  - id: proceed
    label: "Proceed to build"
    detail: "Starts a fresh subagent; this context clears."
  - id: stay
    label: "Stay and revise"
    detail: "Return to the artifacts phase."
  - id: exit
    label: "Exit"
    detail: "Everything on disk is preserved."
resume: "Re-enter with decision: <id>. Work completed so far is on disk."
```
````

The driver then:

1. Presents the decision with **AskUserQuestion**, using `options` verbatim and `context` as the framing. The driver may add options the phase could not know about (e.g. "Back up to dialogs"), but must not drop any. Under `--non-interactive` it does not prompt — it takes the block's `default:` on the two advancement kinds and aborts on the other three (see **Non-interactive mode**).
2. Resumes the same subagent with the chosen `id`, using the adapter's continue-an-agent primitive (on Claude Code, `SendMessage` to the running agent — its context survives, so the phase picks up exactly where it stopped). If the adapter has no such primitive, re-invoke the phase-group subagent with `decision: <id>` in the prompt; the phase re-reads state from disk, which is why `resume:` must say what is already written.
3. If the decision ends the loop (`exit`, or a hard block), does **not** resume the subagent — it ends the loop per step 12.

A phase that emits a decision request must have left the filesystem in a coherent state first: the decision is a pause, not a half-write. A phase that cannot pause safely must instead complete the safe option and report what it did.

**Inline degradation.** If no subagent surface exists at all, the driver runs the phases inline in its own context. There, `AskUserQuestion` works, so the driver prompts directly at each decision point instead of round-tripping a decision request. This mode is slower to blow context but has no confirmation gap.

Running inline means loading the phase modules the subagent would have loaded — the guidance lives in the modules, not in the agent definitions, and a driver that skips them runs the phases without their instructions. For the intents phase specifically, read `.parlay/modules/create-intents.md` and apply its soft boundaries before the phase boundary, exactly as `parlay-designer` would; `add-feature.md` only creates the folder and says nothing about what belongs in it.

## Non-interactive mode

`--non-interactive` is for callers with nobody to ask: CI, a scheduled run, a batch over several features. It changes what the driver does with a decision request, and nothing else — the phases still raise every decision they would have raised, and the same work lands on disk.

**It does not make the loop survive a headless driver.** The flag is about who answers a question, not about whether the driver is alive to receive one. `claude -p --non-interactive` stops at the first phase-group boundary exactly as `claude -p` does, because ending the turn to await a subagent is what the driver does in both modes. That is a liveness problem and step 4 handles it. Naming CI here without saying so is what sent a CI author to `claude -p` and a buildfile with no code beside it.

**Two decisions get answered from their own `default:`.** `phase-boundary` and `override` are the advancement decisions: one option is the recommendation and the rest are the user electing to intervene. Taking the recommendation is what the flag asked for. Read the id from the block's `default:` field — do not infer it from the option order, and do not fall back to "the first option" when the field is absent. A phase that should have declared a default and did not is a bug in that phase, and the correct response is the abort below, not a guess that happens to work.

**Three decisions abort the run.** `ambiguity`, `overwrite`, and `failure` have no safe default, for the reasons the decision protocol gives: the cheapest reading of an ambiguity is exactly what the protocol forbids, either answer to an overwrite destroys something, and proceeding past a failed suite is what CI exists to prevent. On any of the three, do not answer it. Emit a single-line JSON envelope on stderr and end the loop with **exit 11** — the same code and shape `--ambiguity-as-signal` uses, so a CI wrapper needs no second convention:

```json
{"kind":"blocked","decision":"overwrite","phase":"code","feature":"@expenses/submit-expense","question":"src/app/expense-form.ts has changed since it was last generated. Overwrite?","options":["overwrite","keep","exit"],"hint":"re-run without --non-interactive to answer this, or resolve the condition and re-run"}
```

`kind` is `blocked` rather than the decision's own kind because the envelope reports the run's outcome, not the decision's category; `decision` carries the category. Print the phase's `question` and `options` verbatim — the point of the envelope is that whoever reads the CI log can see the choice they need to make without re-running anything.

**What does not change.** Gap analysis still runs and still reports; critical gaps are surfaced in the log even though the boundary is auto-answered, because a gap the run advanced past is exactly what a later reader needs to find. `parlay internal gate --stage build` blockers at the build boundary are still hard blocks — they were never acknowledgeable interactively either, so there is nothing for the flag to relax.

**Thread the flag into the phase subagents.** They raise the decisions, so they need to know a human will not see them — a phase aware of the mode declares its `default:` and, where it has a choice, prefers reporting a condition over raising a decision nobody can answer. Pass it in the subagent prompt alongside the feature reference and starting phase.

## Steps

1. **Resolve the feature target** — Search `spec/intents/{name}/` and `spec/intents/*/{name}/` for a matching feature folder.
   - **Exactly one match** → proceed.
   - **Multiple matches** → ask the user to disambiguate via AskUserQuestion.
   - **Zero matches** → ask via AskUserQuestion whether to create a new feature and where (top-level or inside a named initiative). On confirmation, run `parlay add-feature {name} [--initiative {initiative}]`. On decline, exit cleanly — no filesystem changes.

2. **Validate `--from`** — If specified:
   - Reject any value other than `intents`, `dialogs`, `artifacts`, `build`, `code` with a message listing valid phases.
   - Reject `--to` and `--resume` — the loop has no such flags. Point the user at `--from`.
   - If the starting phase has missing prerequisites on disk (e.g., `--from build` but no surface artifact), offer to back up to the earliest missing phase.

3. **Plan the phase sequence, and warn about what is being skipped** — Starting at the resolved phase, the loop runs forward through every remaining phase to `code` (unless the user exits at a confirmation boundary). Never backward — to revise an upstream artifact, the user exits and re-invokes with `--from`.

   When `--from` skips phases, the artifacts those phases own are **not** re-derived; they are read as-is, and any change made upstream since they were written is carried no further. Before starting, run `parlay internal check-drift @{feature-ref}` and name the concrete consequence rather than the abstraction:

   > `--from artifacts` skips dialogs. `intents.md` changed after `dialogs.md` was last written (constraint "reject requires a reason" is new). The artifacts phase reads dialogs, so that constraint will not reach `surface.yaml`.

   Offer to back up to the earliest phase whose output is stale. This is the only gate that catches an intents↔dialogs contradiction — `parlay internal check-coverage` matches on structure and titles, not meaning, so a dialog that contradicts its intent passes it cleanly.

   The same check-drift call may return two more fields, and each has one
   right response. `ledger_integrity` findings mean a frozen founding doc or
   a recorded amendment was edited — stop and surface them to the user; do
   not proceed as if they were drift to rebuild through, and do not "fix"
   the frozen file yourself (for a pre-v0.4 edit the freeze shouldn't count,
   `parlay migrate-ledger` is the sanctioned repair). `unapplied_amendments`
   names ledger entries whose deltas never reached the contract artifacts —
   route those to `/parlay-refine`, whose apply step is the only path that
   clears them. Intent-level drift does not occur for existing features
   (founding docs freeze at first build); this loop's business here is new
   features, whose birth is unchanged.

4. **Detect subagent support, then session liveness** — Two questions, and the second one is the one that gets missed.

   First: are the `parlay-designer`, `parlay-build`, and `parlay-code` subagents available (on Claude Code, via the Agent tool by name; on Cursor, via the `/parlay-{name}` slash command)? If not, choose **inline degradation** (above) or **fresh-session handoff** (step 8) — inline when the project is small enough to fit, handoff otherwise.

   Second, and only if they are: **will this session still be running when a background subagent reports back?** Delegating a phase-group means ending the turn and waiting for a completion notification. That works when something re-invokes the driver when the notification arrives. It does not work in a single-turn headless driver — `claude -p` with no wrapper that continues the session — because the process exits when the turn ends. The subagent is killed mid-phase, not paused: the next run redoes the work it had already done.

   The failure is quiet, which is why it needs checking rather than discovering. The run exits 0 with a valid `buildfile.yaml`, no `testcases.yaml` and no generated code, and the last assistant message says the phase agent is working and will report back. Nothing says the pipeline stopped.

   So when the driver cannot be re-invoked across turns, do not take the subagent rung even though subagents exist. Take **fresh-session handoff** (step 8): it prints an exact resume command and exits deliberately, which is the same shape a single-turn caller needs anyway. Prefer **inline degradation** instead when the whole feature plausibly fits one context — it finishes in the single turn rather than requiring the caller to drive the next one.

   If you cannot tell which kind of session you are in, say so and take the handoff rung. It costs a resume command; the alternative costs a run that reports success without generating code.

   A CI caller needs to know this: `claude -p` alone stops at the first phase-group boundary. Either continue the session until it stops doing work, or run the phases inline.

5. **Enter the designer phase-group** (if starting phase is intents, dialogs, or artifacts):
   - Invoke the `parlay-designer` subagent with the feature reference, the starting phase, and **both** `--non-interactive` and `--authorize-criteria=machine` when they were passed. The designer phase is where the criteria decision is raised, so a run authorized to proceed without human approval that does not receive the flag stops there — and stops for a reason the caller believed they had answered.
   - It runs the three phases in sequence in one context: author/revise `intents.md`; generate or update `dialogs.md`; determine and create the artifact set.
   - Pre-load on-disk upstream artifacts if `--from` skipped phases in this group (dialogs needs intents; artifacts needs intents + dialogs).
   - It runs **Gap analysis** at the end of the intents phase and the dialogs phase (step 9) and folds the result into the `context:` of the boundary decision.
   - Every boundary and every override comes back as a decision request; the driver prompts and resumes (see above).
   - The artifacts phase's boundary decision carries an `artifacts:` list naming what it wrote. If that list contains `domain-model`, tell the user where the file is and that hand edits made before the build phase are picked up (step 11), before the designer→build boundary is answered.
   - Whatever that list contains, run the contribution review (step 11.5) before the designer→build boundary is answered. A feature's contribution is a proposal against the project model, and the boundary is where it gets accepted or left standing.

6. **Enter the build phase-group** (at the designer→build boundary):
   - End the designer subagent; tell the user the context is clearing — make it explicit, not surprising.
   - Run `parlay internal gate @{feature-ref} --stage build`, adding
     `--authorize-criteria=machine` when this run carries it. This is one command and one exit code where there used to be a bare check-readiness: the gate aggregates check-readiness (including the `unapplied-amendments` error), the ledger's integrity findings, its unapplied tail, and the amendment ledger's own validation — the drift and ledger findings the driver previously only saw at planning time (step 3). **A non-zero exit is a hard block** — not acknowledgeable; surface its `blockers[]` and route the user back to the artifacts phase, or to `/parlay-refine` when a blocker names the ledger tail (`unapplied-amendments`). `warnings[]` are informational, exactly as readiness warnings were.
   - Invoke the `parlay-build` subagent with the feature reference, and both `--non-interactive` and `--authorize-criteria=machine` when they were passed.
   - At the end it returns a `phase-boundary` decision; the driver prompts.

7. **Enter the code phase-group** (at the build→code boundary):
   - End the build subagent; announce the new subagent boundary.
   - Invoke the `parlay-code` subagent (project-level; no `@feature` argument), passing both `--non-interactive` and `--authorize-criteria=machine` when they were passed. The code boundary is where an authorized waiver is recorded, so a code phase that never receives the flag cannot record one and refuses instead.
   - The code phase raises an `overwrite` decision for every generated file that changed since it was last generated, and a `failure` decision if the test suite does not pass. Both come to the driver.
   - After the code phase completes successfully, end the loop with the natural completion summary (step 12). No trailing confirmation — there is no next phase.

8. **Fresh-session handoff** (adapter without subagent support):
   - At the phase-group boundary, print the exact resume command, e.g. `/parlay @{initiative}/{feature} --from build`.
   - Print "Exiting this session. All artifacts are on disk."
   - Exit the current session. The loop persists NO resume state — no on-disk phase cursor, no acknowledged-gap log. Continuity is the user's memory plus the printed hint.

9. **Gap analysis** (at the end of intents and dialogs phases):
   - **intents phase**: intents with unresolved `Questions:` sections, intents missing required fields, contradictory constraints. `parlay internal collect-questions @{feature}` finds the first class.
   - **dialogs phase**: intents without dialogs (coverage gaps), dialogs without matching intents (orphans), dialogs missing branches implied by the intent's Constraints or Verify items. `parlay internal check-coverage` finds structural gaps; read the pairs it reports as matched and confirm they actually correspond — it matches on title overlap, so it both misses renames and blesses contradictions.
   - Classify each gap as **critical** or **minor**. Rule of thumb: gaps that cascade into ambiguous downstream artifacts (unresolved Questions, missing required fields, contradictory constraints, intents without dialogs, orphan dialogs) are critical; stylistic or partial-coverage gaps are minor.
   - Critical gaps go in the boundary decision's `context:` with a recommendation to stay. The user may still advance — no acknowledgement state is persisted, so the same gaps are re-analyzed on a later `--from`.

10. **Phase confirmation** (at every boundary except after code) — the driver's job, per **The driver owns every user interaction**:
    - AskUserQuestion with at least **Proceed**, **Stay and revise**, **Exit**.
    - Name the just-completed phase and the phase about to begin.
    - At phase-group boundaries, say that the next phase starts in a fresh context.
    - On "Stay" — resume the subagent with `stay`; let the user iterate. Re-run gap analysis on request.
    - On "Exit" — end the loop with the user-exit summary (step 12).

11. **Pause for a domain-model review** (at the artifacts boundary, when the artifacts phase wrote `domain-model`):

    The loop is the normal path to a domain model — the artifacts phase authors `domain-model.yaml` whenever a feature introduces entities or vocabulary. A hand edit made here is read; one made after the build phase runs is not, and nothing else in the loop tells the designer that. This is where it belongs.

    Add one more option to the phase-boundary question of step 10, alongside Proceed / Stay / Exit:

    > **Review and edit the domain model before building?** — Opens nothing — edit the YAML directly. The build phase reads this model, so edits made now are picked up; edits made after are not.

    On accept:
    - Name the file: `<activeRoot>/domain-model.yaml`. The user edits it by hand, in whatever editor they already use. Wait for them to say they are done — do not advance while an edit is in progress; the whole point is that the build phase reads the model afterwards.
    - Offer `parlay validate --type domain-model <path>` as the way to check an edit before advancing, and run it when they ask.
    - Then run `parlay internal check-drift @{feature-ref}` and report what it says. The domain model is a **shared** source: an edit dirties every feature that reads it, not only this one. Name that consequence — features other than this one may now be stale, and this loop will not fix them.
    - Then re-present the boundary question. Pausing for a hand edit is not itself an answer to "advance to build?", and treating it as one would advance without a confirmation.

    On decline, proceed as normal — the pause is an offer, never a gate.

    Skip the pause entirely when the session is not interactive. `--non-interactive` skips it for the same reason: there is nobody to hand the file to, and an unattended run has no edit to wait for.

11.5. **Review the feature's domain-model contribution** (at the artifacts→build boundary, before the boundary question of step 10 is answered):

    A feature that needs something the project model lacks writes it into its own `spec/intents/{feature}/domain-model.yaml` rather than editing the root model. That file is a **proposal**. This is where it gets reviewed, and nothing lands in the root model until the designer accepts it here.

    Run `parlay internal domain-impact @{feature-ref}`. Read the JSON:

    - `contributed: false` — the feature proposes nothing. Say nothing and carry on; a project that only ever authors the root model never sees this step.
    - `additions` — entities, fields, enum values and relationships the root model lacks.
    - `conflicts` — the feature describes something the root already describes differently. Each carries both descriptions.
    - `affects` — per touched entity, which **other** features reference it and which of their fixtures would need the new field.

    **Report what changes and who it affects, not the diff.** The additions are one line each; the work they create is spread across other people's fixtures. Say both:

    > `submit-expense` proposes one new field: `ExpenseReport.settledAt` (datetime).
    > Two other features read `ExpenseReport` — `dashboard` and `expense-list` — and two fixtures hold records that would need it: `dashboard/seed`, `expense-list/mixed-status`.

    Then add one more option to the phase-boundary question of step 10:

    > **Accept this contribution into the project domain model?** — Accept / Edit the contribution file / Leave it proposed.

    - **Accept** — run `parlay internal domain-impact @{feature-ref} --apply`. The merge is additive and goes through the same concurrency-checked (etag compare-and-swap), atomic write path as any other accepted contribution. Then run `parlay internal check-drift @{feature-ref}` and report it: the domain model is shared, so accepting dirties every feature that reads it, not only this one.
    - **Edit the contribution file** — edit `spec/intents/{feature}/domain-model.yaml` directly, then re-run `parlay internal domain-impact @{feature-ref}` and re-present, because the answer may have changed.
    - **Leave it proposed** — nothing is written. The contribution stays on disk, and other features referencing its entities report `capabilities-entity-pending` rather than failing. This is a valid answer, not a deferral to nag about.

    **When `conflicts` is non-empty, do not offer Accept.** The command exits non-zero and writes nothing; there is no auto-merge and no last-writer-wins, because which of two descriptions is right is a design question. Present both descriptions and route the user back to the artifacts phase.

    `--non-interactive` skips the offer and reports the impact as information, exactly as it skips step 11 — an unattended run has nobody to accept on its behalf.

12. **End the loop cleanly**:
    - **Natural completion** (after code): print a summary with the feature reference, phases run, and key artifacts on disk. No resume hint. Loop complete.
    - **User-chosen exit**: print a summary naming what completed, plus a resume command (`/parlay {feature-ref} --from {next-phase}`).
    - **Mid-phase session interruption**: no special handling — artifacts on disk are preserved; the user re-invokes with `--from`.
    - **No cleanup ever**: the loop does not delete or roll back any files on any exit path.

## Interactive Questions

The driver — never a phase — uses AskUserQuestion for:
- Feature creation confirmation (zero matches)
- Multiple matches disambiguation
- Phase boundary confirmation (proceed / stay / exit)
- Gap-analysis response (stay / advance anyway / exit)
- Readiness warnings response (proceed / stay / exit)
- Phase failure recovery (retry / stay / exit)
- Backing up to an earlier phase when `--from` prerequisites are missing or upstream output is stale
- The domain-model review pause at the artifacts boundary (step 11)
- The contribution review at the artifacts boundary — accept / adjust / leave proposed (step 11.5)
- Every `parlay-decision` block a phase-group returns

## Hard rules

- NEVER auto-advance between phases — confirmation is mandatory.
- NEVER let a phase prompt the user directly when it is running as a subagent — it must emit a decision request and stop. A prompt inside a subagent is silently skipped, and the phase then answers it for itself.
- NEVER drop an option a phase offered in its decision request. The driver may add options; it may not narrow the choice.
- NEVER persist resume state to disk — no `.parlay/loop-state.yaml`, no phase cursor, no acknowledged-gap log.
- NEVER run phases backward — forward only.
- NEVER silently overwrite designer-authored files (intents.md, dialogs.md) — per CLAUDE.md file-ownership rules.
- NEVER create a new feature without explicit user confirmation — zero matches must prompt, never auto-create.
- NEVER advance a phase-group boundary without a passing `parlay internal gate` for the target stage. The designer→build boundary is gated by the driver's own `parlay internal gate @{feature} --stage build` call (step 6); the build→code and code→done boundaries are gated by the injected **Step 0 — Gate** inside the build and code modules, which report a `failure` decision on a non-zero exit. A gate's `blockers[]` are hard blocks — not acknowledgeable; only `warnings[]` are.
- NEVER advance past a `parlay internal gate --stage build` blocker at the build boundary — a blocker is a hard block; only warnings are acknowledgeable. (This subsumes the former bare check-readiness ERROR rule: the gate includes check-readiness and then some.)
- NEVER treat the domain-model review pause as an answer to the boundary question — re-ask after it, or the loop advances on a confirmation nobody gave.
- NEVER answer an `ambiguity`, `overwrite`, or `failure` decision from a default, under any flag. Abort with the envelope and exit 11. A flag that says "do not ask me" is not a flag that says "guess for me".
- NEVER infer a missing `default:` under `--non-interactive` — a phase that omitted one on an advancement decision is a bug in that phase, and taking the first option hides it behind a run that happened to work.

## Error Handling

- `subagent-not-found` — the required subagent is not available. Check whether `parlay upgrade` has been run. If the adapter has no native subagent support at all, switch to inline degradation or fresh-session handoff.
- `invalid-phase-name` — `--from` value is not one of the five canonical phases. List valid phases and exit.
- `unsupported-flag` — user passed `--to` or `--resume`. Explain these are not supported and point at `--from`.
- `missing-prerequisite-artifact` — starting phase requires an upstream artifact that does not exist. Offer to back up to the earliest missing phase.
- `phase-failure` — a phase-group returned an error rather than a decision request. Surface it and offer retry / stay-in-phase / exit. "Proceed" is not an option for a failed phase.
- `malformed-decision` — a phase returned a `parlay-decision` block missing `question` or `options`. Do not guess an answer. Show the raw block to the user and offer retry / exit.
- `ambiguous-feature` — feature search returned multiple matches. Disambiguate via AskUserQuestion. Under `--non-interactive` this is itself unanswerable: emit the blocked envelope with `decision: ambiguity` and exit 11 rather than picking a match.
- `blocked-non-interactive` — a decision with no safe default was raised under `--non-interactive`. Emit the envelope, exit 11, and change nothing further on disk. Everything already written stays; the run is resumable with `--from` once the condition is resolved.
