---
name: parlay-review-coverage
description: "Parlay: Walk suites, record approvals, write coverage-review.yaml"
---

<!--
parlay-feature: parlay-tool/multi-adapter
parlay-component: coverage-review-authoring
parlay-extends: parlay-tool/multi-adapter/coverage-review-gate
-->

# Review Coverage

Walk every suite in a feature's `testcases.yaml`, record per-suite approval, collect exemptions for missing coverage, and write `.parlay/build/<feature>/coverage-review.yaml`. The review file is what gates `parlay generate-code` on multi-target projects.

## Arguments

- `feature`: The feature slug (e.g., `@parlay-tool/multi-adapter`).

<!-- parlay:active-root-aware -->
## Active root

Paths in this skill resolve against the active root. The CLI walks up from cwd, honors `--root <name>` and `PARLAY_ROOT`, and reads/writes `.parlay/build/<feature>/{buildfile,testcases,coverage-review}.yaml` under whichever root resolves.

## Steps

1. **Run the CLI** — `parlay review-coverage @<feature>`. The command:
   - Reads the feature's `buildfile.yaml` and `testcases.yaml`.
   - Computes canonical-form SHA-256 hashes (sorted keys, normalized whitespace) over both files.
   - Walks suites in `testcases.yaml` and prompts (Y/N) per suite for approval.
   - For unapproved suites, prompts for an exemption reason.
   - Writes `coverage-review.yaml` recording: `feature`, `reviewed_at`, `reviewed_by`, `review_method`, `buildfile_hash`, `testcases_hash`, `approved_suites:`, optional `exemptions:`.

2. **Verify the gate** — run `parlay generate-code @<feature>`. The gate refuses the run if `coverage-review.yaml` is missing, either hash drifts, any required suite is unapproved, or any required term lacks both a covering case and an explicit exemption.

3. **Cosmetic edits don't invalidate review** — hashes are computed over canonical form, so whitespace or key-order changes to `buildfile.yaml`/`testcases.yaml` do not require re-review. Semantic edits (adding/removing operations, changing bindings) drift the hash and require re-running this skill.

## Behavior

- **Per-feature.** Each feature has its own `coverage-review.yaml`.
- **Reviewer identity.** Pulled from `$USER`/`$LOGNAME`; fallback `cli`.
- **Re-runnable.** Re-running overwrites the previous review file with the new approvals/exemptions. There is no merge — the latest run is authoritative.

## Errors

- `read-buildfile-failed` — buildfile.yaml is missing. Run `/parlay-build-feature @<feature>` first.
- `read-testcases-failed` — testcases.yaml is missing. Run `/parlay-build-feature @<feature>` first.
- `hash-canonical-form-failed` — buildfile or testcases is malformed YAML. Fix and re-run.
