---
name: parlay-resolve
description: "Parlay: Act on the review comments people wrote in the files — one thread at a time, through the route each file's governance requires"
---

# Resolve

A person read a spec file, found something wrong, and wrote it down **in the
file, next to the text**. This is the pass that acts on what they wrote.

**Why the annotation is the confirmation.** A reviewer who writes
`<!-- @dwht: product asked for 500 -->` under a constraint has already said
what they want, in their own words, in the place it applies. Asking them again
per thread would turn a review of twenty comments into twenty prompts and
reintroduce exactly the conversation the annotation replaced. So nothing in
step 2 asks "shall I?".

**What that does not license.** Every governance gate that already exists still
fires. The annotation is a request for change, not a bypass of how change is
recorded: a frozen founding document still changes only through an amendment, a
supersession still needs the human decision it always needed, and an applied
amendment is still not editable. The reviewer chose *what*; the file's
governance chooses *how*.

**You never close a thread.** A reply the reviewer has not read is
indistinguishable from one they accept. Closure is an entry a person writes.

## Arguments

- `feature` (optional): one feature, in standard parlay form — `{feature}` or
  `@{initiative}/{feature}`. Omitted, work every feature that has open threads,
  one feature at a time.

<!-- parlay:active-root-aware -->

## Active root

Every path here — `spec/intents/<feature>/`, `spec/handoff/<feature>/`,
`.parlay/build/<feature>/`, and the project-level files every scan reads —
resolves against the **active root**, not the directory you happen to be in.
In a multi-root project a thread belongs to the root whose file carries it, and
a feature slug means nothing without its root: two roots may hold the same slug.
Carry the root through every command you run.

## Interactive or subagent

Invoked directly you have `AskUserQuestion` and answer the §6.2 gates by asking.
As a subagent you do not, so a question asked here reaches nobody and the phase
answers itself — which is indistinguishable from a granted confirmation. Stop at
each gate and return a `parlay-decision` block; the driver prompts and resumes
you with the answer. The determining fact is whether `AskUserQuestion` is
actually available, not the command name.

## 1. Read what is there

```
parlay internal collect-annotations @{feature}
```

With no feature argument, `parlay internal collect-annotations` reports every
feature that has anything, plus the project-level files once. Take them one
feature at a time and finish each before starting the next — a half-resolved
feature is worse than an untouched one, because its boundary is still blocked
and the reason is now split across two passes.

**Project files are in a feature's scan too**, and deliberately: the root
domain model, the page manifests, the adapters, `adapter-set.yaml` and
`blueprint.yaml` are read or depended on by every feature's build, so a thread
in one blocks every feature until it is resolved. The listing marks them
`(project)`. Resolve such a thread once; the other features stop reporting it
when it is gone.

Report the counts, then **the findings first**. A malformed annotation is
someone who tried to say something and whose comment did not land; they need to
know before anything else happens. Show each with its code, its line and its
fix, and **do not guess at what it meant**. `@dwht maybe: not sure` is not a
kind you can infer — it is a reviewer who will recognise their own typo the
moment they see it named.

If `errors` is non-empty, a feature could not be scanned. Say so. "No threads"
and "I could not look" are different answers and only one of them is safe to
build on.

## 2. Work the open threads, in file order

For each thread whose state is `open`:

### Show the anchor before the request

State the resolved unit **first** — its ref or YAML path, its span, and its
text — and only then the thread. A YAML column or a Markdown indent will
occasionally select something other than what the reviewer meant, and this is
where that becomes visible: in the reply they will read, in the place they
wrote, rather than three commits later in a diff. Open your `done` or `answer`
text by naming the unit you acted on.

### Route by the file's governance, then the feature's state

The reviewer writes the same comment either way. You decide the route.

| File | No baseline (never built) | Baseline (built) |
|---|---|---|
| `intents.md`, `dialogs.md` | **Direct edit.** These are designer-authored and the annotation is the designer's written instruction — the permission CLAUDE.md asks for. Re-run `check-coverage`: a changed intent may orphan a dialog. | **Never edited.** The request becomes an amendment through `/parlay-refine`, with the `amends_intents:` mode that fits — `revise` for changed wording, `extend`/`narrow` for scope, `retire` only through the supersession gate. The `done` reply names the amendment. |
| `surface.yaml`, `capabilities.yaml`, `infrastructure.md`, feature `domain-model.yaml` | **Direct edit**, then `parlay validate --type …`. | **`/parlay-refine`**, with the annotation as its prose and `trigger: "annotation by <handle> on <ref>"`. Refine's steps run unchanged: amendment first, then the splice. The `done` reply names the amendment. |
| feature `domain-model.md` (legacy) | Same as `domain-model.yaml` above — it is the same artifact in the retired form, and the collector reads it for exactly that reason. Say in the reply that the file is legacy and that `parlay migrate-domain-model` converts it; a thread here is a good moment to do that, but converting is a separate decision and not part of answering the comment. | same |
| `amendments/NNN-slug.md`, unapplied | **Atomic in-place edit**, preserving the `amendment:` slug and its sequence number. A never-applied record has no baseline hash, so append-only integrity has nothing to anchor on — but backlog `becomes:`, later `supersedes:` and `trigger:` refs may already name it. **Refused** while a refine journal names that record: an interrupted refinement resumes against the text it wrote. | same |
| `amendments/NNN-slug.md`, applied | **Refused** — the scanner reports `annotation-in-applied-record` and there is no thread to work. Tell the reviewer the comment belongs on the contract entry the amendment changed, or in a superseding amendment. | same |
| `*.page.md`, `blueprint.yaml`, adapters, `adapter-set.yaml`, `authored.yaml` | Direct edit, then the file's validator. Project-owned; not ledgered. | same |
| `spec/handoff/<feature>/specification.md` | **Never a direct edit** — see the two branches below. | same |

**The handoff has two branches**, and telling them apart is the whole of the
row above. The handoff is derived and stays derived: a direct edit would be
erased by the next regeneration, so it is never the answer.

- The comment is about the **content** — a promise stated wrongly, a criterion
  missing, an operation described as something it is not. Route to the artifact
  that passage was derived from, by that artifact's row, then regenerate.
- The comment is about **presentation alone** — wording, ordering, the layout
  of the handoff itself. There is no artifact to route to, because the
  complaint is about the generator. Reply `declined`, say that plainly, and
  record it against the handoff generation skill with `parlay note` using kind
  `debt` or `idea`. Name the note's id in the reply so the thread and the item
  point at each other.

Two rules cut across the table:

- **`ask` never routes.** Answer it with a reply. Change nothing, in any file,
  whatever the reviewer seems to be implying. If answering reveals a change
  that ought to happen, say so in the `answer` and let them open a `do`.
- **A `do` that needs a decision you may not take alone** — retiring a founding
  promise, retiring a feature, a change whose affected set reaches other
  features — **stops at the gate that already exists** (refine's supersession
  gate, the loop's boundary). Reply `declined` and name the gate.

  This is the one place the annotation is *not* the confirmation, and the
  reason is the same reason those gates exist: an agent must not take that
  decision on the project's behalf, and a comment in a file is still an agent
  taking it — *unless the comment itself states the decision*. `@dwht: retire
  this intent — the promise is withdrawn` is the human's decision, recorded in
  their own words, and it passes the gate with that text as the confirmation.
  A comment that merely implies it does not.

### Reply, then re-read

```
parlay annotations reply <file>:<line> --kind done|answer|declined --by <you> --text "…"
```

Use the command rather than editing the comment yourself. It places the reply
at the request's own column in the host's own form, refuses text that would
break out of the comment, and re-reads the result before writing — three things
a hand-written reply gets wrong silently.

Then **re-run `collect-annotations`**. Every edit moves line numbers, and the
next thread is taken from a fresh scan. A stale list is how a reply lands on
the wrong thread.

## 3. Validate what moved

Per file changed: `parlay validate --type <type> <path>`. If a founding
document moved, `parlay internal check-coverage @{feature}`. If the ledger
grew, `parlay internal check-amendments @{feature}`.

## 4. Sweep, and hand the reading back

```
parlay annotations clear @{feature}
```

This removes the threads the reviewer already closed and nothing else, which is
why it is safe to run here without asking.

Then close with the list of `answered` threads — the ones you just replied to,
and any that were already answered when you started — and this sentence:

> Read the replies in place; write `close` under the ones you accept, or a new
> request under the ones you do not.

Say plainly that the feature's boundary stays blocked until every thread is
closed and swept, and that this is deliberate: no build reads a file with a
thread in it, because a build is what happens *after* review. Do not offer to
close them.

## What this is not

- **Not `parlay note`.** A note is work observed and not done, filed away from
  the text. An annotation is a request against text, in the text. If you find a
  request you should not act on now — out of scope for this feature, a new
  feature in disguise — reply `declined` and file a note; name the note's id in
  the reply so the thread and the item point at each other.
- **Not a review of your own.** You are acting on what a person wrote, not
  looking for more. If you notice something they did not, that is a note.
