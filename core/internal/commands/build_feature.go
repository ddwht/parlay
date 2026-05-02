package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var buildFeatureCmdImpl = &cobra.Command{
	Use:   "build-feature <@feature>",
	Short: "Generate buildfile and testcases (use /parlay-build-feature skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("build-feature requires an AI agent.")
		fmt.Println("Use the /parlay-build-feature skill in your AI agent (e.g., Claude Code).")
		return nil
	},
}
