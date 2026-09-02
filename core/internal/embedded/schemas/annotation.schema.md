<!--
parlay-section: cross-cutting
parlay-feature: annotations
-->

# Annotation Schema

An anchored review comment: `@you: text` written inside a comment in the file it is about, directly below the text it is about.

A person reviewing a parlay file has three ways to say something is wrong, and each loses something. Editing the file directly is a `ledger_integrity` violation on a frozen founding document, unrecorded on a contract artifact, and forbidden outright on an applied amendment. Saying it in the conversation loses the anchor and evaporates with the session. Writing a `TODO` leaves a marker nothing finds, anchors or retires. An annotation is the fourth route: a comment the tool can find, anchor, hand to a resolver, and see closed.

The file is the inbox. There is no side ledger — an annotation lives next to the text, and git is the history.

<!-- parlay:normative -->
## Grammar

```
@<handle> [<kind>] [section] [ "<phrase>" ]: <text>
```

The **first line's grammar** is byte-identical in both hosts — everything from the `@` to the end of that line. Continuations differ, because YAML repeats a `#` wrapper per line while a Markdown comment simply spans them. The wrapper is the file's own comment marker, not parlay's.

| Part | Rule |
|---|---|
| `@<handle>` | Required. Who is speaking: `[A-Za-z0-9_.-]+`. Free-form and unregistered, the same convention as `parlay note --by`. A `/` immediately after the handle means this is an `@feature/name` ref and not an annotation at all; the line is passed over in silence. |
| `<kind>` | Optional bare word from the closed set below. Absent means `do`. |
| `section` | Optional scope word, **Markdown only**. Widens the anchor from the unit above to the enclosing section. In YAML it is `annotation-malformed` — the column already says how wide a YAML comment is. |
| `"<phrase>"` | Optional double-quoted verbatim substring of the anchored unit, narrowing the anchor to a place inside it. Not a range selector: it never extends past the unit. |
| `:` | Required whenever there is text. The one complete colon-less form is `@handle close`. |
| `<text>` | The comment. May continue onto following lines inside the same comment. Optional only after `close`. In Markdown it may **not contain `-->`**, which would close the comment early and turn whatever follows into content — `parlay annotations reply` refuses such text rather than writing it. |

The words between the handle and the colon are a **set, not a sequence**: each must be a kind or the scope word, a kind may appear once and the scope once, in either order. `@dwht ask section:` and `@dwht section ask:` are the same annotation.

## Host forms

| Host | Marker | Continuation |
|---|---|---|
| Markdown (`.md`) | `<!-- @dwht: text -->` | The comment may span lines; everything up to `-->` is the text. |
| YAML (`.yaml`, `.yml`) | `# @dwht: text` | Consecutive `#` lines at the **same column**. A line at that column beginning `# @` starts a new annotation instead. |

There is no wrapper both formats tolerate. A `#` line in Markdown is a heading; an HTML comment in YAML is a syntax error to every YAML reader. The form is one and the marker is the reader's existing habit.

A `*.page.md` and an amendment record carry both: the YAML frontmatter reads by the YAML rules, the Markdown body below it by the Markdown ones.

**Inside code, the sigil is a picture of a sigil.** A comment opener inside an inline code span or a fenced code block is content, to the scanner and to every structural parser alike — with exactly one exception, a `*.page.md`'s `## Layout` fence, which is YAML the page loader decodes with a reader that drops `#` comments, and so is a host in its own right — one lexer answers it for both, so the same bytes can never be content to one reader and an actionable request to the other. The inline-code half is a deliberate approximation: matching is line-local, because carrying span state across lines would let one stray backtick swallow every following line.

## Kinds

| Kind | Who writes it | Meaning |
|---|---|---|
| `do` *(the default when absent)* | reviewer | Change this. |
| `ask` | reviewer | I do not understand this, or I want to know why. Answered with a reply; **nothing is edited**. |
| `close` | reviewer | This conversation is finished. Text optional. Only a closed thread is ever removed. |
| `done` | resolver | The change was made; the text names the amendment when there is one. |
| `answer` | resolver | The answer to an `ask`. |
| `declined` | resolver | The change was not made, and why — usually a gate the resolver may not pass alone. |

The closed set is `{do, ask, close, done, answer, declined}`. `do` and `ask` are **requests**; `done`, `answer` and `declined` are **replies**; `close` is **terminal** and only a reviewer writes it.

There is deliberately no `ok` or `approved` kind. Approval of criteria has a home already (`approve-criteria`), and "I looked and it is fine" leaves no trace by design — the absence of a comment is the approval.

## Threads

A request followed by its replies is a **thread**: a run of **consecutive** entries at one column, with nothing between them, not even a blank line.

A further **request after a reply continues the same thread** and returns it to `open` — that is how a reviewer disagrees with what was done. A blank line, by contrast, starts a **new** thread on the same text, which is how a conversation is reopened under a closed one.

A thread's state is **derived from its last entry, never stored**:

| Last entry | State | Meaning |
|---|---|---|
| a request (`do`, `ask`) | `open` | the resolver owes a response |
| a reply (`done`, `answer`, `declined`) | `answered` | the reviewer owes a reading |
| `close` | `closed` | the reviewer has said the conversation is over |

Only a **closed** thread is ever removed. `parlay annotations clear` deletes the closed threads and nothing else; an `answered` thread stays until the reviewer closes it, an `open` one until the resolver answers and the reviewer closes it. The tool never decides that a person is satisfied.

## Placement

- **Own line.** An annotation never shares a line with content. Another comment beside it is not content.
- **After, never before.** The anchor is always above. Blank lines between the text and the annotation are transparent to the *anchor* (though not to the *thread*).
- **A unit never claims text below the annotation.** Every span is clamped to the line above the request. The one exception is `section`, which is defined forward from its heading on purpose.

## Anchoring — Markdown

Walk up from the annotation, skipping blank lines and other annotations, to the nearest content line.

| That line is… | Anchored unit |
|---|---|
| a heading (`#`–`####`) | **the heading line itself** — the title, never the section under it: that text comes after the comment |
| a list item, or a continuation of one | that list item, including its continuation lines and nested items |
| a `**Field**:` line | that field |
| a dialog turn (`User:`, `System:`, `System (background):`, `System (condition: …):`) | that turn, including its indented option lines |
| an indented option line | the turn that offered it |
| any other non-blank line | the paragraph — the contiguous run of non-blank lines ending there |
| the frontmatter's closing `---` | the frontmatter block |
| a `---` section separator | `annotation-unanchored` — a separator is not text |

Inside nested lists the comment's own column selects the level: the innermost enclosing item whose **marker** is at or left of the comment. **Align with the dash you mean** — the same gesture as YAML's.

The scope word `section` replaces the unit with the enclosing section: from the nearest heading above to the next heading of equal or higher level, or the next `---`. This is the only way to comment on a whole intent, dialog or fragment, and it is explicit on purpose.

## Anchoring — YAML

The `#` column is the selector. Walk up, skipping comments and blank lines, to the first line whose content starts at a column **less than or equal to** the annotation's. That line opens the anchored node.

| That line is… | Anchored unit |
|---|---|
| a `- ` item | the item and its whole subtree |
| `key:` with a block value | the key and its whole subtree |
| `key: scalar` | that pair |
| nothing above at all | `annotation-unanchored` — the anchor is always above |
| content above, none at or left of the column | the whole document |

Four shapes have a stated rule:

- **`- key: value`** opens two nodes: the item at the dash's column, and the first pair at the key's. Commenting on the whole item means **outdenting to the dash** — the same move a YAML author makes to add a sibling.
- **Block scalars** (`notes: |`, `>`): every indented line under the indicator is scalar content. A sigil there is `annotation-in-block-scalar`.
- **Flow collections** (`[a, b]`, `{k: v}`): one unit, the pair that opens the collection.
- **Tabs** are not indentation in YAML; a tab-indented annotation is `annotation-malformed`.

## Anchor identity

Every anchor carries a **generic** identity any Markdown or YAML file can supply, plus a **ref** when parlay knows the file.

| File | Generic | Ref |
|---|---|---|
| any `.md` | heading path, line span | — |
| any `.yaml` | YAML path, line span | — |
| `intents.md` | as above | `@feature/intent:<slug>` + field (+ item index) |
| `dialogs.md` | as above | `@feature/dialog:<slug>` + turn index |
| `surface.yaml` | as above | `@feature/surface:<fragment>` + path |
| `capabilities.yaml` | as above | `@feature/operation:<id>` + path |
| `infrastructure.md` | as above | `@feature/infrastructure:<slug>` + field |
| `domain-model.yaml` | as above | `@feature/domain:<entity>` |
| `amendments/NNN-slug.md` | as above | `@feature/amendment:<slug>` + section |
| `*.page.md` | as above | page name + region, or `node:<id>` inside the `## Layout` fence — **not** feature-qualified; `affects:` has no page kind |

The ref reuses the `affects:` vocabulary, so a resolution that becomes an amendment names its dirty set without translation. A YAML index resolves to the entry's **name**, not its position: a position stops meaning anything the moment the list is reordered, and a thread outlives the edit that answers it.

## Errors

| Code | Fires when |
|---|---|
| `annotation-malformed` | A comment opens with the sigil but does not match the grammar — no colon, an illegal handle, an unterminated quote, `section` in YAML, a tab-indented YAML annotation. |
| `annotation-word-unknown` | A word between the handle and the colon is neither a kind in `{do, ask, close, done, answer, declined}` nor the scope word `section`, or a kind or the scope appears twice. |
| `annotation-in-block-scalar` | The sigil appears inside a YAML block scalar, where it is content. Fix: place it at the key's column after the scalar ends. |
| `annotation-inline` | The annotation shares a line with content. |
| `annotation-unanchored` | Nothing above it but the top of the file, blank lines, or other annotations — or the line above is a `---` separator. |
| `annotation-phrase-not-found` | The quoted phrase does not occur in the anchored unit. The finding carries the unit's text. |
| `annotation-reply-orphaned` | A reply or a `close` with no entry directly above it. |
| `annotation-reply-column` | A reply at a different column than the entry it follows — in YAML this would silently re-anchor it to different text. |
| `annotation-after-close` | An entry directly follows a `close` at the same column. Leave a blank line and start a new thread instead. |
| `annotation-in-applied-record` | An annotation inside an amendment the baseline has already applied. Those bytes are under the ledger's integrity hash; the comment goes on the contract entry the amendment changed, or into a superseding amendment. |

None of these fires for a well-formed thread **in an allowed location**, in any state: a review in progress is a healthy state. `annotation-in-applied-record` is the deliberate exception — it fires on a grammatically perfect thread, because the objection is to where it is, not to how it is written.

## Boundary codes

| Code | Fires when |
|---|---|
| `open-annotations` | The feature has at least one `open` thread. Blocks designer→build and build→code. Fix: `/parlay-resolve @feature`. |
| `answered-annotations` | The feature has answered threads. Blocks the same boundaries: a reply nobody has read is a review that is not finished. Fix: read each reply in place, then write `close` under the ones you accept or a new request under the ones you do not. |
| `closed-annotations` | Closed threads are still in the files. Blocks until swept, because a first build has no prior signature for `stale-buildfile` to catch their bytes with. Fix: `parlay annotations clear @feature`. |

Every thread state blocks both boundaries. **No build and no emission reads a file with a thread in it** — threads are review state, and a build is what happens after review. That is what keeps every signature free of comment bytes, so clearing a thread can never stale a buildfile for a change in nothing.

A feature's boundary also blocks on threads in the **project sources its build reads or otherwise depends on** — the root domain model, page manifests, adapters, `adapter-set.yaml`, `blueprint.yaml`. Not all of these are signed: `source-signatures` covers the domain model, the layout and the selected presentation adapter, while `adapter-set.yaml` and `blueprint.yaml` are depended on without being signature fields. For v1 the set is deliberately conservative: every project source blocks every feature, since there is no per-feature dependency map to scope it by.
<!-- /parlay:normative -->

## Why a comment in a frozen document is not an edit

The founding-document freeze hashes **parsed** content, and no Markdown parser in parlay can see a comment. So writing an annotation into a frozen `intents.md` or `dialogs.md` moves no hash, raises no `ledger_integrity` finding, and needs no ceremony. Changing the *text* remains exactly what it always was.

This is the property that makes the design work on the files where it matters most, and it is why the parsers were made comment-blind before the scanner was written rather than after.

## Why an applied amendment refuses one

Every applied record's bytes are hashed into `HashedSources.Amendments` and re-checked by `check-drift`, `apply-amendment`, compaction and the applied-history reader. A moved hash there means "recorded history was edited", which is a far more serious claim than "someone left a comment" — and canonicalising the hash would weaken, in five readers at once, the one hash whose entire purpose is to notice any byte moving.

So the annotation is refused instead. An **unapplied** record has no recorded hash and carries annotations like any other file.
