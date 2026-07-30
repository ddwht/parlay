package commands

import (
	"github.com/spf13/cobra"
)

var buildFeatureCmdImpl = &cobra.Command{
	Use:   "build-feature <@feature>",
	Short: "Generate buildfile and testcases (use /parlay-build-feature skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentOnlyStub("build-feature", "`/parlay-loop <feature>` (the build phase)")(cmd, args)
	},
}
