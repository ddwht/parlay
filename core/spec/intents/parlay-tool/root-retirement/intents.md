# Root retirement

> Feature retirement closes one feature, and it is permitted only when the feature built nothing and nothing points at it. A whole subproject ending is a different question that those rules cannot answer: its features have artifacts, some of its work shipped and some was only ever specified, and code it owns is still running inside code that stays. This feature is the operation that ends a root — accounting for every feature it contained, moving authority for anything that survives to a home that already exists, preserving the whole directory as history someone can verify, and removing the root from the project last, so a run that fails leaves the project either as it was or explicitly mid-flight, never quietly half-gone.

---

## End a subproject without pretending it never happened

**Goal**: A maintainer who has decided a subproject is over can make the project stop treating it as live while everything it produced stays readable, so ending a line of work is not a choice between carrying it forever and deleting the record that it happened.
**Persona**: A maintainer deciding that a subproject in this repository is finished
**Priority**: P0
**Context**: A root that is ending is not the small clean case feature retirement handles. Its features have artifacts, some of them delivered code that shipped, some built nothing anyone ever used, and a few own code the rest of the project still calls. The two ways to end such a root today are to leave it registered and increasingly untrue, or to delete it by hand — which loses the record, answers no dependency question, and leaves the project's registration and its directories disagreeing about what exists.
**Action**: Name the subproject that is ending, review what ending it would do, and authorize that, as one governed operation that either completes or never begins.
**Objects**: root, subproject, retirement, archive, preview, authorization

**Constraints**:
- The named target must resolve to exactly one registered child root. A name matching nothing, a name matching more than one candidate, and a directory that merely looks like a root without being registered are each refusals rather than guesses: there is no version of this operation that is safe to run against a root the operator did not mean.
- The root holding the project's shared resources and the registration of every other root cannot be the target. That is not a subproject ending; it is the project ending, and it would leave nothing standing to record the ending in.
- The destination the root's contents are preserved into must not already exist. An existing destination is either an earlier retirement of the same root or something unrelated wearing the same name, and both are questions the operator has to answer before anything is written.
- This operation is authorized in its own right and does not loosen the rule refusing to retire a *feature* that has anything built. That rule is correct about what it governs — feature retirement removes nothing, so a feature with artifacts would keep them and keep being read by everything that enumerates features. Root retirement removes the whole root from the project and preserves its contents, which is what lets it answer for built features, and which is why it is a separate decision carrying its own record rather than a way around a check.
- Nothing is written until the operator has been shown what the run would do and has said yes to that. A preview that describes the run is not a step toward it: asked for on its own, it writes nothing anywhere, reserves nothing, and leaves no state behind.
- A run with nobody present to authorize it refuses and writes nothing. Ending a subproject has no safe default, and an unattended run that proceeds is indistinguishable afterwards from one somebody approved. Preview remains available unattended, because reporting what would happen commits to nothing.
- The retirement leaves a record of what was decided and where the evidence went. That record is history: it never becomes the owner of anything still live. Code that survives the retirement has an active home named before the run, not a citation of the retirement afterwards, because a record of something ending cannot answer questions about something still running.

**Verify**:
- A target naming exactly one registered child root proceeds to preview.
- A target naming no registered child root is refused, and the refusal lists the roots that are registered.
- A target matching more than one candidate is refused rather than resolved by ordering.
- A directory that contains root configuration but is not registered is refused as a target.
- The root that holds the shared resources and the root registration cannot be named as a target.
- A destination that already exists refuses the run before anything else is read.
- A preview asked for on its own leaves the project byte-identical.
- A run that cannot ask a person for authorization writes nothing and says that is why.
- Retiring a root whose features have artifacts is permitted.
- Retiring one of those same features on its own is still refused, with the reason it is refused today.
- The retirement record names no surviving code as belonging to it.

---

## Say what became of every feature in the root

**Goal**: Someone reading this project a year from now can tell, for each feature the retired subproject contained, whether its work shipped and went away with it, was built and never delivered anything, or still exists under a name they can look up — so the end of a subproject reads as a set of decisions somebody made rather than as an undifferentiated deletion.
**Persona**: A maintainer accounting for the features of a root that is ending
**Priority**: P0
**Context**: A root that is ending holds features in genuinely different states, and the differences are what a later reader needs. Silence flattens them into "gone", which is the single answer that is wrong about every one of them. The states are also not recoverable from what is on disk: whether a feature delivered anything is a fact about code that this operation is about to delete, and whether it was built is not readable from any one artifact's presence — the features carry different build artifacts, and several carry build state that hashes nothing at all.
**Action**: Require one disposition for every feature in the root, drawn from a closed set of three, and refuse the retirement until the set of features named is exactly the set of features present.
**Objects**: feature, disposition, closed vocabulary, rationale, re-homed authority, placeholder baseline

**Constraints**:
- The disposition vocabulary is closed at three: the feature's work was delivered and goes away with the root; the feature was built and nothing was ever delivered from it; or authority over something that survives moves to a named feature. A fourth answer invented during a retirement is a category no later reader can look up, and the point of the record is that it can be read by someone who was not there.
- Exactly one disposition per feature, and the features named are exactly the features present. A missing name closes a root over a feature nobody decided about; an extra name is a decision about something that is not there, which in practice means the name is wrong and the feature it meant is unaccounted for.
- Dispositions are declared, never inferred. Nothing about whether a feature was built is read from the presence, absence or emptiness of its build state. The features differ in which build artifacts they carry; a **placeholder baseline** — one written when the feature was first recorded, naming no intents and no sources, hashing nothing — is present while attesting to nothing at all; and a feature can carry a buildfile and testcases while lacking a coverage review, so keying "was this built" off any one artifact misclassifies it. Every such inference disagrees with the operator on precisely the features where the difference matters.
- Every disposition pairs its term with a free-text rationale — the witness for the term, saying where the work went or where it can be seen not to have gone. The rationale is prose written by the operator, never parsed and never consulted to decide anything: it does not extend the closed set, qualify a term, or create a fourth answer by the back door. It exists so that a coarse term stays checkable.
- The rationale is what makes the hardest classification expressible without opening the vocabulary. A feature whose entire delivery was the *removal* of something — its work witnessed only by markers sitting on other features' files, files this same run deletes — is honestly recorded as delivered-and-deleted, and after the run nothing on disk shows that it was, because the witnesses went with the deletion. The term is right and unreadable on its own; the rationale is where "what it delivered was the retraction, recorded at these markers, which are inside this deletion" gets written down. A term with no rationale is a classification nobody can check, which is why the rationale is required on every disposition rather than only on the awkward ones.
- A disposition that re-homes authority names the feature that carries the work now, and naming it settles nothing on its own — the named feature has to actually be there, be live, and already claim the surviving work before this run touches anything.
- The dispositions are authored as a record before the run rather than assembled from answers during it. A set of decisions this size, checked for completeness against the root and preserved with the archive, has to exist as something reviewable before it is executed; answers typed at a prompt leave the operator's reasoning nowhere and make the completeness check a matter of what got typed.
- The disposition record is preserved with the root's contents. The account of what became of each feature is worth exactly as much as its availability to whoever finds the archive.

**Verify**:
- A record naming every feature in the root exactly once, each with a term from the closed set, passes the preflight.
- A record missing one feature is refused, and the refusal names the feature.
- A record naming a feature that is not in the root is refused, and the refusal names it.
- A record naming one feature twice is refused.
- A term outside the closed set is refused.
- A feature carrying only a placeholder baseline is enumerated like any other and needs its own disposition.
- A feature carrying only some of the build artifacts its siblings carry is enumerated like any other and needs its own disposition.
- A feature with a buildfile and testcases but no coverage review is not classified as unbuilt on that basis.
- A disposition carrying a term and no rationale is refused.
- A feature whose delivery was the removal of something, witnessed only by markers on other features' files that this run deletes, is recordable as delivered-and-deleted with that stated in its rationale.
- The rationale never changes which terms are accepted; the closed set is the same three regardless of what any rationale says.
- The preview lists every feature a disposition will be required for, before any disposition is written.
- The disposition record is readable alongside the preserved contents after the run.

---

## Refuse while anything outside the root still stands on it

**Goal**: Ending a subproject can never remove ground that surviving code, surviving specifications or shipped guidance is standing on, so nobody discovers a dependency by finding that something stopped working after the root that provided it was retired.
**Persona**: A maintainer ending a subproject that the rest of the project may still use
**Priority**: P0
**Context**: The project already has an inbound-reference inventory, built for feature retirement, and it is right for that job and insufficient here for two reasons that both point the same way. It enumerates within one root, so run against the retiring root it cannot see the rest of the project and run against the surviving root it cannot see what is leaving. And it deliberately reads a closed set of specification artifacts and no source code, no shipped guidance and no schema documents at all — a closure that is sound where it lives, because a rule blocking on any occurrence of a name is one people route around. Every dependency a retiring root actually has is in the places that inventory does not look: ownership markers on surviving source files, guidance documents instructing readers to run commands only this root provides, and schema documents claiming this root's concerns.
**Action**: Sweep the whole project — every root, source and shipped guidance as well as specifications — for anything pointing into the retiring root, and refuse the retirement while any of it remains unresolved.
**Objects**: inbound reference, out-of-root ownership, re-home target, marker, guidance document, scan failure

**Constraints**:
- The sweep spans every root in the project, not the retiring root and not whichever root happens to be active. A check confined to one root can only establish that the root does not depend on itself.
- The sweep reads source files, shipped guidance documents and schema documents as well as specifications. Both a generated guidance document and the source it is generated from are read, because a stale instruction reaches its reader through the deployed copy and gets restored from the source.
- References to *paths* under the retiring root count, not only references to the features the preflight enumerates. A marker can name something in the root's namespace that has no directory in the root at all, so the enumeration will never produce it and a check comparing only against enumerated features certifies a complete disposition set while live references still point into the root. This case is the reason the sweep is defined over the root's whole path space.
- Ownership held by the root's features over anything living outside the root is reported and blocks the retirement until a disposition re-homes it. Code outside the root is not carried away by the archive, so it would survive with an owner the project no longer has.
- Every re-home target must already exist, be active, and already claim the surviving work — all three checked before this run mutates anything. A target that will be updated afterwards does not own the work now, and the interval between is precisely when live code has no home. A target whose own retirement is recorded, applied or merely authored and waiting, is not active.
- The sweep is fail-closed. A file that is present but unreadable or unparseable is recorded as a scan failure and refuses the retirement. A check whose entire purpose is to establish that nothing points here cannot report a clean result over something it did not read, and "cannot tell" is not "none".
- A refusal names, for every finding, the file or artifact that holds it, the position within it, and the reference itself — enough to verify the finding without repeating the sweep, and enough that a clean result is auditable rather than asserted.
- Prose that mentions the root is not a dependency; guidance that instructs a reader to run something only the root provides is. The line is what the reference does if the root goes away — a sentence keeps reading, an instruction starts naming a command nobody can run.
- The sweep is the authority. Existing dependency probes may inform it, and none of them substitutes for it, because each was built to answer a narrower question and would report clean over the places this one exists to read.

**Verify**:
- A marker on a surviving source file naming a feature of the retiring root blocks the retirement and is named with its file and position.
- A marker naming a path under the retiring root that corresponds to no enumerated feature blocks the retirement.
- A shipped guidance document instructing a reader to run a command only the retiring root provides blocks the retirement.
- The same finding in the source that guidance is generated from is reported separately from the deployed copy.
- A specification reference from a feature in another root blocks the retirement.
- A file that cannot be read refuses the retirement rather than passing it.
- A re-home target that does not exist refuses the retirement.
- A re-home target that exists but is itself retired refuses the retirement.
- A re-home target that exists and is active but does not yet claim the surviving work refuses the retirement.
- Ownership by the root's features over work living outside the root refuses the retirement unless a disposition re-homes it.
- Prose mentioning the root's name in a comment does not block the retirement.
- Every check in this sweep runs to completion before the run changes anything on disk.

---

## Keep the root's work verifiably, not merely out of the way

**Goal**: Anyone who later needs what the retired subproject contained can get exactly what was there and confirm that it is exactly what was there, so preserved history is something a reader can rely on rather than a directory somebody promised not to alter.
**Persona**: A person reading a retired subproject's contents after the fact
**Priority**: P0
**Context**: Most of a retiring root is work that was never delivered — features that were specified and built and produced nothing anyone shipped. That work is the main thing worth keeping, and it is the thing a deletion would silently take. Moving a directory somewhere the scanners do not look preserves it only in the sense that it is still on disk: nothing establishes afterwards that it is unchanged, and nothing distinguishes a file that was corrupted from a file that was always thin.
**Action**: Preserve the complete child directory byte for byte with a manifest recording every member and its content hash, refusing outright anything that cannot be preserved exactly, keep the preserved copy out of everything that enumerates live work, and provide a check that reads the manifest back.
**Objects**: archive, manifest, content hash, integrity check, member, symbolic link, escaping path, placeholder baseline

**Constraints**:
- The preserved copy is the complete child directory, byte for byte, including its configuration, its adapters and all of its build state. A curated subset is a judgment about what mattered, made at the one moment nobody can check it — and configuration and build state are exactly what a later reader needs to interpret the rest.
- The manifest lists every member with its content hash, and the manifest itself is covered, so a later reader can establish that both the contents and the list of contents are what the retirement wrote.
- Preserved paths are excluded from every walk that enumerates live features and live build state. A preserved root that still turns up in enumeration is a root that was not retired; retirement means the project stops treating it as present, and discovery is where the project decides what is present.
- Excluding the preserved copy from discovery does not exclude it from being checked. An integrity check reads the manifest back and reports members that changed, went missing, or appeared without being listed.
- A placeholder baseline is history, not corruption. Build state written when a feature was first recorded, naming no intents and no sources, is preserved exactly as it is and reported as intact. Most of a retiring root can look like this, and a check that read thinness as damage would fire on the majority of what is being preserved — which is how a check stops being read, taking the real findings with it.
- A symbolic link is followed only when its target resolves inside the child directory. One resolving outside it fails the run closed, and so does any member whose resolved path lands outside the directory by any other route, a traversal segment included. There are exactly two wrong answers here and this rule refuses both: following the escape preserves something the root does not own and misreports what the archive contains, while skipping it quietly produces a copy that claims completeness it does not have.
- Escape is judged on the resolved path, not on the name. A name that looks ordinary can resolve outside the directory and a name full of traversal segments can resolve inside it, so the check is what the path resolves to, evaluated for every member during the walk and before any of it is copied.
- A member that cannot be read fails the run closed. There is no partial archive: an unreadable member means the guarantee this whole operation exists to make cannot be made, and a copy silently missing a file nobody could read is worse than no copy, because it is indistinguishable from a complete one.
- Both conditions abort before the run has changed anything. They are properties of the walk, so they are found while the archive is still being assembled and the project is still untouched.
- The archive is created complete before anything else changes, so a failure while writing it is a failure of a run that had not yet altered the project.

**Verify**:
- Every file in the child directory, including configuration, adapters and build state, appears in the preserved copy with identical bytes.
- The manifest names every preserved member with its content hash.
- The integrity check passes immediately after a successful retirement.
- The integrity check reports a member whose bytes changed after the fact.
- The integrity check reports a member that is missing.
- The integrity check reports a member present but not listed in the manifest.
- A preserved feature does not appear in any enumeration of live features.
- Preserved build state does not appear in any enumeration of live build state.
- A preserved placeholder baseline naming no intents and no sources passes the integrity check.
- A symbolic link inside the child directory whose target resolves outside it fails the run before anything is written.
- A symbolic link whose target resolves inside the child directory is preserved.
- A member whose resolved path lands outside the child directory through a traversal segment fails the run before anything is written.
- A member that cannot be read fails the run before anything is written.
- A run aborted by an escaping path or an unreadable member leaves the project exactly as it was.

---

## Never leave a project half-retired

**Goal**: A retirement that is interrupted — by a crash, a full disk, a cancelled process — leaves the project in a state somebody can act on, so recovery is a decision rather than an investigation into which half of the operation happened.
**Persona**: A maintainer whose retirement run did not finish
**Priority**: P0
**Context**: This operation moves a large directory, writes a record, and changes what the project believes exists. Done in the wrong order, or done without recording where it got to, an interruption produces a root the registration still names and whose contents have moved, or contents that were preserved while the project still treats the original as live — states in which every subsequent command is answering from an inconsistent picture and nobody knows which step to repeat.
**Action**: Complete the archive and the record first and change what the project believes exists last, and on failure either restore the prior state or leave an explicit resumable journal of exactly what remains to be done.
**Objects**: retirement run, staging, root registration, rollback, resumable journal, recovery

**Constraints**:
- The archive and the retirement record are complete before the project's registration of the root changes, and the registration changes last. The registration is what everything else reads to know the root exists; changing it first means every reader between then and the end of the run is looking at a project whose stated contents do not match its actual ones.
- A failure at any point either restores exactly the prior state or leaves an explicit resumable journal of what remains. The two are both acceptable and the absence of either is not: an interrupted run whose only trace is what happens to be on disk makes recovery an investigation.
- The journal names the steps still outstanding, in the order they must happen, in terms a person can act on and a resumed run can execute.
- A resumed run continues from that journal rather than starting over. Starting over would re-do steps whose preconditions the first attempt already consumed — a destination that must not exist now does, and work already moved is no longer where the first step expects it.
- While such a journal is outstanding, another retirement of the same root refuses. Two runs converging on one half-finished state is how a recoverable interruption becomes an unrecoverable one.
- The run is complete only when the project's registration no longer names the root. Everything before that is reversible or resumable by construction; that step is what makes the retirement true.

**Verify**:
- Interrupting the run before the archive is complete leaves the project exactly as it was.
- Interrupting the run after the archive is complete but before the registration changes leaves either the prior state or a resumable journal of what remains.
- The journal names the outstanding steps in the order they must happen.
- A resumed run completes the outstanding steps and does not repeat the completed ones.
- A second retirement of the same root refuses while a journal is outstanding.
- After a completed run the registration does not name the root.
- At no point does an interrupted run leave the registration missing the root while its contents are still in place, or the registration naming the root while its contents have moved.

**Questions**:
- Re-homing authority across roots lives in this operation because it is part of ending a root, while the existing move operation stays within a single root. Whether cross-root moves should eventually exist on their own, outside a retirement, is a real question and deliberately not answered here — the case that motivated it is a retirement, and a general cross-root move needs a governance story this operation gets from the retirement it is part of.
- The sweep this operation performs and the inventory feature retirement performs answer the same question over different territory, with different rules about what counts. They are kept separate because feature retirement's closure is deliberate and correct where it lives. Whether they eventually converge on one mechanism with a declared scope is worth revisiting once a second root is retired and the shape of the difference is known from two cases rather than one.
