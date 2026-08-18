---
name: parlay-onboard
description: "Parlay: Onboard existing codebase and draft adapter"
---

# Onboard Existing Codebase

Analyze an existing codebase and draft the framework adapter(s) it needs — with mount-strategies, file conventions, and coding conventions populated from the project's actual structure and patterns — then pin the topology in `.parlay/adapter-set.yaml`. Handles both homogeneous projects (one presentation adapter) and **heterogeneous** ones where the frontend is one technology and the backend another (e.g. React + NestJS + Prisma), drafting a per-kind adapter for each layer. This is a one-time setup skill for brownfield projects.

## Arguments

- `source-root`: Path to the existing source code (e.g., `src/`, `cmd/`, `app/`)

The drafted adapter records that as **two** fields, and the split matters most here because the drafter is guessing at someone else's layout:

- `project-root` — the deployable project location. `.` for a single-package repo; `apps/web` or `packages/ui` in a monorepo. An adapter-set target's `root:` substitutes for this.
- `source-root` — the framework's conventional directory *inside* that project: `src/`, `src/app/`, `cmd/`. Never substituted.

Split the argument you were given at the package boundary. `apps/web/src` in a monorepo is `project-root: apps/web` + `source-root: src`; a bare `src/` at the repo root is `project-root: "."` + `source-root: src/`. Guessing this wrong is not cosmetic — collapsing both into one field is what made three of the four bundled presets emit outside their own build.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Steps

1. **Check prerequisites** — Verify:
   - `.parlay/config.yaml` exists (project initialized via `parlay init`)
   - The source-root directory exists and contains source files
   - If an adapter is already registered, ask the user whether to replace it or cancel

2. **Detect the stack — presentation AND backend** — A project may be homogeneous (one framework, one source root) or **heterogeneous** (frontend one technology, backend another — e.g. React + NestJS + Prisma). Scan for indicators of every layer, and record which adapter *kind* each detected technology fills: `presentation`, `transport`, `application`, `persistence`.
   - **Presentation** (UI): `package.json` deps for React, Angular, Vue, Next.js; `angular.json`; `.tsx`/`.jsx` (React) or `.component.ts` (Angular); UI library imports (`antd`, `@angular/material`, `@clr/angular`, `@mui/material`); `go.mod` with `spf13/cobra` (Go CLI).
   - **Application** (backend orchestration): `@nestjs/*` in `package.json` + `*.controller.ts`/`*.service.ts`/`*.module.ts`; `fastapi` in `requirements.txt`/`pyproject.toml`; Go HTTP handlers.
   - **Persistence** (storage/ORM): `prisma/schema.prisma` + `@prisma/client`; `typeorm`; SQLAlchemy models.
   - **Transport** (wire/protocol, optional): an OpenAPI spec, `@nestjs/swagger`, gRPC `.proto` files.
   - **Source roots**: in a monorepo, each layer often lives under its own root (`apps/web`, `apps/api`, `packages/*`). Record the root each kind emits into — these become the `targets.<kind>.root` values.
   - Test framework: `jest.config`, `vitest.config`, `cypress.config`, `*_test.go`.
   - If a layer's technology cannot be determined, ask the user (offer the bundled options — presentation: React+Ant Design, Angular+Clarity, Angular+Material, Go CLI; application: NestJS, FastAPI; persistence: Prisma+Postgres, TypeORM). A single-stack frontend-only project fills only the `presentation` slot — that is the normal single-target case and needs no backend adapters.

3. **Load base adapter template** — If the detected framework matches a bundled adapter template, read it as a starting point. Its `shows:`, `actions:`, `flows:` mappings will be used as-is. If no bundled template matches, start from a blank adapter structure and ask the user to fill in widget mappings later.

4. **Scan for file conventions** — Analyze the source tree structure:
   - `source-root`: already provided as argument
   - `component-pattern`: detect from directory layout — `feature-modules` (directories per feature), `one-file-per-component` (flat), `atomic` (atoms/molecules/organisms)
   - `naming`: detect from existing filenames — `PascalCase`, `kebab-case`, `snake_case`
   - `entry-point`: find main/App/index file (e.g., `src/App.tsx`, `cmd/root.go`, `src/main.ts`)
   - **`paths:` templates** — derive one per artifact kind from the layout you just detected (`component`, `test`, and whichever of `model`/`service`/`types`/`feature-routes`/`routes` the tree actually has), written **relative to `project-root` + `source-root`** (so they start below the framework directory, not with it) with `{feature}`/`{name}`/`{entity}` placeholders. This is what makes `plan:` derivable; an adapter without it generates a feature whose plan has to be hand-written, and every hand-written plan has drifted from what codegen emitted.
   - **`packages:`** — the shared-code directories (`components`, `hooks`, `utils`, `core`) if the tree has them. Distinct from `paths:`: it names where reusable code lives, which is what `parlay simplify` needs to place an extracted helper.
   - Update the adapter's `file-conventions:` section with detected values

5. **Scan for conventions** — Read 5-10 representative component files to extract coding patterns:
   - State management: Redux, React Context, signals, services, Zustand, useState-only
   - Data fetching: axios, fetch, React Query, HttpClient, custom hooks
   - Error handling: try/catch patterns, error boundaries, notification systems
   - Event naming: `on{Action}{Target}`, `handle{Action}`, other patterns
   - Import style: named vs default exports, barrel exports, path aliases
   - Write detected patterns as adapter `conventions:` entries with `rule:` and `applies-to:` fields
   - When the codebase is inconsistent (e.g., some modules use Redux, others use Context), note the dominant pattern and mention the exception in the rule

6. **Detect mount strategies** — Scan source files for common integration patterns. For each detected pattern:
   - **Tabbed pages**: search for `<Tabs`, `<TabPane`, `<Tab`, `clr-tabs`, `mat-tab-group`
   - **Route definitions**: search for `<Route`, `RouterModule.forChild`, `path:`, `loadChildren`, `AddCommand(`
   - **Navigation menus**: search for `<Menu`, `<Menu.Item`, `clr-vertical-nav`, `mat-nav-list`
   - **Sidebars**: search for `<Sider`, `<aside`, `clr-vertical-nav-group`
   - **Collapsible sections**: search for `<Collapse`, `<Accordion`, `clr-accordion`, `mat-accordion`
   - For each detected pattern:
     - Record which files it appears in and how many instances
     - Extract a representative instance from the source code
     - Generate a mount-strategy entry with `detection` (the pattern used to find it), `template` (generalized from the example instance with `{{placeholders}}`), and `description`
   - If the base adapter template already has mount-strategies for detected patterns, keep the template's version (it's more polished). Only add strategies for patterns NOT already in the template.

7. **Draft the adapter** — Assemble the complete adapter YAML:
   - Start with the base template (from step 3) or blank structure
   - Override `file-conventions:` with values detected in step 4
   - Override `conventions:` with patterns detected in step 5
   - Add any new `mount-strategies:` entries from step 6
   - Keep the base template's `shows:`, `actions:`, `flows:`, `compositions:`, `design-system:`, and `patterns:` unless the user's codebase uses different widgets or patterns

8. **Present for review** — Show the drafted adapter to the user section by section:
   ```
   I've analyzed your codebase. Here's the drafted adapter:

   Framework: <detected framework>

   File conventions:
     project-root: <detected package location, or ".">
     source-root: <detected framework directory inside it>
     component-pattern: <detected>
     naming: <detected>
     entry-point: <detected>

   Mount strategies detected:
     <strategy-name>: Found <detection> in <N> files (<file list>)
     ...

   Conventions:
     <convention-name>: "<rule>" (detected in <N> files)
     ...

   A: Register this adapter
   B: Let me review and edit the YAML first
   C: Re-scan with a different source root
   ```

8.5. **Draft backend adapters (heterogeneous projects only)** — For each non-presentation kind detected in step 2, draft a separate adapter. Backend adapters differ from presentation adapters in shape:
   - Set `kind:` to the layer (`application`, `persistence`, `transport`) — presentation adapters omit `kind:` (it defaults to presentation).
   - **They carry a `supports:` block, not `shows:`/`actions:`/`flows:`.** A non-presentation adapter declares which closed-vocabulary `operation_kinds`, `steps`, `policies`, and `errors` it can generate (drawn from `operation-kinds.schema.md`, `steps.schema.md`, `policies.schema.md`, `errors.schema.md`). Populate it from what the framework can actually do — start from the bundled `nestjs-application` / `prisma-postgres` / `openapi-rest` templates if the detected framework matches, and narrow to what the codebase demonstrates.
   - `file-conventions.paths` for a backend adapter is feature/entity-driven: an application adapter declares `service`/`controller`/`module` templates keyed off `{feature}`; a persistence adapter declares a shared `model:` schema template. See `adapter.schema.md` "Backend (non-presentation) path keys".
   - Do NOT run vocabulary (`shows`/`actions`/`flows`) or mount-strategy detection on backend adapters — those are presentation concerns.

9. **Register** — On approval:
   - Write each drafted adapter to `.parlay/adapters/{name}.adapter.yaml` (the presentation adapter plus any backend adapters from step 8.5).
   - **Write `.parlay/adapter-set.yaml`** pinning the topology: one `targets.<kind>` entry per detected layer, each naming its adapter slug and the `root:` it emits into, plus a `links:` block authorizing the cross-kind edges the stack uses (`presentation → application` via `calls`, `application → persistence` via `persists`, etc.). This replaces the deprecated `config.yaml prototype-framework:` field — do not write `prototype-framework:` (removed in v0.3; a config declaring it fails validation with `prototype-framework-unsupported`). A frontend-only project still writes `adapter-set.yaml` with a single `presentation` slot.
   - Report completion and suggest next steps: "You can now add features with `/parlay-add-feature` and they'll generate code that fits your existing codebase — each target emitting into its own root."

## Error Handling

- `no-config`: Project not initialized. Tell user to run `parlay init` first.
- `empty-source-root`: Source root has no source files. Verify the path.
- `framework-detection-failed`: Could not identify framework. Ask user to specify.
- `no-patterns-detected`: No mount strategies could be detected. This is OK — the adapter works without them (greenfield behavior). Inform the user they can add mount-strategies manually later.
