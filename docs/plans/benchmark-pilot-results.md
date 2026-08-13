# Ledger vs Status-Quo — Gauntlet Pilot Results

2026-08-13. Phase V pilot per [ledger-and-contract-plan.md](ledger-and-contract-plan.md) §Phase V.
One run per variant (n=1 — directional, not conclusive), three identical refinement asks against the
real dogfood feature `parlay-tool/status-feature-phases`, executed by independent Opus 4.8 agents in
two isolated clones, each following only its clone's deployed refine skill. Sequential execution for
clean per-step token attribution. Full Go suite run and green at every step in both variants.

- Status-quo clone: pre-ledger toolchain @ `debb835`
- Ledger clone: full ledger-and-contract implementation @ `eb58068`, `ledger: true`

## Asks

| # | Kind | Ask |
|---|---|---|
| R1 | replace-span | Human output shows phase `done` as `complete`; JSON keeps `done` |
| R2 | small add | `--quiet` flag: one `<id> <phase>` line per feature, no headers |
| R3 | small add | `--json` gains `built_at` from the baseline's generated-at, null when unbuilt |

## Cost per step (output tokens / wall seconds)

| Step | Status quo | Ledger | Ledger delta |
|---|---|---|---|
| R1 (replace) | 31,202 / 484s | 31,501 / 510s | ≈ parity |
| R2 (add) | 55,178 / 850s | 46,613 / 699s | **−15.5% tokens, −18% time** |
| R3 (add) | 58,058 / 830s | 46,442 / 779s | **−20% tokens, −6% time** |
| **Total** | **144,438 / 36.1 min** | **124,556 / 33.1 min** | **−13.8% tokens, −8% time** |

The shape matches the proposal's prediction exactly:

- **Replace-span is parity.** The narrative layer is untouched in both models for an inert tighten,
  so the ledger's advantage has nothing to bite on. Cost of the amendment file ≈ cost of the
  in-place note.
- **Adds are where the tax lives.** Status quo's R2 spread the record across four narrative/spec
  documents — new intent in intents.md, dialog scenes in dialogs.md (added because check-coverage
  flagged the new intent as uncovered), and both surface forms. The ledger's R2 wrote one amendment
  plus surface.yaml. That narrative-sync work is the entire measured difference.
- **The growth curve differs.** Status quo climbed 31.2k → 55.2k → 58.1k (+86% by R3); the ledger
  flattened at 31.5k → 46.6k → 46.4k (+47%, then flat). Cost-of-the-Nth-refinement is the primary
  metric, and at N=3 the curves have already separated.

## End-state quality (judge verdict, condensed)

- **Code: parity.** Near-identical implementations (same helpers, same reuse of the tolerant
  walkers, same explicit-null `built_at`), both suites fully green, slight test-thoroughness edge
  to status quo (one extra edge-case test).
- **Reconstruction record: ledger wins clearly.** Three dated amendments give trigger, affected
  fragment, delta, rationale, acceptance — no git archaeology. In status quo, R1/R3 survive only as
  note-bullets indistinguishable from founding text; "what changed and why" requires git blame.
- **Current-state prose: status quo wins.** The ledger clone's surface.md and dialogs.md are now
  stale by design (frozen), and surface.md still describes the old surface — a newcomer starting
  there gets an outdated picture. This is the reconstruction-vs-current-prose trade the proposal
  predicted; the projection generator (`parlay handoff`) exists for exactly this and was not
  exercised by the pilot.

## Findings beyond the numbers

1. **Underdetermined-spec divergence (headline surprise):** the same ambiguous ask (R2: which
   token does `--quiet` print?) was resolved oppositely — status quo chose the machine token
   (`done`), the ledger chose the human relabel (`complete`) — each self-consistently specced,
   coded, and tested, the ledger's choice argued in its amendment. Not a defect of either variant;
   a measurement of how much a three-line ask underdetermines behavior.
2. **Ledger mode should retire or regenerate legacy prose:** leaving a frozen-but-contradictory
   surface.md in place is worse than deleting it. Action: in ledger projects, `migrate-spec`
   (surface.md → surface.yaml) should be treated as a prerequisite, and/or the projection should
   be regenerated after each apply.
3. **Neither variant records small asks in testcases.yaml** — R1/R3 were covered only at the
   Go-test level in both. Pre-existing gap, not a ledger regression; worth a look independently.
4. **Comparability caveats:** n=1 per variant; the clones' baselines differ by the ledger
   infrastructure itself; build-state churn (baseline/code-hashes rewrites) was comparable noise
   in both.

## Verdict and next step

Directionally, the inversion does what it was designed to do: **the narrative-sync tax on adds is
real (~15–20% of step cost here), the ledger removes it, and the record it produces is the more
reconstructable of the two** — at the cost of stale legacy prose, which the projection generator
is built to cover and the pilot did not exercise.

The full benchmark (plan §Phase V exit criteria) needs: n≥3 runs per variant, a longer gauntlet
(10–15 steps, including cross-feature and shared-source asks), the projection step exercised, and
the finding-2 fix applied first. Total spend for the pilot: ~933k subagent tokens, 79 min
wall-clock, 7 agents.
