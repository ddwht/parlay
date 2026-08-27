---
amendment: mechanical-readiness-replaces-the-review
date: 2026-08-27
trigger: "the coverage-review gate this feature exists to run is being removed"
retires_feature: true
outcome: obsolete
supersedes_intents:
  - insert-a-coverage-review-step-between-the-build-and-code-phase-groups
---

## Change

The loop no longer runs a coverage review between the build and code
phase-groups, and this feature — which exists only to insert that step — ends
with it. Entry to the code phase-group is gated on mechanical readiness
instead: strict testcases and criterion checks the tool computes, rather than a
person approving a list.

## Why

The feature was a correct answer to a real problem. `/parlay-generate-code`
refused to run without an approved `coverage-review.yaml`, the build
phase-group never produced one, and a multi-target loop therefore dead-ended at
`coverage-review-missing`. Inserting the review step made the loop work.

What has changed is the thing it was inserting. The review it runs shows the
reviewer a suite name and asks `approve "name"? [Y/n]` with the default set to
yes — no cases, no criteria, no assertions, no diff. It cannot establish that a
person judged a test plan adequate; it establishes that a person approved a
label. Nine real review files in this repo record 50 approved suites, **zero**
exemptions, and `reviewed_by: node` in five of them: the artifact whose stated
purpose is recording that a person looked mostly records a process name.

Meanwhile the substance moved underneath it. The v0.6.0 criterion work made
coverage bullet-granular and mechanical, so what the human review was nominally
judging is now computed — and computed more consistently than a person clicking
through names can manage.

Retiring the feature rather than amending it is the honest shape: with the
review gone there is no step for it to insert, and a feature whose only promise
was to insert that step has nothing left to promise. `obsolete` rather than
`replaced` because no other feature carries this work — the need itself is
gone, not relocated.

## Acceptance

- A multi-target feature driven through `/parlay-loop` reaches
  `/parlay-generate-code` without a coverage-review step, and without any
  review artifact existing.
- The build-to-code boundary is gated on mechanical readiness, and a feature
  failing it is blocked there rather than at a missing review file.
- A presentation-only feature is unaffected, exactly as it was under the
  retired step.
