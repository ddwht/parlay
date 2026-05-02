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
