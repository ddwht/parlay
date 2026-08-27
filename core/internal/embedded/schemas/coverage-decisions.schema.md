<!--
parlay-section: build-artifact
parlay-feature: parlay-tool/criterion-authority
-->

# Coverage Decisions Schema

`.parlay/build/<feature>/coverage-decisions.yaml` — the judgments a person has
made about test coverage for one feature.

Named for what it holds. It began as a list of waivers and was called
`coverage-exceptions.yaml`; it now also carries approvals that waive nothing,
decisions withdrawn but kept, and answers to judgments inherited from the
retired coverage-review. A file read under the old name is migrated on the next
write.

Tool-internal, but every entry is a claim that a person decided something, so
the shape is published.

<!-- parlay:normative -->
## Top level

| Field | Required | Description |
|---|---|---|
| `schema_version` | yes | `1`. Any other version is refused rather than read leniently. |
| `feature` | yes | The feature slug; refused on mismatch. |
| `criteria_hash` | yes | The standard these decisions were made against. |
| `granted_at` | yes | When this ledger was first opened. Per-decision timing lives on each entry. |
| `exceptions` | no | Decisions in force. |
| `retired_decisions` | no | Decisions withdrawn, kept rather than deleted. |
| `legacy_file_hash` | no | The version of the retired coverage-review that `reconciled_legacy` answers. |
| `reconciled_legacy` | no | Stranded legacy judgments that have been answered. |
| `deferred_legacy` | no | Review attempts that reached no decision. **Not answers.** |

<!-- /parlay:normative -->
There is no file-level "granted by". A single slot is overwritten by every
append, which silently reassigns earlier judgments to whoever wrote last —
attribution that misattributes is worse than none.

<!-- parlay:normative -->
## `exceptions`

| Field | Required | Description |
|---|---|---|
| `ref` | yes | The contract entry. |
| `text` | no | The exact criterion. Omitted means the exception is **entry-wide**, which is broader and warned. |
| `kind` | yes | `waived` or `state-only`. |
| `reason` | yes | Why. An exception nobody can review later is not one. |
| `at`, `by` | yes | Per decision, never file-level. |
| `entry_hash` | no | For an entry-wide exception: the bullet set it was granted over, so adding a bullet invalidates it. A bullet-specific exception needs none — its `(ref, text)` **is** its binding. |
| `suite`, `case` | `state-only` only | The case whose weaker observation is accepted. |
| `case_hash` | `state-only` only | What that case actually observed when approved. |

<!-- /parlay:normative -->
### `kind: waived`

The criterion needs no test. It excuses coverage.

### `kind: state-only`

The criterion **is** discharged, by a case observing state rather than what the
criterion states. This excuses nothing — it accepts a weaker observation.

A downgrade binds to case CONTENT, not to suite and case names. A name survives
its body being replaced, so a decision keyed on names alone keeps matching after
the case comes to observe something else entirely, leaving the reviewer on
record approving an observation they never saw. The fingerprint covers the
case's whole declared content, re-encoded so that reindentation and key
reordering do not read as a changed observation, while steps, cited criterion
and the coverage marker all do.

A decision with no `case_hash` is **pre-binding** — an old-format entry in this
ledger, distinct from a stranded legacy exemption — and must be re-confirmed
rather than honoured.

<!-- parlay:normative -->
### Identity, by kind

A waiver is a claim about the criterion, so one criterion cannot be waived
twice: a second is refused as a duplicate that would shadow the first.

A downgrade is a claim about one case, and several cases may each observe one
criterion weakly for their own reasons. Downgrades are therefore identified by
`(ref, text, suite, case)`. Keying them on `(ref, text)` alone refuses the
second as a duplicate and leaves one case unreviewable.

<!-- /parlay:normative -->
## `retired_decisions`

A decision whose subject changed or went away. The entry is moved here rather
than deleted: an approval that silently vanishes is indistinguishable from one
nobody made.

Each carries the original judgment (`original_reason`, `original_by`,
`original_at`) **and** the choice to withdraw it (`reason`, `by`, `at`). One
without the other tells half the story.

Re-approving a drifted case is retire-then-record, never an edit in place — an
edit makes one review look like two.

<!-- parlay:normative -->
## `reconciled_legacy` and `deferred_legacy`

Both concern exemptions stranded in a retired `coverage-review.yaml`.

| Field | Required | Description |
|---|---|---|
| `ref`, `criterion_text` | yes / no | What the legacy entry named. |
| `fingerprint` | yes | The EXACT legacy entry, hashed over its whole content including its reason. |
| `duplicate` | no | Index among entries identical in every field. |
| `reason`, `at`, `by` | yes | The decision, when, and what made it. |
| `disposition` | `reconciled_legacy` only | `recorded` or `dropped`. |
| `source_hash` | `deferred_legacy` only | The version of the legacy file this attempt was made against. |

<!-- /parlay:normative -->
Identity is content, not position: reordering the legacy file must not change
which judgment a disposition answered. Two exemptions may share a ref and
criterion text while recording different judgments for different reasons, so the
reason is part of the fingerprint — keyed without it, answering one marks both
answered and an unreviewed judgment passes as reconciled.

`legacy_file_hash` binds the whole set. If the legacy file changed after these
were written they no longer demonstrably answer what it now contains, and every
disposition is refused rather than silently repointed.

### Deferrals are not answers

`deferred_legacy` records that somebody looked and could not decide. It never
satisfies reconciliation, no matter how many accumulate — the boundary keeps
reporting the entry. Treating uncertainty as a completed outcome would silently
withdraw a possibly load-bearing waiver, which is the failure reconciliation
exists to prevent.

Attempts append rather than replace: two people independently unable to decide
is a different fact from one attempt overwritten twice. An exact repeat — same
occurrence, same source version, same author, same reason — is idempotent, since
a write can succeed while its response is lost and refusing the retry would turn
transport uncertainty into operator repair.

## What this file establishes, and what it does not

It establishes **what was decided, about what, and when the subject last
changed**. Fingerprints and hashes bind each decision to the exact thing it was
about, so a reader can tell whether a decision still concerns what is there now.

It does **not** establish that a person decided. `reason` accepts any non-empty
text and `by` is asserted attribution that nothing verifies. Guidance must not
describe this file as proof a human reviewed anything.

<!-- parlay:normative -->
## Errors

| Code | When |
|---|---|
| unsupported `schema_version` | Version this build does not implement. |
| missing reason | An entry records no `reason`. |
| missing attribution | An entry records no `by` or no `at`. |
| stale ledger | `criteria_hash` does not match the current standard. |
| stranded legacy exemptions | The retired review holds entries nothing has answered. |
| legacy file changed | `legacy_file_hash` no longer matches. |
| duplicate claim | Two waivers on one criterion, or two downgrades on one case. |
<!-- /parlay:normative -->
