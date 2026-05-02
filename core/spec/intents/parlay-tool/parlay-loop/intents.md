# Parlay-loop

> A single orchestrating command that walks the user end-to-end through the parlay design pipeline — intents, dialogs, artifacts, build, and code — as one continuous, guided process.

---

## Run the Full Design Loop from One Command

**Goal**: Initiate the entire parlay design pipeline (intents → dialogs → artifacts → build → code) via a single command, so the user does not have to remember and chain individual `/parlay-*` commands.
**Persona**: UX Designer, Engineer
**Priority**: P0
**Context**: A user wants to take a feature from idea to generated code. Today this requires manually invoking `/parlay-add-feature`, `/parlay-scaffold-dialogs`, `/parlay-create-artifacts`, `/parlay-build-feature`, and `/parlay-generate-code` in sequence, remembering each step and its prerequisites.
**Action**: The user runs `/parlay-loop {feature} [--from {phase}]`. The skill orchestrates all downstream parlay skills in order, handing off between phases while preserving continuity.
**Objects**: feature, phase, loop-session

**Constraints**:
- The loop is continuous — once started, the user stays inside a single guided flow until they exit or the loop finishes code generation.
- The loop MUST reuse existing `/parlay-*` skills for each phase rather than duplicating their logic. It is an orchestrator, not a reimplementation.
- The loop must work for both brand-new features (where the feature folder does not yet exist) and existing features (where some or all artifacts already exist).
- The loop operates on a single feature at a time, mirroring the rest of the parlay skill surface. If the feature lives inside an initiative, the loop resolves the path correctly but does not orchestrate across sibling features.
- The loop must be supported for every agent parlay supports — it cannot assume any one agent's tool set beyond what parlay's adapter contract guarantees.

**Verify**:
- Running `/parlay-loop my-feature` on a non-existent feature creates it via parlay-add-feature and then enters the intents phase.
- Running `/parlay-loop my-feature` on an existing feature enters the default phase (intents) for adding new intents to the existing feature.
- Running `/parlay-loop @my-initiative/my-feature` on a feature inside an initiative works the same as a top-level feature.
- The loop successfully completes all five phases end-to-end on a simple feature without the user invoking any other `/parlay-*` command manually.

---

## Start From a Specific Phase

**Goal**: Allow the user to enter the loop at any phase, so they can pick up from where they left off or re-run just the downstream steps after editing an upstream artifact.
**Persona**: UX Designer, Engineer
**Priority**: P0
**Context**: The user edited `surface.md` by hand and now wants to regenerate the buildfile, testcases, and code — but does not want to revisit intents and dialogs. Or: the user is resuming work on a feature after a break and wants to jump directly to the build phase.
**Action**: The user passes an optional `--from {phase}` flag (or equivalent argument). Valid phases are exactly: `intents`, `dialogs`, `artifacts`, `build`, `code`. The loop starts at that phase and proceeds forward through the remaining phases in order.
**Objects**: phase, loop-session

**Constraints**:
- The canonical phase names are `intents`, `dialogs`, `artifacts`, `build`, `code` — no aliases, no alternates. The `artifacts` phase is not split: it covers surface and (where applicable) infrastructure in one step, delegating to `parlay-create-artifacts` which already decides what to produce.
- The only phase-selection flag is `--from`. There is no `--to` bound — once the user starts the loop at a phase, it runs forward through every remaining phase to `code` (unless the user exits at a confirmation boundary). Single-phase runs should be done by invoking the corresponding `/parlay-*` skill directly, not by the loop.
- The default phase when `--from` is omitted is `intents`.
- Starting from a downstream phase must not silently skip validation — if prerequisite artifacts are missing (e.g., `--from build` but no surface exists), the loop must tell the user and offer to back up to the earliest missing phase.
- The loop always runs phases in forward order. It never goes backward — to revise an upstream artifact the user exits and re-enters at that phase.
- Starting `--from intents` on an existing feature means "add new intents to this feature", not "start over". Existing intents are preserved.

**Verify**:
- `/parlay-loop my-feature --from artifacts` runs artifacts → build → code, skipping intents and dialogs.
- `/parlay-loop my-feature --from build` on a feature with no surface prompts the user and offers to start from `artifacts` instead.
- `/parlay-loop my-feature` on an existing feature with intents enters the intents phase in "add new intents" mode, not overwrite mode.
- Supplying an invalid phase name returns an error listing the valid phases: `intents`, `dialogs`, `artifacts`, `build`, `code`.
- The loop does not accept a `--to` or equivalent upper bound; passing one is rejected with a message pointing the user at the individual `/parlay-*` skill for single-phase runs.

---

## Confirm Before Advancing to the Next Phase

**Goal**: Require an explicit user confirmation before transitioning from one phase to the next, so the user always stays in control of what happens.
**Persona**: UX Designer, Engineer
**Priority**: P0
**Context**: After a phase completes, the user needs a chance to review its output, make manual edits, or stop the loop entirely before expensive downstream work (code generation, context clearing) happens.
**Action**: At the boundary between every pair of phases, the loop pauses and asks the user to confirm. The user can confirm (proceed), revise (stay in the current phase), or exit (end the loop).
**Objects**: phase-transition, confirmation

**Constraints**:
- Confirmation is mandatory at every phase boundary — the loop MUST NOT auto-advance, even if a phase completes without errors.
- The confirmation prompt must name the phase just completed and the phase about to begin, so the user knows what they are approving.
- The confirmation must be collected via the agent's interactive-question mechanism (e.g., AskUserQuestion on Claude Code), so it works across all supported agents.
- If the user declines, the loop stays in the current phase and lets the user iterate; it does not exit unless the user explicitly chooses to exit.

**Verify**:
- Every phase transition shows a confirmation prompt naming the completed and next phase.
- Declining the prompt keeps the user in the current phase.
- Choosing "exit" ends the loop cleanly and prints a summary of what was completed.

---

## Analyze Gaps and Hold the User in Intents/Dialogs Until Resolved

**Goal**: At the intents and dialogs phases, analyze the artifacts for critical gaps (open questions, ambiguities, missing coverage) and recommend the user stay in the current phase until those gaps are resolved — rather than advancing with an incomplete specification.
**Persona**: UX Designer
**Priority**: P1
**Context**: Premature advancement past intents or dialogs produces weak downstream artifacts. Open questions, ambiguous constraints, and missing verify items at the intents stage cascade into unclear dialogs, ambiguous surfaces, and buggy code. The loop should catch these early.
**Action**: At the end of the intents phase, the loop runs a gap analysis — lists open questions, under-specified intents, and critical ambiguities. It does the same for dialogs at the end of the dialogs phase. If critical gaps exist, the loop recommends staying in the phase; the user can still choose to advance if they accept the risk.
**Objects**: gap-analysis, open-question, intent, dialog

**Constraints**:
- Gap analysis is only a recommendation — the user can always choose to advance anyway (the confirmation step above is still the source of truth for advancement).
- Gap analysis at the intents phase must surface: intents with unresolved `Questions:` sections, intents missing required fields, contradictory constraints, and any existing open-questions report from `parlay-collect-questions`.
- Gap analysis at the dialogs phase must surface: intents without dialogs (coverage gaps), dialogs without matching intents (orphans), and dialogs missing branches implied by an intent's Constraints or Verify items.
- The artifacts phase does NOT have a separate loop-level gap analysis. Enforcement of surface-to-intent traceability (every surface fragment must have a Source intent) already lives in `parlay check-readiness --stage build-feature` as a blocking error (`fragment-missing-source`). The loop relies on this existing gate at the boundary entering the build phase — it does not duplicate the check at artifacts.
- At the boundary entering the build phase, the loop MUST run `parlay check-readiness --stage build-feature` and treat any returned errors as hard blocks. The user cannot advance past them by acknowledgement; they must fix the underlying artifact (typically by going back to the artifacts phase). Warnings (e.g., open questions, intents without surface coverage) are informational and acknowledgeable.
- The severity classification ("critical" vs "minor") at the intents/dialogs phases is a **fixed policy** applied by the agent. The user does not configure it and cannot override which gaps the agent deems critical. The agent uses its judgment against the gap catalogue above — rough rule of thumb: gaps that would cascade into ambiguous downstream artifacts (unresolved intent Questions, missing required fields, contradictory constraints, intents without dialogs, orphan dialogs) are critical; stylistic or partial-coverage gaps (dialogs missing a non-essential branch, cosmetic inconsistencies) are minor.
- Gap analysis should reuse existing parlay skills where possible (`parlay-collect-questions`, `parlay-sync`, `parlay check-readiness`) rather than duplicating their logic.

**Verify**:
- At the end of the intents phase, the loop lists every intent with an unresolved Question and marks it as a critical gap.
- At the end of the dialogs phase, the loop lists intents without dialogs and marks them as critical gaps.
- If no critical gaps exist, the loop says so explicitly.
- The user can advance despite critical intent/dialog gaps by choosing "proceed" at the confirmation prompt after the gap-analysis warning. No acknowledgement state is persisted — if the user later resumes via `--from` at an earlier phase, the same gaps will be re-analyzed and re-warned.
- Entering the build phase, a surface fragment missing a Source intent produces a hard block (error from `check-readiness`), not an acknowledgeable warning. The loop surfaces the error and routes the user back to the artifacts phase.
- There is no configuration knob for gap severity — running the loop on the same inputs produces the same critical/minor classification regardless of user settings.

---

## Manage Context Across Phases via Phase-Group Sub-Agents

**Goal**: Keep related upstream artifacts in the same agent context where they benefit each other (intents + dialogs + artifacts are all designer-authored and reference one another), but clear context between phases where a fresh start is more valuable than shared memory (build-feature, code-generation) — implemented by running each phase-group in a dedicated sub-agent.
**Persona**: Engineer, UX Designer
**Priority**: P0
**Context**: Intents, dialogs, and artifacts are tightly coupled — an agent reading all three together makes better artifacts than one reading them sequentially in isolated contexts. Build-feature and code-generation, however, operate on the finalized surface alone; carrying intent/dialog context forward wastes tokens and can distract the agent from the concrete spec it should be implementing. Sub-agent spawning is the natural primitive for achieving both sharing-within and clearing-between: a sub-agent that runs a whole phase-group inherently shares context inside, and each new sub-agent inherently starts fresh.
**Action**: The loop defines three phase-groups — **designer** (intents + dialogs + artifacts), **build** (buildfile + testcases, produced by the `build` phase), and **code** (code generation). The outer loop agent stays minimal and handles only routing, confirmation, and disk-based gap analysis. Each phase-group runs in its own sub-agent. Within a sub-agent, the loop invokes the underlying `/parlay-*` skills inline (direct calls), so phases inside a group share full context. Between groups, the loop ends one sub-agent and spawns the next — producing a clean context clear as a side effect of the architecture.
**Objects**: context-boundary, loop-session, phase, phase-group, sub-agent

**Constraints**:
- The three phase-groups are: **designer** (covers the `intents`, `dialogs`, `artifacts` phases), **build** (covers the `build` phase, which produces buildfile.yaml and testcases.yaml), **code** (covers the `code` phase). This grouping is fixed — the loop does not expose it as configuration, and the user cannot opt out of phase-group separation (e.g., there is no "run everything in one sub-agent" mode).
- The outer loop agent MUST stay slim: it does not inline sub-skill instructions itself. Its context holds only the loop's own routing logic, the user's answers, and gap-analysis reads from disk.
- Within a phase-group, sub-skills MUST be invoked inline (direct calls) so that, e.g., the dialogs skill sees the just-authored intents in context without re-reading disk.
- Between phase-groups, the loop MUST end the current sub-agent and spawn a new one. This is the context-clearing mechanism — no separate "clear context" primitive is required.
- The sub-agent spawning primitive is part of the adapter contract. For adapters without sub-agent support, the loop falls back to a resumable fresh-session handoff (see the adapter-support intent).
- The user must be informed at each phase-group boundary that a fresh sub-agent is starting, so the effect is not surprising (e.g., the agent "forgetting" an earlier conversation).
- The outer loop passes each sub-agent only the minimum it needs: the feature reference and the phases to run. Artifacts live on disk — the sub-agent reads them as inputs, and any gap analysis is re-derived from disk, not carried forward as state.
- When `--from` lands inside a phase-group (e.g., `--from dialogs` in the designer group, or a future case inside any group), the sub-agent skips the earlier phases in that group but **pre-loads** their on-disk artifacts into its context before starting work. Downstream phases always need upstream context (dialogs require intents; artifacts require both intents and dialogs), so the sub-agent reads them up front rather than re-reading later.
- Pre-loading happens once, at the start of the sub-agent — it is not re-triggered between phases within the group, because all phases in a group share the same sub-agent context.

**Verify**:
- Running the loop end-to-end uses exactly three sub-agents in sequence: designer, build, code.
- Inside the designer sub-agent, the dialogs phase can see the intents authored earlier in the same sub-agent without re-reading them from disk.
- `/parlay-loop my-feature --from dialogs` starts the designer sub-agent with intents.md pre-loaded from disk and proceeds straight into dialogs.
- `/parlay-loop my-feature --from artifacts` starts the designer sub-agent with both intents.md and dialogs.md pre-loaded from disk and proceeds straight into artifacts.
- Before the build and code sub-agents start, the loop announces the new sub-agent boundary to the user.
- After a sub-agent boundary, the next phase's skill operates correctly because all inputs are on disk.
- The outer loop's context does not grow linearly with the number of phases completed — it remains approximately constant across a full run.
- There is no flag or setting to disable phase-group separation — running all three groups in a single sub-agent is not a supported mode.

---

## Adapt Between New and Existing Features

**Goal**: Detect whether the target feature already exists and adjust the loop's behavior accordingly, so the same command works for both new and existing features — and never create a new feature without an explicit user confirmation.
**Persona**: UX Designer, Engineer
**Priority**: P0
**Context**: A user running `/parlay-loop payments` may be creating a brand-new feature or resuming work on one that already has intents, dialogs, and a surface. The feature may live at the top level or inside an initiative. The loop should handle all cases without requiring the user to remember the feature's exact path — and must not silently scaffold a new feature in the wrong place if the user mistyped an existing name.
**Action**: The loop searches for a feature matching the argument across both `spec/intents/{name}/` (top-level) and `spec/intents/*/{name}/` (inside any initiative). Exactly one match → use it. Multiple matches → ask the user to disambiguate. Zero matches → ask the user whether to create a new feature, and if yes, where to create it (top-level or inside a specific initiative). Only after that confirmation does the loop invoke `parlay-add-feature`.
**Objects**: feature, existence-check, feature-search, loop-session

**Constraints**:
- Feature existence is determined by **searching for a matching feature folder**, not by assuming a path. The search covers top-level features and features nested inside initiatives in a single pass.
- When the search finds exactly one match, the loop proceeds with that feature (no additional prompt needed).
- When the search finds more than one match (e.g., a feature named `login` exists at top level AND inside an `auth-overhaul` initiative), the loop MUST ask the user which one was meant before proceeding.
- When the search finds zero matches, the loop MUST ask the user to confirm creating a new feature. The loop MUST NOT silently invoke `parlay-add-feature`. The confirmation prompt also captures the intended location (top-level vs inside a named initiative).
- If the user declines the "create new feature" prompt, the loop exits cleanly without creating anything — preserving the user's ability to retry with a corrected name.
- For an existing feature entering the intents phase, the loop does not distinguish between "add new intents" and "revise existing intents" sub-modes. Both are the same agent behavior — author intents against the current state of intents.md. Whether the user's input adds a new intent or edits an existing one is a property of the user's message, not a loop mode.
- Existing intents are preserved by default. The agent must ask permission before modifying any designer-authored content in intents.md, per the file-ownership rules in CLAUDE.md. Appending a new intent does not require the same permission ask that modifying existing content does.
- For an existing feature with downstream artifacts (e.g., existing dialogs), re-running the dialogs phase does not overwrite them; it invokes the existing update/scaffold behavior from `parlay-scaffold-dialogs`.
- The loop must never silently overwrite designer-authored files (intents.md, dialogs.md). File-ownership rules from CLAUDE.md apply.

**Verify**:
- `/parlay-loop new-feature` with no matching feature asks the user to confirm creation and choose a location before invoking `parlay-add-feature`.
- `/parlay-loop login` when `login` exists both top-level and inside `auth-overhaul` asks the user to pick which one.
- `/parlay-loop existing-feature` finds it unambiguously (top-level or nested) and enters the intents phase without creating anything new.
- In the intents phase on an existing feature, the user can both append new intents and edit existing ones in the same session — the agent doesn't require a mode switch.
- Declining the "create new feature" prompt exits the loop without creating any files.
- `/parlay-loop existing-feature` with existing dialogs does not overwrite them at the dialogs phase.

---

## End the Loop Cleanly

**Goal**: Define what "end" means for the loop — whether it finished all phases, the user chose to exit mid-way, or the agent session was interrupted — so the user always knows what completed and how to continue.
**Persona**: UX Designer, Engineer
**Priority**: P0
**Context**: Because the loop spans multiple sub-agents, multiple phases, and potentially multiple user sessions, the moment the loop "ends" is not self-evident. Without clear end semantics, the user can't tell whether artifacts are partial, whether a re-invocation is needed, or what command to run next.
**Action**: The loop distinguishes four end cases — natural completion, user-chosen exit at a confirmation prompt, mid-phase session interruption, and (separately from this intent) the fresh-session handoff for adapters without sub-agent support. For each case, the loop either prints a final summary or leaves state on disk such that the user can resume via `--from`.
**Objects**: loop-end, loop-summary, session-interruption

**Constraints**:
- **Natural completion** (after the `code` phase succeeds): the code sub-agent ends, and the outer loop prints a final summary containing the feature reference, every phase that ran, and the key artifacts produced (e.g., `spec/intents/{feature}/surface.md`, `.parlay/build/{feature}/buildfile.yaml`, generated code files). There is no trailing confirmation prompt after `code` — there is no next phase to approve, so a single end-summary suffices.
- **User-chosen exit** at any confirmation prompt: the loop prints a summary of what completed, plus a resume command (`/parlay-loop {feature} --from {next-phase}`) the user can run later. The loop then exits.
- **Mid-phase session interruption** (user closes the session, Ctrl-C, agent crash): the loop performs no special handling. Artifacts already written to disk remain. The user resumes by re-invoking the loop with `--from` pointing at the earliest not-yet-completed phase (whose identity they infer from disk state, the same as for the fresh-session fallback).
- **No cleanup on exit**: the loop never deletes or rolls back artifacts on any exit path. Partial work is always preserved. This matches parlay's existing disk-based file-ownership model.
- **Exit summary format is consistent** across natural completion and user-chosen exit: same structure, only the "phases that ran" list and presence of a resume command differ.

**Verify**:
- After the `code` phase completes successfully, the loop prints a single final summary (no extra confirmation) and exits.
- Choosing "exit" at the confirmation prompt between dialogs and artifacts prints a summary showing intents and dialogs as complete, plus `/parlay-loop {feature} --from artifacts` as the resume hint.
- If the user force-closes the session mid-phase, artifacts from completed phases remain on disk and re-invoking `/parlay-loop {feature} --from {phase}` picks up correctly.
- The loop never deletes any file under `spec/intents/` or `.parlay/build/` as part of an exit.

---

## Resolve Features Inside Initiatives

**Goal**: Accept a feature target whether it is top-level or nested inside an initiative, so the user does not have to think about the feature's location when invoking the loop.
**Persona**: UX Designer, Engineer
**Priority**: P1
**Context**: Features may live at `spec/intents/{feature}/` or inside an initiative at `spec/intents/{initiative}/{feature}/`. The rest of parlay's skills already operate per-feature; the loop should do the same. The loop just needs to be "aware" of initiatives enough to resolve the target path and pass the correct feature reference to every downstream skill.
**Action**: The loop accepts the feature argument in the standard parlay reference forms (`{feature}` for top-level, `@{initiative}/{feature}` for nested). It resolves the path, confirms the feature's location, and then drives that one feature through the pipeline using the existing per-feature skills.
**Objects**: feature, initiative, target-resolution

**Constraints**:
- The loop always operates on exactly one feature at a time. It MUST NOT attempt to orchestrate multiple features or treat an initiative as a collective target.
- Target resolution must be unambiguous. If a bare feature name matches both a top-level feature and a feature inside an initiative, the loop asks the user which was meant.
- Every sub-skill invocation receives the fully-qualified feature reference so downstream skills resolve paths correctly.

**Verify**:
- `/parlay-loop upgrade-plan` resolves a top-level feature and drives only that feature.
- `/parlay-loop @auth-overhaul/login` resolves a feature inside the `auth-overhaul` initiative and drives only that feature.
- Ambiguous feature names prompt the user to disambiguate.
- The loop never advances sibling features inside an initiative as a side effect of running on one feature.

---

## Support All Parlay-Supported Agents

**Goal**: The loop works on every agent parlay supports — Claude Code, Cursor, and the Generic CLI adapter today, plus any future adapters — so no user is locked out of the end-to-end experience because of their choice of agent.
**Persona**: Engineer
**Priority**: P1
**Context**: Parlay is agent-agnostic by design. Three agent deployers currently ship: `Claude` (`internal/deployer/claude.go`), `Cursor` (`internal/deployer/cursor.go`), and `Generic` (`internal/deployer/generic.go`). A feature that only works on one agent breaks the core value proposition. The loop uses interactive confirmations, context management, and sub-skill invocation — all of which must go through the adapter contract.
**Action**: The loop implementation uses only primitives exposed by the adapter contract: sub-skill invocation, interactive questions, and context management. Any agent that satisfies the contract automatically supports `/parlay-loop`.
**Objects**: agent, adapter, adapter-contract

**Constraints**:
- The loop MUST NOT rely on any Claude-Code-only feature (e.g., a Claude-Code-specific tool name) without going through the adapter.
- The primary adapter primitive the loop requires is **sub-agent spawning** (used to implement phase-group context boundaries). Adapters that provide this get the full loop experience automatically.
- The loop also requires a portable **skill-invocation primitive** so the outer loop can invoke `/parlay-*` skills by name without hardcoding Claude-Code slash-command syntax.
- **The current agent adapter contract does not yet expose either primitive.** Today the `Deployer` interface at `internal/deployer/deployer.go` covers only skill-file deployment and agent-config writing — there is no runtime contract for skill invocation or sub-agent spawning, and existing skills invoke one another by embedding `/parlay-*` slash commands directly. Implementing parlay-loop therefore requires extending the agent adapter contract to add:
  1. A sub-agent spawning primitive (e.g., `SpawnSubAgent(instructions, inputs) → result`).
  2. A skill-invocation primitive (e.g., `InvokeSkill(name, args) → result`), so the outer loop can delegate to existing `/parlay-*` skills portably.
- Each supported agent's adapter (Claude Code first, then others) must implement these primitives. Claude Code's implementation wraps its Agent/Task tool and its slash-command dispatch; other adapters map to their agent's equivalents or declare them unsupported.
- For adapters **without** sub-agent support, the loop MUST degrade to a **fresh-session handoff**: at each phase-group boundary the loop tells the user that a fresh session is required to continue, and prints the exact command to run next (e.g., `/parlay-loop @my-feature --from build`). The user copies that command into a new session. The loop itself persists **no** resume state — no `--resume` flag, no on-disk phase cursor, no acknowledged-gap log. Artifacts on disk are the only source of truth; the user's memory (plus the printed hint) is the only continuity mechanism across sessions.
- Rationale for no tracked resume: persistent per-user loop state is awkward to keep consistent, especially for existing features where a user may legitimately re-enter at any phase. A printed-hint handoff is explicit, user-controlled, and cannot drift from reality.
- The skill file and deployer registration must be updated so every supported agent receives `/parlay-loop` as part of `parlay init` / `parlay upgrade`.

**Verify**:
- The agent adapter contract is extended with `SpawnSubAgent` and `InvokeSkill` primitives (names final during engineering handoff).
- The Claude Code adapter implements both primitives; the Generic CLI adapter either implements them or declares them unsupported (falling through to the fresh-session fallback).
- Running `/parlay-loop` on Claude Code runs end-to-end in a single user session with three sub-agents transparently.
- Running `/parlay-loop` on an adapter without sub-agent support completes end-to-end across three user sessions, with each phase-group boundary printing an explicit `/parlay-loop ... --from {next-phase}` hint the user copies into a fresh session.
- No `--resume` flag exists. Invoking `/parlay-loop {feature} --resume` returns an error explaining that resumption is done via `--from {phase}`.
- A newly added agent adapter that implements the full contract gets `/parlay-loop` support automatically, without code changes to the loop itself.
- If the adapter lacks a required primitive entirely (not even the fallback works), the loop reports which primitive is missing.

---
