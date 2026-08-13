# Implementation Plan: Ledger + Contract

Implements [ledger-and-contract-proposal.md](ledger-and-contract-proposal.md) — inverting parlay's
source of truth so the four spec artifacts are the always-current contract, intents/dialogs freeze
as founding documents, and change is recorded in append-only amendment files, with the bundled
incrementality upgrades (per-suite coverage staleness, declared dirty-sets, affected-test selection).

Status: planned, not started. 2026-08-13.

---

## Ground rules for every phase

1. **Source-first (dogfooding rule).** All skill/schema changes are made under
   `core/internal/embedded/{skills,schemas}/`, then `make build`, then `./parlay upgrade`
   (or `make sync-skills`), verified with `make verify-skills`. Never edit
   `.claude/skills/parlay-*/SKILL.md` or `.parlay/schemas/*` directly.
2. **DIGEST is derived.** After any schema edit, regenerate the schema digest
   (`schema_digest.go` path) so `.parlay/schemas/DIGEST.md` lists the new error codes.
3. **Versioning policy** (`schema-versioning.schema.md`): tool-generated artifacts
   (buildfile, testcases, capabilities) evolve by regeneration; hand-touched or snapshot
   artifacts (baseline, domain-model, authored) evolve by version bump + migrator.
4. **No breaking existing projects silently.** Every on-disk shape change ships either a
   migrator (surfaced by doctor's pending-migrations detection, run with `--dry-run` first)
   or graceful degradation (missing new field ⇒ old behavior).
5. **Each phase lands independently.** Phase 0 is valuable with no inversion; Phases 1–3
   each leave the repo releasable. Dogfood every phase on parlay-dev's own specs before release.

## Rollout switch

Phase 1 changes refine's behavior for every parlay project. To keep the inversion opt-in
during the transition, gate it behind a project-level flag (working name `ledger: true`)
read from project config (`core/internal/config`). Refine/loop/doctor branch on it:

- flag off → today's behavior (splice intents/dialogs, full narrative sync);
- flag on → append-and-apply flow below.

parlay-dev sets the flag as first dogfood consumer. The flag is removed (ledger becomes the
only mode) after the Phase V gauntlet validates the model. **Open decision:** exact config
surface for the flag — existing project config vs. `.parlay/adapter-set.yaml` is wrong for
this; resolve when Phase 1 starts.

---

## Phase 0 — Decision-proof groundwork

Valuable even if the inversion never ships. No behavior gate needed.

### 0.1 Relocate acceptance criteria: intent Verify bullets → `verify:` on contract artifacts

- **Schemas** (`core/internal/embedded/schemas/`):
  - `capabilities.schema.md`: add optional `verify:` (list of strings) per operation, and
    optional `rationale:` (one line). Document: verify is the source testcase assertions are
    derived from; rationale is provenance, never parsed.
  - `surface.schema.md`: same two fields per fragment.
  - `testcases.schema.md`: document that assertions derive from the owning operation/fragment
    `verify:` entries; intent **Verify** bullets become fallback-only (removed in Phase 1 when
    the flag is on).
- **Skills** (`core/internal/embedded/skills/`):
  - `create-artifacts.skill.md`: when generating capabilities/surface, copy the intent's
    Verify bullets into `verify:` on the operations/fragments they describe.
  - `build-feature.skill.md` (testcase step): derive suite assertions from `verify:` on the
    artifact first; fall back to intent Verify bullets only when `verify:` is absent.
- **Migrator** (Go, follows the `migrate_*.go` pattern in `core/internal/commands/`):
  - New `migrate_verify.go` + test: for each built feature, copy intent Verify bullets into
    `verify:` on the operations/fragments whose `source:` points at that intent. Idempotent,
    `--dry-run` supported.
  - Register in doctor's pending-migrations detection (`doctor.skill.md` migration list):
    detected when capabilities/surface lack `verify:` while intents carry Verify bullets.
- **Tests:** unit tests beside the migrator; extend `validate_*` tests for the new fields;
  build-feature fixture asserting assertions come from `verify:` when present.
- **Dogfood:** run the migrator over `core/` and `studio/` roots; spot-check 2–3 features
  (suggest: `parlay-tool/status-feature-phases`, `studio-support/page-layout-field`).

**Done when:** testcase generation no longer needs to read intents for assertions on any
migrated feature; `make verify-skills` clean; migrator idempotent on second run.

### 0.2 Per-suite coverage staleness

Today `review_coverage.go` stores whole-file `BuildfileHash`/`TestcasesHash`, and
`check_review_gate.go` compares them — one changed suite stales the entire review.

- **Go:**
  - `review_coverage.go`: additionally store a content hash per suite
    (`suites: {<suite-id>: <hash>}`) computed from the suite's canonical YAML subtree.
  - `check_review_gate.go`: when per-suite hashes are present, report staleness per suite —
    only suites whose hash moved (or which are new) are unapproved; untouched suites keep
    their approval. When absent (old files), degrade to today's whole-file comparison.
    Emit which suites are stale in `reviewGateOutput` so the skill can walk only those.
- **Schema:** `coverage-review.schema.md`: document the per-suite hash map; per the
  versioning policy this file stays version-free (graceful degradation covers old files).
- **Skills:** `refine.skill.md` step 10 and `loop.skill.md` review step: re-review only the
  suites the gate reports stale.
- **Tests:** `review_coverage_test.go` / `check_review_gate_test.go`: one-suite change stales
  exactly one suite; old-format file stales everything; approval survives for untouched suites.

**Done when:** a single-suite amendment in dogfood (`studio` root has the only
coverage-review files today) re-prompts for exactly that suite.

---

## Phase 1 — The inversion (behind the `ledger` flag)

### 1.1 Amendment artifact + schema

- **New schema** `core/internal/embedded/schemas/amendment.schema.md`:
  - Location `spec/intents/<feature>/amendments/NNN-<slug>.md`; frontmatter
    `amendment` (slug), `date`, `trigger`, `affects:` (list of `@feature/operation:x`,
    `@feature/surface:x`, `@feature/infrastructure:x`, `@feature/domain:x` refs),
    `supersedes:` (amendment slugs); body sections `## Change`, `## Why`, `## Acceptance`.
  - Delta-shaped by rule: an amendment may not restate a full operation/fragment definition —
    only deltas and refs (validator enforces the cheap approximation: no `operations:`/YAML
    blocks that parse as complete artifact entries).
  - Error codes: `amendment-affects-unresolved`, `amendment-supersedes-unknown`,
    `amendment-out-of-sequence` (NNN gap/duplicate), `amendment-missing-acceptance`
    (warn — an amendment that changes behavior but relocates no verify text).
- **Go:**
  - Parser support in `core/internal/parser` for the amendment file shape.
  - New `check_amendments.go` (+ test) under `parlay internal`: validates a feature's ledger
    (codes above), resolves `affects:` refs against capabilities/surface/infrastructure/
    domain-model, and prints the declared dirty set as JSON (consumed in Phase 3).
  - `validate.go`: register `--type amendment`.
  - Regenerate DIGEST.
- **Ownership docs:** CLAUDE.md file-ownership section (embedded template + deployer):
  amendments are designer-authored, append-only, never edited once written.

### 1.2 Refine v2 — append and apply

`core/internal/embedded/skills/refine.skill.md`, ledger-flag branch:

- Step "record": instead of splicing intents.md/dialogs.md, draft `amendments/NNN-<slug>.md`,
  decision-gate the exact file content, write it. Founding docs are never touched.
- Step "apply": splice the delta into the affected contract artifacts (same splice discipline
  as today, artifacts only); land `## Acceptance` items as `verify:` entries on the affected
  operations/fragments.
- Delete the dialog re-sync step in this branch (`scaffold-dialogs.skill.md` keeps its role
  for feature *birth* only).
- Routing (step 2) reads contract `source:` refs and the amendment ledger, not intent prose.
- Feature-gate (step 2.5) unchanged in spirit; "needs its own dialogs conversation" still
  routes to `/parlay-loop`.
- Everything from diff onward unchanged (rebuild-if-added, scoped codegen, signatures, full
  suite, `save-build-state --partial`, coverage re-review — now per-suite from 0.2).

### 1.3 Freeze founding documents + re-anchor drift

`core/internal/commands/baseline.go`:

- Keep `Sources.Intents`/`Sources.Dialogs` hashes in `Baseline` but **reinterpret** them under
  the flag: they become *freeze hashes*, not drift inputs. `detectDrift`/`runCheckDrift` stop
  reporting intent/dialog changes as feature drift when the flag is on; instead a mismatch is
  surfaced as a **ledger-integrity** finding (founding doc edited after freeze). This reuses
  the existing storage — no baseline shape change for it.
- Add `LastAppliedAmendment string` to `Baseline`; `save-build-state` records the highest
  amendment applied in that run. Bump the baseline schema-version (comments in baseline.go
  show v1→v2 history; this is v3) — baseline is regenerate-on-save, so no migrator, but old
  baselines missing the field must read as "no amendments applied yet".
- Amendment files themselves are hashed into the baseline (`Sources.Amendments map`), enabling
  `amendment-file-mutated` detection.
- Tests in `baseline_test.go`: flag on — intent edit ⇒ integrity finding, not drift; amendment
  append ⇒ no drift until applied; apply + save ⇒ `LastAppliedAmendment` advances.

### 1.4 Loop and downstream skills

- `loop.skill.md`: ledger branch — check-drift gate reads the new semantics; the "spec
  contradiction" check anchors on contract artifacts; domain-model contributions documented
  as one kind of amendment (mechanics unchanged).
- `build-feature.skill.md`: under the flag, reads contract artifacts + (for birth) founding
  docs; testcase assertions come exclusively from `verify:` (fallback removed on this branch).
- `generate-code.skill.md`: no change (already fenced from spec/intents).

**Phase 1 done when:** on parlay-dev with the flag on, a real refinement (pick a small live
ask) runs end-to-end producing an amendment file, an updated contract artifact, a green build,
and `check-drift` clean — with intents.md/dialogs.md byte-identical before/after.

---

## Phase 2 — Stewardship

### 2.1 Doctor learns the ledger

`doctor.skill.md` + Go support:

- **Unapplied amendments:** ledger contains amendments beyond `LastAppliedAmendment`
  (from `check_amendments.go` + baseline read). Repair offer: run the apply step.
- **Compaction advisor:** flag a feature when amendment count exceeds a threshold (start: 8)
  or `supersedes:` chains touch the same `affects:` ref more than twice. Advice only.
- **Ledger integrity:** founding-doc or amendment hash mismatch vs. baseline (from 1.3).
  Repair offer: restore from git, or bless-and-refreeze with explicit confirmation.
- Drop the standing intent↔dialog coverage check on the ledger branch (`check_coverage.go`
  stays for birth-time use by the loop and for flag-off projects).

### 2.2 Projection generator (compaction, cheap level)

- `generate-enggspec.skill.md` / `generate_enggspec.go`: repurpose `parlay handoff @feature`
  to generate `spec/handoff/<feature>/specification.md` from contract artifacts + ledger
  (founding docs + amendments in order) — the derived "current state in prose". Header marks
  it generated + regenerable; never hand-edited. (Zero of these files exist today, so this
  is a repurpose with no migration.)

### 2.3 Re-founding (compaction, ceremonial level)

- New loop/doctor-invoked flow (skill-level, no new binary command initially): generate a
  fresh founding intent set from the current contract, freeze it, mark the feature's baseline
  `compacted-through: NNN`, move the old ledger to `amendments/archive/` (retained, never
  deleted). Decision-gated at every step; rare by design.

### 2.4 Housekeeping commands learn the zone

- `repair.go`: three-tree reconciliation treats `amendments/` as part of the spec tree
  (moves with the feature; never reported as orphan). `move_feature.go` preserves it on
  rename. Tests for both.
- `upgrade.go`/deployer: nothing to deploy (amendments are project-owned), but upgrade's
  CLAUDE.md template gains the ledger ownership text (1.1).

**Phase 2 done when:** doctor on parlay-dev reports a deliberately-staged unapplied amendment
and a tampered founding doc; `parlay handoff` regenerates a faithful specification.md for a
multi-amendment feature.

---

## Phase 3 — Performance (optional, flag-independent where possible)

### 3.1 Declared dirty-sets with trust-but-verify

- `diff.go`: accept the declared set from `check_amendments.go` for the amendment being
  applied; compute the hashed diff as today; **disagreement raises** (new code
  `amendment-affects-mismatch`) instead of proceeding — a mismatch means the amendment or the
  apply step is wrong.
- `refine.skill.md`: scope the *prompts* — build/codegen steps load only dirty components and
  their neighbors (buildfile sections by component, not the whole 2,500-line file). This is
  skill-text work, no Go change.

### 3.2 Affected-test selection with unconditional backstop

- Affected set = declared dirty set + transitive dependents via explicit refs
  (`composition_*.go` machinery already resolves cross-feature operation/flow refs — expose
  a `parlay internal affected-set @feature` that walks it; new command + test).
- `refine.skill.md`: interactive runs execute the affected set; document loudly that the
  **full suite remains an unconditional CI/nightly gate** and refine's final report must say
  which mode ran. Ship a `make`/CI target for the full-suite gate in the same change — the
  backstop must exist before the fast path does.
- Explicit go/no-go: if the team rejects weakening "never bless untested code" from per-run
  to per-day, drop 3.2 and keep 3.1 — they are independent.

---

## Phase V — Validation gauntlet (runs after Phase 0, gates Phases 2+ becoming default)

- Harness: seed project + scripted sequence of 10–15 refinements/features, identical across
  variants; run n≥3 per variant (agent runs are noisy). Orchestrate with the session Workflow
  tooling; store transcripts + metrics per run.
- Variants: (a) status quo, (b) ledger flag on.
- Primary metric: tokens per refinement step and its growth curve (cost of the Nth refinement).
- Secondary: files touched per change, doctor/drift findings per step, test pass rate,
  end-state quality review — explicitly probing whether agents mis-read superseded ledger
  content (the model's specific comprehension risk).
- Exit criteria to make ledger the default and remove the flag: (b) beats (a) on the primary
  metric with no regression in end-state quality; no unresolved comprehension failures.

---

## Order of work and sizes

| # | Work | Depends on | Size |
|---|---|---|---|
| 0.2 | Per-suite coverage staleness | — | S |
| 0.1 | Verify relocation + migrator | — | M |
| V(pilot) | Gauntlet harness, status-quo baseline runs | 0.x | M |
| 1.1 | Amendment schema + parser + check_amendments | 0.1 | M |
| 1.2 | Refine v2 (append+apply, flag branch) | 1.1 | M |
| 1.3 | Baseline reinterpretation + LastAppliedAmendment | 1.1 | M |
| 1.4 | Loop/build skill branches | 1.2, 1.3 | S |
| 2.1 | Doctor ledger checks | 1.3 | M |
| 2.2 | Projection generator | 1.1 | S |
| 2.4 | repair/move/upgrade zone support | 1.1 | S |
| 2.3 | Re-founding flow | 2.1, 2.2 | M |
| V(full) | Gauntlet: ledger vs status quo | 1.x | L |
| 3.1 | Declared dirty-sets + prompt scoping | 1.1 | M |
| 3.2 | Affected-test selection + CI backstop | 3.1 | M |

Suggested first PR train: 0.2 → 0.1 → gauntlet pilot. Nothing after that starts until the
pilot harness runs clean, so the Phase 1 variant can be measured from its first commit.

## Risks tracked against the plan

- **Flag drift:** two behavior branches in refine/loop/doctor until Phase V resolves — keep
  branches small and textual in skills; delete the flag promptly after the gauntlet.
- **Delta-shape enforcement is approximate** (1.1): the validator can only cheaply reject
  obvious restating; the schema prose and refine's decision gate carry the rest. Revisit if
  dogfood amendments bloat.
- **infrastructure.md apply rules** (1.2): prose splice is the least mechanical apply — write
  explicit splice rules into the refine skill; if they don't hold up in dogfood, consider
  fragment-ids for infrastructure.md as a follow-up.
- **Baseline reinterpretation** (1.3) must not confuse flag-off projects: freeze semantics
  strictly behind the flag; `baseline_test.go` covers both modes.
- **CLAUDE.md template churn:** upgrade overwrites project CLAUDE.md — the ledger ownership
  text must go into the embedded template, not just parlay-dev's copy (known dogfooding trap).
