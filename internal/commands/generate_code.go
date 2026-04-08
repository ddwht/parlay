package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var generateCodeCmdImpl = &cobra.Command{
	Use:   "generate-code <@feature>",
	Short: "Generate prototype code from buildfile (use /parlay-generate-code skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("generate-code requires an AI agent.")
		fmt.Println("Use the /parlay-generate-code skill in your AI agent (e.g., Claude Code).")
		return nil
	},
}
