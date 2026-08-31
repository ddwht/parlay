# Governance exception record: the studio-cli-hooks closure (2026-08-31)

Two tooling gaps surfaced while retiring `parlay-tool/studio-cli-hooks`, the
first built spec-only feature this project ever closed. This record is the
durable evidence for the one manual intervention (an in-baseline comment was
attempted first, but `apply-governance`'s later YAML re-marshal strips
comments — baselines cannot carry provenance; records like this one can).

## Gap 1 — spec-only contract amendments are unappliable

`parlay internal apply-governance` refuses any amendment carrying `affects:`
("run /parlay-refine for it"), while refine's step-9 re-baseline blesses a
feature only via emitted CODE files mapped through generation markers. A
feature with no generated output therefore has NO sanctioned path from
"contract amendment authored and spliced" to "recorded applied".

**User-authorized manual procedure performed** (explicit authorization,
2026-08-31, after the auto-mode classifier correctly blocked a scripted
baseline write as would-be forged build state): with amendments 001/002 of
studio-cli-hooks spliced into the artifacts and validated, the feature's
`.baseline.yaml` was advanced by hand, mirroring `advanceAppliedMarker`
exactly — `last-applied-amendment: 2`, per-amendment 16-hex sha256 whole-file
hashes under `sources.amendments`, `generated-at` refreshed. The terminal
amendment 003 then went through `apply-governance --confirm` normally
(`feature_retired: true`).

**Follow-up owed:** a sanctioned applier for spliced contract amendments on
features with no generated output (e.g. `apply-governance --spliced-contract`
requiring resolvable affects + confirmation, or refine blessing via the
journal's feature rather than emitted files alone).

## Gap 2 — compaction exists in the schema but not in the tooling

The amendment schema documents the compacted-ledger shape
(`amendments/archive/`, legitimate sequence gaps), and compaction proved to
be the ONLY resolution to a real deadlock in feature closure: the affects
resolver requires historical refs to resolve forever, while
`feature-retirement-has-output` requires the contract artifacts those refs
resolve against to be disposed. Archiving the applied pre-terminal ledger
(001/002 moved to `amendments/archive/` by hand — no compaction command
exists) satisfied both.

The cross-tool incoherence this exposed — `check-drift`'s ledger-integrity
check read archived amendments as "removed" — was FIXED in the same change
this record ships with: integrity now follows compacted files into
`archive/` (byte-identical = retained history; edited = write-once
violation; in neither place = erased), with regressions covering all three
and the active-tail calculation proven blind to archived files.

**Follow-up owed:** a sanctioned `compact` operation (move + verify in one
governed step), so the manual `git mv` this closure used is the last one.

## Survivor accounting

After the de-studio pass, `studio-support/` appears only as history: in
frozen founding docs and ledger amendments of moved features, in
`docs/plans/` analysis records, and in the retired-root archive — 24
occurrences in the scoped product tree at the time of this record, none of
them a live ownership marker (verified by sweep; the count includes
amendment prose, which an earlier report undercounted as 18 by excluding
amendments).
