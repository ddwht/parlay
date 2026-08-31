---
amendment: nothing-remains-to-hook-into
date: 2026-08-31
trigger: "user ruling of 2026-08-31 — retire everything Studio-related; the studio root was retired earlier the same day, the studio-support initiative dissolved with it, and 001 deferred exactly this decision"
supersedes_intents:
  - detect-parlay-studio-at-runtime
  - prompt-to-open-studio-at-workflow-hand-off-points
  - deferred-artifacts-review-and-reconcile
retires_feature: true
outcome: obsolete
---

## Change

This feature is closed. All three founding promises stop being in force, and
nothing takes them over.

`detect-parlay-studio-at-runtime` goes first, because everything else rested on
it: no command looks up `parlay-studio` on `PATH`, none reads `PARLAY_STUDIO`
(set, unset, or deliberately emptied), none distinguishes a non-executable file
from an absent one, none compares a detected version against an expected range,
and `parlay status` says nothing about Studio on any machine.

`prompt-to-open-studio-at-workflow-hand-off-points` was already emptied by 001
and clarified by 002 — the flag family, the three-gate chain, the dispatch and
the subprocess lifecycle are gone. What 001 left standing was the promise
itself, that Core offers a hand-off at the trio's completion points. That
promise ends here. There are no hook points.

`deferred-artifacts-review-and-reconcile` recorded two prompts as wanted-but-
unbuilt, on the reasoning that the surfaces they would open were still wanted.
They are not: the program that would present them is retired. The record stops
being a deferral and becomes history.

Neither 001 nor 002 used `supersedes_intents:`, so all three intents were still
live when this record was written — including the hook-point promise, whose
machinery 001 removed without retiring the promise behind it. This record
retires exactly the set the parser reports as live, which is all three.

The outcome is `obsolete`, not `replaced`. No feature inherits this work,
because the work was hooks into a Studio, and there is no Studio. A reader
looking for where Studio detection went should find nothing, because nothing is
where it went.

The feature keeps its name. `studio-cli-hooks` is the honest record of what it
was, and it now sits at `parlay-tool/studio-cli-hooks` because the
`studio-support` initiative that held it was dissolved the same day — six
healthy siblings moved to `parlay-tool/` and this one moved with them. The
group is gone; the name is history and stays.

This record closes the feature and, like every amendment, removes nothing. The
contract artifacts (`surface.yaml`, `infrastructure.md`) and the build records
(`buildfile.yaml`, `testcases.yaml`) are still on disk, and parlay's retirement
check refuses to apply a terminal record over them — deliberately, per
`@parlay-tool/feature-retirement`, which defers the disposition machinery until
a feature with output is worth retiring. This is that feature. Disposing of
those artifacts, and of the unapplied tail 001 and 002 left behind, is separate
work this record does not do and does not authorize by implication.

## Why

Every promise still standing here has zero implementing code, and had none
before this record was written.

The detection promise died with its subject. `parlay-studio` was a separate
binary that was never released; Core's detection of it was built, then removed
when the in-process editor replaced it, and the editor was deleted in turn. The
constructor that carried the detection record onto every command context —
`NewContextWithStudioDetection` — is gone, and `internal/config/context.go`
records why in prose: what it actually did was make every parlay invocation
shell out to a program that does not exist. `PARLAY_STUDIO` appears in no Go
file. No `StudioDetection` type exists. `parlay status` has no Studio branch to
suppress. A promise that no code has kept for two removals is not a commitment;
it is a sentence in a file.

Keeping it costs more than deleting it. A spec that still says `parlay status`
reports Studio detection is a spec a reader can act on — by implementing it, by
testing for it, or by trusting it in a design that assumes Studio is reachable.
The version-mismatch warning is the sharpest case: it promises a diagnostic
about a version range for a binary with no versions.

001 said this decision was owed and deliberately did not take it: it settled the
flag family and the dispatch the flag gated, and recorded that Studio detection,
the `parlay status` line and the `PARLAY_STUDIO` override "still stands as
written until a decision that names it retires or re-homes it". This is that
decision, and it retires rather than re-homes, because there is no home. The
user's 2026-08-31 ruling to retire everything Studio-related, following the
retirement of the whole `studio` root the same day, is what settles it.

One piece of this feature's machinery outlived it, and was re-homed before this
record rather than by it. `ttyInteractive` — the TTY gate the hook prompts
consulted — now lives in `internal/commands/tty.go` as a cross-cutting helper,
because `lock-page` and `migrate-domain-operations` gate their own interactive
prompts on it and never depended on Studio. That is a helper surviving its first
caller, not a promise surviving its feature: no intent here is kept alive by it,
and the file names no feature.

Nothing else in the tree claims this feature. No Go file carries a
`parlay-feature:` or `parlay-extends:` marker naming it; the two that did —
`sync.go` and `create_artifacts.go` — were re-homed to their honest owners
before this record was written, on the rule that a retirement record may not own
live code.

## Acceptance

- No parlay command looks up `parlay-studio` on `PATH` or reads `PARLAY_STUDIO`;
  setting either has no observable effect on any command.
- `parlay status` prints nothing about Studio, on a machine with a file named
  `parlay-studio` on `PATH` and on one without.
- No command emits a Studio version-mismatch warning, on stderr or anywhere
  else.
- No file in the source tree carries a `parlay-feature:` or `parlay-extends:`
  marker naming this feature, at either its historical `studio-support/`
  address or its current `parlay-tool/` one.
- Once this record is applied, `parlay internal active-spec
  @parlay-tool/studio-cli-hooks` reports no active promise and every founding
  intent as superseded.
