# Proposal: Ledger + Contract

**Inverting parlay's source of truth — amendment files as the change ledger (Variant B), with incrementality upgrades**

Status: discussion draft — no project changes made. 2026-08-13.

---

## 1. Summary

Parlay currently maintains intents.md and dialogs.md as always-current sources of truth, and every refinement pays an alignment tax to keep the narrative layer in sync with the artifacts, build state, and code. This proposal inverts that authority:

- The **four spec artifacts** (surface, capabilities, infrastructure, domain-model) become the authoritative, always-current **contract**.
- **intents.md / dialogs.md** freeze at feature birth as founding documents — never rewritten again.
- All subsequent change is recorded as **append-only amendment files** — a new first-class artifact — whose deltas are *applied* to the contract artifacts.
- The `.parlay/build/` zone keeps its exact mechanics, and gains three incrementality upgrades that the amendment model makes safe: per-suite coverage staleness, declared dirty-sets with hash verification, and affected-test selection with a full-suite backstop.

## 2. Why (evidence from this repo)

Three findings from a deep read of the pipeline, schemas, and git history:

1. **The hard mechanical spine already excludes intents/dialogs.** Codegen is forbidden from reading `spec/intents/`. The `source-signatures:` freshness gate — the only hard gate — hashes surface, domain-model, layout, capabilities, and infrastructure. Not intents, not dialogs. No validator checks the narrative docs semantically against anything downstream; their only ties are advisory baseline hashes and warning-level `source:` back-references.
2. **The specs are already de facto append-only.** Across 221 commits, no spec file was touched more than 5 times. Growth is additive (multi-root's big refinement: intents +124/−2, dialogs +345/−2). Amendment patterns already emerged organically (`## Deferred:` sections, appended `**Note**:` blocks) — designers append history when left alone.
3. **The churn that does exist is the always-current tax.** The commits touching many spec files at once are symmetric micro-edits (+4/−4 across 11 files) propagating vocabulary renames — retro-editing historical documents to speak today's language.

The proposal moves the official source-of-truth line to where the enforcement line already sits.

## 3. Layer dispositions

| Layer | Disposition | Change |
|---|---|---|
| intents.md, dialogs.md | **Ledger — frozen founding documents** | Immutable after first build; tool refuses post-build edits. Still authored normally at feature birth. |
| `amendments/NNN-<slug>.md` | **Ledger — new first-class artifact** | One file per change, append-only by construction. See §4. |
| capabilities.yaml, surface.yaml, *.page.md | **Contract — always current** | Primary edit targets for refinement. Gain `verify:` (relocated acceptance criteria) and optional `rationale:`. |
| domain-model.yaml (root) | **Contract — always current** | Unchanged; contribution files become one kind of amendment. Migrator chain stays. |
| infrastructure.md | **Contract — always current** | Kept in the signature gate; apply-step needs explicit splice rules so it doesn't drift into a second narrative ledger. |
| buildfile, testcases, baselines, code-hashes, design-spec | **Derived — machine-current** | Mechanics unchanged; three targeted upgrades (§6). |
| coverage-review.yaml | **Derived — machine-current** | Staleness granularity fixed: per-suite hashes (§6.1). |
| specification.md (handoff) | **Repurposed — compaction target** | From dead layer (zero instances today) to derived narrative projection, regenerated on demand, never hand-edited. |
| authored.yaml, adapters, blueprint, adapter-set | **Unchanged** | Orthogonal to this proposal. |

## 4. The amendment artifact

Location: `spec/intents/<feature>/amendments/NNN-<slug>.md`. Files are numbered sequentially and **never edited once written** — immutability enforced by construction (every change is a new file), not by discipline.

```markdown
---
amendment: list-status-field          # slug, unique within feature
date: 2026-08-13
trigger: "@export needs report status to decide exportability"
affects:
  - "@reports/operation:list-reports"
  - "@reports/surface:report-row"
supersedes: []                        # amendment slugs this replaces, if any
---

## Change
`list-reports` output gains `status` (enum: draft | final | archived).
The report row surface shows a status badge.

## Why
Export must skip drafts; status was previously implicit in `finalized_at`.

## Acceptance
- Listing shows one status badge per row, matching the entity state.
- `list-reports` output includes `status` for every item.
```

Lifecycle:

1. **Author** — refine drafts the amendment, decision-gates it with the user (exact content shown before writing).
2. **Apply** — the delta is spliced into the affected contract artifacts; the `## Acceptance` items land as `verify:` entries on the affected operations/fragments.
3. **Build onward** — identical to today: diff → scoped rebuild → codegen → tests → save-build-state → coverage review.

Design constraints that keep amendments healthy:

- **Delta-shaped by schema.** An amendment describes a change, never restates a feature. The schema should make restating awkward (no full operation definitions, only deltas and refs) to prevent amendments becoming a second spec dialect.
- **Cross-feature provenance is first-class.** `trigger:` records *why* — including "because feature X needed it," the causal link that today lives nowhere.
- **Vocabulary renames become one amendment**, not N symmetric micro-edits: the domain-model owns current naming; ledger entries keep the vocabulary of their time.

New validator codes (sketch): `amendment-affects-unresolved` (an `affects:` ref names no existing operation/fragment), `amendment-supersedes-unknown`, `amendment-out-of-sequence`, `amendment-unapplied` (see §5, doctor), `amendment-file-mutated` (a previously-hashed amendment changed — ledger integrity check).

## 5. Pipeline changes by phase

**Design phase (new features): unchanged.** Intents and dialogs are still authored conversationally at birth; artifacts are still generated from them; the first build freezes them.

**Refine (the phase that changes most):**

| Step | Today | Proposed |
|---|---|---|
| Resolve feature, feature-gate | judgment on intent prose | unchanged in spirit; routing uses contract `source:` refs + amendment history (refs are now permanently stable) |
| Record the change | in-place splice of intents.md/dialogs.md; no record a change happened | append `amendments/NNN-<slug>.md` (decision-gated) |
| Sync narrative | re-sync dialogs per intent change | **deleted** |
| Amend spec | splice contract artifact | unchanged (apply step; same splice discipline, artifacts only) |
| Scope the diff | inferred by re-hashing everything vs baseline | **declared** by `affects:`, verified against the hashed diff — disagreement raises, it's a bug signal in the amendment or the apply |
| Rebuild / codegen / re-stamp | component-incremental | unchanged |
| Tests | full suite, always | affected-set from the declared dirty set + explicit composition refs; **full suite retained as unconditional CI/nightly backstop** (§6.3) |
| Re-baseline | save-build-state --partial | unchanged; baseline **drops intents/dialogs hashes**, records `last-applied-amendment` per feature |
| Coverage re-review | whole review stales on any change | only suites whose per-suite hash moved (§6.1) |

**Drift (check-drift / diff):** re-anchors on contract → build → code. A ledger append dirties nothing until applied. `shared_sources_changed` behavior for domain-model/adapter is unchanged.

**Doctor gains three checks:**

- **Unapplied amendments** — an amendment exists beyond `last-applied-amendment` in the baseline: the ledger says a change was decided but the contract never received it.
- **Compaction advisor** — flags a feature when amendments exceed a threshold or `supersedes:` chains indicate contradiction; recommends compaction (§7).
- **Ledger integrity** — founding docs or past amendments modified after freeze.

Doctor *loses* the standing intent↔dialog coverage check (title-matching, false-positive-prone); coverage becomes a birth-time check in the design phase.

**Handoff:** `parlay handoff @feature` becomes the compaction/projection generator (§7).

## 6. Incrementality upgrades (bundled in)

These fix the places that re-do everything today. The first is independent of the ledger; the other two are made safe *by* it.

### 6.1 Per-suite coverage staleness — genuinely should be incremental

Today `coverage-review.yaml` pins whole-file `buildfile_hash` + `testcases_hash`, so touching one suite stales the entire review and forces a full re-walk. Approvals are already per-suite; only the staleness check is file-grained. **Change:** store a content hash per suite; a review is stale only for suites whose hash moved. Best effort-per-payoff fix in the whole flow, zero interaction with anything else, could ship first.

### 6.2 Declared dirty-sets with trust-but-verify

Today the dirty set is inferred after the fact (re-hash everything, compare to baseline). An amendment declares it up front via `affects:`. **Change:** build/codegen scoping consumes the declared set; the hash comparison runs anyway and any disagreement between declared and observed diff raises instead of proceeding. Also scope the *prompts*, not just the writes — the agent loads only dirty components plus their neighbors instead of a 2,500-line buildfile; tokens, not machine steps, are where feature rebuilds actually cost.

### 6.3 Affected-test selection with an unconditional backstop

Today refine runs the full suite always ("blessing untested code is the one thing the build state must never do") — correct as a guarantee, expensive as a default. Parlay can compute a real affected set because the cross-feature dependency graph is explicit (operation refs, `composition-*` refs, flow declarations). **Change:** the interactive refine loop runs the affected set (declared dirty set + transitive composition dependents); the **full suite remains an unconditional gate in CI/nightly**, so a bug in affected-set computation delays detection but can never permanently bless untested code. Note honestly: this weakens "never bless untested code" from per-run to per-day for the interactive path. If that trade is unacceptable, keep full-suite in refine and take only §6.1 and §6.2.

## 7. Compaction

Ledgers rot without it (amendment 30 half-reverses amendment 12; agents resurrect dead decisions). Two levels:

- **Projection (cheap, routine):** regenerate `spec/handoff/<feature>/specification.md` from the contract artifacts + ledger — the always-derivable "current state in prose" for humans and for agent onboarding. Never hand-edited, therefore never a sync obligation.
- **Re-founding (rare, ceremonial):** when the doctor's compaction advisor fires, squash: generate a fresh founding intent set from the current contract, freeze it, and mark the feature `compacted-through: NNN`. The old ledger is retained (archived, never deleted) — compaction adds a summary, it doesn't erase history.

## 8. What deliberately does not change

- The codegen isolation fence (buildfile as the only intermediate) — the proposal aligns with it rather than touching it.
- The signature gate, plan allowlist, hand-authored denylist, `save-build-state` atomicity.
- Domain-model root editing rules and migrator chain.
- Adapters, blueprint, adapter-set, authored.yaml.
- The design phase for *new* features — conversation-first authoring stays; this proposal only changes what happens after birth.

## 9. Migration path

Phased so each step is independently valuable and reversible:

- **Phase 0 — decision-proof groundwork** (valuable even if the inversion never ships):
  relocate intent **Verify** bullets into `verify:` fields on capabilities/surface; point testcase generation at them; per-suite coverage hashes (§6.1).
- **Phase 1 — the inversion:** amendment schema + validator codes; refine v2 (append + apply); freeze existing intents/dialogs as-is (current state = founding state — the churn audit shows they already effectively are); drop intents/dialogs hashes from the baseline.
- **Phase 2 — stewardship:** doctor's unapplied-amendment / compaction-advisor / ledger-integrity checks; the specification.md projection generator; `parlay upgrade`/`repair` learn the amendments zone.
- **Phase 3 — performance (optional):** declared dirty-set scoping and prompt scoping (§6.2); affected-test selection with CI backstop (§6.3).

## 10. Risks and open questions

- **Reconstruction cost.** Understanding a feature now means founding docs + N amendments. Compaction is therefore *required infrastructure*, not hygiene — Phase 2 is not optional if Phase 1 ships.
- **Dialect drift.** Amendments must stay delta-shaped or they become a competing spec language. The schema must enforce this, not a style guide.
- **infrastructure.md apply rules.** Splicing prose is the least mechanical apply; needs explicit rules or it becomes a second narrative ledger inside the contract.
- **Refine routing changes character.** Today it routes by reading intent prose; after inversion it routes via contract refs + amendment history. Probably better (stable refs), but it alters a judgment-call step — exactly what the gauntlet should measure.
- **Cross-feature cost is not reduced.** A change to a feature's contract still triggers its rebuild, tests, and review. The proposal removes the narrative tax, not the mechanical cost of real change (§6 reduces, doesn't remove).
- **Giant accretion features** (multi-root: 2,573-line buildfile) are not fixed — but amendment density per feature finally provides the signal for when to split one.

## 11. Validation

Race this variant against the status quo empirically before committing past Phase 0:

- One seed project + a scripted gauntlet of 10–15 refinements/features, identical across both variants, run repeatedly (agent runs are noisy — single runs are anecdotes).
- Primary metric: **cost of the Nth refinement** (tokens per step and its growth curve) — the failure mode being fixed is degradation with accumulation.
- Secondary: files touched per change, doctor/drift findings per step, test pass rate, and an end-state quality review — including whether agents mis-read superseded ledger content, the specific comprehension risk of the ledger model.
- Phase 0 is decision-proof and can ship before the race settles anything.
