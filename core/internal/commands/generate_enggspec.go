package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var generateEnggspecCmdImpl = &cobra.Command{
	Use:   "generate-enggspec <@feature>",
	Short: "Generate engineering specification (use /parlay-generate-enggspec skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "generate-enggspec requires an AI agent.")
		fmt.Fprintln(cmd.OutOrStdout(), "Use the /parlay-generate-enggspec skill in your AI agent (e.g., Claude Code).")
		return nil
	},
}
