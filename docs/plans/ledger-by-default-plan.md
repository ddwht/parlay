# Ledger by default — retire the `ledger:` flag

Status: IMPLEMENTED (2026-08-17) — target v0.4.0 (behavior change for
existing projects; not a patch on v0.3.0). All four stages landed; the
dogfood roots (core: 42 features, studio: 18) migrated dry-run-clean and
their `ledger: true` keys are gone. One audit correction discovered during
implementation: migrate-config has no config-rewrite path (it only creates
adapter-set.yaml), so the leftover `ledger:` key follows the no_studio
precedent instead — inert under non-strict decoding, documented in
config.go.

## Decision

The ledger-and-contract model stops being opt-in. There is one regime:
founding docs freeze at first build, the four contract artifacts are current
truth, change goes through append-only amendments. The `ledger:` config flag
is removed; `parlay upgrade` carries old projects across, with a migrator for
the one state that genuinely needs repair.

Evidence basis: the WP10 benchmark (docs/plans/benchmark-full-findings.md) —
cost parity with the old regime, fewer frictions, decisively better change
records, and state integrity held where the status-quo run corrupted its own
spec. The flag's purpose was to protect that experiment; the experiment is
done.

## What the flag actually gates today (audit result)

The dual-regime surface is small and known exactly:

| Site | What the flag changes |
|---|---|
| `core/internal/config/config.go:66,72` | `Ledger bool` field + `LedgerEnabled()` |
| `core/internal/commands/baseline.go:413,528` | `ledgerOn` — classifies founding-doc changes as `ledger_integrity` violations instead of ordinary drift; gates `unapplied_amendments` reporting |
| `refine.skill.md:86` | ledger branch: author an amendment + apply to contract, vs. edit spec directly |
| `generate-enggspec.skill.md:43` | projection mode: History section, supersession links, drift disclosure, regenerable stamp |
| `loop.skill.md:150`, `scaffold-dialogs.skill.md:35` | dialogs/intents freeze-at-first-build language |
| `doctor.skill.md:42,89` | ledger-health findings section |
| `deployer.go:51` | CLAUDE.md File Ownership: "Ledger projects (`ledger: true`) add one zone…" conditional prose |

Everything else (amendment file validation, `check-amendments`, baseline v3
`last-applied-amendment`, `--retire-md`) is already unconditional — it was
built harmless-when-absent.

## The one real migration problem

Freezing is implicit: a founding doc is "frozen" at the moment its feature's
baseline is written (`save-build-state` after a green build). There is no
freeze timestamp to stamp during migration — the baseline hashes ARE the
freeze point.

So for an old project, flipping the regime is safe **except in one state**: a
feature whose intents.md/dialogs.md have been edited since its last green
build. Under the old regime that is pending drift ("rebuild me"); under the
new regime it becomes a `ledger_integrity` violation ("you edited a frozen
document") — for an edit that was perfectly legal when it was made. Migration
exists to dissolve exactly this: accept the current text as the founding
state, so the ledger starts clean from today.

**Design point — freeze-stamp must not bless the build.** The obvious repair
tool, `save-build-state`, stamps the whole feature: spec hashes AND
buildfile/testcases/code hashes. Using it on a drifted feature would mark an
un-rebuilt build state green — the WP6 false-stable bug reintroduced as a
migration step. The migrator therefore rewrites **only the spec-side founding
hashes** (baseline `Intents`, `Sources.Intents`, dialog hashes) and leaves
build-side hashes untouched, so any real spec→build staleness keeps
reporting. This is a new narrow write path in baseline.go, not a call to
saveBuildState.

## Stage 1 — Code: ledger semantics unconditional

- `config.go`: delete `Ledger` field and `LedgerEnabled()`. Config decoding
  is non-strict (verified: no `KnownFields`/strict decode anywhere in
  `core/internal/config/`), so a leftover `ledger: true` key in old configs
  is silently inert — no error, no tolerance shim needed.
- `migrate-config`: strip the `ledger:` key when rewriting (it reads raw YAML
  through its own inline struct; add the key to the drop list).
- `baseline.go`: delete `projectLedgerEnabled` and the `ledgerOn` branches.
  The non-ledger arms (`NewIntents`, intent-level `Drifted` for founding
  docs) collapse into the `ledger_integrity` classification. Keep the
  `drifted`/`new_intents` JSON fields for contract-artifact and shared-source
  drift — only the founding-doc arms move.
- `check-drift` fix text for "changed after freeze": name BOTH exits — record
  an amendment (`/parlay-refine`) for a change you meant, or
  `parlay migrate-ledger` for a pre-v0.4 edit the freeze shouldn't count.
- Severity table: no new codes needed for the flag removal itself;
  `ledger_integrity` is already an established finding class.

## Stage 2 — Migration path

New command `parlay migrate-ledger` (sibling of the migrate-* family, and
like them it keeps whatever read paths it needs forever):

1. Per feature with a baseline: compare founding docs against baseline
   hashes. Clean → nothing to do. Drifted → re-stamp spec-side founding
   hashes to current content (the narrow write path above), and print what
   was frozen, per file, so the operator sees exactly which edits got
   grandfathered into the founding state.
2. Per feature without a baseline (never built): nothing — it freezes
   normally at its first green build.
3. Refuse (with fix text) if a `surface.md` is present — `migrate-spec
   --retire-md` first; and refuse if amendments already exist for a feature
   whose founding docs drifted (that project was already ledger-mode; a
   drifted frozen doc there is a real integrity violation the migrator must
   not paper over).
4. `--dry-run` prints the per-feature verdicts without writing.
5. Idempotent: a second run finds everything clean and writes nothing.

`parlay upgrade` integration (same pattern as `offerAdapterKindOptIn` in
`upgrade.go:227`): after deploying skills, run the migrator's dry-run scan;
if any feature needs freezing, prompt interactively to run it, and in
non-interactive mode print a note naming the command. Upgrade never migrates
silently — freezing grandfathers edits into founding history, and a person
should see the list.

Projects that update the binary but never run `parlay upgrade` fail loud, not
wrong: their next `check-drift` reports `ledger_integrity` with fix text
naming `migrate-ledger`. Same fail-loud-with-instructions contract as every
v0.3 removal.

## Stage 3 — Skills and prose: single regime

- `refine.skill.md`: the ledger branch becomes the only flow — a refinement
  IS an amendment applied to the contract. Delete the direct-edit path and
  the "Read `.parlay/config.yaml` before step 3" gate.
- `loop.skill.md`, `scaffold-dialogs.skill.md`: freeze-at-first-build is
  stated as the rule, not as a mode.
- `generate-enggspec.skill.md`: projection behavior (History section,
  supersession links, drift disclosure, regenerable stamp) unconditional.
- `doctor.skill.md`: ledger findings unconditional; add one finding —
  "founding docs drifted with no amendments: run `parlay migrate-ledger`" —
  so doctor is the discovery path for unmigrated projects.
- `deployer.go` `fileOwnershipSection`: drop the "Ledger projects add one
  zone" preface — the amendments zone and frozen-founding-docs rule become
  the layout, stated once. Deletes the current wart where non-ledger projects
  carry doctrine that doesn't apply to them.
- Schemas: `feature-structure.schema.md` documents `amendments/` as a
  standard zone; config schema drops the `ledger:` row; regenerate DIGEST.
- Follow the dogfooding rule per stage: edit embedded sources → `make
  build-noui` → `./parlay upgrade` → `make verify-skills`.

## Stage 4 — Dogfood + verification

- Drop `ledger: true` from this repo's `.parlay/config.yaml` (and core's);
  run `parlay migrate-ledger` on the dogfood roots; confirm dry-run-clean.
- Full suite (`CGO_ENABLED=0 go test -tags noui ./...`), `go vet -tags noui`,
  `make verify-skills`, DIGEST regen.
- Meta-tests to extend: conformance pin for `migrate-ledger` in the command
  matrix; a `TestLedgerFlagIsRemoved` guard (same idiom as
  `TestNoStudioFlagIsRemoved`); baseline tests for the spec-only re-stamp
  (must NOT touch build-side hashes — regression test against the WP6
  failure mode).
- Smoke: one old-regime fixture project end-to-end — upgrade → prompt →
  migrate → refine produces amendment 001 → check-drift clean.

## Explicitly out of scope

- Amendment compaction/archival policy (separate decision).
- WP9 `relation:` field (own plan: relation-field-design.md).
- Any change to the amendment file format or baseline v3 schema shape.

## Sequencing note

Stage 1 and Stage 2 land together (the flag's removal and the migrator must
ship in the same release — a binary with ledger-always and no migrator
strands old projects). Stage 3 can follow in the same release train; Stage 4
gates the tag. Release as v0.4.0.
