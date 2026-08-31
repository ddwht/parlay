# Atomic file writes — Infrastructure

---

## Indivisible replacement of a deployed file

**Affects**: the write primitive every deployment path uses
**Behavior**: New content is staged beside its target under a name derived from the target, flushed to durable storage, and then moved onto the target. From a reader's position the file changes in one step. The staging name is fixed rather than unique, which is correct for one writer at a time and is what makes the leftover-from-a-failed-run case recoverable: the stage is opened so that any previous remnant is discarded rather than extended. Failure is specified per step, because which artifacts a failure leaves behind is the whole of what a caller can reason about afterwards.
**Invariants**:
- Creating the containing location fails before anything is staged, leaving no remnant.
- A failure while staging removes the staged content and leaves the target untouched.
- A failure making the staged content durable removes the staged content and leaves the target untouched.
- A failure at the move leaves the staged content in place and the target untouched.
- A success leaves the target holding the new content and no staged remnant.
- Staged content is made durable before the move, so no completed move can expose content that never reached storage.
- A remnant from a previous failed attempt is discarded at the start of the next attempt, never appended to.
**Source**: @atomic-file-writes/never-read-a-file-that-is-only-half-written
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The move step is substitutable so that a test can make it fail. The alternative — arranging a real move failure — is environment-dependent, and the failure it exercises is the one whose remnant behavior is deliberate rather than incidental.
- A fixed staging name is a single-writer assumption. It holds for deployment, where one process writes a project's files at a time, and would not hold for a document several processes write.

---

## Writing nothing when nothing changed

**Affects**: content-equality check preceding a deployment write
**Behavior**: Before writing, the intended content is compared against what the target already holds, and identical content is not written. The comparison is over content only. Absence of the target is the ordinary first-deployment case and proceeds to the write; a failure to read an existing target is neither absence nor difference, and refuses the write rather than resolving into one. The caller learns whether a write happened, so a run can report what it changed rather than what it considered.
**Invariants**:
- Identical content is not written and the target's timestamp is unchanged.
- Differing content is written through the indivisible replacement.
- An absent target is written.
- A target that exists and cannot be read refuses the write and surfaces the underlying reason.
- Every call reports whether it wrote.
**Source**: @atomic-file-writes/re-run-a-deployment-and-have-nothing-change
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Treating an unreadable target as "differs" would make an unrelated permissions problem into an overwrite, which is the one outcome a deployment must not produce while claiming to be idempotent.

---

## One primitive, reachable from every deployment path

**Affects**: placement of the shared write primitive relative to the parts of the tool that deploy files
**Behavior**: The primitive lives in a unit of its own, depended on by every part of the tool that deploys files and depending on none of them. It is not placed inside any one deployment path, because the deployment paths cannot depend on each other — one of them already depends on the other's contents — and a primitive reachable from only one of them would leave the rest of an upgrade unguaranteed while the guarantee is stated about the upgrade as a whole.
**Invariants**:
- Every part of the tool that deploys files can reach the primitive.
- The primitive depends on no deployment path, so adding or removing one changes nothing about it.
- The idempotency and indivisibility guarantees hold across every deployment path, measured as a whole run rather than per path.
**Source**: @atomic-file-writes/have-the-guarantee-hold-for-every-deployed-file-not-most-of-them
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The primitive was salvaged from a deployment path being deleted, and was the better-engineered of the two on this exact point: the surviving paths replaced every file unconditionally on every run. Porting it before the deletion rather than after was deliberate, and it currently carries the ownership markers of the feature it was salvaged from — a feature in a root being retired. This feature is the home it moves to.
- Its markers name a component about manifest-based ownership as well as atomic writes and idempotency. Only the write half lives here; deciding which files a deployment owns stays with the deployment paths, and this feature does not claim it.

---

## Bypassing the primitive fails the build

**Affects**: build-time check over the parts of the tool that deploy files
**Behavior**: The places that deploy files are named explicitly, and every source file in them is examined for direct use of the underlying write facilities the primitive exists to replace. A use found there fails the build, naming the place and the call. The named set is a claim about coverage rather than about the whole codebase: the guarantee is "a deployment write in one of these places is guarded", and the remedy for a deployment write elsewhere is to move it into a covered place, never to enlarge the set until the check stops meaning anything. The check refuses to pass when its scope resolved to nothing.
**Invariants**:
- A direct write call in a covered place fails the check, naming the place and the call.
- The primitive's own use of the underlying facilities is not reported.
- A generator or tool in a covered place that deploys nothing is excluded explicitly, and the exclusion is by name rather than by pattern.
- The check fails when it examined no files.
- The scope is anchored on the project's own root rather than on the checking file's position, so relocating the check does not silently narrow it.
**Source**: @atomic-file-writes/have-the-guarantee-hold-for-every-deployed-file-not-most-of-them
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The check reads source structure rather than matching text, so a call is recognized by what it is rather than by how it is spelled at that line.
- The recorded precedent is worth keeping: when one write was found outside the covered set, the fix taken was to move that write into a covered place. Widening the set was available and was not the fix.
