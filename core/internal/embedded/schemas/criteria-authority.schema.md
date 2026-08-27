<!--
parlay-section: build-artifact
parlay-feature: parlay-tool/criterion-authority
-->

# Criteria Authority Schema

`.parlay/build/<feature>/criteria-authority.yaml` — who accepted the standard a
feature is graded against, or the record of a run that proceeded without anyone
accepting it.

Tool-internal and never user-facing, but it carries a claim about a person, so
its shape is published: a reader auditing a project later needs to know exactly
what this file does and does not establish.

<!-- parlay:normative -->
## Top level

| Field | Required | Description |
|---|---|---|
| `schema_version` | yes | `1`. A file declaring any other version is refused rather than read leniently — the fields below are evidence, and guessing at an unknown layout would invent evidence. |
| `feature` | yes | The feature slug. Refused when it does not match the feature the file was read for, so a copied file cannot silently vouch for a different standard. |
| `approved` | no | The human approval in force. Absent means nobody has accepted this standard. |
| `machine_runs` | no | Audit records of runs that advanced without human approval. Never authority — see below. |

## `approved`

| Field | Required | Description |
|---|---|---|
| `at` | yes | When the approval was given, RFC 3339. |
| `authority` | yes | **What** accepted the standard, supplied by the decision channel that asked. Never derived from the environment. Reading `$USER` is how the retired coverage-review came to record a background process as a reviewer; a value the tool invents is not evidence. Where no trustworthy identity exists, the honest value names the channel — `interactive decision` — rather than a person. |
| `decision_id` | no | Ties the approval to the interaction that produced it, so it can be traced rather than merely asserted. |
| `criteria_hash` | yes | Hash over the deduplicated canonical `(ref, text)` pairs that were approved. |
| `criteria` | yes | The exact criteria accepted. Recorded in full because the hash alone cannot be reconstructed once the artifacts move on, which is precisely when somebody asks what was approved. |

<!-- /parlay:normative -->
An approval is bound to the criteria it names. When the standard changes, the
hash no longer matches and the approval no longer applies — it is not revoked,
it simply was not about the current standard. Re-approval is a new decision.

<!-- parlay:normative -->
## `machine_runs`

Each entry records one run that advanced a boundary **without** human approval,
permitted by explicit project policy plus an invocation flag.

| Field | Required | Description |
|---|---|---|
| `at` | yes | When the boundary was crossed. |
| `policy_source` | yes | The setting that permitted the waiver, so a reader can find the decision that allowed it rather than inferring one. |
| `run_id` | no | The execution. Free-form prose is not an audit trail: the question later is *which run did this*, and a sentence cannot answer it. |
| `criteria_hash` | yes | The standard that was graded against. |
| `criteria` | yes | Those criteria in full, for the same reason as above. |
| `reason` | no | Which boundary consumed the waiver. |

<!-- /parlay:normative -->
**A machine run is never authority.** An entry here records that something
happened; it does not make a later boundary pass. A past run against an
identical standard does not authorize a present one — that would turn an audit
event into standing permission, which is exactly the property this file exists
to keep separate.

One execution logs one event across the code and done boundaries when a shared
run identity is available (`PARLAY_RUN_ID`, or a CI job id). Without a carrier
proving two crossings belong to one pipeline, each boundary records its own:
claiming they are one run would be a guess.

## What this file establishes, and what it does not

It establishes **that a specific standard was accepted, and what was accepted**:
the criteria are recorded in full and bound by hash, so a later reader can tell
whether today's standard is the one that was approved.

It does **not** establish who a person is. `authority` is attribution supplied
by the decision channel, not verified identity — nothing in the tool proves the
value came from a human rather than the process that wrote it. Guidance must not
describe this file as proof that a person reviewed something; the honest claim is
that the supported workflow requires an explicit human decision, and that this
file records what that decision was about.

<!-- parlay:normative -->
## Errors

| Code | When |
|---|---|
| unsupported `schema_version` | The file declares a version this build does not implement. |
| feature mismatch | `feature` names a different feature than the one being read. |
| stale approval | `criteria_hash` does not match the current standard, so the approval is not about it. |
<!-- /parlay:normative -->
