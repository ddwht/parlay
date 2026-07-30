package commands

import (
	"github.com/spf13/cobra"
)

var generateCodeCmdImpl = &cobra.Command{
	Use:   "generate-code",
	Short: "Generate prototype code from all features' buildfiles (use /parlay-generate-code skill)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentOnlyStub("generate-code", "`/parlay-loop <feature>` (the code phase)")(cmd, args)
	},
}
