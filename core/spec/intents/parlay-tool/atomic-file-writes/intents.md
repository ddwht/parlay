# Atomic file writes

> Every file parlay deploys into a project — schemas, skills, agent configuration — is written by the tool and read by somebody else, often while the tool is still writing. This feature owns the one primitive those writes go through: replace a file indivisibly, or do not touch it at all when its content would not change. It is small on purpose. Its whole value is that there is exactly one of it.

---

## Never read a file that is only half written

**Goal**: Someone reading a file parlay deployed gets a whole file — the previous one or the new one — so a read that happens to land during an upgrade never produces behavior nobody wrote, and a crash mid-upgrade never leaves a project holding a truncated instruction file.
**Persona**: A person or agent reading a deployed file while the tool may be writing it
**Priority**: P0
**Context**: Deployment writes many files at once, and the agents and tools that consume them read whenever they happen to run. Writing in place makes a truncated file an observable state: half an instruction document is not obviously broken, it is quietly wrong, and it reads as an instruction document that says less than it should.
**Action**: Stage the new content beside the target, make it durable, and then move it into place, so the file is replaced in one step rather than rewritten in many.
**Objects**: deployed file, staged content, replacement, interrupted run

**Constraints**:
- Replacing a file is one step from a reader's point of view. There is no moment at which the target holds part of the new content.
- The new content must be durable before the replacement, not merely written. Moving a name onto content that has not reached storage trades a truncated file for an empty one, which is the same failure with a more surprising shape.
- An interruption before the replacement leaves the previous file exactly as it was. The previous file being intact is the outcome that matters; leftover staged content is acceptable and must not be cleaned up at the cost of the target.
- A failed replacement leaves the staged content in place deliberately, so that the next run overwrites it rather than tripping over it. Staged content from an interrupted run is never appended to.
- A successful replacement leaves nothing behind beside the target.
- Where the file is going may not exist yet; a first deployment creates the containing location rather than failing.

**Verify**:
- After a successful write the target holds exactly the new content and no staged remnant is present.
- When the replacement step fails, the target holds exactly its previous content and the staged content remains.
- Staged content left by an earlier failed run is overwritten by the next attempt, not extended.
- A write into a location that does not exist yet creates it and succeeds.
- A reader sampling the target throughout a write only ever observes complete content — the previous or the new.

---

## Re-run a deployment and have nothing change

**Goal**: Someone who runs the same deployment twice over unchanged inputs ends up with a project that is untouched the second time, so a changed file is evidence that something actually changed rather than evidence that a command was run.
**Persona**: A person maintaining a project that receives deployed files
**Priority**: P1
**Context**: Deployment is re-run often — after an upgrade, after a repair, as part of a check. A deployment that rewrites every file every time makes timestamps meaningless, fills reviews with diffs that say nothing, and makes "what did this command change?" unanswerable at exactly the moment someone is trying to answer it.
**Action**: Compare the content about to be written against what is already there and do nothing when they are the same.
**Objects**: deployed file, existing content, unchanged run, change record

**Constraints**:
- The comparison is on content, not on timestamps or sizes. Two files with the same content are the same file for this purpose regardless of when they were written.
- A write that is skipped reports that it was skipped, so a caller can say what a run changed rather than what it visited.
- Being unable to read the existing file is not the same as the content differing, and is never treated as a reason to write. Overwriting a file that could not be read is how an unrelated permissions problem becomes data loss.
- A file that is simply not there yet is the ordinary first-deployment case, not a read failure, and is written.
- The skip is an optimization of the write, never of the decision to deploy: what gets deployed is decided before this, and skipping an identical file changes nothing about that.

**Verify**:
- A second run over unchanged inputs writes zero files and reports zero changes.
- A run over inputs where one file changed writes exactly that file.
- Timestamps of skipped files are unchanged after a run.
- A target that exists but cannot be read fails the write and reports why, rather than being overwritten.
- A target that does not exist is written.

---

## Have the guarantee hold for every deployed file, not most of them

**Goal**: Someone relying on "an upgrade never leaves a half-written file and an unchanged upgrade writes nothing" can rely on it for the whole upgrade, so the claim is a property of the tool rather than a property of the parts of it that were remembered.
**Persona**: A person maintaining the tool's deployment paths
**Priority**: P1
**Context**: Deployment happens from more than one place, because different kinds of deployed content are assembled by different parts of the tool. A guarantee stated about one of them is a guarantee about a fraction of the files a person actually receives, and the fraction is invisible from the outside — the files that skipped the primitive look exactly like the ones that did not.
**Action**: Route every deployment write through the one primitive, and make a deployment write that bypasses it fail the build rather than fail quietly.
**Objects**: deployment path, write primitive, build-time check, guarantee

**Constraints**:
- The primitive is reachable from every part of the tool that deploys files. A helper only one of them can reach leaves the rest of an upgrade unguaranteed while sounding like it covers everything.
- Bypassing the primitive from a deployment path is refused at build time, not reviewed for. A convention held by attentiveness is one that decays at the first hurried change.
- The check names the places it covers, and the honest reading of it is "a deployment write in one of these places is guarded" rather than "no unguarded write exists anywhere". The remedy for a write outside those places is to move the write into a place the check covers, not to widen the check until it means nothing.
- The check fails if it examined nothing. A scan that silently matched no files passes forever and reports success it never earned.
- The primitive itself is exempt from the rule it enforces; it is the thing being mandated.

**Verify**:
- A deployment write that bypasses the primitive fails the build, naming the place and the bypass.
- The check fails when its scope resolves to no files at all.
- The primitive's own use of the underlying write is not reported as a bypass.
- An unchanged upgrade writes zero files across every deployment path, not only one of them.

**Questions**:
- The check covers a named set of places rather than deriving them, so a new deployment path added elsewhere is unguarded until someone adds it. Deriving the set would need a definition of "deploys files" the codebase does not currently have, and a check that guesses at it would either miss paths or block ordinary writes. Naming them and saying so is the honest version; the first deployment path that is genuinely awkward to move is the case that should force the question.
