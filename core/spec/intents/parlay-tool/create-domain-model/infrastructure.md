# Create-domain-model — Infrastructure

---

## CLI Command Rename and Hard Cutover

**Affects**: cli command registration, command dispatch
**Behavior**: The CLI registers a single command named `create-domain-model` for producing a domain model. The previously registered name `extract-domain-model` is removed entirely — there is no alias, no deprecation shim, no fallback. Invocations of the old name reach the dispatcher's standard "unknown command" handler with no special-case message. The new command's help text describes both the brownfield (extract from signals) and greenfield (write empty stub) modes against a single entry point.
**Invariants**:
- The command list exposed by `parlay --help` contains `create-domain-model`
- The command list exposed by `parlay --help` does not contain `extract-domain-model`
- Invoking `parlay extract-domain-model` exits non-zero with the dispatcher's standard unknown-command error
- Brownfield runs of `create-domain-model` produce byte-equivalent output to what `extract-domain-model` produced before the rename, given the same project fixture
- The unknown-command path does not introduce a custom "did you mean…" hint specific to this rename — the dispatcher's standard suggester handles it generically
**Source**: @parlay-tool/create-domain-model/rename-and-preserve-brownfield-behavior
**Backward-Compatible**: no

**Notes**:
- Hard cutover is an explicit design choice, not a default. Rationale: the project rule favors changing code over carrying compatibility shims, and every in-repo caller is updated as part of this feature's rollout sweep.
- The rename does not modify any extractor logic. The brownfield code path consumes the same inputs (intents and dialogs across all features), runs the same recognition pass, and writes to the same output path with the same schema.
- This fragment owns the dispatcher-level rename and the unknown-command behavior. It does not own the embedded-skills rename or the deployer-list updates — those are separate fragments below because they live in different cross-cutting layers.

---

## Embedded Skill Rename Across Deployers

**Affects**: embedded skills, deployer registry, agent-surface materialization
**Behavior**: The embedded skill source file that backs this command is renamed from the old `parlay-extract-domain-model` slug to `parlay-create-domain-model`. The deployer registry — which materializes embedded skills into agent-specific output for Claude Code, Cursor, and the generic deployer — picks up the new slug from the embedded skills list and writes the corresponding agent-surface artifact under the new name. Stale agent-surface artifacts under the old slug are removed during the same materialization pass so a project that runs `parlay upgrade` after this feature ships ends up with exactly one skill file for this command, under the new name.
**Invariants**:
- After `parlay upgrade` completes, the project's agent surface contains a skill artifact under the new slug for every deployer that was previously emitting one under the old slug
- After `parlay upgrade` completes, no agent-surface artifact under the old slug remains in the project's agent surface
- The embedded skills list (the compiled-in source the deployer registry enumerates) contains the new slug and not the old slug
- The skill-titles map (shared by the Claude and Cursor deployers) maps the new slug to the human-readable title for this command and contains no entry for the old slug
- The generic deployer's hardcoded CLI command list contains `create-domain-model` and not `extract-domain-model`
- All three deployers — Claude Code, Cursor, generic — emit consistent output: the same set of skills, named with the same slug, materialized into their respective agent-surface formats
**Source**: @parlay-tool/create-domain-model/rename-and-preserve-brownfield-behavior
**Backward-Compatible**: no

**Notes**:
- The deployer architecture (per the project blueprint) shares the embedded skills list and the skill-titles map across all three deployers. A single rename in those shared sources cascades to all three agent surfaces; this fragment relies on that cascade rather than enumerating per-deployer renames.
- Removal of the stale agent-surface artifact under the old slug is part of the materialization pass, not a separate one-shot migration. The intent is that any future `parlay upgrade` against any project always produces a clean agent surface under the new slug regardless of what slug it had before.
- The generic deployer's hardcoded CLI command list is the one place the deployer registry's "shared embedded skills" cascade does NOT cover — it is a per-deployer artifact that lists CLI commands (not skills) and must be updated alongside the embedded skills list. The blueprint flags this explicitly as a cross-cutting rule for "adding/removing a CLI command", and this fragment treats both lists as a single rename surface that must agree.
- Validation: a unit or integration test verifies that the three deployers, when run against a fixture project, emit a consistent set of skills with the new slug and no stale artifact under the old slug.

---

## Greenfield Branch in the Domain-Model Write Path

**Affects**: domain model write path, no-signals branch
**Behavior**: When the domain-model command runs and the signal-recognition pass returns zero entities, zero relationships, and zero operations across all features, the write path branches to greenfield. In greenfield, it writes a schema-valid empty domain-model artifact to the project root containing the required top-level scaffolding (empty entities, empty relationships, empty operations, mandatory metadata) and prints a one-line status message that distinguishes greenfield from brownfield output (e.g., a "ready to author" message naming the produced path). When at least one signal is recognized, the write path remains on the brownfield branch and produces the populated artifact as before. The branch decision is driven entirely by the recognition pass's verdict — there is no flag, no env var, and no separate detector. Brownfield and greenfield share a single source of truth for "did extraction find anything."
**Invariants**:
- Recognizing zero signals across the entire feature tree triggers the greenfield branch
- Recognizing one or more signals anywhere in the feature tree triggers the brownfield branch
- The greenfield artifact is schema-valid against the domain-model schema (passes the validator without modification)
- The greenfield artifact contains the required top-level keys with empty collections, not missing keys
- The greenfield branch and the brownfield branch consume the same recognition-pass output — there is exactly one place that decides "are there any signals"
- The greenfield branch's stdout message is a single line and is stable enough that another command can pattern-match on it
- The greenfield branch returns exit code 0
- The greenfield branch behaves identically in interactive and non-interactive (CI, piped) contexts — TTY state does not change its output or exit code
**Source**: @parlay-tool/create-domain-model/greenfield-mode-write-empty-stub
**Backward-Compatible**: no

**Notes**:
- "Backward-compatible: no" is correct here because the previous command in greenfield conditions either produced an unhelpful empty file or errored — projects that scripted around that behavior may need to update. This is an intentional behavior change, the substantive payoff of the rename.
- The schema-validity invariant is what makes the stub usable by Studio's Domain Model Editor (a separate feature) — opening it produces a structurally valid model with zero rows, not a syntax error. The stub wording "ready to author" is also what the studio-cli-hooks feature's greenfield prompt chains off of.
- The greenfield branch is interactive-agnostic. The Studio hook is the layer that cares about TTY; this fragment's behavior is the same in CI as in a designer's terminal.

---

## Existing-File Preservation Under No-Signals Conditions

**Affects**: domain model write path, existing-file reconciliation
**Behavior**: If a domain-model artifact already exists at the project root when the command runs and the recognition pass returns zero signals, the command does not overwrite the existing artifact with a greenfield stub. It falls through to the same existing-file reconciliation rules brownfield uses — read the file, merge the (empty) extracted result, write the existing content back unchanged. The user-visible message distinguishes this case from a fresh stub creation. Greenfield stub-write applies only when the recognition pass returns zero signals AND no domain-model artifact is already on disk.
**Invariants**:
- An existing domain-model artifact is preserved byte-for-byte when the recognition pass returns zero signals
- The greenfield-stub branch fires only when both conditions hold: zero signals AND no existing artifact at the target path
- The user-visible message in the existing-file-preserved case differs from the fresh-stub case so a downstream consumer can distinguish them
- Existing-file reconciliation applies the same merge rules brownfield uses — there is no separate code path for "no signals + existing file"
**Source**: @parlay-tool/create-domain-model/greenfield-mode-write-empty-stub
**Backward-Compatible**: yes

**Notes**:
- This fragment exists to pin a safety property: a designer who hand-authored a domain model in Studio and then runs the command must never have their work clobbered by the new greenfield branch. The rule is "greenfield is for empty projects, not for projects that happen to have prose-only intents."
- "Backward-compatible: yes" applies here specifically: the prior behavior also did not clobber existing files (the brownfield merge path handled this), and this fragment preserves that.
- The preservation rule is the simplest possible — same existing-file reconciliation as brownfield. A more complicated rule (e.g., "warn the designer that no signals were found") is deferred until real-workflow evidence shows it's needed.

---
