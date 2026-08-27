// parlay-feature: parlay-tool/criterion-authority
// parlay-component: boundary-claim-registry
//
// What each boundary claims to check, declared once.
//
// Every gate stage was assembled by hand: a function calling checkers in
// sequence, each free to be added, forgotten, or wired to a path nothing takes.
// That is how this release shipped a hard blocker nothing could reach, a
// freshness rule whose findings were discarded, a graduation no artifact could
// trigger, and a waiver recorded for runs that never advanced — seven times, in
// checks whose entire purpose was refusing what they let through.
//
// The registry makes the boundary's own composition the thing under test.
// Stages are lists of claim IDs, so a checker that is not registered is not
// reachable from a gate at all, and the conformance test asserts every
// registered blocker-producing claim has a witness proving it can fire through
// the advancing constructor. Scoping that assertion to "claims we chose to
// register" would be tautological — the eighth missing path would simply go
// unregistered — so registration is where the gate gets its behaviour from
// rather than a list beside it.
//
// IDs are separate from the codes a claim emits. A wrapper like testcase
// readiness renders eight different codes, and a user-facing code may be
// renamed; neither should decide what the architecture guarantees.

package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
)

// claimResult is what one checker contributes to a boundary.
type claimResult struct {
	Blockers []gateBlocker
	Warnings []gateBlocker
	// Pending is a state effect the boundary may commit IF it advances.
	// Carried rather than performed, so evaluation stays pure and a refused
	// run cannot leave a record saying it proceeded.
	Pending *pendingMachineRun
}

// boundaryClaim is one thing a boundary checks.
type boundaryClaim struct {
	// ID is stable and internal. Codes may change; this is what a witness
	// registers against.
	ID string
	// What the claim is, in one line, for the conformance failure message.
	What string
	// Stages this claim participates in.
	Stages []string
	// Blocking is false for a claim that only ever warns, which needs no
	// witness because it cannot hold a boundary shut.
	Blocking bool
	Check    func(cfg *config.Context, slug, featurePath string) claimResult
}

// Claim IDs. Stable strings; renaming one is a deliberate act that fails the
// witness registry until the witness moves with it.
const (
	claimReadiness         = "readiness"
	claimLedgerState       = "ledger-state"
	claimAmendments        = "amendment-ledger"
	claimBuildfileValid    = "buildfile-validity"
	claimComposition       = "cross-feature-composition"
	claimBuildfileFresh    = "buildfile-freshness"
	claimCriteriaAuthority = "criteria-authority"
	claimCoverageExcept    = "coverage-exceptions"
	claimTestcasesReady    = "testcases-readiness"
	claimGeneratedState    = "generated-state"
)

// boundaryClaims is the registry every stage is assembled from.
//
// Adding a checker means adding a row: a checker called directly from a stage
// function would not appear here, and the conformance test asserts stages name
// only registered claims, so the direct call has nowhere to live.
var boundaryClaims = []boundaryClaim{
	{
		ID: claimReadiness, What: "the phase's own readiness checks",
		Stages: []string{gateStageBuild}, Blocking: true,
		Check: claimReadinessCheck,
	},
	{
		ID: claimLedgerState, What: "frozen-document integrity and the unapplied amendment tail",
		Stages: []string{gateStageBuild, gateStageCode, gateStageDone}, Blocking: true,
		Check: claimLedgerStateCheck,
	},
	{
		ID: claimBuildfileValid, What: "the buildfile parses and satisfies its schema",
		Stages: []string{gateStageCode}, Blocking: true,
		Check: claimBuildfileValidCheck,
	},
	{
		ID: claimComposition, What: "no cross-feature contradiction between buildfiles",
		Stages: []string{gateStageCode}, Blocking: true,
		Check: claimCompositionCheck,
	},
	{
		ID: claimBuildfileFresh, What: "the buildfile is not stale against its sources",
		Stages: []string{gateStageCode}, Blocking: true,
		Check: claimBuildfileFreshCheck,
	},
	{
		ID: claimCriteriaAuthority, What: "somebody approved the criteria this feature is graded against",
		Stages: []string{gateStageCode, gateStageDone}, Blocking: true,
		Check: claimCriteriaAuthorityCheck,
	},
	{
		ID: claimCoverageExcept, What: "recorded exceptions are still bound to the contract they were granted against",
		Stages: []string{gateStageCode, gateStageDone}, Blocking: true,
		Check: claimCoverageExceptCheck,
	},
	{
		ID: claimTestcasesReady, What: "the tests mechanically discharge the standard",
		Stages: []string{gateStageCode, gateStageDone}, Blocking: true,
		Check: claimTestcasesReadyCheck,
	},
	{
		ID: claimGeneratedState, What: "generated code matches what was recorded",
		Stages: []string{gateStageDone}, Blocking: true,
		Check: claimGeneratedStateCheck,
	},
}

// claimsForStage returns the claims a stage is assembled from, in registry
// order.
func claimsForStage(stage string) []boundaryClaim {
	var out []boundaryClaim
	for _, c := range boundaryClaims {
		for _, s := range c.Stages {
			if s == stage {
				out = append(out, c)
			}
		}
	}
	return out
}

// runStageClaims assembles a boundary from its registered claims.
func runStageClaims(cfg *config.Context, slug, featurePath, stage string) (blockers, warnings []gateBlocker, pending *pendingMachineRun) {
	for _, c := range claimsForStage(stage) {
		r := c.Check(cfg, slug, featurePath)
		blockers = append(blockers, r.Blockers...)
		warnings = append(warnings, r.Warnings...)
		if r.Pending == nil {
			continue
		}
		if pending != nil {
			// Two claims proposing a state effect is not something to resolve
			// by last-writer-wins: one of them would be silently dropped, and a
			// side effect nobody can account for is the shape of every defect
			// this registry exists to prevent. Only criterion authority
			// produces one today, so this is a guard against a future second
			// rather than a live case.
			blockers = append(blockers, gateBlocker{
				Code:    "boundary-conflicting-side-effects",
				Message: "more than one claim proposed a state effect at this boundary; refusing rather than dropping one silently",
			})
			continue
		}
		pending = r.Pending
	}
	return blockers, warnings, pending
}

// --- claim adapters -------------------------------------------------------
//
// Thin wrappers over the existing checkers. Two of them change behaviour, and
// deliberately: writing each family down as a claim is what exposed them.

func claimReadinessCheck(cfg *config.Context, slug, featurePath string) claimResult {
	var r claimResult
	// unapplied-amendments is stripped here because the ledger claim recomputes
	// that verdict with journal precision; counting both double-reports it and
	// inherits readiness's coarser "any journal suppresses" rule.
	for _, iss := range checkBuildFeatureReadiness(cfg, featurePath, slug) {
		if iss.Code == "unapplied-amendments" {
			continue
		}
		switch iss.Severity {
		case "error":
			r.Blockers = append(r.Blockers, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		case "warning":
			r.Warnings = append(r.Warnings, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		}
	}
	return r
}

func claimLedgerStateCheck(cfg *config.Context, slug, featurePath string) claimResult {
	b, w := gateLedgerState(cfg, slug, featurePath)
	return claimResult{Blockers: b, Warnings: w}
}

func claimBuildfileValidCheck(cfg *config.Context, slug, featurePath string) claimResult {
	var r claimResult
	cb := computeCheckBuildfile(cfg, slug)
	for _, iss := range cb.Issues {
		switch iss.Severity {
		case "error":
			r.Blockers = append(r.Blockers, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		case "warning":
			r.Warnings = append(r.Warnings, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		}
	}
	return r
}

// claimCompositionCheck reports a cross-feature contradiction.
//
// CHANGED by writing it down: computeComposition's error was discarded, so a
// project whose composition could not be computed contributed nothing and the
// boundary read that as coherence. Whether the walk failed and whether it found
// nothing are different facts, and only one of them means it is safe to
// advance.
func claimCompositionCheck(cfg *config.Context, slug, featurePath string) claimResult {
	var r claimResult
	comp, err := computeComposition(cfg)
	if err != nil {
		r.Blockers = append(r.Blockers, gateBlocker{
			Code:    "composition-unresolvable",
			Message: "cross-feature composition could not be computed: " + err.Error() + " — a walk that failed is not a project without contradictions",
			Fix:     "repair the buildfiles the error names, then re-run",
		})
		return r
	}
	for _, f := range comp.Findings {
		r.Blockers = append(r.Blockers, gateBlocker{Code: f.Code, Message: f.Message})
	}
	for _, n := range comp.Notes {
		r.Warnings = append(r.Warnings, gateBlocker{Code: n.Code, Message: n.Message})
	}
	return r
}

func claimBuildfileFreshCheck(cfg *config.Context, slug, featurePath string) claimResult {
	var r claimResult
	r.Blockers = checkBuildfileFreshness(cfg, slug, featurePath, &r.Warnings)
	return r
}

func claimCriteriaAuthorityCheck(cfg *config.Context, slug, featurePath string) claimResult {
	b, w, p := gateCriteriaAuthority(cfg, slug, machineAuthorized())
	return claimResult{Blockers: b, Warnings: w, Pending: p}
}

func claimCoverageExceptCheck(cfg *config.Context, slug, featurePath string) claimResult {
	b, w := gateCoverageExceptions(cfg, slug)
	return claimResult{Blockers: b, Warnings: w}
}

func claimTestcasesReadyCheck(cfg *config.Context, slug, featurePath string) claimResult {
	b, w := gateTestcasesReadiness(cfg, slug)
	return claimResult{Blockers: b, Warnings: w}
}

// claimGeneratedStateCheck verifies generated code against what was recorded.
//
// CHANGED by writing it down, for the same reason as composition: an error from
// computeProjectVerifyOutput contributed nothing, so a snapshot that could not
// be read was indistinguishable from code that matches it — at the boundary
// that certifies completion, which is the strongest claim the ladder makes.
func claimGeneratedStateCheck(cfg *config.Context, slug, featurePath string) claimResult {
	var r claimResult
	verify, err := computeProjectVerifyOutput(cfg)
	if err != nil {
		r.Blockers = append(r.Blockers, gateBlocker{
			Code:    "generated-state-unreadable",
			Message: "generated code state could not be established: " + err.Error() + " — completion cannot be certified over a snapshot that was not read",
		})
		return r
	}
	if verify == nil {
		return r
	}
	if !verify.HasHashes {
		r.Blockers = append(r.Blockers, gateBlocker{
			Code:    "code-not-generated",
			Message: "no generated-code hashes recorded — the code phase has not produced a blessed prototype",
			Fix:     "run /parlay-generate-code",
		})
	}
	for _, f := range verify.Modified {
		r.Blockers = append(r.Blockers, gateBlocker{
			Code:    "generated-file-modified",
			Message: fmt.Sprintf("%s differs from the last recorded emission (possible hand-edit)", f.Path),
			Fix:     "re-run /parlay-generate-code, or reconcile the edit",
		})
	}
	for _, f := range verify.Adopted {
		r.Blockers = append(r.Blockers, gateBlocker{
			Code: "generated-file-adopted", Message: fmt.Sprintf("%s was written outside codegen", f.Path),
		})
	}
	for _, f := range verify.Unknown {
		r.Blockers = append(r.Blockers, gateBlocker{
			Code: "generated-file-unknown-provenance", Message: fmt.Sprintf("%s has undeclared provenance", f.Path),
		})
	}
	for _, f := range verify.Missing {
		r.Blockers = append(r.Blockers, gateBlocker{
			Code:    "generated-file-missing",
			Message: fmt.Sprintf("%s was recorded as generated but is gone from disk", f.Path),
			Fix:     "re-run /parlay-generate-code, or drop the component",
		})
	}
	return r
}
