# Multi-root — Infrastructure

---

## Active Root Resolver

**Affects**: configuration package, every CLI command's path resolution
**Behavior**: Replace today's cwd-relative path constants (`.parlay/` directly off cwd, single hardcoded `spec/intents/`, etc.) with a per-invocation resolver that returns a single `ActiveRoot` value at command entry. Resolution order: (1) honor `PARLAY_ROOT` if set — must be an absolute path containing a `.parlay/`; an invalid value errors rather than falling back. (2) Walk upward from cwd looking for the first `.parlay/`, stopping at a `.git/` boundary or the filesystem root. (3) If walk-up failed but candidate roots are discoverable (a `roots.yaml` higher up, or `.parlay/` directories below cwd), invoke disambiguation in interactive contexts; in non-interactive contexts return the appropriate error. The chosen root is recorded once and threaded through subsequent path lookups — every command becomes "given a root, compute paths" rather than "compute paths from cwd directly."
**Invariants**:
- `parlay --verbose` prints the resolved root and the resolution source (cwd-walk-up, PARLAY_ROOT, --root flag, disambiguation) before any other work.
- A single-root project at the repo root resolves identically to today (same effective `.parlay/` and `spec/` paths).
- Walk-up never crosses a `.git/` boundary — `parlay sync` run inside a repo without any `.parlay/` errors at the boundary instead of continuing upward.
- `PARLAY_ROOT=/nonexistent` errors and does not fall through to walk-up.
- Resolution happens exactly once per command invocation (deterministic, no re-resolution mid-command).
**Source**: @parlay-tool/multi-root/discover-active-root-via-cwd-walk-up
**Backward-Compatible**: yes

**Notes**:
- Touches every CLI command — most changes are mechanical (caller migration from free functions to root-method equivalents).

---

## Roots Index and Parent Pointer

**Affects**: configuration package, on-disk parlay metadata layout
**Behavior**: Introduce a parent → children link. A parent root that has registered children stores `.parlay/roots.yaml` listing each child by short name and relative path; a child root's `.parlay/config.yaml` carries a `parent:` field pointing at its parent via relative path. The roots index is the single source of truth for short-name → child-root translation; child names are slugs (not directory paths). A parent with no children has no `roots.yaml` (or an empty one).
**Invariants**:
- A child root's `parent:` path resolves relative to the child's `.parlay/` directory, so the entire repo can be moved on disk without breaking links.
- Child names follow the same slug rules as feature/initiative slugs (lowercase, hyphens, no punctuation, must be unique within the parent).
- A single-root project (no children) has no `roots.yaml` file — the absence of the file is the canonical "root-of-one" state.
- Loading a parent root reads `roots.yaml` lazily; loading a child root resolves its parent pointer eagerly so subsequent resource lookups have it available.
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project, @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd
**Backward-Compatible**: yes

---

## parlay add-root Command

**Affects**: command registry, on-disk parlay metadata
**Behavior**: New subcommand `parlay add-root <subdir>`. Refuses if cwd is not itself a parlay root, if `<subdir>` already contains a `.parlay/`, or if `<subdir>` is nested inside another registered child. On success, creates `<subdir>/.parlay/config.yaml` with a `parent:` pointer, creates `<subdir>/spec/`, appends an entry to the parent's `roots.yaml`, and triggers the agent-surface refresh hook so the agent-rules file lists the new child immediately.
**Invariants**:
- Running `parlay add-root apps/web` twice errors on the second invocation (subdir already has `.parlay/`).
- Running `parlay add-root apps/web` from a directory that is not itself a parlay root errors with a clear message; no `.parlay/` is created.
- After success, the parent's `roots.yaml` lists the new child with a unique short name.
- After success, `cd <subdir> && parlay sync` operates on the child root, not the parent.
- After success, the agent-rules file at the parent root lists the child root without requiring a separate `parlay upgrade` invocation.
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project
**Backward-Compatible**: yes

---

## parlay promote-root Command

**Affects**: command registry, child-root configuration
**Behavior**: New subcommand `parlay promote-root`. Run inside an orphaned child root (parent path no longer resolves), it removes the `parent:` pointer from the child's `.parlay/config.yaml`. The child becomes its own top-level root from that point forward. Refuses if the parent path still resolves — the orphan-recovery path is intentional and must not be used to silently sever a working parent-child link.
**Invariants**:
- After `promote-root`, the child has no `parent:` field and is treated as a top-level root by the resolver.
- Running `promote-root` while the parent path is intact errors and changes nothing.
- The promoted root does not automatically gain schemas or a deployed agent surface — those must be added separately if it is to be used standalone.
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project
**Backward-Compatible**: yes

---

## Resource Resolution Strategy

**Affects**: configuration package, embedded-bundle loaders, adapter loader
**Behavior**: Add a single resolution layer for shared resources keyed off the active root. Schemas resolve unconditionally to the repo-level root (the root with no parent reachable from the active root). The deployed agent surface (skills, agent-rules file) likewise resolves only to the repo-level root. Adapters resolve child-first: if the active root contains a same-named adapter file, it fully replaces the parent's; otherwise the parent's is loaded as-is. Domain models are NOT inherited — each root reads its own `spec/intents/domain-model.md`. Every resolution decision is recorded so verbose mode can print which root provided each loaded resource.
**Invariants**:
- A child root with no `.parlay/adapters/` directory loads adapters from the parent root.
- A child root with `.parlay/adapters/<name>.adapter.yaml` loads that file (and ONLY that file — no key-by-key merge with the parent's same-named adapter).
- `parlay create-domain-model` run in a child root produces a domain model scoped to the child's intents only.
- Schemas always come from the repo-level root, regardless of which child is active.
- Single-root projects produce identical effective resolutions to today (active root == repo-level root, no parent fallback).
**Source**: @parlay-tool/multi-root/inherit-resources-from-parent-root
**Backward-Compatible**: yes

---

## Forbidden-Directory Startup Validation

**Affects**: configuration package, command-entry validation
**Behavior**: When the resolver picks a child root (one with a parent pointer), validate at command entry that the child does not contain repo-level-only directories — `.parlay/schemas/`, the deployed agent surface for the active adapter (`.claude/skills/parlay-*`, `.cursor/agents/parlay-*`, `AGENT_INSTRUCTIONS.md`, etc.). If found, error with the offending path and the rule it violates; refuse to run the command. Single-root and parent roots skip the check.
**Invariants**:
- `apps/web/.parlay/schemas/` triggers the error; `apps/web/.parlay/adapters/` does not (adapters are the only resource a child can override).
- The list of forbidden directories per adapter comes from the deployer registry, not hardcoded — adding a new adapter automatically extends the validation.
- The check runs at command entry, before any work; the user sees the error immediately, not partway through a build.
**Source**: @parlay-tool/multi-root/inherit-resources-from-parent-root, @parlay-tool/multi-root/single-repo-level-agent-surface-drives-all-roots
**Backward-Compatible**: yes

---

## Feature-Reference Parser Extension

**Affects**: feature-reference parser, every command that accepts a feature reference
**Behavior**: Extend the parser to accept an optional root prefix on a feature reference: `<root-name>:@<feature>` or `<root-name>:@<initiative>/<feature>`. The prefix is a slug followed by a single `:`. A bare reference (no prefix) preserves existing semantics — resolved against the active root. The parser returns the parsed components; resolution against the roots index happens in the resolver, not the parser.
**Invariants**:
- All existing references (no prefix) parse identically to today.
- A prefix that does not match the slug rules errors at parse time.
- Cross-root references inside intent or dialog content trigger a validation error — the prefix is for CLI/cross-tool addressing, not for authored content (out of scope for v1).
**Source**: @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd
**Backward-Compatible**: yes

---

## --root Flag Plumbing

**Affects**: root command, every subcommand that takes a feature reference or operates project-wide
**Behavior**: Add a persistent `--root <name>` flag at the root command level. When set, override cwd walk-up with the named root from the parent's roots index. If the same invocation also has a prefixed feature reference, the two must agree — disagreement errors. `--root` is most useful for project-level commands that don't take a feature reference (`parlay create-domain-model --root web`, `parlay status --root web`).
**Invariants**:
- An unknown root name errors with the list of registered roots — never silently falls through.
- `--root web` and `web:@feat` together work; `--root api` and `web:@feat` together error.
- The flag is inherited by every subcommand, so per-command plumbing is minimal.
**Source**: @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd
**Backward-Compatible**: yes

---

## Ambiguous-Reference Resolver

**Affects**: feature-resolution path inside every read/write command
**Behavior**: When a bare feature reference is parsed (no prefix, no `--root`), look up the feature in the active root first. If found, that wins. If not found, search every registered child root. Single match → auto-select that child and announce the chosen root in normal output. Multiple matches → invoke the disambiguation prompt in interactive contexts; in non-interactive contexts, error with the candidate root list and a hint to use the prefix or `--root`.
**Invariants**:
- A bare reference that matches in the active root never reaches the children-search path (active root always wins when it has the feature).
- The auto-select case prints the chosen root in normal (non-verbose) output — the user always sees when the active root was effectively switched.
- Disambiguation prompt and ambiguous-error message share the same candidate-root rendering format.
- The active-root disambiguation case (no `.parlay/` at cwd, multiple candidate roots elsewhere) shares the same prompt UX as this fragment.
**Source**: @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd
**Backward-Compatible**: yes

---

## Deployer Multi-Root Awareness

**Affects**: deployer registry, every adapter-specific deployer (claude, cursor, generic)
**Behavior**: Each deployer's agent-rules generation includes a multi-root section listing the parent root and every registered child by name, relative path, and one-line description from the child's config. The section sits between adapter-specific markers (e.g. parlay:begin/end comments for Claude) and is replaced wholesale on every refresh; user-authored sections outside the markers are preserved per existing claude-md-section-preservation rules. Each deployer additionally exposes its agent-surface directory list so the forbidden-directory validation can reference it.
**Invariants**:
- A project with no child roots produces no multi-root section (no empty header, no placeholder).
- User-authored content outside the parlay markers survives every refresh — verified by the existing claude-md-section-preservation tests, extended to cover the multi-root section.
- The same rule applies uniformly across adapters: `CLAUDE.md` (Claude Code), `.cursor/rules/parlay.mdc` (Cursor), `AGENT_INSTRUCTIONS.md` (Generic) — all gain a multi-root section at the repo-level root only.
- Adapter output paths are unchanged; only the generated content gains the multi-root section.
**Source**: @parlay-tool/multi-root/single-repo-level-agent-surface-drives-all-roots
**Backward-Compatible**: yes

---

## Auto-Refresh Hook on add-root and remove-root

**Affects**: add-root command, remove-root command, deployer entry point
**Behavior**: After `parlay add-root` and `parlay remove-root` succeed, automatically run the in-process equivalent of `parlay upgrade` at the parent (repo-level) root. The user does NOT need a separate `parlay upgrade` invocation — the agent-rules file lists the new root set on first agent invocation after the command returns.
**Invariants**:
- Successful `add-root` always triggers the refresh; the user does not need to invoke `parlay upgrade` separately.
- A refresh failure does NOT roll back the root change (the on-disk root is created); the command emits a warning, exits non-zero, and the user can re-run `parlay upgrade` manually.
- The refresh runs in-process — not via an external `parlay upgrade` subprocess — so failure modes are surfaced precisely.
- `remove-root` is implied by symmetry; capture as a follow-up if v1 ships without it.
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project, @parlay-tool/multi-root/single-repo-level-agent-surface-drives-all-roots
**Backward-Compatible**: yes

---

## Parent-Pointer Validation

**Affects**: configuration package, command-entry validation
**Behavior**: When the resolver loads a child root, it must verify the parent path resolves to a valid parlay root (a directory with a readable `.parlay/config.yaml` and no `parent:` field of its own). If the parent path is missing, moved, or itself orphaned, every command run against the child errors with "parent root not found at <path>; restore it or run `parlay promote-root` to make this child standalone" and refuses to fall through to a different ancestor via walk-up.
**Invariants**:
- An orphaned child never silently picks up a different ancestor as its parent.
- The validation happens once per invocation, after resolver selection and before any command work.
- The error message names the recorded parent path so the user knows what to restore.
- `parlay promote-root` is the documented escape hatch and is mentioned in the error message.
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project
**Backward-Compatible**: yes

---

## Bare-Parent Empty-Spec Handling

**Affects**: feature enumeration helpers, `parlay status`, `parlay create-domain-model`, every command that lists features
**Behavior**: Every command that enumerates features at a root must treat an empty `spec/intents/` (no feature subdirectories) as a valid state — return an empty list, not an error. `parlay status` at a parent root with empty spec lists zero parent features and the registered children. `parlay create-domain-model` at such a root produces an empty-but-valid domain model. Commands that target a specific feature reference still error with "no such feature" when the slug doesn't match — that error is unchanged.
**Invariants**:
- A parent root with empty `spec/intents/` and one or more registered children is fully functional — no command errors with "no features found in this root".
- `parlay status` at a bare parent shows the parent (zero features), the registered children, and no warning about empty spec.
- Single-root projects with at least one feature are unaffected.
- The "no such feature" error path for specific-feature commands is preserved (different code path).
**Source**: @parlay-tool/multi-root/add-a-child-root-to-an-existing-project, @parlay-tool/multi-root/inherit-resources-from-parent-root
**Backward-Compatible**: yes

---

## Structured Disambiguation Signal from CLI

**Affects**: CLI output contract for ambiguous-reference and ambiguous-active-root cases
**Behavior**: The CLI must emit a structured signal — JSON to stderr, a specific exit code, and a parseable error envelope — when it would otherwise prompt for disambiguation interactively. The signal contains the candidate root list, the trigger (ambiguous-active-root vs. ambiguous-feature-reference), and a re-invocation hint. Interactive CLI runs continue to prompt at stdin; non-interactive runs (no TTY, JSON-output flag set, or skill wrapper invocation) return the signal in a form callers can parse. Skills consume this signal to re-render the prompt through the agent's question mechanism.
**Invariants**:
- A flag (e.g. `--ambiguity-as-signal`) explicitly opts into structured-output behavior; the default in interactive contexts is unchanged.
- Skill wrappers always pass the structured-output flag, so the CLI never blocks waiting for stdin from inside a skill invocation.
- The signal includes enough information to re-invoke deterministically with the user's choice (typically a root prefix to add to the original feature reference).
- The exit code on ambiguity is distinct from generic "command failed" so callers can detect the case.
**Source**: @parlay-tool/multi-root/skill-invocation-in-a-multi-root-project, @parlay-tool/multi-root/discover-active-root-via-cwd-walk-up, @parlay-tool/multi-root/target-a-non-active-root-from-any-cwd
**Backward-Compatible**: yes

---

## Skill-Side Multi-Root Wrapper

**Affects**: every deployed `/parlay-*` skill source under `internal/embedded/skills/`
**Behavior**: Every skill that operates on a feature reference or root-scoped resource follows a uniform invocation pattern: (1) invoke the underlying `parlay` CLI with the structured-disambiguation flag set; (2) on success, render the CLI output to the user, prefixing it with the operating-root announcement when the chosen root differs from cwd-default; (3) on a structured ambiguity signal, render the candidate root list through the agent's native question mechanism, capture the user's choice, and re-invoke the CLI with a root prefix added to the feature reference; (4) on a forbidden-directory error, translate to skill-side guidance with the exact remediation path. The wrapper logic is documented once in the skill template and applied to every skill that takes a feature reference.
**Invariants**:
- No deployed skill text contains a hardcoded `.parlay/` path; every path reference is described as "the active root's <thing>".
- Every skill that takes a feature reference accepts the `<root-name>:@feat` prefix as well as the bare reference.
- `--root <name>` is accepted as a skill argument and passed through to the CLI verbatim.
- The same skill source produces the same multi-root behavior on Claude Code, Cursor, and Generic CLI — no adapter-specific skill content for multi-root concerns.
- Skills do NOT cache "the active root for this conversation" — every invocation re-resolves from scratch via the CLI's resolver.
**Source**: @parlay-tool/multi-root/skill-invocation-in-a-multi-root-project
**Backward-Compatible**: yes

---

## Skill Source Audit for Hardcoded Paths

**Affects**: build-time validation, every file under `internal/embedded/skills/`
**Behavior**: Add a validation step (run during `make build` or as a unit test) that scans every embedded skill file for hardcoded path references that would break under multi-root — strings like `.parlay/schemas/`, `spec/intents/`, etc. that are not phrased as "active root's ..." or wrapped in a placeholder the skill resolves at runtime. Failures fail the build with the offending file and line.
**Invariants**:
- A new skill file added without active-root awareness fails the audit at build time.
- The audit runs as part of `make verify-skills` and CI so multi-root regressions are caught before release.
- Approved hardcoded references (e.g. references in skill prose explaining the parent-root-only resources) are explicitly allowlisted via marker comments.
**Source**: @parlay-tool/multi-root/skill-invocation-in-a-multi-root-project
**Backward-Compatible**: yes

**Notes**:
- Pairs with the dogfooding rule in CLAUDE.md — source-of-truth files are validated; deployed copies are derived state.

---

## Agent-Identity Single-Source Validation

**Affects**: configuration package, every command's config-load path
**Behavior**: At config-load time, enforce that `ai-agent` is declared in exactly one config file per project. In multi-root projects this is the parent's `config.yaml`; in single-root projects it is the only `config.yaml`. Detect three illegal states and hard-error before any command work runs: (1) a child `config.yaml` carries `ai-agent` — error names the offending child path and tells the user to remove the field; (2) both parent and child carry `ai-agent` — error enumerates each declaring file with its value, and explicitly does NOT silently prefer either side, even when values match; (3) a multi-root parent carries no `ai-agent` field while children continue to operate — `parlay upgrade` and any other command that needs the agent identity hard-errors with a "no agent identity declared at parent root" message that names the parent file path. The validator never walks up past the recorded parent and never falls back to a child's value to satisfy the parent's missing field.
**Invariants**:
- A child `config.yaml` containing `ai-agent` causes any command to fail at load time with the offending child path quoted.
- Parent + child both declaring `ai-agent` causes any command to fail at load time, even when values agree, with both file paths and values quoted.
- A multi-root parent missing `ai-agent` fails commands that need the agent identity (upgrade, deployer-driven flows) at load time; commands that don't need it (e.g. `parlay status`) still load and report.
- Single-root projects with `ai-agent` + `sdd-framework` + `prototype-framework` together in one config load identically to today.
- The validator runs once per command invocation (deterministic, no re-validation mid-command).
**Source**: @parlay-tool/multi-root/agent-identity-lives-at-the-parent-in-multi-root-projects
**Backward-Compatible**: no

**Notes**:
- Removes the previous tolerance for child-declared `ai-agent` and for parent-missing `ai-agent`. Existing projects in those states must run `parlay repair` once before they can use commands that touch the agent identity.

---

## Per-Field Config Inheritance Resolver

**Affects**: configuration package, command-entry config load
**Behavior**: When loading a child root's effective configuration, resolve each field independently. `ai-agent` is parent-only — the resolver never reads it from the child and never inherits it through walk-up beyond the recorded parent. `sdd-framework` and `prototype-framework` are child-first with parent fallback — the child's value wins when present, the parent's value is used when the child omits the field, and a hard error fires when neither side declares the field for a command that needs it. Each effective field carries its source file path so verbose mode can render `<field>: <value> (from <file>)` for direct declarations and `<field>: <value> (inherited from <file>)` for parent-fallback cases.
**Invariants**:
- A child config with no `sdd-framework` and a parent that declares `sdd-framework: parlay-spec` resolves to `parlay-spec` for commands run in the child, with no warning.
- A child and parent that both omit `sdd-framework` cause a build-feature command at the child to error with both candidate file paths quoted.
- Children may diverge from siblings — sibling configs are not consulted during inheritance resolution.
- Verbose output records the resolution origin (`from`, `inherited from`, or the literal `not declared`) for every effective field.
- The resolver does not mutate either config file; inheritance is a load-time decision only.
**Source**: @parlay-tool/multi-root/agent-identity-lives-at-the-parent-in-multi-root-projects
**Backward-Compatible**: yes

---

## Init Topology Writer

**Affects**: project initialization, on-disk parlay metadata layout
**Behavior**: `parlay init` writes exactly one `config.yaml` at the directory it runs in and never creates child configs as a side effect. The write is shape-aware: at a directory with no parent pointer, the writer emits `ai-agent`, `sdd-framework`, and `prototype-framework` (single-root or parent shape); at a directory whose `parent: ..` resolves to an existing parent config, the writer omits `ai-agent` and emits only `sdd-framework`, `prototype-framework`, `parent: <relative>` (child shape). The writer never produces a bare-parent state — when the invocation directory will host children (a `roots.yaml` exists or is being created in the same flow), the parent config MUST contain at least `ai-agent`. Re-running `parlay init` against an already-correctly-configured project is a pure idempotent no-op: no fields are re-prompted, no files are touched, the command exits zero with "Project already initialized."
**Invariants**:
- `parlay init` at an empty directory writes one config file with exactly the fields prompted; no other file system writes.
- `parlay init` at a child whose parent has `ai-agent` set writes the child config with no `ai-agent` field — confirmed by reading back the written file.
- `parlay init` re-run on a topologically-correct project changes no files (mtime preserved).
- `parlay init` cannot write a config that fails the Agent-Identity Single-Source validator — the writer and the validator agree on the shape.
- The writer never invokes `parlay add-root` internally; child creation remains an explicit user step.
**Source**: @parlay-tool/multi-root/parlay-init-writes-the-correct-topology-shape
**Backward-Compatible**: yes

---

## Init Agent-Detection Hook

**Affects**: project initialization, agent-identity prompt
**Behavior**: When `parlay init` prompts for `ai-agent`, the prompt consults a detection hook that inspects the runtime environment (env vars, parent process, terminal markers) and returns the running agent's name when one is recognized. The detection result pre-fills the prompt as a default; the user MUST press Enter to confirm or type an alternative — init never proceeds without an explicit choice, even when a default is detected. The detection set covers the same adapters the deployer registry knows about (Claude Code, Cursor, Generic CLI). When no agent is detected, the prompt falls back to free entry with the adapter list as guidance.
**Invariants**:
- Detection is read-only — it never writes to disk or mutates env state.
- A detected default is rendered as `[<name> (detected)]` in the prompt; the user's Enter keypress writes the detected value, an alternative entry overrides it.
- Init does not auto-write the detected value silently — explicit confirmation is required regardless of detection success.
- The detector returns "unknown" rather than guessing when signals are ambiguous; the prompt then has no pre-filled default.
**Source**: @parlay-tool/multi-root/parlay-init-writes-the-correct-topology-shape
**Backward-Compatible**: yes

---

## Add-Root Parent Agent Precondition

**Affects**: `parlay add-root` command
**Behavior**: Before creating a new child root, `parlay add-root` verifies that the resolved parent has a `config.yaml` containing `ai-agent`. If the parent's config is missing or has no `ai-agent` field, the command refuses with `parent is missing ai-agent — run \`parlay init\` at the parent first`, including the resolved parent path in the message, and performs no work — no `<child>/.parlay/`, no `roots.yaml` entry, no agent-surface refresh. The precondition is enforced before any other validation (subdir-already-exists, nesting checks) so the user sees the structural problem first when multiple errors apply.
**Invariants**:
- `parlay add-root` against a bare-parent project (no parent config) errors with the precondition message and the parent path.
- `parlay add-root` against a parent whose `config.yaml` exists but lacks `ai-agent` errors with the same precondition message.
- A successful precondition check is required before any file system writes — partial state is impossible.
- The precondition runs after parent resolution but before subdir validation; the order is fixed so the message is consistent.
**Source**: @parlay-tool/multi-root/parlay-init-writes-the-correct-topology-shape
**Backward-Compatible**: no

**Notes**:
- This is a behavior change from the previous flow that would happily create children against bare-parent projects. Existing bare-parent projects must run `parlay init` (or `parlay repair`) at the parent before adding more children.

---

## Topology Validator

**Affects**: configuration package, `parlay status` and `parlay repair`
**Behavior**: A shared topology-check pass that scans the active project for four specific structural mismatches against the config-shape model: (1) bare-parent — a parent has `roots.yaml` but no `config.yaml`; (2) agent-at-child — a child `config.yaml` declares `ai-agent`; (3) both-have-agent — parent and child both declare `ai-agent`, regardless of whether values agree; (4) single-root-missing-ai-agent — a single-root `config.yaml` lacks `ai-agent`. The pass returns a list of mismatches, each carrying its kind, the offending file paths, the conflicting values when relevant, and a structured "proposed fix" descriptor. The validator is read-only — it never mutates configs, never auto-fixes, never emits side effects. It is invoked by `parlay repair` (full enumeration with prompts) and by `parlay status` (count + boolean used to render the topology line); other commands MUST NOT invoke it on every invocation, and they may surface a one-line hint only when they directly hit a topology error.
**Invariants**:
- Running the validator twice in succession against the same on-disk state returns identical results (deterministic, no caching that drifts).
- The validator's output is structured (mismatch kind enum + file paths + values), so both `parlay status` (which renders a count) and `parlay repair` (which renders per-mismatch detail) consume the same source of truth.
- A single-root project with `ai-agent` + `sdd-framework` + `prototype-framework` declared together returns zero mismatches.
- A multi-root project with parent `ai-agent` and children carrying only framework + parent-pointer returns zero mismatches.
- The validator never writes to disk and never invokes the deployer or any other side-effect-bearing component.
- The four mismatch kinds are mutually identifiable — a project with two simultaneous mismatches (e.g. agent-at-child plus bare-parent) returns two distinct entries.
**Source**: @parlay-tool/multi-root/detect-and-migrate-legacy-topology-mismatches
**Caching**: per-process
**Backward-Compatible**: yes

---

## Repair One-At-A-Time Driver

**Affects**: `parlay repair` command
**Behavior**: `parlay repair` includes a topology-fix loop that wraps the Topology Validator: surface one mismatch via the Repair Per-Mismatch Prompt; on confirm, apply the proposed fix using the structured fix descriptor and re-scan; on skip, record the mismatch and re-scan (skipped mismatches stay surfaced and contribute to the final exit code); on cancel, exit non-zero with the remaining mismatch count. The driver MUST re-scan after every applied fix so cascading mismatches (e.g. fixing bare-parent reveals an agent-at-child that was previously masked) are surfaced naturally. There is no `--all` or `--yes` shortcut in v1 — every fix requires explicit confirmation. Fix application MUST preserve every unrecognized field in the modified config files verbatim; only the targeted fields are added, removed, or changed.
**Invariants**:
- Each fix is applied atomically — either the whole structured change lands or the file is left untouched and an error is reported.
- Re-scanning after a fix uses the same Topology Validator pass — there is no stale state.
- Skipped mismatches do NOT block the next mismatch from being surfaced; the user can address as many or as few as they want in one invocation.
- After all confirmed fixes apply, re-running `parlay repair` against the same project reports zero mismatches (durability).
- `parlay status` reports `topology: ok` after a successful repair pass.
- Unrecognized fields in modified config files survive the fix verbatim — the repair driver is a structural-rewrite, not a re-serialization from scratch.
**Source**: @parlay-tool/multi-root/detect-and-migrate-legacy-topology-mismatches
**Backward-Compatible**: yes

---

## Status Topology-Line Renderer

**Affects**: `parlay status` command
**Behavior**: `parlay status` invokes the Topology Validator in count-only mode and emits one new line in its rendered output: `topology: ok` when zero mismatches are returned, or `topology: needs repair (<N> mismatches — run \`parlay repair\`)` otherwise. The renderer never enumerates per-mismatch detail; per-file diagnostics are reserved for `parlay repair`. The line is uniform across single-root and multi-root projects; a correctly-configured single-root project also reports `topology: ok`. The renderer adds no new failure modes — `parlay status` continues to exit zero whether the topology is clean or dirty.
**Invariants**:
- `parlay status` in a clean project prints exactly `topology: ok`.
- `parlay status` with N mismatches prints `topology: needs repair (<N> mismatches — run \`parlay repair\`)` with no enumerated detail.
- Status's exit code is unchanged by the topology indicator (always zero on a successful read).
- The renderer relies on the Topology Validator and does not duplicate detection logic.
**Source**: @parlay-tool/multi-root/detect-and-migrate-legacy-topology-mismatches
**Backward-Compatible**: yes

---

## Bare-Parent Fallback Removal in deployToRoot

**Affects**: `parlay upgrade` command, the deployToRoot helper
**Behavior**: Remove the bare-parent branch from the per-root deploy step in `parlay upgrade`. When the parent has `roots.yaml` but no `config.yaml`, the command MUST hard-error immediately with `bare-parent topology: <parent>/.parlay/config.yaml is missing — run \`parlay repair\` to create it`, exit non-zero, and perform no deploys (no schemas, no skills, no partial work). The pre-existing "uninitialized project" path (no `roots.yaml` AND no `config.yaml`) continues to error with the existing "run parlay init first" message — that case is distinct. Correctly-configured projects (single-root or multi-root with parent `config.yaml`) deploy quietly with no warnings or info lines about topology — the success path is preserved unchanged.
**Invariants**:
- `parlay upgrade` against a bare-parent project produces the exact error string above and exits non-zero with zero file system writes.
- `parlay upgrade` against a correctly-configured project (single or multi-root) writes schemas and skills with no topology-related warnings.
- `parlay upgrade` against an uninitialized directory produces the pre-existing "not a parlay project" error — distinct text from the bare-parent error so users can disambiguate.
- The deployToRoot code path that previously proceeded with empty config and skipped skills no longer exists — there is one path for correct topology and one path for hard-error.
- After `parlay repair` migrates a bare-parent project, the next `parlay upgrade` runs cleanly with no manual intervention.
- `parlay upgrade --help` text contains no reference to "bare-parent" as a supported configuration.
**Source**: @parlay-tool/multi-root/parlay-upgrade-errors-on-bare-parent-topology
**Backward-Compatible**: no

**Notes**:
- This release removes the only soft-fail path that hid the original drift bug. With Intents A, B, and C in place, the migration story is "run `parlay repair` once" rather than "wait for a deprecation cycle."
