# Studio-cli-hooks — Surface

---

## parlay status reports Studio detection

**Shows**: status, message
**Source**: @studio-support/studio-cli-hooks/detect-parlay-studio-at-runtime

**Page**: parlay-status (CLI)
**Region**: main
**Order**: 50

**Notes**:
- When `parlay-studio` is detected (on `PATH` or via `PARLAY_STUDIO` env override), `parlay status` adds a "Studio detected" line to its standard output, alongside the resolved binary path and version.
- When `parlay-studio` is not detected — absent from `PATH`, suppressed by `PARLAY_STUDIO=` (empty), or present but non-executable — `parlay status` produces no Studio-related line. The absence is silent; designers running on a Studio-free machine see no behavior change.
- The non-executable case is the one exception that surfaces a one-line diagnostic when the designer asks (e.g., `parlay status --verbose`): "studio: not detected (found at /path but not executable)" — but the default `parlay status` output stays silent.
- Order 50 places the Studio line after the existing `parlay status` content (root, registered children, feature counts) and before any closing summary; exact ordering is at the adapter's discretion within the `main` region.

---

## Version-mismatch warning at first detection

**Shows**: message
**Source**: @studio-support/studio-cli-hooks/detect-parlay-studio-at-runtime

**Page**: any-parlay-command (CLI)
**Region**: header
**Order**: 10

**Notes**:
- When `parlay-studio` is detected but its version falls outside the range Core expects, a single warning line is emitted on stderr at the first detection in the process: `warning: parlay-studio version X.Y is older than expected (need >=A.B); some hooks may not work.`
- The warning is suppressed for the rest of the process. A designer running stale Studio sees one message per command invocation, not one per hook point.
- The warning is designer-facing diagnostic; it does not gate any hook from firing — version-mismatched Studio is still considered "detected" for prompt purposes.
- Wording should be terse and one-line. The warning is a hint that something may break, not a blocker.

---

## Studio prompt after parlay create-domain-model

**Shows**: message
**Actions**: confirm, dismiss, hand-off
**Flow**: review-and-approve
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points

**Page**: parlay-create-domain-model (CLI)
**Region**: footer
**Order**: 10

**Notes**:
- Fires after `parlay create-domain-model` completes its main work, when Studio is detected, the session is interactive (TTY), and `--no-studio` is not set.
- **Brownfield wording** (the command produced or updated a model from extractable signals): `Open Studio's Domain Model Editor against this model? (y/N) `.
- **Greenfield wording** (the command wrote an empty stub because no signals were found): `Empty domain model created — ready to author. Open Studio's Domain Model Editor? (y/N) `. The wording shift signals that the stub is unusable until something fills it in.
- On `y` (or any case-insensitive yes), Core invokes `parlay-studio domain-edit` against the same active root and waits for it to exit. On `n`, on Enter (default no), or on EOF, Core proceeds normally — the empty stub stays on disk in the greenfield case.
- The prompt is one line. It does not collect rationale, file selection, or any other input — designers stay in the terminal flow they were in.

---

## Studio prompt after parlay create-artifacts

**Shows**: message
**Actions**: confirm, dismiss, hand-off
**Flow**: review-and-approve
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points

**Page**: parlay-create-artifacts (CLI)
**Region**: footer
**Order**: 10

**Notes**:
- Fires after `parlay create-artifacts` completes its main work (producing `surface.md` and/or `infrastructure.md` for the feature), when Studio is detected, the session is interactive, and `--no-studio` is not set.
- Wording: `Open Studio to review the produced artifacts? (y/N) `.
- On `y`, Core invokes the matching `parlay-studio` artifact-review subcommand for the same feature against the same active root and waits. On `n` or default, Core returns normally with the standard creation summary as the final output.

---

## Studio prompt after parlay sync

**Shows**: message
**Actions**: confirm, dismiss, hand-off
**Flow**: review-and-approve
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points

**Page**: parlay-sync (CLI)
**Region**: footer
**Order**: 10

**Notes**:
- Fires after `parlay sync` completes its coverage and drift analysis, when Studio is detected, the session is interactive, and `--no-studio` is not set.
- Wording: `Open Studio to reconcile this drift visually? (y/N) `.
- On `y`, Core invokes the matching `parlay-studio` reconciliation subcommand for the same feature against the same active root and waits. On `n` or default, Core returns normally with the textual sync report as the final output.

---

## --no-studio flag opt-out

**Shows**: data-value
**Actions**: select-toggle
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points

**Page**: parlay-create-domain-model (CLI), parlay-create-artifacts (CLI), parlay-sync (CLI)
**Region**: toolbar
**Order**: 90

**Notes**:
- A boolean flag added to each of the three trio commands: `parlay create-domain-model --no-studio`, `parlay create-artifacts --no-studio`, `parlay sync --no-studio`.
- Equivalent project-level config: `parlay.no_studio = true` (or whatever key the project's config schema uses) silences the prompt for every trio command in projects that opt out permanently.
- When set, the trio command runs to completion as it does today and skips the Studio prompt entirely — no "Open Studio?" line, no waiting on stdin. Useful for designers who never want the prompt, or for invoking a trio command from inside a script that lives in an interactive terminal but should not pause for input.
- The flag's help text should be one line: "skip the Studio open-editor prompt at the end".

---

## Studio launch failure error

**Shows**: status, message
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points

**Page**: parlay-create-domain-model (CLI), parlay-create-artifacts (CLI), parlay-sync (CLI)
**Region**: footer
**Order**: 20

**Notes**:
- When the designer accepts a Studio prompt and Core invokes Studio, but Studio crashes or exits non-zero (binary corrupted, Studio-side bug, port conflict on a Studio-served UI), Core surfaces a clear failure message on stderr after Studio's own output: `parlay-studio <subcommand> exited with code N — see Studio's output above. <Trio-command artifact> completed before Studio launched and is on disk.`
- The trio command's prior work is not rolled back — `domain-model.yaml` (or `surface.md`/`infrastructure.md`, or the sync report) remains on disk. Engineers consuming the failed exit code can re-run Studio later or open the artifact directly.
- Core itself exits non-zero so callers (scripts, CI) see the failure. This is distinct from "Studio not invoked at all" (e.g., declined prompt) where Core exits with the trio command's own exit code.

---
