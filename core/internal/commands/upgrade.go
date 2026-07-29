// parlay-feature: parlay-tool/multi-root
// parlay-component: upgrade-refresh-confirmation
// parlay-extends: parlay-tool/multi-root/missing-agent-on-upgrade-error
// parlay-extends: parlay-tool/multi-root/bare-parent-upgrade-error
// parlay-cross-cutting: bare-parent-fallback-removal-in-deployToRoot

package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/deployer"
	"github.com/ddwht/parlay/core/internal/embedded"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Re-deploy schemas, skills, and agent config to match the current parlay version",
	Long: `Upgrade the project's schemas, skills, and agent configuration files
to match the version of the parlay binary. This is safe to run at any
time — it only overwrites tool-managed files and never touches project
state (config, intents, dialogs, surfaces, adapters, buildfiles, or
baselines).

Run this after updating the parlay binary (e.g., brew upgrade parlay).`,
	RunE: runUpgrade,
}

// upgradeResult is the outcome of one deployToRoot call, used by
// runUpgrade to render its summary line.
type upgradeResult struct {
	SchemaCount  int
	ModuleCount  int
	SkillCount   int
	DeployerName string
}

// ErrBareParentTopology is returned when parlay upgrade runs against a
// parent that has roots.yaml but no config.yaml. The previous code path
// silently proceeded with empty config and skipped skills; that branch
// has been removed in favour of an atomic hard-error.
var ErrBareParentTopology = errors.New("bare-parent topology: missing config.yaml")

// deployToRoot redeploys schemas, skills, and agent config at the named
// root path. The path is treated as the directory containing .parlay/.
// Used by both `parlay upgrade` and the auto-refresh hook on `parlay
// add-root`. Silent on success — callers print their own summary.
//
// Strict topology contract (multi-root v2): there is exactly one
// success path (config.yaml exists with ai-agent) and two distinct
// hard-error paths:
//   - Bare-parent (roots.yaml present, config.yaml missing) → ErrBareParentTopology
//   - Uninitialized project (neither file present) → "(run parlay init first)"
//
// The previously-tolerant fallback that proceeded with an empty config
// no longer exists — there is one path for correct topology and one
// path for hard-error. After parlay repair migrates a bare-parent
// project, the next parlay upgrade runs cleanly.
func deployToRoot(rootPath string) (upgradeResult, error) {
	cfgPath := filepath.Join(rootPath, config.ParlayDir, config.ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			rootsPath := filepath.Join(rootPath, config.ParlayDir, config.RootsIndexFile)
			if _, statErr := os.Stat(rootsPath); statErr == nil {
				// Bare-parent topology: roots.yaml present but no
				// config.yaml. Hard-error — no partial deploys, no
				// schemas, no skills.
				return upgradeResult{}, fmt.Errorf("[ERR] %w: %s is missing — run `parlay repair` to create it",
					ErrBareParentTopology, cfgPath)
			}
			// Uninitialized project: distinct from bare-parent.
			return upgradeResult{}, fmt.Errorf("read %s: %w (run parlay init first)", cfgPath, err)
		}
		return upgradeResult{}, fmt.Errorf("read %s: %w", cfgPath, err)
	}

	var cfg config.ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return upgradeResult{}, fmt.Errorf("parse %s: %w", cfgPath, err)
	}

	// Multi-root parent must declare ai-agent. There is no longer a
	// silent skip-skills fallback — see Agent-Identity Single-Source
	// Validation invariant.
	if cfg.AIAgent == "" {
		return upgradeResult{}, fmt.Errorf("[ERR] no agent identity declared at parent root.\n  - %s has no `ai-agent` field.\nAdd `ai-agent: <Claude Code|Cursor|Generic CLI>` and re-run, or run `parlay repair`",
			cfgPath)
	}

	// Re-deploy schemas.
	schemasPath := filepath.Join(rootPath, config.ParlayDir, config.SchemasDir)
	if err := embedded.WriteSchemas(schemasPath); err != nil {
		return upgradeResult{}, fmt.Errorf("write schemas: %w", err)
	}
	schemaNames, _ := embedded.SchemaNames()
	if err := writeSchemaDigest(schemasPath); err != nil {
		return upgradeResult{}, fmt.Errorf("write schema digest: %w", err)
	}

	// Re-deploy the phase modules. These are skill sources that no longer
	// appear on the agent's menu — the driver and the phase subagents load
	// them by path. They land beside the schemas because the content is
	// adapter-independent.
	modulesPath := filepath.Join(rootPath, config.ParlayDir, config.ModulesDir)
	moduleCount, err := embedded.WriteModules(modulesPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("write modules: %w", err)
	}
	if err := embedded.PruneStaleModules(modulesPath); err != nil {
		return upgradeResult{}, fmt.Errorf("prune modules: %w", err)
	}

	// Re-deploy skills and agent config. Only command-surface skills reach
	// the menu; module-surface ones were written above.
	allSkills, _ := embedded.ReadAllSkills()
	skills := embedded.CommandSkills(allSkills)
	dep, err := deployer.Get(cfg.AIAgent)
	if err != nil {
		dep, _ = deployer.Get("generic")
	}
	if dep == nil {
		return upgradeResult{SchemaCount: len(schemaNames), ModuleCount: moduleCount}, nil
	}
	if err := dep.Deploy(rootPath, skills); err != nil {
		return upgradeResult{}, fmt.Errorf("deploy skills: %w", err)
	}
	return upgradeResult{
		SchemaCount:  len(schemaNames),
		ModuleCount:  moduleCount,
		SkillCount:   len(skills),
		DeployerName: dep.Name(),
	}, nil
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	pctx := config.FromCtx(cmd.Context())
	rootPath := "."
	if pctx != nil {
		// Always upgrade at the repo-level root (parent if active is child).
		rootPath = pctx.RepoRoot()
	}

	result, err := deployToRoot(rootPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Upgraded to parlay %s:\n", appVersion)
	fmt.Fprintf(cmd.OutOrStdout(), "  schemas — %d updated\n", result.SchemaCount)
	if result.ModuleCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  modules — %d written\n", result.ModuleCount)
	}
	if result.SkillCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  skills  — %d deployed for %s\n", result.SkillCount, result.DeployerName)
	}

	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: adapter-kind-field-opt-in-prompt
	//
	// After re-deploying skills + schemas, offer the kind: opt-in for any
	// adapter file that predates the multi-adapter feature. The validator
	// already treats absent kind: as the legacy presentation default; this
	// prompt makes the default explicit.
	if err := offerAdapterKindOptIn(cmd, rootPath); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Note: adapter kind opt-in skipped (%v)\n", err)
	}

	return nil
}

// offerAdapterKindOptIn scans .parlay/adapters/ under rootPath for adapter
// files lacking an explicit kind: field, and offers to add `kind: presentation`
// to each. Skipping is non-blocking; files keep working without explicit
// kind: per the legacy default.
func offerAdapterKindOptIn(cmd *cobra.Command, rootPath string) error {
	adaptersDir := filepath.Join(rootPath, ".parlay", "adapters")
	entries, err := os.ReadDir(adaptersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var missing []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".adapter.yaml") {
			continue
		}
		path := filepath.Join(adaptersDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !hasKindField(content) {
			missing = append(missing, path)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
		// Non-TTY upgrade — print informational note, don't prompt.
		fmt.Fprintf(cmd.OutOrStdout(), "  Note: %d adapter file(s) lack explicit kind: presentation; run `parlay upgrade` interactively to opt in.\n", len(missing))
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "These adapter files predate the multi-adapter feature; the validator already treats them as kind: presentation.")
	for _, p := range missing {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s — inferred default: kind: presentation\n", p)
	}
	fmt.Fprint(cmd.OutOrStdout(), "Add explicit `kind: presentation` to all of them? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(strings.ToLower(line)) != "y" {
		return nil
	}

	updated := 0
	for _, p := range missing {
		content, _ := os.ReadFile(p)
		newContent := injectKindPresentation(content)
		if err := os.WriteFile(p, newContent, 0644); err == nil {
			updated++
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Updated\n  %d file(s) gained an explicit kind: presentation line\n", updated)
	return nil
}

// hasKindField reports whether the supplied adapter YAML content declares a
// top-level kind: field. The check is line-based rather than YAML-aware so
// it tolerates malformed adapter files (which are validated separately).
func hasKindField(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "kind:") || strings.HasPrefix(t, "kind ") {
			// Top-level only — ignore indented occurrences.
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				return true
			}
		}
	}
	return false
}

// injectKindPresentation inserts `kind: presentation` after the existing
// top-level name: line (or at the top of the file when no name: is found).
func injectKindPresentation(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "name:") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			out := append([]string{}, lines[:i+1]...)
			out = append(out, "kind: presentation")
			out = append(out, lines[i+1:]...)
			return []byte(strings.Join(out, "\n"))
		}
	}
	return []byte("kind: presentation\n" + string(content))
}
