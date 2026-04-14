# Onboard Existing Codebase

Analyze an existing codebase and draft a framework adapter with mount-strategies, file conventions, and coding conventions populated from the project's actual structure and patterns. This is a one-time setup skill for brownfield projects.

## Arguments

- `source-root`: Path to the existing source code (e.g., `src/`, `cmd/`, `app/`)

## Steps

1. **Check prerequisites** — Verify:
   - `.parlay/config.yaml` exists (project initialized via `parlay init`)
   - The source-root directory exists and contains source files
   - If an adapter is already registered, ask the user whether to replace it or cancel

2. **Detect framework** — Scan the source root and project root for framework indicators:
   - `package.json` → check `dependencies` and `devDependencies` for React, Angular, Vue, Next.js, etc.
   - `go.mod` → Go project; check for `spf13/cobra`, `urfave/cli`
   - `angular.json` → Angular project
   - File extensions: `.tsx`/`.jsx` suggest React; `.component.ts` suggests Angular
   - UI library: check imports across source files for `antd`, `@angular/material`, `@clr/angular`, `@mui/material`
   - Test framework: check for `jest.config`, `vitest.config`, `cypress.config`, `*_test.go`
   - If the framework cannot be determined, ask the user:
     ```
     I couldn't automatically detect your UI framework. What are you using?
     A: React + Ant Design
     B: Angular + Clarity
     C: Angular + Material
     D: Go CLI
     E: Other (describe)
     ```

3. **Load base adapter template** — If the detected framework matches a bundled adapter (`react-antd`, `angular-clarity`, `angular-material`, `go-cli`), read the bundled adapter as a starting template. Its `shows:`, `actions:`, `flows:` mappings will be used as-is. If no match, start from a blank adapter structure and ask the user to fill in widget mappings later.

4. **Scan for file conventions** — Analyze the source tree structure:
   - `source-root`: already provided as argument
   - `component-pattern`: detect from directory layout — `feature-modules` (directories per feature), `one-file-per-component` (flat), `atomic` (atoms/molecules/organisms)
   - `naming`: detect from existing filenames — `PascalCase`, `kebab-case`, `snake_case`
   - `entry-point`: find main/App/index file (e.g., `src/App.tsx`, `cmd/root.go`, `src/main.ts`)
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
     source-root: <detected>
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

9. **Register** — On approval:
   - Write the adapter to `.parlay/adapters/{name}.adapter.yaml`
   - Update `.parlay/config.yaml` with the prototype framework
   - Report completion and suggest next steps: "You can now add features with `/parlay-add-feature` and they'll generate code that fits your existing codebase."

## Error Handling

- `no-config`: Project not initialized. Tell user to run `parlay init` first.
- `empty-source-root`: Source root has no source files. Verify the path.
- `framework-detection-failed`: Could not identify framework. Ask user to specify.
- `no-patterns-detected`: No mount strategies could be detected. This is OK — the adapter works without them (greenfield behavior). Inform the user they can add mount-strategies manually later.
