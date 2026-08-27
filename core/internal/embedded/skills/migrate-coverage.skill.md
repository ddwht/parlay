---
name: migrate-coverage
description: "Answer the coverage judgments stranded by the retired coverage-review, one at a time"
surface: module
---

# Migrate coverage judgments

A retired `coverage-review.yaml` holds decisions somebody made: *this
requirement genuinely needs no test, because…* Nothing reads that file any
more, so each of those judgments is stranded — invisible to every check, and
still true or false in a way only a person can settle.

This walks a reviewer through them one at a time.

**You are not performing a migration.** The project's other migrators
transform a file mechanically and finish. This one asks a person to make a
series of judgments, and it is finished only when they have. If that feels
slower than a migration, that is the cost being reported honestly rather than
hidden.

## The rule that governs everything below

**Never let a reviewer reach "answered" without forming a judgment.**

A migration that converts real protections into rubber stamps is worse than no
migration: it leaves a ledger saying a person decided, which is the one claim
the whole system exists to make trustworthy. Every instruction here follows from
that, and where an instruction is inconvenient, it is inconvenient on purpose.

## Steps

### 1. See what is stranded

```
parlay internal migrate-coverage-exceptions @<feature>
```

Read `status`: `answered`, `pending_unreviewed`, `pending_deferred`,
`pending_total`. Tell the reviewer the shape before starting — thirty questions
nobody has looked at is a different prospect from thirty where twenty were
already examined and set aside, and they deserve to know which they are in.

If `pending_total` is zero, say so and stop.

### 2. Ask for the next one

```
parlay internal next-legacy-review @<feature> [--exclude <fingerprint> ...]
```

Pass `--exclude` once for every occurrence already handled **in this sitting**,
using the full `tokens.fingerprint` from when it was handled. Keep that list in
your own working memory for the duration of the run.

Without it you will eventually re-ask a question you just asked. Deferring does
not answer an entry, so a deferred occurrence stays pending — and once every
pending occurrence has been deferred, a bare call hands back the first one
forever.

The exclusion list is **ephemeral by design**. It is not written anywhere, so a
later run starts fresh and offers the deferred work again. That is the intended
behaviour: an unresolved question should come back.

When the command returns `done: true`, stop. Report `all_answered` honestly —
`true` means the migration is finished, `false` means this sitting is finished
and the entries left are still unanswered. Do not describe the second as
completion.

### 3. Show the reviewer the packet

Present `packet.display` **verbatim**. Do not summarise it, reorder it, or
rebuild it from the other fields.

It is ordered deliberately: what the contract requires *now* and what observes
it, and only then the prior reasoning under a label saying it is context. The
reviewer is deciding about the requirement as it stands today. The old reason is
evidence they weigh, not the thing under review, and leading with it invites
them to ratify a conclusion instead of reaching one.

Two things in that display do real work — leave them intact:

- **"NOTHING covers this today."** Frequently the reason the waiver exists.
  Rendering it as an empty list would hide the most important fact on the page.
- **"This waiver covers ALL N requirements."** An old entry naming only an
  operation may now span several. Re-confirming grants a waiver over every one
  of them, which is the thing most easily granted without noticing.

### 4. Ask for the decision

Build the chooser from `packet.decision`. Use its `question` and its three
`options` exactly — do not write your own alternatives, drop one, reorder them
to suggest a preference, or mark any as recommended. There is no default, and
`no_default: true` says so.

The three outcomes are closed:

| Option | Means |
|---|---|
| `reconfirm` | The requirement still genuinely needs no test |
| `drop` | It no longer holds; the requirement goes back to needing a test |
| `defer` | The reviewer looked and cannot say |

**`defer` is never a softer way to say `drop`.** If a reviewer cannot tell,
record a deferral — do not let them, or yourself, route uncertainty to `drop`
because it unblocks the pipeline. Withdrawing a protection is a decision
somebody has to own by name.

### 5. Ask for the reason, separately

After they choose, ask a **second** question for their rationale, in their own
words.

Do not prefill it. Do not offer the old reason as a starting point. Do not
accept the option's own description as the answer — a description they merely
selected is not a reason they gave. For `reconfirm` and `drop` this is the
substance of the decision; for `defer` it is what the next reviewer needs in
order to get further than this one did.

### 6. Record it

Forward `tokens.fingerprint`, `tokens.duplicate` and `tokens.legacy_file_hash`
from the envelope. Never ask the reviewer to read or type them; the short
`label` is for a person to recognise an entry by, and is not accepted here.

```
# reconfirm
parlay internal record-exception @<feature> --from-legacy \
  --legacy-fingerprint <tokens.fingerprint> \
  --legacy-duplicate <tokens.duplicate> \
  --legacy-file-hash <tokens.legacy_file_hash> \
  --ref <packet.subject ref> --criterion "<requirement>" \
  --kind waived --reason "<their words>" --by "interactive decision"

# drop
parlay internal drop-legacy-exemption @<feature> \
  --fingerprint <tokens.fingerprint> \
  --duplicate <tokens.duplicate> \
  --legacy-file-hash <tokens.legacy_file_hash> \
  --reason "<their words>" --by "interactive decision"

# defer
parlay internal defer-legacy-exemption @<feature> \
  --fingerprint <tokens.fingerprint> \
  --duplicate <tokens.duplicate> \
  --legacy-file-hash <tokens.legacy_file_hash> \
  --reason "<their words>" --by "interactive decision"
```

Then add `tokens.fingerprint` to your exclusion list and return to step 2.

### 7. If a write is refused

A refusal saying the retired review **changed after it was listed** means the
file moved underneath the sitting. The reviewer approved what they were shown;
recording now could answer a different occurrence than the one they judged.

**Start the sitting over from step 1.** Do not retry, do not recompute the
tokens, and do not carry the old exclusion list forward — the identities in it
may no longer mean what they meant. Decisions already recorded are safe; they
were written against the version that was current when each was made.

## Hard rules

- Present `packet.display` verbatim; build the chooser from `packet.decision`.
  Never assemble either yourself — the ordering and the closed outcome set are
  properties of the artifact, and reconstructing them locally is how they get
  quietly lost.
- Never carry a prior reason into a new decision, not even as a draft.
- Never treat a deferral as an answer, in your reporting or your counts.
- Never batch. There is no bulk confirm and there will not be one: forty
  authority-bearing decisions are forty decisions, and any affordance that makes
  them feel like one is the rubber-stamp path this exists to remove.
- Only a person answers. If you cannot ask — no interactive channel available —
  stop and say so. Do not answer on their behalf, and do not defer on their
  behalf either: a deferral is also a report about a person's state of
  knowledge, and you do not have theirs.
