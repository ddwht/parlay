# Parlay-loop — Dialogs

---

### Run the Full Design Loop from One Command

**Trigger**: The user runs `/parlay-loop {feature} [--from {phase}]`.

User: `/parlay-loop upgrade-plan`
System: Resolving target `upgrade-plan`… found 1 feature at `spec/intents/upgrade-plan/`.
System: Starting the design loop. Five phases ahead: intents → dialogs → artifacts → build → code. You will be asked to confirm at every phase boundary.
System: Spawning the **designer** sub-agent (shared context for intents, dialogs, artifacts).
System: [designer sub-agent begins] Entering the **intents** phase. The current intents.md has 3 intents. Describe the changes or new intents you'd like to add.

#### Branch: Sub-skill failure mid-phase

System: [build sub-agent] Running `parlay-build-feature`…
System: Error: deep validation failed — `missing-model-reference` in component `upgrade-prompt`.
System: Options: (1) retry the build, (2) exit the loop to fix the surface manually, (3) stay in this phase to debug interactively.
User: Exit — I'll fix the surface by re-running from artifacts.
System: Printing resume hint: `/parlay-loop upgrade-plan --from artifacts`. Exiting.

---

### Start From a Specific Phase

**Trigger**: The user includes `--from {phase}` in the invocation.

User: `/parlay-loop upgrade-plan --from dialogs`
System: `--from dialogs` — skipping the intents phase.
System: Spawning the **designer** sub-agent with intents.md pre-loaded.
System: [designer sub-agent begins] Entering the **dialogs** phase. Running `parlay-scaffold-dialogs` against the existing intents.

#### Branch: Prerequisite artifact missing

User: `/parlay-loop new-feature --from build`
System: `--from build` — but no surface.md exists for `new-feature`.
System: The build phase requires artifacts. Would you like to back up to the **artifacts** phase instead?
User: Yes.
System: Starting from `--from artifacts` instead. Spawning the **designer** sub-agent with intents.md and dialogs.md pre-loaded.

#### Branch: Multiple prerequisites missing

User: `/parlay-loop new-feature --from build`
System: `--from build` — but no dialogs.md and no surface.md exist.
System: The earliest missing prerequisite is **dialogs**. Would you like to start from `--from dialogs` instead?
User: Yes.
System: Starting from `--from dialogs` — dialogs, artifacts, build, code will all run.

#### Branch: Invalid phase name

User: `/parlay-loop upgrade-plan --from surface`
System: `surface` is not a valid phase. Valid phases are: `intents`, `dialogs`, `artifacts`, `build`, `code`. Did you mean `--from artifacts`?

#### Branch: --to flag rejected

User: `/parlay-loop upgrade-plan --from dialogs --to dialogs`
System: The `--to` flag is not supported. The loop always runs forward to `code` once started. For a single-phase run, invoke the corresponding skill directly — in this case, `/parlay-scaffold-dialogs @upgrade-plan`.

#### Branch: --from intents on an existing feature

User: `/parlay-loop upgrade-plan --from intents`
System: Entering the **intents** phase on an existing feature. Your existing intents are preserved. You can append new intents or revise existing ones — both work the same way; just describe what you want to change.

---

### Confirm Before Advancing to the Next Phase

**Trigger**: A phase completes.

System: The **intents** phase is complete. Ready to advance to **dialogs**?
System: Options: (1) Proceed to dialogs, (2) Stay and revise intents further, (3) Exit the loop.
User: Proceed.
System: Advancing to the dialogs phase.

#### Branch: User declines (stay and revise)

System: The **intents** phase is complete. Ready to advance to **dialogs**?
User: Stay — I want to revise intent 2's Verify items.
System: Staying in the intents phase. Describe the revisions you want.
User: [edits intent 2]
System: Intent 2 updated. Let me know when you're ready to re-check for critical gaps and advance.

#### Branch: User chooses exit

System: The **intents** phase is complete. Ready to advance to **dialogs**?
User: Exit.
System: Exiting the loop. Completed: intents. To resume later, run: `/parlay-loop upgrade-plan --from dialogs`.

#### Branch: No confirmation after code (natural completion)

System: The **code** phase completed successfully. No further phases.
System: [prints final summary, exits — see "End the Loop Cleanly"]

---

### Analyze Gaps and Hold the User in Intents/Dialogs Until Resolved

**Trigger**: The end of the intents or dialogs phase.

System: Gap analysis at the end of the **intents** phase:
System:   Critical: 2 intents have unresolved `Questions:` sections (intents 4, 7).
System:   Critical: 1 intent is missing `Goal:` (intent 5).
System:   Minor: 1 intent's Verify items could be more specific (intent 3).
System: I recommend resolving the critical gaps before advancing.
System: Do you want to (1) stay and resolve, (2) advance anyway, (3) exit?
User: Stay.
System: Which gap would you like to address first?

#### Branch: No critical gaps

System: Gap analysis at the end of the **intents** phase:
System:   No critical gaps found. 2 minor observations (informational, not blocking).
System: Ready to advance to **dialogs**?

#### Branch: Dialog-phase gaps

System: Gap analysis at the end of the **dialogs** phase:
System:   Critical: intents 3 and 6 have no matching dialog (coverage gap).
System:   Critical: dialog "Upload File" has no matching intent (orphan).
System:   Minor: dialog "Login" is missing a branch for the "wrong password" Verify item.
System: Recommend resolving the critical gaps.

#### Branch: User advances despite critical gaps

System: Critical gaps still present. You can advance; those gaps will produce ambiguous downstream artifacts. Confirm?
User: Yes, advance anyway.
System: Advancing to the next phase. Note: no acknowledgement is persisted — if you later resume with `--from intents` or `--from dialogs`, these gaps will be re-analyzed and re-warned.

#### Branch: Build-phase entry — fragment-missing-source hard block

System: Entering the **build** phase. Running `parlay check-readiness --stage build-feature`…
System: Hard block: surface fragment `upgrade-prompt-panel` has no `Source:` intent (error: `fragment-missing-source`).
System: This is not an acknowledgeable warning — you cannot advance past it. Back to the **artifacts** phase to fix.
User: OK, send me back to artifacts.
System: Re-entering the artifacts phase to fix the missing Source.

#### Branch: Build-phase entry — warnings only

System: Entering the **build** phase. Running `parlay check-readiness --stage build-feature`…
System: Warnings: 1 intent without surface coverage (`upgrade-plan-creation`). This is informational and does not block.
System: Proceed to build?

---

### Manage Context Across Phases via Phase-Group Sub-Agents

**Trigger**: The loop begins or a phase-group boundary is reached.

System: Spawning the **designer** sub-agent (phases: intents, dialogs, artifacts). This sub-agent will hold all three phases in a single context.
System: [designer sub-agent runs intents → dialogs → artifacts inline, sharing context]
System: All designer-group phases complete. Ending the designer sub-agent.
System: Spawning the **build** sub-agent. This sub-agent starts fresh — it reads the finalized surface.md, dialogs.md, and intents.md from disk and produces buildfile.yaml + testcases.yaml.

#### Branch: --from mid-group pre-loads upstream artifacts

User: `/parlay-loop upgrade-plan --from artifacts`
System: Spawning the **designer** sub-agent. Pre-loading intents.md and dialogs.md from disk (both required as context for the artifacts phase).
System: [designer sub-agent begins at artifacts with prior content in context]

#### Branch: Phase-group boundary (context clear) announced

System: The **designer** phase-group is complete. About to spawn the **build** sub-agent.
System: From here on, I will **no longer remember** the intent/dialog conversation we just had. All finalized artifacts are on disk — the build sub-agent will re-read them as inputs. Ready?
User: Yes.
System: Spawning the build sub-agent.

#### Branch: Inline skill invocation within a group

System: [designer sub-agent, dialogs phase] Running `parlay-scaffold-dialogs` inline. The skill has full context of the intents we just authored — no re-reading from disk.

---

### Adapt Between New and Existing Features

**Trigger**: The loop starts and must determine whether the feature already exists.

User: `/parlay-loop payments`
System: Searching for feature `payments`…
System: Found 1 match at `spec/intents/payments/`. Proceeding with this feature.

#### Branch: Zero matches — confirm creation

User: `/parlay-loop new-thing`
System: Searching for feature `new-thing`…
System: No matching feature found.
System: Would you like to create a new feature named `new-thing`?
System: Options: (1) Yes, top-level, (2) Yes, inside an initiative (which?), (3) No — I mistyped.
User: Yes, top-level.
System: Creating the feature via `parlay-add-feature new-thing`.
System: Feature created at `spec/intents/new-thing/`. Spawning the designer sub-agent, entering the intents phase.

#### Branch: Zero matches — user declines

User: `/parlay-loop new-thng`
System: No matching feature found.
System: Would you like to create a new feature named `new-thng`?
User: No — I mistyped, it should be `new-thing`.
System: Exiting the loop without creating anything. Try again with the correct name.

#### Branch: Zero matches — into an initiative

User: `/parlay-loop reset-password`
System: No matching feature found.
System: Create a new feature? Which location?
User: Inside the `auth-overhaul` initiative.
System: Creating via `parlay-add-feature reset-password --initiative auth-overhaul`.
System: Feature created at `spec/intents/auth-overhaul/reset-password/`.

#### Branch: Multiple matches — disambiguate

User: `/parlay-loop login`
System: Searching for feature `login`…
System: Found 2 possible matches:
System:   1. `spec/intents/login/` (top-level)
System:   2. `spec/intents/auth-overhaul/login/` (inside initiative `auth-overhaul`)
System: Which one did you mean?
User: The one in auth-overhaul.
System: Proceeding with `@auth-overhaul/login`.

#### Branch: Existing feature — preserve existing intents

User: `/parlay-loop upgrade-plan`
System: Found 1 match. Entering the intents phase.
System: The current intents.md has 4 intents. You can append new intents or edit existing ones — both work the same way.
User: Edit intent 2 to add a Verify about timeouts.
System: That change modifies designer-authored content. May I update intent 2's Verify section?
User: Yes.
System: Updated.

#### Branch: Existing dialogs preserved at dialogs phase

System: Entering the dialogs phase. Running `parlay-scaffold-dialogs`.
System: dialogs.md already has content. Running the update pass — I won't overwrite existing dialogs; I'll propose additions for any intent that changed.

---

### End the Loop Cleanly

**Trigger**: The loop reaches an end condition.

System: The **code** phase completed successfully. Final summary:
System:   Feature: `@auth-overhaul/login`
System:   Phases run: intents, dialogs, artifacts, build, code
System:   Artifacts:
System:     - spec/intents/auth-overhaul/login/intents.md
System:     - spec/intents/auth-overhaul/login/dialogs.md
System:     - spec/intents/auth-overhaul/login/surface.md
System:     - .parlay/build/auth-overhaul/login/buildfile.yaml
System:     - .parlay/build/auth-overhaul/login/testcases.yaml
System:     - components/Login.vue (and 3 others)
System: Loop complete.

#### Branch: Mid-phase session interruption

System: [user closes the session mid-intents, or the agent crashes]
System: [no output — artifacts already on disk are preserved]

Later, in a new session:
User: `/parlay-loop upgrade-plan --from intents`
System: Resuming from the intents phase. intents.md has 2 intents already — we were mid-conversation last time. Ready to continue authoring.

#### Branch: No cleanup on exit

User: Exit.
System: Summary… [prints summary]
System: All artifacts on disk are preserved. The loop does not delete or roll back any files. Run `parlay repair @upgrade-plan` if you want to check consistency across the three trees.

---

### Resolve Features Inside Initiatives

**Trigger**: The user invokes the loop with a feature argument.

User: `/parlay-loop upgrade-plan`
System: Resolving target `upgrade-plan`… found 1 feature at `spec/intents/upgrade-plan/` (top-level). Proceeding.

#### Branch: Explicit initiative reference

User: `/parlay-loop @auth-overhaul/login`
System: Resolving target `@auth-overhaul/login`… feature found at `spec/intents/auth-overhaul/login/`. Proceeding.
System: All sub-skill invocations will use the fully-qualified reference `@auth-overhaul/login`.

#### Branch: Unambiguous nested feature via bare name

User: `/parlay-loop reset-password`
System: Resolving `reset-password`… only 1 match at `spec/intents/auth-overhaul/reset-password/`. Proceeding (no disambiguation needed since the match is unique even without the initiative prefix).

#### Branch: Sibling features are never touched

System: Running `/parlay-loop @auth-overhaul/login` does not advance any other feature in the `auth-overhaul` initiative. Only `login` runs through the pipeline.

---

### Support All Parlay-Supported Agents

**Trigger**: The loop is invoked on any supported agent — Claude Code, Cursor, or the Generic CLI adapter.

System: [on **Claude Code**, which supports sub-agent spawning]
System: Adapter: `Claude`. Sub-agent primitive: available (via the Agent/Task tool). Skill-invocation primitive: available.
System: Running the loop end-to-end in this session. [three sub-agents spawn transparently as phase-groups advance]

#### Branch: Cursor adapter

System: [on **Cursor**]
System: Adapter: `Cursor`. Skill-invocation primitive: available (via `/parlay-*` slash commands registered under `.cursor/skills/`). Sub-agent primitive: available via Cursor's Agents feature.
System: Running the loop end-to-end. Phase-group boundaries spawn fresh Cursor agents — three agents in sequence: designer, build, code.

#### Branch: Generic CLI adapter — fresh-session handoff

System: [on the **Generic CLI** adapter, which does not support sub-agent spawning]
System: Adapter: `Generic`. Sub-agent primitive: not available. Using the fresh-session handoff mode.
System: [designer phase-group completes inline in this session]
System: Designer phases complete. To continue with the **build** phase, please start a fresh session and run:
System:   `/parlay-loop @auth-overhaul/login --from build`
System: Exiting this session. All artifacts are on disk.

Later, in a new session:
User: `/parlay-loop @auth-overhaul/login --from build`
System: Resuming at the build phase.

#### Branch: --resume flag rejected

User: `/parlay-loop upgrade-plan --resume`
System: The `--resume` flag does not exist. To pick up where you left off, use `--from {phase}`. For this feature, the next phase is likely `build` — run `/parlay-loop upgrade-plan --from build`.

#### Branch: Adapter lacks a required primitive entirely

System: [on an adapter without sub-agent support AND without the skill-invocation primitive]
System: This adapter lacks the `InvokeSkill` primitive — the loop cannot delegate to `/parlay-*` skills on this agent.
System: Missing primitives: `InvokeSkill`. The loop cannot run on this adapter until the adapter implements this primitive or parlay ships a workaround.
System: Exiting.

#### Branch: Newly-added adapter works without loop changes

System: [a new adapter `fooagent` is added, implementing SpawnSubAgent + InvokeSkill]
System: Adapter: `fooagent`. Both primitives available.
System: `/parlay-loop` runs on `fooagent` end-to-end without any changes to the loop skill itself. The adapter implementations satisfy the contract.

---
