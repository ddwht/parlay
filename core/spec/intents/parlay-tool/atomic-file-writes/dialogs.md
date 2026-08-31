# Atomic file writes — Dialogs

---

### Upgrading a project that is already up to date

**Trigger**: Someone re-runs a deployment over inputs that have not changed since the last run.

User: parlay upgrade
System (background): For each file it would deploy, compares the content it is about to write against what is already there.
System: Deployed ==0== files. ==31== were already identical and were not touched.
System: Nothing changed, so nothing has a new timestamp. A changed timestamp in this project means changed content.

---

### Upgrading after one thing changed

**Trigger**: The same command, after a single source document was edited.

System (background): Compares content file by file; one differs.
System: Deployed ==1== file: ==.parlay/schemas/domain-model.schema.md==. ==30== were already identical.
System: The one line in the run output and the one file in the diff are the same fact, which is the point.

---

### Reading a file while it is being replaced

**Trigger**: An agent reads a deployed instruction file at the moment a deployment is writing it.

System (background): Stages the new content beside the target, flushes it to storage, then moves it into place.
System (condition: the read lands before the move): The reader gets the previous file, whole.
System (condition: the read lands after the move): The reader gets the new file, whole.
System: There is no third case. A reader never observes a file that says less than either version of it said.

---

### An upgrade interrupted partway through a file

**Trigger**: The process is killed between staging the new content and moving it into place.

System (background): The target was never opened for writing; only the staged sibling was.
System: The deployed file is exactly what it was before the run. A staged sibling is left on disk.
System: The next run overwrites that sibling rather than continuing it, so an interrupted attempt costs a re-run and nothing else.

---

### A file that cannot be read

**Trigger**: A deployed file exists but the tool cannot read it back to compare.

System (background): Attempts to read the existing content; the read fails for a reason other than the file being absent.
System: Cannot deploy ==.claude/skills/parlay-loop/SKILL.md== — the existing file could not be read: ==permission denied==.
System: Nothing was written. A file that cannot be read is not a file known to differ, and treating the two the same is how a permissions problem turns into lost content.

---

### Deploying into a project for the first time

**Trigger**: A deployment runs against a project where none of the target locations exist yet.

System (background): Finds no existing content at any target — absence, not a read failure.
System: Deployed ==31== files, creating the locations that were not there.
System: A first deployment writing everything and a second deployment writing nothing are the same rule seen twice.

---

### A deployment write that went around the primitive

**Trigger**: A change adds a file-writing call directly inside a deployment path.

System (background): The build-time check parses every file in the places that deploy content and looks for direct write calls.
System: Build failed — forbidden write primitive used in ==core/internal/embedded==. Use the shared atomic write.
System: The fix is to route that write through the primitive, or to move it out of a deployment path if it is not deploying anything. Widening the check's scope until the call stops being reported is not a fix; it is the guarantee being quietly reduced.

---

### A check that would have passed for the wrong reason

**Trigger**: The set of places the build-time check covers stops resolving to any files.

System (background): Scans the covered places and counts what it examined.
System: Check failed — examined ==0== files. The covered set is wrong.
System: A scan that matched nothing reports success forever, which is worse than a scan that fails, because nobody looks at a check that has always been green.
