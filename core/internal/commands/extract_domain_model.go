package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var extractDomainModelCmdImpl = &cobra.Command{
	Use:   "extract-domain-model",
	Short: "Extract domain model (use /parlay-extract-domain-model skill)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("extract-domain-model requires an AI agent.")
		fmt.Println("Use the /parlay-extract-domain-model skill in your AI agent (e.g., Claude Code).")
		return nil
	},
}
