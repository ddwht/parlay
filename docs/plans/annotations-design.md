# Annotations: anchored human comments, and the resolver that acts on them

**Status**: design proposal, 2026-09-02. Drafted by Claude against the tree at
`3784d6a`; reviewed by Codex, whose objections changed §3.1, §4.1, §4.2, §6.2
(handoff), §6.4, §7, and decisions B, C and D, and stand as recorded
disagreements on A and F, both settled by dwht in Claude's favour. Decision E
was revised on dwht's call to an explicit `close`. The design is approved for
implementation as of 2026-09-02. Nothing is implemented yet.

---

## 1. The problem

A person reviews a parlay file — an `intents.md` they wrote, a `surface.yaml`
the artifacts phase generated, an amendment refine drafted — and finds
something wrong. Today they have three ways to say so, and each loses
something.

**Edit the file directly.** On a founding document of a built feature this is
a `ledger_integrity` violation (`baseline.go:555`), and the skill text tells
them not to — "do not ask permission to edit them (the answer is structural,
not personal)". On a contract artifact it is legal but unrecorded: the change
lands, nothing says who wanted it or why, and the next `check-drift` reports
drift with no explanation attached. On an amendment it is forbidden outright
(append-only, "written once and NEVER edited").

**Say it in the conversation.** The loop's "Stay and revise" option resumes the
phase subagent with the word `stay` and nothing else; the reviewer then types
what is wrong in chat. The comment has no anchor — "the third constraint" is
resolved by the model, not by the file — and it evaporates with the session.
`parlay note` exists precisely because things said in conversation vanish, but
a backlog item is the wrong shape for "this bullet is wrong": it is a separate
file, it carries no position, and it is a record of *undone* work rather than a
request against a specific line.

**Write a `TODO` in the file.** The tree already does this in one place —
`multi-adapter/dialogs.md:258` describes stubs written "with a TODO marker for
designer review". Nothing finds these markers, nothing knows what text they
refer to, and nothing retires them.

What is missing is a **fourth route**: a comment written *in the file, next to
the text it is about*, in a syntax the tool can find, anchor, hand to the
model, and see resolved. The file the reviewer is reading becomes the inbox.

### What already exists to build on

- `intents.md` carries a `**Questions**:` field and `parlay internal
  collect-questions` gathers them; `check-readiness` reports `open-questions`
  as a warning. This is the closest precedent — a human-authored request that
  the tool collects — but it is one field, in one file type, with no anchor
  and no lifecycle.
- The intents parser already strips HTML comments (`intent.go:52`), for the
  stated reason that "a comment is the one construct whose whole purpose is
  'the tools should not read this'". The dialogs and infrastructure parsers do
  not (an HTML comment whose continuation line begins with `User:` or `- `
  parses as content). Founding-doc integrity is computed over **parsed**
  content (`hashIntent`, `hashDialogContent`), so a comment that the parser
  ignores is invisible to the freeze.
- Contract artifacts are hashed **whole-file** (`baseline.go:401-405`,
  `scaffold_signatures.go:105-110`), so any byte added to `surface.yaml` is
  drift to `diff` and a `stale-buildfile` refusal to codegen. Applied
  amendments are whole-file hashed too (`HashedSources.Amendments`), and
  there a moved hash is an integrity violation, not drift.
- Every kind of change a resolver could make already has a governed route:
  direct edit before first build, `/parlay-refine` after it, `amends_intents:`
  for a promise that reads differently, `supersedes:` for an amendment that was
  wrong. The resolver invents no new way to change a spec. It only feeds the
  existing ones from a new source.

---

## 2. Design principles

1. **The file is the inbox.** No side ledger of comments. An annotation lives in
   the file it is about, next to the text it is about, and is gone when the
   reviewer is satisfied. Git is the history.
2. **Invisible to every parser, visible to one scanner.** The syntax is a
   comment in the host format (HTML comment in Markdown, `#` comment in YAML),
   so no schema, validator or hash of parsed content changes. One scanner
   finds it.
3. **The anchor is positional and deterministic.** The comment goes *after*
   the text it is about, and a fixed rule says which text that is. No forward
   references, no ranges, no comment at the end of the file about the top.
4. **The annotation is the confirmation.** A person wrote an instruction in
   the file. The resolver acts on it without asking "are you sure?" — but
   every governance gate that already exists (supersession, retirement,
   amendment ceremony) still fires, because the annotation is a request for
   change, not a bypass of how change is recorded.
5. **The reply goes where the request was.** The resolver answers in the same
   syntax, directly beneath the annotation, so the reviewer reads the outcome
   in the place they raised it. The reviewer closes the thread with an
   explicit `close` entry, and only a closed thread is ever removed. The tool
   never decides that a human is satisfied.
6. **Routing follows the file's governance, not the annotation's wish.** The
   same comment on a frozen `intents.md` becomes an amendment; on an unbuilt
   one it becomes an edit. The reviewer does not need to know which.

---

## 3. The syntax

### 3.1 One grammar, wrapped in the file's own comment marker

An annotation is **one syntax** wherever it appears:

```
@<handle> [<kind>] [section] [ "<phrase>" ]: <text>
```

It sits inside whatever a comment already is in that file, on its own
line(s). That wrapper is the host format's, not parlay's: a YAML author
already writes `#` and never `<!--`; a Markdown author already writes `<!--`.
Everything from the `@` onward is byte-identical in both, so the rule to
remember is one sentence — *an annotation is `@you: text` inside a comment.*

```markdown
- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 -->
```

```yaml
verify:
  - Task text must be 200 characters or fewer
  # @dwht: product asked for 500
```

There is no wrapper both formats tolerate, which is why there are two. A
`#` line in Markdown is a heading. An HTML comment in YAML is a syntax error
to every YAML reader — editors, linters, `parlay validate`, the loaders in
`parser/` — unless each of them pre-stripped it, and a file that only parses
after one tool has rewritten it is not a YAML file. dwht raised the mental
load of two forms on 2026-09-02; the answer is that the form is one and the
marker is the reader's existing habit.

| Part | Rule |
|---|---|
| `@<handle>` | Required. Who is speaking: `[A-Za-z0-9_.-]+`. Free-form, unregistered — the same convention as `parlay note --by`. |
| `<kind>` | Optional bare word. Closed set, §3.3. Absent means **`do`**: a change is requested. |
| `section` | Optional scope word, Markdown only. Widens the anchor from the unit above to the **enclosing section** (§4.1). In a YAML file it is `annotation-malformed`; the column already says how wide a YAML comment is. |
| `"<phrase>"` | Optional double-quoted verbatim substring of the anchored unit. Narrows the anchor from the unit to a phrase inside it. |
| `:` | Required whenever there is text. `<!-- @dwht -->` with no colon and no kind is `annotation-malformed`, not a silent no-op — a reviewer who typed the sigil meant something. The one complete colon-less form is `@dwht close`. |
| `<text>` | The comment. May continue onto following lines inside the same comment (§3.2). Optional only after `close`. |

The words between the handle and the colon are parsed as a set, not by
position: each must be a kind or the scope word, a kind may appear once and
the scope once, in either order (`@dwht ask section:` and `@dwht section ask:`
are the same). Any other word is `annotation-word-unknown`. Codex asked for a
positional grammar or an `on section` form so that `section` could not be
misread as an unknown kind; the set rule answers the same concern with one
token fewer.

The host form is chosen by file extension. A longer example of each:

```markdown
- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 — see the ticket in the intent's Context -->
```

```yaml
      verify:
        - An intent with only Goal and Persona fields is valid
        # @dwht: also needs Priority — the validator refuses an intent without one
```

The sigil is `<!-- @` in Markdown and `# @` in YAML.

*Corrected during WP1, 2026-09-02.* The claim that "the trailing `:` after the
handle rules out an `@feature` ref that happens to open a comment" is false,
and scanning this repo proved it:
`core/.parlay/build/parlay-tool/page-layout-field/buildfile.yaml` carries a
wrapped prose comment opening `# @design-loop/design-loop respectively and
stay byte-equivalent`, which has no colon anywhere and still reached the
grammar. The real discriminator is the **slash**: a handle is
`[A-Za-z0-9_.-]+` and cannot contain one, so `@<something>/` is a ref and the
line is passed over in silence. Everything else opening with the sigil is a
candidate, broken ones included — a reviewer who typed the sigil meant
something. This is the collision Codex's decision-A objection predicted, found
in the tree rather than in the abstract; it is answered by a one-character
rule rather than by the namespace, and the namespace remains available if a
second collision ever appears.

### 3.2 Multi-line annotations

Markdown: the HTML comment may span lines; everything up to `-->` is the text.

YAML: consecutive `#` lines at the **same column** continue the annotation. A
line at that column that begins with `# @` starts a new annotation instead.

```yaml
  - id: task.create
    # @dwht: this is two operations — create, and the confirmation that
    #        returns the id. Split it; the surface fragment already assumes two.
```

### 3.3 Kinds

| Kind | Who writes it | Meaning | Resolver's action |
|---|---|---|---|
| *(absent)* = `do` | reviewer | Change this. | Make the change through the route §6.2 assigns; reply `done`. |
| `ask` | reviewer | I do not understand this, or I want to know why. | Answer in a reply; **edit nothing**. |
| `close` | reviewer | This conversation is finished. Text optional. | Nothing. A closed thread is what `annotations clear` and the boundary sweep remove. |
| `done` | resolver | The change was made. Text says what, and names the amendment when there is one. | — (a reply) |
| `answer` | resolver | The answer to an `ask`. | — (a reply) |
| `declined` | resolver | The change was not made. Text says why — usually a gate it could not pass alone (§6.3). | — (a reply) |

`do` and `ask` are **requests**; `done`, `answer` and `declined` are
**replies**; `close` is the **terminal** entry, and only a reviewer writes it.

*Clarified during WP1, 2026-09-02.* All six are **writable words**. The kind
set §8 names was abbreviated — it omitted `close`, which §3.4 plainly requires
— and `do` has a name in this table, so a reviewer who writes it down means
what a reviewer who leaves it out means. The closed set is
`{do, ask, close, done, answer, declined}`.
A reply is attached to the request it follows, not to the text. Kinds outside
the set are `annotation-word-unknown`. There is deliberately no
`ok`/`approved` kind for text that has no thread: approval of criteria
already has a home (`approve-criteria`), and "I looked and it is fine" leaves
no trace by design — the absence of a comment is the approval. `close` is not
that: it ends a conversation that exists.

### 3.4 Threads and replies

A reply is placed **immediately after** the request it answers, at the same
column, in the same host form. A request followed by one or more replies is a
**thread**. Further requests may follow a reply — the reviewer disagreeing
with what was done — and the thread continues.

*Amended twice during WP1, 2026-09-02; the second amendment reverses the
first.* "Immediately after" is **literal**: a thread is a run of consecutive
entries, with nothing between them, not even a blank line.

The first amendment made blank lines transparent, reasoning from §3.5, where
they are transparent to an anchor. Codex recommended the opposite and was
right, for a reason neither of us had at the start: it breaks the reopen path
this section documents. "Reopen by starting a new thread on the same text"
means writing a request under a closed thread — and with blank lines
transparent that request joins the closed thread, becomes
`annotation-after-close`, and **disappears from the listing** instead of being
read. A blank line is how a person separates two conversations, so it has to
separate them here. The cost is that a reply spaced away from its request is
`annotation-reply-orphaned`, which the reviewer sees and can fix; the benefit
is that nothing a reviewer writes is silently dropped.

```markdown
- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 -->
<!-- @claude done: raised to 500 in amendment 003-task-text-length; founding text untouched -->
<!-- @dwht: 500 is right but the error message in the dialog still says 200 -->
```

A thread's **state** is derived, never stored, from its last entry:

| Last entry | State | Meaning |
|---|---|---|
| a request (`do`, `ask`) | `open` | the resolver owes a response |
| a reply (`done`, `answer`, `declined`) | `answered` | the reviewer owes a reading |
| `close` | `closed` | the reviewer has said the conversation is over |

A reply with no request above it is `annotation-reply-orphaned`.

The reviewer closes a thread by **writing `close`** under it — in the editor,
or with `parlay annotations reply <file>:<line> --kind close --by <handle>`.
It may follow a reply (the usual case: read, accept, close) or a request
directly ("never mind"), and never nothing: a `close` with no entry above it
is `annotation-reply-orphaned`, because a thread born closed would be swept on
the next clear without anything having been asked or answered. Nothing after a `close` is allowed at that column:
a later entry is `annotation-after-close`, because a closed thread is about to
be removed and text under it would vanish with it. Reopen by starting a new
thread on the same text.

Only a **closed** thread is ever removed. `parlay annotations clear` (§6.1)
deletes the closed threads in a feature and nothing else; an `answered`
thread stays until the reviewer closes it, an `open` one until the resolver
answers and the reviewer closes it. Deleting the lines by hand remains
legal — it is the reviewer's file — but it is not what the tool does.

```markdown
- Task text must be 500 characters or fewer
<!-- @dwht: product asked for 500 -->
<!-- @claude done: raised to 500 in amendment 003-task-text-length; founding text untouched -->
<!-- @dwht close -->
```

### 3.5 Placement rules

- **Own line.** An annotation never shares a line with content.
  `User: hello <!-- @dwht: too terse -->` is `annotation-inline`. The reason is
  not aesthetic: the dialog parser would read the comment as part of the turn
  and the founding-doc hash would move.
- **After, never before.** The anchor is always above the annotation (§4).
  Blank lines between the text and the annotation are **transparent**: the
  scanner walks past them and past other threads. An annotation on line 1,
  or one with nothing above it but blank lines and other annotations, is
  `annotation-unanchored`.
- **Column matters in YAML.** The `#` column selects the anchor (§4.2). In
  Markdown the column is ignored except inside nested lists, where the
  comment's indentation selects the list level: the innermost enclosing item
  whose **marker** is at or left of the comment.

  *Amended during WP1, 2026-09-02.* This first said "exactly as a continuation
  line would", which means the item's CONTENT column — and that made a comment
  aligned with a nested item's dash select that item's **parent**, the exact
  opposite of §4.2's YAML rule, where outdenting to a dash widens and aligning
  with one selects it. Two opposite conventions for the same visual gesture is
  a defect in the syntax, not a detail of an implementation, so Markdown now
  reads the way YAML does: **align with the dash you mean.** Codex recommended
  the same rule independently.

---

## 4. Anchoring: which text a comment is about

The anchored **unit** is decided by the host grammar and the lines
immediately above the annotation. The rule is fixed so that a reviewer can
predict it and the scanner can report it; the resolver then hands the model
the unit's text and its structural identity.

### 4.1 Markdown

Walk up from the annotation, skipping blank lines, other annotations and
replies, to the nearest content line.

| That line is… | Anchored unit |
|---|---|
| a heading (`#`, `##`, `###`, `####`) | **the heading line itself** — the title. Not the section: the text under it comes *after* the comment, and a comment is never about text it precedes. |
| a list item, or a continuation line of one | that list item, including its continuation lines and any nested items |
| a `**Field**:` line (`**Goal**:`, `**Trigger**:`, `**Behavior**:`…) | that field — the line plus, for list-valued fields, the bullets under it |
| a dialog turn (`User:`, `System:`, `System (background):`, `System (condition: …):`) | that turn, including its indented `A:`/`B:` option lines |
| any other non-blank line | the paragraph: the contiguous run of non-blank lines ending there |
| frontmatter `---` closing line | the frontmatter block |
| a `---` section separator | `annotation-unanchored` — the separator is not text; put the comment above it |

The scope word **`section`** replaces the unit with the **enclosing section**:
from the nearest heading above the annotation to the next heading of equal or
higher level or the next `---`. This is the only way to comment on a whole
intent, dialog or fragment, and it is explicit on purpose. An earlier draft
widened to the section whenever a blank line separated the comment from the
text; Codex objected that a reviewer who habitually leaves a blank line
before a comment would silently comment on the whole intent, and that a
comment directly under a heading would claim text it has not reached yet.
Both objections hold. So: a comment about one constraint goes under the
bullet, blank line or not; a comment about the whole intent says so.

```markdown
**Verify**:
- A malformed item is refused with a published code and a fix
- The item names the run that produced it

<!-- @dwht section: this intent overlaps "Decide what to do about one item" — merge or split the personas -->
```

### 4.2 YAML

The `#` column is the selector. Walk up from the annotation, skipping comments
and blank lines, to the first line whose content starts at a column **less
than or equal to** the annotation's column. That line opens the anchored node:

| That line is… | Anchored unit |
|---|---|
| `- ` item | the item and its whole subtree |
| `key:` with a block value | the key and its whole subtree |
| `key: scalar` | that pair |
| the document start (no such line) | the whole document |

```yaml
      verify:
        - criterion A
        - criterion B
        # @dwht: B duplicates A            ← column of "- " → the item "criterion B"
      # @dwht: none of these are testable   ← column of "verify:" → the whole verify list
```

Column selection is exactly how a YAML author already reads comments, which
is why it needs no new convention. Frontmatter in `*.page.md` and amendments
is YAML and uses this rule; the Markdown body below it uses §4.1.

Four YAML shapes need a stated rule, all raised by Codex:

- **`- key: value` (a mapping opened on the dash line).** That line has two
  starts: the item at the dash's column and the first pair two columns in.
  A comment at the dash's column anchors to the whole item; a comment at the
  key's column directly under the dash line anchors to that first pair, and
  under any later `key:` line to that pair. Commenting on the whole item
  therefore means outdenting to the dash — the same move a YAML author makes
  to add a sibling item, which is why it reads as intended.
- **Block scalars (`notes: |`, `>`).** Every indented line under the
  indicator is **scalar content**, including one that begins with `# @`. The
  scanner tracks block scalars and does not read inside them; a sigil found
  there is `annotation-in-block-scalar`, with the fix "place it at the key's
  column after the scalar ends". This matters here: `capabilities.yaml`
  carries `notes: |` on nearly every operation, and a comment inside one
  would otherwise be ingested as the operation's notes by the parser and
  invisible to the scanner at the same time.
- **Flow collections (`[a, b]`, `{k: v}`), single- or multi-line.** One unit:
  the pair that opens the collection. The phrase narrows inside it.
- **Tabs.** Not indentation in YAML; a comment line indented with a tab is
  `annotation-malformed` rather than anchored by a column the parser would
  refuse.

### 4.2.1 Two rules WP1 had to reconcile

*Added during WP1, 2026-09-02; both raised by Codex.*

**The document row and `annotation-unanchored` overlapped.** §4.2 gives "no
qualifying line above" the meaning *the whole document*; §3.5 says an
annotation with nothing above it is `annotation-unanchored`. They disagree
only about the top of a file, and §3.5 wins there — the anchor is always
above, so a comment at the top of a file is about nothing yet, and a comment
about the whole document goes at the bottom at column 0. The document row
keeps the case it was written for: content above, but nothing opening a node
at or left of the column.

**An anchor's text never contains the thread.** The span names raw lines,
because a span is for a person opening the file; the TEXT is built from what a
parser can see there, so comment lines — the thread's own entries included —
are not in it. Codex called this fatal for `section` and it is: `section`'s
span runs forward from its heading and therefore covers the annotation itself,
so a quoted phrase could narrow against words the reviewer wrote in their own
request, and `annotation-phrase-not-found` would never fire on a phrase that
existed nowhere but the comment.

**A unit never claims text below the annotation.** The heading row already
says so ("a comment is never about text it precedes"), but a YAML subtree and
a Markdown field or list item can each run PAST a comment written in the
middle of one. Every span is therefore clamped to the line above the request.
The single exception is the `section` scope word, which §4.1 defines forward
from its heading on purpose.

### 4.3 Phrase narrowing

`@dwht "200 characters": too low` anchors to the unit §4.1/4.2 selects and
then to the first occurrence of the quoted text inside it. The phrase must
occur verbatim; otherwise `annotation-phrase-not-found`, reported with the
unit's text so the reviewer can fix the quote. The phrase is a hint to the
model about *where in the unit*, not a range selector — it does not extend
past the unit.

### 4.4 What the scanner reports for an anchor

Every anchor carries a **generic** identity that any Markdown or YAML file
can supply, plus a **ref** when the file is one parlay understands:

| File | Generic | Ref (when resolvable) |
|---|---|---|
| any `.md` | heading path (`## Add Task › **Constraints**`), line span | — |
| any `.yaml` | YAML path (`fragments[1].verify[0]`), line span | — |
| `intents.md` | as above | `@feature/intent:<slug>` + field (+ item index) |
| `dialogs.md` | as above | `@feature/dialog:<slug>` + branch + turn index |
| `surface.yaml` | as above | `@feature/surface:<fragment-slug>` + path |
| `capabilities.yaml` | as above | `@feature/operation:<id>` + path |
| `infrastructure.md` | as above | `@feature/infrastructure:<heading-slug>` + field |
| `domain-model.yaml` | as above | `@feature/domain:<entity>` (or the root model's) |
| `amendments/NNN-slug.md` | as above | `@feature/amendment:<slug>` + section |
| `*.page.md` | as above | page name + region / layout node id — **not** feature-qualified (WP2): `affects:` has no `page` kind, so `@<feature>/page:<name>` would be shaped like an amendment target no amendment could name. A page is project-owned, multi-feature, and scanned once at project level rather than per feature. |

The ref column reuses the `affects:` vocabulary (`operation | surface |
infrastructure | domain`) so that a resolution which becomes an amendment can
name its dirty set without translation.

---

## 5. What a parser must do

Nothing, for YAML: `yaml.v3` drops `#` comments, and `Unmarshal` is what every
YAML loader in `parser/` uses.

For Markdown, **every** structural parser must skip HTML comments the way
`intent.go` already does. Today that is one of four:

| Parser | Today | Required |
|---|---|---|
| `parser/intent.go` | strips HTML comments (`stripComments`) | unchanged |
| `parser/dialog.go` | no comment handling — a comment line is ignored only because it matches no prefix; a continuation line starting `User:` becomes a turn | apply `stripComments` before every rule |
| `parser/infrastructure.go` | same shape as dialog | same fix |
| `parser/page.go` | body sections; `## Layout` fenced YAML | strip in the body; the fence content is YAML and needs nothing |
| `parser/amendment.go` | frontmatter + sections | strip in the body |

This is a correctness fix independent of annotations: a commented-out dialog
parses as a real one today, which is the same bug `intent.go` fixed for
intents. Doing it first means annotations arrive into parsers that already
cannot see them, and `hashDialogContent` cannot move when one is added.

### 5.1 Code is not a comment

*Added during WP0, 2026-09-02, on evidence from the tree; agreed by Codex.*

A comment opener inside an **inline code span** or a **fenced code block** is
content, in Markdown and here. This is not a nicety. The
`claude-md-section-preservation` feature quotes `` `<!-- parlay:begin -->` ``
inside backticks in its `intents.md`, `dialogs.md` and `infrastructure.md` —
that is what the marker looks like when a spec is *talking about* the marker.
Stripping there deletes a quotation out of the middle of a frozen promise: the
naive rule produced a **new** `ledger_integrity` finding on that feature's
`dialogs.md`, and the same defect already present in `intent.go` turns out to
be the cause of the pre-existing false finding on its `intents.md`, which the
exemption clears. A ledger that accuses a reviewer of editing text nobody
touched is worse than one that reads a comment.

The rule binds the scanner as tightly as the parsers, and for the same reason
in reverse: a document that quotes the annotation syntax — this design, the
schema of §WP4, a dialog about annotations — must not be scanned as carrying
one. Both directions are the same sentence: **inside code, the sigil is a
picture of a sigil.** One lexer answers it for both readers, so the same bytes
can never be content to the parser and an actionable request to the resolver.

The inline-code half is a **deliberate approximation**, not CommonMark.
CommonMark resolves a code span by looking for a matching backtick run
anywhere in the paragraph; this scanner looks only on the line. Carrying span
state across lines would be the wrong approximation of that rule rather than a
closer one: a single stray backtick in prose would swallow every following
line until the next backtick, hiding real comments from the parsers and real
annotations from the scanner with no finding to say so. Line-local matching
fails the other way — a comment marker inside a genuinely wrapped code span is
read as a comment — and a wrapped span that needs the exemption can be written
as a fenced block. A false negative on the exemption is recoverable; a silent
disappearance is not.

The re-trim after stripping is symmetric for the same reason. A line the
stripper touched is trimmed on both ends, so a trailing space left where an
inline comment used to be cannot end up inside a bullet's text and move a
founding hash. Lines the stripper did not touch are returned byte-for-byte.

Consequence worth stating plainly: **an annotation in a frozen founding
document is not a `ledger_integrity` finding.** The freeze hashes parsed
content, the parsers cannot see comments, so the hash does not move. Writing a
comment into `intents.md` after first build is legal, and so is the resolver
writing a reply there. Changing the *text* remains what it always was.

---

## 6. The resolver

Two layers, following the project's shape: a CLI that finds and reports
(JSON, no judgement), and a skill that decides and acts.

### 6.1 CLI

**`parlay internal collect-annotations [@feature] [--all]`** — the probe.
Scans every human-facing file of the feature (or every feature; `--all` adds
project-level files: root `domain-model.yaml`, `blueprint.yaml`, adapters,
`adapter-set.yaml`) and emits every thread with its anchor:

```json
{
  "feature": "task-list",
  "threads": [
    {
      "file": "spec/intents/task-list/intents.md",
      "line": 14,
      "state": "open",
      "frozen": true,
      "anchor": {
        "unit": "list-item",
        "span": [13, 13],
        "heading_path": ["Add Task", "Constraints"],
        "ref": "@task-list/intent:add-task",
        "field": "Constraints",
        "index": 1,
        "text": "- Task text must be 200 characters or fewer",
        "phrase": null
      },
      "entries": [
        {"by": "dwht", "kind": "do", "text": "product asked for 500", "line": 14}
      ]
    }
  ],
  "findings": [],
  "counts": {"open": 1, "answered": 0}
}
```

`frozen` says whether the file is under the ledger freeze for this feature
(a baseline exists and the file is `intents.md` or `dialogs.md`) or is an
applied amendment — the two facts the skill's routing needs. `findings`
carries the error codes of §8 for malformed annotations, so a broken sigil is
reported rather than skipped; a file with findings still reports its
well-formed threads.

Mirrors `collect-questions` in placement and shape, for the same consumers.

**`parlay annotations list [@feature]`** — the human-facing listing, in the
`backlog list` style: file, line, state, and — prominently, before the
request's text — the **resolved anchor**: the ref or YAML path and the first
line of the anchored unit. This is the reviewer's check that the column or
position selected what they meant, and it comes before anything acts on it.
Open first.

**`parlay annotations reply <file>:<line> --kind done|answer|declined --by <handle> --text "…"`**
— writes a reply beneath the request at `<line>`, in the host form and at the
column the grammar requires. The skill uses this rather than editing the
comment text itself, so a reply is always well-formed and always placed where
§3.4 says. Refuses (`annotation-reply-orphaned`) if `<line>` is not a request
or a reply.

**`parlay annotations clear [@feature] [--file <path>]`** — deletes every
`closed` thread. Never an `open` or `answered` one. Because closure is an
explicit entry a reviewer wrote, this command carries no judgement and is
safe to run unattended: the build and code skills run it before their gates
(§6.4), and the resolver runs it at the end of a pass. Reports what it
removed.

**`parlay validate`** runs the scanner on every Markdown and YAML file it
already validates and surfaces §8 findings alongside the file's own. A
well-formed annotation is not a validation finding of any severity — a review
in progress is a healthy state.

### 6.2 Routing: what "make the change" means

The route is decided by the **file's governance**, then by the feature's
state. The reviewer writes the same comment either way.

| File | Feature has no baseline (never built) | Feature has a baseline (built) |
|---|---|---|
| `intents.md`, `dialogs.md` | **Direct edit.** These are designer-authored files, and the annotation is the designer's written instruction — the permission CLAUDE.md asks for. Re-run `check-coverage`; a changed intent may orphan a dialog. | **Never edited.** The request becomes an amendment through `/parlay-refine`: `amends_intents:` with the mode that fits (`revise` for changed wording, `extend`/`narrow` for scope, `retire` only through the supersession gate). Reply `done` names the amendment. |
| `surface.yaml`, `capabilities.yaml`, `infrastructure.md`, feature `domain-model.yaml` | **Direct edit**, then `parlay validate --type …`. | **`/parlay-refine`** with the annotation as its prose and `trigger: "annotation by <handle> on <ref>"`. Refine's steps run unchanged: amendment first, then the splice. Reply `done` names the amendment. |
| `amendments/NNN-slug.md` | *(unapplied)* **Atomic in-place edit**, preserving `amendment:` slug and sequence number — a never-applied record has no baseline hash, so append-only integrity has nothing to anchor on (`spec-evolution-design.md` §"The wall"), but backlog `becomes:` values, later `supersedes:` and `trigger:` refs may already name it. Refused while a refine journal (`refine-journal --amendment`) names that record: an interrupted refinement resumes against the text it wrote. | *(applied)* **Annotations are refused here** (`annotation-in-applied-record`, §7). The comment belongs on the contract entry the amendment changed, or becomes a new amendment through refine that names the old one in `supersedes:`. |
| `*.page.md`, `blueprint.yaml`, adapters, `adapter-set.yaml`, `authored.yaml` | Direct edit, then the file's validator. Project-owned; not ledgered. | same |
| `spec/handoff/<feature>/specification.md` | The handoff is derived, and stays derived. Every annotation routes to the artifact the passage was derived from (by that file's row), then the handoff is regenerated. A comment about presentation alone — wording, ordering, layout of the handoff itself — is a comment about the generator: the resolver replies `declined`, names the reason, and files a `parlay note` (kind `debt` or `idea`) against the handoff generation skill. Never a direct edit; the next regeneration would erase it. | same |

Two rules cut across the table:

- **`ask` never routes.** It is answered with a reply and nothing else
  changes, whatever the file.
- **A `do` that turns out to need a decision the resolver may not make alone**
  — retiring a founding promise, retiring a feature, a change whose
  `affected-set` reaches other features — stops at the gate the existing skill
  already has (refine step 3.5's supersession gate, the loop's boundary), and
  the resolver replies `declined` with the gate named. The reviewer either
  takes the decision in the interactive flow or writes a more specific
  request. This is the one place the annotation is *not* the confirmation:
  the gates that exist today exist because an agent must not take those
  decisions on the project's behalf, and a comment in a file is still an
  agent taking it unless the comment itself states the decision
  (`@dwht: retire this intent — the promise is withdrawn`). Then it is the
  human's decision, recorded in their own words, and the gate is passed with
  that text as the confirmation.

### 6.3 The skill: `/parlay-resolve`

One new skill, source at `core/internal/embedded/skills/resolve.skill.md`,
deployed as `.claude/skills/parlay-resolve/SKILL.md`. Arguments: an optional
feature; omitted, every feature with open threads, one feature at a time.

Steps:

1. `parlay internal collect-annotations @feature`. Report the counts and any
   findings. Malformed annotations are shown to the user with the fix and
   **not** guessed at.
2. For each `open` thread, in file order:
   - Show the **resolved unit first** — ref or path, span, and its text —
     then the thread. Codex's point stands: YAML column selection will
     occasionally surprise a reviewer even with §4.2's shapes, so the unit
     the scanner chose is stated in the reply (`done` and `answer` texts open
     by naming it) where the reviewer will read it, and a wrong anchor is
     visible in place rather than discovered in the diff.
   - Decide the route from §6.2 using `frozen` and the feature's baseline.
   - Act: edit, or invoke refine's steps, or answer.
   - `parlay annotations reply … --kind done|answer|declined`.
   - **Re-run `collect-annotations`.** Every edit moves line numbers; the
     next thread is taken from a fresh scan, never from the stale list.
3. Validate what changed (`parlay validate --type …` per file;
   `check-coverage` if a founding doc moved; `check-amendments` if the ledger
   grew).
4. Run `parlay annotations clear @feature` — removing only what the reviewer
   already closed — then close with the list of `answered` threads and the
   sentence: *read the replies in place; write `close` under the ones you
   accept, or a new request under the ones you do not.* The skill never
   closes a thread.

Interactive versus subagent follows the CLAUDE.md rule: invoked directly it
has `AskUserQuestion` for the §6.2 gates; as a subagent it returns a
`parlay-decision` block. Nothing in step 2 asks "shall I?" — the annotation
answered that.

### 6.4 Where the existing skills pick it up

- **`/parlay-loop`, "Stay and revise".** Today `stay` resumes the phase with
  no payload. With annotations, the natural review loop is: the driver shows
  the boundary, the designer annotates the artifacts *in the files*, chooses
  Stay, and the resumed phase subagent runs §6.3 steps 1–3 on the feature
  before doing anything else. The boundary's `context:` already carries gap
  analysis; it gains the thread counts. Closed threads are swept before the
  question is asked. When answered threads remain, the boundary question
  gains a fourth option — **Review answered threads** — and the driver walks
  them one at a time, the way `parlay backlog` walks items: it shows the
  anchored text, the request and the reply, and asks *close* or *keep*. Close
  writes the `close` entry through `annotations reply`; keep leaves the
  thread answered, and the reviewer writes their follow-up request in the
  file. Then the driver sweeps and re-presents the boundary. An open thread
  has no shortcut: the boundary stays blocked until the phase resolves it.
  The domain-model review pause
  (step 11) is the same shape — "edit the YAML directly" — and annotations are
  the way to say what to edit without editing it.
- **`/parlay-refine`.** Gains one input form: a thread ref instead of prose.
  Everything after step 1 is unchanged. `trigger:` records the annotation.
- **`/parlay-doctor`.** Survey step adds `collect-annotations` beside
  `collect-questions`, and offers `/parlay-resolve` as the fix.
- **`check-readiness` and `gate`.** New codes `open-annotations` and
  `answered-annotations`, both errors, emitted for the `build-feature` stage
  and for code emission, and aggregated by `parlay gate`. A `closed` thread
  is not a finding: the build and code skills run `annotations clear` before
  their gates, so closed threads are gone before the check runs, and a
  direct `parlay gate` reports them as `closed-annotations` at info severity
  with the sweep as the fix. The loop's prompt is a convenience over these,
  not the enforcement: `build-feature` already runs
  `check-readiness --stage build-feature` at its step 6, so a direct
  `/parlay-build-feature` blocks the same way; `generate-code` has no
  readiness call today — its gate is the signature check at step 11.6 — and
  gains the sweep and an explicit `collect-annotations` refusal beside it,
  so a direct codegen run never signs or emits over a thread either. Codex asked for
  exactly this: the rule must hold for the direct commands, not only the
  loop UI.
- **`parlay note`.** Unchanged. The two are different things and stay apart:
  a note is work *observed and not done*, filed away from the text; an
  annotation is a request *against* text, in the text. A resolver that finds
  a request it should not act on now (out of scope for the feature, a new
  feature in disguise) replies `declined` and may file a note — the reply
  names the note id, so the thread and the item point at each other.

---

## 7. Interaction with hashing and drift

Two hash regimes, two different answers.

**Founding documents** (`intents.md`, `dialogs.md`): hashed over parsed
content. With §5 done, annotations and replies are invisible. No drift, no
integrity finding, no ceremony to write a comment in a frozen file. This is
the property that makes the design work on the files where it matters most.

**Everything the buildfile signs** is hashed over **bytes**, in two regimes:
the advisory `HashedSources` that `diff` reads (`surface.yaml`,
`capabilities.yaml`, `infrastructure.md`, the root domain model, the adapter),
and the hard `source-signatures:` gate that refuses codegen with
`stale-buildfile` (generate-code step 11.6). The hard gate's inputs are
`intents`, `dialogs`, `surface`, `domain`, `layout` and `adapter-version` —
so it covers the founding documents too, as raw bytes, even though the ledger
freeze reads them parsed. An annotation anywhere in the feature's files moves
at least one signature.

Codex located the defect in the first draft's answer ("accept it, make the
gate say why"): with replies left in place until the reviewer clears them, a
rebuild after resolution signs the reply bytes into the buildfile, and the
reviewer's later clear makes that buildfile stale again for no change in
meaning — one rebuild per thread cleared, forever; and if the rebuild came
before the reply, the reply itself stales it. Codex's own preference is to
canonicalise — hash with thread lines stripped, one shared CLI helper owning
the canonical form, every consumer calling it — and to gate open `do`
threads separately. That cures it, at the cost of teaching three Go sites
and a skill step the same stripping rule, and of a hard gate that
deliberately ignores some bytes.

The design does neither. **No build and no emission reads a file with a
thread in it.** Threads are review state; a build is what happens after
review. So the designer→build and build→code boundaries block on any thread,
`open` or `answered`, in any of the feature's files (`open-annotations`,
`answered-annotations`); the reviewer closes each thread explicitly, and the
skills sweep closed threads before their gates. Codex agreed the route
is coherent on exactly these two conditions — every state blocks both
boundaries, and the direct commands enforce it, not only the loop (§6.4).
The consequences fall out cleanly:

- A `do` thread resolved, closed and swept leaves changed content and no
  comment lines: the drift is real and the rebuild is the one the change
  needed.
- An `ask` thread answered, closed and swept restores the bytes exactly: no
  drift.
- Between annotating and the sweep, `diff` reports the file changed, which it
  has; `check-drift` (WP6) names the thread count beside the file so the
  reviewer sees their own comment as the cause rather than a phantom edit.
- The buildfile never carries a signature over comment bytes, so the sweep
  never stales it.

**Applied amendments** are the third case, and the one where a comment is not
merely drift but a false accusation. Every applied record's bytes are hashed
into `HashedSources.Amendments` at save time and re-checked by `check-drift`,
`apply-amendment`, compaction and the applied-history reader
(`applied_history.go:45`, `compact_txn.go:238`, `apply_amendment.go:413`,
`scope_dispositions.go:429`); a moved hash means "an amendment was edited or
deleted after being recorded" and blocks the ledger. Canonicalising would
touch every one of those readers and weaken the one hash whose whole purpose
is to notice any byte moving. So the scanner refuses the annotation instead:
`annotation-in-applied-record`, with the fix "comment on the artifact entry
this amendment changed, or open a superseding amendment". An **unapplied**
amendment has no recorded hash and may carry annotations like any other file.

§9 axis C records the decision.

---

## 8. Error codes

New codes, reported by `collect-annotations` in `findings` and by
`parlay validate` for the file:

| Code | Fires when |
|---|---|
| `annotation-malformed` | A comment opens with the sigil (`<!-- @` / `# @`) but does not match the grammar — no colon, a handle with illegal characters, an unterminated quote. |
| `annotation-word-unknown` | A word between the handle and the colon is neither a kind in `{do, ask, close, done, answer, declined}` nor the scope word `section`, or a kind or the scope appears twice. |
| `annotation-in-block-scalar` | The sigil appears inside a YAML block scalar (`\|` or `>`), where it is content. Fix: place it at the key's column after the scalar ends. |
| `annotation-inline` | The annotation shares a line with content. |
| `annotation-unanchored` | Nothing above it but the top of the file, blank lines, or other annotations. |
| `annotation-phrase-not-found` | The quoted phrase does not occur in the anchored unit. The finding carries the unit's text. |
| `annotation-reply-orphaned` | A reply kind with no request (or reply) directly above it at the same column. |
| `annotation-reply-column` | A reply at a different column than the request it follows — in YAML this would silently re-anchor it to the text. |
| `annotation-after-close` | An entry follows a `close` at the same column. The closed thread is about to be removed; start a new thread instead. |
| `annotation-in-applied-record` | An annotation inside an amendment the baseline has already applied. The file's bytes are under integrity hash; the comment goes on the contract entry the amendment changed, or into a superseding amendment. |

Readiness / gate:

| Code | Fires when |
|---|---|
| `open-annotations` | The feature has at least one `open` thread. Blocks designer→build and build→code. Message carries the count and the first file. Fix: `/parlay-resolve @feature`. |
| `answered-annotations` | The feature has answered threads and no open ones. Blocks the same boundaries. Fix: read each reply in place and write `close` under it, or a new request. |
| `closed-annotations` | Closed threads are still in the files. Informational; the build and code skills sweep them before their gates. Fix: `parlay annotations clear @feature`. |

None of these is emitted for a well-formed thread in any state.

---

## 9. Decisions

Each axis states the options considered and the choice made. These are final
for the first implementation.

**A. Sigil.** `@<handle>` with a mandatory colon, inside the host's comment
form. Considered: a dedicated token (`>>`, `!!`), a keyword (`review:`,
`note:`), and a bare `@handle`. The handle-as-sigil makes attribution
structurally mandatory — the property the backlog schema calls out as the
one that makes follow-up possible — and the colon is what keeps `@feature`
refs from matching. `note:` collides with the backlog vocabulary and with the
existing `notes:` field in artifacts. **Chosen: `@handle:`.**

*Codex disagreed* and prefers a namespaced sigil (`<!-- parlay @dwht: -->`
or similar), on the ground that absence from today's tree does not establish
collision safety in future human prose. The risk is bounded by scope: the
scanner reads only the spec tree and project-level parlay files, never code,
and the shape it requires — a comment opener, `@`, a handle, then a colon —
is one nobody writes by accident in a spec. If a collision ever appears, the
grammar can gain the namespace word without changing anything else; the
opposite migration would be a rewrite of every annotation in every project.
dwht chose the bare `@handle:` on 2026-09-02.

**B. Anchor rule.** Considered: previous line only; explicit ranges with
start/end markers; quoted-phrase-only. Previous-line is too narrow for a
multi-line `**Behavior**:` field and cannot say "this whole intent". Ranges
were excluded by the brief (comments go after the text) and add a second
marker the reviewer has to place. **Chosen: §4's unit rule — block by
position, node by YAML column, blank lines transparent, and the explicit
`section` scope word for a whole section — with an optional quoted phrase to
narrow.** The first draft widened to the section on a blank line and on a
heading; Codex's objection (§4.1) was accepted, as were its four YAML shapes
(§4.2).

**C. Whole-file hashes.** Options: accept the byte drift and make the gate
say why; canonicalise every signature through one shared helper; keep
threads out of builds. The first was the draft's choice and Codex showed it
produces a spurious `stale-buildfile` on every clear (§7). The second is
Codex's preference; it spreads a stripping rule across every hash consumer
including a skill step, and makes a hard gate ignore bytes on purpose.
**Chosen: threads never survive into a build.** Boundaries block on any
thread; the reviewer clears first. Nothing about hashing changes.

**D. Severity at a boundary.** Options: warning like `open-questions`; error
for `do` and warning for `ask`; error for every thread in every state. The
draft chose the middle. Axis C forces the third: an answered `ask` thread on
`surface.yaml` is bytes the buildfile would sign. Beyond the mechanics it is
also the right reading — a person has written in the very text the next phase
builds from, and advancing over it, whether the comment is a request or a
reply they have not read, builds on a review that is not finished. **Chosen:
every thread blocks designer→build and build→code, enforced in
`check-readiness`, the codegen skill and `gate`, not in the loop prompt.**
Closing is one entry per thread, written by the reviewer; a reviewer who
wants to advance without reading a reply has to write `close` under it, an
act they can see themselves take.

**E. Reply in place, human closes, tool removes only the closed.**
Considered: delete the annotation on resolution (loses the record of what was
done, and the reviewer cannot check it); rewrite the annotation into a
resolved form (same as reply, less readable); a side ledger (a fifth
per-feature file to keep consistent, and a second inbox); and the first
draft's rule, where `clear` removed every answered thread as bulk acceptance.
dwht rejected the last on 2026-09-02: an answered thread the reviewer has not
read is indistinguishable from one they accept, so the command was deciding
something a person had not said. **Chosen: reply in place; the reviewer
writes an explicit `close`; the tool removes closed threads and nothing
else.** Closure is now a fact in the file rather than an inference from the
command being run, which is what makes the sweep safe to run unattended
before a build.

**F. Unapplied amendments may be rewritten.** CLAUDE.md says an amendment is
"written once and NEVER edited". That rule protects append-only integrity,
and integrity is anchored on hashes the baseline recorded at apply time
(`HashedSources.Amendments`); a never-applied record has no such hash and
`spec-evolution-design.md` already relies on deleting one being clean.
**Chosen: an annotation on an unapplied amendment is resolved by an atomic
in-place edit that preserves the record's slug and sequence, refused while a
refine journal names the record; an annotation inside an applied one is
refused, and the change it wanted goes through a superseding amendment.** The
CLAUDE.md sentence gains the qualifier "once applied". The refusal on applied
records rather than a canonicalised hash: an applied record's byte hash is
re-read in at least five places (§7), and the property it guards — no byte of
recorded history moves — is exactly the property a comment in the file would
break.

*Codex disagreed on the unapplied half* and would keep the invariant whole:
resolve by a superseding amendment, or introduce an explicitly mutable draft
status. Its located risks — refs from backlog `becomes:`, later `supersedes:`,
`trigger:` and the refine journal — are real and are what the slug-preserving
and journal-refusing conditions answer. The remaining disagreement is one of
principle: whether a record nobody has applied is history. This design says it
is a proposal in its review window; the apply ceremony that shows the human
the text and binds a digest to it is where it becomes history. dwht chose
in-place editing on 2026-09-02.

**G. No per-annotation confirmation.** The annotation is the instruction; the
resolver acts and replies. The existing gates (§6.2) are the only pauses.
Considered and rejected: an accept/skip prompt per thread — it would turn a
review of twenty comments into twenty prompts and reintroduce the
conversation the annotation was meant to replace.

**H. One skill, three integration points.** `/parlay-resolve` is the front
door; loop-stay, refine and doctor call into it or its CLI. Considered:
folding the whole thing into refine (wrong for unbuilt features, where there
is no amendment) or into doctor (doctor diagnoses; it does not rewrite spec
text). **Chosen: a dedicated skill.**

---

## 10. Implementation plan

Source-first, per the dogfooding rule: edit under `core/internal/embedded/`,
`make build`, `./parlay upgrade`.

**WP0 — parsers cannot see comments.** Extract `stripComments` from
`intent.go` into `parser/comments.go`; apply it in `dialog.go`,
`infrastructure.go`, `page.go` (body) and `amendment.go` (body). Tests: a
commented-out dialog / fragment / section parses as absent;
`hashDialogContent` is unchanged by a comment. Independently shippable and
worth shipping first.

**WP1 — the scanner.** `parser/annotation.go`: host detection by extension,
grammar, thread assembly, generic anchoring (§4.1, §4.2, §4.3) including
block-scalar tracking, §8 findings.
Pure functions over lines; table-driven tests per host and per anchor rule,
including the blank-line-widens-to-section case, the YAML column case, and
each error code.

**WP2 — ref resolution.** `commands/annotations_refs.go`: map a generic anchor
to a parlay ref for the seven known file types by reusing the existing
parsers' slug rules (`Slugify` on headings, fragment `name:`, operation
`id:`). Unknown files keep the generic identity.

**WP3 — CLI.** `parlay internal collect-annotations`, `parlay annotations
list|reply|clear` (clear removes closed threads only); `frozen` computed from the baseline's presence, and an
amendment's applied state from `.baseline.yaml`'s `last-applied-amendment`
and `HashedSources.Amendments` (`baseline.go:57`, `:255`). `reply` and `clear` write through
`atomicfile`. `validate` runs the scanner for `.md`/`.yaml` inputs.
`check-readiness` and `gate` emit `open-annotations` and
`answered-annotations` per §9-D; the generate-code skill gains the explicit
refusal beside its signature gate.

**WP4 — schema and digest.** `embedded/schemas/annotation.schema.md` with the
grammar, anchor rules and the two error tables in `<!-- parlay:normative -->`
blocks so the digest derives; `DIGEST.md` regenerates on upgrade.

**WP5 — the skill and the integration points.** `resolve.skill.md`; the
loop's Stay path and boundary context; refine's thread-ref input and
`trigger:` form; doctor's survey line; the deployer's command table in
CLAUDE.md; the qualifier on the amendment ownership sentence.

**WP6 — drift honesty.** `check-drift` names the thread count beside any
signed file that carries one; `check-readiness` orders the annotation codes
before drift findings, so a reviewer sees their own comment named as the
cause before they see `stale-buildfile`.

Each WP lands with its own tests; WP0 and WP1 have no dependency on each
other and can go in parallel.

---

## 11. Non-goals

- **Comments about text that comes later, or ranges spanning blocks.** Out by
  the brief. A reviewer who needs "everything from here to there" comments the
  section.
- **Persistent guidance** (`keep this in mind whenever you regenerate`). That
  is a constraint or a note in the artifact, not a review comment; it belongs
  in the file as content, where every phase reads it.
- **Annotations in generated code.** Code review has its own tools; parlay's
  contract with generated code is the buildfile, and a comment in a `.go` file
  is not a spec change.
- **A registry of handles or per-handle permissions.** `@dwht` is a name, as
  `--by` is. Who may resolve what is a question for the project, not the
  syntax.
- **Threads surviving the file.** No cross-file identity, no ids. A thread is
  the lines it occupies; move the file and the thread moves with it.
