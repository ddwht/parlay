// parlay-feature: artifact-decision
// parlay-component: ArtifactDecisionPrompt
// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands

package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

var createArtifactsCmdImpl = &cobra.Command{
	Use:   "create-artifacts <@feature>",
	Short: "Determine and create surface.yaml/surface.md, capabilities.yaml/infrastructure.md, or both (use /parlay-create-artifacts skill)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreateArtifactsHandler,
}

func init() {
	// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands
	// --no-studio: skip the Studio open-editor prompt at the end.
	createArtifactsCmdImpl.Flags().BoolVar(
		&noStudioFlag, "no-studio", false, noStudioFlagHelpText,
	)
}

// runCreateArtifactsHandler is the RunE for `parlay create-artifacts`.
// The artifact decision and emission live in the AI skill; this
// handler exists to print the "use the AI skill" message and (post
// this feature) to fire the Studio prompt at the end of a successful
// run.
func runCreateArtifactsHandler(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "create-artifacts requires an AI agent.")
	fmt.Fprintln(cmd.OutOrStdout(), "Use the /parlay-create-artifacts skill in your AI agent (e.g., Claude Code).")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "The agent analyzes intents and dialogs to determine whether the feature")
	fmt.Fprintln(cmd.OutOrStdout(), "needs surface.md (user-facing), infrastructure.md (behind-the-scenes), or both,")
	fmt.Fprintln(cmd.OutOrStdout(), "then proceeds to create the appropriate artifacts.")

	// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-artifacts
	// Hook-point dispatch — fires after the main work prints its
	// summary above and before the command returns. A failure of
	// the main work would skip this branch.
	pctx := config.FromCtx(cmd.Context())
	// No editor hand-off here. This hook offered to open Studio's "artifact
	// editor" — a surface that was designed (see
	// spec/intents/studio-support/studio-cli-hooks/intents.md:53) and never
	// built. The prompt's whole value was saving the designer from remembering
	// a command; with nothing behind it, it was an offer that could only fail.
	_ = pctx
	return nil
}
