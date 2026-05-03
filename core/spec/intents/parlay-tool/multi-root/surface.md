# Multi-root — Surface

---

## Root Resolution Verbose Output

**Shows**: data-value, message
**Actions**: inspect
**Source**: @parlay-tool/multi-root/discover-active-root-via-cwd-walk-up

**Page**: parlay-cli
**Region**: header
**Order**: 1

**Notes**:
- Printed only when `--verbose` is passed; ordinary invocations do not show this fragment.
- `data-value` is the resolved absolute root path.
- `message` names the resolution source — `cwd walk-up`, `PARLAY_ROOT`, `--root flag`, or `disambiguation prompt`.
- One line, before any other command output.

---

## Root Disambiguation Prompt

**Shows**: message, data-list
**Actions**: select-one, dismiss
**Flow**: guided-flow
**Source**: @parlay-tool/multi-root/discover-active-root-via-cwd-walk-up, @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd

**Page**: parlay-cli
**Region**: main
**Order**: 2

**Notes**:
- Triggered when walk-up resolution is ambiguous (no `.parlay/` at cwd but candidate roots exist), OR a bare feature reference does not match the active root and matches more than one other root.
- `data-list` enumerates each candidate root as `<root-name> (<relative-path-from-repo>)`.
- `select-one` lets the user pick which root to operate on; `dismiss` cancels the command.
- Interactive contexts only. Non-interactive runs print the No-Root-Found Error or Ambiguous-Reference Error instead.

---

## No-Root-Found Error

**Shows**: status, message
**Source**: @parlay-tool/multi-root/discover-active-root-via-cwd-walk-up

**Page**: parlay-cli
**Region**: main
**Order**: 3

**Notes**:
- `status` is `error`.
- `message` distinguishes the failure mode: walk-up reached `.git/` (suggest `parlay init` or `parlay add-root`), walk-up reached filesystem boundary, or `PARLAY_ROOT` was set but invalid.
- Shown both in interactive contexts (when no candidate roots are available for disambiguation) and in non-interactive contexts (when disambiguation is unavailable).
- In non-interactive contexts with candidate roots, the message also lists known root names and suggests `--root` or `PARLAY_ROOT`.

---

## Add-Child-Root Result

**Shows**: status, message, data-list
**Actions**: invoke
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project, @parlay-tool/multi-root/single-repo-level-agent-surface-drives-all-roots

**Page**: parlay-cli
**Region**: main
**Order**: 4

**Notes**:
- `invoke` is the command itself: `parlay add-root <subdir>`.
- On success: `status` is `success`; `message` confirms creation and registration; `data-list` shows the path of the new child root, the parent's roots index entry, and the agent-surface refresh result.
- On refusal: `status` is `error`; `message` names the refusal reason — subdir already has `.parlay/`, cwd is not a parlay root, or subdir is nested inside another child root.
- Includes the orphan-parent error case: when a child's recorded parent path no longer resolves, every command run against the child shows the loud "parent root not found" message with the suggested fix (restore parent or run `parlay promote-root`).

---

## Resource Source Listing

**Shows**: data-table
**Actions**: inspect
**Source**: @parlay-tool/multi-root/inherit-resources-from-parent-root

**Page**: parlay-cli
**Region**: main
**Order**: 5

**Notes**:
- Printed only when `--verbose` is passed and a command actually loads resources (build-feature, generate-code, sync, etc.).
- `data-table` rows: resource type (schemas, adapter, deployed skills, domain model), resolved source root, override status (override / fallback / root-only).
- One row per loaded resource. Read-only — no actions on the table.

---

## Forbidden-Directory Error

**Shows**: status, message
**Source**: @parlay-tool/multi-root/inherit-resources-from-parent-root, @parlay-tool/multi-root/single-repo-level-agent-surface-drives-all-roots

**Page**: parlay-cli
**Region**: main
**Order**: 6

**Notes**:
- `status` is `error`.
- `message` names the forbidden directory and the rule it violates: `.parlay/schemas/` in a child root (schemas live at parent only) or a deployed-agent-surface directory in a child root (`.claude/`, `.cursor/`, etc.) — agent surface lives at parent only.
- Refuses to run the command; suggests removing the directory.

---

## Cross-Root Targeting Announcement

**Shows**: data-value, message
**Source**: @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd

**Page**: parlay-cli
**Region**: header
**Order**: 7

**Notes**:
- Printed before command work starts whenever the active root is not the cwd-resolved default — i.e. a root prefix on a feature reference, a `--root` flag, an ambiguous bare reference auto-resolved to a single match, or a disambiguation choice.
- `data-value` is the chosen root name.
- `message` names the trigger: `prefix`, `--root flag`, `auto-resolved (only match)`, or `disambiguated`.
- Distinct from the verbose Root Resolution Output (which appears only with `--verbose`); this announcement is shown in normal output to guarantee the user sees which root was chosen when it differs from cwd.

---

## Ambiguous-Reference Disambiguation Prompt

**Shows**: message, data-list
**Actions**: select-one, dismiss
**Flow**: guided-flow
**Source**: @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd

**Page**: parlay-cli
**Region**: main
**Order**: 8

**Notes**:
- Triggered when a bare feature reference does not match the active root and matches in two or more child roots.
- `data-list` enumerates candidates as `<root-name> (<relative-path>)`.
- Behaviorally similar to the Root Disambiguation Prompt but scoped to a feature-reference search rather than active-root selection. Same vocabulary; different trigger.
- Non-interactive runs replace this fragment with the Ambiguous-Reference Error (status + message naming the candidates and suggesting `prefix` or `--root`).

---

## Cross-Root Reference Validation Error

**Shows**: status, message
**Source**: @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd

**Page**: parlay-cli
**Region**: main
**Order**: 9

**Notes**:
- `status` is `error`.
- Triggered when an intent body or other authored content contains a root-prefixed reference (`web:@some-feature/some-intent`).
- `message` names the file path and line number, plus the rule (cross-root references in authored content are out of scope for v1).

---

## Upgrade Refresh Confirmation

**Shows**: status, summary, data-list
**Actions**: invoke
**Source**: @parlay-tool/multi-root/single-repo-level-agent-surface-drives-all-roots

**Page**: parlay-cli
**Region**: main
**Order**: 10

**Notes**:
- `invoke` is `parlay upgrade` (or the auto-trigger from `parlay add-root` / `parlay remove-root`).
- `status` is `success` when the deployer refresh completes.
- `summary` lists counts: skills deployed, schemas updated, roots listed.
- `data-list` enumerates the registered roots written into the agent-rules file (parent + child roots).
- Refusal case: when invoked from inside a child root, replaces this fragment with No-Root-Found Error variant ("run upgrade from the repo-level root, not from a child").

---

## Status with Bare Parent

**Shows**: data-tree, summary
**Actions**: inspect
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project, @parlay-tool/multi-root/inherit-resources-from-parent-root

**Page**: parlay-cli
**Region**: main
**Order**: 11

**Notes**:
- Output of `parlay status` at any root, expressed as a tree.
- Top node is the resolved active root; child nodes list features (if any) and registered child roots.
- A parent with empty `spec/intents/` shows zero parent features and the registered children — this is the normal "bare-parent" topology and must NOT show an error or warning.
- `summary` reports total feature count across all visible roots from the current vantage point (active root only — child-root features are not aggregated into the parent's count, per the root-scoping rule).

---

## Skill-Side Root Disambiguation Prompt

**Shows**: message, data-list
**Actions**: select-one, dismiss
**Flow**: guided-flow
**Source**: @parlay-tool/multi-root/skill-invocation-in-a-multi-root-project

**Page**: parlay-skill
**Region**: main
**Order**: 1

**Notes**:
- The skill-side counterpart to the CLI Root Disambiguation Prompt and the CLI Ambiguous-Reference Disambiguation Prompt.
- Rendered through the agent's native question mechanism (e.g. AskUserQuestion on Claude Code) — NOT through stdin/stdout.
- Triggered when the wrapped CLI invocation returns an ambiguous-reference or ambiguous-active-root structured signal; the skill catches the signal, re-prompts via the agent's mechanism, and re-invokes the CLI with the user's choice.
- `dismiss` cancels the skill cleanly without leaving partial state.
- Identical UX across deployed adapters — the skill source is one file, deployed by every adapter's deployer.

---

## Skill-Side Operating-Root Announcement

**Shows**: data-value, message
**Source**: @parlay-tool/multi-root/skill-invocation-in-a-multi-root-project, @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd

**Page**: parlay-skill
**Region**: header
**Order**: 2

**Notes**:
- Visible response prefix written by the skill whenever the resolved root differs from the cwd-default — prefix on a feature reference, `--root` flag, single-match auto-resolution, or disambiguation choice.
- One line, before any other skill output: `operating on root: <name> (<source>)` where source is one of `prefix`, `--root flag`, `auto-resolved (only match)`, `disambiguated`.
- Mirrors the CLI Cross-Root Targeting Announcement; both must agree on naming so users see the same message regardless of invocation path.

---

## Skill-Side Forbidden-Directory Guidance

**Shows**: status, message
**Actions**: confirm, dismiss
**Source**: @parlay-tool/multi-root/skill-invocation-in-a-multi-root-project, @parlay-tool/multi-root/inherit-resources-from-parent-root

**Page**: parlay-skill
**Region**: main
**Order**: 3

**Notes**:
- Triggered when the wrapped CLI returns a forbidden-directory error (`.parlay/schemas/` in a child, or a deployed-agent-surface directory in a child).
- `status` is `error`.
- `message` translates the raw CLI error into actionable guidance with the exact path to remove.
- On agents that support tool execution, the skill MAY offer `confirm` (delete the directory now) or `dismiss` (let the user handle it). On agents without tool execution, only the message is shown.
- The CLI's bare error is preserved in skill verbose mode for debugging.

---

## Agent-At-Child Config Error

**Shows**: status, message
**Source**: @parlay-tool/multi-root/agent-identity-lives-at-the-parent-in-multi-root-projects

**Page**: parlay-cli
**Region**: main
**Order**: 12

**Notes**:
- Emitted at config-load time on any command that loads a multi-root project's configs when a child `config.yaml` declares `ai-agent`.
- `status` is `error`.
- `message` is exactly: `agent identity belongs at the parent root; remove ai-agent from <child>/.parlay/config.yaml`. The path is concrete (the offending child file), not abstract.
- Refuses to proceed; no partial work, no warning-then-continue path.

---

## Both-Have-Agent Config Error

**Shows**: status, message, data-list
**Source**: @parlay-tool/multi-root/agent-identity-lives-at-the-parent-in-multi-root-projects

**Page**: parlay-cli
**Region**: main
**Order**: 13

**Notes**:
- Emitted at config-load time when both the parent and at least one child declare `ai-agent`, regardless of whether the values agree.
- `status` is `error`.
- `data-list` enumerates each declaring file with its `ai-agent` value: `<file-path> (ai-agent: <value>)`.
- `message` distinguishes the two sub-cases: matching values say "agent identity declared at multiple levels"; conflicting values add "with conflicting values". Both variants point the user at `parlay repair` for the migration.
- Refuses to proceed even when the values agree — silent preference would let the inconsistency persist.

---

## Missing-Agent-On-Upgrade Error

**Shows**: status, message
**Source**: @parlay-tool/multi-root/agent-identity-lives-at-the-parent-in-multi-root-projects, @parlay-tool/multi-root/parlay-upgrade-errors-on-bare-parent-topology

**Page**: parlay-cli
**Region**: main
**Order**: 14

**Notes**:
- Emitted by `parlay upgrade` when the parent `config.yaml` exists but is missing the `ai-agent` field (and no walk-up is permitted).
- `status` is `error`.
- `message` names the offending file path and lists the valid `ai-agent` values, then suggests `parlay repair` as the alternative migration path.
- Distinct from the bare-parent error (no `config.yaml` at all) and from the agent-at-child error (child holds the field).
- Atomic: nothing is deployed, no schemas updated, no skills updated.

---

## Verbose Field-Resolution Listing

**Shows**: data-table
**Actions**: inspect
**Source**: @parlay-tool/multi-root/agent-identity-lives-at-the-parent-in-multi-root-projects

**Page**: parlay-cli
**Region**: main
**Order**: 15

**Notes**:
- Printed only when `--verbose` is passed and the command loaded the project config (`status`, `upgrade`, `build-feature`, etc.).
- Extends the existing Resource Source Listing with one row per effective config field (`ai-agent`, `sdd-framework`, `prototype-framework`).
- Each row records the field name, resolved value, source file path, and origin label — `from <file>` for direct declaration or `inherited from <file>` when a child silently inherited from the parent.
- Read-only; no actions on the table.

---

## Init Agent-Identity Prompt

**Shows**: message, data-value
**Actions**: provide-text, confirm
**Flow**: onboarding
**Source**: @parlay-tool/multi-root/parlay-init-writes-the-correct-topology-shape

**Page**: parlay-cli
**Region**: main
**Order**: 16

**Notes**:
- Triggered by `parlay init` when writing the parent (or single-root) config and `ai-agent` is not yet recorded.
- `data-value` is the detected agent name when running through a known agent (Claude Code, Cursor, Generic CLI); the prompt surfaces it as the default with `[<detected> (detected)]`.
- `provide-text` accepts a different value to override; `confirm` (Enter) accepts the default.
- Init never proceeds without an explicit user choice — even with a detected default, the user must press Enter or type an alternative.
- When invoked on a fully-configured project, init exits with "Project already initialized" and does NOT re-show this prompt.

---

## Add-Root Refusal Without Parent Agent

**Shows**: status, message, data-value
**Source**: @parlay-tool/multi-root/parlay-init-writes-the-correct-topology-shape

**Page**: parlay-cli
**Region**: main
**Order**: 17

**Notes**:
- Triggered when `parlay add-root <child>` is invoked but the parent's `config.yaml` is missing `ai-agent` (or the parent has no `config.yaml` at all).
- `status` is `error`.
- `message` is exactly: `parent is missing ai-agent — run \`parlay init\` at the parent first`.
- `data-value` is the resolved parent path so the user knows which directory needs `parlay init`.
- Refuses to create the child root; no partial work (no `<child>/.parlay/`, no `roots.yaml` entry).

---

## Init Framework Default-Inheritance Prompt

**Shows**: message, data-value
**Actions**: provide-text, confirm
**Flow**: onboarding
**Source**: @parlay-tool/multi-root/parlay-init-writes-the-correct-topology-shape

**Page**: parlay-cli
**Region**: main
**Order**: 18

**Notes**:
- Triggered when `parlay init` (or `parlay add-root`) prompts for `sdd-framework` / `prototype-framework` at a child root and the parent has a corresponding default.
- `data-value` is the parent's value, surfaced as `[<parent-value> (default from parent)]`.
- `confirm` (Enter) accepts the parent default; `provide-text` overrides.
- Children may diverge from siblings — no enforcement that all children agree.
- When the parent does not declare a value AND the child omits it AND no flag was passed, the prompt collects a value with no default (free entry).

---

## Status Topology Indicator

**Shows**: status, summary
**Source**: @parlay-tool/multi-root/detect-and-migrate-legacy-topology-mismatches

**Page**: parlay-cli
**Region**: main
**Order**: 19

**Notes**:
- One additional line in `parlay status` output, between the root header and the feature/child-root listing.
- Two states: `topology: ok` (all four checks pass) or `topology: needs repair (<N> mismatches — run \`parlay repair\`)`.
- Single-root and multi-root projects use the same line; a correctly-configured single-root project shows `topology: ok`.
- Status NEVER enumerates per-file detail — that belongs in `parlay repair`.
- Read-only; observing the indicator does not trigger any fix.

---

## Repair Per-Mismatch Prompt

**Shows**: message, data-value, data-list, summary
**Actions**: confirm, dismiss, select-one
**Flow**: review-and-approve
**Source**: @parlay-tool/multi-root/detect-and-migrate-legacy-topology-mismatches

**Page**: parlay-cli
**Region**: main
**Order**: 20

**Notes**:
- Triggered for each topology mismatch detected by `parlay repair`. Mismatches are surfaced one at a time; after each confirm-or-skip the tool re-scans and surfaces the next.
- `summary` shows the position in the queue: `Topology mismatch detected (1 of N)`.
- `data-value` names the mismatch kind: `bare-parent`, `agent-at-child`, `both-have-agent`, or `single-root-missing-ai-agent`.
- `message` describes the problem and the proposed fix in concrete file-path terms — which files will be created, modified, or have fields removed.
- `data-list` is used in the `both-have-agent` conflicting-values case to enumerate the candidate values; `select-one` lets the user pick which value to keep at the parent. The standard `confirm`/`dismiss` (Y/n) handles the unambiguous cases.
- Bare-parent and single-root-missing-ai-agent variants additionally show the Init Agent-Identity Prompt to collect the new `ai-agent` value, with a default that prefers the running agent and falls back to a value found in any child config.
- The fix step preserves any unrecognized fields in the modified config files verbatim.
- No `--all` or `--yes` shortcut in v1; granular confirmation per mismatch is required.

---

## Repair-Clean Result

**Shows**: status, message
**Source**: @parlay-tool/multi-root/detect-and-migrate-legacy-topology-mismatches

**Page**: parlay-cli
**Region**: main
**Order**: 21

**Notes**:
- Emitted by `parlay repair` when the topology check finds no mismatches (either initial state or after a successful repair pass).
- `status` is `success`; `message` is `No topology mismatches found.` (initial) or `All topology mismatches resolved.` (after a fix).
- Skipped mismatches are reported separately as `<N> mismatch remaining (skipped). Re-run \`parlay repair\` to address.` with a non-zero exit code.

---

## Bare-Parent Upgrade Error

**Shows**: status, message
**Source**: @parlay-tool/multi-root/parlay-upgrade-errors-on-bare-parent-topology

**Page**: parlay-cli
**Region**: main
**Order**: 22

**Notes**:
- Emitted by `parlay upgrade` when the active root has `.parlay/roots.yaml` but no `.parlay/config.yaml` (bare-parent topology).
- `status` is `error`.
- `message` is exactly: `bare-parent topology: <parent>/.parlay/config.yaml is missing — run \`parlay repair\` to create it`. Path is concrete (resolved parent root).
- Distinct from the uninitialized-project message (`not a parlay project — run \`parlay init\` first`), which fires when neither `roots.yaml` nor `config.yaml` exists.
- Atomic refusal: nothing deployed, no schemas updated, no skills updated, no warnings printed in the success path.
- Upgrade `--help` text contains no reference to "bare-parent" as a supported topology.
