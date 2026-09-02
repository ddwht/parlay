# Backlog support in parlay — proposal

**Status**: Reconciled draft, 2026-09-01. Independently drafted by Claude and Codex,
then synthesized. Every code claim below was verified against the tree at `8c6a312`.

---

## 1. The problem

Two symptoms, one missing capability.

**Discoveries during implementation vanish.** Mid-build you find something real — a
defect, a gap the spec never covered, a shortcut you took knowingly — that is not the
highest priority right now. Today it is said in a conversation and then it is gone.
Parlay records the code, the decisions enforced in the code, the drift, the coverage
judgments and the amendment history. It records nothing about work it *noticed and
did not do*.

**Unfinished features carry no statement of why.** You can already park an idea by
creating a feature and leaving it incomplete. That works, and it is the accidental
backlog this project runs on. What is missing is any declaration of whether the pause
was chosen.

### The evidence is in this repo

`parlay gate --all` against `core` closes with:

```
7 passed, 23 blocked, 17 not yet gated
```

Those 17 print as `— (no boundary yet)` and are indistinguishable from one another.
Among them:

- `features-and-initiatives-renaming` — 4.9 KB of intents, a 54-byte stub `dialogs.md`.
  A deliberate placeholder whose deferral is recorded only in its own intent prose:
  *"feature renaming is reserved for a later pass."*
- `parlay-tool/studio-cli-hooks` — 10.5 KB of dialogs, two parsed dialog blocks,
  untouched for months. Paused? Superseded? Nothing on disk says.

Seventeen undifferentiated lines in the command meant to say whether the project is
healthy. `feature_phase.go` already states the cost of this, in its rationale for
`PhaseHandAuthored`:

> *Reporting a permanent non-problem is how a status line stops being read.*

Separately, `docs/plans/benchmark-full-findings.md` carries a section headed
**"Residual backlog"** — real tracked work in a markdown file no parlay command can see.

---

## 2. Two orthogonal axes

The two symptoms are **not** the same object, and collapsing them loses information.

- **Maturity** — how far the feature has come: `planned → intents → dialogs →
  artifacts → build → done`. Already modelled by `ComputeFeaturePhase`.
- **Activity** — whether the pause was chosen: `active | parked | unclassified`.
  Not modelled at all today.

A parked feature with 4.9 KB of intents is already expressed as intent; it is a
feature that is not being worked on, not a backlog item. A discovery with no home is a
backlog item. They need **one inventory and one promotion path, but two records**:

| | Backlog item | Activity declaration |
|---|---|---|
| Exists when | The work has no home yet | The work has a feature directory |
| Lives in | `spec/backlog/` | `spec/intents/<feature>/activity.yaml` |
| Answers | "What did we notice and not do?" | "Why is this feature paused?" |

`parlay backlog list` shows both, so there is a single place to look.

---

## 3. A phase bug this work must fix first

`parlay add-feature` eagerly writes **both** `intents.md` and `dialogs.md`
(`add_feature.go:164,169`), while `computeFeaturePhaseAtPaths` decides the dialogs rung
on **file existence alone**:

```go
hasDialogs := fileExistsAt(filepath.Join(featurePath, "dialogs.md"))
```

So a brand-new, entirely empty feature reports phase `dialogs` from the moment it is
created, and `PhaseIntents` is unreachable for any feature made this way. Empirically:
of 46 features in `core`, **zero** report `intents`.

**Fix — make the ladder content-aware:**

- `planned` — zero parsed intents. Detectable because the scaffold's example intent is
  commented out precisely so a new `intents.md` parses as zero intents
  (`add_feature.go:287`).
- `intents` — intents parse, dialogs do not.
- `dialogs` — real parsed dialog blocks. Then `artifacts`/`build`/`done` as today.

**Implementation trap:** a stub `dialogs.md` parses to JSON `null` with exit 0, not
`[]`. The content check must treat `null` as zero rather than propagate an error.

### What this fix does and does not clear

Measured against the 17:

| | Count |
|---|---|
| Have ≥1 parsed intent | **17 of 17** |
| Would become `planned` | **0** |
| Move `dialogs` → `intents` (stub dialogs.md, 44–61 bytes) | 9 |
| Stay `dialogs` (1–4 real parsed dialog blocks) | 8 |

The ladder gets more honest and **the inventory noise is unchanged** — still 17 lines
with no disposition, now split 9/8. `planned` fixes a real bug, but that bug is "a
brand-new untouched feature misreports", which affects features created from now on.
The activity axis is what clears all 17.

This measurement belongs in the rationale permanently: it is what stops a later reader
concluding the ladder fix solved the inventory problem.

---

## 4. Scoping principle

> **Parlay tracks work that has been observed but not yet expressed as intent.**

Admits: a defect found while generating code, a gap the dialogs never covered, an idea
nobody has asked for, a deliberate shortcut with a known better shape. Excludes: a
flaky CI job, a dependency bump — anything that could never resolve into a feature, an
amendment or an intent.

`parlay spec` renders what a feature promises **now**. The backlog is its exact
complement: what has been observed and **not yet promised**. Backlog descriptions stay
capture-and-triage notes; rich intents and dialogs belong only in features. The backlog
is not a second requirements system.

---

## 5. Design principle: ceremony scales with disposition, not with capture

Parlay is deliberately ceremonious where acts are hard to reverse. Amendments are
immutable and attributed because they change a frozen contract. `defer-legacy-exemption`
refuses to run without `--reason` and `--by`: *"a deferral nobody can attribute, about
nothing in particular, tells the next reviewer nothing they did not already know."*

A backlog item changes nothing. So:

- **Capture is cheap.** Cheaper to record the thing than to argue about it. No prompts,
  no priority, no triage at the point of capture. If capture costs a round trip to the
  user it will be skipped in exactly the situation it exists for — mid-implementation,
  inside a subagent that cannot ask anyone anything.
- **Closing is ceremonious.** Closing is where things get silently lost, so it carries
  a closed vocabulary, a required reason, and attribution.

---

## 6. Backlog items

```
<activeRoot>/spec/backlog/
  <ulid>-<slug>.yaml
  archive/
```

`spec/`, not `.parlay/` — the `.parlay/` zone rule is explicit that it is never
user-facing, and a backlog item is design intent a person reads and decides on.

**One file per item, with a collision-safe id** (ULID, or UTC timestamp plus short
suffix). Not sequential `NNN-` like `amendments/`: phase agents may record mid-run and
in parallel, and sequential allocation is a race. One file per item also means two
unrelated items never conflict on merge.

```yaml
schema_version: 1
id: 01K4M2QX7N8PZR-rename-collides-with-orphan
kind: gap                       # defect | gap | debt | idea
priority: P1                    # P0 | P1 | P2 — optional; absent means untriaged
title: "Initiative rename does not check orphan-feature collision"
body: |
  Renaming an initiative does not check for a colliding orphan feature in the
  top-level namespace. The collision surfaces later as a qualified-path lookup
  failure with no trace back to the rename that caused it.

captured:                       # immutable once written
  at: 2026-09-01T10:45:00Z
  by: claude
  run: 20260901T104500Z-4412
  feature: "@parlay-tool/multi-root"
  phase: code
  origin_root: core

about:                          # semantic parlay refs
  - "@features-and-initiatives-renaming/operation:rename-initiative"

evidence:                       # optional, structured, filesystem-level
  - path: core/internal/commands/move_feature.go
    line: 118
    detail: "no namespace check before rename"

history:                        # append-only
  - event: deferred
    reason: "Cannot tell whether this is still reachable after the v0.4 freeze."
    at: 2026-09-03T09:04:00Z
    by: dwht
```

The item is **mutable** — `title`, `body`, `priority`, `about` and `evidence` can be
corrected and enriched. Amendment immutability is priced for authority mutation; a low-authority
inbox note needs typo correction, and demanding a supersession record for a fixed typo
is ceremony the act does not warrant. What is *not* mutable: the `captured` block, and
`history`, which is append-only.

**Concurrency: reuse the existing lock pattern, do not invent compare-and-swap.**
`core/internal/atomicfile` provides atomic *replace* (write-temp + fsync + rename) and
`WriteIfChanged`, but no compare-and-swap and no locking — so a read-modify-write
through it is last-writer-wins. It also uses a fixed temp path (`path + ".tmp"`,
opened `O_TRUNC`), so two writers to the *same* path collide on the temp file as well
as on the target.

Distinct ids remove fixed-temp contention between ordinary concurrent creates because
they select distinct paths and distinct temp files. That does **not** make creation
safe by probability alone: exclusive create (or collision retry) below handles the
remaining identity-collision case. The fixed-temp behavior is not safe at all for
**appending to an existing** item's `history`, or for `activity.yaml`.

For those, use the pattern the project already runs in `domainmodel/save.go` and
`commands/authority_preflight.go`: a `github.com/gofrs/flock` file lock with a bounded
wait (5s) and retry (25ms), surfacing a typed busy error rather than blocking forever.
Lock scope is the single file being mutated, so unrelated items never contend. The
writer acquires the lock **before reading**, then validates, appends and atomically
replaces the file while still holding it. New-item creation uses exclusive create (or
retries with a new id on collision); a probable-unique id is not itself an exclusivity
guarantee.

`captured.run` is the `PARLAY_RUN_ID` the loop already mints and exports, so an item
ties back to the exact pipeline run that produced it at zero cost.

**`about[]` and `evidence[]` are distinct on purpose.** `about` holds semantic parlay
refs; `evidence` holds filesystem locations. Evidence is optional — a discovery made in
conversation legitimately has none — but a skill that knows an exact location must
supply it. CLI: repeatable `--evidence path[:line]`.

**`about` is optional too, and may legitimately point away from where the discovery
happened.** In the example above the item was captured in `@parlay-tool/multi-root` and
is *about* `@features-and-initiatives-renaming`. Both facts are true and neither is
redundant, which is why §9's designer read matches on either.

**`priority` is optional, and absent means untriaged.** This mirrors §7 exactly: no
file means undeclared there, no priority means unranked here. Absence is a fact about
the record, never a default smuggled in as one — so `backlog list` can put the
un-ranked items first, which is the pile that actually needs a person.

### Lifecycle is derived, never stored

There is no `state:` field. State is computed from `history`:

- **open** — no terminal event present.
- **promoted | amended | folded | declined | obsolete** — the single terminal event.

`deferred` is **non-terminal**. Deferral attempts accumulate and never change open
state — the accumulate-attempts property lifted from `defer-legacy-exemption`, where
two people independently unable to decide is a different fact from one attempt
overwritten twice.

At most one terminal event is allowed, it must be the final history entry, and MVP
allows no event after it. Reopening is a future schema change, not something inferred
from a later deferral. These invariants keep a terminal disposition terminal rather
than merely asking readers to choose the last terminal entry from a contradictory log.

Deriving rather than storing removes the failure every mutable status field eventually
has: a stored state that disagrees with the history that produced it.

| Terminal event | Meaning | Requires |
|---|---|---|
| `promoted` | Became a feature | `becomes: @feature` |
| `amended` | Became an amendment | `becomes: @feature/NNN-slug` |
| `folded` | Absorbed into another item | `becomes: <item id>` |
| `declined` | Deliberately not doing it | `reason` |
| `obsolete` | The condition that produced it is gone | `reason` |

`declined` and `obsolete` stay distinct for the reason `amendment.schema.md` keeps
`replaced` and `obsolete` distinct: *a reader months later cannot recover from silence
whether the work moved or stopped mattering, and that difference is the whole content
of the decision.*

---

## 7. Activity declarations

```
spec/intents/<feature>/activity.yaml
```

**Colocated with the feature, not centralised.** A central file keyed by qualified
feature id goes stale the moment `parlay move-feature` moves a feature between
initiatives — the key changes and the record does not follow. Colocation moves
atomically with the directory, has no cross-feature write hotspot, avoids branch-merge
conflicts between unrelated parkings, and makes "why is this parked?" answerable by
opening the feature.

```yaml
schema_version: 1
history:                        # append-only
  - event: parked
    reason: "Superseded by the shipped implementation; keeping intents for reference."
    until: "after adapter-set v2 lands"     # optional, free text
    at: 2026-04-18T09:12:00Z
    by: dwht
  - event: unparked
    at: 2026-09-01T11:00:00Z
    by: dwht
```

Current activity derives from the latest event, same rule as items. Two events only:
`parked`, `unparked`. No file means undeclared.

### Interpreting activity in status

| On disk | Reported |
|---|---|
| Latest event `parked` | `parked` |
| Latest event `unparked` | `active` |
| No file, but a passed or blocked boundary | `active` — there is observed pipeline activity |
| No file, no boundary | `unclassified` |

`unclassified` is a diagnostic about a **missing disposition**, not an inferred claim
that work has stalled. Nothing here reads mtime or git history: a checkout, a
migration or a bulk move all perturb timestamps, and inferring "stalled" from them
would be exactly the guess that `retires_feature:` refuses to make — a lifecycle
transition nobody chose is not one to infer.

`parlay park` rejects built and terminal features; parking is a pre-build act.

Parking and retirement are opposite ends of the lifecycle:

| | Parking | Retirement (`retires_feature:`) |
|---|---|---|
| Applies to | An unbuilt feature not yet finished | A built feature being closed |
| Says | Not now | Not ever, here |
| Record | `activity.yaml` | Terminal amendment |
| Reversible | Yes, `unpark` | No — supersession only |

---

## 8. Capture

```
parlay note --kind gap --title "..." [--body ...] [--priority P1] [--feature @X]
            [--phase code] [--about @X/operation:y] [--evidence path[:line]]
            [--root <root>]
parlay backlog edit <item> [--title ...] [--body ...] [--evidence path[:line]]
                           [--about ...] [--priority P0|P1|P2 | --clear-priority]
```

`backlog edit` is the supported correction/enrichment path for the mutable fields in
§6. It runs through the same per-item lock, permits only `title`, `body`, `priority`,
`about` and `evidence`, and refuses any attempt to replace `captured` or existing
`history`.

### The writer is strict; the caller is non-blocking

These are two different obligations and conflating them is a mistake:

- **The writer validates and is atomic.** Malformed input is rejected with a non-zero
  exit. This is durable, user-facing, schema-versioned state — not disposable
  telemetry — and exiting 0 on garbage would silently manufacture corrupt records.
  (`parlay internal feedback-record` is *not* precedent for leniency here: it validates
  closed fields before checking whether feedback is even enabled, and its payload is
  throwaway.)
- **The calling phase treats capture failure as non-blocking.** A note that fails to
  write must never fail the phase that tried. The phase surfaces the failure in its
  `notes:` report and carries on.

### Priority: the intent tokens, with the intent meaning

Backlog items use `P0 | P1 | P2` — the same closed vocabulary as `intents.md`.

Reusing those tokens is only safe if they keep their existing meaning, and they do.
`intent.schema.md` defines Priority as ranking *"the cost of leaving the USER OUTCOME
unmet — not implementation order, technical risk, or ease."* A backlog item is observed
work whose absence has a cost, and it is that cost the rank orders: P0 is work whose
absence is critical, not work scheduled first. Defining it this way is what resolves
the collision — the danger was never the tokens, it was attaching *scheduling*
semantics to a scale the project has already defined as impact.

The mapping is direct for `gap` and most `defect` items, which are unmet user outcomes
already. It is looser for `debt`, whose cost reaches a user only through the code
carrying it — the rank there answers the same impact question, but it is not itself a
user-facing promise.

So a rank **carries into promotion (§10) as a proposal, not an inheritance.** It
arrives at the intents phase in the vocabulary that phase already speaks, rather than
as a scheduling number nobody can map — but the phase confirms it still expresses the
cost of the target user outcome before it stands. A `debt` item ranked P0 for its blast
radius in the codebase is exactly the case that must be re-judged rather than copied;
letting it become an intent P0 unexamined, under the narrower definition, is the
failure this rule exists to prevent.

Two rules keep it honest:

- **Never inferred.** An agent sets `--priority` only when the user actually ranked it.
  A guessed priority is worse than an absent one: it looks like a judgment and is not.
- **Optional at capture, assigned at triage.** Capture stays cheap (§5) — the flag
  exists so "this one is urgent" is not lost when the user says it, and is omitted
  otherwise. Ranking the pile is triage's job, through
  `parlay backlog edit <item> --priority P1`. `--clear-priority` returns an item to
  untriaged: absent is a meaningful state (§6), so a mistaken rank must be reversible
  to it rather than only replaceable by another rank.

### The noise rule for automatic capture

Capture always writes — a `--dry-run` default would destroy the cheapness that is the
whole point. Noise is controlled by a narrow instruction rather than by friction:

> Capture a **concrete, evidenced piece of undone work**, or an explicit
> later/defer/out-of-scope statement from the user. Never capture speculation, generic
> suggestions, or work already recorded.

Because every boundary report lists the ids captured during that phase, noise is
observable rather than silent, and an over-eager agent is caught at the next
confirmation rather than three weeks later.

### The instruction half — without this, nothing changes

A CLI nobody is told to call does not get called:

- **`generate-code.skill.md`** — step 11's report gains a notes line. This is where
  discovery is most common and the context most disposable.
- **The `parlay-decision` block** gains an optional `notes:` field, so a phase reports
  what it filed as part of stopping.
- **`loop.skill.md`** — the driver surfaces `notes:` in the boundary confirmation it
  already presents. **This is the fix for "lost in the conversation":** the user
  already reads every phase boundary, so discoveries appear at the moment they were
  made, in a prompt already on screen. No new place to look, no new habit.
- **`CLAUDE.md`** — one line pointing at `parlay note`.

### Two distinctions to state explicitly in the skills

**Note vs. buildfile `decisions:`.** `generate-code.skill.md` step 13 already records a
judgment call *made and enforced in the code*. A note records work *not done*. If you
wrote it into a file it is a decision; if you walked past it, it is a note.

**Note vs. intent `Questions:`.** A Question blocks authoring — "I cannot write this
intent until someone answers X." A backlog item blocks nothing. Converting a standing
Question into an item is **not MVP**: `intents.md` freezes at first build and may never
be edited, so post-freeze removal would have to route through an amendment. Any future
conversion is restricted to pre-freeze founding docs.

---

## 9. How items surface

Capture is only half a loop. An item nobody sees again is a lost conversation with
extra steps, so the read surface needs specifying as precisely as the write surface.

### `parlay backlog list` and `parlay backlog show`

`list` is the single inventory from §2 — it shows backlog items **and** parked
features, because a reader asking "what are we not doing?" should not have to
remember that the answer lives in two shapes.

```
$ parlay backlog list

UNTRIAGED (4)
  01K4M2QX  gap      Initiative rename does not check orphan collision   @multi-root
  01K4M8ZT  defect   check-drift misreports shared-source dirt           @domain-model
  ...

P0 (1)
  01K3RRPV  debt     Codegen re-reads every buildfile when diff errors   @code-generation

P1 (6)
  ...

PARKED FEATURES (14)
  parlay-tool/code-generation        superseded by shipped impl      2026-04-18
  features-and-initiatives-renaming  rename deferred to a later pass 2026-04-16
```

Untriaged first, deliberately: that is the pile a person is needed for. Flags:
`--about @feature`, `--related @feature`, `--kind`, `--open`, `--untriaged`,
`--priority`, `--root`, and `--json` for agent consumption. `show <item>` prints one
item in full, history and all.

**`--about` and `--related` are not the same filter.** `--about` is the narrow one: it
matches items that declare they concern that feature. `--related` matches an item whose
`captured.feature` is that feature **or** whose `about[]` carries a ref rooted at it.
The difference matters because `about` is optional and, when present, may point
elsewhere — §6's example was captured in `@parlay-tool/multi-root` and is about
`@features-and-initiatives-renaming`. An `--about` read would miss it in the feature
where somebody actually found it, which is precisely the feature whose designer is
about to reopen that ground.

### The read rule: phases write freely, and read once, narrowly

A phase that loads the whole backlog would re-introduce exactly the cost parlay's
scoped read-set was built to remove — `generate-code.skill.md` already forbids
answering uncertainty by loading everything, and enforces it with
`codegen-input-out-of-scope`. It would also invite an agent to pull unrelated work into
a run nobody scoped for it.

So the rule is asymmetric, and deliberately so:

- **Writing is unrestricted.** Any phase may `parlay note` at any point (§8).
- **Reading is one scoped call, with one owner.** At the start of the designer
  phase-group: `parlay backlog list --related @feature --open --json`. The
  `parlay-designer` subagent performs it **once for the whole group** — the chained
  intents, dialogs and artifacts phases do not each repeat it, and on an adapter with
  no subagent primitive the loop driver owns the call instead. A standalone invocation
  of one of those three skills performs it once on entry, because in that case there is
  no phase-group above it to have done so. Items related to this feature are relevant
  to it; the rest of the inventory is not, and is not loaded.
- **The result is reported, never applied.** The designer surfaces "3 open items name
  this feature" in its phase-boundary `parlay-decision` `context:`, with titles and
  ids. The user decides whether any enter scope. A phase must never fold a backlog item
  into the work on its own initiative — that is scope the user did not authorize, and
  §5's cheap capture only stays safe if capture cannot silently become commitment.

### Skill and module changes

This is the work that makes the API real. Nothing below is optional: §5 and §8 both
turn on the fact that a CLI no phase is instructed to call does not get called.

| File | Change | Slice |
|---|---|---|
| `loop.skill.md` | Driver surfaces `notes:` at every phase boundary; carries the designer's scoped-read result into the boundary `context:`; owns the scoped read itself on adapters with no subagent primitive | 2 |
| `parlay-decision` block | Gains optional `notes:` (ids captured this phase) and `backlog_hits:` (scoped-read result) | 2 |
| `create-intents.skill.md`, `scaffold-dialogs.skill.md`, `create-artifacts.skill.md` | Capture rule. Each performs the scoped read **only when invoked standalone** — never inside a chained designer run, where the phase-group has already done it | 2 |
| `build-feature.skill.md` | Capture rule — discoveries while authoring buildfile and testcases | 2 |
| `generate-code.skill.md` | Capture rule; step 11 report gains a notes line; the note-vs-`decisions:` distinction from §8 | 2 |
| Subagent definitions (`parlay-designer`, `parlay-build`, `parlay-code`) | Capture rule restated per agent — a subagent cannot ask, so it must know to file. `parlay-designer` additionally owns the single scoped read for its group | 2 |
| `CLAUDE.md` deploy template | One line pointing at `parlay note` | 2 |
| `doctor.skill.md` | Survey gains backlog counts, untriaged and stale routing; routes to `/parlay-backlog`, does not triage inline (§11) | 3 |
| `backlog.skill.md` (new) → `/parlay-backlog` | Owns triage sessions | 3 |
| `refine.skill.md` | Accepts `trigger: backlog:<id>`; offers to close the item when the amendment applies | 4 |

These are **source** edits under `core/internal/embedded/skills/`, not edits to the
deployed copies: the dogfooding rule is edit source, `make build`, then
`./parlay upgrade`. A change made only to `.claude/skills/parlay-*/SKILL.md` is
overwritten by the next upgrade and never reaches another project.

---

## 10. Promotion

```
parlay promote <item> --as-feature <name> [--initiative X]
parlay promote <item> --as-amendment @feature
```

`--as-feature` creates the normal three feature-tree directories through refactored,
shared `add-feature` scaffold logic. The scaffold operation is idempotent and reports
partial writes for repair/retry; the current `add-feature` implementation is a
sequence of directory and file writes, not an atomic multi-file transaction. Only
after all scaffold files exist successfully does promotion append the terminal
`promoted` event. The item is retained as provenance, never duplicated into active
requirements.

**It writes the standard zero-intent scaffold and does not seed a Goal.** An
implementation observation is usually not a user-world outcome and has no Persona, so
seeding one manufactures exactly the malformed intent the scaffold warns against — and
it would break the `planned` detection from §3, since the promoted feature would parse
as having an intent it does not really have. Instead the scaffold carries a
**non-parsing** `backlog-origin` link, and the intents phase translates the evidence
into a real promise.

`--as-amendment` hands `/parlay-refine` a pre-filled `trigger:`. This closes a loop the
schema is already waiting for: `amendment.schema.md` describes `trigger:` as *"the
causal link that previously lived nowhere."* `trigger: backlog:<id>` makes it
resolvable, so the project can answer a question it currently cannot — which amendments
came from things we noticed while building, rather than from things someone asked for.

If the item carries a `priority`, promotion passes it through as a **proposed** rank
for the intents phase to confirm, never as a decided one — see §8 for why a `debt`
rank in particular must be re-judged against the user outcome rather than copied.

Direct `parlay add-feature` remains valid and produces a `planned` feature.

---

## 11. Review — one decision at a time

A backlog that only grows is a graveyard, and a graveyard is worse than nothing because
the absence of tracking looks like tracking.

Parlay already solved this shape. `next-legacy-review` emits **the next single stranded
judgment for a person to decide** rather than a wall of them. Copy it:

```
parlay internal next-backlog-review
parlay internal next-activity-review
```

Each emits one subject with any prior deferrals attached, so the reviewer starts from
what the last person could not resolve. `/parlay-doctor` reports counts and routes here
but does not triage inline — doctor is about repair, and deciding what to do next is a
different act. A dedicated `/parlay-backlog` skill owns triage sessions.

No due dates, no burndown, no nagging.

---

## 12. Status and gate output

Two columns rather than one overloaded token:

```
core  parlay-tool/code-generation           dialogs   parked        superseded by shipped impl (2026-04-18)
core  features-and-initiatives-renaming     dialogs   parked        rename deferred to a later pass
core  parlay-tool/domain-model              dialogs   unclassified
core  some-new-feature                      planned   active
```

`parlay gate --all` closes with `7 passed, 23 blocked, 14 parked, 3 unclassified`
instead of `17 not yet gated` — **after triage**, see §13.

### JSON: bump to `schema_version: 2`

`featureEntry` gains `activity` (omitempty; absent means unclassified) and `phase`
gains the value `planned`.

The `activity` field alone is additive and would not require a bump. `planned` does.
Adding a value to a documented enum breaks an exhaustive consumer, and the house rule
in `schema-versioning.schema.md` treats exactly this as a bump: `.code-hashes.yaml` is
recorded there as *"currently at 2; v2 added the `hand-authored` provenance, changing
the domain of an existing field."*

`PhaseHandAuthored` added a value to the `phase` enum without bumping
`statusSchemaVersion`, and that looks like an argument for not bumping — but both
changes landed in the **same commit** (`e159f47`), one bumping and one not, for the same
kind of change. That is an internal inconsistency, not a policy. We follow the house
rule and bump; a tolerant consumer is unaffected either way.

---

## 13. Migration and first-run behaviour

**The 17 must not be auto-migrated to `parked`.** On first deployment they report
`unclassified`, because that is what is true: nobody has declared anything about them.
Auto-parking would assert a decision nobody made, and would do it silently across 17
features at once.

Clearing them is a one-time guided triage through `next-activity-review`, one feature
at a time, each declaration attributed. No mtime inference, no bulk default.

The summary line in §12 is therefore what status prints *after* that triage, not what
it prints the day this ships.

---

## 14. Diagnostics

| Code | When it fires |
|---|---|
| `backlog-item-not-parseable` | Invalid YAML, or missing `schema_version` |
| `backlog-item-frontmatter-incomplete` | `id`, `kind`, `title` or `captured` missing |
| `backlog-item-priority-invalid` | `priority` is present but not one of `P0`, `P1`, `P2` |
| `backlog-captured-update-forbidden` | A Parlay mutation command attempts to change an immutable `captured` field |
| `backlog-history-update-forbidden` | A Parlay mutation command attempts anything other than appending a valid history event |
| `backlog-fold-dangling` | `folded` names an item that does not exist |
| `backlog-promotion-dangling` | `becomes:` names a feature or amendment that does not exist |
| `backlog-item-stale` (warning) | Open past an age bucket with no terminal event; prior deferrals remain review context, not a permanent exemption from age visibility |
| `activity-declaration-not-parseable` | Invalid `activity.yaml` |
| `activity-history-update-forbidden` | A Parlay mutation command attempts to edit or remove an existing activity event |
| `activity-parked-feature-advanced` | A parked feature gained artifacts — the parking is stale |
| `activity-undeclared` (warning) | Unbuilt, no boundary, no declaration — §7's `unclassified` |

The update-forbidden diagnostics enforce writes made through Parlay commands. A
validator reading only the current YAML has no prior value against which to prove that
`captured` or `history` was hand-edited, so MVP does **not** claim retrospective tamper
detection for direct edits. That stronger guarantee would require a separate trusted
baseline or ledger, which this design deliberately avoids.

---

## 15. Multi-root

Each active root owns its own `spec/backlog/` and is its own promotion target. Origin
and ownership roots can legitimately differ — a discovery made while working in `core`
may belong to the parent — so `captured.origin_root` records where it was found and
`--root` targets where it is filed. Parent `status` aggregates child backlog counts the
way it already aggregates features. No cross-root mutation.

---

## 16. Deliberately out of scope

- No owners or assignment beyond `by:` attribution.
- No estimates, burndown or sprints.
- No priority *inference*. An agent never guesses a rank; absent means untriaged (§8).
- No scheduling metadata — no due dates, no ordering beyond `P0|P1|P2`.
- No GitHub Issues sync. A later adapter-shaped concern; the item shape should not
  preclude it and nothing here should assume it.
- No `--from-question` conversion (§8).
- No inbox triage UI — one item at a time (§11).

---

## 17. Ship order

Each slice is independently useful, which is the test that the decomposition is real.

1. **Content-aware phase (`planned`) + activity declarations + status/gate reporting +
   guided first-run triage + the shared locked-history mutation helper.** These ship
   together: activity does the immediate cleanup of all 17, `planned` fixes the
   misreporting bug for every feature created from now on, and activity appends use
   the acquire-before-read lock discipline from their first release. Includes the
   status JSON v2 bump.
2. **`parlay note` + `parlay backlog edit` + the item schema/parser/validation +
   read-only `backlog list` + phase discovery instructions and boundary surfacing.**
   Item writes reuse slice 1's locked-history helper. This slice includes the optional
   `parlay-decision` `notes:` field, the scoped designer read, and the loop/build/code
   rules from §8 and §9. The `priority` field lands with the schema here, settable at
   capture but never inferred. Capture now fixes the lost-conversation problem in the
   workflow where discoveries occur; a CLI that no phase is instructed to call would
   not be independently useful.
3. **Triage: defer, decline, obsolete, fold + priority assignment +
   `next-backlog-review` + `/parlay-backlog` + doctor routing.** The backlog becomes
   reviewable, and the untriaged pile becomes rankable.
4. **Promotion + amendment `trigger: backlog:<id>`.** The item leaves the inbox for a
   real feature or amendment and the provenance loop closes.

---

## 18. Decisions worth keeping

Recorded because nothing else records them, and each was contested:

- **Phase fix ≠ inventory fix.** Measured: 0 of 17 become `planned`; 9 move
  `dialogs`→`intents`; 8 do not move. The activity axis clears 17 of 17. This is why
  slice 1 contains both and why neither is described as solving the other's problem.
- **Two records, not one object.** A parked feature is already expressed as intent and
  so cannot be a backlog item under §4. One inventory, one promotion path, two records.
- **Derived state, `deferred` non-terminal.** No stored state field, so a status can
  never disagree with the history that produced it.
- **Colocated activity.** A central file keyed by qualified id goes stale under
  `parlay move-feature`.
- **Strict writer, non-blocking caller.** Durable user-facing state, not telemetry.
- **Command guarantee, not retrospective tamper detection.** Parlay's edit and triage
  commands preserve immutable capture data and append history under lock. A validator
  seeing only current YAML cannot prove a past hand edit; doing so would require the
  separate trusted baseline this design deliberately rejected.
- **Capture ships with its instructions.** `parlay note` without phase rules and
  boundary-visible ids does not solve conversation loss, so both belong to slice 2.
- **Priority reuses `P0|P1|P2` with intent semantics, not scheduling semantics.**
  Reusing the tokens was flagged as a collision, and it would have been one had the
  field meant "do this first". Defined as impact — the cost of the work staying undone,
  which is what `intent.schema.md` already says the scale measures — the two agree well
  enough to share a vocabulary. Not perfectly: `debt` reaches a user only through the
  code carrying it, so a rank carries into promotion as a proposal the intents phase
  confirms, not an inheritance. Never inferred by an agent; absent means untriaged, and
  `--clear-priority` can return it there.
- **Phases write freely and read once, narrowly.** A phase loading the whole backlog
  would undo the scoped read-set discipline and let capture become uncommissioned
  scope. One `--related @feature` read at the designer boundary, reported to the user,
  never self-applied.
- **Status JSON v2.** Per the house rule; `PhaseHandAuthored`'s non-bump was an
  inconsistency within its own commit, not a policy to follow.
- **No new concurrency primitive, and no unnecessary etag.** `atomicfile` cannot do
  compare-and-swap and its fixed `.tmp` path is itself a same-path hazard. History
  mutation reuses the existing `flock` pattern, acquiring before read and holding
  through rename. Domain-model-style etags were considered and deliberately omitted:
  no stale object crosses this CLI command boundary, so the lock already covers the
  full read-modify-write operation.
