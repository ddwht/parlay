# Full Benchmark — Live Findings

Ledger vs status quo, full run. Started 2026-08-13. This file is appended to as legs complete —
raw per-step metrics plus every friction/misalignment the step agents report, so improvement
points are captured even where the headline numbers look fine.

## Design

- 6 asks per leg: R1 `done`→`complete` (replace-span), R2 `--quiet` (add), R3 `built_at` (add),
  R4 orphaned-build-dir warning (add), **R5 reverses R1** — `complete`→`✓ done` (contradiction;
  exercises `supersedes:` vs re-editing), **R6 engineering handoff** (exercises the ledger's
  projection vs the classic enggspec).
- n=3 replicates per variant, each in a fresh clone pair (`parlay-bench/full/rN-{sq,lg}`).
  Replicate 1 fully sequential (clean per-step output tokens); replicates 2–3 run as parallel
  legs (wall-clock + outcomes; token splits approximate, flagged).
- Every step agent reports `frictions`: anything that fought it — unclear skill steps, gates that
  misfired, docs that contradicted, tool errors. One judge per replicate pair.
- Base commits: status quo `debb835`, ledger `a8a10b6` (implementation head + docs).

## Carried over from the pilot (unresolved)

- **P1 — stale prose in ledger mode**: frozen surface.md/dialogs.md go stale by design; pilot
  judge flagged a buildfile comment still claiming fragments come from surface.md. Deliberately
  NOT patched before this run — expect it to recur; the fix (retire/migrate surface.md in ledger
  projects, regenerate projection after apply) is queued behind this run's evidence.
- **P2 — small asks skip testcases.yaml in both variants**: R1/R3 were covered only at the
  Go-test level. Watch whether it repeats across replicates.
- **P3 — spec underdetermination**: `--quiet` phase-token ambiguity got opposite resolutions.
  Watch R2 across replicates: if resolutions flip within a variant, that's ask-level noise, not a
  variant property.

## Live findings

### Update 1 — r1-sq R1–R3 complete (sequential leg)

Metrics: R1 396s, R2 514s, R3 560s — all completed, suites green. (Faster than the pilot's same
asks; fresh-clone state and normal agent variance.)

**Frictions reported by the step agents (status-quo toolchain), deduplicated — reporter count in
parens:**

- **F1 (3×) — skill/CLI name mismatch:** refine step 11 says confirm "`check-drift` is clean" but
  there is no top-level `parlay check-drift`; it is `parlay internal check-drift` (one agent fell
  back to `parlay internal diff`). Skill prose disagrees with the binary's command surface.
- **F2 (3×) — `save-build-state --partial` isn't partial for baselines:** with a 4-file emitted
  manifest it still rewrote EVERY per-feature `.baseline.yaml` (40+ files, re-stamped
  generated-at) and adopted hundreds of unrelated files into `.code-hashes.yaml`. Commits are
  dominated by baseline churn unrelated to the change. `--partial` scopes only the project-level
  code-hashes.
- **F3 (2×) — `review-coverage` has no unattended path for never-reviewed features:** it demands
  interactive approval of all suites and aborts on EOF; `--exempt` exempts individual terms, not
  the review itself. Also: step 10 is unconditional even when testcases did not change and no
  coverage-review.yaml exists — the step's own rationale doesn't apply, but the skill doesn't
  say so.
- **F4 (2×) — `.emitted` manifest is half-specified:** nothing in the toolchain creates it (one
  agent hand-created it before save-build-state could run), and the skill doesn't say whether
  paths are absolute or source-root-relative (reverse-engineered from `.code-hashes.yaml`).
- **F5 (2×) — surface.yaml + surface.md dual-tracking (status-quo variant of pilot P1):** the
  feature carries both with duplicated content, hashed by two different baseline mechanisms;
  nothing states which is canonical, and refine must amend both to stay consistent. This is not
  only a ledger-mode problem — the status quo pays it on every surface amendment.
- **F6 (1×) — add-vs-inert classification ambiguity:** whether a new output-mode flag is an "add"
  (new fragment → rebuild via AI build phase) or an inert extension of an existing fragment is a
  judgment call the skill doesn't ground; and step 5.5's "run the build phase" conflicts in
  spirit with the same skill's "splice, never re-encode" principle.
- **F7 (1×) — cross-altitude falsification:** the R1 change made a sentence in infrastructure.md
  false ("tabwriter rendering uses the constant directly"); changes that span surface + 
  infrastructure have no guidance for keeping the prose artifact truthful.

Ask-level note: the R2 quiet-token ambiguity (pilot P3) was again called out as a genuine
judgment call by the r1-sq agent — evidence it is ask-underdetermination, not variant behavior.

### Update 2 — r1-sq complete (R4 971s, R5 434s, R6 252s, all green)

The three probe asks earned their place. New findings (status-quo toolchain):

- **F8 — refine surfaced REAL pre-existing spec/code drift:** R4's orphaned-build-dir behavior
  already partially shipped in status.go/status_test.go but appeared in no spec artifact and not
  in the buildfile. The refine flow found and had to absorb genuine drift that predates the
  benchmark — evidence the drift problem the ledger model targets exists in the wild, and that
  the add/replace dichotomy (step 4/5.5) has no branch for "reshape behavior whose spec had
  drifted".
- **F9 — the alignment tax failed exactly as theorized:** the R5 agent found dialogs.md examples
  still showing `done` in human output — i.e., the R1 round amended surface + infrastructure but
  nobody synced dialogs, so the narrative layer self-contradicted within one session. This is the
  N-document sync failure the proposal predicted, reproduced organically by an agent following
  the skill.
- **F10 — handoff flow is under-tooled (both variants will hit this):** no CLI command backs the
  enggspec flow; no template/schema for specification.md ships (`.parlay/schemas/` has none);
  and the R6 audit caught TWO drifts introduced earlier in the same session — the buildfile's
  FeatureEntry model was never updated for `built_at` (R3 skipped it), and code carries behavior
  beyond the feature's spec (`PhaseHandAuthored`, the `kind` JSON field). The handoff step is
  currently the only audit that catches such drift, and it is optional/side-branch.
- **F11 — a Go-specific authoring trap:** backtick-containing example text inside a raw-string
  Long help literal silently broke compilation; cost the R4 agent a debugging detour (agent
  self-attributed, but a codegen-guidance note would prevent it).
- Repeats confirmed: F2 (baseline churn — 43 files re-stamped for a 3-file change), F3
  (review-coverage unattended dead-end, now with the sharper wording that the step is chained as
  "not optional" while the command "refuses by design" — hardened in 99596f8), F4 (.emitted not
  auto-created, format undocumented), F5 (surface.yaml/surface.md amended twice by hand), F1
  (check-drift naming).

Cost note: R5 (the contradiction) was cheap in status quo — 434s, a re-edit of the spans R1
touched (leaving no record of the reversal; judge will assess). R6 (handoff) 252s.

### Update 3 — r1-lg R1–R2 (668s, 906s, both green) — ledger-implementation flaws found

These are defects in the ledger toolchain itself (this branch), not inherited ones. All
actionable:

- **L1 — dirty_set vs diff comparison is apples-to-oranges:** refine step 3.5 asserts
  check-amendments' `dirty_set` "should agree with what step 5's diff reports dirty" and treats
  disagreement as a stop condition — but the two speak different vocabularies (amendment refs
  `@f/surface:frag` vs diff's component names), with no defined mapping. The R2 agent hit the
  contradiction the skill told it to stop on, in a run where nothing was wrong. Fix: define the
  ref→component mapping mechanically (or soften the skill claim until 3.1's Go-side comparator
  exists).
- **L2 — step-ordering bug for ADDs:** step 3.5 (write amendment, run check-amendments) precedes
  step 4 (splice the artifact), but `amendment-affects-unresolved` requires the affected fragment
  to already exist — so for a NEW fragment the amendment cannot validate until after the apply.
  Fix: for adds, allow forward refs (validate after apply) or reorder validation.
- **L3 — no way to cite an amendment as a fragment's source:** the surface schema requires every
  new fragment to cite a founding intent via `source:`, but under the freeze genuinely-new
  behavior has no exact intent — the R2 agent was forced into an approximate citation. Fix:
  ledger projects should accept `source: @feature/amendment:NNN-slug` (schema + validators).
- **L4 — `save-build-state --source-root` underspecified, can corrupt silently:** `{root}` is
  never defined; the adapter's declared `file-conventions.source-root` (`internal/commands/`)
  differs from the breadth actually tracked (internal/agent, internal/parser, ...), and code-hash
  keys are repo-root-relative while the skill reads as active-root-relative. One agent reports
  taking the instruction literally "silently corrupted state" before recovering. Also: the
  `.emitted` manifest is deleted on success, and a second `--partial` run pointing at the
  now-missing explicit path does NOT error. Highest-severity engine finding of the run so far.
- **L5 — modules path resolution:** `.parlay/modules/` exists only at the parent root, but skill
  references read as active-root paths; child-root execution has to know to look up. (Multi-root
  resolution gap in skill prose.)
- **L6 — smaller mismatches:** `check-buildfile --json` rejected (flag doesn't exist; JSON is
  unconditional) — doc vs binary; the prebuilt clone binary stays stale after a refine (behavior
  only visible via go test until rebuild) — worth a "rebuild before smoke-test" line in the
  skill; mixed cwd conventions for `parlay internal` invocations.
- Cross-variant repeats in ledger mode: F2 (baseline churn — ~40 baselines + 419-line code-hashes
  diff for a one-line change), F3 (coverage step "not optional" vs tool refusing unattended,
  again with no coverage-review.yaml existing), F1-adjacent naming issues.

Early cost signal, rep1 (sequential, clean tokens pending final result object): ledger R1 668s vs
sq R1 396s; ledger R2 906s vs sq R2 514s — the ledger legs ran SLOWER here, opposite the pilot;
much of the excess is attributable to L1/L2/L4 detours (agents fighting the new machinery). The
inversion's practical overhead is real until these flaws are fixed — exactly the kind of result
the full run exists to surface.

### Update 4 — r1-lg R3–R4 (837s, 853s, green): the dirty_set flaw is now precisely diagnosed

- **L7 (supersedes L1) — `dirty_set` has the wrong scope by design:** check-amendments returns
  the cumulative union of ALL amendments' `affects:` (001+002+003...), while step 5's diff
  reports only what is currently dirty — so the "should agree" comparison the skill mandates can
  NEVER hold once one amendment has been applied and re-baselined. Concrete engine fix: compute
  `dirty_set` from the UNAPPLIED tail only (amendments beyond the baseline's
  `last-applied-amendment`), with the full union available under a separate key. This is a bug in
  the branch's check_amendments.go, found only because real sequential refinements accumulated a
  ledger.
- **L8 — `.emitted` silent degradation confirmed with a reproduction:** the manifest is consumed
  and deleted by save-build-state; a subsequent `--partial --emitted <now-missing-path>` run
  SILENTLY treats it as an empty declaration instead of erroring. Engine fix: explicit `--emitted`
  path that doesn't exist must be an error.
- **L9 — `--source-root` multi-root hazard, second reproduction:** passing the absolute repo root
  pulled 92 `internal/editor/**` files into tracking; the adapter's declared source-root doesn't
  match the tracked breadth; mismatched manifests silently downgrade generated files to
  `adopted`. Fix cluster: define `{root}` in the skill, validate the manifest against the
  scanner's key form, warn on scope surprises.
- **L10 — verify-bullet additions don't trip the rebuild trigger:** adding `verify:` bullets to
  an EXISTING fragment lands acceptance criteria that should become testcases, but step 5.5's
  trigger is worded "added a spec ELEMENT" (new component) — so the criteria never mint suites.
  Wording/semantics fix in refine + build-feature.
- **L11 — YAML scalar trap in acceptance bullets:** bullets containing `key: value` shapes
  (`built_at: null`) break surface.yaml as bare scalars, and check-amendments surfaces it only as
  a generic parse error. The migrator's `yamlScalar` quoting exists for exactly this; the skill's
  apply step needs the same rule, and the error should name the offending bullet.
- **F8 confirmed in the ledger clone with provenance:** the orphan behavior existed in code since
  a pre-benchmark commit, in a DIFFERENT shape than the ask (per-root arrays + multi-line human
  block), and was never specced — both variants had to reconcile an ask with untracked drift.
  Also surfaced: the ask's shape now coexists with the legacy shape (same key name at two JSON
  levels with different types) — a real API-design wart produced by reconciling drift
  conservatively.
- **L12 — hand-maintained code vs the buildfile mandate:** step 5.5 requires an ADD to mint
  components via the build phase, but status.go's orphan logic is hand-maintained and not
  buildfile-driven; the skill has no branch for "the owning code is not generated". (Related:
  authored-units machinery exists but this file isn't declared one.)
- surface.md staleness recurs in ledger mode (still says `done`, lacks quiet) while the
  file-ownership doctrine still lists surface.md as co-canonical — P1, now with the doc citation.

### Update 5 — replicate 1 COMPLETE; parallel replicates underway (4 R1s done)

Replicate-1 wall-clock totals: **status quo 3,127s vs ledger 4,454s — the ledger leg ran ~42%
slower**, reversing the pilot. The excess is visibly attributable to the L-series flaws (source-
root misfires, .emitted deletion recoveries, dirty_set contradictions), not to the model itself:
where the machinery cooperated, ledger steps were comparable. The implementation, not the
inversion, is currently the bottleneck — fix the L-series and re-measure.

- **L13 — CRITICAL: a corrupted amendment entered the frozen ledger undetected.** Amendment 004
  ends with stray tool-call tags (`</content>`, `</invoke>`) — junk written by the R4 agent's
  file write. `check-amendments` accepted it (markdown body tolerates arbitrary text); it was
  caught only by the R6 projection agent actually READING the ledger. Worse: under the freeze,
  the corruption is now immutable by the model's own rules — fixing the file is a
  ledger-integrity violation; the correct paths (correcting amendment, or doctor's
  bless-and-refreeze) both exist but nothing points at them. Fixes: (a) validator: flag
  non-markdown trailing junk / tool-tag artifacts in amendment bodies; (b) refine 3.5: re-read
  and validate the file AFTER writing, before proceeding; (c) doctor: this exact scenario as a
  worked example.
- **L14 — "replace is inert" is false for prose mirrors:** step 5.5 says a replace-span keeps the
  buildfile untouched, but buildfile `description:` prose and testcases `expected:` values still
  referenced the old label after R5 — inert for component TOPOLOGY, not for embedded text. Skill
  needs the distinction.
- **L15 — cross-amendment invariant collision, and the ledger caught it:** R5's `✓ done` label
  breaks the `--quiet` contract documented in amendment 002 (one-space two-field format — the
  label contains a space). The collision existed in both variants; only the ledger leg had the
  002 record that made it discoverable at refine time. Direct evidence for the record's value —
  and for wiring check-amendments to search prior amendments' Acceptance for conflicts.
- **L16 — the projection has a double-derivation ambiguity:** enggspec's ledger guidance says
  "read founding docs, apply amendments in sequence" — but apply has already folded amendments
  into the contract artifacts, so a naive reading applies them twice. Rewrite the guidance:
  contract artifacts are current truth; the ledger is read for History/rationale only.
- **L17 — amendments never touch infrastructure.md:** all five amendments' `affects:` targeted
  surface fragments; the architecture prose now lags the shipped surface (predicted by the
  proposal's infrastructure asterisk; now observed).
- **L18 — "lands Acceptance as verify:" is manual:** no tool performs the landing; the skill's
  language implies automation that doesn't exist. Either build the small tool or reword.
- Cross-variant engine notes (both toolchains): `status_test.go` carries no parlay marker so its
  emitted-manifest line is a silent no-op (2×); one absolute `--source-root` re-keyed the entire
  code-hashes file (silent, exit 0, 274-file WARN flood); the clone's stale July baselines caused
  large catch-up churn on first save (bench-methodology note: pre-baseline the clones next time);
  the feature's buildfile predated source-signatures entirely — step 7's "re-stamp" was a first
  stamp.

Parallel-replicate R1s (indicative wall only): sq 452s/550s, ledger 667s/695s — consistent with
rep1's direction at step 1.

### Update 6 — run interrupted at ~64% by the session usage limit (resets 04:10 Europe/Sofia)

25/39 agents completed before the limit: **replicate 1 complete in both legs with clean tokens**;
r2 legs 3/6 each, r3-sq 4/6, r3-lg 3/6; **all three judges unfired**. The workflow is resumable
from cache (`resumeFromRunId: wf_f1ecdfad-0bd`) — only the 14 failed agents re-run.

**Replicate 1 final per-step numbers (sequential, clean attribution):**

| Step | Status quo (tok / s) | Ledger (tok / s) |
|---|---|---|
| R1 replace | 27,736 / 396 | 44,721 / 668 |
| R2 add | 37,112 / 514 | 60,864 / 906 |
| R3 add | 36,316 / 560 | 56,252 / 837 |
| R4 add+drift | 69,428 / 971 | 58,246 / 853 |
| R5 contradiction | 31,270 / 434 | 66,605 / 924 |
| R6 handoff | 19,326 / 252 | 21,952 / 266 |
| **Total** | **221,188 / 52.1 min** | **308,640 / 74.2 min** |

Read with the friction ledger in hand: the ledger leg spent **+40% tokens / +42% time**, and the
per-step notes attribute the bulk of the excess to L4/L7/L8 recovery detours (source-root
misfires, deleted-manifest reruns, dirty_set contradictions the skill orders a stop on). Two
steps cut the other way: R4 — where status quo paid 69k tokens reconciling the pre-existing
untracked drift (F8) that the ledger leg absorbed more cheaply — and R6, near-parity. R5 is the
starkest reversal-of-expectation: recording the contradiction properly (amendment 005 +
supersedes) cost 2.1× status quo's silent re-edit — the ledger buys its better record at real
price until the machinery smooths out.

Interim conclusion (pre-judges): the **pilot's −14% and this run's +40% bracket the truth** — the
inversion's savings are real where the machinery cooperates (pilot; R4 here) and are currently
overwhelmed by implementation frictions accumulating with ledger depth (L-series). The fix
backlog above is the path to the model's measured potential; re-benchmark after L1–L18 fixes.

### Update 7 — resume aftermath: accidental crash-recovery and idempotency probes

Methodology caveat first: the post-limit resume re-dispatched some already-completed mid-leg
steps instead of replaying them from cache, so replicates 2–3 carry duplicate/no-op step runs and
out-of-order commit labels — their wall-clocks are NOT comparable and are excluded from the cost
analysis (replicate 1 remains the clean dataset). But the collision produced two accidental
stress tests worth more than the lost comparability:

- **F12 — refine has no idempotency branch:** ~8 re-run agents found their exact ask already
  implemented and committed. The skill has no path for "the owning artifact already says what the
  ask requests" — every step from amend through re-review assumes the run makes a change. Agents
  improvised well (verified equivalence, recorded no-op, some used `--allow-empty` to honor the
  commit mandate), but each improvised differently. Fix: an explicit early exit — "if the
  requested state already holds, verify and report, touch nothing."
- **F13 — no crash-recovery story for an interrupted refine:** two clones were handed over with
  a half-finished refine in the working tree (amendment written + surface spliced, code NOT yet
  changed — the exact mid-flight state the session limit created). Nothing in refine, doctor, or
  the ledger checks describes how to recognize or resume/roll back a partial refine; agents had
  to forensically reconstruct what the dead agent had been doing. The ledger actually helped
  here (the uncommitted amendment file WAS the recovery record) — worth formalizing: doctor
  should detect "amendment present but unapplied AND working tree dirty" as an interrupted-refine
  state with a resume procedure.
- **F14 — skill claims AskUserQuestion works in refine:** the deployed skill's "Asking the user"
  section asserts the skill is always driven by a person; run as a subagent that is false and
  contradicts the CLAUDE.md subagent rule. Align the skill text with the decision-block protocol.
- **F15 — uniform `built_at` exposes the baseline-churn bug at the user level:** because
  save-build-state re-stamps every feature's baseline, all 42 features report the IDENTICAL
  `built_at` — the F2 engine bug is now visible in shipped JSON output, not just in noisy diffs.
- **F16 — enggspec has no precedence rule for contradicting co-equal artifacts:** the handoff
  author found intents/infrastructure asserting five phases while the amended surface says
  otherwise, with no rule for which wins (in ledger mode the contract artifacts should win — the
  projection guidance says so only implicitly). Also: producing specification.md for one feature
  creates a handoff-tree entry that tree-parity checks then count against every other feature.
- **F17 — source-root narrowing silently drops tracked files:** passing the adapter's narrower
  source-root writes a shrunken code-hashes file — out-of-scope entries vanish with only a WARN;
  and a pristine-HEAD save in one clone flagged 262 files as changed-outside-codegen (stale
  committed baselines), confirming the clones needed pre-baselining as a harness step.
- Late tail steps that ran clean confirm the L-series list without additions — L7 (dirty_set
  union vs diff) reproduced verbatim twice more, L4/L8/L9 each once more; the R5 ledger step
  (900s) and R4 ledger step (933–1022s) remain the slowest, consistent with rep1.

## Final — judges' verdicts (3 replicates) and consolidated conclusions

**R5, the deliberate reversal (3/3 judges agree):** both variants end unambiguous about current
behavior, but status quo's in-place re-edit **erases the decision trail** — no in-repo record
that `complete` ever existed; the journey lives only in git. The ledger's supersedes chain
preserves it and the projection surfaces it ("001 superseded by 005", net-effect note). The
replicate-3 pair adds the strongest quality datapoint of the whole run: **the status-quo leg
shipped a real regression** — `✓ done` leaked into `--quiet`, breaking its documented two-field
contract, *with a test blessing the broken output* and a self-contradicting handoff (FR-030 vs
FR-031) — while the ledger leg, holding amendment 002's recorded contract in view, scoped the
label to the human table only and documented why. The record didn't just describe quality; it
produced it.

**R6, the handoff (3/3 split verdict):** the ledger projection wins on provenance (regenerable,
History section, supersedes chain, `last-applied-amendment` stamp); the status-quo handoff wins
on code fidelity — it cross-checked the shipped Go and caught field bleed the projection missed.
The projection's failure mode is precise: **it trusts the frozen founding docs for vocabularies**
(claimed a five-value phase enum while code ships six) and never reads code. Fix in the
projection guidance: current truth = contract artifacts + a code cross-check; founding docs are
history only.

**New findings from the judges:**

- **F18 — supersedes chains are half-linked:** 005 supersedes 001, but 001 carries no forward
  pointer (read alone, it asserts stale behavior), and 002's acceptance also mentions the retired
  label yet was never superseded. Fixes: check-amendments should surface superseded-by links
  when reading, and search later amendments' scope against earlier Acceptance text (the L15
  wiring); the projection already partially compensates.
- **F19 — projection must be code-aware** (above).
- **F20 — no gate fires on buildfile-models vs contract divergence:** in BOTH variants, additive
  JSON fields left buildfile `models:` stale (FeatureEntry lacks `built_at`, envelope lacks the
  orphan count), caught only by human handoff authors. A validator comparing buildfile models
  against contract artifacts would close it.
- Judge-confirmed from step reports: L13 (the corrupted amendment 004, verbatim), P1/F5
  (surface.md vestigial and now actively contradictory in every clone, both variants), and the
  harness's own duplicate/empty commits in reps 2–3 (excluded from analysis).

## Overall verdict

1. **Record quality: ledger wins decisively** — 3/3 judges on reconstruction, plus the r3
   regression-prevention datapoint where the record functioned as a live guardrail.
2. **Cost: currently a wash with the truth bracketed** — pilot −14% vs rep1 +40%; the overhead is
   attributable, finding by finding, to the L-series implementation flaws, not to the model.
   Cost-of-Nth-refinement cannot be declared won until those are fixed and re-measured.
3. **The benchmark's real yield is the fix backlog:** ~20 distinct, actionable findings
   (F1–F20, L1–L18, P1–P3 resolved into them) spanning engine bugs (F2/F3/L4/L8/F17), skill/CLI
   mismatches (F1/L5/L6/F14), and model-level gaps (L13 ledger integrity, F18 supersedes
   linking, F19 code-aware projection, F12 idempotency, F13 crash recovery).

**Recommended order:** (1) safety: L13 + F13 + L8/L4 (corruption, crash recovery, silent state
loss); (2) honesty of the machinery: L7 dirty-set scoping + F20 models gate + F18 supersedes
links; (3) cost: F2 partial-baseline scoping (also fixes F15), F3 unattended coverage path,
retire surface.md (P1/F5); (4) then re-run this gauntlet — same asks, fresh clones — to measure
the model's true cost against a toolchain that isn't fighting itself.

Run totals: 39 agents, ~5.8M subagent tokens across both sessions, 6 clones, 36 gauntlet steps,
3 judges, every suite green at every completed step in both variants.

## WP10 re-run — replicate 1 interim (post-improvement toolchain)

Same six asks, pre-baselined clones, sq @ debb835 vs ledger @ becb9d4 (all WP1-WP9 fixes):

| Step | Status quo (tok/s) | Ledger (tok/s) |
|---|---|---|
| R1 replace | 37,286 / 565 | 34,683 / 567 |
| R2 add | 74,828 / 1,095 | 54,990 / 830 |
| R3 add | 44,169 / 691 | 47,574 / 784 |
| R4 add+drift | 69,439 / 1,067 | 71,127 / 1,029 |
| R5 contradiction | 36,408 / 493 | 63,062 / 963 |
| R6 handoff | 23,933 / 294 | 21,836 / 276 |
| **Total** | **286,063 / 70.1m** | **293,272 / 74.2m (+2.5%)** |

**The L-series fixes worked: +40% → +2.5%.** The ledger is now cheaper on the adds (R2 −26%)
and the replace; the entire remaining premium is R5, where the ledger does strictly more work
(supersedes chain + forward links vs silent re-edit) — the record's honest price, now isolated
from implementation friction. Frictions: 37 (sq) vs 31 (ledger), and the ledger's are
skill-wording issues rather than state damage. Pre-baseline stage validated WP1's fail-loud UX
live: the sq binary WARNed-and-committed a narrowing that would have silently under-scoped the
baseline to ~70 of 149+ files; the lg binary refused until given the correct root — no force
flag needed. Open residuals for the backlog: the `--source-root {root}` wording still
under-documents the multi-root child prefix (one MAJOR cross-root pollution report in r1-lg
R2); matrix golden files pin full `--json` output so any additive field reds the package;
`.emitted.consumed` rename costs a manifest-regen on re-runs (intended trade, now measured).
Note for the record: r2-sq R1 completed while the safety classifier was briefly unavailable —
its commit gets extra scrutiny in the judge pass. Reps 2-3 + judges pending (session-limit
interruption, resumed from cache).

## WP10 final — n=3 complete, judges in (2026-08-17)

**Cost: parity achieved.** Wall-clock across three replicates: sq 10,373s vs ledger 10,435s
(**+0.6%**; rep-by-rep +5.8% / −15% / +14% — inside noise). Rep-1 tokens +2.5%. The +40% of the
pre-fix run is gone. Frictions: 106 (sq) vs 91 (ledger), −14%, and different in kind.

**The fixes validated by adversarial reality, 3/3 judges:**
- **Baseline churn (F2): GONE in ledger** — 2 baseline files touched per commit vs sq's 43
  timestamp-only rewrites every single commit. WP1's write-if-changed + WP6's per-feature
  blessing, confirmed at n=3.
- **Silent provenance corruption: the status quo corrupted ITSELF during the benchmark** —
  r1-sq's end state carries 225/225 absolute machine-specific code-hash keys (one R5 save
  flipped the whole ledger non-portable) plus a mass-adopt of 263 entries; the ledger clone
  stayed 185/185 repo-relative and clean. WP1's boundary checks are the difference between a
  benchmark artifact and a corrupted project.
- **`.emitted` trap: gone** (`.emitted.consumed` maintained cleanly across code-authoring
  commits); **fail-loud narrowing** validated at pre-baseline — sq WARNed-and-committed an
  under-scoped baseline, ledger refused until given the correct root, no force flag.
- **R5 reversal: sq leaves no spec-level trace (3/3)** — the `complete` decision and its
  reversal exist only in git; the ledger's supersedes chain records it and the projection
  surfaces it.

**Residual backlog (the next improvement round, judge-sourced):**
1. **P1 is now urgent, 3/3 judges:** stale surface.md in ledger clones is actively
   self-contradictory (still says `complete`, no quiet/built_at). Retire it in ledger projects
   (migrate-spec as prerequisite) — the decision was already made; the benchmark just priced
   the delay.
2. **Forward supersession links need surfacing beyond check-amendments:** 001 carries no
   visible superseded_by where readers look (handoff should annotate; files stay immutable).
3. **Prose mirrors inside buildfile/testcases went stale after the reversal in BOTH variants**
   (quiet-token descriptions still say `complete`) — L14's full fix (replace-span must sweep
   embedded prose) remains open; sq's handoff disclosed it, the ledger's did not — add the
   disclosure to the projection guidance.
4. Minor: `orphaned_build_dirs` key overloaded at two JSON levels (both variants, inherited
   wart); two r2 steps ended blocked/partial on feature-gate refusals — honest outcomes, noted
   for ask-design.

**Bottom line:** the ledger model now matches the status quo on cost, beats it on frictions,
beats it decisively on record quality, and — the strongest single result of the whole program —
its state machinery preserved integrity under the exact conditions that corrupted the status
quo's. Exit criteria for making `ledger: true` the default: met on this evidence, pending the
residual-backlog items above.
