# Backlog And Activity

> Record work that was observed and not done, and whether a feature is paused on purpose.

---

## Capture a discovery without interrupting the work

**Goal**: Stop losing the things found mid-implementation, so an observation survives the session it was made in.
**Persona**: An agent or designer part-way through a pipeline phase.
**Priority**: P0
**Context**: Mid-build, mid-codegen, mid-review — a defect, a gap the spec never covered, or a shortcut knowingly taken comes to light, and it is not what the current run is for. Parlay records the code, the decisions enforced in the code, the drift and the coverage judgments, and records nothing about work it noticed and walked past.
**Action**: One cheap command that writes a durable item and returns.
**Objects**: backlog item, kind, priority, capture provenance, evidence

**Constraints**:
- Capture costs no round trip: no prompt, no triage, no ranking unless somebody actually gave one. A capture that costs a question will be skipped in exactly the situation it exists for — inside a subagent that can ask nobody anything.
- Cheap for the CALLER is not sloppy in the WRITER. The record is durable, user-facing and schema-versioned, so malformed input is refused rather than written.
- A capture that fails must never fail the phase that tried. The calling phase reports the failure and carries on.
- Priority is never inferred. Absent means untriaged, which is a fact about the record rather than a default, and it is what lets a listing surface the pile that needs a person.
- Provenance is immutable once written: who observed it, when, and which run. An observation nobody can attribute is one nobody can follow up.
- Admits only work that could become a feature, an amendment or an intent. A flaky CI job or a dependency bump could never resolve into one of those and does not belong.

**Verify**:
- An agent mid-phase records an item and the phase continues.
- An item captured with no priority is reported as untriaged, never as low.
- A malformed item is refused with a published code and a fix, and nothing is written.
- The item names the run that produced it without the caller passing anything.

---

## Decide what to do about one item at a time

**Goal**: Convert a growing list into decisions, so the record of undone work does not become a graveyard.
**Persona**: A designer sitting down to work through what has accumulated.
**Priority**: P0
**Context**: Items have accumulated across many sessions. A wall of them converts into decisions at roughly the rate of zero; the absence of tracking at least looks like the absence of tracking, whereas a graveyard looks like tracking.
**Action**: Hand over one item with its evidence and its history, and take a disposition that carries a reason and a name.
**Objects**: backlog item, disposition, deferral, reviewer

**Constraints**:
- Ceremony scales with the disposition, not with the capture: closing an item takes a closed vocabulary, an attribution and a reason, because closing is where things get silently lost.
- A deferral is not an answer. Attempts accumulate and never close the item, because two people independently unable to decide is a different fact from one attempt overwritten twice.
- Prior deferrals travel with the item, so the next reviewer starts from what the last one could not resolve rather than from nothing.
- Every ending is distinguishable from every other: chose not to, condition went away, fixed directly, merged into another item, became a feature, became an amendment. A reader months later cannot recover from silence which of those happened.
- The three endings that produce nothing the pipeline carries stay apart rather than collapsing into one: "we chose not to", "the condition disappeared" and "somebody changed the system so it no longer holds" are different facts, and the word for the third is deliberately narrow so it does not become a catch-all that drags the model toward generic task tracking.
- Ranking is not a disposition — it leaves the item open — and is offered only where nobody has ranked yet.
- No due dates, no burndown, no nagging.

**Verify**:
- A review hands back one item carrying every prior deferral.
- Deferring twice leaves the item open with two recorded attempts.
- A closing decision without a reason is refused.
- An item corrected directly reports an ending distinct from both "declined" and "obsolete", without naming anything it became.
- A sitting that resolves some and not others reports what it did not reach.

---

## Say whether a feature is paused on purpose

**Goal**: Tell a deliberate pause apart from neglect, so a listing stops reporting the same ambiguity every time it is read.
**Persona**: A designer reading project status.
**Priority**: P0
**Context**: A feature sits at an early phase. Nothing on disk distinguishes "we stopped here deliberately" from "nobody has touched this", and every status line that says `dialogs` and not why is a line that stops being read.
**Action**: A second axis, declared and attributed, that says what somebody decided rather than what the filesystem happens to show.
**Objects**: feature, activity declaration, parking reason, pipeline phase

**Constraints**:
- Activity is orthogonal to phase. Phase says how far the work has come; activity says whether it is moving. A feature can be at dialogs and parked, or at dialogs and simply undeclared.
- A declaration is a person's decision, so it is never inferred from mtimes and never applied in bulk. A feature with no declaration AND no observed pipeline boundary reports `unclassified` until somebody says otherwise, because that is what is true. One whose work is already evident resolves `active` instead: reporting a missing disposition for work plainly under way is a permanent non-problem, and a status line that prints those stops being read.
- A pause carries its reason. A state without one is half a record.
- History is append-only: an existing declaration is never edited or removed, only added to.
- A declaration outranks observation, but a parking that has gone stale — the feature acquired artifacts after it was parked — is surfaced alongside the state rather than silently overriding it.
- A declaration that cannot be read is never resolved against observation. "Is this parked" has no answer there, not a default one and not yesterday's.

**Verify**:
- A parked feature reports parked, with its reason, in both human and machine output.
- On first deployment, every previously undeclared feature with no observed boundary reports unclassified rather than being migrated to parked. (This project had seventeen at that moment; the count is a fixture, not a promise.)
- A parked feature that gains artifacts is reported as parked AND stale.
- An unreadable declaration is reported as unavailable, never as active.

---

## Turn an observation into work the pipeline carries

**Goal**: Let an item leave the inbox as a real feature or a real amendment, so the causal link between what was noticed and what was built is recorded rather than remembered.
**Persona**: A designer who has decided an item is worth doing.
**Priority**: P1
**Context**: An item has been triaged and should become work. Without a route out, the backlog is an inbox nothing ever leaves, and the connection between the discovery and the change that answered it lives only in somebody's memory.
**Action**: Promote it, either into a new feature or into an amendment against a promise that already exists.
**Objects**: backlog item, feature, amendment, trigger, provenance

**Constraints**:
- A new feature and an amendment are different acts: one makes a promise the project has not made, the other changes one it has. The tool does not guess between them.
- Promotion to a feature seeds no Goal. An implementation observation is usually not a user-world outcome and has no Persona, so seeding one manufactures a malformed intent and reports the feature as further along than it is.
- Promotion to an amendment writes nothing. An amendment is authored with a person in the loop, so a command that wrote one alone would record a decision nobody made.
- An item is closed against an amendment only once that amendment exists AND names the item. An item closed against an amendment nobody wrote is an observation lost with a receipt saying it was handled.
- A priority travels as a proposal for the intents phase to confirm, never as a decided rank.
- The item is retained as provenance, never moved and never duplicated into active requirements.
- One feature, one promotion: if two items name the same target, one wins and the other stays open, because whether two observations are the same work is a judgment for a person.
- An interrupted promotion resumes. The scaffold is a sequence of writes rather than a transaction, so a run can die with the feature half-created, and re-running must complete its own work rather than mistake it for somebody else's.

**Verify**:
- A promoted feature reports `planned` and carries the observation as a non-parsing origin link.
- Promotion to an amendment writes nothing and emits the pre-filled trigger.
- Closing against an amendment whose trigger names a different item is refused.
- A promotion interrupted mid-scaffold, re-run, yields one feature, one origin link and one terminal event.

---

## Show what is outstanding without being asked twice

**Goal**: Make the size and shape of undone work visible where the project is already being read, so nobody needs a new habit to find it.
**Persona**: A designer running status, doctor, or a pipeline phase.
**Priority**: P1
**Context**: Capture and triage only matter if what they record reaches somebody. A CLI that no phase is instructed to call does not get called, and an inventory nobody reads is the graveyard again.
**Action**: One listing that answers "what are we not doing", surfaced at the boundaries a person already reads.
**Objects**: inventory, parked feature, finding, root

**Constraints**:
- One inventory, not two: items and parked features answer the same question and appear together, distinguished within the listing rather than by which command was run.
- Reading the backlog inside a phase is one scoped call with one owner; writing to it is unrestricted. A phase that loaded the whole backlog would re-introduce the cost the scoped read-set exists to remove.
- What a scoped read finds is reported to the user, never applied. A phase that folded an item into its work would be taking scope nobody authorized.
- Counts describe the listing in front of the reader; project totals describe the project. Reporting one as the other is a trap for any consumer that reads both.
- Every row carries its root. Two roots may hold the same id prefix or the same feature slug, and a row without its root is one nobody can act on.
- Nothing is silently dropped: a record that cannot be read, a root that could not be enumerated, and a reference that stopped resolving are each reported under their own diagnosis.
- Absence and unavailability are different answers. A target that could not be read is never reported as removed.
- Silent when there is nothing outstanding — an always-present line stops being read.

**Verify**:
- A listing from a parent root includes child items and parked features, each attributed to its root.
- A scoped read reports how many of the project's items concern this feature without conflating the two counts.
- A root that could not be enumerated is named, and the listing does not read as clear.
- A healthy project's status output is unchanged.
