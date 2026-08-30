# Root retirement — Dialogs

---

### Seeing what retiring a root would do

**Trigger**: A maintainer has decided a subproject is finished and wants to know what ending it involves before deciding anything else.

User: parlay retire-root studio --preview
System (background): Resolves ==studio== against the registered child roots; confirms it names exactly one and is not the root holding the shared resources.
System (background): Enumerates every feature in the root; sweeps every root, source tree, shipped guidance and schema document for anything pointing into ==studio==.
System: ==studio== resolves to one registered child root at ==studio/==. Nothing has been written.
System: ==18== features need a disposition. None is recorded yet:
    a. ==design-loop/design-loop==
    b. ==design-loop/design-loop-fallback==
    c. ==design-loop/vocabulary-validation==
    d. ==… 15 more==
System: ==102== files would be preserved, including ==studio/.parlay/config.yaml==, ==2== adapters and ==49== files of build state.
System: ==4== things outside the root are owned by its features and would survive without an owner:
    a. ==core/internal/atomicfile/atomicfile.go== · ownership marker · `studio-foundation/studio-deployer`
    b. ==core/internal/commands/domain_validator.go== · ownership marker · `domain-model-editor/domain-model-editor-validation`
    c. ==core/internal/agent/domain_contribution.go== · ownership marker · `domain-model-editor/feature-contributions`
    d. ==core/internal/commands/root.go== · ownership marker · `design-loop/vocabulary-validation`
System: ==c== names a path in the root's namespace with no feature directory in the root, so it will never appear in the disposition list. It still has to be resolved.
System: Record a disposition for each feature, then run without `--preview`.

---

### Retiring a root whose account is complete

**Trigger**: The dispositions are written and the surviving work already has its new homes.

User: parlay retire-root studio --dispositions studio-retirement.yaml
System (background): Re-runs the whole preflight: target resolution, destination availability, disposition completeness, project-wide inbound sweep, and re-home target readiness.
System: Dispositions cover all ==18== features exactly once: ==5== delivered-and-deleted, ==11== built-but-undelivered, ==2== authority-re-homed-to.
System: Re-home targets checked — ==@parlay-tool/atomic-file-writes== and ==@parlay-tool/domain-document-api== both exist, are active, and already claim the code that survives.
System: Nothing outside the root points into it.
System: This will preserve ==102== files to ==.parlay/retired/studio/== with a manifest, write the retirement record, and remove ==studio== from the project's root registration.
System: Retire ==studio==?
    a. Retire — preserve the root and deregister it
    b. Cancel — change nothing
User: a
System (background): Writes the archive and its manifest, then the retirement record, then removes the root registration.
System: Retired ==studio==. ==102== files preserved and hashed at ==.parlay/retired/studio/==; the retirement record and the dispositions are preserved with them.
System: `parlay doctor` no longer enumerates the root's features. Run `parlay retired-roots --check` to verify the archive against its manifest.

---

### Refusing an incomplete account of the features

**Trigger**: The disposition record does not match the features actually in the root.

System (background): Enumerates the root's features and compares them against the names in the record.
System: Cannot retire ==studio== — the dispositions do not account for the root as it is:
    a. ==studio-deferred/mid-edit-resumability== is in the root and has no disposition
    b. ==studio-foundation/figma-mcp-client== is named twice
    c. ==studio-foundation/legacy-preview== has a disposition and is not in the root
System: ==b== has to become one entry, and ==c== is usually a name that is wrong rather than a feature that is missing — the feature it meant is still unaccounted for.
System (condition: a disposition uses a term outside the closed set): ==design-loop/design-loop-fallback== records `abandoned`, which is not one of the three: `delivered-and-deleted`, `built-but-undelivered`, `authority-re-homed-to <feature>`. A fourth term is a category nobody can look up later.
System (condition: a disposition has no rationale): ==studio-foundation/figma-mcp-via-host-agent== records `delivered-and-deleted` with no rationale. The term alone is a classification nobody can check — write what it delivered and where that can be seen.
System (condition: the operator asks how to record a retraction): ==studio-foundation/figma-mcp-via-host-agent== delivered the *absence* of code, and its only witnesses are ==8== extension markers on files this run deletes, so after the run nothing on disk shows it delivered anything. The term stays `delivered-and-deleted` — there is no fourth term and the rationale does not create one. Put the awkward part in the rationale: what it delivered was the retraction, recorded at those markers, which are inside this deletion.
System (condition: a feature carries only a placeholder baseline): ==studio-deferred/mid-edit-resumability== has a baseline naming no intents and no sources. That is history, not evidence — it says when the feature was recorded and nothing about what became of it. It needs a disposition like every other feature, and nothing is inferred from the emptiness.
System: Nothing was read past the preflight and nothing was written.

---

### Refusing while surviving code still belongs to the root

**Trigger**: A feature outside the root, or code that survives, still points into the retiring root.

User: parlay retire-root studio --dispositions studio-retirement.yaml
System (background): Sweeps every root, not only the active one, and reads source files, shipped guidance and schema documents as well as specifications.
System: Cannot retire ==studio== — ==6== things outside the root still point into it:
    a. ==core/internal/atomicfile/atomicfile.go==:1 · ownership marker · `studio-foundation/studio-deployer`
    b. ==core/internal/commands/root.go==:456 · ownership marker · `design-loop/vocabulary-validation`
    c. ==core/internal/agent/domain_contribution.go==:1 · ownership marker · `domain-model-editor/feature-contributions`
    d. ==core/internal/embedded/skills/loop.skill.md== · instruction · `parlay domain-edit`
    e. ==.parlay/modules/loop.md== · instruction · `parlay domain-edit`
    f. ==core/internal/embedded/schemas/layout.schema.md==:58 · ownership marker · `design-loop/design-loop`
System: ==c== names a path in the root's namespace that has no feature directory in the root; the disposition list will never contain it, and the sweep is what catches it.
System: ==d== and ==e== are the same instruction in its source and in the copy that ships. Both are reported, because a reader reaches the shipped copy and the next deployment restores it from the source.
System: The existing feature-retirement inventory finds none of these — it reads a closed set of specification artifacts inside one root, which is right where it lives and blind to every finding above.
System: Resolve each of these, then retire.

---

### Naming a home for surviving work that is not ready

**Trigger**: A disposition re-homes authority to a feature that does not exist, is retired, or has not taken the work on yet.

System (background): Checks each re-home target before touching anything.
System (condition: the named feature does not exist): ==@parlay-tool/atomic-file-writes== is not a feature in this project. Code that survives a retirement needs a home that exists before the run, not a name it acquires afterwards.
System (condition: the named feature is itself retired): ==@parlay-tool/domain-document-api== is itself retired, by amendment ==004-folded-into-domain-core==. A retirement record cannot own live code.
System (condition: the named feature exists and is active but does not claim the code): ==@parlay-tool/atomic-file-writes== is active but does not yet claim ==core/internal/atomicfile/atomicfile.go== — the file still names ==studio-foundation/studio-deployer==. Move the claim first: until it moves, retiring leaves the file owned by something the project no longer has.
System: Nothing was written.

---

### Refusing to preserve something it cannot preserve exactly

**Trigger**: The archive walk meets a path that escapes the root, or a member it cannot read.

System (background): Walks the complete child directory to build the archive and its manifest.
System (condition: a symbolic link resolves outside the child directory): ==studio/.parlay/adapters/shared.adapter.yaml== is a symbolic link resolving to ==core/.parlay/adapters/go-cli.adapter.yaml==, outside ==studio/==. Following it would preserve something the root does not own; skipping it would produce an archive claiming to be complete when it is not. Neither is on offer.
System (condition: a member's resolved path escapes the child directory): ==studio/.parlay/build/_project/../../../core/domain-model.yaml== resolves outside ==studio/==. Escape is judged on where a path resolves, not on how the name looks.
System (condition: a symbolic link resolves inside the child directory): ==studio/.parlay/adapters/local.adapter.yaml== resolves inside ==studio/== and is preserved like any other member.
System (condition: a member cannot be read): ==studio/.parlay/build/design-loop/design-loop/testcases.yaml== cannot be read. There is no partial archive — an archive silently missing a file nobody could read is worse than no archive, because it is indistinguishable from a complete one.
System: Nothing was written. No archive, no record, no change to the root registration — the project is exactly as it was.

---

### Refusing a destination that is already there

**Trigger**: The place the root would be preserved into already exists.

User: parlay retire-root studio --dispositions studio-retirement.yaml
System: Cannot retire ==studio== — ==.parlay/retired/studio/== already exists. That is either an earlier retirement of this root or something unrelated with the same name, and both are your call to make. Nothing was read past this point.

---

### Picking up an interrupted retirement

**Trigger**: A run failed after the archive was written and before the root registration changed.

User: parlay retire-root studio --dispositions studio-retirement.yaml
System (background): Finds the resumable journal left by a previous run.
System: A retirement of ==studio== is part-finished. The archive at ==.parlay/retired/studio/== is complete and verified against its manifest; the retirement record is written; the root is still registered.
System: The journal lists what is outstanding, in the order it must happen:
    a. Remove ==studio== from the root registration
System: Resume?
    a. Resume — finish the outstanding steps
    b. Cancel — leave it as it is
User: a
System: Retired ==studio==. The completed steps were not repeated.
System (condition: a second retirement of the same root is attempted while the journal is outstanding): A retirement of ==studio== is already part-finished. Finish or discard it before starting another — two runs converging on one half-finished state is how a recoverable interruption stops being one.

---

### Declining to answer for a person who is not there

**Trigger**: A retirement is reached with nobody available to authorize it.

User: parlay retire-root studio --dispositions studio-retirement.yaml --non-interactive
System: Blocked — retiring a root ends a subproject, and there is no safe default for that. Nothing was written.
System: `--preview` reports the same preflight unattended and commits to nothing. Re-run interactively to authorize the retirement itself.

---

### Retiring a root whose features were built

**Trigger**: The maintainer asks why this is allowed here and refused for a single feature.

User: Half these features have artifacts. Why doesn't the "nothing built" rule stop this?
System: That rule governs feature retirement, and it is right there: retiring a feature removes nothing, so a feature with artifacts would keep them on disk and keep being read by everything that enumerates features.
System: This operation removes the whole root from the project and preserves its contents with a manifest, which is what lets it answer for built features. It does not loosen that rule — it is a separate decision with its own record and its own preflight.
System: ==11== of the ==18== features here were built and delivered nothing. That work is the main thing the archive exists to keep.
