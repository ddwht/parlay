# Spec evolution: promises that change without dying

Status: **design proposal**, not approved. Written 2026-08-31, after the
applied-authority remediation (`applied-authority-remediation-plan.md`) hit a
wall that turned out to be a symptom rather than a bug.

Prompted by a user observation that reframes the problem: *we still treat
documents as the source of truth and the prose in them as currently
authoritative. An old spec is historical data — true as of its time. Each new
change should layer on top and override the previous decision, or at minimum
flag a mismatch. The state of the system at time T equals its spec at time T.
The new state should equal a new spec.*

## The wall, and why it is a symptom

WP1 of the remediation correctly refuses an amendment carrying both `affects:`
and `supersedes_intents:`, because no single operation can apply one:
`apply-governance` refuses anything with `affects:`, while a splice would
withdraw the named promises with no confirmation shown.

But that shape is not exotic. `intent-supersession-unaccounted-affect`
(`check_amendments.go:739`) requires, at error severity, that an amendment
retiring a founding intent name in **its own** `affects:` every contract entry
whose `source:` points at that intent — the accounting map is
`accountedBy[amendment][ref]`, deliberately per-amendment. Refine's step 3.5
guidance says the same. So retiring a promise whose scope is still live
*produces* the refused shape by construction.

The escape routes do not survive inspection:

- **A later amendment cannot clear it.** The record stays in the pending tail
  and the refusal repeats.
- **Splitting it in the same tail fails.** The governance half immediately
  trips the same accounting rule, because accounting is per-amendment.
- **Re-sourcing the entries first** works mechanically and is dishonest: it
  re-points `source:` at an unrelated surviving intent to dodge the rule. It is
  also frequently impossible — adding a founding intent to a built feature is a
  `ledger_integrity` violation (`baseline.go:462`), so there may be no
  legitimate lineage to re-source to.

The one honest thing an author can do is delete the never-applied amendment
(clean: append-only integrity is anchored on hashes the baseline *recorded*,
and an unapplied record has none) and restructure — but only the *removed*
disposition restructures honestly.

**The root cause is a missing verb.** `supersedes_intents:` says a promise is
GONE. Nothing says a promise now READS DIFFERENTLY. Every evolution is
therefore modelled as death, death orphans everything the promise justified,
and the accounting rule exists to make a human clean up after an orphaning the
tool induced. The rule is not the mistake. The vocabulary is.

## What other spec-driven frameworks do

Checked because the same pressure should exist anywhere specs are durable.

**OpenSpec** ([concepts](https://github.com/Fission-AI/OpenSpec/blob/main/docs/concepts.md))
is close to the target model. `openspec/specs/` holds current truth; a change
is a typed delta — `## ADDED Requirements`, `## MODIFIED Requirements`,
`## REMOVED Requirements`; `openspec archive` mechanically merges the delta into
the spec and moves the change folder to `archive/` as a permanent record.
History is the archive, current truth is derived by application, and the delta
is typed rather than prose.

Two weaknesses relative to what parlay needs: identity is keyed on the
requirement **title**, so a rename breaks lineage; and MODIFIED is a full
restatement carrying a prose note (`(Previously: 30 minutes)`) that nothing can
verify.

**Spec Kit** ([evolving specs](https://github.com/github/spec-kit/blob/main/docs/guides/evolving-specs.md))
does not solve it — it offers three *persistence models* to choose between:
flow-forward (a new feature directory per change, the old one kept as history),
living spec (edit `spec.md`, regenerate downstream), and flow-back (change
originates anywhere and ripples backward). No typed deltas, no change records.
The pain is acknowledged in its tracker: issue
[#1191](https://github.com/github/spec-kit/issues/1191) reports being unable to
refine specs after implementation, closed only via a later PR adding update
commands; [#916](https://github.com/github/spec-kit/issues/916) asks for
best practices on evolving specs at all.

**The decisive difference.** OpenSpec's own documentation states it tracks
nothing about which code or artifacts a requirement justifies, and calls this an
intentional separation. Spec Kit has no equivalent either. That gap is exactly
parlay's `source:` field.

So neither framework hits this wall **because neither models the link parlay
models**. Retiring a requirement is free when nothing downstream is attributed
to it. Parlay's traceability is a real capability, and the wall is the price of
having only given itself a verb for severing it.

## The target model

**1. Three tiers, and the middle one is derived as far as it can be.**

- *History* — append-only decision records. Answers "what was true, and why".
- *Current spec* — derived. Answers "what is true now".
- *Build state* — materialised code, baselines, provenance. Answers "what runs".

Spec Kit's living-spec model gets this wrong by making the human edit tier 2.
OpenSpec gets it right by making `archive` apply deltas mechanically. The
discipline is what makes "current" trustworthy.

**Honest scope through Stages 1–4.** Tier 2 is only half derived in that window:
intents are projected, while contract snapshots are still MUTATED IN PLACE by
the splice. Full materialisation is Stage 5 and optional. Do not read tier 2 as
"nothing is hand-edited" before then — read it as "the promise half is derived,
the contract half is a stored snapshot with recorded provenance".

**2. A promise is a lineage with versions.**

The slug is a durable decision lineage, never reused. An amendment creates
version N+1. Current text is the latest version; history is all of them.
Stronger than OpenSpec's title-keyed identity, which a rename breaks.

**3. A typed evolution algebra: structurally checked, semantically attested.**

One field carrying a mode, rather than several unrelated fields:

| mode | meaning | what it costs the author |
|---|---|---|
| `extend` | same lineage, additive; prior entries stay **attributed**, and the author attests their semantic support is preserved | the closure attestation, plus naming entries the splice changed |
| `revise` | same lineage, replacement text | approve the promise delta; closure attestation plus exceptions |
| `narrow` | same lineage, weaker scope | closure attestation plus every entry judged to lose or change justification — at least one |
| `retire` | lineage ends | dispositions over a **tool-derived** inventory |

**What is checked, and what is attested.** An earlier draft of this document
claimed the tool could refute a mis-declared mode. It cannot, and the
distinction matters enough to name two different failures:

- **Referential orphaning** — an entry whose `source:` lineage is missing or
  dead. A fact about the graph, and decidable, given closed graph and explicit
  resolution semantics.
- **Semantic justification drift** — an entry whose lineage is alive and
  resolving, but whose revised promise no longer entails it. NOT decidable.
  Under `revise` and `narrow` the lineage deliberately survives, so `source:`
  keeps resolving; the mismatch hides behind a valid edge.

`extend` cannot structurally kill a lineage by definition, so the dangerous lie
— prose labelled `extend` that actually narrows the promise — is exactly the one
the graph cannot refute. The honest contract is therefore:

- the tool verifies **mode-shape consistency** and **structural consequences**
  (does the lineage exist and stay active, was an entry removed or changed, was
  every required disposition enumerated, is a referenced version retained and
  trusted);
- the human **attests the semantic classification**;
- for `revise` and `narrow`, the human supplies a **scope-impact declaration**
  whose COMPLETENESS the tool checks against mechanically visible mutations and
  removals — without claiming the prose entails the survivors.

The schema must say this plainly rather than implying the mode is verified.

**4. Attribution binds to the lineage, not the text.**

`source: @feature/lineage` survives `extend` and `revise` untouched, so entries
stay ATTRIBUTED to the lineage. Whether they stay JUSTIFIED is the human
assertion above — stable identity prevents automatic collateral orphaning, it
does not prove continued entailment. Only `narrow` and `retire` can orphan scope
referentially, so the accounting rule applies where it is genuinely right
instead of everywhere.

**4a. Who may apply a transition — a distinct ceremony, shared internals.**

`save-build-state` is storage plumbing: it records build evidence after a caller
has established authority. Five work packages went into removing its ability to
manufacture authority from ledger shape, workflow convention and maximum
sequence. Teaching it to interpret an evolution mode, present a promise delta
and consume a human confirmation would put policy back into the primitive that
caused the bypass — and a flag becomes habitual, as the output-less path already
showed.

So the transition is a distinct public operation (`apply-amendment`), invoked by
refine, which identifies one exact amendment and mode, verifies the journal's
splice and test evidence for that same record, computes and presents the exact
old and new promise versions plus the scope declaration, obtains the
mode-appropriate confirmation, binds that confirmation to hashes of the
amendment, the old and new promise text, the scope declaration and the splice
evidence, and advances the authority capsule atomically through the shared
low-level writer.

`save-build-state` consumes a prepared authority grant; it never creates one.
Internals — planner, hashing, atomic baseline write — are shared. The ceremony
is what stays separate.

Longer term one mode-aware `apply-amendment` should cover splice, revision,
governance and combined transitions, because authority derives from the explicit
transition contract and proof bundle rather than from which executable was
called. That unification belongs above the storage layer.

**4b. Scope impact: exception-plus-closure, not enumeration.**

Neither a bare property assertion nor full enumeration is sufficient. The
declaration says, in substance, *"all unlisted entries sourced to this lineage
retain their semantic justification"*, and then enumerates ONLY entries whose
relationship changes, with a disposition each.

The tool separately derives the mechanically visible change set and requires
every removed or mutated sourced entry to appear in the declaration. It can
verify that listed refs exist, belong to the lineage, and match the actual
splice. It cannot verify that the unlisted survivors are entailed — the closure
statement is explicitly the human attestation.

Per mode: `extend` cannot carry removals at all (structurally incompatible), but
must name entries the splice changed, and naming one does not imply lost
justification. `revise` lists exceptions plus all mechanically changed or
removed entries, dispositioned `retained`, `revised`, `removed` or
`replaced-by`. `narrow` requires at least one narrowing consequence, while the
tool admits it cannot prove the human found every semantic loser. `retire` is
the one case where the tool CAN derive the complete attributed population,
because the lineage dies — so it should generate that inventory rather than make
the author rediscover it, ask for dispositions, and persist the resolved set.

**Why this is not today's pain under a new name.** The pain was never that exact
affected identities were recorded. It was that a revision had to enumerate the
ENTIRE attributed population as though all of it had become collateral. Negative
enumeration means a broad revision touching two of fifty entries records two
entries and one preservation assertion, not fifty boilerplate rows.

**The residual epistemic limit, stated rather than hidden.** Under `revise` and
`narrow` a human can omit an unchanged-looking entry whose justification really
did disappear. No schema can prove otherwise from prose. The contract can only
make the blanket attestation conspicuous, bind it to the exact old and new text
and the derived population, and preserve it durably. For high-risk narrowing the
interface can show the full sourced population for review without requiring the
author to encode every retained entry.

**5. The current spec is a rendered composite.**

`parlay spec @feature` renders the composite view: projected current intents
plus the STORED current contract snapshots. The composite is rendered; its
contract half is not. That is still the "new state equals a new spec"
requirement as a first-class artifact rather than something a reader
reconstructs from frozen prose plus a ledger — but the claim is a rendered
VIEW, not a fully materialised document, until Stage 5.

**6. Time is a query** — `parlay spec @feature --at <amendment>`, with date
queries only if an application chronology earns them (Stage 4).

## What already exists

Verified against this checkout. The projection is not a thing to invent.

| Target tier | Status |
|---|---|
| Immutable history | frozen founding docs, append-only amendments, `archive/` |
| Projection seam | `resolveIntents(cfg, slug, authority)` with two authority modes, consumed by coverage, verify routing and dialog generation |
| Attribution | `source:` on operations, surface fragments, infrastructure fragments |
| Build state | baselines, code-hashes, provenance — hardened by WP1–WP5 |
| "What was true then" | WP3's trusted-applied predicate (marker + exact retained-byte hash) is the primitive a time query needs |

**The precise gap.** `SupersededIntent` (`resolve_intent_authority.go:70`)
carries `{Intent, ByAmendment, Seq, Applied}` — the OLD promise and what killed
it. Nothing anywhere carries replacement text. The projection is subtractive
only: active = founding minus superseded.

## The path

**Stage 0 — finish the two-proof combined applier.** Not part of this design;
it is the safety floor. It unblocks the remediation branch and gives already
stranded records a recovery path. It becomes less load-bearing after Stage 1 but
never disappears: genuine `narrow` and `retire` coupled to contract edits still
need it.

**Stage 1 — intent versioning.** An `amends_intents:` field carrying mode and
new text; `IntentResolution` returns current versions. This dissolves the
ORPHANING: extend or revise a lineage and its entries stay attributed, so the
mandatory enumeration of every affected entry disappears.

It does NOT make such a record ordinary. A revision still combines a
promise-version transition with an artifact splice, and it must not inherit
authority merely from avoiding `supersedes_intents:`. It needs its own contract
— one proof plus a splice — where the workflow shows and durably records the
exact before/after promise delta the human approved. The two-proof applier
remains for withdrawal; this is a sibling of it, not the old splice path
unchanged.

**Stage 2 — bind attribution to lineage.** Mostly narrowing an over-applied
rule: keep accounting for `narrow` and `retire`, drop it for `extend` and
`revise`. Less code than Stage 1.

**Stage 3 — `parlay spec @feature`.** Read-only, new command, no migration, no
pipeline changes. Cheap once Stage 1 exists, and it is the artifact the
requirement actually names.

**Stage 4 — `--at <amendment>`, and date only if earned.** Replaying intent
versions is easy because versions are snapshots, not patches — but the two
query forms are not equally well defined.

`--at <amendment>` is well defined by ledger sequence. `--at <date>` is NOT.
WP3's predicate (marker plus exact retained-byte hash) proves a record belongs
to the currently trusted applied prefix; it records nothing about WHEN that
record became applied. Amendment `date:` is authoring time, and filesystem
metadata is not evidence. So one of three, chosen deliberately:

1. defer date queries entirely;
2. define `--at <date>` as amendment-AUTHORED time and say so in the help text,
   accepting that it answers "what had been decided by then", not "what was in
   force then"; or
3. add an append-only application chronology, which is the only option that
   supports the stronger reading.

**Decided: option 1, deferred, and the flag does not exist.** Not "not yet
implemented" — absent, with the reason in the command's own help text.

Option 2 was the tempting one and is the reason for writing this down. A
`--at <date>` that answers "what had been decided by then" would be a flag whose
name says one thing and whose behaviour is another, which is the exact defect
class this whole line of work has been removing: the founding document that
*reads* like current truth, the receipt that *looks* like evidence, the label
that disagrees with its body. Shipping it would have added one more, in the
command built to fix the original.

Option 3 remains open and is cheaper than it looks: the capsule already records
per-amendment evidence under the authority lock, so an `applied-at` beside each
hash would be an append-only chronology acquired at the moment that is actually
being recorded. It is a deliberate schema decision with its own review, not a
side effect of adding a query flag.

**What `--at <amendment>` renders, and what it refuses to.** The promise half
only. The contract artifacts are a stored snapshot the splice edits in place, so
there is no earlier version of them to read — and showing today's entries under
an earlier promise set would attribute present facts to a past state. That is
the more tempting mistake, because the output would look complete. The omission
is stated in the view rather than left to look like an empty contract.

It also refuses forwards. A sequence above the applied marker is not an earlier
state but a proposal — what the feature *would* promise once that record is
applied, which is the apply ceremony's question and carries an approval with it.
A read-only view answering both in the same shape makes them indistinguishable
to the reader.

**Stage 5 — artifact-specific projection, only if it proves worth it.**
The objection to projecting contract artifacts is that amendments carry prose
and refs, not replayable patches. OpenSpec suggests a way around it: store
snapshots rather than patches, so replay is "take the latest version of each
entry".

That is NOT universally tractable, and an earlier draft overclaimed it.
"Latest version of each entry" reconstructs a flat keyed map only if the
artifact also supplies: stable entry identities independent of labels and
positions; tombstones for deletion; ordering keys with deterministic
tie-breaking; container-level metadata and schema-version evolution;
cross-entry invariants; and an owner for the comments, headings and prose that
belong to no entry at all.

`capabilities.yaml` probably satisfies that — it is a keyed list of operations.
Surfaces may be ordered or nested graphs. Infrastructure Markdown may carry
argument structure whose meaning does not decompose into independent fragments;
forcing it into entries either demands a typed semantic IR plus a deterministic
renderer — a real redesign — or silently loses document-level semantics.

So: per-artifact rules, not one rule. Per-entry snapshots where an artifact has
genuine stable entries; whole canonical snapshots or checkpoints where it does
not. Pilot ONE naturally keyed artifact, make its current file generated, and
verify it against the reducer before considering any other. OpenSpec
demonstrates replayable requirement replacement; it does not demonstrate that
every document decomposes safely into requirements.

Stages 1–3 deliver most of the requirement and touch none of the codegen
pipeline. The expense is concentrated in Stage 5.

## Migration

The new field is additive on disk, but "no data migration" is too strong a
compatibility claim and an earlier draft made it.

**A missing mode cannot globally mean `retire`.** Most historical amendments
carry no `supersedes_intents:` at all — they are splices, and plainly not
retirements. The rule applies only to a historical record that DOES carry
`supersedes_intents:` and no evolution mode.

**And even there, `retire` is the wrong label.** Executing it as retirement is
operationally safe and faithful to what the old resolver actually did. It is not
necessarily faithful to what the author MEANT: retirement was the only available
spelling, so an author who intended a revision had no way to say so. Silently
relabelling that as a known retirement rewrites history to match a vocabulary
that did not exist when it was written.

So model it as a third value, `legacy_supersession`: the same conservative
authority effect as retirement, without the false claim about intent. Then
either require an explicit migration classification for legacy records before a
historical query reports a semantic mode, or let current authority projection
treat legacy supersession as retirement while `--at` reports the mode as
legacy/unknown.

**New records require an explicit mode.** Only the legacy reader gets the
compatibility rule.

Checked: all four existing governance amendments in this repo are genuine
retirements and classify without ambiguity. That is evidence about this
checkout, not a general compatibility guarantee.

## Risks and open questions

**`extend` vs `revise` is a judgement, and most changes are `revise`.** The
motivating example proves it: "email only" to "email plus SMS" adds behaviour
AND removes an exclusivity constraint, so it is not monotone. If nearly
everything is `revise`, has anything improved?

Yes, and the reason is worth stating precisely: the pain today is not the
approval. It is that the only verb available *orphans everything the promise
justified*, forcing enumeration of every affected entry and producing a record
no command can apply. `revise` keeps the lineage alive, so entries stay
ATTRIBUTED, the mandatory enumeration disappears, and the transition has an
applier. The human still approves a promise delta — which is the thing a human should be
approving, instead of an inventory of collateral.

**Mode-checking is only half decidable, and the doc now says so.** Referential
orphaning is a fact about the graph. Semantic justification drift is not
decidable at all, and it is the failure the modes most need to catch: an
`extend` cannot structurally kill its own lineage, so a mis-labelled one is
invisible to every structural check. The residual risk is that the modes read as
guarantees. Mitigation is wording — in the schema, in the refine prompt, and in
whatever the human is shown before attesting.

**Bias warning for whoever evaluates this.** This repository is poor evidence
about frequency. Its recent history is teardown work, which is systematically
the one disposition (`removed`) that never needs the combined shape — 12
amendments, 4 supersessions, 0 combined. A product evolving features rather than
deleting them lives in `revise` and `extend`, which this repo has never had to
express. Do not read the zero as reassurance.

## Working note: what counts as evidence

Carried from the applied-authority remediation's ground rule 6, and extended by
what this stage cost:

- A regression is evidence only when its fixture demonstrably reaches the guard,
  and when it asserts the IDENTITY of that guard rather than that something
  failed.
- **A mutation result is evidence only after the mutant BUILDS and the observed
  failure identifies the intended invariant.** A mutation that does not compile
  produces no failing tests and reads as a passing guard; a mutation aimed at
  the wrong mechanism reads as a dead guard when the real binding sits
  elsewhere. Both happened here.

Choosing the mutation is as easy to get wrong as choosing the assertion.

## What this actually is

Not merely a complete verb set. A complete **transition contract**:

- stable identity for the promise,
- a versioned proposition rather than frozen prose reinterpreted forever,
- explicit semantic attestation by a human,
- mechanically checked structural effects,
- and durable evidence of which transition was authorised and applied.

The verb set is the visible part. The contract is the thing that makes an
evolving spec trustworthy, and it is what the current model is missing.

**The invariant that keeps both halves honest:** concision may come from
generated inventories, defaults and closure assertions — but the authority
evidence must stay bound to an immutable, exact subject set and an exact promise
delta.
