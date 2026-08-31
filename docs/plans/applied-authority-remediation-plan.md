# Applied-Authority Remediation Plan

Origin: the dogfooding closure of `parlay-tool/studio-cli-hooks` (2026-08-31, recorded in
[spec-only-apply-and-compaction-gaps.md](spec-only-apply-and-compaction-gaps.md)), extended by a
peer review with Codex the same day. That review confirmed both recorded gaps, corrected the
generality claim on each, and — while checking a constraint Codex raised about whole-tail
accounting — surfaced a **governance bypass** that outranks both.

Six work packages (WP0–WP5). Every anchor below was verified against this checkout, not inferred.
The bypass chain was the one claim that began as inference rather than execution. **WP0 settled it
(2026-08-31): it reproduces on both save paths.** See Verification status for the observed numbers.

## The defect class

All four findings are the same mistake wearing different clothes: **authority is inferred rather
than proven.**

- `buildBaseline` infers "applied" from the ledger maximum.
- `apply-governance` and the splice path route by frontmatter *shape* (`affects:` present or not).
- The affects resolver infers that history must keep resolving against current artifacts.
- Partial save infers feature membership solely from generated-file provenance.

Each inference is locally reasonable and each is false at an edge the other mechanisms can reach.
The fixes below replace every one of them with a proof supplied by the caller.

## Ground rules

1. **Source-first dogfooding** (per CLAUDE.md): embedded skills/schemas → `make build` →
   `./parlay upgrade` → `make verify-skills`; DIGEST regenerated after any schema edit.
2. **Test loop** green before every commit.
3. **WP0 lands first and lands red.** The bypass is inferred from five separate mechanisms. No
   fix is written until an executable regression demonstrates it.
4. **Authority is never classified by frontmatter shape.** Not by `affects:` presence, not by
   `supersedes_intents:` presence. Only by workflow proof. WP1 onward depend on this.
5. **No partial application states.** Where a record cannot be fully applied by one proven path,
   refuse. A half-applied amendment is the ambiguous authority the ledger exists to prevent.
6. **A regression must discriminate.** Every test here must (a) establish its precondition — prove
   the fixture actually reaches the guard before asserting on it; (b) assert the IDENTITY of the
   intended guard or transition, not merely that an error was non-nil; and (c) where practical, be
   mutation-tested by reverting the production condition it claims to pin, and observed to fail.

   This is not advice. Seven tests on this branch passed while reaching nothing they named: an
   archive test whose records the loader could not see; an ownership test whose snapshot keys never
   matched; two where an earlier check fired first; a fixture generator journalling a state the real
   selector cannot produce; a conflict test that never invoked the command it was named for; and a
   concurrency test whose mutation landed before the code under test ran. Four were caught in
   review, three only because an assertion on message text happened to fail.

   **Green is an outcome; discriminating failure is the evidence.**

---

## WP0 — Executable proof of the bypass · size S · **lands failing, first**

The chain, every link verified and — as of WP0 — reproduced end to end:

| Link | Anchor |
|---|---|
| Refine preflight permits an existing unapplied tail | `.claude/skills/parlay-refine/SKILL.md:329-331` |
| Refine authors its amendment at the *next* sequence, so a pending governance record sits below it, inside the tail | refine step 3.5 |
| `dirty_set` cannot see a pure governance record — it has no `affects:` | `check_amendments.go:246` |
| Emitted-feature scoping admits the feature | `save_build_state.go:262-263` |
| `buildBaseline` hashes every visible amendment and sets `LastAppliedAmendment` to the maximum, with no journal or tail input | `baseline.go:290-304` |
| Stage 1 writes that baseline before stage 3 runs | `save_build_state.go:246` vs `:357` |

The regression: baseline at N; pending **pure governance** amendment N+1; refine journal for an
`affects:` amendment N+2; an emitted file for the feature; run partial save. Assert the save is
**refused**, that `last-applied-amendment` and `sources.amendments` are unchanged, and that N+1's
promises remain in force.

Companion tests, all of which must stay green: a clean one-amendment refine advances only its own
record; a direct full save does not sweep a pending governance record; `apply-governance` still
advances it only with `--confirm`.

**Why this is a safety defect and not a workflow gap.** The skill is explicit that a governance
amendment must go through `apply-governance --confirm`, "because it, not the amendment filename,
is what you are approving" (`SKILL.md:201-213`). The bypass withdraws promises the user was never
shown. Worse, the swept-in record becomes **both sequence-applied and hash-recorded**, so it would
satisfy the trusted-applied test WP3 introduces. The bug does not merely bypass confirmation — it
manufactures the evidence that confirmation occurred.

The guard that should catch it is structurally blind: step 4 expects `dirty_set` to be "exactly
the amendment you just added" (`SKILL.md:254-260`), and a governance amendment contributes nothing
to `dirty_set`. The one class requiring explicit confirmation is the one class invisible to the
check that would notice it was swept in.

## WP1 — Fail-closed exact-tail guard · size M · safety patch

Any partial save for a feature with an unapplied tail must load a matching refine journal and
prove the **entire** tail is what this run is authorized to advance.

With today's single-amendment journal that means: exactly one pending amendment; its `seq` equals
the journal's amendment; the journal reached splice-applied / rebuilt / tested. Any other pending
record — above or below, contract or governance — **refuses and names it**.

- Pure governance (`supersedes_intents:` / `retires_feature:` with no `affects:`) must never
  advance through `save-build-state`. Only `apply-governance` may move it.
- A **combined** record (both `affects:` and `supersedes_intents:`) is **refused outright** at this
  stage. See the design target below for why it cannot be half-applied.
- All new guards preflight **before any baseline write**. Stage 1 currently writes per-feature
  baselines (`save_build_state.go:246`) ahead of stage 3 checks (`:357`), so a late-discovered
  invalid claim would already have advanced the tail. WP0 proves this is not theoretical: the run
  that must be refused currently *succeeds*, consuming the emitted manifest and creating
  project-level state before anything notices — destroying the retry's own inputs.
- **WP1 closes both safety paths, not just the partial one.** The exact-tail journal logic is
  partial-specific, but a minimal full-save guard -- refuse when a pending governance record exists
  -- ships in the same package. Deferring the full path to WP2 would leave WP1 with a knowingly red
  suite, contradicting ground rule 3, and leave the same authority primitive reachable by direct
  internal use.

## WP2 — `buildBaseline` becomes observational · size M–L · root cause

`buildBaseline` must stop inferring applied authority from the ledger maximum
(`baseline.go:290-304`, whose own comment justifies the stamp with "save-build-state runs after a
green build, at which point the ledger is by definition fully applied" — an assumption that is
false for every partial save).

- Take an explicit trusted-applied set (or through-value) from the caller.
- **Preserve the prior applied marker by default.**
- Store amendment hashes **only for records actually proven applied.** Otherwise a pending
  record's hash silently becomes the matching-hash evidence WP3 trusts.
- Full save must likewise preserve applied state or accept an explicit proof set. Green tests do
  not mean "everything visible is applied."
- `apply-governance` remains the sole authority source for governance records; refine/save supplies
  authority only for the journal-accounted splice amendment.

## WP3 — Applied-history-aware resolution · size M · unblocks retirement

The retirement deadlock: `resolveAmendmentRef` runs over the **whole** ledger
(`check_amendments.go:239`, `:498`), so a historical ref requires its contract entry to exist
forever, while `feature-retirement-has-output` (`:1131-1157`) requires exactly those artifacts gone
— the gate's delete list at `:1096-1120` covers `surface.yaml`, `capabilities.yaml`,
`infrastructure.md`, `domain-model.yaml`, page/layout files, plus the five build artifacts.

Move the fatal case to the unapplied tail. Four cases to pin:

1. Pending / untrusted `operation`, `surface`, `infrastructure` refs stay **fatal** when unresolved.
2. Applied refs become historical **only** when `seq <= lastApplied` **AND** the amendment's
   current-or-archived bytes match `sources.amendments`.
3. Forged tail, missing hash, or hash mismatch stays **strict**.
4. A direct contract deletion still reports whole-file drift — with attribution now file-level.

**Scope.** Only three of four ref kinds deadlock. `domain` refs are root-scoped — "the ref's feature
part records who is asking" (`check_amendments.go:541-543`) — so they survive their own feature's
retirement and keep their existing behavior.

**The trade, stated rather than discovered.** `capabilities.yaml`, `infrastructure.md` and
`surface.yaml` are whole-file *advisory* hashes with no per-operation granularity
(`baseline.go:77-88`). So drift reports "capabilities.yaml changed", not "operation X was deleted".
This is a loss of **attribution granularity, not of detection**, and `source-signatures:` remains
the hard emission gate (generate-code step 11.6). Write it into the schema and the tests.

**Note.** `all_affects` has no consumer anywhere in the tree — it is documented as available "for
audit" (`amendment.schema.md:196`). Changing its semantics is therefore cheap, but should still be
explicit: represent unresolved historical refs as declarations rather than silently omitting them.

## WP4 — Output-less feature-scoped blessing · size M

A spec-only feature can be created and baselined by a full save but never re-blessed by a partial
one, because partial save's only membership proof is generated-file provenance. There is no
sanctioned path from "contract amendment authored and spliced" to "recorded applied".

Not `apply-governance --spliced-contract`: resolving `affects:` proves the contract entry *exists*,
never that the delta was folded into it. That command shape overclaims.

Instead, a narrow path in `save-build-state`:

- Interface: `--partial --outputless-feature @x --confirm-outputless`. Ordinary emitting features
  stay **exactly** on today's manifest-inferred path.
- Two **disjoint** sets internally: `emittedFeatures` keeps controlling provenance and the project
  baseline's `emitted:` reporting, unchanged; `baselineFeatures` is `emittedFeatures` plus the one
  confirmed output-less feature and controls **only** stage-1 per-feature baselining. The
  output-less slug is never inserted into `emittedFeatures`.
- Preconditions, all preflighted before any write: slug resolves and matches the active refine
  journal; journal reached splice-applied / rebuilt / tested; journal's amendment is the whole
  pending tail (WP1's rule); an explicitly present **empty** manifest, not a missing one;
  `generatedFilesOwnedBy` returns no failures and no prior owned files.
- **`generatedFilesOwnedBy` is necessary but not sufficient.** It reads the *prior* snapshot
  (`check_amendments.go:1182-1207`) and returns empty in three distinct cases: no snapshot at all,
  output first introduced by this very amendment, and codegen silently failing on a formerly
  output-less feature. It proves previous output vanished — never that no output was owed.
- Therefore add an output-obligation check over whatever plan inventory is authoritative: if it
  predicts creates/modifies, **refuse** the empty emission. Where no mechanical inventory can prove
  absence, `--confirm-outputless` is an honest human assertion — and the refine skill must actually
  ask before passing it. The result names the assertion and the feature, so manifest, journal and
  command output form an audit trail.

## WP5 — Compaction as an authority-projection operation · size L

No `compact` command exists, though the schema legitimizes the compacted shape
(`amendment.schema.md:67`) and `LoadFeatureAmendments` already skips the subdirectory
(`parser/amendment.go:114`). After WP3, compaction is **ledger hygiene, not the deadlock escape.**

It is not a file mover. Because the loader ignores `archive/`, archiving changes *authority*:
archived `supersedes_intents` claims disappear and founding intents can reactivate; an active
amendment naming an archived slug in `supersedes:` triggers `amendment-supersedes-unknown`
(`check_amendments.go:206`).

**Live evidence, and a near miss.** `studio-cli-hooks` archived 001 and 002 and kept 003 active.
Archived 002 names 001 in `supersedes:`. Both went to `archive/` together, so the edge vanished
cleanly and the feature reports `ready: true` with zero issues. It is sound **by construction, not
by any check** — had a threshold fallen between them, 002 would have stayed active naming an
amendment no longer in the ledger. A naive `compact` archiving at `seq <= last-applied` reaches
exactly that split whenever the superseding record sits above the threshold.

Requirements:

- Prove the **effective authority projection is identical before and after**: active / retired /
  pending intents, retirement head, `supersedes_intents` heads, amendment supersession validity.
- Refuse archival of records still needed by the active graph, unless a retained compaction-head
  or refounding record carries them forward.
- Leave `sources.amendments` hashes **untouched** — the archive-aware integrity check
  (`baseline.go:556-578`) verifies archived files byte-identical against exactly those hashes.
- Verify after the move rather than trusting it. `seq <= lastApplied` plus hash verification is
  necessary but **not** sufficient.

---

## Design target — the two-proof application transaction

`affects:` is required *unless* `supersedes_intents:` is non-empty, and naming a
`supersedes_intents:` entry means `affects:` "may **then** be empty" (`amendment.schema.md:50,55`).
May, not must — a **combined** amendment is legal. And `apply-governance` refuses any amendment
with a non-empty `affects:` unconditionally (`apply_governance.go:106`).

So a combined record is refused by the only command allowed to move governance authority, while the
splice path that could handle its `affects:` half would advance its intent supersession with no
promise list ever shown. This is not a fourth edge case — **it disproves shape-based routing as an
authority model**, and in both directions at once: "has `affects:` ⇒ safe to splice" is wrong, and
`apply-governance`'s "has `affects:` ⇒ someone owes a splice" is equally wrong for this record.

The end state is **one coordinated application transaction carrying two independent proofs**:
splice completion for `affects:`, and explicit promise-list confirmation for `supersedes_intents:` /
retirement. Neither proof substitutes for the other, and the baseline advances only once both are
present. WP1 refuses the combined case in the interim rather than inventing a partial application
state; that transaction eventually subsumes WP1 and WP4.

## Order and rationale

**WP0 → WP1 → WP2 → WP3 → WP4 → WP5.**

WP0 first because the bypass is the one inferred claim. WP1 next because it is a safety defect that
manufactures authority evidence, and it must land before WP3 gives that evidence more weight. WP2
removes the inference WP1 guards against. WP3 is self-contained and unblocks retirement. WP4 needs
WP1's whole-tail rule to be safe. WP5 is hygiene once WP3 has removed the deadlock.

## Verification status

Verified against this checkout: every file:line anchor above; the retirement gate's delete list;
the four ref-kind resolvers; `generatedFilesOwnedBy`'s three empty cases; `all_affects` having no
consumer; the `studio-cli-hooks` ledger reporting `ready: true`; the schema's combined-amendment
allowance.

### Before remediation — the WP0 exploit (2026-08-31)

The bypass is no longer inferred. `applied_authority_test.go`
reproduces it on both paths:

- **Partial save (reachable).** Feature at last-applied 1, pending pure-governance 002, pending
  splice 003, a refine journal completed through `tested` naming **003 only**, and an emitted
  manifest. The save *succeeds*, the marker moves **1 -> 3**, 002's whole-file hash is written into
  `sources.amendments`, the manifest is consumed, and project-level state is created. The swept
  record then satisfies `seq <= lastApplied` AND stored-hash-matches -- the exact trusted-applied
  predicate WP3 relies on. The bug manufactures the evidence of its own authority.
- **Full save (latent).** Same stamp, marker **1 -> 2**. `buildBaseline`'s ledger-maximum is a
  primitive of both saves; the loop gate merely blocks this path earlier in practice.
- **Over-refusal guard (green, must stay green).** A lone pending splice amendment, journalled as
  this run's own, still saves and still advances to exactly itself.

### After remediation (2026-08-31)

All six work packages are implemented and reviewed. The results above are the **pre-fix** state,
retained as the evidence that motivated the work; the current state is:

- **The bypass is closed on both paths.** WP1's read-only preflight refuses before any write; WP2
  removed the ledger-maximum inference from `buildBaseline`, which is now preserve-by-default.
- **A second forging primitive was found and closed during set-level review.**
  `apply-governance`'s `advanceAppliedMarker` had survived WP2 untouched: it recomputed hashes for
  all applied history on every run using advisory hashing that skipped failures silently, so it
  could both re-bless a mutated applied record and advance past one with no evidence. WP3 had by
  then taught the tool to trust exactly those hashes. Both writers now go through one
  `applyAuthorityCapsule`.
- **WP3 validated on real data.** With `studio-cli-hooks` restored to its pre-compaction state —
  contract artifacts disposed, four historical refs unresolvable — `check-amendments` reports
  `ready: true`, zero issues, `all_affects: 4`, `dirty_set: []`. The ledger fingerprint is
  byte-identical before and after the reversible check. Compaction was never the fix.
- **WP4 is operational, not merely implemented.** The path first shipped as CLI flags no deployed
  guidance named, which left the original deadlock intact behind working code. Refine step 9 now
  drives it, asking a question branched on whether a mechanical inventory exists.
- **WP5 compaction is crash-recoverable.** The journal records amendment filenames only — never
  paths — is validated against the feature being recovered, and recovery derives its own paths.
  Restoring locations is not recovery: the restored ledger must reproduce the recorded projection
  *and* every record must still verify against its stored hash before the journal is discharged.

**Explicitly retained limitations.** These are declared follow-up scope, not solved:

- `fullGreenBuildInference` remains a workflow assertion, not a machine proof. `save-build-state`
  is directly invocable, so hoisting it to the caller made it explicit and non-inheritable, nothing
  more. The constant is the grep target for the work that replaces it.
- **Valid combined amendments remain refused.** A record carrying both `affects:` and
  `supersedes_intents:` is legal and has no applier; the two-proof transaction that would apply one
  is named in this plan and not built.
