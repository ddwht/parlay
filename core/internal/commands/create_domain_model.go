// parlay-section: cross-cutting
// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-domain-model
// parlay-extends: parlay-tool/create-domain-model/cli-command-rename-source-file

package commands

import (
	"github.com/spf13/cobra"
)

var createDomainModelCmdImpl = &cobra.Command{
	Use:   "create-domain-model",
	Short: "Create domain model from features (use /parlay-create-domain-model skill)",
	RunE:  runCreateDomainModelHandler,
}

// runCreateDomainModelHandler is the RunE for `parlay
// create-domain-model`. The actual extraction is the AI agent's
// responsibility; invoked from a shell this command can only refuse.
//
// The open-editor prompt that used to follow the refusal is gone, and so is
// the dispatcher behind it. Its own comment claimed it "fires after the main
// work above completes successfully" — but the main work was a printed notice,
// so it fired on a run that had produced nothing, offering to open an editor on
// a domain model that had not been created.
//
// There is no editor left to offer. The CLI-side helpers, their wording table,
// the brownfield/greenfield mode enum, and the flag that silenced the prompt
// all went the same way — they had no caller and described a
// handoff to a program that no longer exists.
func runCreateDomainModelHandler(cmd *cobra.Command, args []string) error {
	return agentOnlyStub("create-domain-model", "`/parlay-loop domain-model`")(cmd, args)
}
