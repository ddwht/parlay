# Criteria Authority Schema — authoring digest

Derived from `criteria-authority.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

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

---

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

---

## Errors

| Code | When |
|---|---|
| unsupported `schema_version` | The file declares a version this build does not implement. |
| feature mismatch | `feature` names a different feature than the one being read. |
| stale approval | `criteria_hash` does not match the current standard, so the approval is not about it. |
