// parlay-feature: artifact-decision
// parlay-component: ArtifactDecisionPrompt
// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands

package commands

import (
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
	// --no-editor: skip the open-editor prompt at the end. Also binds the
	// deprecated --no-studio spelling.
	registerNoEditorFlags(createArtifactsCmdImpl)
}

// runCreateArtifactsHandler is the RunE for `parlay create-artifacts`.
// The artifact decision and emission live in the AI skill; invoked from a
// shell this command can only refuse.
//
// The trailing hook dispatch is gone with the rest of the success path. It
// was already inert — its own comment records that the Studio "artifact
// editor" it offered was designed and never built — and it sat after a
// printed notice that the code described as "the main work", so it ran on a
// run that had produced nothing.
func runCreateArtifactsHandler(cmd *cobra.Command, args []string) error {
	return agentOnlyStub("create-artifacts", "`/parlay-loop <feature>` (the artifacts phase)")(cmd, args)
}
