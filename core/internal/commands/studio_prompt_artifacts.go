// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-prompt-artifacts
//
// runStudioPromptForArtifacts is the per-trio adapter the
// create-artifacts handler calls after surface.md and/or
// infrastructure.md are written. It uses the "default" wording variant
// and forwards the feature context (which artifact set the prompt
// belongs to) to the parlay-studio subprocess.

package commands

import (
	"github.com/ddwht/parlay/core/internal/config"
)

// runStudioPromptForArtifacts runs at the end of the create-artifacts
// handler. featureCtx is the @feature/page reference the trio
// command was invoked against — passed through to parlay-studio
// artifacts-review as its first positional argument so the editor
// opens against the same artifact set the command just produced.
func runStudioPromptForArtifacts(pctx *config.Context, featureCtx string, noStudio bool) error {
	return dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:        pctx,
		TrioCommand: "create-artifacts",
		Mode:        "default",
		NoStudio:    noStudio,
		FeatureCtx:  featureCtx,
	})
}
