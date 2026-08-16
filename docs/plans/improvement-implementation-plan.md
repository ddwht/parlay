# Improvement Implementation Plan

Implements the decisions in [improvement-solutions.md](improvement-solutions.md), grounded in a
code-level analysis of this checkout (2026-08-16). Ten work packages (WP1–WP10) mapped to the
consolidated execution order; every anchor below was verified against the source, not inferred.

## Ground rules (carried from the ledger plan, plus three new ones)

1. Source-first dogfooding: embedded skills/schemas → `make build-noui` → `./parlay upgrade` →
   `make verify-skills`; DIGEST regenerated after any schema edit.
2. Test loop: `CGO_ENABLED=0 go test -tags noui ./...` green before every commit.
3. **Meta-test lockstep:** the embedded audit/conformance suites pin schema sentences. Notably
   `audit_test.go:240-280` currently pins the *stale half* of the buildfile-schema
   contradiction (the literal "not yet the accepted shape" survives only inside a sentence
   retracting it). Any schema reconciliation must update its pins in the same commit.
4. **`knownUnimplementedCodes` only shrinks.** Documented-but-unimplemented codes are either
   implemented or removed from the schema — never newly excused.
5. Work continues on `ledger-and-contract` (the integration branch); WP boundaries are commit
   boundaries so a PR can be split later.

---

## WP1 — Fail-loud state boundary + write-if-changed (Theme 2 A+B) · size S–M

The save flow is three stages in `saveProjectBuildState` (save_build_state.go:181): per-feature
baselines for ALL features (:189-223), project baseline (:225-240), project code-hashes
(:242-360), then the manifest is consumed (`os.Remove`, :362-367). Atomicity is per-file only
(`writeFileAtomic`, :430) — the "commit the full project atomically" doc claim (:174) is
aspirational; do not rely on it.

1. **Explicit-but-missing manifest → error.** `loadEmittedManifest` (:399-401) returns an empty
   declaration for an explicit `--emitted` path that doesn't exist (deliberate, comment
   :381-386). Change: explicit path + `IsNotExist` = hard error naming the path and the
   consumed-manifest cause. The no-flag case keeps its existing loud WARN (:294-298) — the
   asymmetry is principled: explicitly naming a missing file is always a caller mistake.
2. **Manifest consumed by rename, not delete.** `os.Remove` (:362-367) → rename to
   `.emitted.consumed`. The loader never reads the consumed name; the error message from (1)
   points at it so a re-run diagnosis is one `ls` away.
3. **Source-root vs stored key shape.** Keys are formed verbatim from the walk root
   (`marker.Path = path`, parser/marker.go:106; `ScanGenerated`, marker.go:208), and manifest
   lookups normalize both sides (`normalizeWriteSetPath`, check_write_set.go:362; used at
   save_build_state.go:413 and code_hashes.go:270) — but nothing checks the two prefixes agree.
   Insert after `loadProjectCodeHashes` (:256): if the previous snapshot is non-empty, compare a
   stored key's prefix against the passed `--source-root`; mismatch = error naming both and the
   expected invocation. Also stop discarding the previous-snapshot load error (`_` at :342) —
   at minimum warn.
4. **Scope shrink refuses.** Detection exists (`filesDroppedBySourceRootNarrowing`,
   code_hashes.go:137; warned at save_build_state.go:344-350). Promote to error unless a new
   `--allow-narrowing` flag is passed, mirroring the strict adopted-gate at :328-330.
5. **Write-if-changed baselines.** Between `marshalBaseline` (:204) and `writeFileAtomic`
   (:213): load the existing baseline, zero `GeneratedAt` on both (the struct's only volatile
   field, baseline.go:39 — verified), compare; skip the write when equal. Do NOT attempt the
   same for code-hashes in this WP: freshly-emitted entries also stamp `EmittedAt`
   (code_hashes.go:272), so a naive compare always differs — that file's churn is addressed by
   WP6's scoping instead.
6. **Feature-skip visibility.** The stage-1 loop silently `continue`s features whose
   `buildBaseline` errors (:192-195). Collect and print a skipped-features summary.
7. **Amendment post-write validation** (kills L13's entry path): refine.skill.md step 3.5 gains
   "after writing, re-read the file and run `parlay validate --type amendment` on it; a parse
   or shape failure means the write is corrupt — fix before proceeding."

Tests (save_build_state_test.go): explicit-missing-manifest errors; prefix-mismatch errors;
narrowing refuses without the flag; unchanged baselines byte-identical after a save (the F2
regression test); changed feature still written; skipped features reported.

## WP2 — Reconciliation sweep (Theme 3 A) · size M

1. **Shared buildfile reader.** The canonical v2-aware resolution is
   `deepBuildfile.resolvedComponents()` (agent/validate.go:328-336, switching on
   `AdapterSet != ""`). Export a helper (e.g. `agent.ResolveBuildfileComponents(content)`)
   built on it, and rewrite `parseBuildfileRefs` (check_coverage.go:317-344 — reads top-level
   `components:` ONLY; the confirmed BP1 break) to use it. Then audit the other own-struct
   parse sites the survey found — notably the section hashers `hashBuildfileSections`
   (baseline.go:637-687) and `hashMergedBuildfileSections` (diff.go:703-766), plus
   emission_groups.go:115, composition_flow.go:99, check_composition.go:155,
   agent/composition_seed.go:59 — and record a per-site verdict (v2-aware / v1-only-broken /
   v1-only-correct-because-X) in the commit message. Fix the broken ones via the shared helper.
2. **Schema reconciliation.** buildfile.schema.md:27-31 ("not what the validator accepts
   today… is rejected") is stale — the code sides with line 17 (`ValidateBuildfile`,
   agent/validate.go:51-63, accepts v2 when `adapter-set:` + resolvable presentation adapter).
   Rewrite :27-31 to match; leave :213 (consistent with the code); update the audit_test.go:240
   pin in the same commit (ground rule 3). Add a one-line "which reading won and why" note.
3. **`dirty_set` scoping (L7).** check_amendments.go: read the feature baseline's
   `LastAppliedAmendment`; `dirty_set` = refs from amendments with `Seq >` it; full union moves
   to a new `all_affects` key. Update amendment.schema.md's dirty-set section and refine
   step 3.5's agreement language to reference the tail.
4. **Amendment ordering for ADDs (L2).** Refine step 3.5 reorders: write amendment → apply the
   splice (step 4) → then `check-amendments`; the decision gate already covers both texts
   before anything is written, so nothing is lost by validating after apply.
5. **Coverage-step no-op branch (F3).** Refine step 10 + generate-code gate docs: when no
   coverage-review.yaml exists AND `check-review-gate` reports ready, record "skipped: gate
   inactive" and proceed; the unattended stance (record-skipped-with-reason) becomes the
   documented path instead of an improvisation.
6. **Command-surface sweep.** Fix bare `check-drift` mentions (refine step 11; the lint misses
   them because `parlayVerbPattern` requires a literal `parlay ` prefix —
   skill_cli_verb_lint_test.go:36); remove the phantom `--json` on `check-buildfile`
   (build-feature.skill.md — the command emits JSON unconditionally); document the `.emitted`
   path format (keys are walk-root-prefixed, e.g. `core/...` in multi-root); note modules
   resolve at the parent root; fix refine's AskUserQuestion claim (F14); add "rebuild the
   binary before smoke-testing" to refine.

Tests: v2-shape buildfile fixture through check-coverage (the BP1 regression); tail-scoped
dirty_set; audit pin updated.

## WP3 — Collision detection tier (Theme 5 A) · size S–M

All three lints are new *warning* codes (severity table + schema docs + DIGEST).

1. **Page+region sharing** — `surface-region-shared`: in `validatePageReferences`
   (validate.go:568-620), which already holds the full cross-feature fragment set from
   `ScanAllSurfaces` (surface.go:107-154): group by (page, region); more than one *feature*
   contributing → warning naming both features and fragments. Exact (page, region, order)
   duplicates get a sharper message — `assembleRegions` (view_page.go:197-205) is the
   precedent and stays as the view-time reporter.
2. **Shared-concept listing** — `infrastructure-concept-shared`: new
   `parser.ScanAllInfrastructure` modeled on `ScanAllSurfaces`, reusing
   `ParseInfrastructureFile` (parser/infrastructure.go:31-94); group fragments by normalized
   `Affects`; a concept constrained by >1 fragment (especially cross-feature) prints one line
   per concept at `validate --project` and in doctor's survey.
3. **Amendment-scope overlap** — `amendment-scope-overlap` (the L15/F18 wiring):
   check_amendments.go flags a later amendment whose `affects:` intersects an earlier
   amendment's, when the earlier one is not in its `supersedes:`; and the JSON output gains a
   computed `superseded_by` reverse map so forward links exist at read time without touching
   the immutable files.

## WP4 — Criterion-driven testcases (Theme 1 A+B+E) · size M–L

Ground truth that reshapes this WP: the enumeration gates are weaker than documented.
`testcases-route-uncovered`/`-flow-uncovered` are schema-only (excused at
conformance_test.go:75-76); `testcases-operation-uncovered` exists
(validate_testcases_v2.go:239-244) but its ONLY caller passes `canonicalOperations = nil`
(validate.go:795) — it has never fired. The flip to criterion-driven coverage is therefore
greenfield, not surgery.

1. **Schema** (testcases.schema.md, case template at :21-42 — today a case is only
   name/description/steps): add per-case `criterion:` ({ref: `@feature/<kind>:<name>`, text:
   pinned criterion}), `exercises:` (targets the steps must mutate), `observes:` (targets the
   expectations read), `coverage: full | state-only`.
2. **Validator** (validate_testcases_v2.go): replace the loose `[]map[string]yaml.Node` case
   decode in `validateSuiteCases` (:96) with a typed struct (today only
   `caseStepShape{Action,Verify,Target}` exists, :88-92 — `value`/`expected` aren't decoded).
   New codes: `testcases-case-vacuous` (no declared exercise appears among step targets),
   `testcases-case-claims-unmet` (expectation reads outside `observes:`),
   `testcases-case-criterion-missing` (warning during transition).
3. **Feed and flip the coverage gate:** pass real canonical operations at validate.go:795
   (the plumbing the comment at :785-792 deferred), and add `verify-criterion-uncovered` —
   every `verify:` entry on the feature's contract artifacts needs a case whose `criterion.ref`
   matches, or an exemption. Remove the never-implemented route/flow-uncovered rows from the
   schema and delete their `knownUnimplementedCodes` entries (ground rule 4).
4. **build-feature.skill.md:** derive cases 1:1 from `verify:` entries; fill
   criterion/exercises/observes at authoring; stamp `coverage: state-only` when a
   display-shaped criterion compiles to store-level assertions (E). Suite-per-component
   enumeration language is replaced by criterion coverage.
5. **Backstop lints** (audit-time, for pre-existing testcases): the vacuous/claims checks run
   in authoring AND build mode; the fixture-aware unsatisfiable/divergence checks are deferred
   to WP9 where fixtures gain structure.

## WP5 — Conformance ratchet (Theme 3 B) · size M

Extend the proven suites (conformance_test.go machinery: `repoSchemaCodes` :107,
`goSourceCorpus` :155; embedded iterators `ReadAllSkills`/`SchemaNames`/`ReadSchema`).

1. **Bare-verb lint:** `parlayVerbPattern` only matches `parlay <verb>` forms — bare
   `` `check-drift` `` escapes. Add a second pass: backtick spans that exactly match a
   registered *internal-only* subcommand name, presented without the `parlay internal` path,
   fail with the full invocation in the message. Conservative matching + a small ignore-list.
2. **Flag conformance:** for each `parlay <verb> [<sub>]` match, extract adjacent `--flag`
   tokens and assert `Flags().Lookup` finds them on that command (kills the phantom-`--json`
   class).
3. **Reader allowlist:** a test enumerating every file that unmarshals buildfile YAML with its
   own struct (grep the comment-stripped corpus for buildfile-shaped yaml tags), asserting
   each site is in an allowlist with a recorded reason — the matrix `excused` pattern
   (matrix_test.go:764-772) applied to parsers, so WP2's audit can't silently regress.

## WP6 — Per-feature blessing (Theme 2 C) · size M

Staged behind WP1 by decision. The seams found: per-feature baselines already carry independent
`GeneratedAt`s (safe); the carry-forward classifier row (code_hashes.go:277) is the mechanism
for non-emitted files; complications are the flat project-wide code-hashes map
(code_hashes.go:33), single project-level `GeneratedAt`s (diff.go:751, code_hashes.go:32), and
the scanner walking the whole tree regardless of feature (save_build_state.go:243-245 — though
`marker.Feature` filtering exists, code_hashes.go:241).

1. Under `--partial`, stage 1 writes baselines ONLY for features in the emitted set (manifest
   paths → features via `marker.Feature`) plus any explicitly named feature; all others keep
   their existing baselines — **dirty flags survive** (the BP6 fix).
2. The project baseline records `emitted: [slugs]` per save for audit.
3. `diff`/`check-drift` audit: per-feature reads are already independent; verify no reader
   assumes a shared instant across features; document per-feature instants in
   schema-versioning notes.
4. The BP6 regression test: partial save advances only the emitted feature; an un-emitted
   dirty feature still reports dirty afterward.

## WP7 — Decisions block + rationale propagation (Theme 4) · size M

1. **buildfile `decisions:`** (buildfile.schema.md): list entries {id, component, decided, why,
   enforced-by: [files], obsolete-when, supersedes}. Preservation follows the `wiring:`
   precedent ("designer-authored rules, preserved verbatim"): build-feature.skill.md carries
   the same verbatim-preserve instruction for `decisions:`; generate-code.skill.md reads the
   block for affected components BEFORE regenerating (zero read-set widening — the buildfile is
   already in codegen's allowlist) and appends a record for every judgment call it makes.
2. **Propagation check** — `rationale-stranded` (warning): for elements carrying `rationale:`
   or a `deliberate:` marker, the emitted files named by their plan rows must contain the
   decision id; lexical check at check-buildfile/verify-generated time. Scoped to explicitly
   marked elements only.

## WP8 — Composition vocabulary + appears (Themes 5B, 1C) · size L

1. Surface schema + parser: `supersedes: @feature/fragment` and `interactive: false` on
   fragments; occupancy escalation: WP3's `surface-region-shared` warning becomes
   `surface-region-conflict` (error) when neither `supersedes:` nor a page-manifest ordering
   resolves multiple occupants; two-headed supersedes chains are errors (mirroring duplicate
   amendment sequence numbers).
2. Codegen: supersedes-aware routes composition (the composed result, not parallel files) —
   per-adapter; `interactive: false` emits non-hit-testable output.
3. Testcase vocabulary: `appears` action at `mounted | output | content` levels, adapter
   capability-gated (`adapter-supports-missing-step` machinery); derived per-page assembly
   suite (mounted + hit-reachable/not) generated by build-feature from what the surface already
   declares; `state-only` stamps lift where `appears` becomes available.

## WP9 — `relation:` + computed expectations + fixture oracle (Theme 1 C/D) · size L

Design-doc-first (own decision gate before implementation): structured `relation:` expressions
on infrastructure fragments; fixture fields carry `derived-from:`/`asserted:`; validator
recomputes fixtures and expectations against relations; the WP4 backstop lints gain their
fixture-aware halves (unsatisfiable, divergence-from-source).

## WP10 — Re-run the gauntlet

Same six asks, fresh clones, n≥3 — with the harness fixes the last run taught: pre-baseline the
clones (the stale-July-baseline churn), verify resume-from-cache replay before relying on it,
and keep replicate 1 sequential for clean token attribution. Exit criterion for the ledger
flag's default-on decision per the original plan.

---

## Order and dependencies

| WP | Depends on | Unlocks | Size |
|---|---|---|---|
| 1 | — | 6 | S–M |
| 2 | — | 5 (reader allowlist), 3 (clean baseline hashers) | M |
| 3 | — (2 helpful) | 8 (escalation path) | S–M |
| 4 | — | 8 (state-only ↔ appears), 9 | M–L |
| 5 | 2 | — | M |
| 6 | 1 | 10 (honest cost measurement) | M |
| 7 | — | — | M |
| 8 | 3, 4 | 10 | L |
| 9 | 4 | — | L |
| 10 | 1–6 minimum | the cost verdict | harness exists |

Suggested commit train: WP1 → WP2 → WP3 → WP4 → WP5, then WP6/WP7 in either order, WP8/WP9 as
capacity allows, WP10 when 1–6 are in.

## Risks

- **WP1.3's prefix check** depends on a previous snapshot existing; first-ever saves have
  nothing to compare — fall back to the WP2.6 documented convention, don't guess.
- **WP2.1's hasher audit** may find v2 projects have had empty section hashes all along —
  fixing that changes hash values and will dirty baselines project-wide once; land with a note
  and a re-baseline step, and do it BEFORE WP6 makes baselines precious.
- **WP4 flips a gate** from enumeration to criteria; pre-existing projects with empty
  `verify:` fields would go from silently-unfed to loudly-uncovered — the migration path is
  `parlay migrate-verify` (already shipped) plus `testcases-case-criterion-missing` staying a
  warning for one release.
- **WP6 relaxes the one-instant invariant** — the BP6 regression test and the false-stable
  scenario tests are the gate for that commit, not optional.
- **Meta-test pins** (audit_test.go, conformance excuses) must move in the same commits as the
  claims they pin — three separate places were found where a pin currently enforces a stale
  sentence.
