---
amendment: dry-run-flag-removed-as-never-implemented
date: 2026-08-31
trigger: "the backend-only teardown audit found --dry-run declared but never read: no code path ever consumed the flag, so the preview the spec promises has never existed"
affects:
  - "@parlay-tool/domain-model-yaml-migration/surface:migrate-domain-model-dry-run-preview"
  - "@parlay-tool/domain-model-yaml-migration/infrastructure:migrate-domain-model-command-and-idempotency-guard"
---

## Change

`parlay migrate-domain-model` no longer accepts `--dry-run`. The flag is
removed from the command, from its help text, and from the `parlay-extends`
marker that attributed it. `--force` and `--root` are unchanged.

## Why

The flag was declared and documented but its variable was never read: no
branch of `runMigrateDomainModel` ever consulted it, so every run behaved
identically with or without the flag. The spec's promise — print the planned
YAML and a diff, write nothing — described behavior that was never built, and
there is nothing for it to preview: the actual translation happens in an AI
skill, not in the Go command, so the Go side has no planned YAML to print.
A flag that silently does nothing is worse than no flag: it invites a user to
trust that "nothing was written" on a run that would have written nothing
anyway, and it would keep inviting that trust the day the command grows a
write path.

Removing it is honest-guidance repair, not a behavior change — no behavior
existed. If a real preview is wanted later, it should be specified against
the AI-skill boundary that actually performs the translation, as a new
decision, not by resurrecting this flag.

## Acceptance

- `parlay migrate-domain-model --dry-run` is an unknown-flag error.
- The command's help text does not mention `--dry-run`.
- All other flags and the idempotency guard behave exactly as before.
