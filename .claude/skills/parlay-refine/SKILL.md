---
name: parlay-refine
description: "Parlay: Make a small, precise change to an existing feature — spec, code, tests and baselines together"
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
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Asking the user

**Which protocol you use depends on how you were invoked — check before you
prompt.** The gates below are real questions and must reach a human either way;
what differs is the channel.

- **Invoked directly** — a person typed `/parlay-refine` and you are the agent
  they are talking to (you have `AskUserQuestion`). Prompt directly: there is no
  driver to round-trip through, and emitting a `parlay-decision` block would
  throw YAML at nobody while the person who asked waits.
- **Invoked as a subagent** — a loop driver or another orchestrator is running
  this flow and you have no `AskUserQuestion` (the standard phase-subagent
  situation, per the CLAUDE.md subagent rule). A question asked here reaches
  nobody and the phase answers itself, which is indistinguishable from a granted
  confirmation. Stop at each gate and return a `parlay-decision` block instead;
  the driver prompts the human and resumes you with the answer.

The determining fact is whether `AskUserQuestion` is actually available to you,
not the command name — do not assume refine is always person-driven (it is not).
Either way the gate is answered by a human before you proceed; neither channel
lets the skill confirm to itself.

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

## The ledger — record before you apply

Every project runs the ledger-and-contract model: the founding docs froze at
first build, the contract artifacts are current truth, and change goes
through append-only amendments. A refinement IS an amendment applied to the
contract. Two things follow for the steps below.

**surface.md does not exist anymore.** If the feature still
carries one, it is pending retirement — run `parlay migrate-spec --retire-md`
(then `parlay internal scaffold-signatures @{feature}`) BEFORE amending, and
never mirror an amendment into a surface.md: surface.yaml is the only
surface artifact, and a maintained-looking .md that lags the contract is the
most misleading document a feature can carry.

**The founding documents are frozen.** `intents.md` and
`dialogs.md` are the historical record of the feature's founding — never
written again after first build. Do not splice them, do not re-sync dialogs
to match a change, do not ask permission to edit them (the answer is
structural, not personal). The contract artifacts — `surface.yaml`,
`capabilities.yaml`, `infrastructure.md`, the domain model — are the current
truth, and they are what step 4 amends. `parlay internal check-drift` enforces
this: an edit to a frozen doc surfaces as a `ledger_integrity` finding, not as
drift.

**Frozen is not immutable in force.** A founding intent records what the
feature promised and why, at the moment it was founded. When the project
decides otherwise, that document does not become wrong — it becomes history,
and the new decision is recorded on top of it. `supersedes_intents:` on an
amendment is how (see step 3.5). This matters most for a feature that owns no
contract artifact: `affects:` has nothing to resolve against there, so before
supersession existed such a feature could not be changed at all, and the only
routes were to edit a frozen document or to leave the spec contradicting the
code indefinitely. Neither is a route; both are damage.

**Read the promise set before proposing a change.** Run
`parlay internal active-spec @{feature}`. It reports which founding promises
still stand (`active`), which a later decision retired and by which amendment
(`retired`), and whether a retirement is recorded but not yet applied
(`pending`, with `blocked: true`). Two things follow. A change that contradicts
an `active` promise is a supersession and takes step 3.5's decision gate below —
not a quiet contradiction, and never an edit to the frozen file. A change that
merely refines what a `retired` promise used to say needs no supersession: that
promise is already history.

**The change is recorded before it is applied.** Insert this step between
steps 3 and 4:

3.5. **Record — append the amendment.** Write
   `spec/intents/{feature}/amendments/NNN-{slug}.md` where NNN is one past
   the highest existing sequence (001 for a first amendment):

   - Frontmatter: `amendment:` (the slug, matching the filename),
     `date:`, `trigger:` (what prompted this — name the asking feature as
     `@feature` when the pressure is cross-feature), `affects:` (the
     contract entries this changes, as `@{feature}/<kind>:<name>` refs with
     kind one of `operation | surface | infrastructure | domain`),
     `supersedes:` (earlier amendment slugs this replaces, usually empty), and
     `supersedes_intents:` (founding intent slugs **in this feature** whose
     promise this decision replaces — omit unless the gate below applies).
   - Body: `## Change` (the delta, in prose — never a restatement of the
     feature), `## Why` (the reasoning; this is the only place it gets
     recorded), `## Acceptance` (criteria; step 4 lands them as `verify:`
     entries on the affected artifact entries — omit only for renames and
     pure-prose changes).

   **If the change contradicts a founding promise, gate it separately first.**
   Step 3's read of `parlay internal active-spec` tells you whether it does. This is the one
   decision in the skill with **no safe default and no non-interactive path**:
   retiring a promise reduces what the feature commits to, and an agent that
   answered it alone would be deciding, on the project's behalf, to stop doing
   something the project said it would do. Under `--non-interactive`, raise a
   `parlay-decision` block and **write nothing** — not the amendment, not the
   splice.

   Present what is actually being given up, taken from `parlay internal active-spec` rather
   than paraphrased: the promise's **Goal**, its **Verify** bullets, the
   **replacement** (`## Change` and `## Acceptance`), and the **disposition of
   every contract entry** whose `source:` names it. Name each such entry in
   `affects:` and state in `## Change` what becomes of it — replaced, removed
   or retained. `affects:` carries refs only: it can prove you enumerated an
   entry, never which of the three you chose, so the disposition itself lives
   in the prose and nowhere else. `parlay internal check-amendments` reports
   `intent-supersession-unaccounted-affect` for an entry you did not
   enumerate, but the reviewer should see the list before it is written, not
   after.

   Then record it as a supersession: name the intent slug in
   `supersedes_intents:`, and write both `## Why` and `## Acceptance` — neither
   is optional here, and the rename/pure-prose exemption does not apply.
   `## Acceptance` is the successor criteria step 4 lands as `verify:` entries
   on the affected contract entries — that splice is what makes them current
   truth, not the amendment file itself. Without one the amendment retires a
   promise and puts nothing in its place, which the validator refuses as
   `amendment-supersession-no-successor`.

   Three refusals you cannot argue past, all reported by
   `parlay internal check-amendments` after the write: an intent this feature
   does not declare, a second live amendment retiring an intent another already
   retired (name the earlier in `supersedes:` to settle it), and retiring a
   feature's **last** live promise — a feature that promises nothing is a
   lifecycle question with its own dependency checks, not a ledger entry. If
   you hit the last one, stop: what you are doing is retiring the feature, and
   that is not this operation.

   **The retirement takes effect only once applied.** Until the baseline
   records this amendment, the feature still makes the old promise — the
   artifacts and the generated code still keep it — so every advancing
   boundary blocks with `unapplied-amendments` naming the pending supersession.
   That is correct and is not something to work around. **A governance
   amendment does not apply through the splice.** One that supersedes an
   intent or retires a feature changes what the founding documents promise,
   and no artifact edit can express that — the ordinary splice-and-rebaseline
   path has nothing to splice. Apply it with:

   ```
   parlay internal apply-governance @<feature> --confirm
   ```

   which applies every pending governance amendment on that feature and moves
   the applied marker. Without `--confirm` it refuses and names the promises
   that would stop being in force — read that list before confirming, because
   it, not the amendment filename, is what you are approving. Only after that does the boundary stop naming it. Reach for the
   splice for amendments that change an artifact's content; reach for
   `apply-governance` for ones that change what the feature promises at all.

   **Decision-gate the exact file content before writing** — same rule as
   step 4's gate, and the two are one decision when convenient: show the
   amendment and the artifact splice together. Gating both texts here is what
   makes the ordering below safe: the reviewer has already approved the
   amendment AND the splice before either lands. An amendment is written once
   and never edited; a correction later is a new amendment naming this one
   in `supersedes:`. After writing, **re-read the file you just wrote and run
   `parlay validate --type amendment spec/intents/{feature}/amendments/NNN-{slug}.md`
   on it.** A parse or shape failure means the write itself is corrupt — a
   truncated frontmatter, a dropped `affects:`, a mangled heading — and every
   later step reads a broken ledger entry. Fix the file before proceeding.
   This single-file check is shape only; it does **not** resolve `affects:`
   against the contract, so it is safe to run now, before the splice.

   **Do not run `parlay internal check-amendments` yet.** That command resolves every
   `affects:` ref against the contract artifacts, and an **ADD** amendment —
   one that introduces a new operation, fragment, or entity — names a ref that
   does not exist until step 4 splices it in. Running it here would report a
   spurious `amendment-affects-unresolved` for exactly the additions the
   amendment authorizes (L2). So the order is: write the amendment (this
   step) → apply the splice (step 4) → **then** `parlay internal check-amendments`. Nothing is
   lost by resolving after applying, because the decision gate above already
   fixed both texts before either was written.

   **Then journal it:**
   `parlay internal refine-journal @{feature} --step amendment-written --amendment NNN --ask "{the prose you were given}"`.
   From here the run is recoverable: a session that dies after this point
   resumes at the splice with the amendment it already wrote, instead of
   restarting and minting a duplicate sequence number for the same ask.
   Stamp each later boundary the same way — the step list below names its
   own stamp — and step 9 clears the journal.

Step 4 then applies the delta **to contract artifacts only** — step 3's
altitude table routes "user-visible" to `surface.yaml` (the narrative record
already happened in 3.5).

**After the splice lands, run `parlay internal check-amendments @{feature}`.**
Now every `affects:` ref resolves — including the entries step 4 just added —
and the command validates the ledger and emits `dirty_set`: the resolvable
`affects:` of the **unapplied tail** (amendments past the baseline's
`last-applied-amendment`), which after this write is exactly the amendment you
just added. That tail is what step 5's diff should report dirty — the two
answer the same "changed since the last build" question and must agree.
(`all_affects`, the whole-ledger union, is emitted alongside it for audit; it
is NOT the rebuild-scoping set and will name long-applied refs.) A
disagreement between `dirty_set` and the diff, or any
`amendment-affects-unresolved` now that the splice is in, means the amendment
or the splice is wrong; stop and reconcile rather than proceeding.

Steps 5–10 run unchanged; step 9's re-baseline records the new amendment as
applied, which is what clears it from `parlay internal check-drift`'s
`unapplied_amendments`.
The feature-gate in step 2.5 is unchanged: an ask that is a new feature still
goes to `/parlay-loop`, which authors founding docs for the NEW feature —
birth is not what froze.

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

1.5. **Pre-flight — is there anything to do?** Run
   `parlay internal check-applied @{feature}` **before reading any artifact,
   schema or module.** One call, a few hundred bytes: the drift verdict, the
   ledger index (frontmatter only — seq, slug, date, trigger, affects, and
   whether each is applied), and any interrupted run. Two branches end the
   run here, and both are cheap by construction — you have loaded nothing.

   **Already applied.** `clean_state: true` and an indexed amendment whose
   `trigger`/`slug`/`affects` describe this same ask: open that one file (the
   index gives its path) to confirm, then STOP and report
   "already applied as NNN-{slug} on {date}" with its `## Change` line. Do not
   re-amend, do not "verify by regenerating", do not improvise a fresh
   no-op path — the whole point of this step is that eight different agents
   improvising eight different no-op paths is what it replaces. If the user
   wants it changed *further*, that is a new refinement of the current text,
   and it gets its own amendment.

   **Matching is a judgment, not a string compare.** The index is there so
   the judgment costs frontmatter rather than a loaded context. Same rule as
   step 2: do not match on lexical overlap. When you are unsure whether the
   ask is the recorded one, say so and ask — an unnecessary refinement costs
   a run; a missed contradiction costs the record's honesty.

   **In flight.** `in_flight` present means a previous refinement died
   mid-run. It names the amendment it wrote and the steps it completed;
   resume at the first incomplete step rather than restarting. Critically:
   if `amendment` is set, that file already exists — amend it, never mint a
   new sequence number for the same ask. Confirm with the user that they
   want the interrupted run finished (show its `ask`) before continuing.

   **Anything else** — drift, an unapplied tail, integrity findings —
   proceed with the steps below; those are handled where they already were
   (step 5's scope, `/parlay-loop`'s gate, doctor's dispositions).

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
   - **User-visible** → `surface.yaml`. What someone sees, does, or is told.
     (Never `intents.md` or `dialogs.md` — those are frozen founding
     documents; the narrative record of the change is step 3.5's amendment.)
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
   `parlay internal scaffold-signatures` splices its block by line rather than re-encoding the
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

   **Inert for topology is not inert for prose.** The buildfile's
   `description:` fields and testcases' `expected:`/description text routinely
   restate the words you just replaced, and skipping the rebuild leaves them
   asserting the old text — three benchmark replicates out of three shipped
   internal files still describing a retired label. After a replace-span,
   grep `.parlay/build/{feature}/buildfile.yaml` and `testcases.yaml` for the
   replaced literal(s) and splice those occurrences too (tool-internal files;
   no decision gate needed). Say in the report whether any were found.

   **Adding** — a new intent block, a new dialog turn, a new surface fragment,
   a new capability operation — is not inert. New spec elements imply new
   components, new plan rows and new suites, none of which exist yet. Carry
   that fact into step 5.5 and say which it was in your report.

   Journal it once the splice is on disk:
   `parlay internal refine-journal @{feature} --step splice-applied`.

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
   this feature, then continue. (In a multi-root project the `modules/`
   directory — like schemas and adapters — resolves at the **parent** repo-level
   root, not the child; read it from there even when the feature lives in a
   child root.)

   **Why this step has to exist.** Steps 6–7 regenerate code from the buildfile
   and then re-stamp `source-signatures` so the freshness gate passes. Without
   a rebuild, an added operation produces: the operation in the contract,
   nothing in the buildfile, no component, no test — and a green run, because
   the gate was satisfied by re-stamping rather than by the buildfile actually being fresh.
   The spec would document a capability that does not exist, and every check
   would agree it was fine.

   Re-running the build phase regenerates `testcases.yaml`, which is what makes
   step 10's re-review necessary rather than merely tidy.

   Journal it — whether you rebuilt or skipped:
   `parlay internal refine-journal @{feature} --step rebuilt`. A skipped
   step is still a completed one; the journal records where the run GOT TO,
   not what it chose to do.

6. **Regenerate** — Preserve stable, regenerate dirty, exactly as `generate-code`
   does. Append every file written to `.parlay/build/_project/.emitted`, one
   path per line, as you write it. The manifest is what makes step 9 a scoped
   re-baseline rather than a project-wide one.

   Journal it once the last file is written:
   `parlay internal refine-journal @{feature} --step emitted`.

   **Path format:** each line is the file path as it appears under the walk
   root that step 9 passes as `--source-root`, i.e. the same prefix the code-hash
   keys carry. In a multi-root project that prefix is the child root — a file
   generated into the `core` child is written as `core/internal/...`, not
   `internal/...`. save-build-state normalizes both the manifest and the stored
   keys before matching, but a wrong prefix here still under-scopes the
   re-baseline; write the path exactly as it sits from the project root.

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
   failures in `context:`. Do not re-baseline. **Leave the journal in place** —
   it is what lets the next invocation pick up at the test step instead of
   re-doing the amendment and the splice.

   Green suite → `parlay internal refine-journal @{feature} --step tested`.

   **Rebuild before smoke-testing.** If you go on to smoke-test the running app
   or a compiled binary — not just the test suite — rebuild it first. Step 6
   regenerated source; a binary compiled before this refine still runs the old
   behavior, so the change is visible to `go test`/the test runner but not to a
   stale binary. Smoke-testing the old binary reports a pass or a failure that
   describes neither the code you just wrote.

9. **Re-baseline** — `parlay internal save-build-state --source-root {root} --partial --emitted .parlay/build/_project/.emitted`

   `--partial` is required and it makes `--emitted` mandatory. A partial run
   with no manifest does not degrade, it is wrong: it would mark every tracked
   file in the project unknown on the strength of a run that touched three, and
   `--strict` would then fail on all of them. With the manifest, files this run
   did not touch keep the verdict they already had.

   `--partial` also scopes the *baselines*, not just the provenance half. It
   advances the `.baseline.yaml` only for the features whose files appear in the
   manifest (resolved through each file's generation marker); every other
   feature keeps the baseline — and the dirty flags — it already had. So a
   feature whose source drifted but which this refine did not regenerate still
   reports dirty afterward, instead of being silently re-blessed into a false
   "stable". The project baseline records the blessed feature slugs under
   `emitted:` for audit. (See `schema-versioning.schema.md`, "Per-feature
   blessing instants".)

   **If the manifest is empty — a spec-only feature.** Step 6 writes nothing
   when the amended artifacts drive no generated file. The ordinary command
   then blesses nothing: the manifest resolves to no feature, so this feature's
   baseline never advances and the amendment you just spliced stays pending
   forever, blocking every advancing boundary. There is no other sanctioned
   route to "applied" for such a feature.

   Do NOT reach for this whenever a run happens to write nothing. First
   establish that it is genuinely output-less:

   - The manifest must be **present and empty**, not missing. A missing
     manifest means this run did not record what it wrote, which is a
     different problem.
   - Read the feature's `buildfile.yaml`. If its `plan:` names any `creates:`
     or `modifies:` entry, the feature **owes** generated code and an empty
     manifest means codegen did not write it. That is a failure to fix, not a
     feature to bless — raise a `failure` decision.
   - The command re-checks both, and also refuses if the feature already owns
     tracked generated files.

   Then **ask the user**. Ask the precise question rather than a paraphrase —
   it is the one judgement no check can make — and ask the one that matches
   what you actually found, because the two cases rest on different evidence:

   **A readable plan that names nothing:**

   > `{feature}` emitted no files this run, and its buildfile plans none.
   > Confirm this feature owes no generated code, so its amendment can be
   > recorded applied?

   **No buildfile, or no plan to read:**

   > `{feature}` emitted no files this run, and it has no buildfile plan, so
   > nothing mechanical can establish whether it owes generated code.
   > Confirm from your own knowledge that this feature owes none, so its
   > amendment can be recorded applied?

   Never ask the first question when there is no plan. "Its buildfile plans
   none" is false about a feature that has no buildfile, and a confirmation
   obtained by telling the user something untrue is not a confirmation.

   Only on an explicit yes:

   ```
   parlay internal save-build-state --source-root {root} --partial \
     --emitted .parlay/build/_project/.emitted \
     --outputless-feature @{feature} --confirm-outputless
   ```

   Never infer that answer, never pass `--confirm-outputless` on the ordinary
   path, and never pass it for a non-empty manifest. It confirms exactly one
   thing — that this feature owes no generated code — and it is recorded
   durably against the exact amendment in the baseline, because a human
   judgement nothing can reconstruct afterwards is what the recorded authority
   rests on.

   **It confirms nothing about promises.** A governance amendment, or a
   combined one carrying both `affects:` and `supersedes_intents:`, stays
   refused on this path however empty the manifest is.

   **If this refinement CHANGES a founding promise without ending it.** An
   amendment carrying `amends_intents:` with mode `extend` or `revise` changes
   what the feature promises, and no save may apply it — a save records build
   evidence and approves nothing. Apply it with:

   ```
   parlay internal apply-amendment @{feature}
   ```

   Run it with no `--confirm` first. It prints three things, and the user needs
   all three:

   - the **field-by-field delta** between the promise in force and the new one,
     with cleared fields shown explicitly (a version is a complete snapshot, so
     an omitted field is removed, not kept);
   - the **claim they are being asked to make** — that an `extend` takes nothing
     away, or that they approve this replacement and its scope impact;
   - the **contract entries attributed to that promise**, split into the ones
     this record declares it changes and the ones it does not.

   The second and third are the point. Nothing checks either claim: the tool can
   see that a lineage still resolves, and cannot see whether a revised promise
   still entails an entry attributed to it. **Show the user the whole list and
   get an explicit yes**, then re-run with `--confirm <digest>`. The digest binds
   the approval to the promise text, the attributed population, the scope
   declaration and the applied authority, so a yes given for one state cannot be
   carried to another.

   The amendment must declare `scope_impact:` with `version: 1`, and
   `preserves_unlisted: true` whenever any transition leaves a promise behind.
   Do not add it yourself to make the command proceed — it is the author's claim that every attributed entry it does not
   list still holds, and inventing it means approving a claim nobody made.

   **All four modes apply here: `extend`, `revise`, `narrow` and `retire`.**
   The last two take support away from contract entries, so they must declare
   what becomes of each one that loses it, using `scope_impact.exceptions`:
   `retained` (still supported by the changed promise), `revised` (survives, and
   may now be justified elsewhere), `removed` (gone, nothing takes it over), or
   `replaced-by` with `replaced_by:` naming where the work went. The tool checks
   each against the artifacts, so a disposition that contradicts what is on disk
   is refused rather than recorded.

   **A retirement is presented as an ending, not a delta**, and has no closure:
   every entry the promise justified owes a consequence, and nothing may still
   name the retired promise as its source afterwards. `retained` is impossible
   there — there is no promise left to retain anything. So a retire-only record
   must OMIT `preserves_unlisted:`; asserting it is refused, because the claim
   is that unlisted entries stay supported by the promise and there is none. On
   a record that retires one promise and revises another, keep
   `preserves_unlisted: true` — it applies to the living lineage only. The two ordinary
   consequences are `revised` for an entry that survives re-sourced to whatever
   justifies it now, and `replaced-by` for one that is gone with another
   carrying its work.

   **Capture the pre-splice inventory before you splice.** `refine-journal
   --step amendment-written --amendment N` records what each lineage justified
   at that moment. Run it after writing the amendment and BEFORE editing the
   artifacts: afterwards a removed entry is invisible, and for a retirement the
   whole population is, so a late capture would show nothing lost.

   **If this refinement withdraws a founding promise in the LEGACY spelling.**
   New records should end a promise with `amends_intents:` and `mode: retire`,
   which goes through the ceremony above. `supersedes_intents:` records no mode,
   so what its author meant was never captured and it cannot be executed as any
   particular one; it keeps the path below, which asks rather than assumes.

   An amendment carrying both `affects:` and `supersedes_intents:` has a splice
   to record AND promises to withdraw, and neither half may be applied without the other. That is the
   shape step 3.5 produces whenever the retiring promise still has live contract
   entries, so it is ordinary, not exotic. The save will refuse it. Apply it
   with:

   ```
   parlay internal apply-amendment @{feature}
   ```

   Run it with no `--confirm` first. It prints the founding promises that would
   stop being in force — their goal and verify bullets, not their slugs — and a
   digest. **Show that list to the user and get an explicit yes**, then re-run
   with `--confirm <digest>`. The digest binds the approval to the amendment's
   bytes, the promise set, the affected entries and the applied authority, so a
   yes given for one state cannot be carried to another; if anything moved, ask
   again.

   Do not answer it yourself, and do not proceed under `--non-interactive` —
   raise a `parlay-decision` block instead. A promise nobody agreed to withdraw
   is still in force.

   A record that withdraws promises and touches no contract entry has no splice
   to record and goes through
   `parlay internal apply-governance @{feature} --confirm` as before — that
   command prints the same kind of list, and that list, not a filename, is what
   withdrawal requires.

   **Then end the run's recoverable window**, in this order:
   `parlay internal refine-journal @{feature} --step re-baselined` followed by
   `parlay internal refine-journal @{feature} --clear`. The save is the
   commit point — after it the amendment is applied, the code is blessed,
   and a "resume" would re-do finished work. Clearing last means a crash
   between the two leaves a journal whose only incomplete step is the one
   that already succeeded; the next run's pre-flight shows it, and
   re-running save-build-state on an unchanged tree is a no-op.

10. **Check the standard still holds** — a refinement that changed criteria
    changed what this feature is graded against, and the approval was bound to
    the old set. `parlay internal criteria-authority @{feature}` reports whether
    the current criteria are approved and, when they are not, exactly which
    bullets were added or removed.

    An unchanged standard needs nothing: what was approved is the criteria, not
    the testcases derived from them, so regenerating suites re-approves nothing
    and asks nobody. A changed one routes back to the artifacts phase, where the
    mapping from each intent bullet to the criterion it became is shown and
    approved — approving from here would approve a standard nobody was shown,
    which is what the retired suite-name gate did.

    If the feature carries coverage exceptions, the same command's ledger check
    reports any that were granted against criteria that have since moved. Those
    block until re-reviewed rather than being dropped: a judgment that a
    specific criterion needs no test says nothing about a criterion that has
    been rewritten.

11. **Report** — What changed, in this order: the artifact and the span amended;
    the components regenerated; the test result; the baseline and coverage
    review refreshed (or `coverage re-review skipped: gate inactive` from
    step 10 when the gate is inactive). Then run
    `parlay internal check-drift @{feature}` and report the sentence that
    matters: it is clean, because the spec and the code agree again. (Do not
    assert it clean without running it — the run is the evidence.)

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
