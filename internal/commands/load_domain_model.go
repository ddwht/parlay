package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loadDomainModelCmdImpl = &cobra.Command{
	Use:   "load-domain-model <path>",
	Short: "Load domain model (use /parlay-load-domain-model skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("load-domain-model requires an AI agent.")
		fmt.Println("Use the /parlay-load-domain-model skill in your AI agent (e.g., Claude Code).")
		return nil
	},
}
