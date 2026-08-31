---
amendment: no-editor-flag-family-removed
date: 2026-08-31
trigger: "parlay becomes backend-only: the domain-model editor the trio hooks opened is deleted, so the flag that silenced them has nothing left to silence"
affects:
  - "@studio-support/studio-cli-hooks/surface:no-studio-flag-opt-out"
  - "@studio-support/studio-cli-hooks/infrastructure:no-studio-flag-and-config-plumbing"
  - "@studio-support/studio-cli-hooks/infrastructure:hook-point-dispatch-at-trio-command-completion"
  - "@studio-support/studio-cli-hooks/infrastructure:studio-subprocess-lifecycle"
---

## Change

The opt-out flag family is gone, in every spelling it ever had: `--no-editor`
on the trio commands, its pre-rename `--no-studio` predecessor, and both
project-config keys (`parlay.no_editor`, `parlay.no_studio`). No trio command
accepts the flag, no config key is read, and the registration helper that
attached it to `create-domain-model`, `create-artifacts` and `sync` is deleted.

The hook-point dispatch it gated goes with it. The trio commands no longer
consult a three-gate chain (TTY, detection, opt-out) at completion, no longer
emit an "Open Studio?" line, and no longer hand a workflow context to a
subprocess. The subprocess lifecycle — invoke synchronously, wait, propagate
the exit code, never roll back the trio command's prior work — is unreachable
and removed with them.

What replaces the prompt at the one hook point that had a working surface is
not another prompt. `parlay create-domain-model` prints its greenfield stub
line and exits; the designer edits `domain-model.yaml` by hand and checks the
edit with `parlay validate --type domain-model <path>`. The loop's artifacts
boundary makes the same offer as a review pause rather than as a launch.

## Why

The flag existed for one reason: to silence a prompt. Deleting the editor —
`internal/editor/{ui,server,config}`, `parlay domain-edit`, `parlay internal
serve` — deletes the prompt, and a flag whose only effect was suppressing
something that no longer happens is a promise that something still reads it.
Nothing does. Keeping it would leave three commands advertising a knob wired
to dead code, which is the same defect the 0.2.0 record in this feature's
intents already called out for `artifacts-review` and `reconcile`: an offer
with nothing behind it has none of the offer's value and all of its cost.

Removing the flag and leaving the dispatch would be worse than removing
neither. The flag is one of the dispatch's three gates; a dispatch that can no
longer be silenced, firing at a binary that is not coming back, is a louder
failure than the one being fixed. They are one decision, recorded once.

This record is deliberately narrow. It settles the flag family and the trio
dispatch the flag gated. It does not settle what remains of this feature's
Studio-detection promise — `parlay status` reporting detection, the
version-mismatch warning, the `PARLAY_STUDIO` override — which still stands as
written until a decision that names it retires or re-homes it. Recording that
boundary is the point: a teardown that silently widens its own scope is how a
ledger stops being a record of decisions.

One consumer outside this feature was resting on it. `create-domain-model`'s
greenfield-stub line was pinned stable "for `studio-cli-hooks` to
pattern-match", and `build-feature` and `generate-code` both cited that reason
for the exception to their own no-pattern-matching-stdout rule. With the hook
gone the wording is still pinned, on its own terms — `parlay-tool/create-domain-model`'s
own testcases assert it verbatim — so the exception survives with an honest
justification instead of a dangling one.

## Acceptance

- No trio command (`parlay create-domain-model`, `parlay create-artifacts`,
  `parlay sync`) accepts `--no-editor` or `--no-studio`; passing either is an
  unknown-flag error.
- `parlay.no_editor` and `parlay.no_studio` in project config are read by
  nothing and affect no command's behavior.
- Completing any trio command emits no "Open Studio?" prompt, on a TTY or off
  one, and starts no subprocess.
- `parlay create-domain-model` in greenfield mode still prints its pinned
  one-line stub message and exits 0, unchanged by the hook's removal.
