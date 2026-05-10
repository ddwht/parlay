<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Coverage Review Schema

File: `.parlay/build/<feature>/coverage-review.yaml`. Records human approval of a feature's testcases.yaml and gates `parlay generate-code`. The presence and freshness of this file is a precondition for codegen on multi-target projects.

## Structure

```yaml
feature: <feature-slug>
reviewed_at: <RFC3339 timestamp>
reviewed_by: <reviewer identifier — email, login, or "cli">
review_method: <cli | ide | api>
buildfile_hash: <sha256 of buildfile.yaml canonical form>
testcases_hash: <sha256 of testcases.yaml canonical form>
approved_suites:
  - <suite-id>
  - <suite-id>
exemptions:
  - suite: <suite-id>
    item: <covered term — operation id, error code, etc.>
    reason: <free-text justification>
```

| Field | Required | Description |
|---|---|---|
| `feature` | Yes | Feature slug; must match the directory. |
| `reviewed_at` | Yes | RFC3339 UTC timestamp of when the review was recorded. |
| `reviewed_by` | Yes | Reviewer identifier. |
| `review_method` | Yes | `cli`, `ide`, or `api`. |
| `buildfile_hash` | Yes | SHA-256 hash over the canonical-form serialization of `buildfile.yaml`. |
| `testcases_hash` | Yes | SHA-256 hash over the canonical-form serialization of `testcases.yaml`. |
| `approved_suites` | Yes | List of suite ids the reviewer has approved. Every required suite must appear or be exempted. |
| `exemptions` | No | List of `{ suite, item, reason }` entries documenting why a required term has no covering case. |

## Canonical-form hashing

Hashes are computed by `internal/agent/coverage_hash.go` over a canonical-form serialization (sorted map keys, normalized whitespace, stable list ordering where the schema permits it) so that cosmetic edits to the source files do not invalidate the review. Editing `buildfile.yaml` to add or remove a binding changes the hash; reordering wiring rules without semantic impact does not.

## Gate behavior

`parlay generate-code` reads this file before any other input. Failures surface via `internal/agent/validate_coverage_review.go`:

| Code | When it fires |
|---|---|
| `coverage-review-missing` | The file does not exist. Codegen refuses to start. |
| `coverage-review-stale` | `buildfile_hash` or `testcases_hash` differs from the current canonical-form hash. Names the drifted hash. |
| `coverage-review-suite-unapproved` | A suite present in `testcases.yaml` is absent from `approved_suites:` and has no exemption. |
| `coverage-review-uncovered` | A canonical-form-required term (declared error, declared operation) lacks both a covering testcase and an explicit exemption. |

The review is recorded by `parlay review-coverage <feature>`.

## Backward compatibility

Presentation-only projects (no non-presentation slot in `.parlay/adapter-set.yaml`) skip the gate entirely — `parlay generate-code` does not require `coverage-review.yaml` for legacy v1 testcases. Once a project transitions to multi-target mode, the gate activates on the next `parlay generate-code` invocation.
