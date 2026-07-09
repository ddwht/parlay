---
name: parlay-collect-questions
description: "Parlay: Collect open questions from intents"
---

# Collect Open Questions

Scan intents for unresolved design questions. Use this as a quality gate before running build-feature.

## Arguments

- `feature` (optional): The feature slug. If omitted, scans all features.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. **Collect questions** — Run: `parlay collect-questions @{feature}` (or `parlay collect-questions` for all features)

2. **Present results** — Show the user:
   - Total open question count
   - Questions grouped by intent, with priority shown
   - If count is 0: confirm the feature is ready for build

3. **If questions exist** — Ask the user:
   - A: Resolve them now (walk through each question)
   - B: Proceed anyway (acknowledge the gaps)
   - C: Skip — just the report

4. **If resolving** — For each question:
   - Present the question in context (show the intent's Goal and Constraints)
   - Ask for the designer's answer
   - Offer to update intents.md: remove the resolved question and add any new Constraints or Verify bullets based on the answer
