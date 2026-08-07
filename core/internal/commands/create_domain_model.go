// parlay-section: cross-cutting
// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-domain-model
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands
// parlay-extends: parlay-tool/create-domain-model/cli-command-rename-source-file

package commands

import (
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

var createDomainModelCmdImpl = &cobra.Command{
	Use:   "create-domain-model",
	Short: "Create domain model from features (use /parlay-create-domain-model skill)",
	RunE:  runCreateDomainModelHandler,
}

func init() {
	// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands
	// --no-editor: skip the open-editor prompt at the end. The flag has no
	// inverse --editor form; config-level disables can only be reverted by
	// changing config. Also binds the deprecated --no-studio spelling.
	registerNoEditorFlags(createDomainModelCmdImpl)
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
// The offer now lives in the loop skill, which runs on the path where the model
// is actually written. The CLI-side helpers, their wording table, and the
// brownfield/greenfield mode enum went with the dispatcher — they had no caller
// and described a handoff to a binary that no longer exists.
func runCreateDomainModelHandler(cmd *cobra.Command, args []string) error {
	return agentOnlyStub("create-domain-model", "`/parlay-loop domain-model`")(cmd, args)
}

// loadProjectConfigNoEditor reads the project-config opt-out from the
// active root's config.yaml. Returns false on any read error — a missing
// or unreadable config is treated as "not opted out", same as if the key
// were absent. The merge with the --no-editor flag is OR, so a malformed
// config never silently changes the default.
func loadProjectConfigNoEditor(pctx *config.Context) bool {
	if pctx == nil {
		return false
	}
	cfg, err := pctx.LoadProjectConfig()
	if err != nil || cfg == nil {
		return false
	}
	return cfg.NoEditorEnabled()
}
