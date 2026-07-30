# add-feature

_Create a new feature_

# Add Feature

Create a new feature folder with intents.md and dialogs.md. Optionally place it inside an initiative.

## Arguments

- `name`: The feature name (e.g., `upgrade plan creation`)
- `initiative` (optional): The initiative to create the feature inside (e.g., `auth overhaul`). Auto-creates the initiative if it doesn't exist.

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
kind: phase-boundary        # phase-boundary | override | overwrite | failure | ambiguity
phase: <the phase you are in>
question: "<the one question, in the user's terms>"
context: |
  <what you found, and what is already on disk>
options:
  - id: <slug>
    label: "<what the user picks>"
    detail: "<the consequence, when it isn't obvious>"
resume: "Re-enter with decision: <id>. <what is written so far>"
```
````

Leave the filesystem coherent before you stop — a decision is a pause, not a half-write. If you genuinely cannot pause at that point, take the option that preserves the user's work, never the one that destroys it, and say so in your report.

Two things not to do: never narrow the options to spare the user a question, and never resolve an ambiguity by taking the reading that is cheapest to implement. Both turn a decision the user should own into one you made quietly.

## Steps

1. If the user specified an initiative: run `parlay add-feature {name} --initiative {initiative}`
2. Otherwise: run `parlay add-feature {name}`
3. Tell the user to start authoring intents.md.
