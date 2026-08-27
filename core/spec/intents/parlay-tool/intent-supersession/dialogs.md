# Intent supersession — Dialogs

---

### Superseding a founding intent that owns no contract artifact

**Trigger**: A maintainer runs `/parlay-refine` on a feature whose founding intent no longer describes what the project intends to do, and the feature has no contract entry to amend.

User: /parlay-refine @parlay-tool/loop-coverage-review-phase
User: The loop should no longer run a coverage review before codegen. Mechanical readiness replaces it.
System (background): Resolves the feature; finds `intents.md` and `dialogs.md` and no contract artifact, so there is no `affects:` target for an ordinary amendment.
System: This change contradicts the founding intent ==insert-a-coverage-review-step-between-the-build-and-code-phase-groups==, which has no contract entry to amend. Recording it means superseding that intent — the frozen document is kept and marked as replaced, not edited.
System: Superseded intent — Goal: ==Make /parlay-loop run the coverage review before it enters the code phase-group==. Verify: ==5 bullets==.
System: Replacement — Change: ==clean mechanical readiness replaces blanket review==. Acceptance: ==3 criteria==.
System: Downstream contract entries sourced from this intent: none — the feature carries no contract artifact.
System: Supersede this intent?
    a. Supersede — write amendment 001 naming it in `supersedes_intents:`
    b. Cancel — leave the intent live and record nothing
User: a
System (background): Writes `amendments/001-mechanical-readiness-replaces-blanket-review.md` with an empty `affects:` and the intent named in `supersedes_intents:`. `intents.md` is not opened for writing.
System: Wrote amendment 001. The intent stays in `intents.md` as history and is no longer current specification once the amendment is applied.

---

### Refusing a supersession that would orphan generated work

**Trigger**: The same flow on a feature that does carry contract artifacts, where the superseded intent is the source of entries nobody has accounted for.

User: /parlay-refine @parlay-tool/multi-adapter
User: The coverage-review failure codes are replaced by the mechanical readiness codes.
System (background): Resolves entries whose `source:` names the intent being superseded.
System: Cannot record this supersession yet. 3 contract entries are sourced from ==coverage-review-gate==, and each needs an explicit disposition before the intent can be retired:
    a. `@multi-adapter/infrastructure:coverage-review-gate-failure` — replace, remove or retain?
    b. `@multi-adapter/surface:gate-failure-report` — replace, remove or retain?
    c. `@multi-adapter/operation:check-review-gate` — replace, remove or retain?
System: Retiring an intent without accounting for what it produced leaves generated scope with no owner. Name each entry in `affects:` with its disposition, then re-run.

---

### Declining to answer for a person who is not there

**Trigger**: A supersession is reached during an unattended run.

System (background): `/parlay-refine --non-interactive` reaches a change that would supersede a founding intent.
System: Blocked — superseding a founding intent reduces the scope this feature promised, and there is no safe default for that. Nothing was written.
System: Re-run interactively, or record the amendment by hand with `supersedes_intents:` and an `## Acceptance` stating what replaces the retired promise.

---

### Reading a feature whose premise has been replaced

**Trigger**: A maintainer asks what a feature currently promises, after a supersession has been applied.

User: /parlay status @parlay-tool/loop-coverage-review-phase
System: Current criteria come from amendment 001 — ==3 acceptance criteria==.
System: History — the founding intent ==insert-a-coverage-review-step-between-the-build-and-code-phase-groups== was superseded by amendment 001 on ==2026-08-26==. Its Goal and Verify are retained and are no longer current specification.
System (condition: the amendment is authored but not yet applied): Amendment 001 supersedes ==insert-a-coverage-review-step-between-the-build-and-code-phase-groups== but has not been applied. The founding intent is still current specification and this boundary is blocked until the amendment is applied.
