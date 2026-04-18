# Repair

Validate and reconcile the three parallel trees (spec/intents/, spec/handoff/, .parlay/build/) after external filesystem operations.

## Arguments

None.

## Steps

1. Run: `parlay repair`
2. For each detected mismatch, the command will prompt interactively. Confirm or skip each repair.
3. If the designer wants to preview without applying: `parlay repair --dry-run`
4. If the designer wants to auto-confirm unambiguous repairs: `parlay repair --yes`
