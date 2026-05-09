// parlay-section: cross-cutting
// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-domain-model
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands
// parlay-extends: parlay-tool/create-domain-model/cli-command-rename-source-file

package commands

import (
	"fmt"

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
		&noStudioFlagDomainModel, "no-studio", false, noStudioFlagHelpText,
	)
}

// runCreateDomainModelHandler is the RunE for `parlay
// create-domain-model`. The actual extraction is the AI agent's
// responsibility — this handler exists to print the
// "use the AI skill" message and to fire the Studio hook prompt at
// the end of a successful run.
//
// The trio-command name passed into dispatch is "create-domain-model"
// — the wording table is keyed by this slug, and the surface intent's
// rename from extract-domain-model has now landed.
func runCreateDomainModelHandler(cmd *cobra.Command, args []string) error {
	fmt.Println("create-domain-model requires an AI agent.")
	fmt.Println("Use the /parlay-create-domain-model skill in your AI agent (e.g., Claude Code).")

	// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-domain-model
	// Hook-point dispatch — fires after the main work above
	// completes successfully and before the command returns. A
	// failure of the main work skips the hook entirely; we only
	// reach this point when the message above has been printed.
	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		// No active root resolved — nothing to hand off to Studio
		// against. Skip the hook silently; the command already
		// printed its own output.
		return nil
	}
	noStudio := resolveNoStudioForDomainModel(loadProjectConfigNoStudio(pctx))
	mode := pickDomainModelPromptMode()
	return runStudioPromptForDomainModel(pctx, mode, noStudio)
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
