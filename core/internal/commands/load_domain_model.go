package commands

import (
	"github.com/spf13/cobra"
)

var loadDomainModelCmdImpl = &cobra.Command{
	Use:   "load-domain-model <path>",
	Short: "Load domain model (use /parlay-load-domain-model skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentOnlyStub("load-domain-model", "`/parlay-loop domain-model --from <path>`")(cmd, args)
	},
}
