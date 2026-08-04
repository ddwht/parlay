// parlay-feature: parlay-tool/status-feature-phases
// parlay-section: cross-cutting
// parlay-source: shared-feature-phase-helper

package commands

import (
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// FeaturePhase is the furthest pipeline phase whose required on-disk
// artifacts exist for a given (root, feature) pair. The "code" phase is
// intentionally excluded — generated code is not tracked here.
type FeaturePhase string

// Phase constants. The string values are the lowercase tokens emitted in
// human and JSON output. They form the contract floor for the
// `phase` column in `parlay status` and the `phase` field in
// `parlay status --json`.
const (
	PhaseIntents   FeaturePhase = "intents"
	PhaseDialogs   FeaturePhase = "dialogs"
	PhaseArtifacts FeaturePhase = "artifacts"
	PhaseBuild     FeaturePhase = "build"
	PhaseDone      FeaturePhase = "done"

	// PhaseHandAuthored is not a rung on the ladder above — it is the
	// statement that the ladder does not apply. A hand-authored unit has
	// intents.md and nothing else the phases measure, so walking the
	// ladder pins it at "intents" forever and reports a unit that will
	// never progress as a feature stuck at the first step. Reporting a
	// permanent non-problem is how a status line stops being read.
	PhaseHandAuthored FeaturePhase = "hand-authored"
)

// ComputeFeaturePhase returns the furthest pipeline phase whose required
// on-disk artifacts exist for the (rootCtx, featureSlug) pair.
//
// Determinism / purity contract:
//
//   - Side-effect free: no os.Exit, no fmt.Println, no JSON marshalling,
//     no logging. The only I/O is os.Stat-equivalent file-existence
//     checks on a fixed set of paths derived from rootCtx.
//   - Per-root: every path is resolved through rootCtx, so the helper
//     consults that root's spec/intents/<feature>/ and
//     .parlay/build/<feature>/ — never a sibling root's tree.
//   - Idempotent: invoking with the same (rootCtx, featureSlug) and the
//     same on-disk state returns the same FeaturePhase value.
//   - Total: the return value is always one of the five exported
//     constants above.
//
// The phase ladder is monotonic — each phase is reached only when every
// earlier phase's required artifact is also present.
func ComputeFeaturePhase(rootCtx *config.Context, featureSlug string) FeaturePhase {
	if rootCtx == nil || featureSlug == "" {
		return PhaseIntents
	}
	featurePath := rootCtx.FeaturePath(featureSlug)
	buildPath := rootCtx.BuildPath(featureSlug)
	return computeFeaturePhaseAtPaths(featurePath, buildPath)
}

// computeFeaturePhaseAtPaths is the path-level core of ComputeFeaturePhase.
// It accepts pre-resolved feature and build directories so callers that
// already hold those paths (e.g. check_readiness, which receives a raw
// featurePath argument from its public API) can route through the same
// shared logic without re-deriving them through a *config.Context.
func computeFeaturePhaseAtPaths(featurePath, buildPath string) FeaturePhase {
	// Checked first: a unit carries intents.md exactly as a feature does,
	// so every rung below would otherwise read it as an ordinary feature
	// that has merely not got very far.
	if config.IsAuthoredUnit(featurePath) {
		return PhaseHandAuthored
	}
	// done — buildfile present (and everything below). The engineering
	// spec under spec/handoff/<feature>/specification.md is intentionally
	// NOT consulted; build is the terminal tracked phase.
	buildfile := filepath.Join(buildPath, "buildfile.yaml")
	hasBuild := fileExistsAt(buildfile)

	hasSurface := parser.ResolveSurfacePath(featurePath) != ""
	hasInfra := fileExistsAt(filepath.Join(featurePath, "infrastructure.md")) ||
		fileExistsAt(filepath.Join(featurePath, "capabilities.yaml"))
	hasArtifacts := hasSurface || hasInfra

	hasDialogs := fileExistsAt(filepath.Join(featurePath, "dialogs.md"))

	// Walk the ladder top-down, requiring every lower rung as well.
	if hasBuild && hasArtifacts && hasDialogs {
		return PhaseDone
	}
	if hasBuild {
		return PhaseBuild
	}
	if hasArtifacts {
		return PhaseArtifacts
	}
	if hasDialogs {
		return PhaseDialogs
	}
	return PhaseIntents
}

// fileExistsAt is the single os.Stat call backing every existence check
// in ComputeFeaturePhase. Kept private to make the purity invariant
// trivial to audit.
func fileExistsAt(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
