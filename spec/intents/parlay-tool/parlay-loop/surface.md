# Parlay-loop — Surface

---

## Loop Invocation and Feature Resolution

**Shows**: status, message
**Actions**: invoke
**Flow**: guided-flow
**Source**: @parlay-tool/parlay-loop/run-the-full-design-loop-from-one-command, @parlay-tool/parlay-loop/resolve-features-inside-initiatives

**Page**: parlay-loop
**Region**: header
**Order**: 1

**Notes**:
- Output at the start of every `/parlay-loop` invocation.
- `invoke` is the command itself: `/parlay-loop {feature} [--from {phase}]`.
- `status` reports the resolution outcome: "Resolving `upgrade-plan`… found 1 feature at `spec/intents/upgrade-plan/`".
- `message` names the planned path forward: the starting phase (default `intents`), the total number of phases remaining, and that confirmations will occur at each boundary.
- The fragment declares `guided-flow` because it opens the overarching multi-step sequential process that the whole loop constitutes.
- On unambiguous match → transitions straight into Phase Progress Announcement.
- On zero matches → transitions to New Feature Confirmation.
- On multiple matches → transitions to Feature Disambiguation.

---

## Feature Disambiguation

**Shows**: message, data-list
**Actions**: select-one
**Source**: @parlay-tool/parlay-loop/adapt-between-new-and-existing-features, @parlay-tool/parlay-loop/resolve-features-inside-initiatives

**Page**: parlay-loop
**Region**: header
**Order**: 2

**Notes**:
- Shown only when the feature search returns more than one match (bare name collides across top-level and initiative-nested locations).
- `data-list` enumerates each candidate with its full path: `spec/intents/{feature}/` vs `spec/intents/{initiative}/{feature}/`.
- `select-one` picks the intended feature. No default — ambiguity must be resolved explicitly.
- After selection, transitions to Phase Progress Announcement.

---

## New Feature Confirmation

**Shows**: message
**Actions**: select-one, confirm, dismiss
**Source**: @parlay-tool/parlay-loop/adapt-between-new-and-existing-features

**Page**: parlay-loop
**Region**: header
**Order**: 3

**Notes**:
- Shown only when the feature search returns zero matches.
- `message` names the feature that was not found and asks whether to create it.
- `select-one` captures the intended location: top-level vs a specific initiative (the user names which).
- `confirm` triggers invocation of `parlay-add-feature` with the chosen location; `dismiss` exits the loop cleanly without creating any files — protecting against typos in the feature name.
- After confirmation, transitions to Phase Progress Announcement for the intents phase on the freshly-created feature.

---

## Phase Progress Announcement

**Shows**: status, message
**Source**: @parlay-tool/parlay-loop/run-the-full-design-loop-from-one-command, @parlay-tool/parlay-loop/start-from-a-specific-phase, @parlay-tool/parlay-loop/manage-context-across-phases-via-phase-group-sub-agents

**Page**: parlay-loop
**Region**: main
**Order**: 1

**Notes**:
- Printed at the start of each phase inside a sub-agent (intents, dialogs, artifacts, build, code).
- `status` names the current phase — "Entering the **dialogs** phase".
- `message` names what is about to happen: the underlying skill being invoked (`parlay-scaffold-dialogs`, `parlay-create-artifacts`, etc.) and what inputs it will use.
- When `--from` lands mid-group, the message also lists the on-disk artifacts being pre-loaded into the sub-agent's context (e.g., "Pre-loading intents.md and dialogs.md from disk").
- No action — this is pure informational output; the skill that runs next drives its own interaction.

---

## Phase Confirmation Prompt

**Shows**: status, message
**Actions**: select-one
**Flow**: review-and-approve
**Source**: @parlay-tool/parlay-loop/confirm-before-advancing-to-the-next-phase, @parlay-tool/parlay-loop/manage-context-across-phases-via-phase-group-sub-agents

**Page**: parlay-loop
**Region**: main
**Order**: 2

**Notes**:
- Shown at the boundary between every pair of phases. Mandatory — the loop never auto-advances.
- `status` reports the just-completed phase.
- `message` names the next phase the user would advance to.
- `select-one` offers exactly three choices: proceed, stay-and-revise, exit.
- At phase-group boundaries (designer → build, build → code), the `message` additionally warns that advancing will spawn a **fresh sub-agent** that loses the current conversation context. This is a pre-confirmation warning, not a separate prompt.
- Declining ("stay") keeps the user in the current sub-agent and lets them iterate; exiting transitions to Exit Summary.

---

## Gap Analysis Report

**Shows**: summary, data-list, message, status
**Actions**: select-one
**Source**: @parlay-tool/parlay-loop/analyze-gaps-and-hold-the-user-in-intents-dialogs-until-resolved

**Page**: parlay-loop
**Region**: main
**Order**: 3

**Notes**:
- Shown only at the end of the intents phase and at the end of the dialogs phase (not at other phases).
- `summary` gives counts: "Critical: 2, Minor: 1".
- `data-list` enumerates each gap with its severity tag and a pointer to the offending intent or dialog (e.g., "[Critical] intent 4 has unresolved Questions").
- `status` is "no critical gaps" when everything is clean; the loop still advances via the Phase Confirmation Prompt.
- `message` gives the recommendation when gaps exist ("recommend resolving critical gaps before advancing").
- `select-one` offers: stay-and-resolve, advance-anyway, exit. Advancing despite critical gaps is allowed but not persisted — later resumption via `--from` re-analyzes and re-warns.

---

## Readiness Check Result

**Shows**: status, message, data-list
**Actions**: select-one
**Source**: @parlay-tool/parlay-loop/analyze-gaps-and-hold-the-user-in-intents-dialogs-until-resolved

**Page**: parlay-loop
**Region**: main
**Order**: 4

**Notes**:
- Shown at the boundary entering the **build** phase. Output of `parlay check-readiness --stage build-feature`.
- `status` is pass / warning / error.
- `data-list` shows each error or warning with its code (e.g., `fragment-missing-source`) and a human-readable description of the offending artifact location.
- `message` names the consequence: errors are hard blocks (no acknowledgement); warnings are informational.
- `select-one` options differ by result:
  - Errors present: only `go-back-to-artifacts` or `exit` (no "advance anyway" — errors are not acknowledgeable).
  - Warnings only: `proceed-to-build` or `stay-in-artifacts` or `exit`.
  - Clean: no prompt, transitions directly to Phase Progress Announcement for build.

---

## Sub-Skill Failure Prompt

**Shows**: status, message, code
**Actions**: select-one
**Source**: @parlay-tool/parlay-loop/run-the-full-design-loop-from-one-command

**Page**: parlay-loop
**Region**: main
**Order**: 5

**Notes**:
- Shown when an underlying `/parlay-*` skill invocation returns an error (validation failure, read failure, unexpected exception) mid-phase.
- `status` names the failure: "Build phase failed".
- `message` describes the error at a level the user can act on.
- `code` shows the raw error detail (validation JSON, stack excerpt, etc.) so the user can copy it for debugging.
- `select-one` offers three choices: retry the same skill invocation, stay-in-phase to debug interactively with the sub-agent, or exit the loop with a resume hint pointing at the current phase.
- Unlike the Phase Confirmation Prompt, "proceed" is NOT an option — a failed skill has no output to proceed with.

---

## Phase-Group Handoff (Fresh Session)

**Shows**: message, code, status
**Actions**: acknowledge
**Source**: @parlay-tool/parlay-loop/support-all-parlay-supported-agents

**Page**: parlay-loop
**Region**: main
**Order**: 6

**Notes**:
- Shown only when the current agent adapter lacks sub-agent spawning support (e.g., the Generic CLI adapter).
- Replaces the sub-agent transition at each phase-group boundary.
- `status` names the completed phase-group.
- `message` explains the handoff: the user must start a fresh session to continue.
- `code` shows the exact resume command to copy, e.g., `/parlay-loop @auth-overhaul/login --from build`.
- `acknowledge` closes the current session; there is no in-place continuation on this adapter.
- No persisted state — on resume, the fresh `/parlay-loop --from` invocation is the only continuity mechanism.

---

## Exit Summary

**Shows**: summary, data-list, message
**Source**: @parlay-tool/parlay-loop/end-the-loop-cleanly

**Page**: parlay-loop
**Region**: footer
**Order**: 1

**Notes**:
- Printed at every loop exit: natural completion (after `code`), user-chosen exit at a confirmation prompt, and — when detectable — after a recovered session interruption.
- `summary` reports: feature reference, phases completed, outcome (success / user-exit / partial).
- `data-list` enumerates the artifacts produced with their on-disk paths (intents.md, dialogs.md, surface.md / infrastructure.md, buildfile.yaml, testcases.yaml, generated code files).
- `message` includes a resume hint (`/parlay-loop {feature} --from {next-phase}`) only on partial exit; natural completion omits it.
- Never includes cleanup actions — the loop does not delete or roll back any files on any exit path.

---

## Input Validation Feedback

**Shows**: message, data-list
**Source**: @parlay-tool/parlay-loop/start-from-a-specific-phase, @parlay-tool/parlay-loop/support-all-parlay-supported-agents

**Page**: parlay-loop
**Region**: header
**Order**: 4

**Notes**:
- Shown when the invocation itself is malformed — invalid `--from` value, use of unsupported `--to`, use of unsupported `--resume`, or adapter-level primitive missing entirely.
- `message` names the specific problem and the recommended next action.
- `data-list` enumerates valid alternatives (e.g., the five canonical phase names for an invalid `--from`; the equivalent `--from` command for `--resume`).
- No action widget — the loop exits immediately; the user re-invokes with corrected input.

---
