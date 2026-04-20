---
name: parlay-loop
description: "Parlay: Walk a feature end-to-end through the parlay design pipeline"
---

# Loop

Walk a feature end-to-end through the parlay design pipeline — intents → dialogs → artifacts → build → code — as one continuous guided process. The loop orchestrates the existing `/parlay-*` skills rather than re-implementing their logic. Confirmations are mandatory at every phase boundary; context is managed via three pre-defined subagents (parlay-designer / parlay-build / parlay-code) that `parlay init` / `parlay upgrade` deploys to your agent.

## Arguments

- `feature`: The feature reference in standard parlay form — `{feature}` for a top-level feature, `@{initiative}/{feature}` for a feature nested inside an initiative.
- `--from {phase}` (optional): Starting phase. Valid values: `intents`, `dialogs`, `artifacts`, `build`, `code`. Default: `intents`.

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

Within a phase-group, skills are invoked inline (direct dispatch) so that, e.g., the dialogs skill sees the intents authored moments earlier in the same context. Between phase-groups, the loop ends the current subagent and invokes the next one — context clears as a side effect of the subagent boundary; no separate "clear context" primitive is required.

## Steps

1. **Resolve the feature target** — Search `spec/intents/{name}/` and `spec/intents/*/{name}/` for a matching feature folder.
   - **Exactly one match** → proceed.
   - **Multiple matches** → ask the user to disambiguate via AskUserQuestion.
   - **Zero matches** → ask via AskUserQuestion whether to create a new feature and where (top-level or inside a named initiative). On confirmation, invoke `/parlay-add-feature` with the chosen location. On decline, exit cleanly — no filesystem changes.

2. **Validate `--from`** — If specified:
   - Reject any value other than `intents`, `dialogs`, `artifacts`, `build`, `code` with a message listing valid phases.
   - Reject `--to` and `--resume` — the loop has no such flags. Point the user at `--from` or the individual `/parlay-*` skills.
   - If the starting phase has missing prerequisites on disk (e.g., `--from build` but no surface.md), offer to back up to the earliest missing phase.

3. **Plan the phase sequence** — Starting at the resolved phase, the loop will run forward through every remaining phase to `code` (unless the user exits at a confirmation boundary). Never backward — to revise an upstream artifact, the user exits and re-invokes with `--from`.

4. **Detect subagent support** — Check whether the `parlay-designer`, `parlay-build`, and `parlay-code` subagents are available on the current agent (e.g., on Claude Code, attempt invocation via the Agent tool by name; on Cursor, via the `/parlay-{name}` slash command). If available, use them. If not, use fresh-session handoff mode (step 7).

5. **Enter the designer phase-group** (if starting phase is intents, dialogs, or artifacts):
   - Invoke the `parlay-designer` subagent with the feature reference and the starting phase. The subagent's prompt handles the phase flow internally — see `.claude/agents/parlay-designer.md` for its scope.
   - The subagent runs each phase in sequence, invoking the underlying `/parlay-*` skill inline:
     - `intents`: guide the user to author/revise intents.md. Invoke `/parlay-add-feature` only if the feature does not exist (and only after user confirmation from step 1).
     - `dialogs`: invoke `/parlay-scaffold-dialogs @{feature-ref}`.
     - `artifacts`: invoke `/parlay-create-artifacts @{feature-ref}`.
   - Pre-load on-disk upstream artifacts if `--from` skipped phases in this group (dialogs needs intents; artifacts needs intents + dialogs).
   - Run gap analysis at the end of the intents phase and at the end of the dialogs phase (step 8).
   - At every phase boundary, prompt the user for confirmation (step 9).
   - The subagent returns a summary when the designer group is complete or the user exits.

6. **Enter the build phase-group** (at the designer→build boundary):
   - End the designer subagent; announce the subagent boundary to the user — make the context-clear effect explicit, not surprising.
   - At the boundary, run `parlay check-readiness --stage build-feature @{feature-ref}`. Treat errors as HARD BLOCKS — the user cannot advance by acknowledgement; they must fix the underlying artifact (route them back to the artifacts phase). Warnings are informational and acknowledgeable.
   - Invoke the `parlay-build` subagent with the feature reference.
   - The subagent invokes `/parlay-build-feature @{feature-ref}` inline.
   - At the end, prompt for confirmation (step 9).

7. **Enter the code phase-group** (at the build→code boundary):
   - End the build subagent; announce the new subagent boundary.
   - Invoke the `parlay-code` subagent.
   - The subagent invokes `/parlay-generate-code` inline (project-level; no @feature arg).
   - After the code phase completes successfully, end the loop with the natural completion summary (step 10). No trailing confirmation — there is no next phase.

8. **Fresh-session handoff** (adapter without subagent support):
   - At the phase-group boundary, print the exact resume command, e.g. `/parlay-loop @{initiative}/{feature} --from build`.
   - Print "Exiting this session. All artifacts are on disk."
   - Exit the current session. The loop persists NO resume state — no on-disk phase cursor, no acknowledged-gap log. Continuity is the user's memory plus the printed hint.

9. **Gap analysis** (at the end of intents and dialogs phases):
   - **intents phase**: surface intents with unresolved `Questions:` sections, intents missing required fields, contradictory constraints, and any open-questions report from `parlay-collect-questions`.
   - **dialogs phase**: surface intents without dialogs (coverage gaps), dialogs without matching intents (orphans), and dialogs missing branches implied by the intent's Constraints or Verify items. Reuse `parlay sync` / `parlay check-coverage` where applicable.
   - Classify each gap as **critical** or **minor** using fixed agent judgment (no user configuration). Rule of thumb: gaps that cascade into ambiguous downstream artifacts (unresolved Questions, missing required fields, contradictory constraints, intents without dialogs, orphan dialogs) are critical; stylistic or partial-coverage gaps are minor.
   - If critical gaps exist, recommend staying in the phase. The user can advance anyway via the confirmation prompt — no acknowledgement state is persisted; the same gaps are re-analyzed if the user later resumes with `--from` at the same phase.

10. **Phase confirmation** (at every boundary except after code):
    - Use AskUserQuestion to present three options: **Proceed**, **Stay and revise**, **Exit**.
    - Name the just-completed phase and the phase about to begin.
    - At phase-group boundaries, include the fresh-subagent warning as part of the message.
    - On "Stay" — remain in the current phase; let the user iterate. Re-run gap analysis on explicit request.
    - On "Exit" — end the loop with the user-exit summary (step 11).

11. **End the loop cleanly**:
    - **Natural completion** (after code): print a summary with the feature reference, phases run, and key artifacts on disk. No resume hint. Loop complete.
    - **User-chosen exit**: print a summary naming what completed, plus a resume command (`/parlay-loop {feature-ref} --from {next-phase}`).
    - **Mid-phase session interruption**: no special handling — artifacts on disk are preserved; the user re-invokes with `--from`.
    - **No cleanup ever**: the loop does not delete or roll back any files on any exit path.

## Interactive Questions

Use AskUserQuestion (or adapter equivalent) for:
- Feature creation confirmation (zero matches)
- Multiple matches disambiguation
- Phase boundary confirmation (proceed / stay / exit)
- Gap-analysis response (stay / advance anyway / exit)
- Readiness warnings response (proceed / stay / exit)
- Sub-skill failure recovery (retry / stay / exit)
- Backing up to an earlier phase when `--from` prerequisites are missing

## Hard rules

- NEVER auto-advance between phases — confirmation is mandatory.
- NEVER persist resume state to disk — no `.parlay/loop-state.yaml`, no phase cursor, no acknowledged-gap log.
- NEVER run phases backward — forward only.
- NEVER silently overwrite designer-authored files (intents.md, dialogs.md) — per CLAUDE.md file-ownership rules.
- NEVER create a new feature without explicit user confirmation — zero matches must prompt, never auto-create.
- NEVER advance past a `parlay check-readiness` ERROR at the build boundary — errors are hard blocks; only warnings are acknowledgeable.

## Error Handling

- `subagent-not-found` — the required subagent (`parlay-designer`, `parlay-build`, or `parlay-code`) is not available on this agent. Check whether `parlay upgrade` has been run. If the adapter has no native subagent support at all, switch to fresh-session handoff.
- `invalid-phase-name` — `--from` value is not one of the five canonical phases. List valid phases and exit.
- `unsupported-flag` — user passed `--to` or `--resume`. Explain these are not supported and point at `--from`.
- `missing-prerequisite-artifact` — starting phase requires an upstream artifact that does not exist. Offer to back up to the earliest missing phase.
- `sub-skill-failure` — an invoked `/parlay-*` skill returned an error. Surface the error and offer retry / stay-in-phase / exit. "Proceed" is not an option for a failed skill.
- `ambiguous-feature` — feature search returned multiple matches. Disambiguate via AskUserQuestion.
