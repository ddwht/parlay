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
- `parlay extract-domain-model` run in a child root produces a domain model scoped to the child's intents only.
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
**Behavior**: Add a persistent `--root <name>` flag at the root command level. When set, override cwd walk-up with the named root from the parent's roots index. If the same invocation also has a prefixed feature reference, the two must agree — disagreement errors. `--root` is most useful for project-level commands that don't take a feature reference (`parlay extract-domain-model --root web`, `parlay status --root web`).
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

**Affects**: feature enumeration helpers, `parlay status`, `parlay extract-domain-model`, every command that lists features
**Behavior**: Every command that enumerates features at a root must treat an empty `spec/intents/` (no feature subdirectories) as a valid state — return an empty list, not an error. `parlay status` at a parent root with empty spec lists zero parent features and the registered children. `parlay extract-domain-model` at such a root produces an empty-but-valid domain model. Commands that target a specific feature reference still error with "no such feature" when the slug doesn't match — that error is unchanged.
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
