# Code Generation — Dialogs

---

### Build Feature

**Trigger**: /parlay-build-feature @feature-name

User: /parlay-build-feature @upgrade-plan-creation
System (background): Loads schemas, adapter, and feature files.
System (background): Runs readiness check and diff.
System (condition: first build): No committed state yet — generating the full buildfile from scratch.
System (condition: incremental): Diff report: ==N== stable, ==M== dirty, ==K== removed.
System (background): Generates buildfile.yaml and testcases.yaml. Validates with deep checks.
System: [OK] Build specification ready. Internal artifacts saved to .parlay/build/upgrade-plan-creation/.
System: Next: run /parlay-generate-code to produce the prototype code.

---

### Generate Prototype Code

**Trigger**: /parlay-generate-code

User: /parlay-generate-code
System (background): Loads buildfiles, adapter, blueprint. Does NOT read spec/intents/.
System (background): Runs diff, scan-generated, verify-generated.
System (condition: first generation): No code state committed yet — treating every component as new.
System (condition: stable file modified): ==filename== has been edited since last generation.
  A: Overwrite (lose my edits)
  B: Skip this file
  C: Show me the diff first
System (background): Generates code, runs tests.
System (condition: tests pass): [OK] All tests pass. Build state committed.
System (condition: tests fail): Tests failed:
System: - ==test name== — ==failure summary==
  A: Show me the failures in detail
  B: Regenerate the failing components
  C: Stop, I'll investigate manually

---

### Mount Feature into Existing Page

**Trigger**: During /parlay-generate-code when a route targets an existing page

System (background): Found existing file for page "Settings" (no parlay marker — user-owned).
System (background): Scans adapter mount-strategies for detection patterns in file content.
System (condition: one match): Found matching mount strategy on line ==N==.
System: Proposed change to ==filename==:
System: ==unified diff==
  A: Apply this change
  B: Skip — I'll integrate manually
  C: Edit the proposed change
System (condition: zero matches): ==filename== uses widgets that don't match any mount strategy.
  A: Show me the file so I can describe the pattern
  B: Skip — I'll integrate manually
  C: Add as a new standalone route instead
System (condition: multiple matches): ==filename== has multiple integration points:
  A: ==strategy 1== (found on line ==N==)
  B: ==strategy 2== (found on line ==M==)
  C: Skip — I'll integrate manually

---

### External Type Disambiguation

**Trigger**: During /parlay-generate-code when grepping for an entity name yields matches

System (background): Checking source tree for existing type definitions.
System (condition: one match): Found existing definition for "==Entity==" at ==path==. Will import instead of generating.
System (condition: multiple matches): Found multiple existing definitions for "==Entity==":
  A: ==path1== — ==type snippet==
  B: ==path2== — ==type snippet==
  C: Generate a new type (ignore existing definitions)
System (condition: no match): No existing definition found — will generate a new type declaration.

---
