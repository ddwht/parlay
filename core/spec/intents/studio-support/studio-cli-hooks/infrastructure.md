# Studio-cli-hooks — Infrastructure

---

## Runtime Studio detection

**Affects**: process startup, runtime context for hook decisions
**Behavior**: At Core process startup, before any command-specific logic runs, look up the `parlay-studio` binary. The lookup prefers the `PARLAY_STUDIO` environment variable when set, falls back to `PATH` lookup when unset. Capture the resolved binary path and reported version into a runtime-context value that subsequent hook code consults. Treat several conditions as "not detected" — absent from `PATH` with no env override; `PARLAY_STUDIO=` set to the empty string (explicit suppression); a found binary whose executable bit is unset. Detection is read-only — never invoke Studio just to confirm it works; presence of the file plus the executable bit is sufficient evidence. Detection failure is silent at the level of normal command output — only `parlay status` (and similar diagnostic commands) surface the not-detected state. The runtime-context value is the single source of truth for "is Studio available right now"; later code reads it instead of re-checking PATH or the environment.
**Invariants**:
- Detection runs exactly once per Core process invocation. Repeated reads of the runtime-context value within the same process do not re-stat the binary or re-read the env var.
- `PARLAY_STUDIO` set to an explicit empty string forces "not detected" regardless of PATH contents.
- `PARLAY_STUDIO` set to a path that is not executable produces "not detected" with the not-executable reason recorded.
- A `parlay-studio` entry on `PATH` whose executable bit is unset produces "not detected" with the not-executable reason recorded; no permission error reaches normal command output.
- When detection is "not detected", no hook prompt, warning, or log line related to Studio appears in any command's output.
**Source**: @studio-support/studio-cli-hooks/detect-parlay-studio-at-runtime
**Caching**: per-process

**Notes**:
- The runtime-context value carries enough information to drive both the `parlay status` surface line (path, version, detected/not-detected) and the hook-point dispatch (boolean detected flag).
- The not-detected reason ("absent", "suppressed by env", "not executable") is preserved in the context so diagnostic surfaces can explain why, but normal commands ignore it and treat all three as the same gate.

---

## Studio version compatibility check

**Affects**: process startup, designer warnings
**Behavior**: When Studio detection succeeds, compare the reported Studio version against the version range Core expects. If the reported version falls outside the expected range, emit one warning line on the diagnostic stream — the FIRST time in the process that detection produces a successful result. Suppress the warning on every subsequent read of the runtime-context value within the same process. The warning is informational; it does NOT change the runtime-context flag (Studio is still considered "detected" for hook purposes). A version mismatch never blocks a hook from firing; it only tells the designer that some hook may misbehave.
**Invariants**:
- A version-mismatched Studio is reported as "detected" in the runtime-context value, not "not detected".
- The version-mismatch warning fires at most once per Core process invocation, regardless of how many hook points fire afterward.
- The warning text identifies the detected version and the expected range. It does not abort the command.
- When the reported version is within the expected range, no version warning is emitted at any point.
**Source**: @studio-support/studio-cli-hooks/detect-parlay-studio-at-runtime
**Caching**: per-process

**Notes**:
- The "expected range" is encoded in Core at compile time. Bumping Core's expected Studio version is a Core release action.
- If the version cannot be obtained from the detected Studio binary (e.g., `parlay-studio --version` fails or returns an unparseable string), treat as detected-but-version-unknown and emit the warning once with text reflecting the parse failure.

---

## TTY interactivity gate for Studio prompts

**Affects**: hook-point dispatch, non-interactive sessions
**Behavior**: Before any Studio prompt fires, check whether the session is interactive — specifically, whether stdin (and conventionally stdout) is attached to a TTY. When not interactive (output piped, input redirected, running under CI, running under any non-terminal launcher), short-circuit before emitting any prompt text. The non-interactive case is one of the three gates on whether a prompt fires; the other two are Studio detection and the `--no-studio` flag. Never write the prompt text to a non-interactive output stream — designers piping a trio command's output into a file or grepping it must not see "Open Studio?" appear in their captured output.
**Invariants**:
- A trio command run with stdin or stdout redirected does not emit any "Open Studio?" prompt text on any stream.
- A trio command run inside CI (no controlling TTY) does not emit any "Open Studio?" prompt text.
- The TTY check produces no false-positive prompts and no false-negative skips when stdin is interactive but stdout is piped, or vice versa — the conservative choice (skip the prompt) wins in any ambiguous case.
- The TTY gate runs before the Studio-detection gate and before the `--no-studio` gate, so non-interactive runs short-circuit fastest.
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points
**Caching**: none

**Notes**:
- Implementation is per-invocation, not per-process — a trio command checks at the moment its main work completes, not at startup.
- Choosing "stdin and stdout both interactive" as the precise condition keeps the gate honest in the common cases (interactive shell vs. piped command) without inventing exotic terminal configurations.

---

## Hook-point dispatch at trio command completion

**Affects**: trio command completion sites, runtime-context-to-prompt wiring
**Behavior**: Each of the three trio commands (`parlay create-domain-model`, `parlay create-artifacts`, `parlay sync`) gains a hook-point dispatch immediately after its main work succeeds and before its final exit. The dispatch consults three gates in order — TTY interactivity, Studio detection, and the `--no-studio` flag — and emits a one-line "Open Studio?" prompt only when all three allow it. The wording of the prompt is supplied by the trio command (different commands ask differently; `create-domain-model` further differentiates between brownfield and greenfield modes), but the dispatch mechanics are shared. On user confirmation, the dispatch hands the runtime-context value plus the active root plus the feature/page workflow context to the Studio subprocess lifecycle (separate fragment). The dispatch implements the always-ask policy: each invocation of each trio command runs the gates fresh — there is no per-session, per-process, or per-config memory of prior y/N answers.
**Invariants**:
- The trio command's main work completes (and its artifact is written) BEFORE the hook prompt fires. A failure of the main work skips the hook entirely.
- Each trio command fires its hook independently. Within a single Core process, accepting or declining one hook does not gate any other hook from firing.
- Within a single shell session (multiple trio command invocations), each invocation re-runs all three gates and emits the prompt fresh — there is no shared memory of prior decisions.
- The hook-point dispatch is the only path that emits the "Open Studio?" prompt text. No other code in Core writes that text directly.
- Commands outside the trio (`parlay add-feature`, `parlay lock-page`, and every other parlay command) do NOT call the hook-point dispatch and never emit a Studio prompt under any conditions, even with Studio detected, TTY, and no `--no-studio` flag.
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points
**Caching**: none

**Notes**:
- The starter set of three commands is intentional. Expanding to additional commands is a future change tracked outside this feature.
- The greenfield-vs-brownfield distinction inside `create-domain-model` is computed by the trio command itself (it knows whether it produced a stub or extracted from signals) and passed to the dispatch as part of the prompt-wording supplier.

---

## Studio subprocess lifecycle

**Affects**: subprocess management, exit-code propagation
**Behavior**: When the designer accepts a Studio prompt, Core invokes the appropriate `parlay-studio` subcommand as a synchronous subprocess. Pass the active root and the feature/page workflow context as arguments to the subprocess so Studio opens against the same project and feature the designer just operated on. Run Studio in the same terminal — Core's process waits for Studio to exit rather than spawning a detached process the designer must track. When Studio exits zero, the trio command also exits zero (success) and returns control to the designer's shell. When Studio exits non-zero or crashes, the trio command surfaces a clear error line that names the Studio subcommand, the exit code, and a reminder that the trio command's prior work is on disk; the trio command itself then exits non-zero so callers (scripts, CI) see the failure. The trio command's prior work — the artifact it produced before the prompt — is NEVER rolled back as a result of Studio's failure.
**Invariants**:
- Studio is invoked as a synchronous subprocess inheriting the parent's stdin, stdout, and stderr — designers see Studio's output directly.
- The active root and feature/page context are passed to Studio explicitly; Studio does not re-resolve from cwd.
- A Studio non-zero exit propagates as the trio command's non-zero exit. The error message names both the Studio subcommand and the exit code.
- Studio's failure does not cause the trio command to delete, truncate, or revert any file the trio command itself wrote before the prompt fired. The artifact remains on disk in whatever state the trio command's main work produced.
- Declining the prompt (no Studio invoked) means the trio command exits with the exit code its main work produced — typically zero.
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points
**Caching**: none

**Notes**:
- The mapping from trio command to Studio subcommand is a small, hard-coded table. The current set: `create-domain-model` → `domain-edit`; `create-artifacts` → the artifact-review subcommand; `sync` → the reconciliation subcommand. Exact subcommand names are finalized by the Studio side of the contract; the hook is robust to a subcommand the version of Studio doesn't recognize (it surfaces as a non-zero exit and the standard failure path).
- "Same terminal" means inheriting the parent's controlling TTY. If Studio launches a UI process (browser, native window) that itself outlives the subprocess, that is Studio's concern, not Core's — Core's wait completes when the subprocess Studio chose to launch exits.

---

## --no-studio flag and config plumbing

**Affects**: trio command flag surface, project config schema
**Behavior**: Each of the three trio commands gains a boolean `--no-studio` flag. The flag is also readable from a project-level config setting (e.g., `parlay.no_studio = true`); the flag and the config combine via OR — either one set silences the prompt for that invocation. The flag's value is consulted by the hook-point dispatch as one of the three gates; when set, dispatch short-circuits before emitting any prompt text. The flag has no effect on commands outside the trio. Adding the flag must update the CLI command registration (per the project's blueprint, `root.go` for Core's CLI library, and the cross-cutting "adding/removing a CLI command" rule that propagates to every deployer's command list).
**Invariants**:
- `parlay create-domain-model --no-studio`, `parlay create-artifacts --no-studio`, and `parlay sync --no-studio` all run their main work and exit without emitting any Studio prompt.
- A project config setting `parlay.no_studio = true` silences the prompt across all three trio commands without requiring the flag at every invocation.
- `--no-studio` is recognized as a flag by argument parsing — passing it to a non-trio command produces a normal "unknown flag" error, not silent acceptance.
- The flag's help text is one line and identifies it as a Studio-prompt opt-out.
- The flag silences the prompt regardless of Studio detection or TTY state — no probe, no log line, just a clean exit at the trio command's normal exit code.
**Source**: @studio-support/studio-cli-hooks/prompt-to-open-studio-at-workflow-hand-off-points
**Backward-Compatible**: yes

**Notes**:
- Adding three flags and one config key is a small change but cross-cuts CLI command registration, generic-deployer hardcoded command surface, and the project's config schema. The build-feature step must surface those touches.
- The flag does not have an inverse `--studio` form. The default is "prompt when allowed"; the flag silences. If a designer wants to override a config that disables prompts, they should change the config — there's no per-invocation re-enable.

---
