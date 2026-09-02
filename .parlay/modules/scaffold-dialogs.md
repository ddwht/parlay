# scaffold-dialogs

_Scaffold dialog templates from intents_

# Scaffold Dialogs

Generate complete dialogs from authored intents, and update existing dialogs when intents change.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Asking the user

This skill runs as a **phase module** — normally inside a parlay-loop subagent, where no interactive tool exists. A question asked there is written into a transcript nobody reads, and you then answer it yourself; that is not a confirmation, it is a decision made on the user's behalf. So do not prompt. **Stop and return a decision request** as your final output. The driver prompts and resumes you with the chosen `id`, with your context intact, so you continue exactly where you stopped.

````
```yaml parlay-decision
kind: phase-boundary        # phase-boundary | override | overwrite | failure | ambiguity | impasse
phase: <the phase you are in>
question: "<the one question, in the user's terms>"
context: |
  <what you found, and what is already on disk>
options:
  - id: <slug>
    label: "<what the user picks>"
    detail: "<the consequence, when it isn't obvious>"
default: <id>               # advancement kinds ONLY — see below
resume: "Re-enter with decision: <id>. <what is written so far>"
```
````

**The `default:` field.** It names the one option id a driver running `--non-interactive` may take without asking. It exists so an unattended run has a defined answer rather than an inferred one, and it must be an id from your own `options:` list.

Only the two advancement kinds may carry a default: `phase-boundary` (normally `proceed`) and `override` (your recommended set). Those are decisions where one answer is the recommendation and the others are the user electing to intervene — taking the recommendation unattended is what the user asked for by passing the flag.

The other four kinds must NOT carry one, and a driver must abort rather than invent one, because on each of them every available answer is wrong in a way the user would want to know about:

- `ambiguity` — the protocol already forbids resolving one by taking the cheapest reading. A flag must not become the exception that makes it allowed.
- `overwrite` — one answer destroys work that may have been hand-edited; the other ships a prototype that diverges from its spec. There is no safe default, only a choice about which loss is acceptable.
- `failure` — the safe-looking answer proceeds past a suite that did not pass, which is the one outcome a CI run exists to prevent.
- `impasse` — the pipeline cannot express what the spec asks for, and the offered way forward hands the work to a person permanently. Accepting that is a scope reduction nobody can consent to on the user's behalf.

So: when you raise one of those four, omit `default:`. Adding one does not make the run smoother; it makes an unattended run take an action nobody authorized.

**`impasse` vs `ambiguity`.** An ambiguity has two readings and you cannot pick between them; an impasse has none — the pipeline has no way to express what the spec asks for, whichever reading you take. They are separate kinds because their resolutions differ in kind: an ambiguity is settled by the user choosing a reading, an impasse by the user agreeing that this part of the system will be written by hand, declared as a unit, and never generated. Filing an impasse as an ambiguity offers the user a choice between readings that all fail.

Leave the filesystem coherent before you stop — a decision is a pause, not a half-write. If you genuinely cannot pause at that point, take the option that preserves the user's work, never the one that destroys it, and say so in your report.

Two things not to do: never narrow the options to spare the user a question, and never resolve an ambiguity by taking the reading that is cheapest to implement. Both turn a decision the user should own into one you made quietly.

## Read the backlog for this feature — ON ENTRY, and only when invoked standalone

```
parlay backlog list --related @{feature} --open --json
```

**Before any phase work below.** This section sits above the procedure because
that is when it runs — the contract is that the USER decides whether a hit
enters scope, and a read performed after you have already written has nothing
left to decide about. It said so from the bottom of the file once, which
asserted an order it did not have.

**Only when you were invoked on your own.** Inside a chained designer run the
phase-group has already done this once for the whole group, and repeating it
per phase is the cost the scoped read-set exists to remove.

Report what it returns — titles and ids — and let the user decide whether any
of it enters scope. Never fold a backlog item into the work on your own
initiative: cheap capture only stays safe if capture cannot silently become
commitment. Read `counts` for this listing, not `project_totals`, which
describes the whole project regardless of the filter.

A failed read must never fail the phase.

## Steps

1. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md` (may not exist yet)

2. **Check for ambiguities** — Before generating, scan each intent for ambiguities that would affect the dialog (unclear Goal, contradictory Constraints, missing Context for branching decisions). If found, raise an `ambiguity` decision request BEFORE generating anything (see **Asking the user**). Do not generate dialogs with gaps, and do not resolve an ambiguity by picking the reading that is easiest to write a dialog for.

3. **Generate dialogs for uncovered intents** — For each intent that has no matching dialog:
   - Read the intent's Goal, Context, Action, Constraints, Verify, and Objects
   - Generate a complete dialog flow: trigger, happy path with user/system turns, branches for each Constraint that implies user-visible behavior, branches for each Verify item that describes an edge case
   - The generated dialogs should be complete enough that the designer can review and approve with minor edits — not empty templates requiring rewriting
   - Run `parlay create-dialogs @{feature}` for the mechanical scaffolding, then enrich each template with full content

4. **Never re-sync existing dialogs** — dialogs freeze at first build as
   founding documents. Intents cannot have changed underneath them
   (check-drift enforces the freeze), and post-birth change is recorded in
   the amendment ledger instead of re-synced into the transcript. This
   skill's job is step 3 only, at birth. If a dialog looks out of date with
   the current behavior, that is the ledger working as designed — the
   transcript records the founding conversation, and the contract artifacts
   carry current truth.

5. **Report** — Summarize: how many dialogs were generated. If every intent is covered: "Dialogs are up to date — no updates needed."

## Capturing what you notice and do not do

Mid-phase you will find things: a defect, a gap the spec never covered, a
shortcut you took knowingly. Record them instead of mentioning them.

```
parlay note --kind gap --title "..." --by <you> [--feature @{feature}] [--phase {phase}] \
            [--evidence path:line] [--about @{feature}/operation:x]
```

**What to capture.** A concrete, evidenced piece of undone work, or an explicit
later/defer/out-of-scope statement from the user. Never speculation, never a
generic suggestion, never work already recorded. Every phase boundary reports
the ids captured during it, so noise is visible rather than silent — an
over-eager capture is caught at the next confirmation, not three weeks later.

**Never guess a priority.** Pass `--priority` only when a person actually ranked
it. Absent means untriaged, which is a fact about the record; a guessed rank
looks like a judgment and is not.

**A failed capture must never fail your phase.** The writer is strict — malformed
input is refused rather than written as a corrupt record — but that refusal is
yours to absorb. Report it in `notes:` and carry on.

**Note vs. `decisions:`.** If you wrote it into a file, it is a decision. If you
walked past it, it is a note.
