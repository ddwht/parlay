# Token-cost reduction — scope the reads, digest the schemas, short-circuit the no-op

Status: PLANNED — target v0.4.x (no behavior change for project state; every
workstream changes what agents *read*, not what the tool writes).

## Decision

Three workstreams, in landing order:

- **WS-A** — refine gets a deterministic pre-flight that ends no-op runs
  before any module loads, and a step journal so an interrupted run resumes
  instead of restarting.
- **WS-B** — the code phase's read-set is scoped by the diff: buildfiles are
  read per-feature on demand, and the two genuinely cross-feature needs
  (merged routes, plan presence) move into compact CLI emits.
- **WS-C** — the schema corpus gains derived per-schema *authoring digests*
  (the DIGEST move applied to authoring), and the build/code modules load
  those instead of seven full schemas.

Evidence basis: a read-set audit of the deployed modules (numbers below) and
the WP10 benchmark's friction ledger. The audit's headline: **the CLI is not
the token sink.** Every command emits name-scoped JSON that grows with
counts, never with file content. The cost is concentrated in whole-file read
mandates in the phase modules — and it currently scales with *project* size,
not *change* size, which is the wrong asymptote for a tool whose pitch is
that the Nth refinement stays cheap.

## Where the tokens actually go (audit result)

Measured on the dogfood core root (22 built features), bytes on disk,
~4 bytes ≈ 1 token:

| Read | Mandated by | Size |
|---|---|---|
| ALL buildfiles, every built feature | `generate-code.md:161` step 4 | ~542 KB (largest 103 KB, median ~21 KB) |
| ALL testcases at the test step | `generate-code.md:482` | ~421 KB (largest 44 KB, median ~19 KB) |
| 7 schemas whole, build phase | `build-feature.md:102` step 1 | ~186 KB |
| 3 schemas whole, code phase | `generate-code.md:152` step 1 | ~132 KB (buildfile 56 KB + adapter 59.5 KB load in BOTH phases) |
| Framework adapter YAML | both phases, step 1/3 | ~25 KB, twice |
| Phase module itself | code 85 KB, build 56.5 KB, designer 35 KB | fixed per phase |

A single code-phase run on the dogfood core ingests ≈ 1.2 MB (~300k tokens)
before any generation happens; most of it describes features the change does
not touch. By contrast every CLI emit inspected (`diff`, `check-coverage`,
`check-buildfile`, `check-readiness`, `domain-impact`) is a list of names
and finding records — compact by construction.

The benchmark's cost findings point the same way: the L-series fixes already
took the ledger leg from +40% to +0.6% wall-clock at n=3, and what remains
is either honest work (R5's contradiction recording) or the read overhead
this plan targets. Two frictions are directly in scope: **F12** (refine has
no idempotency branch — ~8 re-run agents found the ask already implemented
and each improvised a different no-op path at 30–70k tokens per run) and
**F13** (no crash-recovery; a resumed session re-dispatched completed
steps).

## WS-A — refine: no-op pre-flight and step journal

Smallest, fully self-contained, lands first.

1. **New command `parlay internal check-applied @feature`** — one cheap call
   combining what step 0 needs: `check-drift`'s verdict, the unapplied tail,
   `last-applied-amendment`, and — new — the ledger's frontmatter index
   (`[{seq, slug, date, trigger, affects}]`, ~200 bytes per amendment, never
   the bodies). Deterministic facts only; whether the *ask* matches an
   applied amendment stays a judgment call, but it becomes a judgment over
   frontmatter lines instead of over a fully-loaded refine context.
2. **refine step 0** — run `check-applied` before reading anything else.
   Clean state + an amendment whose slug/trigger plausibly matches the ask →
   present the match, and on confirmation stop with a canonical
   "already applied as NNN-<slug>" report. One sanctioned no-op exit instead
   of eight improvised ones (F12).
3. **Step journal** — `.parlay/build/<feature>/.refine-journal.yaml`
   (tool-internals zone, same precedent as `.emitted`): the skill stamps
   step boundaries — `amendment-written: NNN`, `splice-applied`, `rebuilt`,
   `emitted`, `tested`, `re-baselined`. Step 9's save clears it. A refine
   invoked while a journal exists resumes at the first incomplete step —
   and, because the journal names the amendment file, a resumed run cannot
   write a duplicate 002 for the same ask (F13, L16's double-apply cousin).
4. Tests: conformance pin for the new command; a journal-resume test
   (interrupt after amendment-written → resume must not re-write); skill
   lints stay green.

## WS-B — code phase: read what the diff names, not the directory

The biggest lever, and the one that fixes the scaling shape. The module
already orders preserve-stable-verbatim for *writes*; this extends the same
discipline to *reads*.

What "load ALL buildfiles" actually buys today (audit of steps 4–6):

- the merged `routes:` dispatch table (step 5),
- the `plan:`-presence hard stop (step 4),
- context that the diff (step 8) then tells the agent to mostly ignore.

Stages:

1. **CLI: make the cross-feature facts a compact emit.**
   - New `parlay internal merged-routes` — reads every buildfile's `routes:`
     section in Go, emits the merged table with per-route provenance
     (`feature`, `path`, blueprint join fields). Sibling of
     `scaffold-seed`/`check-composition`, which already prove the pattern:
     cross-feature coherence computed deterministically, emitted as names.
   - `parlay internal diff` (project mode) additionally reports
     `missing_plan: [feature…]`, so the step-4 hard stop needs no buildfile
     reads.
2. **Module: reorder and scope.** `generate-code.md` runs the project diff
   *first*. Buildfiles are then read only for features with
   `dirty`/`removed` components (plus `first_build` features); the merged
   routes come from the CLI; stable features' buildfiles are never opened.
   Same at the test step: read `testcases.yaml` only for features being
   regenerated.
3. **Escape hatch, stated in the module.** When generation surfaces a
   cross-feature contradiction the scoped set cannot explain, the agent
   widens to the *named* feature's buildfile — an explicit escalation,
   never a silent fallback to load-everything.
4. **Fallback preserved.** No project baseline (first generation) or a diff
   that errors → the current full-load path, verbatim.
5. Tests: conformance pins for `merged-routes` and the new diff field; a
   matrix note (mutation-adjacent commands stay out of the verdict
   surfaces); module/skill lints.

Expected effect: a one-feature change on the dogfood core drops from 22
buildfiles + 22 testcases to 1–3 of each — on the order of 200k tokens per
code run — and the saving grows with project size, which is the point.

## WS-C — authoring digests: the DIGEST move, applied to the phases

`DIGEST.md` already proved the shape: 337 KB of validation corpus → a 21 KB
mechanically-derived routing table, regenerated at build time so it cannot
drift. The build/code phases still pay ~186 KB / ~132 KB of full schema
prose per run, most of it rationale and history an authoring agent needs
only when something goes wrong.

The digest generator's own comments name the prerequisite: closed
vocabularies were deliberately NOT extracted because "the phrasings vary too
much across schemas for a line-level heuristic; extracting them properly
needs the schemas to mark their closed sets in a parseable way, which is a
schema change rather than a digest change." That schema change is stage 1.

1. **Normative markers.** A fence convention in the schema sources —
   `<!-- parlay:normative -->` … `<!-- /parlay:normative -->` — around field
   tables, closed vocabularies, and invariants. Added schema-by-schema,
   reviewed; the heterogeneous section structure (18 `##` headings in
   adapter.schema.md, 8 in capabilities.schema.md, no shared shape) is why
   this cannot be inferred.
2. **Extractor.** `BuildAuthoringDigest` beside `BuildSchemaDigest`: per
   schema, emit `<name>.digest.md` = title + purpose + the marked normative
   blocks, verbatim. Deployed to `.parlay/schemas/digests/` by
   init/upgrade, pruned like schemas.
3. **Module read-lists swap.** `build-feature.md` step 1 and
   `generate-code.md` step 1 point at digests, with the DIGEST discipline
   stated: open the full schema only when a validator finding or an
   authoring question routes you there.
4. **Anti-drift ratchet.** A conformance test asserts every error code and
   every marked block in a schema appears in its digest (trivially true —
   derived), and a size ratchet keeps each digest under a fixed fraction of
   its schema, so rationale prose cannot migrate inside the fences over
   time.

Expected effect: ~150 KB → ~30–40 KB of schema context per pipeline run
(~30–40k tokens), every run, plus the same per-phase saving on the adapter
double-load (load once, reference).

## Explicitly out of scope

- Re-litigating ideas the improvement round already rejected: CLI refs
  generated from the cobra tree, a contract registry, mutation testing,
  adversarial expectation passes.
- The R5 premium (honest contradiction recording at ~2×) — a chosen cost,
  not overhead.
- Test-suite narrowing defaults in refine step 8 — stays an explicit
  project-level opt-in, per the skill's existing doctrine.
- Prompt-cache/harness-level optimizations — outside parlay's control.

## Sequencing note

WS-A first (small, self-contained, kills a measured friction class). WS-B
second (CLI emit + module reorder; the largest saving). WS-C third (touches
every schema source, widest review surface). Each workstream is
independently shippable and independently measurable: the feedback-mode
tally plus a before/after refine and code-phase run on the dogfood core is
the acceptance measurement for each.
