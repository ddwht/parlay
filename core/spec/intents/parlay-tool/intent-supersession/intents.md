# Intent supersession

> A frozen founding intent can be recorded as replaced by a later decision, so a feature's premise can change without editing the document that states it. Parlay already has one "this replaces that" model at the fragment level (`supersedes:` on a surface fragment) and at the amendment level (`supersedes:` in the ledger); `surface.schema.md` calls it "deliberately the same concept ... one 'this replaces that' model at every level". It does not exist at the intent level, so a founding intent can never become history — only be contradicted. This feature completes the principle the project already states.

---

## Supersede a founding intent through the amendment ledger

**Goal**: Record that a founding intent's premise no longer holds and name the decision that replaces it, so a feature can evolve without anyone editing a frozen document or leaving the spec permanently contradicting the code.
**Persona**: Parlay tool maintainer
**Priority**: P0
**Context**: `amendment.schema.md` requires `affects:` and constrains its refs to contract entries — "Never an intent ref — amendments change the contract, not the frozen founding docs". An amendment therefore has nothing to resolve against in a feature that owns no contract artifact. In this repo that is not an edge case: 18 of 27 `parlay-tool` features carry no feature-local contract artifact, and on the strictest reading at least 17 have no natural amendment target at all. Every one of them is live. Four exist to deprecate something — `deprecate-buildfile-adapter`, `deprecate-buildfile-models`, `deprecate-domain-model-operations`, `deprecate-prototype-framework` — and cannot themselves be revised, which is direct evidence the project has repeatedly needed to retire commitments and has done it outside the ledger every time. `build-feature` already states the principle at ingestion — "where the founding docs and the contract artifacts disagree, the artifacts win ... the founding documents are narrative context" — but that rule needs a contract artifact to do the disagreeing, and a protocol-only feature has none, so nothing can ever override its intent. The two options available today are to edit a frozen document, which `check-drift` correctly reports as `ledger_integrity`, or to let the spec contradict the implementation indefinitely.
**Action**: Add a `supersedes_intents:` field to the amendment front matter, naming intent slugs in the amendment's own feature. Permit `affects:` to be empty only when `supersedes_intents:` is non-empty, producing a governance amendment that does not pretend to splice a contract artifact. The superseded document is never modified.
**Objects**: amendment, founding-intent, ledger, feature, supersession, governance-amendment

**Constraints**:
- The frozen file is never written to. Supersession records a later decision beside it; it does not edit, annotate, or delete the document it supersedes.
- The field is named `supersedes_intents:`, not `retires:`. The amendment is the replacing decision, parallel to amendment-level `supersedes:` — a commitment must not be able to disappear without a replacement taking its place.
- `affects:` keeps its existing meaning and is not extended to accept intent refs. It resolves contract entries and drives dirty-set calculation, splice targeting, rebuild scoping and overlap detection; an intent retirement is none of those, and sharing the field would make `dirty_set` ambiguous and force an "except intent refs" branch into every consumer.
- `affects:` may be empty only when `supersedes_intents:` is non-empty. An amendment with both empty remains invalid.
- Refs resolve only to intent slugs in the amendment's own feature. Cross-feature pressure may be recorded in `trigger:`, but one feature may never retire another's founding promise.

**Verify**:
- An amendment naming a founding intent in `supersedes_intents:` with an empty `affects:` validates, and the superseded `intents.md` is byte-identical afterwards.
- An amendment with both `affects:` and `supersedes_intents:` empty fails validation.
- A `supersedes_intents:` ref naming an intent in another feature fails with a dedicated code.
- A `supersedes_intents:` ref naming no intent in this feature fails with a dedicated code.
- Editing a superseded `intents.md` still raises `ledger_integrity` — supersession grants no hash exemption, because the file is never touched.

---

## Refuse a supersession that abandons work rather than replacing it

**Goal**: Make retirement a deliberate, reviewable decision with a successor, so the mechanism cannot become a way to delete any commitment that has become inconvenient.
**Persona**: Parlay tool maintainer
**Priority**: P0
**Context**: A supersession reduces scope and authority. If it were accepted on the strength of a free-text reason, an agent could manufacture that reason, and a founding promise would be one sentence away from disappearing. The guard has to be structural — resolvable refs, mandatory successor criteria, and an accounting of what the retired intent was producing — rather than a string a validator cannot judge.
**Action**: Require a successor decision and a full accounting of downstream scope before a supersession is accepted; block the cases where retirement would orphan generated work, fork the ledger, or empty a feature.
**Objects**: supersession, acceptance-criteria, contract-entry, disposition, ledger-fork, decision-protocol

**Constraints**:
- An intent-superseding amendment must carry a non-empty `## Why` and a non-empty `## Acceptance`. The rename and pure-prose exemptions that apply to ordinary amendments do not apply here; the Acceptance becomes the replacement's active criteria.
- Every contract entry sourced wholly or partly from a superseded intent must be given an explicit disposition — named in ordinary `affects:` as replaced, removed or retained — or proven not to exist. A protocol-only feature satisfies this with zero entries; a feature carrying artifacts must account for each one.
- A superseded intent may not be superseded again by a second live amendment. A second claimant is a fork and blocks, unless it supersedes the first through the existing amendment `supersedes:` relation.
- The last live intent of a feature may not be retired this way. A feature with no live intent is a lifecycle question with its own dependency checks, not a ledger entry.
- Supersession is never auto-accepted under `--non-interactive`. It raises a decision with no safe default, presenting the superseded Goal and Verify, the replacing Change and Acceptance, and the downstream disposition.
- Identity recorded on the decision is attribution, not proof that a person judged it. The safety comes from the unanswerable-by-default workflow and the ledger record, not from the recorded name.

**Verify**:
- An intent-superseding amendment with an empty `## Acceptance` fails validation, and so does one with an empty `## Why`.
- A contract entry whose `source:` names the superseded intent, with no matching `affects:` disposition, fails with a dedicated code naming the entry.
- A second live amendment superseding an already-superseded intent blocks; the same amendment validates once it names the first in `supersedes:`.
- Superseding the only live intent of a feature fails.
- Under `--non-interactive` a supersession raises a blocked decision and writes nothing.

---

## Resolve current specification from live intents plus applied supersessions

**Goal**: Give every consumer one answer to "what does this feature currently promise", so a superseded intent stops being read as current without any consumer growing its own retirement filter.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: Founding intents are read by coverage, drift and readiness checks, the handoff projection, the designer and build phase ingestion, and every walker that reads `Verify:` bullets. If supersession were applied by each of those independently, they would disagree, and the disagreement would surface as a contradiction between phases rather than as a bug. The amendment ledger already faces this and answers it with `check-amendments` plus a `superseded_by` map, stating that "an amendment with a `superseded_by` entry is history, not specification" — the same resolution belongs one level up.
**Action**: Add a single resolver returning a feature's live founding intents together with the applied amendments that supersede the rest, and route every consumer of founding intents through it.
**Objects**: resolver, active-specification, applied-tail, baseline, handoff-projection, verify-walker

**Constraints**:
- Supersession takes effect only when applied. An authored but unapplied supersession is proposed specification and blocks the boundary; it is not permission to ignore the old intent while artifacts and code still reflect it.
- Application follows the existing applied-tail model — it advances `last-applied-amendment` on the feature baseline. A feature with no contract artifact still requires a real completion step rather than an automatic advance.
- Consumers do not implement their own retirement test. Coverage, drift and readiness, handoff projection, phase ingestion and the `Verify:` walkers all read the resolver.
- A superseded intent's Goal and Verify are rendered as history, not omitted. The record of what was promised and why it was replaced stays legible to a later reader.

**Verify**:
- After an intent-superseding amendment is applied, the feature's current criteria contain the replacement's Acceptance and no longer contain the superseded intent's Verify bullets.
- Before it is applied, the same amendment leaves the old intent's Verify bullets current and blocks the boundary.
- `check-amendments` reports the supersession chain, naming which amendment superseded which intent.
- The handoff projection shows a superseded intent under history with the amendment that replaced it, rather than dropping it.
- A cyclic or self-referential supersession chain is detected and fails rather than resolving.
- The feature that introduces the mechanism is unaffected by it: this feature's own intents are active under the resolver as it behaves before the extension lands, and the resolver returns the same active set for them afterwards. Installing supersession does not retroactively alter the authority of the feature that installed it.
