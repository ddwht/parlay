# Domain document API

> The project's shared vocabulary lives in one document, and the code that can read it, write it safely, and say whether it is valid was built inside an editing surface that is going away. This feature is where that code belongs instead. Its promise is that the DOCUMENT is the contract: a published format, complete validation of it, and document-level read and write with concurrency safety — so a hand editor, an agent, and any editing surface built later are all the same kind of client, and none of them holds a private path into the file.

---

## Change the shared domain model without the tool that wrote it

**Goal**: Someone who needs the project's shared vocabulary to say something new can make it say that, working only from the documented file, so the change is never gated on having one particular editing surface running.
**Persona**: A person or agent changing the vocabulary the whole project shares
**Priority**: P0
**Context**: The domain model is a single document every feature reads, and the only complete read-modify-write path over it — safe load, deterministic rewrite, concurrency-checked save, contribution merge — was built as the inside of one editing surface. Everyone else has the file and a validator, and no guarded way to write. When that surface goes away the safe path must not go with it, and when a new one is built later it must not be given a private door.
**Action**: Make the document itself the contract — publish its format, validate that format completely, and offer read and write of the whole document — rather than exposing per-element operations shaped by one editor's screens.
**Objects**: domain model document, document format, validation finding, content-identity token, contribution

**Constraints**:
- The unit of the contract is the whole document. Per-element operations are refused as a contract shape: they encode the editing gestures of whichever surface asked for them first, and every later client inherits that surface's model of what an edit is.
- Editing conveniences are explicitly NOT contract. Cascading a rename, prefilling a field, generating a name from a label — these may differ between clients and are allowed to, because every write is checked against the same rules and lands through the same guarded writer.
- The submitted document is in the published format itself, not a translation of it. A client that can produce the file can write; a client that could only produce some other shape would be asking for a second definition of the format.
- Exactly one writer persists the document, and every client reaches it. Two writers means two definitions of what a valid write is, which this project has produced before and paid for.
- What a client may do is bounded by what the validation rules allow, not by what the client's own screens prevent. A rule an editing surface enforces only on its own side is not a rule.
- The same guarantees hold for the derived writer that merges a feature's contribution: it is a client of the same core, not a second path around it.

**Verify**:
- A document produced by hand and the same document produced through the tool are byte-identical on disk.
- No operation in the contract names a single entity, field, enum or relationship as its subject; the subject is always the document.
- Every persisted change to the document is observable as having passed the single writer.
- A client that enforces its own extra restriction still cannot write anything the shared rules reject.
- A client that enforces none of its own restrictions still cannot write anything the shared rules reject.

**Questions**:
- The guarded write path exists but has no entry point outside this codebase yet, so today the principle is real for callers inside the tool and only aspirational for anyone else. Giving it a public entry point is the first thing that makes this intent true end-to-end, and it is deliberately not part of establishing where the code lives.

---

## Never overwrite a change you did not see

**Goal**: A person editing the shared vocabulary can be confident that saving their work never quietly discards someone else's, so two people working the same afternoon do not lose one of the two changes without either of them learning that it happened.
**Persona**: A person or agent saving a change to a document others also edit
**Priority**: P0
**Context**: The document is a single file with several editors: a person with it open, an agent applying a contribution, a regeneration step rewriting it. Nothing about the file itself prevents the second write from landing on top of a first that the second writer never read.
**Action**: Hand out a content-identity token with every read, require it back on every write, and refuse the write when the document has moved on since.
**Objects**: domain model document, content-identity token, conflict report, save

**Constraints**:
- A write must present the token it was given when the document was read. There is no unconditional write and no way to ask for one: an override flag is a way to lose someone's work on purpose, and the recovery — read again, reapply, write again — is available to every client.
- The token is derived from the document's exact stored content, so two clients holding identical content hold the same token and a change by anyone, through any path, invalidates it.
- A rejected write persists nothing at all. A partial write is worse than a refusal, because a refusal is legible.
- A rejection reports both the token presented and the token the document now carries, so the client can tell "someone else changed it" from "I sent the wrong thing" without reading the file to guess.
- The writer never merges. Reconciling two changes is a judgement about meaning, and a writer that guesses at it is a writer that occasionally guesses wrong silently.
- A project that has no document yet is a real starting state, not an error: a distinguished token stands for "there is nothing here", and a write presenting it creates the document. Any other token presented against an absent document is a stale read, not a create.
- The check must hold across separate processes, not only within one, because the writers are separate processes.

**Verify**:
- A write presenting the token from its own read succeeds, and the stored document afterwards is what was submitted.
- A write presenting a token from before someone else's write fails, and the document on disk is unchanged.
- The failure names both the current and the attempted token.
- A write presenting the empty-document token against an absent document creates it.
- A write presenting any other token against an absent document fails.
- Two writes from separate processes racing on the same document leave one of the two changes stored whole; neither leaves a blend of the two.

**Questions**:
- The check protects writers that cooperate with it. An editor that ignores the protocol entirely can still replace the file between another writer's read and its write; that write is then detected on the next read, not prevented. Stating the bound is the honest position, and narrowing the window is a hardening task rather than a change to this promise.
- Between comparing the token and replacing the document there is a window in which a second cooperating writer can compare successfully too, and the second replacement then lands on top of the first with both writers believing they were current. The replacement itself is indivisible, so nobody reads a blend — but a change is lost and nothing says so, which is the outcome the whole promise exists to prevent. Closing the window needs the comparison and the replacement to happen under one exclusion that spans processes. That is the first hardening this intent asks for, and it is deliberately not part of establishing where the code lives.

---

## Get back a document that changed only where you changed it

**Goal**: Someone reviewing a change to the shared vocabulary sees exactly what changed and nothing else, so the review is about the decision and not about reconstructing which of forty moved lines were meaningful.
**Persona**: A person reviewing or authoring a change to the shared vocabulary
**Priority**: P0
**Context**: The document is checked in and reviewed as a diff. A writer that re-derives the whole file from an in-memory shape will happily reorder declarations, normalize away spellings it does not model, and rewrite parts nobody touched — and the reviewer cannot tell any of that from the change that was actually intended.
**Action**: Rewrite the document deterministically, preserving the order the author arranged, and carry through unchanged every part the tool does not model.
**Objects**: domain model document, declaration order, deprecated operations block, stored content

**Constraints**:
- The same content rewritten twice produces identical bytes. Determinism is what makes the content-identity token mean anything at all, so this constraint and the save protocol stand or fall together.
- Declaration order is the author's, not the tool's. Enums, entities, fields and values keep the order they were arranged in; nothing is alphabetized on the way out.
- Optional detail that was never set is absent from the rewritten document, not present and empty. An empty value asserts something the author did not say.
- The deprecated operations block is carried through exactly as stored, byte for byte, from the document being replaced. The tool does not model it, and content the tool does not model is content the tool must not rewrite from its own understanding of it.
- Content the tool does not model and cannot carry through faithfully is not silently dropped either. Losing a key because the reader did not recognize it is the failure this constraint exists to prevent, and it is not made acceptable by being invisible.
- This is a bounded exception, not a general policy of preserving unrecognized bytes: it covers a named block that is on its way out, and it exists so an unrelated edit does not become a rewrite of it.

**Verify**:
- Rewriting the same content twice produces byte-identical results and therefore the same content-identity token.
- A change to one entity leaves every other declaration byte-identical, in its original order.
- A change to one entity leaves the deprecated operations block byte-identical.
- An optional detail left unset does not appear in the rewritten document.
- A document with no deprecated operations block gains none.

---

## Ask for vocabulary the shared model does not have

**Goal**: Someone working on one feature can say what vocabulary that feature needs without taking on the authority to change what the whole project already agreed, so the request is recorded and reviewable instead of being either an unannounced edit or a conversation that never happens.
**Persona**: A person or agent developing one feature that needs a shared concept the project has not defined
**Priority**: P1
**Context**: A feature reaching for an entity, field, enum value or relationship the shared document lacks has two bad options: edit the shared document directly, which changes what every other feature reads without anyone asking, or work around the gap locally, which puts a second definition of the same concept in the project. The shared document has to stay the agreed thing while a feature is still asking.
**Action**: Let a feature record what it needs alongside its own specification as a proposal, report what accepting it would do to the shared document, and merge it only when accepting adds and never rewrites.
**Objects**: contribution, shared domain model, addition, conflict, redundant element, merge

**Constraints**:
- A contribution is a proposal and lands only when someone accepts it. Until then the shared document is unchanged and remains the single source of truth.
- A contribution is written in the same format as the shared document and holds only what the feature proposes. A second format for proposals would be a second definition of the same vocabulary.
- Accepting a contribution only ever adds. An element the shared document already describes differently is a conflict — a question for a person about which description is right — and is never resolved by preferring one side.
- A contribution that conflicts is refused whole; nothing is merged partially. Applying the non-conflicting half leaves the shared document holding some of a proposal that was never accepted, and no record of which half.
- An element the shared document already describes identically is redundant, not a conflict and not an addition. A feature restating something already agreed is normal and must not read as a disagreement.
- Comparison is per element at the granularity a person would argue about. A feature adding one value to an existing enum has not proposed replacing the enum, and reporting it as a conflict would make the common case look like a dispute.
- The report is ordered by the contribution's own declaration order, so running it twice over unchanged inputs reads the same both times.
- Accepting a contribution goes through the same single writer and the same concurrency check as any other change, computed against the document as it stands at that moment rather than against a copy read earlier.

**Verify**:
- An element absent from the shared document is reported as an addition.
- An element present and identical is reported as redundant.
- An element present and different is reported as a conflict, carrying both descriptions.
- A new value on an existing enum is an addition, not a conflict on the enum.
- Accepting a contribution with no conflicts leaves the shared document holding every addition and nothing else changed.
- Accepting a contribution that conflicts writes nothing and says why.
- A feature with no contribution of its own is reported as having proposed nothing, distinctly from having been checked and found empty.
- Running the report twice over unchanged inputs produces the same ordering.

---

## Know who a proposed change reaches before accepting it

**Goal**: Whoever decides on a proposed vocabulary change can see who else it lands on before deciding, so the decision is made with the consequences visible rather than discovered by the features that break afterwards.
**Persona**: A person deciding whether to accept a feature's proposed vocabulary change
**Priority**: P1
**Context**: The shared vocabulary is shared: other features name the same entities in their own specifications, and existing test data holds records of those entities. A change that is obviously right for the feature proposing it can quietly oblige three other features and a dozen fixtures, and none of that is visible from the proposal itself.
**Action**: Report, alongside what a contribution proposes, which other features name the affected entities and which existing fixtures would need the newly proposed detail.
**Objects**: contribution, entity, affected feature, fixture, audience report

**Constraints**:
- The audience is the OTHER features that name the entity, not the one proposing. A feature listed as affected by its own proposal is noise in the one place the reader is trying to see signal.
- Fixtures are reported concretely enough to act on: which fixture, and which proposed detail its records do not yet carry. A count communicates that there is work without saying where.
- The same rule answers this question wherever it is asked, so a project-wide check and a single-contribution report never disagree about who is affected.
- The report is a function of the facts it is given about the project, gathered once by the caller, so the answer does not depend on the order in which files happened to be read.
- Reporting the audience is not deciding. The report never withholds a merge that the conflict rules allow, and never permits one they refuse.

**Verify**:
- A feature whose specification names an affected entity appears in the audience; the proposing feature does not.
- A fixture holding records of an affected entity is named together with the proposed detail its records lack.
- A fixture that already carries the proposed detail is not reported as needing it.
- The report is the same whichever order the project's files were read in.
- An entity nothing else names produces an empty audience rather than an absent one.

---

## Get the same verdict on the same document wherever it is checked

**Goal**: Someone told their document is invalid gets the same answer, in the same words, however they asked — so a document accepted in one place and refused in another is not something they ever have to reason about.
**Persona**: A person or agent checking a domain model document before relying on it
**Priority**: P0
**Context**: The document is checked from more than one place: on the way in to a write, and directly when someone asks. Those two paths once ran different code — one shelled out to the other — and every divergence between them showed up as the tool contradicting itself about the same file.
**Action**: Keep one engine that turns a document into findings, and have every checking path call it and pass what it returns through unaltered.
**Objects**: domain model document, validation rule, finding, severity, element path, fix

**Constraints**:
- One engine owns every rule. A checking path that adds a rule of its own has created a document that is valid in one place and not another.
- Findings pass through verbatim: the code, the element path, the severity, the message and the suggested fix are the engine's, and a consumer neither renames, reclassifies, nor rewords them.
- The element path anchoring a finding uses one grammar across every path, so a person can carry a finding from one report to another without translating it.
- A finding that is about the document as a whole rather than any element says so with a distinguished path, rather than pointing at an arbitrary element or at nothing.
- The severity the engine assigns is the severity that decides. Whether a deprecated construct blocks a save is the engine's classification to make, and a checking path that promotes or demotes it has taken a policy decision that belongs in the rules.
- Which rules apply may depend on why the document is being checked — authoring is a different question from building — but that choice is named at the call, from the engine's own vocabulary, and never approximated by filtering its output afterwards.
- The engine reaches the code that checks documents as a supplied capability rather than as a hard-wired dependency, so the direction of dependency between the document core and the rules stays one-way and substitutable under test. The seam is a design choice, not a workaround for where the packages happen to sit.

**Verify**:
- The same document checked through a write and checked directly produces the same set of findings, field for field.
- A finding's code, path, severity, message and fix are identical between the two paths.
- A document with no path of its own reports the same anchoring token from both paths, so their messages match exactly.
- A whole-document finding carries the distinguished whole-document path.
- A deprecated construct classified as a warning does not block a write; the same construct classified as an error does.
- The document core can be exercised against a substituted set of findings without the real engine present.
