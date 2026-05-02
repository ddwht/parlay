package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/deployer"
	"github.com/ddwht/parlay/internal/embedded"
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
	SchemaCount int
	SkillCount  int
	DeployerName string
}

// deployToRoot redeploys schemas, skills, and agent config at the named
// root path. The path is treated as the directory containing .parlay/.
// Used by both `parlay upgrade` and the auto-refresh hook on `parlay
// add-root`. Silent on success — callers print their own summary.
func deployToRoot(rootPath string) (upgradeResult, error) {
	cfgPath := filepath.Join(rootPath, config.ParlayDir, config.ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("read %s: %w (run parlay init first)", cfgPath, err)
	}
	var cfg config.ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return upgradeResult{}, fmt.Errorf("parse %s: %w", cfgPath, err)
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
