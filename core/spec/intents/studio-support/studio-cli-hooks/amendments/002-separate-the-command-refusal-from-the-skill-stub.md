---
amendment: separate-the-command-refusal-from-the-skill-stub
date: 2026-08-31
trigger: "post-merge review found amendment 001 conflating the agent skill with the shell command: it said `parlay create-domain-model` prints the greenfield stub and exits, but the shell command refuses through the agent-only stub (stderr, exit 2) and writes nothing — the stub line belongs to the skill/loop path"
supersedes:
  - no-editor-flag-family-removed
affects:
  - "@studio-support/studio-cli-hooks/infrastructure:hook-point-dispatch-at-trio-command-completion"
---

## Change

Amendment 001's decision — the --no-editor flag family and the trio dispatch
are gone — stands unchanged. Its description of what replaced the prompt is
corrected: it attributed the greenfield stub line to the shell command.

The truth has two halves. `parlay create-domain-model` invoked as a SHELL
command is agent-only: it prints a two-line refusal to stderr and exits 2,
writing nothing — that is the whole of its direct behavior. The greenfield
stub (the pinned one-line message and the stub `domain-model.yaml`) is
produced by the create-domain-model SKILL running inside an AI agent, which
is the path the loop drives. 001's acceptance line "parlay create-domain-model
in greenfield mode still prints its pinned one-line stub message and exits 0"
described the skill's outcome as if it were the command's.

## Why

The command/skill split is this project's load-bearing distinction — the CLI
refuses what only an agent can do, precisely so nobody mistakes a refusal for
the work — and an amendment blurring it re-creates the confusion the
agent-only stubs exist to prevent. A reader acting on 001's acceptance would
run the shell command, get exit 2, and conclude the teardown broke it.

## Acceptance

- `parlay create-domain-model` run as a shell command prints the agent-only
  refusal on stderr and exits 2, writing nothing — with or without a TTY.
- The create-domain-model skill, driven by an agent (directly or via the
  loop), still produces the greenfield stub and its pinned one-line message;
  that behavior is owned by `parlay-tool/create-domain-model`, not by this
  feature.
- Neither path emits any editor prompt or reads any --no-editor spelling, as
  001 established.
