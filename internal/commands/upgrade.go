package commands

import (
	"fmt"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/deployer"
	"github.com/ddwht/parlay/internal/embedded"
	"github.com/spf13/cobra"
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

func runUpgrade(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("no parlay project found (run parlay init first): %w", err)
	}

	// Re-deploy schemas.
	if err := embedded.WriteSchemas(config.SchemasPath()); err != nil {
		return fmt.Errorf("failed to write schemas: %w", err)
	}
	schemaNames, _ := embedded.SchemaNames()

	// Re-deploy skills and agent config.
	skills, _ := embedded.ReadAllSkills()
	dep, err := deployer.Get(cfg.AIAgent)
	if err != nil {
		fmt.Printf("  Warning: no deployer for agent %q, using generic\n", cfg.AIAgent)
		dep, _ = deployer.Get("generic")
	}
	if dep != nil {
		if err := dep.Deploy(".", skills); err != nil {
			return fmt.Errorf("failed to deploy skills: %w", err)
		}
	}

	fmt.Printf("Upgraded to parlay %s:\n", appVersion)
	fmt.Printf("  schemas — %d updated\n", len(schemaNames))
	if len(skills) > 0 {
		fmt.Printf("  skills  — %d deployed for %s\n", len(skills), dep.Name())
	}

	return nil
}
