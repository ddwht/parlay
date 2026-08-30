# Domain document API — Dialogs

---

### Checking a document you edited by hand

**Trigger**: Someone edits the project's domain model in their own editor and wants to know whether it is still valid before anything else reads it.

User: parlay validate --type domain-model domain-model.yaml
System (background): Runs the one engine that owns the rules and passes its findings through unaltered.
System: ==2== findings:
    a. ==error== · ==entities.ExpenseReport.fields.settledAt== · ==field-duplicate-name== — ==the field `settledAt` is declared twice on `ExpenseReport`==
       Fix: ==remove one of the two declarations, or rename the one that means something different==
    b. ==warning== · ==<domain-model>== · ==domain-operations-deprecated== — ==the `operations:` block is deprecated==
       Fix: ==move these into the owning feature's capabilities==
System: The warning does not block anything. The error does.

---

### Getting the same answer whichever way you asked

**Trigger**: The same document reaches the checking path a write uses, rather than the direct one.

System (background): The write path calls the same engine, in the same mode, anchoring a document with no path of its own on the same token the direct path uses.
System (background): Compares the two finding sets field for field — code, element path, severity, message, fix.
System: Identical. A document accepted here and refused there would be the tool contradicting itself about one file, and the parity check is what makes that a test failure rather than a bug report.

---

### Reviewing what a feature is asking the shared vocabulary to say

**Trigger**: The loop reaches the artifacts→build boundary on a feature that wrote a contribution of its own.

System (background): Reads the feature's proposal and the shared document, and compares them element by element.
System (background): Collects which other features name the affected entities and which fixtures hold records of them.
System: ==@expenses/submit-expense== proposes ==one== new field: ==ExpenseReport.settledAt== (==datetime==).
System: ==Two== other features read ==ExpenseReport== — ==@expenses/dashboard== and ==@expenses/expense-list== — and ==two== fixtures hold records that would need it: ==dashboard/seed==, ==expense-list/mixed-status==.
System: Accept this contribution into the project domain model?
    a. Accept — merge the additions
    b. Leave it proposed — write nothing
User: a
System (background): Merges through the single writer, computing the merge against the document as it stands now.
System: Accepted. The shared document is a shared thing, so this dirtied every feature that reads ==ExpenseReport==, not only ==@expenses/submit-expense==.

---

### A proposal the shared document already answers differently

**Trigger**: The same review, on a feature whose proposal disagrees with what the project already agreed.

System (background): Compares element by element; one element is present on both sides and described differently.
System: Cannot accept ==@expenses/submit-expense== — ==1== element disagrees with the project model:
    a. ==entities.ExpenseReport.fields.amount== — project: ==integer, minor units==; proposed: ==decimal==
System: Nothing was merged, including the ==2== additions that do not conflict. Which of two descriptions is right is a design question, and merging the half that agrees would leave the project holding part of a proposal nobody accepted.
System: The other ==2== additions are still recorded in the proposal and land whole once this is settled.

---

### A proposal that restates what is already there

**Trigger**: The same review, on a feature that names an entity the project already defines identically.

System (background): Finds the element present on both sides and described the same way.
System: ==@expenses/dashboard== proposes nothing new — ==3== elements it names are already in the project model, described identically.
System: Nothing to accept and nothing to resolve. Restating a shared concept is how a feature says it depends on one.

---

### The document moved while you were deciding

**Trigger**: The shared document is changed by someone else between the moment a change is read and the moment it is written.

System (background): Presents the content-identity token from its own read.
System (background): The stored document no longer matches that token.
System: Nothing was written. The project model changed since this was read — presented ==sha256:9f2c…==, current ==sha256:41ab…==.
System: Read it again and re-decide. The two changes are not merged, because which of them is right where they meet is not something a writer can work out.

---

### Starting a project that has no shared vocabulary yet

**Trigger**: The first change lands on a project with no domain model document at all.

System (background): The read finds no document and returns the distinguished empty token.
System (condition: the write presents the empty token): Creates the document holding exactly what was submitted.
System (condition: the write presents any other token): Nothing was written — a token that is not the empty one describes a document that was read and has since gone, which is a stale read rather than a first write.

---

### Reviewing the change rather than the rewrite

**Trigger**: A change touching one entity is reviewed as a diff before it is committed.

System (background): Rewrites the document deterministically, in the author's declaration order, carrying the deprecated operations block through byte for byte.
System: The diff is ==4== lines, all inside ==ExpenseReport==. Every other declaration, and the deprecated ==operations:== block, is byte-identical to what was there before.
System: That is the point of rewriting deterministically: a review that also has to establish which of forty moved lines were meaningful is not a review of the decision.
