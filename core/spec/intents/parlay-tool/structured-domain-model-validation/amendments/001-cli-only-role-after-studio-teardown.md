---
amendment: cli-only-role-after-studio-teardown
date: 2026-08-31
trigger: "the Studio-side consumer this feature declared parity with — studio/domain-model-editor/domain-model-editor-validation — no longer exists"
affects:
  - "@parlay-tool/structured-domain-model-validation/infrastructure:json-validation-mode-for-domain-model-validate"
  - "@parlay-tool/structured-domain-model-validation/infrastructure:machine-usable-element-path-on-every-finding"
  - "@parlay-tool/structured-domain-model-validation/infrastructure:emit-domain-operations-deprecated-in-authoring-mode"
---

## Change

This feature is no longer half of a parity contract. It is a CLI contract,
whole in itself.

Every behavior it delivered stands exactly as specified: `parlay validate
--type domain-model --json` emits one finding per violation rather than a
collapsed aggregate, accepts `-` to read a model from stdin, and exits 0 under
`--json` whether or not findings are present; every finding carries a
machine-usable element path from a closed, versioned dotted grammar, with a
distinguished whole-model token for findings that own no element; and a
populated `operations:` block raises `domain-operations-deprecated` at the
severity the per-mode table already declares.

What changes is who those behaviors are for and what justifies them. The three
fragments named a single out-of-process consumer — Studio's domain-model editor
— as the reason each contract had to hold: the stdin path because the editor's
draft lived only in memory, the path grammar because the editor's inline
markers resolved it to form controls and diagram nodes, the deprecated warning
because the editor prompted the designer to migrate. That consumer is gone.
The contracts are not restated by this record and are not weakened by it; they
now rest on their own terms, as the CLI's structured-output contract for any
caller — a script, a CI job, a `--json` consumer that does not exist yet, and
the in-process validation seam the domain document API will call.

## Why

The parity framing was accurate when written and is false now, and a spec that
justifies a live contract by naming a dead consumer is worse than one that
justifies nothing: the next reader who checks the citation finds nothing there
and has to guess whether the contract survived the consumer. Two of these
contracts are easy to mistake for editor-shaped scaffolding — reading a model
from stdin and anchoring findings to element paths both read as concessions to
a GUI. They are not. A structured finding that cannot say *where* is unusable
to any programmatic caller, and validating bytes that are not yet a file is
what every gate-before-write path needs. The behaviors were always more
general than the consumer that motivated them; only the justification was
narrow.

Recasting rather than retiring is the honest shape. Retirement is for work
that stopped mattering or moved elsewhere, and this work did neither — it is
running, it is called, and the domain document API's validation seam is built
on the same engine. Nothing here needs to change for the teardown to be
correct. What needed to change is the reason on the record, so the contract
outlives the citation.

This record settles the framing only. It adds no behavior, removes none, and
changes no output shape or exit code; a caller written against these contracts
before the teardown behaves identically after it.
