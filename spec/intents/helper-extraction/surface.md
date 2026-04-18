# Helper Extraction — Surface

---

## Duplication Scan Results

**Shows**: summary, data-list, empty-state
**Actions**: invoke
**Source**: @helper-extraction/extract-duplicated-helpers-into-shared-packages

**Page**: simplify
**Region**: main
**Order**: 1

**Notes**:
- Output of the initial `/parlay-simplify` scan phase — before any extraction is proposed.
- `summary` reports the total count of duplicated helper groups found across generated files.
- `data-list` shows each duplicated group: function name, which files contain it, and whether it's identical or near-identical (with similarity percentage).
- `empty-state` is shown when no duplicates are found — "No duplicated helpers found across generated files. Nothing to extract."
- `invoke` triggers the extraction review flow (Fragment 2) for the first group. If no duplicates, the command exits cleanly.
- Only parlay-generated files are scanned (identified by parlay markers). User-owned files are explicitly excluded and never listed.

---

## Extraction Proposal Review

**Shows**: diff, message, data-value, status
**Actions**: select-one, provide-text
**Flow**: review-and-approve
**Source**: @helper-extraction/extract-duplicated-helpers-into-shared-packages

**Page**: simplify
**Region**: main
**Order**: 2

**Notes**:
- Shown once per duplicated helper group, sequentially. The designer reviews each extraction independently.
- `message` names the function being extracted, lists the source files containing the duplicate, and names the proposed target package (determined from the adapter's file-conventions).
- For near-identical matches, `message` also notes the differences (e.g., "differs only in error message string") and which version will be used as the canonical one.
- `diff` shows the unified diff of the proposed extraction: function removed from each source file, added to the shared package, import statements updated.
- `data-value` shows the proposed target path (e.g., internal/config/helpers.go).
- `select-one` presents the approval menu: (A) Apply, (B) Skip, (C) Use the other version (for near-identical), (D) Change the target package. Option C only appears for near-identical groups. Option D accepts free-form input via `provide-text` for the override path.
- `status` after each decision: [OK] for applied extractions (with commit message), "Skipped" for declined ones.
- After all groups are processed, a final `status` + `message` summarizes: "Extracted N helpers into shared packages. Each extraction is a separate commit."
- The flow is review-and-approve: read the diff, then decide. No batch mode — each extraction is independent so the designer can accept some and skip others.

---
