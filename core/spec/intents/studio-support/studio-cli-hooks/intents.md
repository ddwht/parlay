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

**Goal**: When the designer reaches one of three specific points in Core's design loop — authoring or refreshing a domain model, reviewing produced artifacts, reconciling spec drift via `parlay sync` — Core offers a one-line prompt to hand off to Studio rather than requiring the designer to remember the right command.
**Persona**: UX Designer
**Priority**: P1
**Context**: Without hooks, designers either know the Studio command incantation by heart or never use Studio. The architecture (§2) commits to "Core prompts 'open editor?'" at appropriate points; this intent decides which points and what the prompts look like.
**Action**: At the completion of each command in the **design-loop trio** — `parlay create-domain-model`, `parlay create-artifacts`, `parlay sync` — check the runtime-detection flag and the workflow context, and emit a one-line "Open Studio? (y/N)" style prompt. On confirmation, Core invokes the appropriate `parlay-studio` subcommand with the active root and feature/page context wired through. On dismissal or non-interactive runs, Core proceeds as today.
**Objects**: hook-point, prompt, parlay-studio-subcommand, workflow-context, design-loop-trio

**Constraints**:
- Hooks are one-line prompts, not multi-step wizards. Designers stay in the terminal flow they were already in
- Hooks fire only when Studio is detected and the session is interactive (TTY attached). Non-interactive runs (CI, scripts) skip prompts
- Each hook respects an opt-out flag (`--no-studio` on the parent command, or a config setting) so designers who never want the prompt can silence it permanently
- Confirmed hook hand-offs run Studio in the same terminal — Core's process waits for Studio to exit, then resumes — rather than spawning a detached process the designer has to track
- If the Studio invocation fails (Studio crashes, exits non-zero), Core surfaces the error without rolling back any prior Core work that was completed before the prompt
- The starter set is exactly the design-loop trio: `parlay create-domain-model`, `parlay create-artifacts`, and after `parlay sync`. `parlay add-feature` and `parlay lock-page` are explicitly excluded from the starter set — `add-feature` is too early in the workflow (no content to edit yet) and `lock-page` is rarely run interactively in the design loop. Expansion to additional commands is deferred to real-workflow evidence
- The `create-domain-model` hook handles two modes against a single command. **Brownfield** (extractable signals or an existing model present): the hook offers Studio to edit the freshly produced model. **Greenfield** (no model and no extractable signals): the command creates an empty stub and the hook is framed as "ready to author — open Studio?" rather than as a soft offer
- Each hook always asks — there is no per-session memory of prior y/N answers. Detection + TTY + `--no-studio` remain the only gates on whether the prompt fires

**Verify**:
- Running `parlay create-domain-model` in **brownfield** mode (extractable signals or existing model) on a project with Studio detected ends with a one-line prompt offering the Domain Model Editor against the produced model; declining proceeds normally
- Running `parlay create-domain-model` in **greenfield** mode (no model, no extractable signals) on a project with Studio detected creates an empty stub and ends with a one-line "ready to author — open Studio?" prompt; declining leaves the empty stub on disk and proceeds normally
- Running `parlay create-artifacts` on a project with Studio detected ends with a one-line prompt offering Studio's artifact editor; declining proceeds normally
- Running `parlay sync` on a project with Studio detected ends with a one-line prompt offering Studio (e.g., to reconcile drift visually); declining proceeds normally
- Running `parlay add-feature` or `parlay lock-page` on a project with Studio detected does NOT prompt — these commands are deliberately excluded from the starter hook set
- The same trio commands on a project without Studio detected do not show any prompt
- Confirming any trio prompt invokes the corresponding `parlay-studio` subcommand against the same active root, with Core waiting for it to exit
- Passing `--no-studio` to any trio command skips the prompt regardless of detection
- Running any trio command in a non-TTY context (piped output, CI) skips the prompt regardless of detection
- Within a single Core process, declining the first hook does not suppress subsequent hooks — each hook fires independently and asks again
- An integration test exercises the actual `parlay-studio` subprocess invocation end-to-end when the binary is on `PATH` (verifying exit-code propagation, the failure-line wording, and the wait-and-resume contract). When the binary is absent — the world we are in until Studio ships — the test skips with a clear "parlay-studio not on PATH" message rather than passing trivially. The skipped count surfaces the still-pending Studio dependency in test output and flips to passing automatically once Studio is installed
- A contract test asserts that the installed `parlay-studio` binary honors each of the three Studio subcommand names that Core hard-codes (`domain-edit` for the create-domain-model hook, `artifacts-review` for the create-artifacts hook, `reconcile` for the sync hook), e.g., by invoking each with `--help` and checking for a non-error exit. The test is **fail-hard red, NOT skipped**, when `parlay-studio` is absent from `PATH` — the absence is itself a contract violation while Studio is still pending. The test is also red when the binary is present but any subcommand is unhonored. The test goes green only when `parlay-studio` is on `PATH` and all three subcommands respond non-error to `--help`. This louder signal is intentional: the subcommand-name table inside Core is the single source of truth today, and a yellow/skipped test would let the contract drift unobserved. Distinguishing this from the skip-until-binary integration test above is deliberate — that test cannot meaningfully run without the binary, so skip is correct; this contract test asserts a fact about the binary's existence, so absence is a fail

**Note**: This intent presupposes a CLI rename from `parlay extract-domain-model` to `parlay create-domain-model` (with the new command spanning both extraction-from-signals and empty-stub creation). The rename itself is out of scope for this feature and is tracked separately. Affected surfaces include other intents under `parlay-tool/domain-model` and `parlay-tool/multi-root`, the `qualified-identifier-resolver` intents, the `studio-support/domain-model-yaml-migration` feature, and the deployer / embedded skills / embedded schemas / CLI command registration that ship the current command name.

---

## Deferred: artifacts-review and reconcile

Two of the three hooks specified above were dropped in 0.2.0. They are recorded here rather than
deleted, because the surfaces they were designed to open are still wanted.

- **`artifacts-review`** — the `create-artifacts` hook (see the Verify bullet on reviewing produced
  artifacts) offered Studio's *artifact editor*: read `surface.yaml` / `capabilities.yaml` /
  `infrastructure.md` visually instead of as YAML.
- **`reconcile`** — the `sync` hook offered Studio *to reconcile drift visually*: resolve an
  intents/dialogs divergence in a diff view instead of by hand.

Neither surface was built. Studio's dispatcher never recognized either subcommand name, so Core's
hook map pointed at commands that did not exist. While unknown arguments fell through to a bare
server boot this was merely wrong — Studio opened on the wrong page. Once unknown commands started
exiting non-zero, accepting either prompt made a successful `parlay create-artifacts` or `parlay sync`
return an error after its work was already on disk.

Both prompts and both map entries are therefore removed. The `create-domain-model` hook, whose
surface exists, is unchanged.

**Re-adding either requires the surface first, not the prompt.** The prompt's stated value in this
feature is saving the designer from remembering a command; an offer with nothing behind it has none of
that value and a real cost. The contract test now derives its subcommand list from Core's map rather
than restating it, so a name re-added here without an implementation fails loudly.
