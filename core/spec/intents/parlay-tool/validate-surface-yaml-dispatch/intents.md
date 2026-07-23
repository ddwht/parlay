# Validate surface YAML dispatch

> `parlay validate --type surface` always runs the legacy Markdown fragment-heading parser regardless of the target file's extension, so it fails on any `surface.yaml` with the hardcoded error `surface.md has no fragment headings (## )`. The YAML surface parser already exists — `parlay validate --type yaml <same file>` returns OK — but `--type surface` never dispatches to it by extension. The consequence is concrete: the `/parlay-migrate-spec` skill's own step-3 parity check tells the designer to "run `parlay validate --type surface` on the new YAML to confirm parity," and that command cannot succeed on the very artifact the skill just produced. This feature makes `--type surface` select the right validator from the file's shape so a `surface.yaml` validates as a surface.

---

## `--type surface` validates surface.yaml through the YAML surface validator

**Goal**: Make `parlay validate --type surface <path>` succeed on a well-formed `surface.yaml`, applying the same structural checks the surface model requires, while continuing to validate a legacy `surface.md` through the Markdown parser exactly as today. The caller names the artifact kind (`surface`); the tool figures out the serialization.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: In `core/internal/commands/validate.go` the `case "surface":` branch (around line 125) unconditionally assigns `validator = agent.ValidateSurface`. `agent.ValidateSurface` (in `core/internal/agent/validate.go`, around line 47) treats its input as Markdown text: it returns `surface.md has no fragment headings (## )` when the content contains no `## ` heading, and `surface.md has no **Shows**: fields` when it finds no `**Shows**:` marker. A `surface.yaml` has neither marker, so it always fails. Meanwhile `case "yaml":` maps to `agent.ValidateYAML`, which parses the same file cleanly — proving the project already carries a YAML-shaped surface validator path; the `surface` type just never routes to it. The `/parlay-migrate-spec` skill (step 3, "Verify") instructs `parlay validate --type surface` on the freshly-emitted YAML, so the skill's documented flow is un-followable on its own output.
**Action**: In `runValidate`, before invoking the surface validator, classify the target as Markdown-surface vs YAML-surface and dispatch accordingly. Classification is deterministic — prefer the file extension (`.yaml`/`.yml` → YAML surface, `.md` → Markdown surface); when the extension is absent or ambiguous, sniff the content (a leading `---` document / mapping keys → YAML; `## ` fragment headings → Markdown). The YAML branch runs a surface-shaped structural check (valid YAML that decodes to the surface model — fragments with the required fields), not merely `ValidateYAML`'s "is this parseable YAML" check. The Markdown branch keeps `agent.ValidateSurface` byte-for-byte.
**Objects**: validate-command, surface-type, surface-yaml, surface-md, validator-dispatch, migrate-spec-parity-check

**Constraints**:
- Dispatch is a pure function of the path and content — no adapter, no network, no AI.
- A legacy `surface.md` that validates today continues to validate identically; error codes and messages for the Markdown path are unchanged (`surface.md has no fragment headings (## )`, `surface.md has no **Shows**: fields`).
- A `surface.yaml` that decodes to a valid surface model validates OK; a `surface.yaml` that is parseable YAML but not a valid surface (e.g., missing fragments, a fragment missing its shows) fails with a surface-specific code, not the Markdown "no fragment headings" message.
- The YAML-surface error messages must not reference `surface.md` — an author validating a `.yaml` file should never be told about `## ` fragment headings they cannot add to a YAML document.
- `parlay validate --type yaml <surface.yaml>` continues to work as the generic-YAML escape hatch; this feature does not remove or change it.

**Verify**:
- `parlay validate --type surface path/to/surface.yaml` on a valid migrated surface returns OK (exit 0), where today it fails with `surface.md has no fragment headings (## )`.
- `parlay validate --type surface path/to/surface.md` on a valid legacy surface returns OK with byte-identical behavior to today (no regression).
- `parlay validate --type surface path/to/surface.yaml` on a YAML file that parses but is not a valid surface fails with a surface-specific code whose message names the missing surface element, and never mentions `## ` headings or `surface.md`.
- Following `/parlay-migrate-spec` end-to-end — migrate a `surface.md`, then run the skill's step-3 `parlay validate --type surface` on the emitted `surface.yaml` — completes cleanly with no manual substitution of `--type yaml`.
