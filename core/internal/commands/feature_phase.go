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
	// PhasePlanned is a feature that exists but promises nothing yet:
	// its intents.md parses to zero intents. Every feature passes
	// through it, because `add-feature` writes both founding files
	// eagerly and the scaffolded intents.md is deliberately commented
	// out so it parses empty.
	//
	// Before this rung existed the ladder asked only whether
	// dialogs.md was PRESENT, and add-feature had already created it —
	// so a feature reported "dialogs" from the moment it was created
	// and PhaseIntents was unreachable for anything made that way. A
	// rung nothing can ever occupy is not a ladder, and a brand-new
	// empty folder claiming to have authored dialogs is worse than no
	// answer at all.
	PhasePlanned   FeaturePhase = "planned"
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
//     no logging. The I/O is os.Stat-equivalent file-existence checks on
//     a fixed set of paths derived from rootCtx, plus a read-and-parse of
//     intents.md and dialogs.md for the two content-aware rungs. Reading
//     is still observation: nothing is written and nothing is cached.
//   - Per-root: every path is resolved through rootCtx, so the helper
//     consults that root's spec/intents/<feature>/ and
//     .parlay/build/<feature>/ — never a sibling root's tree.
//   - Idempotent: invoking with the same (rootCtx, featureSlug) and the
//     same on-disk state returns the same FeaturePhase value.
//   - Total: the return value is always one of the exported
//     constants above.
//
// The ladder is walked top-down, each rung carrying its own evidence
// contract. It is NOT monotonic in the strong sense of every rung
// requiring every lower rung: `build` is deliberately returned for a
// buildfile that has no artifacts, because a buildfile is itself the
// evidence that the build phase ran. Only the terminal rung asserts a
// conjunction.
func ComputeFeaturePhase(rootCtx *config.Context, featureSlug string) FeaturePhase {
	// The floor is planned, the same floor a missing feature gets.
	// A defensive return that disagreed with the ladder's own bottom
	// rung would make "planned" mean two things.
	if rootCtx == nil || featureSlug == "" {
		return PhasePlanned
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
	// done — the build phase COMPLETE (and everything below). The engineering
	// spec under spec/handoff/<feature>/specification.md is intentionally
	// NOT consulted; build is the terminal tracked phase.
	//
	// Complete means both files, not just the buildfile. The build phase emits
	// buildfile.yaml and testcases.yaml, and this rung used to ask only for the
	// first — so a run that died between them reported `done`. That is exactly
	// what a headless driver does when it ends its turn waiting for a phase
	// subagent that will never report back: buildfile written, testcases not,
	// no generated code, exit 0, and a status ladder calling it finished. The
	// one signal that could have contradicted the exit code agreed with it
	// instead.
	buildfile := filepath.Join(buildPath, "buildfile.yaml")
	hasBuild := fileExistsAt(buildfile)
	hasTestcases := fileExistsAt(filepath.Join(buildPath, "testcases.yaml"))

	hasSurface := parser.ResolveSurfacePath(featurePath) != ""
	hasInfra := fileExistsAt(filepath.Join(featurePath, "infrastructure.md")) ||
		fileExistsAt(filepath.Join(featurePath, "capabilities.yaml"))
	hasArtifacts := hasSurface || hasInfra

	// Two rungs, two different questions, deliberately asked two
	// different ways.
	//
	// Below the artifacts rung the question is "has anybody authored
	// anything yet", and presence cannot answer it: add-feature writes
	// dialogs.md eagerly, so its existence says only that the feature
	// was created. That rung reads content.
	//
	// The terminal rung measures COMPLETION EVIDENCE — artifacts plus
	// both build outputs — and checks the founding documents only for
	// structural presence. It does not read dialog content, and it
	// infers nothing from finding none: a feature can reach done
	// carrying the untouched header-only stub add-feature wrote, and
	// this rung does not pretend that stub was a deliberate
	// declaration. Content validity at that altitude belongs to the
	// build and readiness gates, which own it.
	//
	// Requiring parsed dialog turns here would be wrong for a
	// different reason: a backend feature legitimately has none.
	// `parlay-tool/structured-domain-model-validation` is fully built
	// and its dialogs.md reads "CLI/backend feature — no interactive
	// dialog turns"; an earlier cut of this function demoted it to
	// `build` for saying so. Distinguishing that deliberate case from
	// an untouched stub would need an explicit no-dialog declaration in
	// the dialog schema. Inference cannot do it, and this rung does not
	// try.
	dialogsAuthored := hasAuthoredDialogs(featurePath)

	// BOTH founding documents, not just dialogs. The terminal rung's
	// claim is structural integrity of the feature, and a feature
	// missing intents.md has none — it promises nothing, so there is
	// nothing for the build outputs to be the completion OF. Status
	// enumeration usually hides this, because a feature is discovered
	// through its intents.md in the first place, but this helper is
	// callable directly and an invariant that holds only via the
	// caller's discovery order is not one.
	intentsPresent := fileExistsAt(filepath.Join(featurePath, "intents.md"))
	dialogsPresent := fileExistsAt(filepath.Join(featurePath, "dialogs.md"))

	// Walk the ladder top-down.
	if hasBuild && hasTestcases && hasArtifacts && intentsPresent && dialogsPresent {
		return PhaseDone
	}
	if hasBuild {
		return PhaseBuild
	}
	if hasArtifacts {
		return PhaseArtifacts
	}
	if dialogsAuthored {
		return PhaseDialogs
	}
	if !hasAuthoredIntents(featurePath) {
		return PhasePlanned
	}
	return PhaseIntents
}

// hasAuthoredIntents and hasAuthoredDialogs report whether a founding
// document carries real, parsed content — not merely whether the file is
// on disk.
//
// A parse FAILURE is deliberately reported as "has content". The rungs
// below this one are the emptier ones, so treating an unreadable file as
// empty would demote a feature on the strength of a file the tool could
// not read — announcing a feature as newer and less finished than it is,
// on exactly the evidence that says the least. Presence is the
// conservative fallback: it is what the ladder used before, and it never
// makes a feature look emptier than the last answer did.
func hasAuthoredIntents(featurePath string) bool {
	return hasParsedContent(filepath.Join(featurePath, "intents.md"), func(p string) (int, error) {
		items, err := parser.ParseIntentsFile(p)
		return len(items), err
	})
}

func hasAuthoredDialogs(featurePath string) bool {
	return hasParsedContent(filepath.Join(featurePath, "dialogs.md"), func(p string) (int, error) {
		items, err := parser.ParseDialogsFile(p)
		return len(items), err
	})
}

func hasParsedContent(path string, count func(string) (int, error)) bool {
	if !fileExistsAt(path) {
		return false
	}
	n, err := count(path)
	if err != nil {
		// Unreadable, not empty. Fall back to presence.
		return true
	}
	return n > 0
}

// fileExistsAt is the single os.Stat call backing every existence check
// in ComputeFeaturePhase. Kept private to make the purity invariant
// trivial to audit.
func fileExistsAt(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// HasObservedPipelineActivity reports whether a feature shows evidence of
// having been worked on, independent of anything anybody declared.
//
// Artifacts are the line. Below it a feature may simply have been created
// and left; at or above it somebody has run a phase and the tool wrote
// something as a result. This is the fact that lets `unclassified` mean
// "nobody has said, and nothing shows" rather than the much weaker
// "nobody has said" — reporting a missing disposition for work whose
// activity is evident is the permanent non-problem that makes a listing
// stop being read.
//
// It coincides with `gate`'s notion of having reached a boundary, and
// that is not a coincidence to rely on silently: gate assigns the build
// boundary from the artifacts phase upward, so the two questions have the
// same answer today. Keeping one definition here means they cannot drift
// apart without somebody changing this function.
func HasObservedPipelineActivity(phase FeaturePhase) bool {
	switch phase {
	case PhaseArtifacts, PhaseBuild, PhaseDone:
		return true
	default:
		return false
	}
}
