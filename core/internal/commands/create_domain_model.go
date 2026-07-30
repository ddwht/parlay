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
	// --no-studio: skip the Studio open-editor prompt at the end. The
	// flag has no inverse --studio form; config-level disables can
	// only be reverted by changing config.
	createDomainModelCmdImpl.Flags().BoolVar(
		&noStudioFlag, "no-studio", false, noStudioFlagHelpText,
	)
}

// runCreateDomainModelHandler is the RunE for `parlay
// create-domain-model`. The actual extraction is the AI agent's
// responsibility; invoked from a shell this command can only refuse.
//
// The Studio hook dispatch that used to follow the refusal is gone.
// Its own comment claimed it "fires after the main work above completes
// successfully" — but the main work was a printed notice, so the hook fired
// on a run that had produced nothing, offering to open an editor on a domain
// model that had not been created. Refusing and then running a
// success-path side effect is the same confusion as refusing and exiting 0.
//
// The hook belongs on the path where the model is actually written, which is
// the agent's; when that lands it should call runStudioPromptForDomainModel
// directly. The helper and its wording table are kept for that caller.
func runCreateDomainModelHandler(cmd *cobra.Command, args []string) error {
	return agentOnlyStub("create-domain-model", "`/parlay-loop domain-model`")(cmd, args)
}

// pickDomainModelPromptMode chooses the brownfield vs greenfield
// wording. The actual signal is whether the AI skill that ran above
// produced a populated model from extractable signals (brownfield)
// or wrote an empty stub (greenfield). The CLI surface here cannot
// see that decision directly — the agent is the one with the
// signal — so we default to "brownfield" (the longer-standing case)
// when invoked through the bare CLI. The agent invokes this command
// with the appropriate mode set via a future flag or sentinel; for
// now both wordings are functionally available via the wording
// table.
func pickDomainModelPromptMode() DomainModelPromptMode {
	return DomainModelPromptBrownfield
}

// loadProjectConfigNoStudio reads parlay.no_studio from the active
// root's config.yaml. Returns false on any read error — a missing
// or unreadable config is treated as "not opted out", same as if
// the key were absent. The merge with the --no-studio flag is OR,
// so a malformed config never silently changes the default.
func loadProjectConfigNoStudio(pctx *config.Context) bool {
	if pctx == nil {
		return false
	}
	cfg, err := pctx.LoadProjectConfig()
	if err != nil || cfg == nil {
		return false
	}
	return cfg.NoStudio
}
