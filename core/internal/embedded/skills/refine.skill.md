---
name: refine
description: "Make a small, precise change to an existing feature — spec, code, tests and baselines together"
---

# Refine

Make one small change to a feature that already exists, and leave the spec, the
code, the tests and the baselines agreeing with each other afterwards.

**What this is for.** Once an app exists, most work is small and precise: move
the filter above the table, make the approval step notify the requester, widen
a timeout. The pipeline was built for the other case — a feature from nothing —
and running the whole thing for a two-line change is absurd, so people don't.
They prompt an agent directly instead, the change lands in code, and parlay
learns nothing about it. Every later drift check then compares generated output
against a spec that no longer describes the system, and the divergence is
invisible because nothing recorded that it happened.

This is the tracked replacement for that. **Same prompt, same change** — the
difference is that the spec learns it too.

There are four ways to detect divergence in this toolkit and, before this,
nothing that resolves one. That asymmetry is the problem being fixed.

## Arguments

- `<prose>` (required): the change, in your own words. One argument, one
  change. "Move the status filter above the table." "The approval step should
  notify the requester too."
- `feature` (optional): `{feature}` or `@{initiative}/{feature}`. Omitted, it
  is resolved from the prose and the project; ambiguity is raised, not guessed.

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

## Asking the user

**You own the interaction — prompt directly.** This skill is invoked by a
person typing `/parlay-refine`, not by the loop driver, so `AskUserQuestion`
works here and there is no decision request to round-trip. An earlier version
carried the phase-module decision protocol, which opens by telling you that no
interactive tool exists and to return a `parlay-decision` block instead. That is
the right rule for a phase running inside a subagent and the wrong one here: it
would have you emit YAML at a driver that is not listening while the person who
asked for the change waits.

The gates below are real questions. Ask them.

## The one invariant: amend first

**Amend the spec artifact before regenerating anything.** Not after, not
alongside.

This is not a stylistic preference, it is what makes the rest of the chain
work unchanged. `parlay internal diff` compares current sources against the
recorded baseline and reports which components are dirty. Amending first means
the diff already describes the change, so the regeneration scope falls out of a
mechanism that exists — with **no change to `diff` itself**.

Regenerate-first has no vocabulary anywhere in this codebase. There is no way
to ask "what would this change affect" before it is written down, because
every scoping mechanism in the toolkit is a comparison against a baseline.

**What amend-first does NOT settle is which diff to run.** An earlier version
of this skill claimed the scope fell out "for free" and hard-coded
`diff @{feature}` at step 5. That holds only while the amendment stays inside
the feature's own directory, and an amendment can land in the blueprint, the
project domain model or an adapter — at which point the feature-scoped diff is
the wrong query and answers `stable` about files it never looked at.

So amend-first buys one thing precisely: after step 4 you *know* where the
change landed, which is the input step 5 needs. Use it there.

## Ledger mode

Read `.parlay/config.yaml` before step 3. If it carries `ledger: true`, this
project runs the ledger-and-contract model and two things change about the
steps below; if not, skip this section entirely — nothing else here applies.

**The founding documents are frozen.** In a ledger project, `intents.md` and
`dialogs.md` are the historical record of the feature's founding — never
written again after first build. Do not splice them, do not re-sync dialogs
to match a change, do not ask permission to edit them (the answer is
structural, not personal). The contract artifacts — `surface.yaml`,
`capabilities.yaml`, `infrastructure.md`, the domain model — are the current
truth, and they are what step 4 amends. `check-drift` enforces this: an edit
to a frozen doc surfaces as a `ledger_integrity` finding, not as drift.

**The change is recorded before it is applied.** Insert this step between
steps 3 and 4:

3.5. **Record — append the amendment.** Write
   `spec/intents/{feature}/amendments/NNN-{slug}.md` where NNN is one past
   the highest existing sequence (001 for a first amendment):

   - Frontmatter: `amendment:` (the slug, matching the filename),
     `date:`, `trigger:` (what prompted this — name the asking feature as
     `@feature` when the pressure is cross-feature), `affects:` (the
     contract entries this changes, as `@{feature}/<kind>:<name>` refs with
     kind one of `operation | surface | infrastructure | domain`), and
     `supersedes:` (earlier amendment slugs this replaces, usually empty).
   - Body: `## Change` (the delta, in prose — never a restatement of the
     feature), `## Why` (the reasoning; this is the only place it gets
     recorded), `## Acceptance` (criteria; step 4 lands them as `verify:`
     entries on the affected artifact entries — omit only for renames and
     pure-prose changes).

   **Decision-gate the exact file content before writing** — same rule as
   step 4's gate, and the two are one decision when convenient: show the
   amendment and the artifact splice together. An amendment is written once
   and never edited; a correction later is a new amendment naming this one
   in `supersedes:`. After writing, run
   `parlay internal check-amendments @{feature}` — it validates the ledger
   and every `affects:` ref, and its `dirty_set` should agree with what
   step 5's diff reports dirty. A disagreement means the amendment or the
   splice is wrong; stop and reconcile rather than proceeding.

Step 4 then applies the delta **to contract artifacts only** — the ledger
branch of step 3's altitude table routes "user-visible" to `surface.yaml`
(the narrative record already happened in 3.5). Steps 5–10 run unchanged;
step 9's re-baseline records the new amendment as applied, which is what
clears it from `check-drift`'s `unapplied_amendments`. The feature-gate in
step 2.5 is unchanged: an ask that is a new feature still goes to
`/parlay-loop`, which authors founding docs for the NEW feature — birth is
not what froze.

**Scope your reading, not just your writes.** The amendment's `affects:`
names the dirty entries before any hashing happens. Use it: at steps 5.5–6,
load the dirty components and their immediate neighbors from the buildfile
rather than the whole file — a 2,500-line buildfile read wholesale to change
one component is where a refinement's cost actually goes. The stable entries
are preserved verbatim by construction; you do not need them in context to
leave them alone.

**Test narrowing is an opt-in, not the ledger default.** `parlay internal
affected-set @{feature}` answers "who could this change touch" — the feature
plus every feature whose buildfile references it. Step 8 still runs the full
suite: narrowing the interactive run to the affected set trades "never bless
untested code" from per-run to per-backstop, and that trade is the
project's to make, not this skill's. Narrow only when BOTH hold: the user
has said so for this project, and an unconditional full-suite gate exists
somewhere scheduled (CI, nightly). When you do narrow, say so in the step-11
report — which mode ran is part of what was blessed.

## Steps

1. **Resolve the feature** — If `feature` was given, use it. Otherwise infer it
   from the prose and the project's features. If two features could plausibly
   own the change, raise an `ambiguity` decision listing them; do not pick the
   likelier one.

2. **Locate the owning artifact** — Which artifact does this change belong to?

   Read the candidates and decide. `surface.yaml` fragments carry `source:`,
   `capabilities.yaml` operations carry `source:`, `infrastructure.md`
   fragments carry `**Source**:` — those references are the map back from a
   change to the thing that owns it.

   **This is a judgment call, and it must stay one.** Do not match on title
   similarity. Lexical overlap both misses renames — the fragment was called
   something else when it was written — and blesses contradictions, matching a
   fragment that merely sounds related while the real owner goes untouched. A
   matcher here is worse than no automation, because it is confidently wrong.

   Raise an `ambiguity` decision when: no artifact clearly owns the change; two
   do; or the change contradicts what the owning artifact currently says.
   Contradiction especially — the user may be correcting the spec deliberately,
   or may have forgotten what it says, and those want opposite handling.

   Report which artifact you chose and why, before amending it.

2.5. **Is this a refinement at all? — stop and push back if not.**

   Run this before step 3 and before anything is written. Step 2 is where you
   learn the answer: an artifact that owns the change either exists or it does
   not, and if it does not, the request is not a refinement.

   **Adding one intent to a feature that already exists is refine's job**,
   provided the new intent attaches to what is already there — a second way to
   trigger something the feature already does, a case the existing intents did
   not cover, a constraint that turned out to need its own goal. That is a
   refinement of a feature, and handing it to the loop would restate four
   phases to add one block.

   Push back when the ask is a **new feature** rather than an addition to one:

   - **It needs several intents, not one.** A cluster of new goals arriving
     together is a feature, whatever the request was called.
   - **Step 2 found no owning artifact and no feature it attaches to.** Not
     "no exact intent" — no feature. A new intent still has to belong
     somewhere.
   - **It introduces a new user goal rather than extending one.** "Export as
     STL as well" extends; "add a project library with its own browsing,
     search and deletion" does not.
   - **It would need its own dialogs conversation designed from scratch**,
     rather than turns added to an existing one.
   - **It introduces domain vocabulary the model has no shape for** — a new
     entity, a new relationship. One new enum value is an addition; a new
     entity with its own lifecycle is a feature.

   When one holds: **do not amend, and do not offer to.** Say plainly which
   signal fired, and recommend `/parlay-loop {feature}` — or
   `/parlay-loop {feature} --from intents` when the feature exists and is
   gaining a cluster of new goals.

   Then ask whether to hand off, and default to yes in your recommendation. The
   user may know something you do not — a feature that exists but is worded so
   the reverse lookup missed it, a cluster that is genuinely one goal expressed
   awkwardly — so this is a question, not a refusal.

   But do not resolve it by proceeding. A feature's worth of intents spliced in
   here gets one pass of a chain designed for one change: the artifact-set
   decision is never raised, the dialogs are never designed as a conversation,
   and the coverage walk sees suites for goals nobody talked through. It arrives
   in the spec looking finished and is not, and nothing downstream can tell the
   difference.

   **Size is not the signal, and neither is "is it new".** "Add a keyboard
   shortcut for export" is one sentence and a new intent, and it belongs here —
   it hangs off an export intent that already exists. "Add a project library"
   is four words and belongs to the loop, because browsing, searching and
   deleting are three goals that need a conversation of their own. Judge by
   whether the ask attaches to a feature that exists, never by how much the
   user typed or whether the word "new" appears.

3. **Classify the altitude** — Two destinations:
   - **User-visible** → `intents.md`, `dialogs.md`, or `surface.yaml`. What
     someone sees, does, or is told.
   - **Implementation-shaped** → `infrastructure.md`. Boundaries, probes,
     allowlists, dependency pins, timeouts — architectural constraints that do
     not reduce to operations. See that schema's promotion section.

   Backend behaviour that *is* operation-shaped belongs in `capabilities.yaml`
   as an amendment to the operation its `source:` names.

   Everything is promoted to the spec. A change that lives only in code is the
   untracked path this command exists to replace.

4. **Amend — splice, never re-encode** — Change the span that needs changing
   and leave every other byte alone.

   The only amend semantics in this codebase say it exactly:
   *preserve hand-authored entries, replace extractor-owned spans*. And
   `scaffold-signatures` splices its block by line rather than re-encoding the
   buildfile, because round-tripping a 700-line reviewed document through a
   YAML encoder preserves every value and destroys the folded descriptions, the
   grouping blank lines, and the comments explaining why a field is the way it
   is. A refinement that reformats the artifact it touches makes the next
   review diff unreadable, which costs more than the refinement was worth.

   **Decision-gate the amendment.** Show the exact before/after span and get
   agreement before writing. These are designer-authored documents; editing one
   beyond what was asked for is the thing the designer brief already forbids.

   **Note whether you replaced a span or added one — the next steps differ.**

   Replacing text in place is structurally inert: a reworded constraint, a
   changed label, a tightened threshold. The set of components the buildfile
   derives does not move, so steps 5–7 as written are correct.

   **Adding** — a new intent block, a new dialog turn, a new surface fragment,
   a new capability operation — is not inert. New spec elements imply new
   components, new plan rows and new suites, none of which exist yet. Carry
   that fact into step 5.5 and say which it was in your report.

5. **Scope — the amendment decides how far to look, not the feature name.**

   Look at where step 4 actually wrote:

   - **Inside `spec/intents/{feature}/`** → `parlay internal diff @{feature}`.
   - **Anywhere else** — `.parlay/blueprint.yaml`, the project `domain-model.yaml`,
     an adapter, `.parlay/adapter-set.yaml` → **`parlay internal diff`** with no
     feature argument. The project diff is the only one that carries
     `sections.blueprint`, and it is what `generate-code` step 14 keys its
     blueprint-derived regeneration off.

   **Why this is not a formality.** The per-feature diff hashes exactly three
   buildfile sections — `models`, `routes`, `fixtures`. It has no blueprint key
   at all, so a blueprint change does not read as `changed` there; it is
   *absent*, and every rule that depends on it silently never fires.

   The failure this prevents, observed in a real run: a request to adjust a
   footer. The footer is a `chrome` region of a shell that declares
   `wraps: all`, so the amendment landed in the blueprint and its blast radius
   was every route in the app. `diff @{feature}` reported `routes: stable`,
   nothing was regenerated, and the run reported success with the visual defect
   still on screen.

   **A small request is not evidence of a small change.** Chrome, shells,
   navigation, guards and domain vocabulary are all things a person describes
   in one sentence and all things that live above the feature. You cannot know
   the scope before amending — which is exactly why this step reads the
   amendment rather than the argument you were given.

   When the project diff is the right one, say so in the report: the user asked
   about one feature and is about to see other features regenerate.

5.5. **Rebuild the buildfile — only if step 4 added something.**

   If the amendment replaced a span, skip this: the buildfile still describes
   the same components, and rebuilding it means re-running an AI phase over a
   reviewed 700-line document to change nothing.

   If the amendment **added** a spec element, the buildfile does not know about
   it yet. Read `.parlay/modules/build-feature.md` and run the build phase for
   this feature, then continue.

   **Why this step has to exist.** Steps 6–7 regenerate code from the buildfile
   and then re-stamp `source-signatures` so the freshness gate passes. Without
   a rebuild, an added intent produces: the intent in `intents.md`, nothing in
   the buildfile, no component, no test — and a green run, because the gate was
   satisfied by re-stamping rather than by the buildfile actually being fresh.
   The spec would document a capability that does not exist, and every check
   would agree it was fine.

   Re-running the build phase regenerates `testcases.yaml`, which is what makes
   step 10's re-review necessary rather than merely tidy.

6. **Regenerate** — Preserve stable, regenerate dirty, exactly as `generate-code`
   does. Append every file written to `.parlay/build/_project/.emitted`, one
   path per line, as you write it. The manifest is what makes step 9 a scoped
   re-baseline rather than a project-wide one.

7. **Refresh signatures** — `parlay internal scaffold-signatures @{feature}`.
   The amended artifact's hash moved; the buildfile's `source-signatures:` has
   to move with it or the freshness gate fires on the next run.

8. **Run the full test suite** — Not the affected suites. The whole thing.

   This is a deliberate cost. Re-baselining records "this output is blessed",
   and blessing untested code is the one thing the build state must never do.
   A refinement is small by definition, which is exactly when it is tempting to
   check only what you think it touched — and exactly when a missed interaction
   is most likely, because nobody is looking. If the latency becomes the reason
   people stop using this, affected-suites-only is the escape valve; take it
   deliberately, not by drifting into it.

   Tests failing stops the refinement. Raise a `failure` decision with the
   failures in `context:`. Do not re-baseline.

9. **Re-baseline** — `parlay internal save-build-state --source-root {root} --partial --emitted .parlay/build/_project/.emitted`

   `--partial` is required and it makes `--emitted` mandatory. A partial run
   with no manifest does not degrade, it is wrong: it would mark every tracked
   file in the project unknown on the strength of a run that touched three, and
   `--strict` would then fail on all of them. With the manifest, files this run
   did not touch keep the verdict they already had.

10. **Re-review coverage** — `parlay review-coverage @{feature}`.

    Not optional, and not tidiness. The refinement changed the buildfile, which
    invalidates the hashes `coverage-review.yaml` pins, so the review gate exits
    non-zero on the *next* codegen run — after this command has reported
    success and everyone has moved on. Chaining it here is what keeps a refined
    project in a state the next command can actually work from.

    Use `--exempt <suite>:<item>=<reason>` for terms that legitimately have no
    covering case.

11. **Report** — What changed, in this order: the artifact and the span amended;
    the components regenerated; the test result; the baseline and coverage
    review refreshed. Then the sentence that matters: `check-drift` is clean,
    because the spec and the code agree again.

## What this does not do

- **Not a second codegen path.** Steps 6 and 7 are `generate-code`'s behaviour,
  invoked for a smaller scope. Divergence between them is a bug in this skill.
- **Not for new features.** Adding one intent to a feature that already exists
  is refine's job; a cluster of new goals that needs its own conversation is
  the loop's. Step 2.5 is the gate that decides which of the two you were
  handed, and it turns on whether the ask attaches to an existing feature — not
  on whether anything is being added.
- **Not a way into a hand-authored unit.** A unit's code is written by a person
  and fenced off from codegen; a prose request to change one is a request to a
  person, not to this command. Refuse and say so.
- **Not a bulk editor.** One prose argument, one change. Several changes at once
  make the decision gates unanswerable, because the user is agreeing to a batch
  rather than to an edit.
