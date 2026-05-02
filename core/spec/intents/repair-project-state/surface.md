# Repair Project State — Surface

---

## Repair Report

**Shows**: status, message, data-list, summary
**Actions**: select-one, confirm
**Flow**: guided-flow
**Source**: @repair-project-state/repair-project-state

**Page**: repair
**Region**: main
**Order**: 1

**Notes**:
- Output of `parlay repair` — the interactive reconciliation flow for three-tree lockstep.
- `summary` at the start reports the total mismatch count found across the three trees.
- `data-list` shows each detected mismatch with its category (initiative rename, feature move, missing directory, extra directory, ambiguous, unrecognized) and the full paths on each tree.
- For each mismatch, the flow presents a resolution:
  - Unambiguous (rename, move, missing dir): `confirm` with Y/n prompt showing the proposed repair and full paths.
  - Ambiguous (multiple candidate pairings): `select-one` listing the candidate pairings plus a "resolve manually" option.
  - Extra directory (potentially destructive): `confirm` with default-no, showing file count.
  - Unrecognized: `message` naming the offending paths, instructing manual resolution.
- `status` per mismatch: [OK] for applied, [ERR] for failed (with rollback note), [SKIP] for deferred.
- `summary` at the end reports total applied, failed, and unresolved. Exit status non-zero if any unresolved.
- `--dry-run` variant: shows [WOULD FIX] / [WOULD ASK] prefixes without applying or prompting.
- `--yes` variant: auto-confirms unambiguous repairs (prints [OK] without prompting), pauses on ambiguous ones.
- `message` when no mismatches: "Project is in lockstep. No repairs needed."

---
