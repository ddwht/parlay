package commands

import (
	"github.com/spf13/cobra"
)

var generateEnggspecCmdImpl = &cobra.Command{
	Use:   "generate-enggspec <@feature>",
	Short: "Generate engineering specification (use /parlay-generate-enggspec skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentOnlyStub("generate-enggspec", "`/parlay-loop handoff @<feature>`")(cmd, args)
	},
}
