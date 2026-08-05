---
name: parlay-refine
description: "Parlay: Make a small, precise change to an existing feature — spec, code, tests and baselines together"
---

# Refine

Make one small change to a feature that already exists, and leave the spec, the
code, the tests and the baselines agreeing with each other afterwards.

**What this is for.** Once an app exists, most work is small and precise: move
the filter above the table, make the approval step notify the requester, widen
a timeout. The pipeline was built for the other case — a feature from nothing —
and running the whole thing for a two-line change is absurd, so people don't.
They prompt an agent directly instead, the change lands in code, and parlay
learns nothing about it. Every later drift check then compares generated output
against a spec that no longer describes the system, and the divergence is
invisible because nothing recorded that it happened.

This is the tracked replacement for that. **Same prompt, same change** — the
difference is that the spec learns it too.

There are four ways to detect divergence in this toolkit and, before this,
nothing that resolves one. That asymmetry is the problem being fixed.

## Arguments

- `<prose>` (required): the change, in your own words. One argument, one
  change. "Move the status filter above the table." "The approval step should
  notify the requester too."
- `feature` (optional): `{feature}` or `@{initiative}/{feature}`. Omitted, it
  is resolved from the prose and the project; ambiguity is raised, not guessed.

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

## The one invariant: amend first

**Amend the spec artifact before regenerating anything.** Not after, not
alongside.

This is not a stylistic preference, it is what makes the rest of the chain
work unchanged. `parlay internal diff @{feature}` compares current sources
against the recorded baseline and reports which components are dirty. If the
artifact is amended first, `diff` already reports exactly the components the
change affects — the regeneration scope falls out for free, with **no change
to `diff` at all**.

Regenerate-first has no vocabulary anywhere in this codebase. There is no way
to ask "what would this change affect" before it is written down, because
every scoping mechanism in the toolkit is a comparison against a baseline.

## Steps

1. **Resolve the feature** — If `feature` was given, use it. Otherwise infer it
   from the prose and the project's features. If two features could plausibly
   own the change, raise an `ambiguity` decision listing them; do not pick the
   likelier one.

2. **Locate the owning artifact** — Which artifact does this change belong to?

   Read the candidates and decide. `surface.yaml` fragments carry `source:`,
   `capabilities.yaml` operations carry `source:`, `infrastructure.md`
   fragments carry `**Source**:` — those references are the map back from a
   change to the thing that owns it.

   **This is a judgment call, and it must stay one.** Do not match on title
   similarity. Lexical overlap both misses renames — the fragment was called
   something else when it was written — and blesses contradictions, matching a
   fragment that merely sounds related while the real owner goes untouched. A
   matcher here is worse than no automation, because it is confidently wrong.

   Raise an `ambiguity` decision when: no artifact clearly owns the change; two
   do; or the change contradicts what the owning artifact currently says.
   Contradiction especially — the user may be correcting the spec deliberately,
   or may have forgotten what it says, and those want opposite handling.

   Report which artifact you chose and why, before amending it.

3. **Classify the altitude** — Two destinations:
   - **User-visible** → `intents.md`, `dialogs.md`, or `surface.yaml`. What
     someone sees, does, or is told.
   - **Implementation-shaped** → `infrastructure.md`. Boundaries, probes,
     allowlists, dependency pins, timeouts — architectural constraints that do
     not reduce to operations. See that schema's promotion section.

   Backend behaviour that *is* operation-shaped belongs in `capabilities.yaml`
   as an amendment to the operation its `source:` names.

   Everything is promoted to the spec. A change that lives only in code is the
   untracked path this command exists to replace.

4. **Amend — splice, never re-encode** — Change the span that needs changing
   and leave every other byte alone.

   The only amend semantics in this codebase say it exactly:
   *preserve hand-authored entries, replace extractor-owned spans*. And
   `scaffold-signatures` splices its block by line rather than re-encoding the
   buildfile, because round-tripping a 700-line reviewed document through a
   YAML encoder preserves every value and destroys the folded descriptions, the
   grouping blank lines, and the comments explaining why a field is the way it
   is. A refinement that reformats the artifact it touches makes the next
   review diff unreadable, which costs more than the refinement was worth.

   **Decision-gate the amendment.** Show the exact before/after span and get
   agreement before writing. These are designer-authored documents; editing one
   beyond what was asked for is the thing the designer brief already forbids.

5. **Scope** — Run `parlay internal diff @{feature}`. Because step 4 already
   landed, this reports the right component subset with no special handling.

6. **Regenerate** — Preserve stable, regenerate dirty, exactly as `generate-code`
   does. Append every file written to `.parlay/build/_project/.emitted`, one
   path per line, as you write it. The manifest is what makes step 9 a scoped
   re-baseline rather than a project-wide one.

7. **Refresh signatures** — `parlay internal scaffold-signatures @{feature}`.
   The amended artifact's hash moved; the buildfile's `source-signatures:` has
   to move with it or the freshness gate fires on the next run.

8. **Run the full test suite** — Not the affected suites. The whole thing.

   This is a deliberate cost. Re-baselining records "this output is blessed",
   and blessing untested code is the one thing the build state must never do.
   A refinement is small by definition, which is exactly when it is tempting to
   check only what you think it touched — and exactly when a missed interaction
   is most likely, because nobody is looking. If the latency becomes the reason
   people stop using this, affected-suites-only is the escape valve; take it
   deliberately, not by drifting into it.

   Tests failing stops the refinement. Raise a `failure` decision with the
   failures in `context:`. Do not re-baseline.

9. **Re-baseline** — `parlay internal save-build-state --source-root {root} --partial --emitted .parlay/build/_project/.emitted`

   `--partial` is required and it makes `--emitted` mandatory. A partial run
   with no manifest does not degrade, it is wrong: it would mark every tracked
   file in the project unknown on the strength of a run that touched three, and
   `--strict` would then fail on all of them. With the manifest, files this run
   did not touch keep the verdict they already had.

10. **Re-review coverage** — `parlay review-coverage @{feature}`.

    Not optional, and not tidiness. The refinement changed the buildfile, which
    invalidates the hashes `coverage-review.yaml` pins, so the review gate exits
    non-zero on the *next* codegen run — after this command has reported
    success and everyone has moved on. Chaining it here is what keeps a refined
    project in a state the next command can actually work from.

    Use `--exempt <suite>:<item>=<reason>` for terms that legitimately have no
    covering case.

11. **Report** — What changed, in this order: the artifact and the span amended;
    the components regenerated; the test result; the baseline and coverage
    review refreshed. Then the sentence that matters: `check-drift` is clean,
    because the spec and the code agree again.

## What this does not do

- **Not a second codegen path.** Steps 6 and 7 are `generate-code`'s behaviour,
  invoked for a smaller scope. Divergence between them is a bug in this skill.
- **Not for new features.** A change with no owning artifact is a new intent —
  `/parlay-loop` owns that. Refine amends what exists.
- **Not a way into a hand-authored unit.** A unit's code is written by a person
  and fenced off from codegen; a prose request to change one is a request to a
  person, not to this command. Refuse and say so.
- **Not a bulk editor.** One prose argument, one change. Several changes at once
  make the decision gates unanswerable, because the user is agreeing to a batch
  rather than to an edit.
