# Domain document API — Infrastructure

---

## One home for the document core

**Affects**: package boundary owning the domain-model document's read, rewrite, save and contribution logic
**Behavior**: Everything that turns the stored domain-model document into a usable shape and back — reading it, migrating an older version in memory, rewriting it deterministically, deriving its content-identity token, replacing it under the concurrency check, and comparing a feature's proposal against it — lives in one unit inside the tool's own tree. Consumers depend on that unit; it depends on none of them. A second decoder of the same document is the failure this exists to prevent: this project has produced that shape repeatedly, and the cost each time was two definitions of the same file diverging quietly until something read the file through the wrong one.
**Invariants**:
- Exactly one unit decodes the domain-model document; no other package parses it directly.
- The unit does not depend on any consumer of it, so the dependency graph around it stays one-way and free of cycles.
- Moving a consumer, adding one, or removing one changes nothing about the unit's contents.
- Every artifact this feature owns names this feature as its owner, so ownership is readable from the code itself rather than inferred from where the code sits.
**Source**: @domain-document-api/change-the-shared-domain-model-without-the-tool-that-wrote-it
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The unit currently sits outside the tool's own tree, inside an editing surface being removed, and its ownership markers still name features of that surface. This feature is the authority home the code moves into; the move itself, and the marker rewrite that records it, are separate work. The order matters: an owner has to exist before the code can be re-homed to it, because a record of something being retired cannot own code that is still running.

---

## Validation reaches the document core as a supplied capability

**Affects**: dependency direction between the document core and the validation rules
**Behavior**: The document core does not import the rules engine. It declares what it needs — something that turns a draft document into findings — and receives an implementation from the layer that legitimately knows both halves. That layer supplies the real engine, named with the mode the rules should be applied in; a test supplies a chosen set of findings. The seam is a design choice about direction, not a workaround: the rules engine depends on the document core's own shapes, so the core importing the engine would close a cycle.
**Invariants**:
- The document core does not reference the rules engine by name anywhere.
- The core's behavior can be exercised end to end against a substituted finding set, with no rules engine present.
- The supplied implementation carries every field of a finding through unchanged — code, element path, severity, message, fix — and neither renames, reclassifies, nor rewords any of them.
- The mode the rules are applied in is chosen at the point the real implementation is supplied, from the engine's own vocabulary, and never approximated by filtering findings afterwards.
- A finding about the document as a whole carries the distinguished whole-document path, identically on every path that produces one.
**Source**: @domain-document-api/get-the-same-verdict-on-the-same-document-wherever-it-is-checked
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The seam predates this feature and was originally justified by a visibility rule that disappears once the editing surface does. The justification that survives is the cycle: the direction is what the seam is for, and it would be the right shape even if both halves were mutually visible.
- The identifier the seam's marker currently carries describes a mechanism that is no longer used — validation was once run as a separate process and now is a call. The name is a spec anchor read by the coverage and drift checks, so correcting it is a migration across those artifacts rather than an edit to a comment.

---

## One writer for the stored document

**Affects**: write path for the shared domain-model document
**Behavior**: Every persisted change to the document goes through a single writer that performs the concurrency comparison and the replacement together. Producers differ in where their content comes from — a submitted document, or a merge computed from a feature's proposal — and are identical from the comparison onwards. Nothing else writes the document. A second write path is a second definition of what a valid write is, and the two diverge on exactly the cases nobody tests.
**Invariants**:
- No path other than the single writer replaces the stored document.
- Every producer reaches the same comparison, the same rewrite, and the same replacement, so a fault injected in that shared core is observed identically by all of them.
- A derived change computes its content from the document as read inside the same write, never from content read earlier and held.
- A refused write leaves the stored document byte-identical to what it was.
**Source**: @domain-document-api/change-the-shared-domain-model-without-the-tool-that-wrote-it, @domain-document-api/never-overwrite-a-change-you-did-not-see
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The comparison and the replacement are two steps today, with a window between them in which a second cooperating writer can also compare successfully. The replacement is indivisible, so no reader sees a blend, but the earlier change is lost silently. Holding the pair under one exclusion that spans processes is what closes it; the writer's shape already anticipates that, since both producers pass through the same core.

---

## Deterministic rewrite with faithful passthrough

**Affects**: rewriting of the shared document from its in-memory shape
**Behavior**: Rewriting the same content twice produces identical bytes, which is what makes a content-identity token mean anything at all: the token is derived from stored bytes, so a rewrite that varied would invalidate itself. Ordering is the author's — declarations keep the arrangement they were given, and nothing is sorted on the way out. Optional detail that was never set is absent rather than present and empty. The one part of the document the tool does not model is spliced back exactly as it was captured from the document being replaced, so an edit elsewhere cannot perturb it.
**Invariants**:
- Two rewrites of the same content are byte-identical and therefore carry the same content-identity token.
- Declaration order is preserved exactly, at every level that has an order.
- Optional detail left unset is omitted, never written as an empty value.
- The unmodelled block is byte-identical before and after a change made elsewhere in the document.
- The unmodelled block is captured from the document being replaced rather than reconstructed from any parsed view of it.
- A document with no such block gains none.
**Source**: @domain-document-api/get-back-a-document-that-changed-only-where-you-changed-it
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Passthrough is deliberately bounded to one named block that is being deprecated. It is not a general policy of preserving unrecognized content: a general policy would compete with the strictness the validation rules are meant to provide, and it exists here only so that an unrelated edit does not become a rewrite of something on its way out.

---

## Conflict reporting belongs to the document core

**Affects**: ownership of the stale-read conflict result
**Behavior**: The result that reports a refused write — carrying the token presented and the token the document now holds — is defined by the document core itself, in terms that name no transport. A conflict is a fact about the document, not about how someone asked; a caller renders it as a status, an exit code, or a message, and none of those renderings belong to the core.
**Invariants**:
- The conflict result is defined by the document core and names no transport concept.
- The document core does not depend on any transport in order to report a conflict.
- The result carries both the presented and the current token, so a caller can distinguish a stale read from a malformed request without re-reading the document.
- A caller can render the conflict without inspecting anything the core did not put in the result.
**Source**: @domain-document-api/never-overwrite-a-change-you-did-not-see
**Backward-Compatible**: no

**Notes**:
- Today the core reaches into the editing surface's own transport layer for this result, which is the one place the dependency runs the wrong way. The transport is being removed, so the fix is forced rather than optional — but the shape it forces is the one the boundary wanted anyway.

---

## Durable replacement of the stored document

**Affects**: durability of the document replacement step
**Behavior**: A replacement is staged beside the target, flushed to durable storage, and then moved into place, so an interruption at any point leaves either the previous document or the new one and never a partial file. The containing location is created if it is not there yet. A reader arriving mid-write sees one whole document; a truncated document is not a state the project can reach.
**Invariants**:
- An interruption between staging and the move leaves the previous document intact and readable.
- The staged content is durable before the move, so an interruption after the move cannot leave the new name pointing at empty content.
- Leftover staged content from a previous interrupted attempt is replaced rather than appended to.
- A successful replacement leaves no staged remnant behind.
**Source**: @domain-document-api/never-overwrite-a-change-you-did-not-see
**Backward-Compatible**: yes

**Notes**:
- A separate feature owns the equivalent primitive for the tool's deployment paths. They are not the same code today and this fragment does not claim them as one; whether they converge is a question for whichever of the two is next revised, not a promise made here.
- Staging under a fixed derived name is safe for one writer and not for concurrent ones, which is part of the same hardening the single-writer fragment describes.

---

## The document has one read path

**Affects**: resolution of the shared document's location
**Behavior**: Reads and writes target exactly one document under the resolved project root. A legacy alternate form of the same artifact is never parsed, never merged, and never consulted as a fallback under any path — a fallback would make "which document is authoritative" depend on which files happen to exist, which is not a question a reader should be able to get a different answer to than a writer.
**Invariants**:
- The resolved location is a pure function of the project root; nothing else influences it.
- The legacy alternate form is never read, on any path, including when the canonical document is absent.
- An absent canonical document reads as the empty starting state, not as an error and not as a reason to look elsewhere.
**Source**: @domain-document-api/change-the-shared-domain-model-without-the-tool-that-wrote-it
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- In a project with more than one root the resolved root's document is the sole target; there is no selector and no override. That is a deliberate narrowing rather than an omission, and widening it is a change to what "shared" means across roots.

---

## The contribution comparison is the same comparison everywhere

**Affects**: ownership of the proposal-versus-project comparison and of the audience walk
**Behavior**: What a feature's proposal would do to the shared document — additions, conflicts, and elements already present identically — is decided in one place, by the unit that already owns decoding and rewriting that document. Who a proposed change reaches, in contrast, is a fact about the rest of the project rather than about the document, and is answered by the layer that owns project-wide knowledge, as a function of facts collected by its caller. The two questions are separated along that line so that a project-wide check and a single-proposal report cannot disagree about either one.
**Invariants**:
- One definition of what counts as a conflict serves every caller.
- Comparison follows the proposal's own declaration order, so repeated runs over unchanged content report identically.
- The audience walk reads only the facts it is given and touches no files, so its answer does not depend on the order files were read in.
- The proposing feature never appears in its own audience.
- A project-wide check and a single-proposal report name the same affected features for the same change.
**Source**: @domain-document-api/ask-for-vocabulary-the-shared-model-does-not-have, @domain-document-api/know-who-a-proposed-change-reaches-before-accepting-it
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The proposal artifact is the same format as the shared document, holding only what one feature proposes. That is why the comparison can live with the decoder: there is one decoder, and both sides of the comparison go through it.
- These parts are currently marked as belonging to a feature that has no specification directory anywhere in the project. They are code with an owner named and no owner defined, which is why re-homing them is a founding act rather than a tidy-up.
