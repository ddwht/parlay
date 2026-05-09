// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-prompt-domain-model
//
// runStudioPromptForDomainModel is the per-trio adapter the
// create-domain-model handler calls after the model file is written.
// It picks the brownfield/greenfield wording variant based on whether
// extractable signals were present, then defers to dispatchStudioHook
// for the gates and Studio handoff. Each invocation re-runs all gates
// fresh — there is no per-session memory of previous y/N answers.

package commands

import (
	"github.com/ddwht/parlay/core/internal/config"
)

// DomainModelPromptMode names the wording variant the
// create-domain-model command should ask for. Brownfield = the
// command produced a populated model from extractable signals.
// Greenfield = the command wrote an empty stub because no signals
// were found.
type DomainModelPromptMode string

const (
	DomainModelPromptBrownfield DomainModelPromptMode = "brownfield"
	DomainModelPromptGreenfield DomainModelPromptMode = "greenfield"
)

// runStudioPromptForDomainModel is invoked at the end of the
// create-domain-model handler — after the YAML is on disk. NoStudio
// is the merged --no-studio + project-config disable signal computed
// by the trio command. Returns nil on the no-prompt paths and on a
// successful Studio run; returns the wrapped error from
// dispatchStudioHook when Studio exits non-zero so the trio
// command's RunE propagates the failure.
func runStudioPromptForDomainModel(pctx *config.Context, mode DomainModelPromptMode, noStudio bool) error {
	return dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:        pctx,
		TrioCommand: "create-domain-model",
		Mode:        string(mode),
		NoStudio:    noStudio,
	})
}
