# Create-domain-model — Dialogs

---

### Rename to `parlay create-domain-model` and Preserve Brownfield Behavior

**Trigger**: A designer (or scripted caller) invokes `parlay create-domain-model` against a project that has features with extractable signals — intents and dialogs that name entities, relationships, and operations the extractor recognizes. This is the brownfield code path: same input, same extractor, same output as the previous `parlay extract-domain-model` command, just under the new name.

User: Runs `parlay create-domain-model` from the project root on a project whose features have extractable entities, relationships, and operations.
System: Walks every feature under `spec/intents/`, runs the extractor against intents and dialogs, assembles entities/relationships/operations, and writes a populated `domain-model.yaml` to the project root. Prints a one-line summary along the lines of `Wrote domain-model.yaml — N entities, M relationships, K operations.` Exit code 0. The byte content of the produced file is identical to what `parlay extract-domain-model` produced before this feature shipped against the same fixture.

#### Branch: `parlay extract-domain-model` is invoked after the rename ships

User: Runs `parlay extract-domain-model` (the old command name) on a project that has been upgraded past this feature's release.
System: The CLI's command dispatcher does not find a registered command named `extract-domain-model`. Returns the standard "unknown command" error message ("Error: unknown command \"extract-domain-model\" for \"parlay\"" or whatever the underlying CLI library produces) and exits non-zero. There is no alias, no deprecation warning, no "did you mean `create-domain-model`?" hint beyond whatever the standard suggester produces by edit-distance — Core does not add a special case. Hard cutover.

#### Branch: `parlay --help` lists the new name only

User: Runs `parlay --help` after the rename ships.
System: The command list shows `create-domain-model` with its description. `extract-domain-model` is absent. Designers reading the help text see exactly one canonical command for producing a domain model. The generic-CLI hardcoded command list is consistent with what `parlay --help` advertises — both updated together.

#### Branch: `parlay upgrade` migrates the deployed skill

User: Pulls a Core release containing this feature and runs `parlay upgrade` in an existing project.
System: The deployer enumerates the embedded skills list, finds `parlay-create-domain-model` and not `parlay-extract-domain-model`, writes `.claude/skills/parlay-create-domain-model/SKILL.md` from the embedded source, and removes the now-stale `.claude/skills/parlay-extract-domain-model/SKILL.md` directory. After upgrade, the project has exactly one skill file for this command — under the new name. The deployer's embedded-skills list and the generic-CLI hardcoded command list are both consulted from the same compiled-in source, so they cannot disagree.

#### Branch: in-repo callers of the old name in OTHER features' specs

User: Searches `spec/intents/` for `extract-domain-model` after this feature's intent edits land but before the cross-cutting rollout sweep across other features.
System: Finds stale references in `parlay-tool/domain-model/`, `parlay-tool/multi-root/`, `qualified-identifier-resolver/dialogs.md`, `infrastructure-layer/dialogs.md`, and the heavy `studio-support/domain-model-yaml-migration/` feature. These are explicitly out of scope for this feature's specs — they are tracked separately for their own design-loop runs and are expected to be updated as those features are revisited. This dialog branch documents the boundary: this feature owns the rename of the command itself; it does not own re-authoring other features' intents and dialogs.

#### Branch: brownfield extraction tests pass under the new name

User: A maintainer runs the brownfield-extraction unit and integration tests after the rename.
System: Test fixtures and assertions are unchanged from the pre-rename suite — the only diff in test code is the command name in the invocation harness. All tests pass green. The pre-existing extractor logic, fixture YAML, and golden output files are reused without edit, confirming behavior preservation.

---

### Greenfield Mode: Write a Schema-Valid Empty Stub When There Is Nothing to Extract

**Trigger**: A designer runs `parlay create-domain-model` against a project where the extractor returns no entities, no relationships, and no operations. This is the greenfield code path — typically a brand-new project fresh from `parlay init`, or a project whose features describe behavior in prose without yet introducing any extractable domain language.

User: Runs `parlay create-domain-model` on a project that has zero features registered (a fresh `parlay init`).
System: The extractor walks the empty `spec/intents/` tree, finds no features, returns the no-signals verdict. The command branches to greenfield, writes a schema-valid empty `domain-model.yaml` to the project root with the required top-level scaffolding (empty `entities`, `relationships`, `operations` collections plus mandatory metadata), prints `Created empty domain-model.yaml stub at {path} — ready to author.`, and exits 0. The produced file passes the domain-model schema validator without modification.

#### Branch: project has features but none have extractable signals

User: Runs `parlay create-domain-model` on a project with a handful of feature folders whose intents and dialogs are mostly prose — no `Objects:` lines that name entities, no relationship language the extractor pattern-matches.
System: The extractor walks every feature, runs every recognizer, returns zero entities/relationships/operations across the tree. Same verdict as the zero-features case: branches to greenfield, writes the empty stub, prints the same `ready to author` message, exits 0. The brownfield/greenfield boundary is exactly "did the extractor find any signal at all" — not "are there any features."

#### Branch: any single extractable signal flips the run to brownfield

User: Runs `parlay create-domain-model` on a project where exactly one feature names exactly one entity in its `Objects:` line and the rest of the tree is empty prose.
System: The extractor returns one entity (and whatever relationships/operations cascade from it). The no-signals check fails. The command branches to brownfield and writes a populated `domain-model.yaml` containing that single entity — NOT an empty stub. The produced one-line summary reads `Wrote domain-model.yaml — 1 entity, 0 relationships, 0 operations.` This branch pins the threshold: "any signal at all" is enough for brownfield. There is no fuzzy "looks mostly empty so let's stub it" middle ground.

#### Branch: existing `domain-model.yaml` is preserved when greenfield would otherwise stub

User: Runs `parlay create-domain-model` on a project that already has a `domain-model.yaml` at the project root (perhaps hand-authored in Studio earlier) but whose current intents and dialogs introduce no extractable signals.
System: The command notices the existing file before writing. It does NOT clobber the file with an empty stub. Instead it falls through to the same existing-file reconciliation rules brownfield uses — read the file, merge with the (empty) extracted result, write back the existing content unchanged. Prints a message that distinguishes this case from a fresh stub creation, e.g., `domain-model.yaml present and no extractable signals — leaving existing model untouched.` Exit 0.

#### Branch: stub validates against the domain-model schema

User: Runs `parlay validate --type domain-model domain-model.yaml` (or whichever entry point validates a domain model against its schema) immediately after a greenfield stub is produced.
System: The validator parses the stub, finds all required top-level keys present (empty collections are valid), reports no errors, exits 0. The stub is a well-formed-but-empty model, ready for Studio's Domain Model Editor or a hand edit to start filling in entities. A designer who opens the file in Studio sees a structurally valid model with zero rows in each section, not a syntax error.

#### Branch: greenfield run inside CI / non-interactive

User: Runs `parlay create-domain-model` inside a CI job on a fresh project (e.g., a smoke test of `parlay init` followed by `parlay create-domain-model`) where stdin is not a TTY.
System: Greenfield mode is interactive-agnostic. The command writes the stub, prints the same one-line `ready to author` message to stdout, and exits 0. The Studio hook (a separate feature) is what cares about TTY; greenfield itself behaves the same in CI as in a designer's terminal. Scripts and CI gates can chain off the message and exit code without special-casing for interactivity.

#### Branch: studio-cli-hooks chains off the greenfield wording

User: Runs `parlay create-domain-model` interactively with `parlay-studio` detected on `PATH` and no `--no-studio`.
System: The greenfield branch produces the empty stub and prints `Created empty domain-model.yaml stub at {path} — ready to author.` The studio-cli-hooks feature observes the greenfield outcome and follows up with its own one-line `Empty domain model created — ready to author. Open Studio's Domain Model Editor? (y/N) ` prompt. This dialog branch documents the boundary: this feature owns the stub-write and the wording; the hook prompt belongs to studio-cli-hooks. The wording chosen here is what that feature's prompt chains off of, which is why the message is single-line and stable.

#### Branch: greenfield unit-test fixture matches an empty-stub golden

User: A maintainer adds a unit test that runs the greenfield code path against a synthetic zero-features project and compares the produced YAML to a checked-in fixture.
System: The fixture is the canonical empty stub: top-level keys present, all collections empty, mandatory metadata filled in with deterministic values (or asserted shape-only if values include timestamps). The unit test passes when the produced bytes match the fixture. A second unit test exercises a synthetic project with one minimal extractable signal and asserts the produced YAML is populated rather than the empty stub — pinning the brownfield/greenfield boundary at the unit level so a future refactor cannot drift it accidentally.

---
