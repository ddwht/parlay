package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var generateCodeCmdImpl = &cobra.Command{
	Use:   "generate-code",
	Short: "Generate prototype code from all features' buildfiles (use /parlay-generate-code skill)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("generate-code requires an AI agent.")
		fmt.Println("Use the /parlay-generate-code skill in your AI agent (e.g., Claude Code).")
		fmt.Println()
		fmt.Println("This command operates at the project level — it reads ALL features'")
		fmt.Println("buildfiles and generates code for the entire project incrementally.")
		return nil
	},
}
