<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Coverage Review Schema

> **Superseded.** The gate this file describes stops a person at a list of suite
> NAMES, defaulting empty input to yes, and records a reviewer read from the
> environment — which is why real review files in the wild name a background
> process as the approver. It proves someone answered, never that they saw
> anything. What it should have asked about is the criterion set: the standard a
> feature is graded against, produced by rewriting each intent's Verify bullets
> into atomic claims and routing them, after the designer's last look and
> without ever being shown. That is now `criteria-authority.yaml` — see
> `parlay internal criteria-authority`. This document describes the artifact
> during its deprecation window; its exemptions are still read.

<!-- retired: coverage-review-uncovered. The walk behind it had no production
     writer for its input, so the code — a default ERROR, the strictest verdict
     available — could never fire in a real run. Removed with the walk rather
     than left documented, because a documented code that cannot fire is how an
     agent ends up reverse-engineering the validator at runtime. -->

File: `.parlay/build/<feature>/coverage-review.yaml`. Records human approval of a feature's testcases.yaml and gates `parlay generate-code`. The presence and freshness of this file is a precondition for codegen on multi-target projects.

## Structure

```yaml
feature: <feature-slug>
reviewed_at: <RFC3339 timestamp>
reviewed_by: <reviewer identifier — email, login, or "cli">
review_method: <cli | ide | api>
buildfile_hash: <sha256 of buildfile.yaml canonical form>
testcases_hash: <sha256 of testcases.yaml canonical form>
suite_hashes:
  <suite-id>: <sha256 of that suite's canonical form>
  <suite-id>: <sha256 of that suite's canonical form>
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
| `testcases_hash` | Yes | SHA-256 hash over the canonical-form serialization of `testcases.yaml`. Whole-file fallback for reviews without `suite_hashes`. |
| `suite_hashes` | No | Map of suite id (or name, when a suite has no id) to the SHA-256 hash of that suite's canonical form. Present in reviews written by current `parlay review-coverage`; its presence switches testcases staleness from whole-file to per-suite. Absent reviews fall back to `testcases_hash`. |
| `approved_suites` | Yes | List of suite ids the reviewer has approved. Every required suite must appear or be exempted. |
| `exemptions` | No | List of `{ suite, item, reason }` entries — optionally `criterion_text:` — documenting why a required term has no covering case. `item` is the **covered term** — an operation id, an error code, whatever `source_refs:` names — never the suite id. The gate keys on the term, so a suite-keyed entry can never discharge anything. |
| `exemptions[].criterion_text` | No | Narrows an exemption to a **single `verify:` bullet** on `item`, matched against the contract's own wording (trimmed; not otherwise normalized). Omitted, the exemption is **entry-wide** and excuses every bullet on `item` — which is how every exemption written before criterion coverage became bullet-granular has to be read, since none of them could have recorded a text. Prefer the narrow form when writing new exemptions: an entry-wide exemption on a five-bullet operation excuses four criteria nobody reviewed. |

## Versioning

No `schema_version:` field (see `schema-versioning.schema.md` for the house rule) — this artifact's freshness isn't governed by its own version timeline at all. It's gated by `buildfile_hash`/`testcases_hash` matching the CURRENT canonical-form hash of those two files; a shape change to `coverage-review.yaml` itself would be caught by `parlay review-coverage` simply re-emitting the current shape on every run (it's tool-generated, never hand-edited). Same reasoning as `adapter-set.schema.md`: shape pinned by what it gates, not an independent version.

## Canonical-form hashing

Hashes are computed over a canonical-form serialization (sorted map keys, normalized whitespace, stable list ordering where the schema permits it) so that cosmetic edits to the source files do not invalidate the review. Editing `buildfile.yaml` to add or remove a binding changes the hash; reordering wiring rules without semantic impact does not.

## Gate behavior

`parlay generate-code` reads this file before any other input. Failures surface as:

| Code | When it fires |
|---|---|
| `coverage-review-missing` | The file does not exist. Codegen refuses to start. |
| `coverage-review-stale` | `buildfile_hash` differs from the current canonical-form hash, or — for a review without `suite_hashes` — `testcases_hash` differs. Names the drifted hash. |
| `coverage-review-suite-stale` | A review with `suite_hashes` approved a suite whose canonical form has since changed. Fires per drifted suite, naming it, so only those need re-review; unchanged approved suites stay valid. |
| `coverage-review-suite-unapproved` | A suite present in `testcases.yaml` is absent from `approved_suites:` and has no exemption. |

The review is recorded by `parlay review-coverage <feature>`.

## Recording exemptions

Interactively, declining a suite prompts for a reason and records one exemption **per term that suite covered** (its `source_refs:`), because that is what the gate consults. A suite with no `source_refs:` falls back to being keyed on its own name — the only term available.

Non-interactively, `--exempt <suite>:<item>=<reason>` pre-records one, and is repeatable. **It always records an entry-wide exemption** — it has no `criterion_text:` form, and so is strictly broader than an exemption recorded interactively against one bullet. The grammar is the reason: it splits on the first `=` and the first `:` so a reason may contain either, and threading a third free-text field through that buys ambiguity rather than precision. Automation authors should not assume the flag and the interactive flow grant equally narrow exemptions — to exempt one bullet, write the entry into the file with `criterion_text:`. A suite whose every covered term is exempted this way is not prompted for; a suite with only *some* terms exempted still is, since the rest remain undecided. Parsing splits on the first `=` and the first `:` before it, so a reason may contain either character — `report-suite:@f/operation:submit=covered by the engine: see ADR-4` records the item as `@f/operation:submit` and keeps the reason whole.

## Backward compatibility

Presentation-only projects (no non-presentation slot in `.parlay/adapter-set.yaml`) skip the gate entirely — `parlay generate-code` does not require `coverage-review.yaml` there. Once a project transitions to multi-target mode, the gate activates on the next `parlay generate-code` invocation.
