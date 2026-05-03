// parlay-feature: parlay-tool/multi-root
// parlay-component: upgrade-refresh-confirmation
// parlay-extends: parlay-tool/multi-root/missing-agent-on-upgrade-error
// parlay-extends: parlay-tool/multi-root/bare-parent-upgrade-error
// parlay-cross-cutting: bare-parent-fallback-removal-in-deployToRoot

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

	// Re-deploy skills and agent config.
	skills, _ := embedded.ReadAllSkills()
	dep, err := deployer.Get(cfg.AIAgent)
	if err != nil {
		dep, _ = deployer.Get("generic")
	}
	if dep == nil {
		return upgradeResult{SchemaCount: len(schemaNames)}, nil
	}
	if err := dep.Deploy(rootPath, skills); err != nil {
		return upgradeResult{}, fmt.Errorf("deploy skills: %w", err)
	}
	return upgradeResult{
		SchemaCount:  len(schemaNames),
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

	fmt.Printf("Upgraded to parlay %s:\n", appVersion)
	fmt.Printf("  schemas — %d updated\n", result.SchemaCount)
	if result.SkillCount > 0 {
		fmt.Printf("  skills  — %d deployed for %s\n", result.SkillCount, result.DeployerName)
	}

	return nil
}
