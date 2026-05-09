// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-prompt-sync
//
// runStudioPromptForSync is the per-trio adapter the sync handler
// calls after the textual coverage and drift report is produced.
// Uses the "default" wording variant and forwards the feature
// context (the @feature reference) to parlay-studio reconcile.

package commands

import (
	"github.com/ddwht/parlay/core/internal/config"
)

// runStudioPromptForSync runs at the end of the sync handler.
// featureCtx is the @feature reference the sync was run against —
// forwarded to parlay-studio reconcile so the visual reconciliation
// view opens against the same feature.
func runStudioPromptForSync(pctx *config.Context, featureCtx string, noStudio bool) error {
	return dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:        pctx,
		TrioCommand: "sync",
		Mode:        "default",
		NoStudio:    noStudio,
		FeatureCtx:  featureCtx,
	})
}
