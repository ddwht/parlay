---
name: parlay-doctor
description: "Parlay: Diagnose and repair project state — coverage, drift, tree consistency, and pending migrations"
---

# Doctor

Inspect the project, report what is wrong or out of date, and offer the fix.
Diagnosis-first: you determine which checks apply by looking at the project,
not by asking the designer to know which of a dozen commands they need.

This replaces the previous per-operation skills — the former sync,
collect-questions and five migrate-* skills. Every underlying
CLI command still exists and can be run directly; this skill is the front door
that decides which ones matter right now.

## Arguments

- `feature` (optional): limit feature-scoped checks to one feature, in
  standard parlay form — `{feature}` or `@{initiative}/{feature}`. Omit to
  check every feature under the active root.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Steps

### 1. Survey the project

Gather state before proposing anything. None of these mutate:

- `parlay status` — active root, features, phases, orphaned build dirs
- `parlay repair --dry-run` — three-tree consistency
- `parlay validate --project` — cross-feature collisions the per-feature
  passes cannot see, including `infrastructure-concept-shared` (one line per
  architectural concept two or more features constrain). Warnings, not
  failures — report them, do not treat them as repairs.
- `parlay internal check-coverage @{feature}` — intent/dialog coverage, chain gaps, drift
- `parlay internal collect-questions @{feature}` — unresolved `Questions:` blocks
- `parlay internal check-drift @{feature}` — sources changed since the last build
- `parlay internal check-amendments @{feature}` — amendment ledger health and
  the declared dirty set

Then detect pending migrations by looking at what is on disk:

| If you find | Migration | Command |
|---|---|---|
| `spec/intents/*/surface.md` | surface.md → surface.yaml | `parlay migrate-spec` — a lingering surface.md is not optional debt: it goes stale against the amended surface.yaml and actively misleads (measured 3/3 benchmark replicates). Follow up with `parlay migrate-spec --retire-md` (refuses per-feature when the .md carries fragments the .yaml lacks), then re-stamp `parlay internal scaffold-signatures @{feature}` for each retired feature. |
| `ledger_integrity` findings on a feature with **no** `amendments/` directory | pre-v0.4 founding-doc edits → freeze at current text | `parlay migrate-ledger` (run `--dry-run` first; it prints per-feature verdicts). This is how an unmigrated pre-v0.4 project surfaces: founding-doc edits that were ordinary drift under the old regime read as integrity violations now, and the migrator dissolves exactly that state. |
| `prototype-framework:` in `.parlay/config.yaml` | legacy config → adapter-set | `parlay migrate-config` |
| operation-shaped fragments in `infrastructure.md` | infrastructure → capabilities | `parlay migrate-capabilities` |
| `domain-model.md` (not `.yaml`) | domain model → YAML | `parlay migrate-domain-model` |
| a populated `operations:` block in `domain-model.yaml` | domain operations → per-feature capabilities | `parlay migrate-domain-operations` |
| intents with **Verify** bullets whose sourcing operations/fragments lack `verify:` | verify bullets → contract artifacts | `parlay migrate-verify` (run `--dry-run` first; it prints the would-be routing) |
| a retired `.parlay/build/*/coverage-review.yaml` still holding exemptions nothing has answered | stranded coverage judgments → current decisions | **Not a migrator.** Invoke the `migrate-coverage` module. Each stranded entry is a judgment only a person can re-make, so this asks a reviewer one question at a time rather than transforming a file. `parlay internal migrate-coverage-exceptions @{feature}` reports how many are left without changing anything. |

### 2. Enhance coverage findings with semantic matching

**Run this analysis only for features not yet built.** After
first build the founding docs freeze, an intent↔dialog gap in frozen
documents is a historical fact rather than a repairable finding, and the
sync that would "fix" it is exactly the write the freeze forbids. Coverage
is a birth-time concern; skip it for frozen features and say so in the
report.

`parlay internal check-coverage` matches intents to dialogs on title and word
overlap, so it reports false gaps. Before presenting anything:

- Check whether an "uncovered" intent is in fact covered by an orphan dialog
  under a different name — e.g. intent "Configure Project Tools" and dialog
  "Bootstrap Project" are the same thing with different words.
- Present suspected matches to the designer for confirmation rather than
  asserting either the gap or the match.

### 3. Interpret drift

If drift exists, do not just report the hashes:

- Read the downstream artifacts for the drifted sources (surface, buildfile,
  testcases).
- Compare what changed against what those artifacts encode.
- Flag meaningful mismatches — a changed operation whose surface still
  reflects the old one — and distinguish them from cosmetic edits.
- Note that a changed **shared** source (`domain-model.yaml`, the adapter)
  dirties every feature that reads it, not just the one you asked about.
  `parlay internal check-drift` reports these under `shared_sources_changed`.

### 3.5 Ledger findings

`parlay internal check-drift` carries two ledger
fields and `parlay internal check-amendments` adds a third dimension. Each
finding has one right disposition:

- **`unapplied_amendments`** — the ledger records a decision whose delta
  never reached the contract artifacts. This is the highest-priority ledger
  finding: the project's recorded history and its current contract disagree.
  Route to `/parlay-refine` — its apply step is the only path that clears
  the tail. Do not apply the delta ad hoc from doctor.
- **`ledger_integrity`** — a frozen founding doc was edited, or a recorded
  amendment was mutated or deleted. First check for the one non-violation
  case: a feature with these findings and **no** amendments at all is an
  unmigrated pre-v0.4 project — route to `parlay migrate-ledger` (see the
  migrations table), not to the repairs below.

  **Second special case: an intent "removed after freeze" that is still in the
  file, inside an HTML comment.** Earlier versions parsed `## ` headings inside
  `<!-- ... -->` as real intents, so a feature built then has a baseline entry
  for a commented block that the current parser correctly ignores. The generic
  remedy below is ambiguous here, so ask which the author meant:
  - It should still be a founding promise — **uncomment the identical block**.
    The same slug and hash come back and no founding change occurred; nothing
    to amend.
  - It was meant to be inactive — that is a lifecycle decision, not a parser
    artifact. Retire it through supersession (`supersedes_intents:` in an
    amendment). Do not let it disappear as a side effect of an upgrade.

  Never restamp the baseline to make this finding go away: that would decide
  the question on the author's behalf, and the whole point of the finding is
  that the promise set changed and somebody has to say whether that was
  intended. Otherwise present the
  specific findings and offer exactly two repairs, both
  destructive-adjacent and both needing explicit confirmation:
  - **Restore** — `git checkout` the affected file(s) back to the recorded
    state. The right answer almost always; history stays intact.
  - **Bless and refreeze** — accept the edited state as the new frozen
    text by re-running `parlay internal save-build-state` for the feature
    after a full green test run. This rewrites what "frozen" means and is
    only correct when the edit was itself a sanctioned correction (e.g. a
    typo fix a human insists on keeping). Name the cost out loud before
    doing it: the ledger no longer explains this change.
- **Compaction advisor** — count the ledger. When a feature exceeds **8
  amendments**, or `supersedes:` chains touch the same `affects:` ref more
  than twice, recommend re-founding (advice only, never automatic):
  1. Generate a fresh founding intent set from the current contract
     artifacts — what the feature is NOW, stated once.
  2. Decision-gate it with the designer, then replace intents.md/dialogs.md
     with the approved text.
  3. Move the existing ledger to `amendments/archive/` (retained, never
     deleted) and start the sequence fresh.
  4. Re-run the build phases and `parlay internal save-build-state` so the
     baseline refreezes on the new founding text.

### 3.7 Coverage decisions that outlived their subject

Two shapes to look for, both reported by the code and done boundaries:

- **`downgrade-approval-stale` / `downgrade-approval-orphaned`** — somebody
  approved a test case observing a requirement weakly, and that case has since
  been rewritten, renamed, removed, or strengthened. The approval was a judgment
  about what the case observed, so it no longer stands over anything.

  Repair with `parlay internal retire-decision @{feature} --ref <ref>
  --criterion "<text>" --suite "<suite>" --case "<case>" --reason "<why it no
  longer applies>" --by "<who decided>"`. This does not delete the decision — it
  moves it to the retired list carrying both the original judgment and the
  choice to withdraw it, so the ledger still answers who decided what and when.

  To re-approve a case whose content merely drifted, retire the old decision and
  record a fresh one against the case as it now stands. Never edit the old one:
  an edit makes one review look like two.

- **Stranded legacy exemptions** — see the migration table above. Those need the
  `migrate-coverage` module, not a repair here.

Both are decisions, so both need a person. Report them and let the designer
choose; do not retire anything on their behalf.

### 4. Report, then offer

Present one consolidated picture — tree consistency, coverage, open
questions, drift, pending migrations — and only then propose actions. Order
by what blocks what: a pending migration usually blocks a clean build, and
unresolved questions block a meaningful one.

Use AskUserQuestion for anything that mutates. Never apply a migration, a
repair, or an artifact edit without explicit confirmation.

### 5. Apply what the designer approves

- **Migrations** — run the command from the table. Each is idempotent and
  supports `--dry-run`; show the dry-run before applying.
- **Repair** — `parlay repair` reconciles the three trees, prompting per
  mismatch. Renames are detected and applied as moves, preserving buildfile,
  testcases and baseline. `--dry-run` previews; `--yes` auto-confirms
  unambiguous fixes.
- **Coverage gaps** — offer to generate dialog templates for uncovered
  intents (all, or a chosen subset).
- **Criterion authority** — when a feature's criteria have never been approved,
  or have changed since they were, `parlay internal criteria-authority @{feature}`
  reports the standard and what moved. Approval belongs to the artifacts phase,
  where the mapping from each intent bullet to the criterion it became is shown;
  offer to route there rather than recording an approval from here, since
  approving a standard nobody was shown is what the retired suite-name gate did.
- **A long applied ledger** — a feature carrying many applied amendments can
  have its history compacted into `amendments/archive/`, shortening the active
  ledger without changing what the feature promises:

  ```
  parlay internal compact @{feature} --confirm
  ```

  Offer this only as hygiene, and only when the user asks for a tidier ledger.
  It is **not** a fix for anything: applied history no longer has to resolve
  against the current contract, so nothing is blocked by an uncompacted ledger.

  Without `--confirm` it reports what it would archive and stops. It moves only
  records the baseline can prove were applied — marker plus matching stored
  hash — and refuses rather than splitting a `supersedes:` edge, archiving a
  record that retires a founding intent nothing else restates, or moving the
  terminal retirement record. It captures the feature's authority projection
  before the move and rolls back if it differs after.

  Tell the user what compaction genuinely costs: archived records leave the
  ledger walk, so the amendment listing and `all_affects` shrink. Authority is
  preserved; the audit view is deliberately shortened. That trade is the whole
  operation, and it is not reversible through this command.

## Hard rules

- NEVER mutate without explicit confirmation. Every command in step 5 changes
  the project; every one of them needs a yes first.
- NEVER modify designer-authored files (`intents.md`, `dialogs.md`) without
  asking, per the project's file-ownership rules.
- NEVER apply a migration just because it is available. A project can sit on
  the legacy form deliberately.
- Report what you did NOT check. If a feature was skipped because its
  artifacts are missing, say so — a silent partial diagnosis reads as a
  clean bill of health.

## Error Handling

- `no-root` — not inside a parlay project. Suggest `parlay init`.
- `no-features` — the project has no features yet. Suggest
  `parlay add-feature <name>`.
- A migration command reporting "nothing to migrate" is a success, not a
  failure — it means that migration is already complete.
