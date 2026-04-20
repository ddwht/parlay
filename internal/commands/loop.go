// parlay-feature: parlay-tool/parlay-loop
// parlay-component: LoopInvocationAndFeatureResolution
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loopCmd = &cobra.Command{
	Use:   "loop <@feature> [--from <phase>]",
	Short: "Walk a feature end-to-end through the parlay design pipeline (use /parlay-loop skill)",
	Long: `Orchestrate intents → dialogs → artifacts → build → code as one continuous guided process.
Requires an AI agent with sub-agent spawning support (Claude Code, Cursor). On adapters
without sub-agent support (Generic CLI), the loop degrades to a fresh-session handoff.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("loop requires an AI agent.")
		fmt.Println("Use the /parlay-loop skill in your AI agent (e.g., Claude Code or Cursor).")
		return nil
	},
}

func init() {
	loopCmd.Flags().String("from", "intents", "Starting phase: intents | dialogs | artifacts | build | code")
}
