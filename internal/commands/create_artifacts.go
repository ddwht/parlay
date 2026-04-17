// parlay-feature: artifact-decision
// parlay-component: ArtifactDecisionPrompt

package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var createArtifactsCmdImpl = &cobra.Command{
	Use:   "create-artifacts <@feature>",
	Short: "Determine and create surface.md, infrastructure.md, or both (use /parlay-create-artifacts skill)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("create-artifacts requires an AI agent.")
		fmt.Println("Use the /parlay-create-artifacts skill in your AI agent (e.g., Claude Code).")
		fmt.Println()
		fmt.Println("The agent analyzes intents and dialogs to determine whether the feature")
		fmt.Println("needs surface.md (user-facing), infrastructure.md (behind-the-scenes), or both,")
		fmt.Println("then proceeds to create the appropriate artifacts.")
		return nil
	},
}
