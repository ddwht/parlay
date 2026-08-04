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
| `exemptions` | No | List of `{ suite, item, reason }` entries documenting why a required term has no covering case. `item` is the **covered term** — an operation id, an error code, whatever `source_refs:` names — never the suite id. The gate keys on the term, so a suite-keyed entry can never discharge anything. |

## Versioning

No `schema_version:` field (see `schema-versioning.schema.md` for the house rule) — this artifact's freshness isn't governed by its own version timeline at all. It's gated by `buildfile_hash`/`testcases_hash` matching the CURRENT canonical-form hash of those two files; a shape change to `coverage-review.yaml` itself would be caught by `parlay review-coverage` simply re-emitting the current shape on every run (it's tool-generated, never hand-edited). Same reasoning as `adapter-set.schema.md`: shape pinned by what it gates, not an independent version.

## Canonical-form hashing

Hashes are computed over a canonical-form serialization (sorted map keys, normalized whitespace, stable list ordering where the schema permits it) so that cosmetic edits to the source files do not invalidate the review. Editing `buildfile.yaml` to add or remove a binding changes the hash; reordering wiring rules without semantic impact does not.

## Gate behavior

`parlay generate-code` reads this file before any other input. Failures surface as:

| Code | When it fires |
|---|---|
| `coverage-review-missing` | The file does not exist. Codegen refuses to start. |
| `coverage-review-stale` | `buildfile_hash` or `testcases_hash` differs from the current canonical-form hash. Names the drifted hash. |
| `coverage-review-suite-unapproved` | A suite present in `testcases.yaml` is absent from `approved_suites:` and has no exemption. |
| `coverage-review-uncovered` | A canonical-form-required term (declared error, declared operation) lacks both a covering testcase and an explicit exemption. |

The review is recorded by `parlay review-coverage <feature>`.

## Recording exemptions

Interactively, declining a suite prompts for a reason and records one exemption **per term that suite covered** (its `source_refs:`), because that is what the gate consults. A legacy v1 suite carries no `source_refs:` and falls back to being keyed on its own name — the only term available.

Non-interactively, `--exempt <suite>:<item>=<reason>` pre-records one, and is repeatable. A suite whose every covered term is exempted this way is not prompted for; a suite with only *some* terms exempted still is, since the rest remain undecided. Parsing splits on the first `=` and the first `:` before it, so a reason may contain either character — `report-suite:@f/operation:submit=covered by the engine: see ADR-4` records the item as `@f/operation:submit` and keeps the reason whole.

## Backward compatibility

Presentation-only projects (no non-presentation slot in `.parlay/adapter-set.yaml`) skip the gate entirely — `parlay generate-code` does not require `coverage-review.yaml` for legacy v1 testcases. Once a project transitions to multi-target mode, the gate activates on the next `parlay generate-code` invocation.
