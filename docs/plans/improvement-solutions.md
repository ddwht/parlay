# Improvement Solutions — decisions record

Working through the five themes distilled from the two findings corpora
([benchmark-full-findings.md](benchmark-full-findings.md), block-printing `findings.md`),
one theme at a time: the problem in user-experience terms, root causes, options considered,
and the decision. Decisions are appended as they are made with the designer.

---

## Theme 1 — "Green must mean verified" · DECIDED 2026-08-16

### The problem, as the user experiences it

The pipeline reports all-green; the user ships or refines on top of it. Later the product is
broken — a blank viewport, a violated output contract — and the suite was green the whole time,
in one case including a test that asserted the broken behavior. From then on no green can be
trusted, which destroys the toolchain's core promise.

### Why the pipeline authors such tests (root causes, not symptoms)

1. **One author, one pass:** the build phase invents a case's steps, expectations, and often
   its fixture values together, with no independent source for any of them. Self-agreement is
   treated as correctness; the cases that were caught were caught only because self-agreement
   *accidentally* failed.
2. **The claim is prose, the mechanics are structure, nothing connects them:** a case's name
   says "widening the wall…" (unchecked text) while its steps are the machine-readable part —
   they can diverge silently, and did, in four distinct shapes.
3. **Coverage gates count existence, not meaning:** every gate asks *is there a suite*, none
   asks whether its steps touch the subject and its expectations read the result. A vacuous
   suite satisfies the gate; a missing one fails it — the generator is structurally pushed
   toward quantity (the ceremony-test problem).
4. **Fixtures are invented literals with no oracle:** nothing derives fixture values from
   declared relations or a real engine, so arithmetically impossible fixtures pass every gate
   until the engine becomes real.
5. **Some claims are inexpressible, so they silently compile to weaker ones:** "the viewport
   shows the mesh" has no vocabulary, so it becomes "the store holds the mesh" with no record
   that a downgrade happened.

### Options considered

- **A. Criterion-driven derivation** — cases generated 1:1 from `verify:` acceptance criteria
  on the contract artifacts; coverage gates flip to "every criterion has a case."
  Kills cause 3; every case has a stated, reviewable reason to exist. Risk: coverage quality
  moves upstream to the `verify:` review (accepted — that is the human-reviewed layer).
- **B. Machine-readable claims** — cases declare `exercises:` (fields/actions the steps must
  mutate) and `observes:` (fields the expectations read); the validator refuses at write time
  a case whose mechanics don't match its declaration. Kills cause 2; vacuity becomes
  unauthorable rather than merely detectable.
- **C. Independent expectation derivation** — expected values computed from fixture + steps
  via declared `relation:` formulas (or a real engine), not authored. Only cure for cause 1.
- **D. Fixture oracle** — fixture values carry `derived-from:` a relation/engine call;
  hand-set values need an explicit `asserted:` marker. Only cure for cause 4.
- **E. Expression-gap honesty** — when a display-shaped criterion compiles to store-level
  assertions (no `appears` support yet), the case is stamped `coverage: state-only` and the
  coverage reviewer sees it. Stops the silent downgrade of cause 5 without vocabulary work.
- **Two-agent adversarial expectation pass** — rejected: pays inference cost to approximate
  what declared relations do deterministically.
- **Mutation testing** — rejected: expensive, slow, framework-bound; wrong instrument when the
  information is statically available.

### Decision

**A + B now, with E as the transitional honesty marker.** One design, three parts: a test
exists because a criterion demands it (A), states its claim checkably (B), and admits what it
could not express (E). **C and D staged behind the `relation:` field** — they are the same
investment (structured relations) seen from two sides and the only cure for causes 1/4, but
A+B+E cover most observed defects without them. The audit-time lints (unsatisfiable case,
vacuous case, assertion/fixture divergence, models-vs-contract gate) remain as the **backstop**
for pre-existing testcases — net, not process.

### The `appears`/render design (the vocabulary fix behind E, staged)

Scope line: assert **mounted → produced output → content reached the renderer** (scene
graph/DOM facts, e.g. rendered row count or triangle count vs the current fixture); pixels
(A4/snapshots) deliberately rejected. Two homes, because the composition defects (no mount
point; a sibling capturing input) are invisible to any per-component check:

1. **Derived assembly suite, one per page** — generated with no author in the loop from what
   the surface already knows: every declared component `mounted: true`; components with
   `actions` hit-reachable; output-only fragments (new surface field `interactive: false`)
   NOT capturing input — the flag also lets the adapter emit non-hit-testable output, making
   the defect impossible, not just visible.
2. **Explicit `appears` step** in the testcase vocabulary at levels
   `mounted | output | content`, used by criterion-driven cases; capability-gated per adapter
   (existing `adapter-supports-missing-step` machinery), with `coverage: state-only` stamped
   wherever the adapter can't deliver it yet.

Evidence mapping: cache-serves-stale-mesh → `content` level; unmounted component and dead
input → assembly suite; importer-never-presents → `mounted`. The store was correct in all
four — no state assertion could have caught any of them.

---

## Theme 2 — the `save-build-state` family · under discussion

## Theme 3 — toolchain self-disagreement · pending

## Theme 4 — where rationale lives · pending

## Theme 5 — composition arbitration · pending
