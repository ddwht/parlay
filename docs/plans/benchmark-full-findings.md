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
