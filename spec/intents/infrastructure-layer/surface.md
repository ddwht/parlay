# Infrastructure Layer — Surface

---

## Check-Readiness Infrastructure Support

**Shows**: status, message
**Actions**: invoke
**Source**: @infrastructure-layer/define-the-infrastructure-artifact

**Page**: check-readiness
**Region**: main
**Order**: 1

**Notes**:
- Updated check-readiness output that accepts the new "at least one of surface.md or infrastructure.md" rule.
- `status` shows success when infrastructure.md is present (even without surface.md), or error when neither exists.
- `message` on error names both files and suggests how to create each (run /parlay-create-artifacts for the decision flow, or author infrastructure.md directly for behind-the-scenes features).
- `invoke` directs the developer to the appropriate creation path.

---

## Infrastructure Validation Result

**Shows**: status, message, data-list
**Actions**: invoke
**Source**: @infrastructure-layer/define-the-infrastructure-schema

**Page**: validate
**Region**: main
**Order**: 1

**Notes**:
- Output of `parlay validate --type infrastructure`.
- `status` carries pass/fail. On pass, reports fragment count and "all required fields valid."
- `data-list` on failure shows each validation error: fragment name, field name, error description — structured for agent consumption via `--json` flag.
- Portability warnings are shown separately from errors: fragments with framework-specific content in Behavior or Affects trigger `[WARN] Portability:` messages listing the offending content.
- `invoke` directs the developer to fix the reported issues and re-validate.

---

## Build-Feature Infrastructure Report

**Shows**: status, message, data-value
**Actions**: invoke
**Source**: @infrastructure-layer/bridge-infrastructure-to-framework-specific-cross-cutting

**Page**: build-feature
**Region**: main
**Order**: 1

**Notes**:
- Build-feature output when processing a feature that has `infrastructure.md`.
- `status` shows [OK] on success.
- `message` summarizes: "Buildfile contains N cross-cutting entries" (pure infrastructure) or "N components and M cross-cutting entries" (mixed feature). For pure infrastructure features, explicitly notes "no components — this is a pure infrastructure feature."
- `data-value` shows the path to the generated buildfile.
- `invoke` directs the developer to run /parlay-generate-code next.
- When resolution required designer input (Affects scope couldn't be auto-resolved), the message notes how many fragments needed manual guidance.

---

## Cross-Cutting Merge Review

**Shows**: status, message, diff, data-value, data-list
**Actions**: select-one, invoke
**Flow**: review-and-approve
**Source**: @infrastructure-layer/update-generate-code-to-process-infrastructure-sourced-entries

**Page**: generate-code
**Region**: main
**Order**: 1

**Notes**:
- Generate-code output when processing `cross-cutting:` entries from the buildfile.
- At this stage, all entries are framework-specific (build-feature already resolved abstract infrastructure to concrete targets). Generate-code never reads infrastructure.md — the buildfile is the boundary.
- For `target-files:` entries: `diff` shows the unified diff of the Tier 2 intelligent merge against each target file.
- For `target-pattern:` entries: `data-list` first shows the files matched by the grep pattern and the match count, then `diff` shows each file's merge diff in sequence.
- `select-one` presents the A/B/C review menu per diff: (A) Apply, (B) Skip — integrate manually, (C) Edit the proposed change.
- `status` shows [OK] after all diffs are applied, [WARN] for zero-match patterns, [ERR] for missing target files on modify-only entries.
- `data-value` shows before/after paths and the entry's source reference for traceability.
- `message` at the end summarizes: "Applied N cross-cutting changes across M files. Infrastructure is in place."

---
