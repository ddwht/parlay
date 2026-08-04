# The feedback log format

Repo-only documentation. Deliberately **not** in `core/internal/embedded/schemas/`:
that directory is deployed into every user project and `DIGEST.md` routes agents
into it when they are authoring an artifact. A telemetry wire format is not
something an agent writing a `capabilities.yaml` should ever be routed to.

## What this is for

Parlay's most expensive failures are not crashes, they are agents working around
it — authoring a shape, having it rejected, retrying a different one. None of
that was ever written down, so a schema that teaches by rejection looked exactly
like one that teaches by documentation.

The mode is **off by default**. A user turns it on with `feedback: true` in
`.parlay/config.yaml` (or `PARLAY_FEEDBACK=1` for one run), reproduces a problem,
runs `parlay feedback-export`, and sends the bundle. Parlay never transmits
anything.

## The safety property

**Nothing sensitive is captured, so nothing sensitive can leak.** Sanitising at
capture rather than at export means the on-disk log is already safe: a user can
read it, commit it, or send it without review, and a mistake cannot be undone by
a filter that missed a case.

That is enforced in three layers:

1. **Closed per-kind payloads** (`FindingData`, `TallyData`, `SessionData`,
   `AgentData` in `core/internal/feedback/feedback.go`). Free text has no field
   to live in. Adding one means editing that file, which is the review point.
2. **A validating encoder at the single write point.** Every value must match
   `^[a-z0-9][a-z0-9._\- ]{0,63}$` or be a known vocabulary member; anything else
   becomes the literal `redacted`. A sentinel rather than a drop, so the log
   records *that* something was rejected — that entry is a bug report about the
   recorder.
3. **Tests.** `TestNothingSensitiveReachesTheLog` pushes paths, emails,
   sentences and YAML fragments through every producer and asserts no 8-character
   substring survives into the written bytes. `TestFeedbackPayloadsCarryNoFreeText`
   walks the AST of the producing packages and rejects payload fields assigned
   from `Sprintf`, `err.Error()`, `filepath.Join` and friends.

### What is deliberately not captured

Validator **messages**, `Context`, `Fix`, argv, and error strings. The message is
rendered from a template this repo already owns, so the only new information in
it is the interpolated values — and those are the user's data: roughly one
message in five carries a filesystem path, ~171 lines quote user identifiers,
and a handful quote spec prose verbatim. The `code` plus `site` answer the same
question without any of it.

## Event shape

One JSON object per line. Every event carries:

| Field | Meaning |
|---|---|
| `v` | Schema version, currently `2`. Export refuses anything lower. |
| `at` | RFC3339Nano UTC. |
| `run` | Pipeline-run correlation id, **hashed**. Set from `PARLAY_RUN_ID`. |
| `proc` | Per-process id. Present on every line — see *Denominators*. |
| `kind` | One of the kinds below. |
| `data` | Kind-specific, all values from closed vocabularies or hashed. |

### Kinds

| Kind | Written by | `data` |
|---|---|---|
| `finding` | CLI | `code`, `mode`, `severity`, `site`, and optionally `phase`, `artifact`, `subject` |
| `tally` | CLI, at process exit | `command`, `exit`, `ms_bucket`, `findings`, `completed` |
| `session` | CLI, once per day file | `version`, `os`, `arch`, `multi_root`, `features`, `adapters`, `interactive` |
| `phase`, `decision`, `retry`, `improvised`, `note` | Agent, via `parlay internal feedback-record` | closed enums only |

An agent may not emit `finding`, `tally` or `session` — asserting one would put a
fact in the log that nothing observed.

`site` is the emitting function's symbol (`agent.validatesupportsblock`), from
`runtime.FuncForPC`. It exists because several codes fire from more than one
branch — `validate_adapter.go` uses one `add(code, msg)` closure across ~20
conditions — so a code alone cannot tell an investigator which branch fired. The
symbol only; never `runtime.Caller`'s file, which under `go run` is a path inside
the user's tree.

## Denominators

A finding count is uninterpretable alone. `authored-field-missing` forty times
means "this schema teaches by rejection" against 45 runs and "someone typo'd"
against 4,000.

Rates come from the `tally`, one per process, summed at read time. There is no
shared mutable counter: that would need locking, and locking adds a hang risk to
a subsystem whose whole principle is *never break a command*. Appending is safe
without it, because a single small write to an `O_APPEND` regular file allocates
its offset atomically — which is why JSONL is the container.

**Read denominators as distinct `proc` values, not as tally count.** A process
killed mid-run writes its findings but never its tally, and failing runs are the
likeliest to be killed — counting tallies alone therefore biases
findings-per-run upward. Distinct procs with no tally are incomplete runs, which
is itself a signal worth having.

Known gap: `init` and `version` skip root resolution and so are never recorded.

## Identifiers

Feature, unit and operation names are hashed with a per-project salt at
`.parlay/feedback/.salt` (16 random bytes, mode 0600, created on first enable).

Salted rather than a bare digest because an unsalted SHA-256 of `checkout` is the
same everywhere and falls to a dictionary immediately. Stable within a project so
"the same subject failed four times" survives; meaningless across projects.

The only analytic loss is cross-project identifier joins — whether `checkout` is
a globally troublesome name — which is worth approximately nothing, because these
diagnostics are about artifact *shape*, not which noun someone picked.

`feedback-export` replaces each distinct hash with `subject-1`, `subject-2`. That
is readability, not extra safety: an ordinal reads better in an investigation and
has no relationship to the input at all. **The salt is never exported.**

## Retention and containment

- One file per day, `.parlay/feedback/<YYYY-MM-DD>.jsonl`, mode 0600. Patterns
  worth seeing — the same code four times, a phase that always retries — span
  runs, so the day is the right grain.
- Files older than 14 days are pruned on the next enabled invocation.
- `.parlay/feedback/.gitignore` is written on first enable, and by `upgrade` for
  projects that predate it. `.parlay/` is version-controlled by convention, so
  without it a user would commit their log and push it.

## Migration from v0.2.3

The first release captured argv, error strings and full validator messages. Those
logs are `v` 1 or unversioned.

- `feedback-export` **refuses** any event below the current version. Old logs are
  mechanically un-sendable, not merely discouraged.
- `parlay upgrade` prints a notice naming the count and does **not** delete
  anything. They are the user's files.
- `parlay feedback-prune --legacy` removes them when the user chooses.
