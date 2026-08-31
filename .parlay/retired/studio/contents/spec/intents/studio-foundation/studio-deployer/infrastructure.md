# Studio-deployer — Infrastructure

---

## Studio embedded source surface and deployer subcommands

**Affects**: build-time embedded source surface and deployer subcommand vocabulary
**Behavior**: The parlay-studio binary owns a dedicated embedded source surface internal to its source tree, with subdirectories that mirror the canonical parlay embedded-source layout (skills today; agents, schemas, and adapters when future features add them). Every Studio-owned skill source lives at exactly one canonical location within that surface; no project-local development-convenience copy exists outside it. The contents of the embedded surface are baked into the parlay-studio binary at build time so the binary ships its own skills self-contained. The binary exposes two deployer subcommands — an initial-install command and an idempotent re-install command — that resolve the active parlay project root using studio-config's existing project-root resolver, refuse to operate against a directory that is not a parlay-initialized project (no parlay state directory present), detect every active agent surface, and write each embedded skill to every detected agent's target path. Both subcommands report the deployed file list to the standard output stream with one line per file (path plus source-component attribution). The deployer reads only its own embedded source and writes directly; it does not invoke the parlay binary or shell out to any external tool. The first concrete skill embedded is the design-loop skill that today exists as a project-local development convenience; this feature relocates it to the canonical embedded location, after which the project-local copy is removed from the source tree and reappears in the working tree only as deployer output during the dogfooding self-install.
**Invariants**:
- The canonical source location for every Studio-owned skill is the Studio embedded source surface; checking for a second copy of any Studio skill source elsewhere in the source tree returns nothing
- Every Studio skill source begins with a YAML frontmatter block declaring a name key and a description key; a source file that fails this shape check fails the Studio build with a named error
- The Studio binary contains its embedded source surface at link time; running the binary against a project with no on-disk Studio sources still deploys the full skill set
- The two deployer subcommands accept the same project-root resolution inputs (the project flag, the project-root environment variable, and the working-directory walk-up precedence) and produce the same deployed file set when run against the same embedded source
- The deployer refuses to operate against a directory without a parlay state directory, failing with a stable error code that names the missing marker
- The deployer does not invoke the parlay binary; static analysis of the Studio deployer source shows no execve, shell-out, or parlay-binary path lookups
- A built Studio binary's help text for both deployer subcommands describes the deployer behavior, the per-agent fan-out, and the file-ownership rules
**Source**: @studio-foundation/studio-deployer/studio-binary-embeds-its-skills-and-deploys-them-via-init-upgrade-subcommands
**Caching**: none
**Backward-Compatible**: no

**Notes**:
- The strict-vs-chain question for the initial-install subcommand resolves to **strict**: the deployer fails with the stable parlay-not-initialized error code rather than auto-bootstrapping parlay. Auto-bootstrap was rejected because it would couple Studio's binary to the parlay binary being on PATH, would muddle the two binaries' independence, and would be less predictable in scripted environments. An opt-in bootstrap flag is a separate future feature.
- Brew formula authoring, release pipelines, cross-compilation matrices, and versioning schemes are external infrastructure concerns out of scope for this feature.
- The three Studio-related spec schemas (vocabulary, design-loop-result, design-loop-conflicts) remain in parlay-core's embedded set rather than relocating to Studio's embedded set, because they describe spec-layer shapes any parlay project might use even without parlay-studio installed.

---

## Multi-agent target resolution and stable failure codes

**Affects**: agent-surface detection vocabulary and feature-stable error codes
**Behavior**: The deployer detects every agent surface present in the active parlay project and fans out to each detected surface, writing one deployed file per embedded skill per detected agent using the per-agent target-path convention parlay already documents for that surface. The deployer does not select one preferred agent and skip the others. Three agent surfaces are recognized today: an interactive-editor surface that uses one marker directory and per-agent skill subdirectories; a second interactive-editor surface that uses a different marker directory and a different per-agent target-path convention; and a headless command-line surface used in continuous-integration and other non-interactive environments. The per-agent fan-out is atomic per agent (all writes for one agent surface land together as one coordinated step) but failures on one agent surface do not block writes to other detected agent surfaces — each detected agent gets its own atomic write. When a detected agent's parent marker is present but its per-skill target subdirectory has not yet been created, the deployer creates the missing subdirectory as part of the same per-agent atomic write step. When no agent surface is detected, the deployer fails with a stable error code naming the agent surfaces parlay knows about. When the parlay state directory is absent, the deployer fails earlier with a separate stable error code naming the missing marker. The Studio deployer maintains its own local agent-detection implementation rather than importing detection code from parlay-core; the local implementation matches parlay-core's current conventions exactly and is updated by a parlay-studio binary release whenever parlay-core adds a new agent surface.
**Invariants**:
- A project with only the first interactive-editor surface present receives deployed files only at that surface's per-agent target paths
- A project with only the second interactive-editor surface present receives deployed files only at that surface's per-agent target paths
- A project with both interactive-editor surfaces present receives deployed files at both surfaces' target paths
- A project with only the headless command-line surface present receives deployed files at the headless surface's per-agent target paths
- A project with no detected agent surface causes the deployer to fail with the stable error code `studio-deployer-no-agent-detected` and to write zero files
- A project without a parlay state directory causes the deployer to fail earlier with the stable error code `studio-deployer-parlay-not-initialized` and to write zero files
- A project with a parent agent marker but a missing per-skill subdirectory has the subdirectory created as part of the per-agent atomic write step
- A failure on one detected agent surface produces a non-zero exit code but does not block the per-agent atomic writes on other detected agent surfaces
- The Studio deployer source contains its own agent-detection implementation; no import of a parlay-core agent-detection module is present
- The two stable error codes (`studio-deployer-no-agent-detected`, `studio-deployer-parlay-not-initialized`) are surfaced verbatim in the deployer's error output and are documented as feature-stable codes outside the closed errors vocabulary
**Source**: @studio-foundation/studio-deployer/multi-agent-target-resolution
**Caching**: none
**Backward-Compatible**: no

**Notes**:
- The reuse-vs-duplicate question for agent detection resolves to **duplicate locally**. Importing from a hypothetical parlay-core public-package agent module was considered but rejected because parlay-core has not yet promoted its detection from internal to a public package with stability commitments, and Studio's independence from parlay-core's internal layout is worth the small duplication footprint. If a future parlay-core refactor promotes the detection module to a public package with stability guarantees, a follow-up feature can switch Studio over.
- When parlay-core adds a new agent surface, Studio's deployer ignores the new surface until a parlay-studio binary release picks it up. The deployer does not fail in that situation; it simply does not fan out to the unrecognized surface.

---

## Manifest-based file ownership, atomic writes, and idempotent re-deploy

**Affects**: deployer output-file ownership invariants, atomic-write primitive, and build-time write-call guardrail
**Behavior**: Studio's deployer maintains an explicit owned-files manifest derived from the embedded source surface at build time. Every entry in the embedded source surface maps to one or more deployed-file paths — one per detected agent surface — and a file on disk is owned by the deployer if and only if its path appears on the manifest produced for the current parlay-studio binary build. Ownership is by manifest, not by name prefix: the `parlay-` naming prefix is a documented convention for Studio-owned files (matching parlay-core's prefix convention so the visual boundary is consistent) but the prefix alone does not grant ownership. A user-authored file whose path happens to match the convention is not owned by Studio and is never touched. The deployer reads, writes, or skips only paths on the current manifest; it never touches paths outside the manifest under any condition, regardless of name or location. Every output file is written atomically using a write-temp-then-rename pattern (a temporary sibling file is written and flushed, then renamed over the target) so that a deployer crash mid-run cannot leave a half-written file observable from outside the deployer. Before writing, the deployer computes a content hash of each embedded source and compares against the existing on-disk file; when hashes match, the write is skipped and the file is reported as unchanged. A repeated deployer run over an unchanged embedded source against an unchanged on-disk state produces zero writes. When a previous parlay-studio binary version shipped a skill that the current binary's manifest no longer includes, the deployer reports that on-disk file as an orphan, logs a stable warning code naming the file path and the previously-owning Studio version when discoverable, and takes no further action — the file is preserved on disk and the operator decides whether to remove it manually. The deployer never auto-deletes orphans. The deployer's standard-output summary lists every file with one of four statuses: written (content changed), unchanged (content matched, write skipped), orphan (on disk but not on the current manifest, logged warning, left untouched), failed (an error occurred). The exit code is zero when every file is written, unchanged, or orphan, and non-zero when any file is failed. If a future Studio feature ever writes to a shared file that mixes Studio-owned and user-owned content, the write uses marker-based section preservation — content between named begin and end markers is replaced, content outside the markers is preserved verbatim — and no shared-file writes occur today. A build-time guardrail scans the deployer source for direct write-primitive calls (any path-and-bytes write that bypasses the canonical atomic-write helper) and fails the build if any are found outside the helper itself.
**Invariants**:
- The deployer's owned-files manifest is byte-equivalent to the file list derived from the embedded source surface; every embedded source produces exactly the documented set of per-agent target paths and no other paths
- The deployer never reads, modifies, or deletes any path outside the current manifest, including paths that match the `parlay-` naming prefix but are not on the manifest
- A user-authored file at a path matching the prefix convention but not on the manifest is left untouched and does not appear in the deployer's standard-output summary
- A stale Studio skill from a previous binary version, present on disk but not on the current manifest, is reported with the orphan status, logs the stable warning code `studio-deployer-orphan-detected`, and is left on disk
- Orphan reporting never causes the deployer to delete a file; the operator removes orphans manually after confirming they are no longer needed
- A fresh deployer run against an empty target reports every output file as written; a second deployer run against the resulting on-disk state reports every file as unchanged
- A second deployer run over an unchanged embedded source against an unchanged on-disk state produces zero filesystem writes and zero temporary sibling files at any point during the run
- A crash between writing the temporary sibling and the rename leaves the original target file intact and observable as its prior content; the orphaned temporary sibling is cleaned up at the start of the next deployer run before any new write begins
- The standard-output summary uses exactly the four-status enum (written, unchanged, orphan, failed) and the exit code is zero when no file has the failed status
- The build-time guardrail returns zero matches for direct non-atomic write primitives anywhere in the Studio deployer source outside the canonical atomic-write helper
- No shared-file writes that mix Studio-owned and user-owned content exist in this feature; any future shared-file writes use named begin/end markers and preserve content outside the markers verbatim
**Source**: @studio-foundation/studio-deployer/file-ownership-atomic-writes-and-idempotency
**Caching**: none
**Backward-Compatible**: no

**Notes**:
- The manifest is reconstructed on every deployer invocation from the embedded source surface and is never persisted between runs; drift between the manifest and the embedded source is impossible by construction.
- Orphan reporting includes the previously-owning Studio binary version only when that information is discoverable from the on-disk file itself (a header line, a sidecar metadata file) or from a parlay-studio-maintained history; when the previous owner cannot be identified, the orphan report names only the file path.
- The four-status enum is feature-stable: external tooling that parses the deployer's standard-output summary may rely on the four values written, unchanged, orphan, and failed. Adding a fifth status is a breaking change to the deployer's output contract and requires a separate intent.

---
