# Studio Editor Hooks in Core CLI

> Add hook points where Core's CLI offers to open Parlay Studio for editing — the Domain Model Editor or the Design Loop, depending on the workflow stage. Detection of `parlay-studio` is dynamic: projects without Studio installed see no behavior change; projects with Studio installed get a one-line prompt at the right moments. This is the "designer hands off to Studio, then back to Core" handshake.

---

## Detect `parlay-studio` at Runtime

**Goal**: Discover whether a `parlay-studio` binary is available and invokable from Core's process, so Core can decide whether to surface Studio prompts at all.
**Persona**: UX Designer
**Priority**: P0
**Context**: Core ships independently; Studio is a separate binary that may or may not be present. Core must not hard-depend on Studio (per P1) but should offer Studio's capabilities when they are reachable.
**Action**: At Core invocation, look up `parlay-studio` on `PATH` and capture the result in the resolution context. Subsequent commands that have Studio hook points consult this flag. Do not invoke Studio just to detect it — presence on `PATH` is sufficient evidence.
**Objects**: parlay-studio, runtime-detection, hook-point

**Constraints**:
- Detection is read-only — Core does not invoke Studio just to see if it works
- Detection happens once per Core invocation and is cached for the duration of the process
- Detection failure is silent — if Studio is not on `PATH`, Core's subsequent behavior is exactly what it would be without Studio installed
- Detection is overridable for testing via an env var (e.g., `PARLAY_STUDIO=/path/to/binary`) so CI can simulate "Studio installed" or "Studio absent"
- A version mismatch between Core and Studio surfaces a single warning at first detection, not at every hook point — so a designer running stale Studio sees one message, not noise

**Verify**:
- On a machine with `parlay-studio` on `PATH`, `parlay status` reports Studio as detected
- On a machine without `parlay-studio` on `PATH`, no Studio-related output appears anywhere in normal commands
- Setting `PARLAY_STUDIO=` (empty) suppresses detection even when the binary is on `PATH`
- A non-executable `parlay-studio` on `PATH` is treated as "not detected" rather than producing a permission error during normal commands

---

## Prompt to Open Studio at Workflow Hand-Off Points

**Goal**: When the designer reaches a point in Core's workflow where Studio's tools are appropriate — reviewing a domain model, locking down a page's layout — Core offers a one-line prompt to hand off to Studio rather than requiring the designer to remember the right command.
**Persona**: UX Designer
**Priority**: P1
**Context**: Without hooks, designers either know the Studio command incantation by heart or never use Studio. The architecture (§2) commits to "Core prompts 'open editor?'" at appropriate points; this intent decides which points and what the prompts look like.
**Action**: At specific Core CLI commands' completion, check the runtime-detection flag and the workflow context, and emit a one-line "Open Studio? (y/N)" style prompt. On confirmation, Core invokes the appropriate `parlay-studio` subcommand with the active root and feature/page context wired through. On dismissal or non-interactive runs, Core proceeds as today.
**Objects**: hook-point, prompt, parlay-studio-subcommand, workflow-context

**Constraints**:
- Hooks are one-line prompts, not multi-step wizards. Designers stay in the terminal flow they were already in
- Hooks fire only when Studio is detected and the session is interactive (TTY attached). Non-interactive runs (CI, scripts) skip prompts
- Each hook respects an opt-out flag (`--no-studio` on the parent command, or a config setting) so designers who never want the prompt can silence it permanently
- Confirmed hook hand-offs run Studio in the same terminal — Core's process waits for Studio to exit, then resumes — rather than spawning a detached process the designer has to track
- If the Studio invocation fails (Studio crashes, exits non-zero), Core surfaces the error without rolling back any prior Core work that was completed before the prompt
- Hook trigger points are conservative initially — start with the points where the architecture explicitly calls them out (§2) and expand based on real workflows, not speculation

**Verify**:
- Running `parlay create-domain-model` (or whatever the relevant command is) on a project with Studio detected ends with a one-line prompt offering the Domain Model Editor; declining proceeds normally
- The same command on a project without Studio detected does not show the prompt
- Confirming the prompt invokes `parlay-studio domain-edit` against the same active root, with Core waiting for it to exit
- Passing `--no-studio` to the parent command skips the prompt regardless of detection
- Running the parent command in a non-TTY context (piped output, CI) skips the prompt regardless of detection

**Questions**:
- Q5 from the v4 spec: which Core commands get hooks? Candidates: `parlay add-feature`, `parlay extract-domain-model`, `parlay create-artifacts`, after `parlay sync`. Decide the starter set during dialog authoring; expand later
- Should the prompt remember the designer's prior answer for a session and stop asking, or always ask? Always-ask is simplest; remember-for-session is friendlier. Pick during dialog authoring

---
