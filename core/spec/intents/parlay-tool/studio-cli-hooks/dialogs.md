# Studio-cli-hooks — Dialogs

---

### Detect `parlay-studio` at Runtime

**Trigger**: Any `parlay <command>` invocation. Detection runs once during process startup, before any command-specific logic; the result is cached in the resolution context and consulted by later hook points.

User: Runs `parlay status` on a machine where `parlay-studio` is on `PATH`.
System: During startup, looks up `parlay-studio` on `PATH`, finds it, captures the resolved path and version in the resolution context. `parlay status` then reads the cached flag and reports a "Studio detected" line alongside its other status output.

#### Branch: Studio absent from PATH

User: Runs `parlay status` on a machine where `parlay-studio` is not on `PATH`.
System: Lookup returns nothing. The resolution context records "studio: not detected". `parlay status` produces its normal output with no Studio-related line — the absence is silent. Subsequent commands that have Studio hook points see the not-detected flag and behave exactly as they would in a Studio-free world: no prompts, no warnings, no log lines.

#### Branch: `PARLAY_STUDIO` env override points at a custom path

User: Sets `PARLAY_STUDIO=/opt/parlay-studio-dev/parlay-studio` and runs any parlay command.
System: Detection prefers the env var over `PATH` lookup. Stats the path, confirms it is executable, captures it as the detected Studio binary. The path appears in `parlay status` Studio output. Used by CI and by designers running a local-build Studio alongside an installed one.

#### Branch: `PARLAY_STUDIO=` (empty) suppresses detection

User: Sets `PARLAY_STUDIO=` (explicit empty string) and runs any parlay command, even though `parlay-studio` is on `PATH`.
System: Treats the explicit empty value as "force not-detected". Skips `PATH` lookup entirely. Resolution context records "studio: not detected (suppressed by env)". No prompts fire from any hook in this process. Used by CI to simulate a Studio-absent environment on a machine that has Studio installed.

#### Branch: non-executable `parlay-studio` on PATH

User: Has a file named `parlay-studio` on `PATH` that is not executable (e.g., a stale text file, a config artifact, a permission-stripped binary).
System: Lookup finds the entry, stats it, sees the executable bit is unset. Treats this as "not detected" — does not surface a permission error to the user during normal commands. The resolution context records "studio: not detected (found at /path but not executable)" so `parlay status` can surface a one-line diagnostic if the designer asks; everything else proceeds as if Studio were absent.

#### Branch: version mismatch between Core and Studio

User: Has a `parlay-studio` on `PATH` whose version does not match Core's expected Studio version range.
System: Detection succeeds — Studio is recorded as present and reachable. At first detection only (the first command in the process that triggers a successful detect), Core surfaces a single warning line: "warning: parlay-studio version X.Y is older than expected (need >=A.B); some hooks may not work." The warning is suppressed for the rest of the process — subsequent hook points fire prompts without re-emitting the warning, so a designer running stale Studio sees one message, not noise.

#### Branch: detection caching within one process

User: Runs a multi-step parlay command sequence within one process (e.g., a skill that triggers several CLI calls in turn — though typically each `parlay` invocation is a fresh process).
System: Within a single Core process, detection happens exactly once. Subsequent hook-point checks read the cached flag from the resolution context without re-stating PATH or re-checking the env var. Caching scope is the process — a new `parlay` invocation re-detects from scratch.

---

### Prompt to Open Studio at Workflow Hand-Off Points

**Trigger**: A trio command — `parlay create-domain-model`, `parlay create-artifacts`, or `parlay sync` — completes its main work successfully. Before returning, the command consults the cached Studio-detection flag and the session context (TTY, `--no-studio`) to decide whether to emit a one-line "Open Studio?" prompt.

User: Runs `parlay create-domain-model` on a project with feature intents in place, an interactive terminal, Studio detected, and no `--no-studio` flag. The command extracts the model from intents and writes `domain-model.yaml`.
System: Prints the normal post-extraction summary, then on the final line prompts `Open Studio's Domain Model Editor against this model? (y/N) `. The designer types `y`. Core invokes `parlay-studio domain-edit` with the active root wired through, waits for it to exit, then returns control to the original shell with Core's exit code reflecting Studio's exit code (or success, if the designer just exits Studio cleanly).

#### Branch: `parlay create-domain-model` in greenfield mode

User: Runs `parlay create-domain-model` on a project that has no existing model and no extractable signals (intents and dialogs do not name any entities, relationships, or operations the extractor recognizes), with Studio detected and TTY.
System: The command writes an empty stub `domain-model.yaml` (schema-valid, zero entities). Instead of the brownfield "edit this model?" framing, the prompt reads `Empty domain model created — ready to author. Open Studio's Domain Model Editor? (y/N) `. The wording shift signals that the stub is unusable until something fills it in. On `y`, Core invokes `parlay-studio domain-edit` against the stub and waits. On `n`, the empty stub stays on disk and the command returns successfully.

#### Branch: `parlay create-artifacts` happy-path hook

User: Runs `parlay create-artifacts @some-feature` with Studio detected and TTY. The command produces `surface.md` and/or `infrastructure.md` for the feature.
System: After the standard creation summary, prompts `Open Studio to review the produced artifacts? (y/N) `. On `y`, Core invokes the matching `parlay-studio` subcommand for artifact review (e.g., `parlay-studio artifacts-review @some-feature`) against the same active root and waits. On `n`, returns normally.

#### Branch: `parlay sync` happy-path hook

User: Runs `parlay sync @some-feature` with Studio detected and TTY. The command reports coverage and any drift between intents and dialogs.
System: After the sync report, prompts `Open Studio to reconcile this drift visually? (y/N) `. On `y`, Core invokes `parlay-studio reconcile @some-feature` (or the configured Studio sync subcommand) against the same active root and waits. On `n`, returns normally with the textual sync report as the final output.

#### Branch: designer declines a hook prompt

User: Runs any trio command, sees the prompt, types `n` (or just hits Enter — the default is no).
System: Core does not invoke Studio. The command exits normally with whatever exit code its main work would have produced. No state is persisted recording the decline — the next time the same trio command runs in a new process, the prompt fires fresh.

#### Branch: declines do not silence subsequent hooks within one session

User: Within an interactive shell session, runs `parlay create-domain-model` and declines the Studio prompt, then runs `parlay create-artifacts @some-feature` and declines the Studio prompt, then runs `parlay sync @some-feature`.
System: Each command is a separate process, but even if they were in the same process, the always-ask constraint means each hook fires independently. The `parlay sync` invocation prompts the designer normally despite the two prior declines. There is no per-session memory of y/N answers — detection + TTY + `--no-studio` are the only gates.

#### Branch: Studio not detected

User: Runs `parlay create-domain-model` (or any trio command) on a project where Studio is not on `PATH` and no `PARLAY_STUDIO` env var is set.
System: The command produces its normal output and exits without ever showing a Studio prompt. The hook code consults the cached "studio: not detected" flag and short-circuits. From the designer's perspective, behavior is identical to Core in a Studio-free world.

#### Branch: non-TTY context (piped output, CI)

User: Runs `parlay create-domain-model > model.log` (output redirected) or runs the same command inside CI where stdin is not a TTY.
System: The hook detects that stdout (or stdin, depending on implementation) is not attached to a TTY. Skips the prompt entirely — emits no "Open Studio?" line into the log or the CI output stream. The command's normal output is the only thing produced. Behavior is identical regardless of whether Studio is detected; non-interactive sessions are always prompt-free.

#### Branch: `--no-studio` flag opt-out

User: Runs `parlay create-domain-model --no-studio` (or sets `parlay.no_studio = true` in the project's config) on a project where Studio is detected and the session is interactive.
System: The hook checks the flag/config first and short-circuits before any prompt logic. The command produces its normal output and exits without prompting. Useful for designers who never want the prompt, or for running the trio command from inside a script that lives in an interactive terminal but should not pause for input.

#### Branch: excluded commands do not prompt

User: Runs `parlay add-feature my-feature` or `parlay lock-page some-page` on a project with Studio detected, TTY, and no `--no-studio` flag.
System: Neither of these commands has a Studio hook in the starter set. They produce their normal output and exit without any Studio prompt. The exclusion is deliberate — `add-feature` is too early in the workflow (no content yet to review in Studio), and `lock-page` is rarely run interactively in the design loop. Future expansion may add hooks here, but not in this feature.

#### Branch: Studio invocation fails after the designer accepts

User: Runs `parlay create-domain-model`, sees the Studio prompt, types `y`. Core invokes `parlay-studio domain-edit` but Studio crashes or exits non-zero (e.g., binary corrupted, Studio-side bug, port conflict on a Studio-served UI).
System: Core's wait returns with Studio's non-zero exit code. Core surfaces the failure to the designer with a clear message: "parlay-studio domain-edit exited with code N — see Studio's output above. Domain model write completed before Studio launched and is on disk." The prior Core work — writing `domain-model.yaml` — is not rolled back. Core exits non-zero so callers (scripts, CI) see the failure, but the design-loop artifact survives.

---
