# Intent supersession — Infrastructure

---

## Governance amendment shape

**Affects**: amendment ledger record shape and its required-field rule
**Behavior**: The ledger record gains a field naming founding intents that a later decision replaces. The existing field naming affected contract entries keeps its meaning and its vocabulary unchanged — it is never extended to accept an intent reference, because it drives dirty-set calculation, splice targeting, rebuild scoping and overlap detection, none of which an intent retirement participates in. The rule that a ledger record must declare affected contract entries is relaxed to a disjunction: a record must declare either affected contract entries or superseded intents, and a record declaring neither remains invalid. A record declaring only superseded intents is a governance record, which changes what a feature promises without claiming to splice any artifact.
**Invariants**:
- A ledger record naming a founding intent as superseded, and declaring no affected contract entries, validates.
- A ledger record declaring neither affected contract entries nor superseded intents fails validation.
- A superseded-intent reference that names an intent outside the record's own feature fails with a dedicated code.
- A superseded-intent reference that resolves to no intent in the record's own feature fails with a dedicated code.
- The affected-entries vocabulary rejects an intent reference exactly as it did before this feature.
**Source**: @intent-supersession/supersede-a-founding-intent-through-the-amendment-ledger
**Backward-Compatible**: yes

**Notes**:
- Records written before this feature declare affected contract entries and are unaffected by the relaxation.
- The superseded document is never opened for writing. Byte-integrity checking over founding documents is unchanged and continues to report any edit, because supersession grants no exemption from it.

---

## Retirement admissibility

**Affects**: ledger record validation, contract entry provenance walk
**Behavior**: A record that supersedes a founding intent is admissible only when it carries a successor and accounts for what the retired intent produced. It must state its reasoning and its acceptance criteria, with no exemption for renames or prose-only changes, because the acceptance criteria become the replacement's active promise. Every contract entry whose provenance names the superseded intent must be given an explicit disposition — replaced, removed or retained — among the record's affected entries, or be shown not to exist. An intent already superseded by a live record may not be superseded again unless the later record declares that it replaces the earlier one through the existing record-supersession relation. The last live intent of a feature may not be retired through this mechanism, because a feature promising nothing is a lifecycle question with its own dependency checks.
**Invariants**:
- A superseding record with empty reasoning, or empty acceptance criteria, fails validation.
- A contract entry whose provenance names the superseded intent, absent from the record's affected entries, fails with a dedicated code naming that entry.
- A feature carrying no contract artifact satisfies the accounting rule with an empty affected-entry set.
- A second live record superseding an already-superseded intent fails; the same record validates once it declares that it replaces the earlier one.
- Superseding the only live intent of a feature fails.
**Source**: @intent-supersession/refuse-a-supersession-that-abandons-work-rather-than-replacing-it
**Backward-Compatible**: yes

**Notes**:
- The admissibility rules are structural rather than textual. A free-text justification can be produced on demand by any author, human or agent, and cannot carry the weight of retiring a promise.

---

## Unattended retirement refusal

**Affects**: refinement flow decision protocol
**Behavior**: Superseding a founding intent reduces the scope and authority a feature declared, so it has no safe default and is never resolved automatically when nobody is available to answer. In an unattended run the refinement flow reports the blocked decision and writes nothing. When a person is present, the choice is presented with the superseded goal and verification bullets, the replacing change and acceptance criteria, and the disposition of every affected contract entry, so the decision is made against what is actually being given up.
**Invariants**:
- An unattended refinement run that reaches a supersession reports a blocked decision and leaves the ledger unchanged.
- The interactive presentation includes the superseded goal, the superseded verification bullets, the replacement's acceptance criteria, and the disposition of each affected contract entry.
**Source**: @intent-supersession/refuse-a-supersession-that-abandons-work-rather-than-replacing-it
**Backward-Compatible**: yes

**Notes**:
- Any identity recorded alongside the decision is attribution, not evidence that a person exercised judgement. The protection is that the decision cannot be answered by default, together with the durable ledger record — not the recorded name.

---

## Active specification resolution

**Affects**: founding-intent read path shared by coverage, drift, readiness, projection and phase ingestion
**Behavior**: One resolver answers what a feature currently promises, returning its live founding intents together with the applied records that replaced the rest. Every consumer of founding intents reads it rather than testing for retirement itself, so no two consumers can disagree about whether a promise is current. A supersession takes effect only once applied: until then the superseded intent is still current specification and the boundary is blocked, because the artifacts and the code still reflect the promise the record proposes to withdraw. A superseded intent is rendered as history rather than omitted, so a later reader can still see what was promised and which decision replaced it.
**Invariants**:
- After a superseding record is applied, the feature's current criteria include the replacement's acceptance criteria and exclude the superseded intent's verification bullets.
- Before it is applied, the superseded intent's verification bullets remain current and the boundary is blocked.
- The supersession chain is reported, naming which record superseded which intent.
- A projection of the feature shows a superseded intent under history alongside the record that replaced it.
- A supersession chain that is cyclic or self-referential fails rather than resolving.
- This feature's own founding intents resolve as active both before and after the mechanism is installed.
**Source**: @intent-supersession/resolve-current-specification-from-live-intents-plus-applied-supersessions
**Caching**: per-process
**Backward-Compatible**: yes

**Notes**:
- A feature carrying no contract artifact still requires a real completion step to apply a record, rather than an automatic advance of the applied marker.
- The final invariant is the bootstrap check: installing supersession must not retroactively change the authority of the feature that introduced it.
