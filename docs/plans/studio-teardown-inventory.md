# Studio teardown inventory (Phase 0.1)

Status: analysis artifact for Phase 0.1 of the backend-only/document-as-API
programme. Companion to `domain-editor-api-plan.md` (architecture, FINAL v11)
and `domain-editor-implementation-plan.md` (implementation, FINAL v9). This
document is evidence, not authority: it records what is on disk on branch
`teardown-inventory` at the commit named below, so the D10 root-retirement
preflight can be written against facts rather than recollection.

Swept at: `main` @ `25a165a` (2026-08-31).
Scope of the sweep: the whole repository except `.git/`, `node_modules/`, and
built `dist/` output.

---

## 1. Method

Every claim below is reproducible with one of these commands, run from the
repository root.

**1a. Every marker, everywhere (the completeness check).**

```bash
grep -rn -E "parlay-(feature|component|extends):" . \
  --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist
# 1017 marker lines across Go, TS/TSX, embedded skills, embedded schemas,
# CAVEAT (2026-08-31): this count is not reproducible as written — the sweep
# does not exclude THIS report, which quotes many markers, so re-running it
# after this file exists inflates the number. Reproduce with
# --glob '!docs/plans/studio-teardown-inventory.md' (and treat the figure as
# indicative, not contractual).
# deployed skills (.claude/skills/), deployed modules (.parlay/modules/),
# deployed schemas (.parlay/schemas/), build artifacts, and testdata fixtures.

grep -rhoE "parlay-(feature|component|extends): *[A-Za-z0-9_./@-]+" . \
  --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist \
  | sort | uniq -c | sort -rn
# The distinct marker vocabulary, used to identify which values name a
# studio-root feature rather than a core-root one.
```

**1b. Markers naming a STUDIO-root feature.** The studio root's six groups are
`design-loop`, `domain-model-editor`, `studio-ai-authoring`, `studio-deferred`,
`studio-foundation`, `studio-multi-adapter`. Note that `studio-support` is a
**core-root** group (`core/spec/intents/studio-support/`) and is NOT part of
the retiring root — the name is a trap.

```bash
grep -rnE "parlay-(feature|component|extends): *(design-loop|domain-model-editor|studio-ai-authoring|studio-deferred|studio-multi-adapter|studio-foundation)/" . \
  --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=studio
```

**1c. Prose references (not markers).**

```bash
for pat in 'domain-edit' 'no-editor' 'no_editor' 'NoEditor' 'PARLAY_EDITOR' \
           'internal serve' 'OpenDomainEditor' 'STUDIO_'; do
  grep -rn "$pat" . --exclude-dir=.git --exclude-dir=node_modules \
    --exclude-dir=dist --exclude-dir=studio -l
done

grep -rniE "domain-edit|no-editor|no_editor|PARLAY_EDITOR|editor|web server|browser|studio" \
  core/internal/embedded/skills core/internal/embedded/schemas \
  .claude/skills .parlay/modules .parlay/schemas README.md docs/ CLAUDE.md Makefile

grep -rcniE "design.loop|figma|design-spec" \
  core/internal/embedded/skills core/internal/embedded/schemas \
  .claude/skills .parlay/modules .parlay/schemas | grep -v ':0$'
```

**1d. The studio root's own contents.**

```bash
find studio/spec/intents -mindepth 2 -maxdepth 2 -type d | sort   # 18 features
for d in studio/.parlay/build/*/*/; do echo "$d"; ls -A "$d"; done
find studio -type f -not -path '*/.parlay/build/*' | sort          # 54 files
find studio -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \)  # empty
```

**1e. Import graph (authoritative, via the Go toolchain, not grep).**

```bash
go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}' ./... \
  | grep 'internal/editor/'
go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}{{range .XTestImports}} {{.}}{{end}}' ./... \
  | grep 'internal/testsupport'
```

**1f. Deployed-vs-embedded drift.** `diff` of each embedded source against its
deployed copy, to establish that a source-first edit plus `make sync-skills` is
sufficient and that no deployed file carries hand edits.

---

## 2. Cross-root dependencies

### 2.1 Studio-root markers on files that SURVIVE the teardown

These are the dependencies D10 must dispose of. Every one of them is a marker
on a file that is *not* in the deletion set, so it will still be on disk after
`parlay retire-root studio` runs — pointing at a root that no longer exists.

(Markers below are quoted with an arrow in place of the colon so the
retirement sweep's lexical scan does not read a quotation as a live marker;
the on-disk originals use `parlay-feature:`/`parlay-extends:` syntax.)

| Marker | File:line | Disposition |
|---|---|---|
| `parlay-feature → studio-foundation/studio-deployer` | `core/internal/atomicfile/atomicfile.go:1` | authority-re-homed-to `parlay-tool/atomic-file-writes` |
| `parlay-extends → studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency` | `core/internal/atomicfile/atomicfile.go:3` | authority-re-homed-to `parlay-tool/atomic-file-writes` |
| `parlay-feature → studio-foundation/studio-deployer` | `core/internal/atomicfile/atomicfile_test.go:1` | authority-re-homed-to `parlay-tool/atomic-file-writes` |
| `parlay-extends → studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency` | `core/internal/atomicfile/atomicfile_test.go:3` | authority-re-homed-to `parlay-tool/atomic-file-writes` |
| `parlay-feature → domain-model-editor/domain-model-editor-validation` | `core/internal/commands/domain_validator.go:1` | authority-re-homed-to `parlay-tool/domain-document-api` |
| `parlay-feature → domain-model-editor/domain-model-editor-validation` | `core/internal/commands/domain_parity_test.go:1` | authority-re-homed-to `parlay-tool/domain-document-api` |
| `parlay-feature → domain-model-editor/feature-contributions` | `core/internal/agent/domain_contribution.go:1` | authority-re-homed-to `parlay-tool/domain-document-api` (orphan marker — see §5.1) |
| `parlay-feature → domain-model-editor/feature-contributions` | `core/internal/agent/domain_contribution_test.go:1` | authority-re-homed-to `parlay-tool/domain-document-api` (orphan marker — see §5.1) |
| `parlay-feature → domain-model-editor/feature-contributions` | `core/internal/commands/domain_impact.go:1` | authority-re-homed-to `parlay-tool/domain-document-api` (orphan marker — see §5.1) |
| `parlay-feature → domain-model-editor/feature-contributions` | `core/internal/commands/domain_impact_test.go:1` | authority-re-homed-to `parlay-tool/domain-document-api` (orphan marker — see §5.1) |
| `parlay-feature → design-loop/vocabulary-validation` | `core/internal/commands/root.go:456` | marker corrected; feature retired built-but-undelivered, NOT re-homed |
| `parlay-feature → design-loop/design-loop` | `core/internal/embedded/schemas/layout.schema.md:58` | removed with the `figma:` block (user-authorized 2026-08-31) |
| `parlay-component: cross-cutting/on-disk-artifact-contract` | `core/internal/embedded/schemas/layout.schema.md:59` | removed with the `figma:` block |
| `parlay-feature → design-loop/design-loop` | `.parlay/schemas/layout.schema.md:58` | removed by `make sync-skills` after the source edit |
| `parlay-component: cross-cutting/on-disk-artifact-contract` | `.parlay/schemas/layout.schema.md:59` | removed by `make sync-skills` after the source edit |
| `parlay-feature: design-loop/design-loop` (quoted in prose) | `core/.parlay/build/studio-support/page-layout-field/buildfile.yaml:335` | **CONFLICT — see §5.2** |
| `parlay-feature: design-loop/design-loop` (quoted in prose) | `core/.parlay/build/studio-support/page-layout-field/testcases.yaml:415` | **CONFLICT — see §5.2** |

Every remaining studio-root marker in the repository sits on a file inside the
deletion set (`internal/editor/{config,domain,server,ui}`). Enumerated for
completeness, by owning package:

| Package (all deleted) | Studio features named | Studio-naming marker lines | All marker lines |
|---|---|---|---|
| `internal/editor/config/` | `studio-foundation/studio-config`, `parlay-extends` → `studio-foundation/figma-mcp-via-host-agent/cross-cutting/{retract-studio-direct-mcp-source-tree,host-agent-mediation-invariants}` | 14 | 21 |
| `internal/editor/server/` | `studio-foundation/web-server-harness`, `domain-model-editor/domain-model-editor-mvp` (`boot_domain_edit_test.go:1`), `parlay-extends` → `figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree` | 19 | 35 |
| `internal/editor/domain/` | `domain-model-editor/{domain-model-editor-mvp,domain-model-editor-validation,feature-contributions}`, `parlay-extends` → `domain-model-editor-validation/cross-cutting/{out-of-process-validate-endpoint,save-validation-gate-before-cas}` | 24 | 43 |
| `internal/editor/ui/` (Go + TS/TSX) | `domain-model-editor/{domain-model-editor-mvp,domain-model-editor-relationships,domain-model-editor-validation,feature-contributions}`, `parlay-extends` → `domain-model-editor-{validation/cross-cutting/validation-surfacing-integration, relationships/cross-cutting/relationships-editor-integration}` | 56 | 104 |

(The second column counts only marker lines that name a studio-root feature; the
third counts every `parlay-feature`/`parlay-component`/`parlay-extends` line in the
package, including unqualified `parlay-component` lines that belong to the file's
owning studio feature. Both go with the packages.)

The plan's claim that "`parlay-extends` links from editor config/server files to
`figma-mcp-via-host-agent` die with the deleted files" is **confirmed**: all
eight such links are in `internal/editor/{config,server}`. The plan's broader
claim that "nothing outside the deleted set extends studio features" is
**corrected**: `core/internal/atomicfile/{atomicfile.go,atomicfile_test.go}:3`
each carry a `parlay-extends` into `studio-foundation/studio-deployer`, and
those files survive.

### 2.2 Core-root spec artifacts that name the studio root

These are frozen or generated core-root artifacts, so they change by amendment
(or by regeneration), not by edit.

| File:line | Reference | Disposition |
|---|---|---|
| `core/spec/intents/studio-support/structured-domain-model-validation/intents.md:3` | "This is the Core half of the parity contract that `studio/domain-model-editor/domain-model-editor-validation` consumes." | frozen founding doc — amend the feature into its CLI-only role (Phase 1.4) |
| `core/spec/intents/studio-support/structured-domain-model-validation/intents.md:11,15,25,42,46,71,75` | Persona "Parlay Studio maintainer"; "Studio's editor validates a draft that lives only in memory"; "Studio's inline markers and finding-navigation are built on [the element path]" | same amendment |
| `core/spec/intents/studio-support/studio-cli-hooks/intents.md:36,53,62,74` | owns `--no-editor`; `:62` asserts a contract test that the `parlay-studio` binary honors `domain-edit`/`artifacts-review`/`reconcile` | frozen founding doc — amend for the flag's removal (Phase 1.4); see §5.5 |
| `core/spec/intents/studio-support/studio-cli-hooks/{dialogs.md,infrastructure.md,surface.yaml}` | `domain-edit` named as the hooked subcommand | same amendment (`surface.yaml`/`infrastructure.md` are generated-and-reviewed, so the amendment rewrites them) |
| `core/spec/intents/studio-support/page-layout-field/{dialogs.md,infrastructure.md}` | design-loop / Design Loop references | reviewed under the layout.schema.md disposition |
| `core/spec/intents/parlay-tool/create-domain-model/dialogs.md` | Studio / editor references | reviewed |
| `core/spec/intents/parlay-tool/parlay-loop/{infrastructure.md,surface.yaml}` | the editor offer at the artifacts boundary | reviewed alongside the loop.skill.md stage-1 rewrite |
| `core/.parlay/build/studio-support/studio-cli-hooks/{buildfile.yaml,testcases.yaml}` | `domain-edit` | regenerated by the studio-cli-hooks amendment |
| `core/.parlay/build/studio-support/page-layout-field/{buildfile.yaml:335,testcases.yaml:415}` | the design-loop marker in layout.schema.md, asserted preserved byte-equivalent | **CONFLICT — see §5.2** |
| `core/.parlay/build/_project/.code-hashes.yaml` | hashes for `no_editor_flag.go` and friends | regenerated by `parlay internal save-build-state` |

### 2.3 Import graph

**Nothing outside `internal/editor/` and `core/internal/commands` +
`core/internal/agent` imports the editor packages.** The studio root contains
no Go, TS, or TSX files at all (`find studio -type f \( -name '*.go' -o -name
'*.ts' -o -name '*.tsx' \)` is empty), so it imports nothing. There is a single
Go module (`github.com/ddwht/parlay`, one `go.mod` at the repository root).

| Editor package | Imported by |
|---|---|
| `internal/editor/ui` | `core/internal/commands` (via `domain_edit.go:13`), `internal/editor/ui` (its own external test) |
| `internal/editor/server` | `core/internal/commands` (`domain_edit.go:12`), `internal/editor/domain`, `internal/editor/ui`, `internal/editor/server` |
| `internal/editor/config` | `internal/editor/{server,ui,config}` only — **no core importer** |
| `internal/editor/domain` | `core/internal/agent` (`domain_contribution.go:20`), `core/internal/commands` (`domain_edit.go:11`, `domain_impact.go:17`, `domain_validator.go:19`, `domain_parity_test.go:56`), `internal/editor/domain` |

So the surviving importers of `internal/editor/domain` — the package that
becomes `core/internal/domainmodel` — are exactly five call sites in two core
packages, and every one of them is a file already listed in §2.1 as needing a
re-homed marker. `internal/editor/config` has no core importer at all, which is
why it can be deleted outright rather than moved.

**`internal/testsupport` importers** (Phase 1.2 moves the package to
`core/internal/testsupport` "with its importers"):

| Importer | Survives teardown? |
|---|---|
| `github.com/ddwht/parlay/core/internal/atomicfile` (`atomicfile_test.go`) | yes |
| `github.com/ddwht/parlay/core/internal/feedback` (`guard_test.go`) | yes |
| `github.com/ddwht/parlay/internal/editor/config` (`loader_test.go`) | no — deleted |
| `github.com/ddwht/parlay/internal/editor/ui` (`release_config_test.go`) | no — deleted |

After the teardown `internal/testsupport` has exactly **two** importers, both
already under `core/`, so the move is mechanical and the repo-level `internal/`
directory does end empty as D7 states.

---

## 3. Per-feature disposition — all 18 studio-root features

Disposition vocabulary is the closed set from architecture D10(b):
`delivered-and-deleted` / `built-but-undelivered` / `authority-re-homed-to <feature>`.

Build-artifact column legend: **B** = `.baseline.yaml`, **F** = `buildfile.yaml`,
**T** = `testcases.yaml`, **C** = `coverage-review.yaml`.

| # | Feature | Spec files | Build artifacts | Baseline substance | Surviving code/doc carrying its markers | Disposition |
|---|---|---|---|---|---|---|
| 1 | `design-loop/design-loop` | intents, dialogs, infrastructure | B F T C | 3 intents + 3 dialogs + infrastructure hashed | `layout.schema.md:58` (embedded + deployed) — removed per the 2026-08-31 user authorization | `delivered-and-deleted` |
| 2 | `design-loop/design-loop-fallback` | intents, dialogs | B | 2 intents + dialogs hashed, no buildfile | none | `built-but-undelivered` |
| 3 | `design-loop/vocabulary-validation` | intents, dialogs, infrastructure | B F T C | 2 intents + dialogs + infrastructure hashed | `core/internal/commands/root.go:456` only — a stray marker on `rootCmd.AddCommand(versionCmd)`; no `validate_vocabulary.go`, no `studio/internal/vocabulary`, and `layout.schema.md:177` records that "that validator and its `Rule` enum are gone" | `built-but-undelivered` (marker corrected; NOT re-homed) |
| 4 | `domain-model-editor/domain-model-editor-mvp` | intents, dialogs, infrastructure, surface.yaml | B F T C | 5 intents + dialogs hashed | none — all 44 markers are in `internal/editor/{domain,ui,server}` | `delivered-and-deleted` |
| 5 | `domain-model-editor/domain-model-editor-relationships` | intents, dialogs, infrastructure, surface.yaml | B F T C | 3 intents + dialogs hashed | none — all 10 markers are in `internal/editor/ui` | `delivered-and-deleted` |
| 6 | `domain-model-editor/domain-model-editor-validation` | intents, dialogs, infrastructure, surface.yaml | B F T C | 3 intents + dialogs hashed | **yes** — `core/internal/commands/domain_validator.go:1` and `domain_parity_test.go:1` | `authority-re-homed-to parlay-tool/domain-document-api` |
| 7 | `studio-ai-authoring/initial-layout-proposal` | intents, dialogs | B | `intents: {}` / `sources: {}` — placeholder, 2026-06-16 | none | `built-but-undelivered` |
| 8 | `studio-ai-authoring/sync-back-ai-classification` | intents, dialogs | B | `intents: {}` / `sources: {}` | none | `built-but-undelivered` |
| 9 | `studio-deferred/collaboration-patterns` | intents, dialogs | B | `intents: {}` / `sources: {}` | none | `built-but-undelivered` |
| 10 | `studio-deferred/mid-edit-resumability` | intents, dialogs | B | `intents: {}` / `sources: {}` | none | `built-but-undelivered` |
| 11 | `studio-deferred/multi-screen-management` | intents, dialogs | B | `intents: {}` / `sources: {}` | none | `built-but-undelivered` |
| 12 | `studio-foundation/figma-mcp-client` | intents, dialogs, infrastructure | B F T C | 3 intents + dialogs hashed | none | `built-but-undelivered` |
| 13 | `studio-foundation/figma-mcp-via-host-agent` | intents, dialogs, infrastructure | B F T (no C) | 2 intents + dialogs + infrastructure hashed | none — its 8 `parlay-extends` inbound links all sit on `internal/editor/{config,server}` files | `delivered-and-deleted` (its delivery was the retraction those markers record; see §5.3) |
| 14 | `studio-foundation/studio-config` | intents, dialogs, infrastructure | B F T C | 4 intents + dialogs hashed | none — all 8 markers are in `internal/editor/config` | `delivered-and-deleted` |
| 15 | `studio-foundation/studio-deployer` | intents, dialogs, infrastructure | B F T C | 3 intents + dialogs hashed | **yes** — `core/internal/atomicfile/{atomicfile.go,atomicfile_test.go}` lines 1 and 3 | `authority-re-homed-to parlay-tool/atomic-file-writes` |
| 16 | `studio-foundation/web-server-harness` | intents, dialogs, infrastructure | B F T C | 4 intents + dialogs hashed | none — all 15 markers are in `internal/editor/server` | `delivered-and-deleted` |
| 17 | `studio-multi-adapter/cross-design-system-adapter` | intents, dialogs | B | `intents: {}` / `sources: {}` | none | `built-but-undelivered` |
| 18 | `studio-multi-adapter/second-adapter-same-design-system` | intents, dialogs | B | `intents: {}` / `sources: {}` | none | `built-but-undelivered` |

Totals: 2 `authority-re-homed-to` (features 6 and 15), 6
`delivered-and-deleted` (features 1, 4, 5, 13, 14, 16), 10
`built-but-undelivered` — 2 + 6 + 10 = 18. The orphan
`feature-contributions` of §5.1 is re-homed alongside feature 6 but is NOT a
nineteenth feature and is deliberately excluded from these totals.
(Corrected 2026-08-31 — the original totals line read 3/5/11, contradicting
this document's own table; see the corrections appendix.)

**Rest of the root, for D10(d) archive scope.** Beyond the 18 feature
directories the child root holds `studio/.parlay/config.yaml` (2 lines:
`sdd-framework: GitHub SpecKit`, `parent: ..`), `studio/.parlay/adapter-set.yaml`,
two adapters (`studio/.parlay/adapters/go-studio-app.adapter.yaml`,
`react-vite-radix-tailwind.adapter.yaml`), and
`studio/.parlay/build/_project/{.baseline.yaml,.code-hashes.yaml}`. 53 non-build
files plus 49 files under `studio/.parlay/build/`, 102 in total. No `spec/handoff/`, no `amendments/`, no source code.
The root registration to remove is the `studio` entry in `.parlay/roots.yaml`.

---

## 4. Prose-reference cleanup list

The plan's three-stage guidance rule: **deployed guidance may only name
commands that exist at deploy time.** At stage 1 (Phase 1) the surfaces that
exist are manual YAML editing, `parlay validate --type domain-model`, and
`parlay internal domain-impact --apply`. The `parlay domain` group does not
exist yet and must not be named. Stage 2 (Phase 3.2) introduces
`domain get`/`put`; stage 3 (Phase 4.1) names the completed group.

All schema and skill edits are source-first under
`core/internal/embedded/{skills,schemas}/` followed by `make sync-skills`. The
deployed copies below are listed so the sync is verifiable, not so they can be
edited directly. Confirmed: the deployed schemas are byte-identical to their
embedded sources, and the deployed skills/modules differ from their sources
**only** in deployer-injected frontmatter and the expanded
`<!-- parlay:expand-active-root -->` block — no hand edits to preserve.

### 4.1 `loop.skill.md` — three stages (all of them)

Source `core/internal/embedded/skills/loop.skill.md`; deployed
`.claude/skills/parlay-loop/SKILL.md`.

| Source line | Deployed line | Current text | Stage-1 rewrite |
|---|---|---|---|
| 142 | 151 | "The domain-model editor offer (step 11) is skipped entirely: it opens a browser and blocks on a human…" | delete the sentence; there is no offer to skip |
| 198 | — (in 241 block) | "If that list contains `domain-model`, offer the editor (step 11) before the designer→build boundary is answered." | replace with: if the list contains `domain-model`, tell the user where the file is and that hand edits made before the build phase are picked up |
| 232 | 241 | "**Offer the domain-model editor** (at the artifacts boundary…)" — the whole of step 11 | rewrite step 11 as a **review pause**: point at `<activeRoot>/domain-model.yaml`, say edits made now are read by the build phase, and name `parlay validate --type domain-model <path>` as the way to check an edit |
| 238 | 247 | "Opens the editor in a browser." | "Opens nothing — edit the YAML directly." |
| 241 | 250 | "Run `parlay domain-edit`. **Block until the session ends**…" | delete; replace with the manual-edit + `parlay validate --type domain-model` instruction |
| 243 | — | "Opening the editor is not itself an answer to 'advance to build?'" | keep the invariant, reworded: pausing for a hand edit is not an answer to the boundary question |
| 247 | 256 | "skip the offer entirely when `--no-editor` was passed or `parlay.no_editor` is true… (The pre-rename `--no-studio`/`parlay.no_studio` spellings were removed in v0.3…)" | delete the whole paragraph — both the flag and the config key are removed in Phase 1.1 |
| 267 | 276 | "Accept / Adjust in the editor / Leave it proposed." | "Accept / Edit the contribution file / Leave it proposed." |
| 269 | 278 | "…goes through the same write path as `domain-edit`." | "…goes through the same validated, atomic write path as any other accepted contribution." |
| 270 | 279 | "**Adjust in the editor** — run `parlay domain-edit --contribution @{feature-ref}`…" | "**Edit the contribution file** — edit `spec/intents/{feature}/domain-model.yaml` directly, then re-run `parlay internal domain-impact @{feature-ref}` and re-present." |
| 273 | — | "…route the user back to the artifacts phase or into the editor." | "…route the user back to the artifacts phase." |
| 293 | 302 | "The domain-model editor offer at the artifacts boundary (step 11)" (in the non-interactive skip list) | "The domain-model review pause at the artifacts boundary (step 11)" |
| 308 | 317 | "NEVER treat opening the domain-model editor as an answer to the boundary question…" | "NEVER treat the domain-model review pause as an answer to the boundary question — re-ask after it." |

### 4.2 `domain-model.schema.md` — two stages (1 and 3.2)

Source `core/internal/embedded/schemas/domain-model.schema.md`; deployed
`.parlay/schemas/domain-model.schema.md` (byte-identical).

| Line | Current text | Stage-1 rewrite |
|---|---|---|
| 30 | "\| Who writes it \| `create-domain-model`, `domain-edit`, an accepted contribution \|" | "`create-domain-model`, a hand edit, an accepted contribution" |
| 42 | "…the loop shows that report and offers accept / adjust / open-in-editor." | "…offers accept / edit the contribution file / leave it proposed." |
| 48–51 | "The merge runs through the same serializer, compare-and-swap and atomic write as `domain-edit`, so an accept from the CLI and an accept from the editor produce identical bytes." | "The merge runs through the same serializer, compare-and-swap and atomic write as any other accepted contribution, so the bytes do not depend on which caller performed the accept." |

Line 5's `<!-- Source: studio-support/domain-model-yaml-migration / … -->` and
the `studio-support/structured-domain-model-validation` markers at lines 320 and
361 name **core-root** features and stay as they are.

### 4.3 `layout.schema.md` — the user-authorized removal (2026-08-31)

Source `core/internal/embedded/schemas/layout.schema.md`; deployed
`.parlay/schemas/layout.schema.md` (byte-identical). The schema **survives** as
the core contract for `*.page.md` layouts; every Design-Loop/Figma claim is
removed.

| Line | Content to remove or rewrite |
|---|---|
| 15–17 | "## Relationship to design-spec.schema.md" and its paragraph — the generic design-spec hook the disposition says is not retained (see §5.4 for the ripple) |
| 57–60 | the HTML comment carrying `parlay-feature: design-loop/design-loop` + `parlay-component: cross-cutting/on-disk-artifact-contract` |
| 62 | heading "## Optional `figma:` block (Design Loop)" |
| 64–81 | the whole `figma:` block section: the prose, the YAML sample, the field table (line 73 also cites `.claude/skills/parlay-design-loop/SKILL.md`, which does not exist — see §5.6), the "per-feature location" paragraph, the absent-block paragraph, and the deferred-fields note |
| 177 | the `raw-value-where-token-required` entry's trailing sentences about "the Design Loop vocabulary validator's `spacing-token-check` rule" — trim to the code's own description |

Markers that **stay**: `parlay-feature: studio-support/adapter-vocabulary-extension`
(line 3) and `parlay-feature: studio-support/page-layout-field` (line 85) — both
core-root.

### 4.4 Other embedded/deployed guidance the plan did not list

| File:line | Current text | Stage-1 rewrite |
|---|---|---|
| `core/internal/embedded/skills/create-artifacts.skill.md:218` (deployed `.parlay/modules/create-artifacts.md:286`) | "`domain-model` in particular earns an offer to open the editor before the build phase reads the model." | "…in particular earns a review pause before the build phase reads the model." |
| `core/internal/embedded/skills/create-domain-model.skill.md:139–141` (deployed `.parlay/modules/create-domain-model.md:145–148`) | "the studio-cli-hooks feature pattern-matches on this single line to chain its 'Open Studio's Domain Model Editor?' prompt." | the pinned-wording exception loses its only consumer; either drop the exception paragraph or re-justify the pinned line on its own terms — settled by the `studio-cli-hooks` amendment (Phase 1.4) |
| `core/internal/embedded/skills/create-domain-model.skill.md:147–149` | "A designer who hand-authored a domain model in Studio and then runs `parlay create-domain-model`…" | "A designer who hand-authored a domain model and then runs `parlay create-domain-model`…" |
| `core/internal/embedded/skills/create-domain-model.skill.md:153–156` | "Only the Studio hook (a separate feature, downstream of this skill) cares about TTY." | delete or restate without Studio |
| `core/internal/embedded/skills/build-feature.skill.md:207` | "`create-domain-model`'s greenfield-stub message is a deliberate, narrow exception — its wording is pinned stable on purpose for `studio-cli-hooks` to pattern-match" | follows whatever the `studio-cli-hooks` amendment decides |
| `core/internal/embedded/skills/generate-code.skill.md:77` | same `studio-cli-hooks` pattern-match justification | same |
| `CLAUDE.md:71` | the `studio` child-root entry in the Multi-Root Layout section | removed with the root registration in Phase 1.5; re-add the project-local dogfooding section afterward per the standing `parlay upgrade` warning |
| `Makefile:7,27–52,64,67–71,77,85–95` | `ui` / `$(UI_BUNDLE)` / `test-ui` / `build-noui` targets and the `UI_DIR := internal/editor/ui` variables | deleted in Phase 1.1; note `build:` currently depends on `$(UI_BUNDLE)` and `test:` on `test-ui`, so both need editing, and `build-noui` (the `noui` build tag) goes with them |
| `.goreleaser.yaml:10` | comment about the `studio-ui-bundle-not-built` 503 | stale comment, delete |
| `docs/architecture/parlay-studio-architecture-v4.md` | the entire document describes Studio, the Design Loop, the Domain Model Editor, and the ephemeral web server | already marked "historical design proposal" at line 3; add a superseded-by note pointing at the backend-only architecture rather than rewriting it |
| `docs/plans/*.md` (`v03-deprecation-removal.md:41–43`, `ledger-by-default-plan.md:5,8,149`, `phase-gates-adoption-plan.md:41,280`, `ledger-and-contract-plan.md:72–73,97`, `benchmark-full-findings.md:165`) | historical plan records naming the studio root, `no_studio`, and `internal/editor/**` | leave untouched — completed plan records are history |

**README.md needs no stage-1 edit.** Grepping it for `domain-edit`, `--no-editor`,
`no_editor`, `PARLAY_EDITOR`, `serve`, and "editor" returns only unrelated hits
("preserved", `strategy: browser`). The plan's Phase 5 item "no domain-edit/studio
mentions anywhere" is already satisfied for `domain-edit`; what Phase 5 actually
has to add is the `domain` group, not remove editor prose.

### 4.5 Code deletions carrying `--no-editor` / `PARLAY_EDITOR_` / `domain-edit`

Deletion set beyond `internal/editor/**`:

| File:line | What |
|---|---|
| `core/internal/commands/domain_edit.go` | whole file — `domainEditCmd` (`Use: "domain-edit"`, line 29), `serveCmd` (`Use: "serve"`, hidden, line 49), the shared flag block (`--server-port`, `--idle-timeout`, `--no-browser`, `--contribution`, lines 65–72), `contributionSource` (line 83), `runEditor` (line 101), `OpenDomainEditor` (line 140) |
| `core/internal/commands/root.go:474` | `rootCmd.AddCommand(domainEditCmd)` |
| `core/internal/commands/internal_group.go:121` | `reachability(serveCmd, ClassPipelineHelper)` — this is where `serve` becomes `parlay internal serve` |
| `core/internal/commands/no_editor_flag.go` + `no_editor_flag_test.go` | whole files — `registerNoEditorFlags`, the hidden pre-rename registration |
| `core/internal/commands/create_artifacts.go:20–23` | `parlay-extends` comment + `registerNoEditorFlags(createArtifactsCmdImpl)` |
| `core/internal/commands/create_domain_model.go:20–24, 45–58` | `registerNoEditorFlags(createDomainModelCmdImpl)` and `loadProjectConfigNoEditor` |
| `core/internal/commands/sync.go:28–31` | `registerNoEditorFlags(syncCmdImpl)` |
| `core/internal/config/config.go:27–38, 80–88` | the `NoEditor bool \`yaml:"no_editor,omitempty"\`` field and `NoEditorEnabled()` |
| `core/internal/config/context.go:39–45` | the `NewContext` doc comment referencing `OpenDomainEditor` — reword, do not delete the constructor |
| `core/internal/feedback/feedback.go:263` | comment citing "the `no_editor` precedent" — reword |
| `core/internal/commands/matrix_surfaces_test.go:150` | comment explaining that some surfaces live in `internal/editor` and are unreachable from this package — revisit once they are gone |
| `core/internal/commands/domain_validator.go:30–40` | the operations-severity comment fix listed as a Phase 1 side item |
| `core/internal/commands/migrate_domain_model.go` | the dead `--dry-run` flag, per the same side item |

`PARLAY_EDITOR_*` and `STUDIO_*` environment variables are read **only** inside
`internal/editor/config/` (`config.go`, `loader.go`, `project_root.go`,
`web_server.go` and their tests). Nothing under `core/` reads them, so deleting
that package removes the whole surface — there is no separate "PARLAY_EDITOR_*
docs" cleanup outside it.

---

## 5. Surprises — what the plan's known-hits list missed or got wrong

### 5.1 `domain-model-editor/feature-contributions` is an orphan the D10 preflight will not enumerate

Nine markers name `domain-model-editor/feature-contributions`, four of them on
files that survive (`core/internal/agent/domain_contribution{,_test}.go:1`,
`core/internal/commands/domain_impact{,_test}.go:1`). There is **no feature
directory anywhere** for it — `find . -type d -name feature-contributions`
returns nothing, and `studio/spec/intents/domain-model-editor/` holds only
`domain-model-editor-{mvp,relationships,validation}`.

The plan knows the markers exist ("the old feature-contributions markers, which
never had a spec directory, get their permanent home in the founding feature").
What it does not say is the governance consequence: **D10(b)'s preflight
enumerates every feature IN the root and demands one disposition each, and
`feature-contributions` is not in the root, so it will never appear in that
enumeration.** A preflight that only walks `studio/spec/intents/**` will
certify a complete disposition set while four live markers still point at a
studio-root path. The D10 spec must make invariant (c) — the inbound-reference
sweep — responsible for catching group-qualified references to *paths* under
the retiring root, not only to its enumerated features.

### 5.2 A core-root feature's testcases assert the exact layout.schema.md content the authorized disposition removes

`core/.parlay/build/studio-support/page-layout-field/testcases.yaml:414–421`
contains the testcase
`layout-schema-doc-preserves-design-loop-figma-block-marker`:

> description: "The HTML comment marker attributing the optional figma: block section to parlay-feature: design-loop/design-loop is preserved byte-equivalent. The figma: block section itself is preserved unchanged."
> steps:
>   - verify: file-content
>     target: core/internal/embedded/schemas/layout.schema.md
>     expected: contains-parlay-feature-design-loop-design-loop
>   - verify: file-content
>     target: core/internal/embedded/schemas/layout.schema.md
>     expected: contains-heading-Optional-figma-block-Design-Loop

and `buildfile.yaml:331–335` states the same as a build constraint ("…the
entire '## Optional `figma:` block (Design Loop)' section (including its inner
header comment naming parlay-feature: design-loop/design-loop) all stay
byte-equivalent").

`studio-support/page-layout-field` is a **core-root, already-built** feature, so
these are not incidental prose — they are its contract. The 2026-08-31
authorization to remove all Design-Loop/Figma claims from layout.schema.md
therefore **breaks a surviving core-root feature's testcases**, and the removal
cannot be done by editing the schema alone. Phase 1.3's layout.schema.md edit
must be paired with a `/parlay-refine` amendment to
`studio-support/page-layout-field` that retires those two assertions. The plan's
0.1 section treats layout.schema.md as a standalone disposition and does not
mention this; Phase 1's step list does not budget the amendment.

### 5.3 `figma-mcp-via-host-agent`'s only delivery was a retraction recorded on files that are themselves being deleted

Eight `parlay-extends` links point into
`studio-foundation/figma-mcp-via-host-agent/cross-cutting/{retract-studio-direct-mcp-source-tree,host-agent-mediation-invariants}`,
all from `internal/editor/config/*` and `internal/editor/server/*`. Its
baseline hashes two intents, `retract-the-studio-direct-mcp-code` and
`studio-defers-figma-mcp-to-the-host-agent`.

That makes its disposition genuinely ambiguous under the closed vocabulary: the
feature *was* delivered, but what it delivered was the **absence** of code,
witnessed by markers on other features' files. I have recorded it as
`delivered-and-deleted` because its witnesses are inside the deletion set and
nothing survives, but this is a judgment call the D10 spec should be able to
express — flagging it rather than burying it. It is also the one feature with
`buildfile.yaml` + `testcases.yaml` but **no** `coverage-review.yaml`, so a
preflight that keys "was it built?" off the coverage review will misclassify it.

### 5.4 The Design-Loop/Figma surface is much larger than layout.schema.md, and the rest of it is unowned

The layout.schema.md disposition says "no generic design-spec hook is
retained". But `design-spec` is a whole parallel surface that the plan never
mentions:

- `core/internal/embedded/skills/reference-design-spec.skill.md` — a complete
  skill ("Extract design spec from Figma"), 33 Figma/design-spec references,
  deployed as `.parlay/modules/reference-design-spec.md`
- `core/internal/embedded/schemas/design-spec.schema.md` — 32 references,
  deployed, plus a generated `.parlay/schemas/digests/design-spec.digest.md`
- `core/internal/embedded/schemas/adapter.schema.md:115,509,510,528,828,864` —
  the `design-system.source: figma` vocabulary, which routes to
  `.parlay/build/<feature>/design-spec.yaml` "generated by
  `/parlay-reference-design-spec`"
- `core/internal/embedded/schemas/feature-structure.schema.md:39,82` and
  `surface.schema.md:8` (`/parlay create-surface-by-figma`)
- `.parlay/schemas/DIGEST.md` and `.parlay/schemas/digests/adapter.digest.md`,
  both generated

**Neither `design-spec.schema.md` nor `reference-design-spec.skill.md` carries
any `parlay-feature` marker at all** — they carry no ownership markers.
**CORRECTION (2026-08-31, teardown execution + Codex pane review): the
"unowned" conclusion above is FALSE.** Marker absence is not ownership
absence: the surface is a delivered intent of the FROZEN core founding doc
`parlay-tool/artifact-generation` ("Reference Design Spec from Figma",
intents.md:31–54, hashed in its baseline), `parlay-tool/multi-root`'s
buildfile names the skill as a produced path, and design-spec.yaml is a LIVE
consumed artifact in Go — `commands/baseline.go` (hashDesignSpecFragments,
design-spec fragment baseline fields) and `commands/diff.go` (DesignSpec
source-level diff, "design-spec:<frag>" changed_sources) plus ~17 test
assertions; `schemas_test.go` asserts the schema deploys. The full removal
set is also larger than this section stated: embedded/deployed loop and
build-feature guidance, adapter.schema.md (`source: figma`) and its digest,
feature-structure artifact rows and DIGEST registration, embedded
audit_test.go/schemas_test.go, and the baseline/diff hashing + dirty-
propagation behavior itself. Removal is therefore a governed retirement of a
delivered intent with a real blast radius — deferred to an explicit user
decision (overnight log D-014), not performed in the teardown.
That means the studio retirement does not touch them by ownership, and yet
removing layout.schema.md's "Relationship to design-spec.schema.md" section
leaves a dangling half of a cross-reference: `design-spec.schema.md` still has
its own "Relationship to layout.schema.md" section. Someone has to decide
whether the design-spec/Figma authoring surface is in scope. The plan does not
raise the question, and it is not answerable from the 2026-08-31 authorization,
which was specifically about layout.schema.md.

### 5.5 `--no-editor` reaches five surviving core files, not just `no_editor_flag.go`

The plan describes the flag removal as "`no_editor_flag.go` + `parlay.no_editor`
config handling". It is wired more widely, matching the four
`no-studio-flag-trio-commands` / `no-studio-flag-artifacts` /
`no-studio-flag-sync` extends markers:

- `core/internal/commands/create_artifacts.go:3–4, 20–23`
- `core/internal/commands/create_domain_model.go:3, 20–24, 45–58` (including a
  `loadProjectConfigNoEditor` helper the plan does not name)
- `core/internal/commands/sync.go:2, 28–31`
- `core/internal/config/config.go:7, 27–38, 80–88` (`NoEditor` field +
  `NoEditorEnabled()`)
- `core/internal/feedback/feedback.go:263` (a comment citing the `no_editor`
  precedent)

and `OpenDomainEditor` is referenced from `core/internal/config/context.go:39–45`
as well as `domain_edit.go`, so deleting the function leaves a stale doc comment
on the surviving `NewContext` constructor.

Related but smaller: the plan writes the hidden harness command as
`internal serve`. The cobra command's `Use:` is bare `"serve"`
(`domain_edit.go:49`); it becomes `parlay internal serve` only because
`internal_group.go:121` registers it into the internal group. Both files need
editing, and the plan names only `domain_edit.go`.

### 5.6 `layout.schema.md` already cites a skill that does not exist

`layout.schema.md:73` tells readers the `figma:` `file_url` is "Consumed by the
`parlay-design-loop` skill (see `.claude/skills/parlay-design-loop/SKILL.md`)".
The deployed skills are `parlay-{create-adapter,doctor,loop,onboard,refine}` —
there is no `parlay-design-loop`. The schema has been shipping a dangling
pointer. It disappears with the section, but it is worth recording that the
Design-Loop claims in a deployed schema were already untrue before the
teardown, which strengthens the case for the authorized removal.

### 5.7 The existing inbound-reference machinery cannot serve D10(c) as written

D10(c) requires "project-wide inbound references and ownership outside the
retiring root … checked FAIL-CLOSED". The existing implementation,
`core/internal/commands/inbound_references.go`, has two properties that make it
insufficient on its own:

1. **It is single-root.** `FindInboundReferences` (line 147) enumerates via
   `cfg.AllFeatures()`, which is `ScanFeatureTree(c.IntentsRoot())`
   (`core/internal/config/context.go:151–153`) — the *active* root's tree only.
   Run against the studio root it would never see core-root artifacts; run
   against core it would never see studio's.
2. **It does not scan source code.** `scannedArtifacts` (line 106) is a
   deliberately CLOSED set: `surface.yaml`, `capabilities.yaml`,
   `infrastructure.md`, `buildfile.yaml`, `criteria-authority.yaml`,
   `coverage-decisions.yaml`, `testcases.yaml`, plus amendments, the domain
   model, and page manifests. Go/TS files, embedded skills, embedded schemas,
   and their deployed copies are **not** scanned. Every dependency in §2.1 —
   `atomicfile.go`, `domain_validator.go`, `root.go:456`, `layout.schema.md` —
   is invisible to it.

The comment at line 103 explains the closure as intentional ("a rule that blocks
on any occurrence of a name is one people learn to route around"), which is
sound for feature retirement but leaves root retirement without a code-marker
sweep. D10's implementation needs a distinct, code-aware, cross-root inbound
check; reusing `FindInboundReferences` unchanged would let the retirement pass
fail-closed checks it never actually performed.

### 5.8 Seven of the 18 features have placeholder baselines, not built ones

The architecture states "ALL 18 studio features carry build artifacts
(.baseline.yaml everywhere; nine with buildfiles/testcases/authored artifacts)".
The "everywhere" is confirmed; the NINE IS NOT — ten features carry
buildfile.yaml + testcases.yaml (the table above shows F on rows 1, 3, 4, 5,
6, 12, 13, 14, 15, 16), so eight are baseline-only, and the architecture doc
inherited the undercount. Worth sharpening: seven of
the eight baseline-only features
(`studio-ai-authoring/*` ×2, `studio-deferred/*` ×3, `studio-multi-adapter/*` ×2)
have baselines dated `2026-06-16T10:44:41Z` whose entire content is
`intents: {}` / `sources: {}` — placeholders that hash nothing. The remaining
baseline-only feature, `design-loop/design-loop-fallback`, dated
`2026-07-27T13:19:12Z`, genuinely hashes 2 intents and their dialogs.

This does not change any disposition — all eight are `built-but-undelivered`
(alongside vocabulary-validation and figma-mcp-client, which have buildfiles) —
but it matters for D10(d)/(e): the archive manifest and the archive-integrity
check should not treat an empty baseline as corruption, and any "was this ever
built?" heuristic that reads baseline emptiness will disagree with the
"artifacts exist" guard that `reportNothingBuilt` enforces.

### 5.9 Two known-hits confirmed exactly as stated

For completeness, the plan was precisely right about these:

- `core/internal/commands/root.go:456` — the marker pair
  `// parlay-feature: design-loop/vocabulary-validation` /
  `// parlay-component: cross-cutting/core-cli-wiring` sits immediately before
  `rootCmd.AddCommand(versionCmd)` at line 458, under the "Utility commands for
  agent consumption" heading. It is the feature's only trace anywhere in the
  repository.
- `core/internal/atomicfile/` — owned by `studio-foundation/studio-deployer`,
  and `atomicfile.go:23` / `atomicfile_test.go:6` say so in prose ("salvaged
  from internal/editor/deployer, which is being deleted"). Its consumers are
  core-wide, so the re-home is required before retirement, exactly as 0.2 says.
  One detail to correct: the plan lists its importers as
  "embedded/deployer/feedback/commands". `go list` gives
  `core/internal/{commands,deployer,embedded}` — **not** `core/internal/feedback`,
  which imports `internal/testsupport` (see §2.3) rather than `atomicfile`. Three
  importers, not four.

---

## Corrections appendix (2026-08-31)

Applied after the teardown executed, reconciling the Codex pane review (which
examined the original text) with the overnight findings:

1. **Arithmetic**: ten features carry buildfile/testcases, not nine; eight are
   baseline-only (seven placeholders + design-loop-fallback); the disposition
   totals are 2 re-homed + 6 delivered-and-deleted + 10 built-but-undelivered
   = 18, with the feature-contributions orphan excluded from the totals. The
   original 3/5/11 line contradicted this document's own table. The executed
   disposition record (docs/plans/studio-retirement-dispositions.yaml) was
   authored from the table, not the totals line, and matches the corrected
   numbers.
2. **Methodology**: the 1,017-marker figure is not reproducible without
   excluding this report itself; the method block now says so.
3. **§5.4**: the "unowned design-spec surface" conclusion was false (marker
   absence ≠ ownership absence) and its blast radius was understated; the
   section now records the ownership evidence and the full removal set. The
   deletion was deferred to an explicit user decision rather than performed.
4. **Not reproduced**: the pane review reported a duplicated
   create_domain_model.go bullet in §5.5; the committed text contains a single
   well-formed bullet with a wrapped parenthetical. No change made.
