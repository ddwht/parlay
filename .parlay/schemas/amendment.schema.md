<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/ledger-and-contract
parlay-extends: parlay-tool/intent-supersession/supersede-a-founding-intent-through-the-amendment-ledger
parlay-extends: parlay-tool/intent-supersession/refuse-a-supersession-that-abandons-work-rather-than-replacing-it
-->

# Amendment Schema

File: `spec/intents/<feature>/amendments/NNN-<slug>.md`. The append-only change ledger of the ledger-and-contract model: one file per change, written once and **never edited** — a later change that alters the same ground is a NEW amendment that names the old one in `supersedes:`. A standard zone of every feature (since v0.4 the ledger model is the only regime); a feature that has never been refined simply has no `amendments/` directory yet.

An amendment records a refinement to an existing feature: what changed, why, and — machine-readably — which contract artifact entries it touches. The apply step splices the delta into those artifacts and lands the `## Acceptance` bullets as `verify:` entries on them. The founding documents (intents.md, dialogs.md) are frozen at feature birth and are never modified by an amendment.

## Structure
<!-- parlay:normative -->



```markdown
---
amendment: <slug — must equal the filename's <slug> part>
date: <YYYY-MM-DD>
trigger: <one line: what prompted this — a user ask, another feature's need (@feature), a defect>
affects:
  - "@<feature>/<kind>:<name>"
supersedes:
  - <earlier amendment slug this replaces — omit or empty when none>
supersedes_intents:
  - <founding intent slug in THIS feature that this decision replaces — omit when none>
retires_feature: <true when this record closes the whole feature — omit otherwise>
outcome: <replaced | obsolete — required with retires_feature>
replacement_feature: <@feature that carries this work now — required by replaced, forbidden by obsolete>
---

## Change
<the delta, in prose — what is different after this amendment>

## Why
<the reasoning — recorded here because nothing else records it>

## Acceptance
- <criterion the apply step lands as a verify: entry on the affected entries>
```

| Field | Required | Description |
|---|---|---|
| `amendment` | Yes | Slug identity. Must match the filename slug — a file may not disagree with its own name. |
| `date` | Yes | ISO date the amendment was decided. |
| `trigger` | No | What prompted the change. Cross-feature pressure (`@export needs …`) is exactly what this field exists to record — the causal link that previously lived nowhere. |
| `affects` | Yes, unless `supersedes_intents` is non-empty | The declared dirty set: contract entries this amendment changes, as `@<feature>/<kind>:<name>` refs with kind one of `operation` (capabilities.yaml id), `surface` (fragment name slug), `infrastructure` (fragment heading slug), `domain` (root-model entity name). Never an intent ref: this field routes contract splices and drives the dirty set, and retiring a founding intent is a different act with its own field (`supersedes_intents`). |
| `supersedes` | No | Earlier amendment slugs in the same ledger that this one replaces. What compaction and contradiction detection walk. |
| `retires_feature` | No | `true` marks this the feature's **terminal record**: it closes the feature rather than changing it. Declared, never inferred — an amendment that merely names every live intent stays the `amendment-supersedes-last-intent` error it is today, because it carries none of retirement's obligations and a lifecycle transition nobody chose is not one to infer. Permits retiring every live intent, which no other record may. |
| `outcome` | With `retires_feature` | Closed at `replaced \| obsolete`. A reader months later cannot recover from silence whether the work moved or stopped mattering, and that difference is the whole content of the decision. |
| `replacement_feature` | With `outcome: replaced` | The feature that carries this work now. Forbidden by `obsolete`. Must exist, must not be the retiring feature, and must not itself be retired. It is metadata about the outcome and **never permission**: a reference aimed at the retiring feature does not begin aiming at the replacement by being told about it, so `replaced` faces the same zero-inbound rule as `obsolete`. |
| `supersedes_intents` | No | Founding intent slugs **in this amendment's own feature** that this decision replaces. Bare slugs — a qualified `@feature/slug` is refused, because one feature may never retire another's founding promise (record cross-feature pressure in `trigger:`). Naming one makes this a **governance amendment**: `affects:` may then be empty, and `## Why` and `## Acceptance` become required. The superseded `intents.md` is never modified. |
| `amends_intents` | No | The **evolution vocabulary**, and the field new records use. One entry per founding promise: `intent:` (the lineage slug, bare and same-feature), `mode:` (closed at `extend \| revise \| narrow \| retire`), and — for every mode but `retire` — a `version:` block holding the promise's **complete** new text. A promise is a durable decision **lineage**; an amendment creates its next version. See *Intent evolution* below. |
| `## Change` | Yes | The delta in prose. Delta-shaped by rule: an amendment describes a change, never restates a feature. |
| `## Why` | No (strongly encouraged) | The reasoning. |
| `## Acceptance` | Behavior changes: yes | Criteria the apply step lands as `verify:` on the affected entries. Legitimately absent for renames and pure-prose changes. |

<!-- /parlay:normative -->

## Filename and sequence
<!-- parlay:normative -->



`NNN-<slug>.md`, three digits, sequential from `001`. Lexical order equals ledger order in every directory listing. A gap in the sequence is what a compacted ledger legitimately looks like (`amendments/archive/` holds the pre-compaction files); a duplicate is a collision. Files not matching the pattern are invisible to the ledger and reported by `check-amendments`.

<!-- /parlay:normative -->

## Versioning

No `schema_version:` field (see `schema-versioning.schema.md`). Amendments are immutable once written, so there is nothing to migrate in place — a shape change to this schema applies to new files only, and the parser stays tolerant of the old shape, which remains on disk forever by design.

## Diagnostics

Single-file (`parlay validate --type amendment`):

| Code | When it fires |
|---|---|
| `amendment-not-parseable` | Missing or unterminated frontmatter, or invalid YAML in it. |
| `amendment-frontmatter-incomplete` | `amendment:` or `date:` missing. |
| `amendment-affects-missing` | `affects:` is empty — an amendment that names no contract entry cannot be applied or scoped. |
| `amendment-affects-malformed` | An `affects:` entry is not `@<feature>/<kind>:<name>` with a known kind. |
| `amendment-missing-change` | No `## Change` section. |
| `amendment-missing-acceptance` (warning) | No `## Acceptance` bullets — fine for renames and pure-prose changes, wrong for anything testable. |

Ledger-level (`parlay internal check-amendments <@feature>` — JSON, also emits `dirty_set`):

| Code | When it fires |
|---|---|
| `amendment-slug-mismatch` | Frontmatter `amendment:` disagrees with the filename slug. |
| `amendment-out-of-sequence` | Two files share a sequence number, or a file in `amendments/` matches no `NNN-<slug>.md` shape and is invisible to the ledger. |
| `amendment-sequence-gap` (warning) | Sequence numbers jump — expected after compaction, otherwise a numbering mistake. |
| `amendment-supersedes-unknown` | `supersedes:` names no earlier amendment in this ledger. |
| `amendment-supersedes-intent-malformed` | A `supersedes_intents:` entry is empty. |
| `amendment-supersedes-intent-foreign` | A `supersedes_intents:` entry is qualified (`@feature/slug` or `feature/slug`). One feature may not retire another's founding promise; record cross-feature pressure in `trigger:`. |
| `amendment-supersedes-intent-unknown` | A `supersedes_intents:` entry names no intent in this feature's `intents.md`. |
| `amendment-supersedes-intent-forked` | Two live amendments supersede the same intent with no ordering between them. Naming the earlier in `supersedes:` settles it. |
| `amendment-supersedes-last-intent` | The ledger would retire every founding intent of the feature. Retiring a whole feature is a lifecycle operation with its own dependency checks. |
| `amendment-supersession-no-successor` | A superseding amendment has no `## Acceptance`. The rename/pure-prose exemption does not apply: retiring a promise with nothing in its place is deletion. |
| `amendment-supersession-no-rationale` | A superseding amendment has no `## Why`. The frozen intent cannot record why it stopped being true, so this is the only place that reasoning will exist. |
| `amendment-supersedes-governance-incomplete` | An amendment supersedes one that retired an intent, without restating that retirement in its own `supersedes_intents:`. |
| `amendment-retirement-fields-without-marker` | `outcome:`/`replacement_feature:` set without `retires_feature:`. |
| `amendment-retirement-outcome-missing` / `-unknown` | Missing, or outside `{replaced, obsolete}`. |
| `amendment-retirement-replacement-missing` / `-unexpected` | `replaced` without a replacement, or `obsolete` with one. |
| `amendment-retirement-replaces-itself` / `-replacement-unknown` / `-replacement-retired` | The replacement is the retiring feature, does not exist, or is itself retired. |
| `amendment-retirement-incomplete` | The terminal record does not name every live intent. |
| `amendment-retirement-names-retired-intent` | It names an already-retired intent. The set must be exactly the live ones; padded with history it reads complete while a live promise goes unnamed. |
| `amendment-retirement-over-unapplied-tail` | The ledger carries other unapplied records. |
| `amendment-retirement-not-terminal` | More than one record carries the marker, or a record follows the retirement. A feature ends once, and ends last. |
| `feature-retirement-has-output` | The feature still has contract artifacts, a buildfile or testcases. Retirement removes nothing, so those would outlive the feature; refusing is the honest answer. |
| `feature-retirement-scan-incomplete` | An artifact could not be read or parsed, so nothing was established. Unknown is not clean. |
| `feature-retirement-still-referenced` | Something still points at the feature. Names each owning artifact, position and ref. |
| `intent-supersession-unaccounted-affect` | A contract entry whose `source:` names a superseded intent has no disposition in `affects:`. |
| `amendment-affects-unresolved` | An `affects:` ref names an operation/fragment/entity that does not exist in the referenced feature's contract artifacts, and the record declaring it is not trusted applied history (see *Applied history and resolution*). |
| `amendment-compaction-incomplete` | A compaction of this feature was interrupted and its journal is still in place, so the ledger may be half-moved. Every authority writer refuses while it stands — `save-build-state` and `apply-governance` both stop — because recording authority over a half-moved ledger blesses a state nobody intended and that recovery is about to undo. Re-running `parlay internal compact @<feature>` recovers and clears it. |
| `amendment-intent-lineage-unknown` | A transition names a founding promise this feature does not declare. |
| `amendment-intent-lineage-ended` | A transition changes a lineage a previous record already ended. |
| `amendment-transition-receipt-missing` | A trusted-applied `amends_intents:` record has no transition receipt in the baseline. Nothing shows its approval ceremony ran, so its dispositions evidence nothing. |
| `amendment-transition-receipt-invalid` | The receipt exists but does not describe this record: wrong ceremony mode, a promise approved twice or not at all, a closure value that contradicts the mode, or a consequence whose disposition, replacement or fingerprints disagree with the amendment. |
| `amendment-intent-transition-malformed` | An `amends_intents:` entry has an unknown mode, authors the read-only `legacy_supersession`, supplies no new goal for a mode that requires one, supplies text for `retire`, names one lineage twice, or states a lineage in both vocabularies. |
| `amendment-authority-unreadable` | The feature's `.baseline.yaml` exists but its applied-authority record could not be read, so no amendment can be shown applied. Reported rather than degraded to "nothing applied", which would turn every historical ref back into a fatal one and read as drift rather than as a broken baseline. |
| `amendment-scope-overlap` (warning) | A later amendment's `affects:` intersects an earlier amendment's, and the earlier one is not named in the later's `supersedes:`. Two amendments editing the same contract entry with no ordering between them. Naming the earlier in `supersedes:` — the declaration that this change replaces it — silences the warning. |

### Applied history and resolution

An `affects:` ref is resolved against the **current** contract. For a record in
the unapplied tail that is non-negotiable: the entry it claims to change must
exist, or the amendment is describing work against something that is not there.

For a record that is **trusted applied history**, resolution is relinquished.
This is what makes retirement possible at all: `feature-retirement-has-output`
requires a retiring feature's `capabilities.yaml`, `surface.yaml`,
`infrastructure.md` and `domain-model.yaml` to be gone, while whole-ledger
resolution required the entries inside them to exist forever. The two cannot
both hold.

**Trusted applied is a checked fact, never a claim.** A record qualifies only
when *both*:

- `seq` is at or below the baseline's `last-applied-amendment`, **and**
- the baseline's `sources.amendments` entry **for that exact filename** matches
  the whole-file hash of the bytes history retains — in `amendments/`, or after
  compaction in `amendments/archive/`.

A marker moved by hand with no recorded evidence buys nothing. A stored hash
that no longer matches the record buys nothing. A record above the marker is
pending however good its hash looks.

### Intent evolution

A founding promise used to have exactly one possible transition: death.
`supersedes_intents:` said a promise was GONE, and nothing could say it now
READS DIFFERENTLY. Every evolution was therefore modelled as a retirement,
every retirement orphaned the contract entries the promise justified, and
`intent-supersession-unaccounted-affect` existed to make an author clean up
after an orphaning the tool induced.

`amends_intents:` replaces that vocabulary. The slug is a durable decision
**lineage**, never reused; an amendment creates version N+1 of it. Attribution
binds to the lineage, so an entry sourced to a promise stays **attributed**
across a revision.

| mode | meaning |
|---|---|
| `extend` | Same lineage, additive. Prior entries stay attributed and the author attests their support survives. |
| `revise` | Same lineage, replacement text. Scope may move either way, so the promise delta needs approving. |
| `narrow` | Same lineage, weaker scope. Some entries may lose justification. |
| `retire` | The lineage ends and nothing takes the promise over. |

**A version is held to the same minimum as the founding intent it replaces.**
`title`, `goal` and `persona` are required; `priority` must be `P0`, `P1`, `P2`
or omitted. The list fields — `verify`, `constraints`, `objects`, `questions` —
stay clearable, because removing an answered question or a dropped constraint is
what a snapshot is for. Without this, "omission means cleared" would silently
turn required identity fields into clearable ones and let a titleless promise
become a feature's current one.

`questions` is versioned rather than founding-only on purpose: the current work
queue reads it, so a revision that answers or introduces a question must change
it. The founding questions stay preserved in frozen history either way.

**A version is a SNAPSHOT, not a patch.** Every mode but `retire` must supply a
`version:` block carrying the promise's complete new text — `title`, `goal`,
`persona`, `priority`, `context`, `action`, `objects`, `constraints`, `verify`,
`questions`. A field omitted from a version is **absent**, not inherited: that
is what makes a field clearable, and it is why the design chose snapshots over
patch algebra. `retire` must supply no version at all — a promise that is over
does not also read differently.

The lineage **slug is the one field a transition may never change**, because
attribution binds to it. `title` is versioned precisely so a human-facing name
can change without breaking identity.

One record states one transition per lineage, in one vocabulary.

**A retirement written in the new vocabulary carries every obligation the legacy
spelling does — but the SAFETY ones, not the old spelling's semantic fiction.**
Every lineage-ending transition owes a `## Why` and `## Acceptance` describing
what is observably true afterwards, feeds the same scope accounting and the same
terminal-completeness tally, and satisfies `retires_feature:` — a terminal record
naming its promises in `amends_intents:` is not "naming none".

What does NOT carry over is the claim that something must replace the promise.
Legacy supersession assumed one always did, because withdrawal was the only verb
the vocabulary had; `mode: retire` means the opposite by definition. A known
retire is never told that "retiring a promise without stating what replaces it
is deletion" — that would contradict the mode its author explicitly chose and
rebuild inside the validator the conflation this vocabulary exists to remove.
The legacy wording is preserved for legacy records, whose author's intent was
never recorded.

**What is checked, and what is attested.** The tool verifies mode-shape
consistency and structural consequences. It cannot verify the *semantic*
classification: `extend` cannot structurally end its own lineage, so prose
labelled `extend` that actually narrows the promise is precisely the lie no
structural check can refute. That judgement is the author's, and this schema
does not imply otherwise.

**Reading older records.** A record carrying `supersedes_intents:` and no mode
reads as **`legacy_supersession`**, not as `retire`. Executing it as a
retirement is operationally safe and faithful to what the old resolver did; it
is not necessarily faithful to what the author MEANT, because retirement was
the only available spelling and an author intending a revision had no way to
say so. Nothing may report a legacy record's semantics as known.
`legacy_supersession` is derived on read and may never be authored.

**Ledger validation.** A transition must name a founding promise this feature
declares (`amendment-intent-lineage-unknown`), and a lineage that has ended
cannot be changed afterwards (`amendment-intent-lineage-ended`) — a later record
cannot resurrect a promise that is over, and that is an error rather than
something the resolver quietly ignores.

**Projection.** `parlay internal active-spec` answers with the CURRENT version
of each promise: the founding text as amended by every APPLIED transition. The
founding document is never rewritten — it is history, and that is what makes it
readable rather than contradictory. An unapplied transition leaves the founding
text standing and is reported pending, exactly as an unapplied retirement is:
the artifacts and the generated code still make the old promise. A pending
retirement does **not** roll back an already-applied revision — it says the
promise is about to end, not that a change which already happened did not.

Pending transitions are reported **by mode**. An unapplied revision is not a
pending retirement, and saying so would tell an operator a promise is about to
be withdrawn when it is about to be reworded.

### Applying an intent transition

`parlay internal apply-amendment @<feature>` owns the ceremony. No save applies
one — a save records build evidence and approves nothing.

The approval covers **three** propositions, not one:

1. this exact promise now reads differently;
2. the declared mode is semantically honest;
3. the transition does not silently invalidate downstream entries outside its
   declared scope.

So the preflight prints a **field-aware delta** between the promise currently in
force — the applied version, never the founding text, since a lineage may
already have been revised — and the version this record would put in force, with
cleared fields shown explicitly. Then the **attestation** the author is being
asked to make, in plain language and phrased as their claim rather than as a
result the tool checked. Then the **exact contract entries attributed to that
lineage**, partitioned into those this record's `affects:` names and those it
does not.

The third is what makes this more than side-by-side text. A revision can stop
supporting an entry while the lineage stays alive and resolving, so the entry is
never referentially orphaned and no structural check can see the loss.

**`scope_impact:` is required and is the author's.** `version: 1`, an
`exceptions:` list, and — for every mode that leaves a promise behind —
`preserves_unlisted: true`. The closure assertion is that every attributed entry
the record does not list remains supported by the new promise: a human claim
nothing establishes. It is declared rather than inferred from the mode, because
inferring it would mean the ceremony manufacturing a preservation claim the
amendment never made.

**The assertion is MODE-AWARE, because it is a claim about a promise that still
exists.** A retirement leaves no new promise, so on a record whose every
transition is a `retire` the assertion is not merely unnecessary — it is false,
and `preserves_unlisted: true` there is refused as contradictory. Omit it; the
exceptions are the complete account, since a retirement has no closure and every
entry the promise justified owes a consequence.

On a MIXED record — one promise retired, another revised — the flag is a claim
about the living lineages only. It stays **one authored bool**: there is no way
to make independent closure choices per lineage, and none is offered. What is
per-subject is its *interpretation and its record*: the receipt stores `false`
against a retirement subject and `true` against its revising sibling, because a
single record-level value carried into the receipt would either attribute a
false claim to the retirement or drop a real one from the revision.

**An applied record in this vocabulary must carry its receipt.** The ledger's
`intent-supersession-unaccounted-affect` rule does not run for authored claims
because the dispositions replaced it — and that is only sound while something
durable shows the dispositions were checked. Trusted-applied means the marker
covers the record and its stored hash matches the retained bytes; it does not
mean a ceremony ran. So `check-amendments` requires a transition receipt for
every trusted-applied `amends_intents` record, and audits it against the record and
against itself:

- one subject per declared transition, with matching mode, the mode-scoped
  closure value, the mode's exact attestation, and — for a living mode — the
  promise text materialised from the amendment's own `version:` block, with a
  delta its own before and after actually describe. A retirement's subject
  leaves nothing behind and carries no delta;
- every declared exception has a consequence whose disposition, replacement,
  fingerprints and human-readable claim match; no exception is filed under a
  lineage the record does not transition;
- every consequence is present in the subject's own captured pre-splice
  inventory with the same fingerprint. That inventory is itself validated —
  present and exact lineage, canonical unique refs, real digests, no entry both
  declared and undeclared, and a `Named`/`Unlisted` partition matching the
  amendment's canonical `affects:` — before anything rests on it. Otherwise a
  synthesised digest-shaped string passes as evidence that the promise ever
  justified the entry, which is the plausible-absent-ref problem the capture
  exists to close, one layer in;
- the same validation applies to the subject's recorded surviving population,
  and the two are related to each other. A retirement records none: nothing is
  justified by a promise that is over. For a living mode, a captured entry left
  to the closure must appear among the survivors — the closure claims a
  survival, and a receipt that does not show it is asserting one it has no
  record of. A survivor with neither a consequence nor a closure over it is in
  the record for no stated reason;
- wherever a consequence's after subject also appears among the survivors, the
  two must describe the same entry. Not byte identity with the before
  fingerprint — an affected surviving entry may legitimately change — but
  subject continuity. `retained` additionally requires that appearance, because
  its whole claim is that this promise still supports the entry. `revised` is
  exempt from being required to be present, since a revision may re-source an
  entry to whatever justifies it now — but exemption from being required is not
  exemption from agreeing when present, and a receipt binding an entry to one
  thing while recording another as surviving contradicts itself however it got
  that way;
- every captured entry is covered: by a consequence always, or by the closure
  where a living mode asserts one. A retirement has no closure, so a captured
  entry with no consequence is unaccounted for however the exception list reads. Missing is
`amendment-transition-receipt-missing`; mismatched is
`amendment-transition-receipt-invalid`. Both are errors, never a fallback to the
weaker legacy accounting.

The adversary here is a **re-digested** receipt. The digest already refuses one
whose bytes were edited, but it proves a receipt is self-consistent and never
that it describes the record beside it — so the audit compares receipt to
amendment, which is the thing a digest cannot do for itself. This is consistency
and audit validation, not authenticity: the baseline is unsigned, and anyone who
can write it can write a consistent forgery. What the audit removes is the
forgery that is consistent and **still wrong about this record**.

The digest binds the promise text, the exact attributed population **with a
semantic fingerprint per entry**, its partition, the scope declaration, the
attestation, the tested-journal proposition and the complete prior capsule.

A ref alone is not the subject: the human is asserting continued support of what
each entry *promises*, so an edit that rewrites an operation's acceptance
criteria while keeping its id and source produces the same address and a
different promise. The fingerprint is taken over the entry's **resolved generic
form** — each entry decoded so unknown fields survive and aliases and merges are
expanded, then canonically re-encoded and hashed. Not the parsed struct, which
drops keys the parser does not model; and not the literal YAML node, which binds
an alias name while leaving its anchor target unbound. It is —
narrower than a whole-file hash, which would invalidate approvals over unrelated
entries and formatting, and unlike a fingerprint over the *parsed* struct it
covers keys the parser does not model. That distinction is not theoretical:
`summary` is semantic contract text that today's artifacts carry and the parser
drops, so a parsed fingerprint left an edit confined to it invisible.

Aliases and merge keys are **expanded** before fingerprinting. Hashing an entry's
node verbatim would bind `<<: *defaults` — the alias name — while leaving the
anchor target unbound, so changing that target would change what the entry
promises and leave the fingerprint identical.

The derivation also refuses to manufacture an ambiguous subject: a duplicate
operation id or fragment name, a non-sequence entry list, an entry with no
identity, or a disagreement between what the parser found and what the reader
enumerated all refuse rather than resolving by last-writer-wins.

Because attribution comes from mutable contract artifacts, the population and
the amendment's own bytes are re-derived and rehashed under the lock immediately
before the write. Deriving the population **fails closed**: an artifact that
exists and will not parse is an unknown, and an approval cannot be given over a
population with a hole in it. Presence is probed with `Lstat`, so only genuine
absence counts as absent — a dangling artifact is something that cannot be read,
not something that is not there.

**Consequence accounting.** Every entry the closure does not cover carries a
`disposition:`, and each disposition has an operational meaning the tool checks
against the artifacts rather than a label left to the reader:

| disposition | claim | checked |
|---|---|---|
| `retained` | survives, and the changed promise still supports it | present **and** still attributed to that lineage |
| `revised` | survives, this record changed it, and it may now be justified elsewhere | present **and** named in `affects:` |
| `removed` | gone, nothing takes it over | the entry is **not** present |
| `replaced-by` | gone, and the named entry carries its work now | absent, and `replaced_by:` resolves |

Each exception names its `intent:` — the lineage the consequence belongs to.
Explicit, because a removed entry cannot be assigned to a lineage from what is
left in the contract, and without it one consequence would silently satisfy
another lineage's obligation. Identity is `(intent, ref)`: one contract entry
may name several source intents, and a shared entry that disappears owes a
consequence under **each** promise that justified it.

**The replacement ref space is the whole ref grammar.** `replaced_by:` accepts
exactly what `affects:` accepts: `@feature/operation:name`, `@feature/surface:name`,
`@feature/infrastructure:name`, and the root-scoped `@feature/domain:Entity` —
and the feature part may name **any** feature, not only this one. That is the
ordinary shape of a narrowing rather than an exotic case: work that stops being
promised here rarely evaporates, it moves to whatever already owns that ground,
and that owner is frequently in another feature or is a domain entity.

The resolver honours all of it. A replacement is resolved and fingerprinted
wherever it lives, so the approval binds the entry's **content**, not merely its
address, and a cross-feature replacement is no weaker a promise than a local one.
Resolution *failure* is reported as failure: an artifact that exists and will not
parse says so, and is never reported as a missing reference — that message sends
somebody to fix a reference that was correct.

`removed` and `replaced-by` are deliberately not required in `affects:` — an
`affects:` ref must resolve against the current contract and a removed entry does
not, so the disposition *is* the record of the removal.

**Three different questions, never conflated.** *Existence* is asked of the whole
contract by the resolver; *attribution* is asked of the promise's population; and
`affects:` is *declared mutation* — provenance, never evidence of existence. An
entry the splice re-sourced to another promise has left this lineage's population
while remaining in the contract, and calling that `removed` is a lie the
resolver catches.

**Completeness rests on a pre-splice capture.** Scope derived after the splice
cannot see a removed entry, so the refine journal records what each lineage
justified when the amendment was written and before any artifact was mutated.
`apply-amendment` then requires that every entry in that prior population either
survives under the closure or carries exactly one disposition — a disappearance
nobody accounted for is precisely what this accounting exists to catch — and
refuses a consequence declared over an entry the promise never justified.
Without that capture the record would be evidence only that somebody claimed a
consequence.

The capture is bound to the **exact record** — filename, whole-file hash and
lineage set — not merely to a sequence. A sequence is not identity: a
replacement of `NNN-slug.md` keeps it while changing the meaning, and the
ceremony would then consume old evidence for new bytes.

Each consequence is recorded **structurally**, not as prose: the lineage, the
entry as it was (with its fingerprint), the disposition, and — for `retained`,
`revised` and `replaced-by` — the entry or replacement as it now stands, bound
by its own fingerprint and read from the whole-contract index rather than from a
lineage's slice, because the subject may have left that slice. Those subjects
are re-derived under the lock alongside the population, since the population
alone cannot see a replacement.

A `narrow` must declare at least one consequence. A narrowing that takes nothing
away is a revision, and should say so — otherwise `narrow` becomes a quieter
`revise` that skips the delta approval a revision owes.

**All four authored modes apply here: `extend`, `revise`, `narrow` and
`retire`.** A record touching several lineages is all-or-nothing, because a
partly applied one leaves a state no reader can classify.

`retire` is presented as an ENDING, not as a delta. A retirement rendered as a
before/after with every field blanking reads like a rewrite, and the human is
being asked a different question than that — not "is this the right new text"
but "is this promise over". So the ceremony shows the promise being ended in
full, and asks for no closure assertion: `preserves_unlisted` claims the entries
this record does not list stay supported by the promise, and a retirement leaves
no promise to support them.

That is also why a retirement has **no closure**. Every entry the lineage
justified owes a consequence, and nothing may still name the retired promise as
its source afterwards. Two consequences carry most retirements:

- an entry that SURVIVES, re-sourced to whatever justifies it now, is `revised`
  — it is still in the contract, so nothing replaced it; what changed is which
  promise supports it;
- an entry that is gone, with another carrying its work, is `replaced-by`.

`retained` cannot appear under a retirement at all, and is refused in those
terms: it claims the changed promise still supports the entry, and there is no
promise left to do so.

The ledger's `intent-supersession-unaccounted-affect` rule does not run for a
claim authored in this vocabulary. The dispositions ARE that accounting and a
stricter form of it — each says what BECAME of the entry and is checked against
the artifacts, where `affects:` only records that the amendment touched it.
Requiring both would ask the author to say the same thing twice, the second time
more weakly. The rule keeps the legacy `supersedes_intents:` spelling, which has
no dispositions to check.

The legacy spelling still goes to the withdrawal ceremony, for a different
reason: its mode was never recorded, so it cannot be executed as any particular
one, and that ceremony asks what the record meant rather than assuming.

### Applying a combined record

An amendment carrying BOTH `affects:` and `supersedes_intents:` is legal, and
for a built feature it is what the accounting rule PRODUCES: retiring a promise
whose contract entries are still live requires naming each of them in that
amendment's own `affects:`.

Such a record has a splice somebody performed and promises somebody must
approve, and neither half may be applied without the other. `save-build-state`
records build evidence and approves nothing, so it refuses. `apply-governance`
refuses anything carrying `affects:`, because that has a splice to record.

`parlay internal apply-amendment @<feature>` is the applier. It requires two
independent proofs, both bound to the exact record:

1. **Splice evidence** — the refine journal names this amendment and reached
   the test step, exactly as an ordinary refinement must. Relaxed for nothing.
2. **Promise approval** — the founding promises that stop being in force are
   printed in full, and the confirmation is bound by digest over the
   amendment's bytes, the promise set and its text, the affected contract
   entries, and the prior authority capsule.

Run without `--confirm` to see the promises and obtain the digest; re-run with
it to apply. The digest is the full SHA-256 of a canonical payload carrying a
scheme identifier, the feature slug, the amendment's sequence and filename, the
transition mode, the amendment hash, the promise snapshots, the affected refs
and the complete prior capsule. It is a **bearer token**, so it is bound to its
feature and its record: two features holding identical content do not share
one. Any edit to the record, change to the promise set, or movement in the
applied authority produces a different digest.

Every writer of a feature's applied authority — `save-build-state`,
`apply-governance` and `apply-amendment` — goes through one transaction
boundary: acquire the feature's authority lock, re-observe the capsule, refuse
if it no longer matches what the operation planned against, and only then
replace. An atomic rename prevents a torn file, not a lost update, and a lock
one writer skips is not exclusion.

**Do not split such a record into two amendments.** The accounting rule is
per-amendment, so a governance-only half trips
`intent-supersession-unaccounted-affect` for every entry sourced to the
retiring promise. There is no split that satisfies both rules.

### How a record was applied

The baseline records the *method* alongside the evidence, in the same file and
the same atomic write:

- `last-applied-amendment` — how far the ledger is applied. Not the ledger's
  highest sequence: a save moves it only as far as the caller could **prove**,
  and the earlier reading ("a save follows a green build, so the ledger is by
  definition fully applied") is what let a partial save carry a pending
  governance record past it.
- `sources.amendments` — per-filename whole-file hashes: the evidence.
- `outputless-amendments` — optional, per exact amendment **filename**, marking
  a record blessed on a confirmed output-less claim rather than on emitted
  files. Additive, so no schema version changes.
- `transition-receipts` — optional, per exact amendment **filename**, holding
  the COMPLETE canonical approval payload alongside its digest, so the digest
  can be recomputed from the receipt rather than reconstructed from elsewhere.
  Written by `apply-amendment`. A boolean would record only that a code path
  ran; this records what was approved, so the decision is auditable. Note what
  it is *not*: the baseline is not a signed store, so recomputation is
  consistency and audit validation, never cryptographic authenticity. Same
  additive rules and the same absence semantics below.

  **Presence** positively records a confirmed output-less blessing. **Absence
  records only that no method was written by the baseline version or path that
  produced the file** — never that the record was blessed the ordinary way. A
  baseline predating the field has none regardless of how its authority was
  obtained, and a marker advanced by hand carries none either. Legacy method is
  unknown, and no rule may read absence as permission or as trust. A future
  rule that wants to infer ordinary application from absence needs a version
  bump and a migration.

All three are copied forward untouched when a save proves no advance, appended
to only for records an advance actually earned, and never recomputed or removed
by a later save. Re-deriving a hash for an already-applied record would
re-bless an edit to it and mint fresh trusted evidence for a write-once
violation.

The method belongs here rather than in the project-level baseline for three
reasons, each of which loses it: the project baseline is written in a later
stage, so a failure between them leaves authority with no explanation; the next
unrelated save replaces it wholesale; and it is keyed by feature, which cannot
identify *which* amendment was blessed. The project baseline's `outputless:`
list remains, as current-run reporting only.

**Scope is the three feature-local kinds only.** `operation`, `surface` and
`infrastructure` resolve against files retirement deletes, and only they
deadlock. A `domain` ref is root-scoped — it outlives its own feature's
retirement — and a cross-feature ref resolves against another feature's
contract, whose disposal is that feature's own drift responsibility. Neither
gains an exemption.

**The trade, stated rather than discovered.** After trusted application the
tool keeps **file-level** detection that a contract artifact was deleted or
mutated: the baseline's whole-file hashes for `capabilities.yaml`,
`infrastructure.md` and `surface.yaml` still move, and `source-signatures:`
remains the hard emission gate. What is relinquished is **entry-level
historical attribution** — drift reports that `capabilities.yaml` changed, not
that operation `X`, which amendment 002 once edited, is gone. That is the price
of letting retirement dispose of feature-local contracts, and it is deliberately
paid.

A tolerated historical ref stays in `all_affects`, which is the cumulative
audit footprint; dropping it would make that footprint silently lose exactly
the retired history this rule preserves. It never enters `dirty_set`, which is
tail-only and scopes rebuilds.

## Superseding a founding intent

The founding documents are frozen at feature birth and no amendment modifies
them. That freezes the *file*, and for a long time it also froze the *promise*:
`affects:` resolves contract entries, so an amendment had nothing to name in a
feature that owns no contract artifact. Such features are not rare — in parlay's
own tree, 18 of 27 `parlay-tool` features carry no feature-local contract
artifact, and four of them exist to deprecate something and could not themselves
be revised. Their only options were to edit a frozen document, which
`ledger_integrity` correctly reports, or to let the spec contradict the code
indefinitely.

`supersedes_intents:` closes that. It names founding intents in the amendment's
own feature that this decision replaces, and it makes `affects:` optional — an
amendment naming only superseded intents is a **governance amendment**, which
changes what a feature promises without claiming to splice an artifact. This is
the same "this replaces that" relation `supersedes:` uses one level down, and
that `supersedes:` on a surface fragment uses one level down again; `surface.schema.md`
calls it "deliberately the same concept ... one 'this replaces that' model at
every level". Intent level was the level it was missing from.

Four rules keep supersession from becoming deletion:

- **A successor is required.** `## Why` and `## Acceptance` are both mandatory
  here, with no rename or pure-prose exemption. `## Acceptance` is the successor
  criteria the apply step lands as `verify:` entries on the affected contract
  entries, the same as for any other amendment — the splice is what makes them
  current truth, not the ledger file. The field is named `supersedes_intents`
  rather than `retires` for this reason: a commitment may be replaced, not
  dropped.
- **Scope is accounted for.** Every contract entry whose `source:` names a
  superseded intent must appear in `affects:`, and `## Change` must say what
  becomes of it — replaced, removed or retained. `affects:` carries refs only,
  so it proves enumeration and never which disposition was chosen; that lives
  in the prose. A feature with no contract artifact satisfies this with an
  empty set; a feature with artifacts must say what becomes of each, or the
  generated scope outlives the promise that justified it. **Accounting is per
  standing decision:** an amendment that replaces an earlier one restates the
  disposition, because the earlier is history and no longer speaks.
- **Only live heads count.** A fork is more than one *standing* decision
  retiring the same promise. Ordering after a decision that has itself been
  replaced settles nothing, and a genuine chain of replacements has one head
  and is not a fork.
- **Replacing a retirement means taking it over.** An amendment that names a
  governance amendment in `supersedes:` must restate its `supersedes_intents:`,
  and inherits the `## Why`, `## Acceptance` and scope accounting that come
  with it. Otherwise the two rules disagree: read the retirement as standing
  and it rests on a record the ledger calls history; read it as lapsed and the
  promise silently returns, un-retired by an ordinary amendment that never
  faced the decision gate retiring it required. `amendment-supersedes-governance-incomplete`.
- **One decision at a time.** Two live amendments superseding the same intent
  fork the ledger and block, unless the later names the earlier in `supersedes:`.
- **A feature must still promise something.** Retiring the last live intent
  fails; whole-feature retirement is a lifecycle operation with broader
  dependency checks.

Supersession takes effect only once **applied**. An authored but unapplied
supersession is proposed specification and blocks the boundary — the artifacts
and the code still reflect the promise it proposes to withdraw. And it grants no
exemption from byte-integrity: editing a superseded `intents.md` still raises
`ledger_integrity`, because the file is never touched. What changes is semantic
authority, not the hash. `check-amendments` reports the link in
`superseded_intents`, since the forward direction cannot live in the frozen file.

## Reading the ledger

An amendment file alone is not current truth: files are immutable, so a superseded amendment
still asserts its retired behavior and carries only whatever `supersedes:` links existed when
it was written — the forward direction is never in the file. Before relying on any amendment's
content, run `parlay internal check-amendments <@feature>` and consult its `superseded_by`
map; the handoff projection renders the same links in its History section. An amendment with a
`superseded_by` entry is history, not specification.

## The dirty set

`check-amendments` emits `dirty_set` — the resolvable `affects:` refs of the **unapplied tail** only: amendments whose sequence exceeds the feature baseline's `last-applied-amendment`. Everything at or below that sequence was already folded into generated code when the baseline was saved, so it is not what a rebuild must touch. This is the **declared** counterpart of what `parlay internal diff` **infers** by hashing: consumers scope rebuilds, prompts, and (later) test selection with it, while the hash comparison remains as trust-but-verify. A disagreement between the declared set and the observed diff is a bug signal — in the amendment or in the apply — not something to proceed past.

*Which reading won and why (L7):* the tail, not the cumulative union. `dirty_set` names what has changed since the last build — the same question `parlay internal diff` answers — so the two agree. The union kept naming long-applied refs as dirty forever and never converged with the observed diff. The full union is still available, honestly named, as `all_affects` (every amendment's resolvable refs regardless of application state, plus the tolerated trusted-historical refs that no longer resolve — see *Applied history and resolution*) for consumers that want the whole ledger footprint rather than the rebuild-scoping tail. With no baseline (never built, or pre-v3) `last-applied-amendment` reads as 0, so `dirty_set` equals `all_affects` — the conservative from-scratch reading.

## Forward links: `superseded_by`

`supersedes:` points backward — a later amendment names the earlier ones it replaces. The forward link ("which later amendment replaced me?") cannot live in the earlier file, because an amendment is immutable once written: nothing may edit a landed ledger entry to record something that happened after it. So `check-amendments` computes the reverse at read time and emits it as `superseded_by` — a map from each superseded slug to the slugs of the later amendments that supersede it. The map is always present (possibly empty); consumers index it without a nil check. This is the same "compute the forward link rather than mutate the immutable record" move the composition vocabulary uses for surface `supersedes:` (F18).
