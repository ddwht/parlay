# Surface Criteria Vacancy — Fix Plan (benchmark finding #7)

**Status:** implemented, 2026-08-25; both open decisions closed the same day
(see *Open decisions*). Written and peer-reviewed the same day; the
review located a prerequisite defect (WS 0) the first draft not only missed but
contradicted. Landed as `3d6bab3` (WS 0), `784b4a6` (WS A+B), `978dd64` (WS C),
`3e25a05` (cross-kind), `0d0d23f` (WS D) here, and `506050d` in
`parlay-benchmark` (WS E).

One item in WS D has no automated form: B6's degradation — build-feature
omitting a criteria-less suite and reporting it — is skill prose executed by an
agent, so it is asserted by the graded batch's metrics rather than by a Go test.
The gate half of it (vacancy warns without blocking) is covered.
**Source:** `parlay-benchmark/docs/findings.md` §7 — "A surface fragment with no
`verify:` is a silent hole that surfaces as a test defect", observed on
`feature/phase-gates` (`ba01878`) in the 2026-08-25 Phase-0 batch.
**Thesis:** the finding's suggested fix (a `surface-fragment-no-criteria`
warning) is necessary but treats the symptom's *visibility*, not its cause. The
cause is a contradiction between two rules parlay ships: `create-artifacts`
routes an intent's criteria to the operation whenever an operation covers that
intent, and `build-feature` derives presentation cases from *fragment*
criteria. A feature whose every intent produces an operation is therefore
**specified to have zero presentation criteria** — which is exactly what the
candidate run produced. Underneath both sits a third defect: criterion coverage
is tracked per contract *entry*, not per criterion, so the "cases come 1:1 from
`verify:` entries" rule the pipeline is built on has never been enforceable.

## What the run actually shows

The finding reports the phase-gates arm wrote 21 presentation cases with 0
criteria and drew `testcases-case-criterion-missing` ×21, against 12 criteria /
1 warning in the v0.5.0 arm. Re-reading both arms' artifacts out of
`runs/2026-08-25_1241_phase-gates/*/workspace.tar.gz` sharpens the diagnosis:

| | phase-gates | v0.5.0 |
|---|---|---|
| surface fragments | 4 | 5 |
| fragments carrying `verify:` | 0 | 2 |
| capability operations | 4 | 4 |
| operations carrying `verify:` | 4 | 4 |
| distinct source intents | 3 | 3 |
| intents covered by an operation | **3 of 3** | **3 of 3** |

Both arms have `browse-customers`, `create-a-customer` and `update-a-customer`,
and in both arms every one of the three is covered by an operation
(`customer.list`/`customer.get`, `customer.create`, `customer.update`).

Read against the shipped rule — `create-artifacts.skill.md:111`, "**Populate
`verify:` on each fragment** … *but only for intents no capabilities operation
covers*; an intent that produces an operation carries its criteria there
instead" — the phase-gates arm is **literally correct**. Zero fragments should
carry `verify:`. The v0.5.0 arm produced 12 criteria by *breaking* the rule on
two of its five fragments.

So this is not a branch regression and not model variance in the direction the
raw numbers suggest. Compliance with the documented routing rule is what
produces the 21 warnings. The branch is exonerated; the rule is the defect.

### The contradiction, stated plainly

- `create-artifacts.skill.md:111` — a fragment carries criteria **only** for
  intents no operation covers.
- `build-feature.skill.md:268` — component (presentation) suites derive
  assertions from "the fragment's `verify:` in surface.yaml", with **no**
  fallback since v0.3.
- `surface.schema.md:58` restates the exclusion; `migrate-verify`
  (`core/internal/commands/migrate_verify.go:99-110`) implements it —
  operations first, then "only intents no operation covered fall through to
  the surface".

Any feature with a backend satisfies the first rule by emptying the input to
the second. The two rules cannot both be followed by a full-stack feature.

### Why nothing caught it

`verify-criterion-uncovered` (`validate_testcases_v2.go:417-431`) fires when a
contract entry *carrying* `verify:` has no case discharging it. A fragment
carrying none demands nothing and so is coverage-complete by vacancy. Absence
of criteria is invisible; only their non-discharge is visible. The gate added
by `feature/phase-gates` aggregates `checkBuildFeatureReadiness`
(`core/internal/commands/check_readiness.go:221`), which checks a fragment's
`source:`, `page:` and `region:` — and not its criteria.

## Workstream 0 — Criterion identity (prerequisite)

**This must land first.** WS A increases the number of `verify:` bullets per
contract entry, and the validator currently cannot tell which of an entry's
bullets a case discharges.

Coverage is tracked per **entry**, not per criterion:

- `validate.go:1169` — `if len(op.Verify) > 0 { in.Criteria = append(in.Criteria, ref) }`.
  An operation with five bullets contributes **one** ref.
- `validate.go:1183` — the same for a fragment: one
  `@feature/fragment:<name>` ref regardless of bullet count.
- `validate_testcases_v2.go:169` — a case contributes `crit.Ref` alone, and
  `criteriaCovered[ref] = true` marks the whole entry discharged.

So one case citing a five-bullet operation discharges all five. Two shipped
claims are false as a result: `build-feature.skill.md:340`'s "cases come 1:1
from `verify:` entries (coverage)" is unenforceable, and
`testcases.schema.md:137`'s promise that `criterion.text` "pins the criterion's
wording so a later edit to the contract shows up as drift here" is untrue —
`caseCriterion.Text` (`validate_testcases_v2.go:124`) is decoded and never
read. The field is inert.

**W0.1 — make `criterion.text` load-bearing.** Criterion identity becomes the
pair `(canonical ref, canonical bullet text)`. No change to `criterion.ref`
syntax and no new field: `text` already exists and `build-feature` already
populates it.

**W0.2 — `Criteria` becomes a richer value.** `TestcasesV2Input.Criteria`
changes from `[]string` to a slice of `{Ref, Text}`, populated per bullet
rather than per entry in `testcasesCoverageInputs`.

**W0.3 — three distinct failures**, replacing today's single walker:

| condition | meaning |
|---|---|
| a case cites a ref no contract entry declares | miscitation |
| a case cites a known ref whose `text` matches none of that entry's current bullets | drift, or a fabricated criterion |
| a `(ref, text)` pair no case discharges | the real uncovered criterion |

**W0.4 — normalization stays narrow.** Trim surrounding whitespace, normalize
line endings. Do **not** lowercase, collapse internal whitespace, or strip
punctuation: those merge materially distinct claims, and the whole point of
text identity is that a wording edit invalidates the case that cited the old
wording.

**W0.5 — duplicate bullets are a defect, not an identity problem.** Two
identical bullets on one entry are indistinguishable under text identity and
carry no distinct meaning. Report them as duplicate criteria rather than
inventing an index to tell them apart.

**W0.6 — exemption semantics move with coverage**, and this is a
`coverage-review.yaml` schema evolution even though `criterion.ref` syntax is
unchanged. `ExemptCriteria` (`validate.go:1194`) exempts a whole ref today.
Under bullet-level coverage that is a large escape hatch. Concretely:

- Add an optional `criterion_text:` beside `item:` in an `exemptions[]` entry
  (today `{suite, item, reason}`, `parser/coverage_review.go:38`). Present ⇒
  the exemption discharges exactly the `(item, criterion_text)` pair.
- Absent ⇒ entry-wide, which is how every existing exemption is read. No
  migration required, and no existing review file changes meaning.
- The non-interactive `--exempt <suite>:<item>=<reason>` form
  (`coverage-review.schema.md:69`) stays entry-wide: its grammar splits on the
  first `=` and the first `:`, so threading a third free-text field through it
  is not worth the ambiguity. Bullet-specific exemptions are written by the
  review flow into the file. The schema and the review UI must both **say**
  that `--exempt` is entry-wide and therefore broader than an interactive
  bullet-specific exemption — an automation author who assumes the two are
  equivalent in precision writes a wider exemption than they meant to.
- The review flow emits `criterion_text:` for newly recorded exemptions.

*Considered and rejected: indexed or hashed bullet IDs in the ref.* They keep a
stable identity across wording edits — but the stated contract is that a
wording edit **should** invalidate the citing case, so text identity is the one
aligned with the intent. Their other advantage, precise exemptions, W0.6
delivers without new syntax.

## Workstream A — Fix the routing rule (the cause)

Route criteria by **what is asserted**, not by whether an operation exists.
Operation coverage of the intent stops being the routing input entirely.

**A0. Route atomic claims, not bullets.** "What the bullet asserts" is too
coarse: real intent bullets routinely mix stimulus, backend result and visible
evidence in one sentence, and routing whole bullets by their dominant flavour
invites either arbitrary placement or wholesale duplication. The decision
procedure, to be written into the skill:

1. Extract the independently testable claims from the bullet.
2. A claim about user-observable presentation or output, attributable to a
   specific fragment, goes on that fragment.
3. A claim about transport-independent input validation, state change, output
   shape or error contract goes on the operation.
4. A sentence carrying both is **rewritten into separate criteria** — one per
   destination. Never relocate the same sentence verbatim to both places.

**A0a. "Visible" does not imply a surface fragment.** A CLI or TUI feature with
no surface artifact has observable output and no fragment to carry it. In that
case the output claims stay on the operation, and the presence walker (WS B)
must not report a feature that has no surface artifact at all.

**A1. `core/internal/embedded/skills/create-artifacts.skill.md:111`** — drop
the "but only for intents no capabilities operation covers" exclusion. Replace
with A0's procedure, stating explicitly that an intent producing an operation
still contributes its presentation claims to every fragment that sources it.

**A2. `core/internal/embedded/schemas/surface.schema.md:58`** — the `Verify`
row currently reads "carried by the fragment when no capabilities operation
covers that intent". Restate as: carries the presentation claims attributable
to this fragment, independent of operation coverage.

**A3. `core/internal/embedded/schemas/capabilities.schema.md:54`** — mirror
the complement: `operations[].verify` carries transport-independent contract
claims, not display-shaped ones.

**A4. `core/internal/embedded/skills/build-feature.skill.md:268`** — the
derivation rule was right and does not change. Add the hard stop (B6) and,
under WS 0, correct line 340's coverage claim to match what the validator now
actually enforces.

**A5. Deploy** per CLAUDE.md's dogfooding rule: `make sync-skills`, then
`make verify-skills`. `DIGEST.md` and `digests/*.digest.md` regenerate from the
schemas, so A2/A3 propagate without a second edit.

## Workstream B — Make the vacancy visible

A new walker in the `agent` package, called at the designer→build boundary,
alongside the existing coverage walkers rather than inside them (the testcases
walkers need a `testcases.yaml`; this must fire before one exists).

**B1. `core/internal/agent/validate_criteria_presence.go`** —

```go
type CriteriaPresenceInput struct {
    Feature    string
    HasSurface bool
    Fragments  []parser.SurfaceFragment
    Operations []parser.CapabilityOperation
}
func ValidateCriteriaPresence(mode ValidationMode, in CriteriaPresenceInput) []ValidationOutcome
```

| code | fires when | severity |
|---|---|---|
| `surface-fragment-no-criteria` | a fragment carries no `verify:` | warning, both modes |
| `capability-operation-no-criteria` | an operation carries no `verify:` | warning, both modes |
| `feature-surface-no-criteria` | the feature has ≥1 fragment and **none** carries `verify:` | warning, both modes |

The aggregate is named for what it detects: all fragments vacant. The
per-fragment warnings locate *partial* vacancy; the aggregate is the shape the
benchmark run actually had.

*The first draft called this `feature-contract-no-criteria` and defined it as
"zero `verify:` anywhere in the feature". That is mis-targeted: the observed
run had four operations all carrying criteria, so it would not have fired on
the very case it was written for.*

**B2. Severity — all three are warnings in the first release.** The first draft
graded the aggregate an error in `ModeBuild`. That is wrong for now on the
policy the neighbouring codes in `ruleSeverityTable`
(`core/internal/agent/validation_mode.go:44`) already state: every artifact in
existence predates the field. It is worse than the usual case here, because
readiness converts a `ModeBuild` error into a **gate blocker**
(`gate.go:139`), so old projects would be blocked before having any migration
opportunity.

**B3. Wire into readiness** — `checkBuildFeatureReadiness`
(`check_readiness.go:221`) gathers the fragments already; add the
`capabilities.yaml` load and map the outcomes into `readinessIssue`s with
fixes. The gate inherits this for free: `gateBuild` (`gate.go:139`) copies
readiness errors to blockers and warnings to warnings, stripping only
`unapplied-amendments`. No gate edit.

**B4. Wire into `validate` for both artifact types** — `validate --type
surface` runs `agent.ValidateSurface(path, content)`, which cannot see
`capabilities.yaml`. Add a gatherer in `core/internal/commands/validate.go`
shaped like `testcasesCoverageInputs` (`validate.go:1137`), and run it for
`--type surface` **and** `--type capabilities`. Wiring only one means the same
cross-artifact condition appears or vanishes depending on which file the user
happened to validate.

**B5. Schema rows** — document the three codes in `surface.schema.md` and
`capabilities.schema.md` diagnostics tables with the `(warning)` marker.
`severity_doc_test.go` enforces that an unmarked row reads as blocking, so an
omitted marker fails the build rather than misleading a reader.

**B6. `build-feature` degrades, it does not stop.** A suite whose source
contract entry carries no criteria is **omitted**, with the omission reported;
the rest of the build proceeds. If omitting leaves a surface feature with no
meaningful presentation build at all, report *that* explicitly as a degraded
outcome. What must never happen is the 21 vacuous cases.

This is the transition policy, and it is a deliberate choice between two
coherent options rather than a split of the difference:

- **(A) Vacancy blocks this release.** Make the readiness aggregate an error
  and defend the migration break honestly.
- **(B) Compatibility wins this release.** Warn everywhere, omit the affected
  suites, report the degradation, graduate to a blocker later.

**Take B.** A had the merit of a single mechanical stop, but blocks every
legacy all-vacant surface at the gate before its owner has any way forward.

The first draft's combination — warning severity *plus* an unconditional
build-feature hard stop — was neither. Every legacy all-vacant surface would
pass the gate, enter build-feature, and then fail to build: operationally the
same breaking change as option A, only later and less mechanically reported,
which made the severity concession cosmetic. A version or migration marker
letting newly authored artifacts hard-fail while legacy unmarked ones warn
would make warn-plus-stop coherent; absent that discriminator it is not, and
adding one is more machinery than this transition needs.

**B7. No exemption machinery yet.** State the default invariant — a generated
fragment or operation is expected to carry criteria — warn on exceptions, and
find out from real projects' artifacts whether structural or UI-only-acceptance
entries make the warning noisy. (Not from feedback telemetry: it is opt-in, off
by default, local rather than phoned home, and counts outcomes produced rather
than surfaced.) The absence of that machinery is also why only the aggregate
blocks: with no way to record "this fragment is structural", a per-fragment
error would have no honest escape. If they do, the exemption belongs **upstream on the contract
entry** (an explicit `criteria_exempt: {reason: …}`), not in
`coverage-review.yaml`: that file lives under `.parlay/build/<feature>/`, so it
does not exist at the designer→build boundary where the vacancy is found, and
an entry with no `verify:` has no bullet-level criterion to name in
`ExemptCriteria` anyway. Coverage-review stays scoped to its own question —
"a criterion exists and no case discharges it" — not "this entry intentionally
declares none".

## Workstream C — Migration path for existing projects

`migrate-verify` cannot do WS A's split itself: it is a textual relocator and
cannot tell a presentation claim from a contract claim. Two changes keep it
honest rather than making it smart.

**C1. Report, don't guess** (`migrate_verify.go:60-140`) — report every surface
fragment left carrying no `verify:`, as its own line and its own summary count,
distinct from today's `unrouted` (which counts only intents matching
*nothing*). A project that ran migrate-verify under the old rule looks fully
migrated today; this is what tells it otherwise.

Compute projected occupancy **in memory** from the two splice passes' own
insert lists, not by re-reading the files afterwards: the write at
`migrate_verify.go:279` is guarded by `!migrateVerifyDryRun`, so under
`--dry-run` a re-read returns pre-splice state and the report would name
fragments the real run would have filled.

**C2. `--fragments` opt-in** — a flag that also splices an operation-covered
intent's bullets onto the fragments sourcing it, duplicating rather than
routing. Off by default: a contract-shaped bullet copied onto a fragment
demands a display case that cannot be written honestly. The flag is for a
project that would rather review duplicated criteria than author them from
scratch.

It must **merge** missing bullets into an entry that already carries some,
rather than skip that entry wholesale as both splice passes do today
(`migrate_verify.go:196`, `:227`). Wholesale-skip is what makes the current
migrator idempotent, so merging needs bullet-level de-duplication against the
existing list to keep that property — and under WS 0 a duplicated bullet is
itself a reportable defect.

## Workstream D — Tests

- `validate_criteria_presence_test.go` — the three codes against: a fragment
  with criteria; one without; the phase-gates shape, operations-only, which
  must produce 4 fragment warnings **and** the aggregate; a feature with no
  surface artifact at all (silence, per A0a); an authored unit (silence).
- WS 0 coverage tests — a five-bullet operation with one citing case must now
  report four uncovered criteria, not zero; a case whose `text` matches no
  current bullet reports drift; a case citing an unknown ref reports
  miscitation; duplicate identical bullets report as duplicates.
- `check_readiness` matrix — extend with the operations-only shape, asserting
  the warning set and that a pure infrastructure feature stays quiet.
- `gate_test.go` — a build-stage gate over the phase-gates artifacts reports
  the warnings and, per B2, still passes.
- Degradation test (B6) — an all-vacant surface feature builds, omits its
  presentation suites, reports the omission, and emits **zero** criterion-less
  cases.
- `migrate_verify_test.go` — the C1 report fires on an operations-only project
  and is correct under `--dry-run`; `--fragments` is idempotent on a second run
  (the existing `SecondRunIsNoOp` shape) via merge rather than skip.
- Cross-kind fixture (see below) — a testcase whose call step targets a
  canonical operation ref. None exists today.
- Golden inputs: the two extracted arm artifacts. phase-gates is the vacancy
  case, v0.5.0 the mixed case.

## Workstream E — Benchmark side (`parlay-benchmark`)

- **`scripts/l0-findings.py`** — add check `F7`, asserting the presence
  diagnostics fire on a locked corpus whose every intent is operation-covered.
  `expect_fixed_from` set to the release carrying WS 0+A+B. State in the check's
  `what` that it proves **detection only** — it cannot prove authoring
  improved, because L0 is scoped by its own docstring to "deterministic — one
  container, no model, seconds", and asserting that operation-covered intents
  now yield fragment criteria requires a model run.
- **`bench/metrics.py`** — record `verify:` entries per surface fragment and
  per operation. This is where the authoring-improvement assertion lives. The
  finding is right that this is the leading indicator and `criterion-missing`
  the lagging, derived one; the Phase-1 comparison must not read the latter as
  test quality on its own.
- **`docs/findings.md` §7** — append a note that the root cause was traced past
  the missing diagnostic to the routing rule, and past that to entry-level
  coverage, with the both-arms intent-coverage table above. Leave the original
  text intact; the finding was accurately reported and its "suggested fix" is
  now one workstream of three.

## Cross-kind criterion refs

The first draft rejected "let presentation cases cite operation criteria"
outright. That overclaims. An operation criterion can legitimately be observed
through presentation in an end-to-end case; a suite's `kind:` need not equal
its criterion's owner. What must be prohibited is narrower: an operation ref
used as a **substitute** for a missing display claim.

Nothing enforces this today — `validateSuiteCases` collects any ref without
inspecting suite kind, so a presentation case citing
`@feature/operation:customer.list` validates.

The rule to add: a presentation case may cite an operation ref only when it
actually exercises that operation and the criterion text is contract-shaped; it
may not discharge claims about rendered presence or content.

**Only the first clause is mechanizable.** Exact membership of `criterion.ref`
in the case's `stepTargets` proves the operation was *invoked*. It says nothing
about whether that operation's criterion is semantically suitable for a
presentation case — "contract-shaped, not a substitute for a missing display
claim" stays an **authoring rule** in the skill unless criteria carry
classification metadata, which this plan does not add. The diagnostic catches
the ref cited by a case that never calls the operation; a reviewer catches the
rest.

**Not** via `exercises:`. That looked like the cheap check and does not work:
the vacuity walker (`validate_testcases_v2.go:239-256`) builds `missing` but
gates the emit on `!touched`, so it fires only when *no* step targets *any*
declared exercise. Requiring the operation in `exercises:` therefore proves
nothing about the steps. Fixing vacuity to require every declared exercise is a
separate change with its own blast radius and is deliberately not in this plan.

A fixture is needed either way: nothing in the repo currently uses a canonical
operation ref as a step or exercise target — the existing cases target
component names like `submit-button` — so the mechanism is unproven as well as
unenforced.

## Sequencing

1. **WS 0 first.** WS A raises bullets per entry while the validator cannot
   tell which are covered; doing A first makes coverage *less* accurate.
2. WS A (rule + schemas) with WS B1–B2 (walker + severities) — land together.
   A without B leaves the hole invisible; B without A fires on every compliant
   spec and reads as a false positive.
3. WS B3–B7 (wiring, degradation, docs), then `make sync-skills && make verify-skills`.
4. WS C — before any existing project rebuilds, or their next build-feature
   inherits the vacancy silently.
5. WS D throughout; WS E once a binary carrying WS 0+A+B exists to point
   `expect_fixed_from` at.

## Resolved decisions

1. ~~When does `feature-surface-no-criteria` graduate to an error?~~
   **Decided 2026-08-25: it graduates now.** `ModeBuild` error, `ModeAuthoring`
   warning; the two per-entry codes stay warnings in both modes, so partial
   vacancy locates without blocking and total vacancy blocks.

   Two things settled it. The evidence the warning-first position was waiting
   for already existed: in the observed run the agent met this condition,
   diagnosed it, tried `migrate-verify`, found it could not help, and emitted
   four test files of criterion-less cases anyway — a warning stopped nothing.
   And the cost that justified waiting is absent here: the affected population
   is a few projects, all the owner's own, so blocking cannot strand anyone
   whose remedy has not reached them.

   The remedy also now exists, which it did not when the run was observed:
   `--fragments` seeds the criteria and the routing rule tells the designer to
   author them. Blocking means "stop and use the remedy" rather than "stop".

   *A note on the trigger this section used to offer.* "Graduate on telemetry"
   was not a usable option and should not be proposed again without checking:
   `parlay-feedback` is opt-in and off by default, writes to `.parlay/feedback/`
   locally rather than phoning home (so a human must send it), and records
   outcomes **produced** rather than surfaced — the presence codes fire on both
   the readiness and the validate path, so any threshold read off that log
   double-counts. What would actually inform a question like this is a one-off
   look at real projects' artifacts, not a monitoring programme.
2. ~~Does `--fragments` duplication belong in the tool at all?~~
   **Decided 2026-08-25: it stays, framed as draft seeding.** Not "faster but
   less accurate" — it produces a draft a human must review, and it is never
   the authoritative fix. It relocates text and cannot tell a presentation
   claim from a contract one, so it will attach backend-shaped criteria to UI
   fragments; unreviewed, those demand display cases that cannot be written
   honestly and the build phase writes vacuous ones to discharge them.

   Routing every backfill through `/parlay-refine` was rejected as
   disproportionate *and* as ledger pollution: refine appends a permanent,
   never-editable amendment to the feature's design history, regenerates code
   and runs the full suite — per feature. Backfilling criteria is a tool
   migration, not a product decision, and the ledger exists to record why the
   product changed. `/parlay-refine` remains the route where the split needs
   genuine design judgement.

   The wording is deliberately identical in the command help, the readiness
   fix text, `build-feature`, and `surface.schema.md`: seeds a draft, requires
   review, never the fix.

## Explicitly out of scope

- The finding's "Not explained by the above" — four empty operation suites and
  no API test emitted, `testcases-operation-uncovered` ×4, despite 8 operation
  criteria available to cite. A separate defect on the build phase's side, not
  inherited from this one. Stays a Phase-1 n=5 question.
- The vacuity walker's any-vs-all semantics (see *Cross-kind criterion refs*).

## Alternatives considered and rejected

**Let presentation cases cite operation criteria, generally.** Cheapest
possible fix — the validator already accepts it. Rejected as a *general*
answer: it makes every display criterion a store assertion by construction and
without the `coverage: state-only` stamp that exists to record exactly that
downgrade, and it leaves surface fragments criteria-free in any feature with a
backend. Narrowed to the rule above rather than banned outright.

**Make `create-artifacts` copy operation criteria down to fragments
automatically.** Rejected for the same reason `--fragments` is opt-in: a
contract-shaped claim copied onto a fragment demands a display case that cannot
be written honestly, and the agent will write a vacuous one to discharge it —
`testcases-case-vacuous` one phase later, for a criterion the tool invented.

**Indexed or hashed bullet IDs for criterion identity.** See WS 0.

**Diagnostic only, as the finding suggests.** Rejected as a complete fix, kept
as WS B. It would fire on every spec that follows the documented rule to the
letter, which trains the reader to ignore it.
