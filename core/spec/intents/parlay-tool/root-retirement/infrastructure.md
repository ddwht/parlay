# Root retirement — Infrastructure

---

## Retirement target and destination preconditions

**Affects**: root resolution for the retirement operation, archive destination availability
**Behavior**: The retirement operation accepts a target name and resolves it against the project's registration of child roots. Resolution succeeds only when the name identifies exactly one registered child; a name matching nothing, a name matching more than one candidate, and a directory that carries root configuration without being registered are each refused rather than resolved by ordering or by proximity. The root that holds the project's shared resources and the registration of every other root is never a valid target. The location the root's contents will be preserved into is checked for absence before anything else is read, and an existing location refuses the run — an existing destination is either a previous retirement of the same root or unrelated content under the same name, and the operation cannot tell which. Resolution and destination checking happen once, before any enumeration, sweep or read of the root's contents. Resolution is not the whole of the check: the registration records a path, and the operation reads that directory into an archive and afterwards deletes it, so being named in the registration is not by itself authorization to act on a location. The registered path must resolve strictly inside the project — judged on the resolved path, so a link pointing out of the project is refused however ordinary its name — and a registration that is absolute, that leaves the project through traversal, or that resolves to the project root itself is refused before anything is enumerated, read or removed. The registration is an ordinary file that a hand edit or a bad merge can change, and the one guarantee worth having about a destructive operation is that its target could not have been moved by editing a list.
**Invariants**:
- A target naming exactly one registered child root resolves; a target naming none is refused and the refusal enumerates the registered roots.
- A target matching more than one candidate is refused without selecting one.
- A directory carrying root configuration that is not registered is refused as a target.
- The root holding the shared resources and the root registration cannot be resolved as a target.
- An existing destination refuses the run before the root's contents are read.
- No enumeration, sweep or content read occurs before resolution and destination checking have both succeeded.
- A registered path that resolves outside the project refuses the run, and the refusal happens before any enumeration, archive or removal.
- An absolute registered path is refused; a child root is located relative to its parent.
- A registered path leaving the project through traversal segments is refused.
- A registered path that is a link resolving outside the project is refused, judged on the resolved path rather than on the name.
- Content outside the project is neither archived nor removed by a run refused on any of these grounds.
- The containment judgment is applied again to the path a resumed run reads from its journal, not only to the one it read from the registration.
**Source**: @root-retirement/end-a-subproject-without-pretending-it-never-happened
**Caching**: none
**Backward-Compatible**: yes

---

## Feature disposition preflight

**Affects**: retirement preflight over the retiring root's feature set, disposition vocabulary
**Behavior**: Before a retirement may proceed, an authored disposition record must account for every feature in the retiring root. The operation enumerates the root's features itself and compares that enumeration against the record: exactly one disposition per feature, every enumerated feature named, and no name that the enumeration did not produce. Each disposition pairs one term from a closed set of three — the work was delivered and goes away with the root; the feature was built and delivered nothing; authority over surviving work moves to a named feature — with a required free-text rationale saying where the work went or where it can be seen not to have gone. The rationale is prose the operator writes and the operation never parses: it is not a fourth term, it does not qualify or extend a term, and no rationale changes which terms are accepted. Its purpose is to keep a deliberately coarse vocabulary checkable, and it is what lets the vocabulary express a feature whose entire delivery was the removal of something — work witnessed only by markers on other features' files that the same run deletes, so that after the run nothing on disk shows the delivery happened. Dispositions are declared. Nothing about whether a feature was built is derived from the presence, absence or emptiness of its build state: features in one root legitimately carry different build artifacts, a feature may carry a buildfile and testcases without a coverage review, and a placeholder baseline naming no intents and no sources is present while attesting to nothing, so any such derivation would classify precisely the features whose classification matters. The record is authored before the run rather than assembled from answers during it, and it is preserved with the root's contents. It is also read structurally closed. A key the record's shape does not define is refused rather than ignored, because a dropped key is indistinguishable from one that was never written: a misspelled rationale field silently becomes a missing rationale, and a misspelled top-level key silently becomes an empty record that authorizes every deletion by accounting for nothing. For the same reason a term that names no target is refused when a target is present anyway — the term moves nothing while the target says authority moved, and honouring either one of the two statements decides on the operator's behalf which of them was meant.
**Invariants**:
- A record naming every enumerated feature exactly once, each carrying a term from the closed set and a rationale, passes the preflight.
- A record omitting an enumerated feature is refused, naming the omitted feature.
- A record naming a feature the enumeration did not produce is refused, naming it.
- A record naming one feature more than once is refused.
- A term outside the closed set of three is refused.
- A disposition carrying a valid term and no rationale is refused.
- A rationale is never parsed and never widens the accepted terms; the same three terms are accepted whatever any rationale says.
- A feature whose delivery was the removal of something, witnessed only by markers on other features' files that this run deletes, is recordable as delivered-and-deleted with that stated in its rationale.
- A feature carrying only a placeholder baseline is enumerated and requires a disposition like any other.
- A feature carrying an incomplete subset of the build artifacts its siblings carry is enumerated and requires a disposition like any other.
- A feature with a buildfile and testcases and no coverage review is not classified as unbuilt on that basis.
- No mutation occurs while the preflight is unsatisfied.
- A record carrying a key the shape does not define is refused, naming the unrecognized key, rather than decoded with that key dropped.
- A disposition carrying a target under a term that names no target is refused as the contradiction it is.
- A record that says exactly what its shape defines is accepted unchanged by the closed reading.
- The disposition record, rationales included, is present and readable alongside the preserved contents after a completed run.
**Source**: @root-retirement/say-what-became-of-every-feature-in-the-root
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The coarse vocabulary is deliberate and is why the rationale is required. A feature whose entire delivery was the removal of something, recorded only as markers on other features' files that the same run deletes, classifies honestly as delivered-and-deleted and leaves no trace afterwards showing that it was delivered at all. Requiring a free-text rationale makes that expressible in the record without adding a term for it — which is the alternative, and the one that would turn a closed vocabulary into an open one on the first awkward case.
- The enumeration is the authority for which features exist; the record is the authority for what became of them. Neither substitutes for the other, which is what makes "no missing and no extra names" a meaningful check rather than a restatement of the record.

---

## Project-wide, source-aware inbound sweep

**Affects**: cross-root reference scanning over specifications, source trees, shipped guidance and schema documents
**Behavior**: Root retirement performs its own inbound-reference sweep, distinct from the inventory that governs feature retirement. It spans every root in the project rather than the active one, because a sweep confined to one root can establish only that the root does not depend on itself. It reads source files, shipped guidance documents, schema documents and the authoring sources those documents are generated from, in addition to specifications — the closed specification-only scope that serves feature retirement well is blind to ownership markers on surviving source, to guidance instructing readers to run commands only the retiring root provides, and to schema documents claiming the root's concerns, which is where a retiring root's dependencies actually live. The sweep matches references to the retiring root's whole path space, not only to the features the disposition preflight enumerates: a marker may name a location in the root's namespace that has no directory in the root at all, so the enumeration will never produce it and a check comparing against enumerated features alone would certify a complete disposition set while live references still point into the root. Ownership held by the root's features over anything outside the root's directory is reported as a finding and blocks the retirement until a disposition re-homes it. The sweep recognizes the ways a feature is actually written down, not only the decorated ones. Ownership markers, `@feature` refs and command flags naming the root are the forms that announce themselves; the ordinary form is the group-qualified slug written plainly — `<group>/<feature>` in a comment, in a configuration value, in prose — and its component-qualified extensions. A grammar covering only the decorated forms misses most live references, so plain group-qualified and component-qualified occurrences are matched too, bounded at both ends so a longer identifier containing a feature's name is not mistaken for a reference to it. A single-segment feature name is an ordinary word as often as it is a reference and counts only where a path continues it; a group-qualified name is a reference as written. What survives those bounds is reported rather than dropped on suspicion of coincidence: a person dismisses a false positive while reading the preview, whereas a missed reference is discovered only after the root is gone.

The sweep is a line-based lexical scan, and its fail-closed rule reaches exactly as far as that. It reads each eligible file as text and applies its patterns line by line; it does not parse source, configuration or schema documents into structure, and it claims no understanding of structure — a reference is found because it is written down, wherever it is written down. Content that is binary carries no textual reference and is passed over. A file that is present but cannot be READ, and a directory that cannot be listed, are scan failures that refuse the retirement: "cannot tell" is never reported as "none". There is no separate unparseable condition, because nothing here parses; a structurally broken document is scanned like any other text and the references in it are still found. Prose mentioning the root is not a reference; an instruction naming something only the root provides is. Every finding names the artifact holding it, the position within it and the reference itself.
**Invariants**:
- A reference held in a root other than the retiring one is found; a sweep run against a single root is insufficient and is not what runs.
- An ownership marker on a surviving source file naming a feature of the retiring root blocks the retirement.
- A reference naming a location in the retiring root's namespace that corresponds to no enumerated feature blocks the retirement.
- An instruction in a shipped guidance document naming something only the retiring root provides blocks the retirement.
- The same instruction in a generated document and in the source it is generated from is reported as two findings.
- An ownership claim by the root's features over work living outside the root blocks the retirement unless a disposition re-homes it.
- A plain group-qualified feature reference is found wherever it is written: in source comments, in configuration values, in prose, and in a sibling root's own specifications.
- A component-qualified reference under a retiring feature is found.
- The same plain reference in a deployed document, in the module it was deployed from and in the authoring source it was generated from is three findings.
- A longer identifier that merely contains a feature's name is not reported as a reference to it.
- A single-segment feature name is reported only where a path continues it, and the same name in ordinary prose is not.
- Every finding carries the reference exactly as written, with no trailing punctuation absorbed into it.
- A file that is present but cannot be read refuses the retirement rather than being passed over.
- A structurally broken text document is scanned as text and the references written in it are found; being unparseable is not itself a condition this sweep detects or reports.
- Binary content is passed over without becoming a scan failure.
- Narrative prose mentioning the retiring root does not block the retirement.
- Every finding carries owning artifact, position and exact reference.
- The sweep completes before any mutation.
**Source**: @root-retirement/refuse-while-anything-outside-the-root-still-stands-on-it
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The existing inbound-reference inventory may inform this sweep and is never its authority. Its scope is a closed set of specification artifacts within one root, chosen deliberately so that a rule cannot be routed around by moving a name into prose — sound for the question it answers, and unable to see any of the dependencies a root retirement has.
- The one reference position that is genuinely new here is the location reference: matching against the root's path space rather than only its feature names is what makes an unenumerable reference visible, and it is the reason this sweep cannot be expressed as the existing inventory run twice.

---

## Re-home target readiness

**Affects**: validation of dispositions that move authority to a surviving feature
**Behavior**: A disposition that moves authority names a feature that must satisfy three conditions before the run mutates anything: it exists, it is active, and it already claims the surviving work. Existence is not directory presence; a feature carrying a retirement of its own — applied, or authored and waiting to be applied — is not active, because a record of something ending cannot own live code. The third condition is the one that has to be checked rather than promised: a target that will take ownership after the retirement does not own the work during the retirement, and that interval is exactly when surviving code has no home. All three conditions are evaluated across roots, since the target of a re-home is by construction in a different root from the feature releasing it.
**Invariants**:
- A re-home target that names no feature in the project refuses the retirement.
- A re-home target that is itself retired refuses the retirement.
- A re-home target carrying an authored but unapplied retirement refuses the retirement.
- A re-home target that exists and is active but does not yet claim the surviving work refuses the retirement.
- A re-home target satisfying all three conditions permits the retirement to proceed, and the check runs before any mutation.
- Target resolution crosses root boundaries.
**Source**: @root-retirement/refuse-while-anything-outside-the-root-still-stands-on-it, @root-retirement/say-what-became-of-every-feature-in-the-root
**Caching**: none
**Backward-Compatible**: yes

---

## Complete-directory archive with manifest

**Affects**: preservation of a retiring root's contents, archive manifest shape
**Behavior**: The preserved copy is the complete child directory reproduced byte for byte, including its configuration, its adapters and all of its build state — not a curated subset, since curation is a judgment about what mattered made at the one moment nobody can review it, and configuration and build state are what a later reader needs in order to interpret everything else. Alongside the copy the operation writes a manifest listing every preserved member with its content hash, and the manifest is itself covered, so a reader can establish both that the contents are unchanged and that the list of contents is unchanged. The archive and its manifest are complete before any other part of the project changes. Completeness here means verified, not assembled: the member hashes are necessarily computed on the source during the walk that judges escape and readability, which is the only moment those judgments can be made before anything is written, and that leaves an interval between the walk and the copy in which the source can change and the copy can go wrong. So every staged member is read back and re-hashed against the manifest before the archive is promoted. Verification means hashing the archived bytes; counting the members proves only that a list is the right length, and an archive that fails its own integrity check the moment it is written preserves nothing verifiable. A disagreement aborts the run while the project is still untouched. Members that cannot be preserved exactly are governed by the next fragment and abort the run rather than being copied approximately.
**Invariants**:
- Every file in the child directory, configuration and adapters and build state included, is present in the preserved copy with identical bytes.
- The manifest names every preserved member with its content hash and covers itself.
- Every staged member is re-read and re-hashed against the manifest before the archive is promoted, and a member whose archived bytes disagree with its recorded hash aborts the run, naming that member.
- A source member that changed between the walk and the copy is caught by that verification rather than promoted into an archive that fails its own integrity check.
- The verification hashes archived bytes; an archive whose member count and paths are intact but whose contents are not still fails it.
- A failure during archiving leaves the project exactly as it was.
- The archive and manifest are complete before the project's registration of the root changes.
**Source**: @root-retirement/keep-the-roots-work-verifiably-not-merely-out-of-the-way
**Caching**: none
**Backward-Compatible**: yes

---

## Escaping paths and unreadable members fail closed

**Affects**: the archive walk over the retiring root's contents, path resolution and read failure handling
**Behavior**: Two conditions encountered while walking the child directory abort the retirement rather than being worked around, and both are detected during the walk, before any content is copied and before the project has changed in any way. The first is a path that escapes the child directory. A symbolic link is followed only when its target resolves inside the directory; one resolving outside it aborts the run, as does any member whose resolved path lands outside the directory by any other route, including a traversal segment. Escape is judged on the resolved path rather than on the name, since an ordinary-looking name can resolve outside and a name full of traversal segments can resolve inside. There are exactly two wrong answers available and this rule refuses both: following the escape preserves content the root does not own and misreports what the archive holds, while skipping it quietly yields a copy that claims a completeness it does not have. The second condition is a member that cannot be read. There is no partial archive — an unreadable member means the guarantee the operation exists to make cannot be made, and a copy silently missing a file nobody could read is worse than no copy, because it is indistinguishable from a complete one. Both conditions report the member they refused on.
**Invariants**:
- A symbolic link whose target resolves outside the child directory aborts the run before anything is written.
- A symbolic link whose target resolves inside the child directory is preserved.
- A member whose resolved path lands outside the child directory through a traversal segment aborts the run before anything is written.
- Escape is determined by the resolved path, not by the textual form of the name.
- A member that cannot be read aborts the run before anything is written.
- Neither condition is ever resolved by skipping the member and continuing.
- A run aborted by either condition leaves the project exactly as it was, with no archive, no record and no change to the root registration.
- The refusal names the member that caused it.
**Source**: @root-retirement/keep-the-roots-work-verifiably-not-merely-out-of-the-way
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- These are the archive-side counterpart of the inbound sweep's fail-closed rule: in both places, a condition the operation cannot evaluate honestly refuses rather than reporting the optimistic answer. The archive states that it holds exactly the child directory, and a member that escapes it or cannot be read makes that statement unverifiable at the one moment it is being made.

---

## Archive invisibility to discovery, and archive integrity

**Affects**: feature and build-state discovery walks, archive verification check
**Behavior**: Preserved paths are excluded from every walk that enumerates live features and live build state — discovery is where the project decides what is present, and a preserved root that still appears there has not been retired. Exclusion from discovery is not exclusion from verification: a separate integrity check reads a manifest back and reports members whose content changed, members that are missing, and members present but unlisted. A placeholder baseline — build state written when a feature was first recorded, naming no intents and no sources — is history rather than corruption and passes the check unchanged. A retiring root will typically hold many such features, in some cases most of them, and a check reporting them as damage would fire on the bulk of what is being preserved; the second reader ignores that check, and the real findings go with it.
**Invariants**:
- A preserved feature does not appear in any enumeration of live features.
- Preserved build state does not appear in any enumeration of live build state.
- The integrity check passes immediately after a completed retirement.
- The integrity check reports a preserved member whose content changed after the fact.
- The integrity check reports a preserved member that is missing.
- The integrity check reports a member present in the archive and absent from the manifest.
- A preserved placeholder baseline naming no intents and no sources passes the integrity check unchanged.
- No integrity finding is raised on the grounds that preserved build state is empty.
**Source**: @root-retirement/keep-the-roots-work-verifiably-not-merely-out-of-the-way
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Emptiness is never read as a signal anywhere in this feature — not as corruption here, and not as evidence about whether a feature was built in the disposition preflight. Both rules have the same origin: a placeholder baseline is a fact about when a feature was recorded and carries no information about what became of it. Reading it as either damage or as proof of an unbuilt feature invents a claim from an absence.

---

## Mutation order, rollback and the resumable journal

**Affects**: transactional ordering of the retirement, rollback and resumable-journal state after an interrupted run
**Behavior**: The retirement completes its archive and its record before the project's registration of the root changes, and changes that registration last. The registration is what every other reader consults to know the root exists, so altering it first leaves every reader between that point and the end of the run looking at a project whose stated contents disagree with its actual ones. A failure at any point either restores exactly the prior state or leaves an explicit resumable journal of what remains outstanding; the absence of both is what turns an interruption into an investigation. The journal names the outstanding steps in the order they must happen, in terms a person can act on and a resumed run can execute. A resumed run continues from the journal rather than restarting, because restarting would re-attempt steps whose preconditions the first attempt consumed — a destination required to be absent now exists, and content already moved is no longer where the first step looks for it. While a journal is outstanding, another retirement of the same root refuses.

An in-flight retirement is found by looking for its journal, before the registration is consulted at all. This is what makes the ordering resumable rather than merely recorded. The final step removes the root's directory, then the registration, then the journal; the state a resume most needs to reach is therefore one where the registration no longer names the root, and a run that resolved its target through the registration first could never see it — the journal would document an interruption nothing could finish. Every step the journal names is idempotent, so a resumed run completes whatever remains regardless of how much of the final step already happened. A journal that cannot be read or parsed refuses the run rather than being read as nothing in flight, since starting a fresh destructive run over a part-finished one is the failure that ordering exists to prevent. The retirement is complete only once the registration no longer names the root and no journal outlives it.
**Invariants**:
- An interruption before the archive is complete leaves the project exactly as it was.
- An interruption after the archive is complete and before the registration changes leaves either the prior state or an outstanding resumable journal.
- No interruption leaves the registration missing the root while its contents are still in place.
- No interruption leaves the registration naming the root while its contents have moved and no journal exists.
- The journal lists the outstanding steps in execution order.
- A resumed run performs the outstanding steps and does not repeat completed ones.
- A second retirement of the same root refuses while a journal is outstanding.
- An in-flight retirement is found by its journal without consulting the root registration, and is found equally when named by the root's registered path.
- A run interrupted at any mutation boundary is completed by the next run, including one interrupted after the registration has already been changed.
- Every step the journal names can be performed again without harm, whatever part of it already succeeded.
- A journal that cannot be read or parsed refuses the run rather than being treated as nothing in flight.
- After a completed run the registration does not name the root and no journal outlives it.
**Source**: @root-retirement/never-leave-a-project-half-retired
**Caching**: none
**Backward-Compatible**: yes

---

## Retirement authorization, preview and unattended runs

**Affects**: the retirement operation's command surface and its confirmation contract
**Behavior**: The operation exposes a preview that reports the entire preflight — target resolution, destination availability, the features requiring dispositions, the inbound findings, the re-home readiness and the extent of what would be preserved — while writing nothing, reserving nothing and leaving no state behind. Execution requires explicit authorization from a person after that preview has been shown. A run with nobody available to authorize it refuses and writes nothing: ending a subproject has no safe default, and an unattended run that proceeds is afterwards indistinguishable from one somebody approved. Preview remains available unattended, since reporting what would happen commits to nothing. This authorization is the operation's own and does not relax the rule that refuses to retire a feature with anything built; that rule keeps its current meaning and its current scope, because a feature retirement removes nothing while this operation removes the root and preserves its contents. The record this operation writes is history: it states what was decided and where the evidence went, and it never becomes the owner of surviving work.
**Invariants**:
- A preview requested on its own leaves the project byte-identical.
- Execution proceeds only after explicit authorization following a shown preview.
- An unattended execution refuses and writes nothing, and says that the absence of a person is the reason.
- An unattended preview succeeds and writes nothing.
- Retiring a root whose features have artifacts is permitted by this operation.
- Retiring one of those features on its own remains refused with its existing reason and message.
- The retirement record is not accepted as the owner of any surviving work.
**Source**: @root-retirement/end-a-subproject-without-pretending-it-never-happened
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Dispositions are authored ahead of the run rather than collected at the prompt, so the confirmation is a decision about executing a reviewed plan rather than the last of many answers. The unattended refusal therefore blocks only the execution, not the preparation.
