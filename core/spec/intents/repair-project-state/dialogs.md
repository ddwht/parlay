# Repair Project State — Dialogs

---

### Repair Project State

**Trigger**: /parlay-repair (or `parlay repair`)

User: /parlay-repair
System (background): Scans all three trees: spec/intents/, spec/handoff/, .parlay/build/. Classifies each directory (feature, initiative, deferred) using spec/intents/ as the authoritative source. Compares the directory sets across the three trees to find mismatches.
System (background): Found 2 mismatches:
System: Detected 2 inconsistencies:
System:   1. Initiative rename: spec/intents/auth-redesign/ exists but spec/handoff/auth-overhaul/ and .parlay/build/auth-overhaul/ still use the old name.
System:   2. Missing directory: spec/intents/billing/password-reset/ exists but spec/handoff/billing/password-reset/ does not.
System: **Mismatch 1: Initiative rename**
System:   spec/intents/auth-overhaul/ → spec/intents/auth-redesign/ (detected rename)
System:   spec/handoff/ and .parlay/build/ still have auth-overhaul/.
System:   Rename auth-overhaul → auth-redesign across spec/handoff/ and .parlay/build/? [Y/n]
User: Y
System (background): Applies rename across both trees using git mv.
System: [OK] Renamed auth-overhaul → auth-redesign on spec/handoff/ and .parlay/build/.
System: **Mismatch 2: Missing directory**
System:   spec/intents/billing/password-reset/ exists (feature — has intents.md)
System:   spec/handoff/billing/password-reset/ — missing
System:   .parlay/build/billing/password-reset/ — exists
  A: Recreate spec/handoff/billing/password-reset/ (restore lockstep)
  B: Leave it — I'll handle this manually
User: A
System (background): Creates spec/handoff/billing/password-reset/.
System: [OK] Created spec/handoff/billing/password-reset/.
System: Repair complete. Applied 2 fixes. Project is now in lockstep.

#### Branch: Project already in lockstep

User: /parlay-repair
System (background): Scans all three trees. Compares directory sets. No mismatches found.
System: Project is in lockstep. No repairs needed.

#### Branch: Unambiguous feature move

User: /parlay-repair
System (background): Detects that spec/intents/auth-overhaul/password-reset/ exists but spec/intents/password-reset/ does not. Meanwhile, spec/handoff/password-reset/ and .parlay/build/password-reset/ still exist at the top level.
System: Detected 1 inconsistency:
System: **Feature move detected**
System:   password-reset moved from top level to auth-overhaul/ (in spec/intents/)
System:   spec/handoff/ and .parlay/build/ still have it at the top level.
System:   Move password-reset/ into auth-overhaul/ on spec/handoff/ and .parlay/build/? [Y/n]
User: Y
System (background): Applies the move across both trees.
System: [OK] Moved password-reset into auth-overhaul/ on spec/handoff/ and .parlay/build/.
System: Repair complete. Applied 1 fix. Project is now in lockstep.

#### Branch: Ambiguous — multiple renames

User: /parlay-repair
System (background): Detects that spec/intents/ has auth-redesign/ and payments/ where spec/handoff/ and .parlay/build/ have auth-overhaul/ and billing/. Two old names, two new names — multiple possible pairings.
System: Detected 1 ambiguous inconsistency:
System: **Ambiguous rename — two initiatives changed simultaneously**
System:   Old names (on spec/handoff/ and .parlay/build/): auth-overhaul/, billing/
System:   New names (on spec/intents/): auth-redesign/, payments/
System:   Which pairing is correct?
  A: auth-overhaul → auth-redesign AND billing → payments
  B: auth-overhaul → payments AND billing → auth-redesign
  C: None of these — I'll resolve manually
User: A
System (background): Applies both renames in sequence.
System: [OK] Renamed auth-overhaul → auth-redesign, billing → payments on spec/handoff/ and .parlay/build/.
System: Repair complete. Applied 2 fixes. Project is now in lockstep.

#### Branch: Extra directory on handoff/build

User: /parlay-repair
System (background): Finds spec/handoff/old-feature/ exists but there is no corresponding spec/intents/old-feature/.
System: Detected 1 inconsistency:
System: **Extra directory** — spec/handoff/old-feature/ (3 files) has no matching entry in spec/intents/.
System:   This is likely stale output from a feature that was removed from spec/intents/.
  A: Delete spec/handoff/old-feature/ (3 files) [default: no]
  B: Keep it — I plan to re-create the source later
User: A
System (background): Deletes spec/handoff/old-feature/.
System: [OK] Deleted spec/handoff/old-feature/ (3 files).

#### Branch: Destructive prompt defaults to no

User: /parlay-repair
System: **Extra directory** — .parlay/build/archived-feature/ (18 files) has no matching entry in spec/intents/.
  A: Delete .parlay/build/archived-feature/ (18 files) [default: no]
  B: Keep it
User: (presses Enter without typing)
System: Kept .parlay/build/archived-feature/ — no changes.

#### Branch: Unrecognized mismatch

User: /parlay-repair
System (background): Detects a directory structure that doesn't match any known category — e.g., spec/intents/foo/ is a feature, spec/handoff/foo/ is an initiative (contains subdirs with intents.md), and .parlay/build/foo/ doesn't exist.
System: Detected 1 inconsistency:
System: **Unresolved** — spec/intents/foo/ classifies as a feature but spec/handoff/foo/ classifies as an initiative. This mismatch doesn't fit any repair category.
System:   spec/intents/foo/intents.md exists (feature)
System:   spec/handoff/foo/bar/intents.md exists (makes foo/ an initiative in handoff tree)
System:   .parlay/build/foo/ — missing
System: Please resolve this manually and re-run parlay repair.
System: Repair finished with 1 unresolved mismatch. Exit status: 1.

#### Branch: --dry-run

User: parlay repair --dry-run
System (background): Scans trees. Finds 2 mismatches.
System: Dry run — no changes will be applied.
System: Detected 2 inconsistencies:
System:   1. [WOULD FIX] Initiative rename: auth-overhaul → auth-redesign across spec/handoff/ and .parlay/build/
System:   2. [WOULD ASK] Missing directory: spec/handoff/billing/password-reset/ — would ask whether to recreate

#### Branch: --yes auto-confirms unambiguous

User: parlay repair --yes
System (background): Scans trees. Finds 3 mismatches: 2 unambiguous, 1 ambiguous.
System: Auto-confirming unambiguous repairs (--yes).
System: [OK] Renamed auth-overhaul → auth-redesign on spec/handoff/ and .parlay/build/.
System: [OK] Created spec/handoff/billing/password-reset/.
System: **Ambiguous rename** — pausing for input (--yes does not auto-confirm ambiguous repairs):
System:   Old names: alpha/, beta/. New names: gamma/, delta/.
  A: alpha → gamma AND beta → delta
  B: alpha → delta AND beta → gamma
  C: None — resolve manually
User: A
System: [OK] Applied. Repair complete. 3 fixes applied. Project is now in lockstep.

#### Branch: Rollback on single repair failure

User: /parlay-repair
System (background): Detects 2 mismatches. Applies the first successfully.
System: [OK] Renamed auth-overhaul → auth-redesign on spec/handoff/ and .parlay/build/.
System (background): Attempts the second repair — creating spec/handoff/billing/. Permission denied on spec/handoff/.
System: [ERR] Could not create spec/handoff/billing/: permission denied. This repair was rolled back.
System: Repair finished. Applied 1 fix, 1 failed. Fix the permissions and re-run parlay repair for the remaining mismatch.

---
