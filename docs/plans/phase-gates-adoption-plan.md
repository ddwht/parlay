# Phase Gates — Adoption Plan

**Status:** proposed. Written 2026-08-21.
**Thesis:** keep the agent as the pipeline driver, but move phase-transition
enforcement into the CLI as pure, recomputed gates — "agent proposes, CLI
disposes." Three workstreams: close the spec-authority holes (WS1), build the
gate command and inject it into every phase module at deploy time (WS2), and —
later, optionally — a headless executor for the mechanical tail (WS3).

## Background and motivation

Parlay's pipeline (intents → dialogs → artifacts → build → code) is driven by
an agent following skill prose. The deterministic checkers exist —
`check-readiness`, `check-drift`, `check-amendments`, `diff`,
`check-review-gate` — but they run only when the agent follows the text.
Every invariant that matters ("gates ran before the phase advanced",
"artifacts weren't regenerated from pre-amendment founding docs", "frozen
docs weren't touched") is currently a prompt, and prompts leak. The project
has two scars proving it: the subagent-AskUserQuestion rule in CLAUDE.md and
the surface.md retirement.

The alternative — inverting the architecture so a deterministic loop drives
and the agent becomes a stateless per-iteration worker — was considered and
rejected: parlay's artifacts are long-lived amended contracts, not disposable
plans, and the designer conversation is the product. What this plan takes
from that model is its one transferable structural insight: **completion is
verified against disk by code, never asserted by the agent.**

A second motivation is stale-spec confusion. The founding documents
(intents.md, dialogs.md) are frozen after first build; post-birth change lives
in the amendment ledger and is spliced into the contract artifacts
(surface.yaml, capabilities.yaml, infrastructure.md, domain-model.yaml).
An agent that reads founding docs without an authority ordering can describe
or build against a design the product has amended away. The code phase is
already fully insulated (generate-code.md forbids reading `spec/intents/`),
and testcases derive from `verify:` fields with no intent fallback since
v0.3 — but the build phase's input step and the artifacts phase have no such
ordering. Those are the WS1 holes.

Note on current state: no `amendments/` directory exists anywhere in this
repo yet (core or studio), and `.parlay/build/` has never been populated in
this checkout. The amendment machinery is untested against reality; the
validation step in the sequencing section exists for exactly that reason.

## Design principles

1. **Gates are pure recomputations.** No persisted "gate passed" stamp. The
   loop skill's hard rule (SKILL.md:296 — never persist resume state, no
   phase cursor) is correct and this plan conforms to it: a stored pass-stamp
   goes stale the moment a spec file changes and then becomes the stale-state
   problem itself. `ComputeFeaturePhase` is already a pure ladder over disk;
   gates follow the same contract. Passage is re-derived from hashes at every
   boundary.
2. **Enforcement has two layers.** A deploy-time-injected gate step in every
   phase module (catches agents that follow instructions — and makes the
   instruction impossible to omit from any module, present or future), plus a
   repo-level sweep usable as pre-commit/CI (catches everything else; this is
   the genuinely unskippable layer).
3. **Aggregate, don't reimplement.** The gate command composes the existing
   checkers in-process. No checker logic is duplicated.
4. **The designer group stays interactive forever.** Headless execution (WS3)
   applies only to the mechanical tail (build, code). Decision points in
   headless mode surface as blocked-envelopes; they are never auto-answered.

## What already exists (analysis findings)

The implementation survey found the infrastructure is mostly present:

| Piece | Where | Status |
|---|---|---|
| Phase ladder | `core/internal/commands/feature_phase.go` — `FeaturePhase` (`intents/dialogs/artifacts/build/done` + `hand-authored`), `ComputeFeaturePhase` (pure, monotonic, disk-derived) | exists |
| Stage-keyed readiness | `check_readiness.go` — required `--stage` flag (`dialogs`, `create-surface`, `build-feature`), `readinessOutput{feature, stage, ready, issues[]}` | exists |
| Gate template | `check_review_gate.go` — closest existing analogue (coverage gate: JSON out, `NewExitCodeError(1)` on closed) | exists |
| Amendment position | `baseline.go` — `Baseline.LastAppliedAmendment` (:51), `HashedSources.Amendments` per-file ledger hashes (:160), `detectLedgerFindings` (:521) computing the unapplied tail | exists |
| Ledger validation | `check_amendments.go` (whole-ledger: sequence, supersedes, affects, dirty_set), `check_applied.go` (refine pre-flight, `clean_state`) | exists |
| Refine progress journal | `refine-journal` steps: `amendment-written → splice-applied → rebuilt → emitted → tested → re-baselined → --clear` | exists |
| Deploy-time injection | `core/internal/embedded/skills.go` — marker expansion into deployed modules (decision-protocol block, active-root section); `surface:` frontmatter routes skill→`.claude/skills/`, module→`.parlay/modules/` | exists |
| Exit conventions | `exit_code.go` (`ExitCodeError`), `disambig_signal.go` (exit 11 + one-line JSON envelope on stderr) | exists |
| Headless protocol | loop SKILL.md:127-135 — `--non-interactive` takes `default:` on advancement kinds, aborts exit-11 `{"kind":"blocked",...}` on `ambiguity`/`overwrite`/`failure` | exists |

Gaps confirmed by the survey:

- `build-feature.md` step 5 (deployed :133-141) reads `intents.md`/`dialogs.md`
  raw, unconditionally, with no amendment/ledger awareness and no authority
  ordering. The only artifacts-win language in the module is buried in the
  testcases bullet (:331).
- `create-artifacts.md` has zero amendment awareness. Re-running it on an
  amended feature would regenerate "current truth" artifacts from
  pre-amendment founding docs, silently reverting the amendments. Only
  `artifacts-already-exist` handling exists (:201), which asks
  regenerate-vs-skip without knowing about the ledger.
- `check-readiness --stage build-feature` does not treat unapplied amendments
  as an error (check-drift reports them, but only the loop driver reads
  check-drift, and only at planning time).
- The driver's boundary checks are scattered (check-readiness at one
  boundary, check-drift at planning, nothing mechanical at build→code beyond
  module prose) and each is individually skippable.

---

## Workstream 1 — Close the authority holes

**Effort:** ~half a day. **Dependencies:** none. All skill edits follow the
dogfooding rule: edit source under `core/internal/embedded/skills/`,
`make build`, `./parlay upgrade` (or `make sync-skills`), verify with
`make verify-skills`, re-add the project-local CLAUDE.md section.

### 1a. Precedence rule in build-feature step 5

In `core/internal/embedded/skills/build-feature.skill.md`, extend step 5
(Read feature files) with an authority-ordering paragraph:

> The founding documents (`intents.md`, `dialogs.md`) are narrative context —
> they record why the feature exists and how it was conceived. The contract
> artifacts (`surface.yaml`, `capabilities.yaml`, `infrastructure.md`,
> `domain-model.yaml`) are current truth — they have absorbed every applied
> amendment, which the founding documents are frozen against. Where they
> disagree, the artifacts win. Do not carry an intent- or dialog-level detail
> into the buildfile when the contract artifacts contradict it.

Mirrors the rationale already present at the testcases bullet ("intents'
bullets are history and may predate amendments the contract has absorbed") so
the ordering is stated at ingestion, not only at test derivation.

### 1b. Amendment guard in create-artifacts

In `create-artifacts.skill.md`, add a pre-step before step 2 (Analyze
intents), copying the pattern of refine's step 1.5 pre-flight:

- Run `parlay internal check-applied @{feature}`.
- If the feature has a non-empty `amendments/` ledger, or
  `ComputeFeaturePhase` reports phase ≥ build (founding docs frozen), stop
  with an `impasse` decision block routing to `/parlay-refine`. This module
  authors artifacts at birth only; post-birth change goes through the ledger.
- Extend the `artifacts-already-exist` error-handling entry to state the
  reason: regenerating from founding docs after amendments silently reverts
  the amendments.

### 1c. `unapplied-amendments` as a readiness error

In `core/internal/commands/check_readiness.go`, add an issue code
`unapplied-amendments` (severity **error**, fix text: "run /parlay-refine to
apply the ledger tail") to the `build-feature` stage, reusing
`detectLedgerFindings` / `Baseline.LastAppliedAmendment` from `baseline.go`.

Payoff: check-readiness already runs at **two** choke points — the loop
driver's designer→build boundary (loop SKILL.md:198, errors are hard blocks
per hard rule :300) and build-feature's own step 6 (:143). One Go change puts
a mechanical stop in both places with no skill edits.

Caveat: this error must respect the refine in-flight exception — see 2b. In
WS1 (before the gate exists), scope the error to fire only when no active
refine journal is present, using the same journal probe refine's tooling
uses.

Tests: table-driven cases in `check_readiness_test.go` (`setupTestDir`,
`resetFlagsAfterTest`, `testCommandWithContext`, assert `ExitCodeError.Code`
and parsed JSON): ledger with unapplied tail → error; fully applied ledger →
no issue; no ledger → no issue; unapplied tail + active journal → no error.

---

## Workstream 2 — Gate command + injected preamble

**Effort:** ~2–3 days including tests. **Dependencies:** 1c (reuses the
readiness error); the amendment-exercise validation step (see Sequencing).

### 2a. `parlay internal gate @{feature} --stage <boundary>`

New file `internal/commands/gate.go` (cmd var + `init()` flags +
`runGate` starting with `mustContext(cmd)`), registered in
`internal_group.go`. Modeled on `check_review_gate.go`.

The gate is an **aggregator**: it invokes the existing checkers in-process
(direct function calls, not subprocess) and merges findings into one verdict.

| `--stage` | Boundary | Aggregates |
|---|---|---|
| `build` | designer → build | check-readiness `build-feature` (incl. 1c's `unapplied-amendments`); check-drift findings `ledger_integrity` + `unapplied_amendments`; check-amendments when a ledger exists |
| `code` | build → code | check-buildfile; check-composition; buildfile signature freshness (`source-signatures:` comparison that generate-code step 11.6 currently performs by prose) |
| `done` | code → complete | verify-generated; check-review-gate |

Output:

```json
{"feature": "...", "stage": "build", "passed": false,
 "blockers": [{"code": "...", "message": "...", "fix": "..."}],
 "warnings": [...]}
```

Written to `cmd.OutOrStdout()`; `return NewExitCodeError(1)` when
`passed:false`. Blocker codes flow into the DIGEST generation so agents can
pre-check them (per the schema-loading contract in CLAUDE.md).

Design constraints:

- **Pure.** No file writes, no stamps. Same purity contract as
  `ComputeFeaturePhase` (document it in the same style).
- Stage vocabulary aligns with the `FeaturePhase` ladder, not with
  check-readiness's legacy `create-surface` naming; the gate maps internally.
- Multi-root: nothing special — `mustContext` resolves the active root like
  every other command; `--ambiguity-as-signal` behavior comes free from
  `persistentPreRun`.

### 2b. Journal-aware gating (the refine edge case)

A `build`-stage gate that hard-fails on unapplied amendments would break
`/parlay-refine` step 5.5, which legitimately rebuilds the buildfile *while*
the ledger tail is mid-application. Resolution: the gate reads the refine
journal; an active journal at step `splice-applied` or later marks the dirty
ledger as sanctioned in-flight work and **downgrades that blocker to a
warning** (all other blockers still block). A journal at an earlier step, or
a stale journal for a different amendment than the dirty tail, does not
downgrade.

This is the one subtle piece of gate logic; it deserves a focused design
review and its own test table (journal absent / early / late / stale /
mismatched-amendment).

### 2c. Deploy-time gate preamble

- Add a `gate-stage:` frontmatter field to module skill sources.
- In `core/internal/embedded/skills.go`, add a `gateMarker` expansion
  (sibling of the decision-protocol marker) that injects a uniform
  "Step 0 — Gate" block into every deployed module whose source declares
  `gate-stage:`:

  > Run `parlay internal gate @{feature} --stage <X>`. If it exits non-zero,
  > stop: surface the blockers via a `failure` decision block. Do not
  > proceed, do not fix-and-retry silently — the blockers name their own
  > fixes and the driver decides.

- `build-feature.skill.md` → `gate-stage: build`;
  `generate-code.skill.md` → `gate-stage: code`. Modules without the field
  (add-feature, scaffold-dialogs, create-artifacts, etc.) are untouched.

Property this buys: no module author — including future ones — can write a
phase module that forgets the gate, because the deployer writes it in. Same
mechanism that today guarantees every module carries the decision protocol.

### 2d. Loop and refine updates

- `loop.skill.md`: the driver's designer→build boundary step calls
  `parlay internal gate --stage build` instead of bare check-readiness
  (one command, one exit code, includes drift/ledger findings the driver
  previously only checked at planning time). Add a hard rule to the list at
  :294-303: "NEVER advance a phase-group boundary without a passing
  `parlay internal gate` for the target stage."
- `refine.skill.md`: no structural change — step 5.5 reads the build module
  and inherits the injected gate; 2b keeps it passable mid-refine.
- Keep the module-side gate AND the driver-side call. Redundant by design:
  the driver's call gates advancement; the module's Step 0 gates direct
  invocation (`/parlay-refine` 5.5, `--from build`, manual module runs).

### 2e. Repo-level backstop: `parlay gate --all`

Thin **top-level** command (stable surface — CI shouldn't depend on
`internal` shapes): for every feature in every root, compute
`ComputeFeaturePhase`, run the corresponding stage gate, print a table,
exit non-zero on any blocker. Wire as a pre-commit hook and CI job.

This is the genuinely unskippable layer: the injected preamble (2c) catches
agents that follow instructions; the sweep catches everything else. It is
also, verbatim, the merge gate for a future worktree-per-run model: when
pipeline runs execute in a worktree, "`parlay gate --all` green" is the merge
condition, and frozen-doc enforcement (reject modified files under
`spec/intents/<feature>/` post-first-build; admit only new files under
`amendments/`) joins the same sweep. That extension is out of scope here but
the command should be written knowing it's coming (e.g. a `--check` list
flag).

### Tests (WS2)

`internal/commands/gate_test.go`, table-driven, fixture trees under
`internal/commands/testdata/` (the `matrix/` / `multitarget/` style):

- per-stage pass and each blocker class;
- 2b's journal matrix;
- aggregation: multiple failing checkers → merged blockers, stable ordering;
- `gate --all` sweep across a multi-root fixture (core + studio style);
- exit codes (0 / 1 / 11-ambiguity passthrough).

Deployment verification: `make sync-skills && make verify-skills`; grep
deployed `.parlay/modules/build-feature.md` and `generate-code.md` for the
injected Step 0.

---

## Workstream 3 — Headless executor for the mechanical tail (later, optional)

**Effort:** ~1 week spike. **Dependencies:** WS2 dogfooded for a few weeks.
A headless loop without mechanical gates would combine the weaknesses of both
architectures — autonomous execution with nothing verifying it — so this
lands last.

### 3a. `parlay run @{feature} --from build`

Go orchestrator scoped to the build and code groups only:

```
loop:
  phase   = ComputeFeaturePhase(feature)
  gate    = parlay internal gate --stage <phase.next>   # pre-gate
  if !gate.passed → stop, report blockers
  spawn fresh agent session on the phase module (--non-interactive)
  gate again                                            # post-gate: trust-but-verify
  if !gate.passed → retry once, else stop
  advance
```

- Executor abstraction: configurable command template (default `claude -p`),
  so the agent CLI is swappable. Note that `claude -p` bills against the
  separate Agent SDK credit pool for Claude subscription users — evaluate
  whether that changes the default before building.
- Decision points: modules under `--non-interactive` already abort with the
  exit-11 `{"kind":"blocked",...}` envelope on `ambiguity`/`overwrite`/
  `failure`. `run` surfaces the envelope and stops. It never answers a
  decision itself — the loop's cardinal sin (a skipped confirmation being
  indistinguishable from a granted one) made structurally impossible.
- The designer group is out of scope permanently; `run` refuses
  `--from intents|dialogs|artifacts`.

### 3b. Worktree airlock

`parlay run --worktree`: create `.parlay/worktrees/<feature>`, execute
there, merge to the main tree only on green
`parlay gate --all`, delete the worktree after merge. Worktree per **run**,
not per feature — the artifacts' permanent home stays on main; the worktree
is scratch space for one pipeline pass. Combined with the frozen-doc checks
in the 2e sweep, this delivers the append-only-main property: every change
to a founding document arrives as a new amendment file, enforced at the
merge boundary rather than by convention.

---

## Sequencing

| # | Step | Depends on |
|---|---|---|
| 0 | *(optional but recommended)* Fix `parlay upgrade` to preserve the project-local CLAUDE.md dogfooding section — WS1/WS2 involve many sync-skills cycles and the manual re-add will be error-prone | — |
| 1 | WS1 (1a, 1b, 1c) | — |
| 2 | **Exercise the amendment machinery once, end-to-end, on a throwaway feature**: full loop on a toy feature, one amendment via `/parlay-refine`, observe check-amendments / check-applied / `last-applied-amendment` / re-baseline behave. The ledger code has never run against reality in this repo; gates must not be built on unexercised layers. Findings become WS2 fixtures. | 1 |
| 3 | WS2 (2a → 2b → 2c+2d together in one sync-skills → 2e) | 1c, 2 |
| 4 | Dogfood WS2 for a few weeks (all pipeline runs in this repo go through gates) | 3 |
| 5 | WS3 decision point: build `parlay run` or not, based on how often the mechanical tail actually runs unattended | 4 |

## Risks and open questions

- **Gate runtime cost** — aggregation re-runs checkers that phases may run
  again internally. Mitigated by in-process calls (no subprocess overhead);
  if it grows, checkers can expose result reuse, but don't build that yet.
- **2b (journal-aware gating)** is the one subtle design; review it
  separately before implementation.
- **Stage vocabulary drift** — check-readiness's `create-surface` vs the
  ladder's `artifacts`. The gate maps internally; consider a follow-up
  migration to unify, but don't block on it.
- **Testing without runtime state** — this checkout has no `.parlay/build/`
  tree; all WS2 tests are fixture-based. The step-2 amendment exercise
  supplies realistic fixtures.
- **CLAUDE.md overwrite on upgrade** — every `make sync-skills` clobbers the
  dogfooding section until step 0 is done.
- **`--non-interactive` default-taking** (advancement kinds take `default:`)
  is load-bearing for WS3 — audit which boundaries carry defaults before
  trusting headless advancement.

## Alternatives considered and rejected

- **Stdout sentinel protocols** (magic marker strings parsed from agent
  output) — parlay's `parlay-decision` blocks are a better-typed version of
  the same idea.
- **Checkbox-grammar plan files** as the execution ledger — the buildfile is
  a far richer intermediate; a plan-as-mutable-ledger model fits disposable
  plans, not amended contracts.
- **Lifecycle-by-location** (moving artifacts to a `completed/` folder to
  mark them done) — parlay artifacts are permanent; lifecycle is carried by
  the ledger and (later) the merge gate, not by file relocation.
