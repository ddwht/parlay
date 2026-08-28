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
//
// Completeness is per (claim, branch), not per claim. An earlier version keyed
// on the claim alone, which read stronger than it was: a wrapper rendering
// several codes satisfied it with one witness, so adding a refusal path inside
// a wrapper required no registry change and no new witness. That was the same
// hiding place the registry was built to close, one level down — and it was
// occupied. Four paths had no witness when the per-branch check first ran.
//
// Scoping is compositional rather than per-code. A pass-through family declares
// branchPropagation only: its diagnostics have their own leaf conformance, and
// what the boundary owns is that a child blocker reaches it. Exploding those
// into every schema code would make an architecture test track diagnostic
// wording. A wrapper that decides for itself declares every path it decides on.
//
// WHAT THIS STILL DOES NOT GUARANTEE: a developer can declare a branch and
// point an existing witness at it, or reuse the wrong branch ID. Omission is
// harder, not impossible. What changed is that adding a path now requires
// touching the registry, where the omission is visible, instead of being free.

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
	// Branches are the independent refusal paths this claim owns, each of
	// which needs its own witness. A pass-through family declares only
	// branchPropagation: its diagnostics are proven by their own leaf suite,
	// and what the boundary owns is that a child blocker reaches it.
	//
	// A wrapper that decides for itself — reading a file, judging staleness,
	// aggregating a child verdict — declares every path it decides on, because
	// no leaf suite covers those and nothing else would notice one going dead.
	Branches []string
	Check    func(cfg *config.Context, slug, featurePath string) claimResult
}

// Claim IDs. Stable strings; renaming one is a deliberate act that fails the
// witness registry until the witness moves with it.
const (
	claimReadiness         = "readiness"
	claimLedgerState       = "ledger-state"
	claimBuildfileValid    = "buildfile-validity"
	claimComposition       = "cross-feature-composition"
	claimBuildfileFresh    = "buildfile-freshness"
	claimCriteriaAuthority = "criteria-authority"
	claimCoverageExcept    = "coverage-decisions"
	claimTestcasesReady    = "testcases-readiness"
	claimGeneratedState    = "generated-state"
	claimRetiredOutput     = "retired-contribution"
)

// Branch IDs name the INDEPENDENT refusal paths a claim owns.
//
// The registry proved each claim family is reachable. That is weaker than it
// read: a wrapper rendering several codes satisfied the completeness test with
// one witness, so an unreachable path added inside a wrapper required no
// registry change and no new witness. Adding a ninth branch to an eight-branch
// wrapper was free — the same hiding place the registry was built to close,
// one level down.
//
// Branches are internal and semantic. User-facing codes stay free to be renamed
// and merged as diagnostics improve; completeness keys on (claim, branch) so an
// architecture test never tracks wording.
const (
	// Pass-through families: the wrapper's own job is propagation, and the
	// diagnostics it forwards have their own leaf conformance. One witness that
	// a child blocker reaches the boundary is the whole claim.
	branchPropagation = "propagation"

	// Paths a boundary wrapper owns itself, and that no leaf suite covers.
	branchSubjectMissing      = "subject-missing"
	branchSubjectUnreadable   = "subject-unreadable"
	branchStaleState          = "stale-state"
	branchUnapproved          = "unapproved"
	branchStrandedLegacy      = "stranded-legacy"
	branchNotGenerated        = "not-generated"
	branchModified            = "modified"
	branchAdopted             = "adopted"
	branchSubjectRemoved      = "subject-removed"
	branchUnappliedTail       = "unapplied-tail"
	branchDowngradeUnapproved = "downgrade-unapproved"
	branchRetiredEmitted      = "retired-emitted"
	branchRetiredShared       = "retired-shared-path"
	branchRetiredUnaccounted  = "retired-unaccounted"
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
		Branches: []string{branchPropagation},
		Check:    claimReadinessCheck,
	},
	{
		ID: claimLedgerState, What: "frozen-document integrity and the unapplied amendment tail",
		Stages: []string{gateStageBuild, gateStageCode, gateStageDone}, Blocking: true,
		Branches: []string{branchPropagation, branchUnappliedTail},
		Check:    claimLedgerStateCheck,
	},
	{
		ID: claimBuildfileValid, What: "the buildfile parses and satisfies its schema",
		Stages: []string{gateStageCode}, Blocking: true,
		Branches: []string{branchPropagation},
		Check:    claimBuildfileValidCheck,
	},
	{
		ID: claimComposition, What: "no cross-feature contradiction between buildfiles",
		Stages: []string{gateStageCode}, Blocking: true,
		Branches: []string{branchPropagation},
		Check:    claimCompositionCheck,
	},
	{
		ID: claimBuildfileFresh, What: "the buildfile is not stale against its sources",
		Stages: []string{gateStageCode}, Blocking: true,
		Branches: []string{branchPropagation},
		Check:    claimBuildfileFreshCheck,
	},
	{
		ID: claimCriteriaAuthority, What: "somebody approved the criteria this feature is graded against",
		Stages: []string{gateStageCode, gateStageDone}, Blocking: true,
		Branches: []string{branchUnapproved, branchStaleState},
		Check:    claimCriteriaAuthorityCheck,
	},
	{
		ID: claimCoverageExcept, What: "recorded exceptions are still bound to the contract they were granted against",
		Stages: []string{gateStageCode, gateStageDone}, Blocking: true,
		Branches: []string{branchStaleState, branchSubjectUnreadable, branchStrandedLegacy},
		Check:    claimCoverageExceptCheck,
	},
	{
		ID: claimTestcasesReady, What: "the tests mechanically discharge the standard",
		Stages: []string{gateStageCode, gateStageDone}, Blocking: true,
		Branches: []string{branchSubjectMissing, branchSubjectUnreadable, branchDowngradeUnapproved},
		Check:    claimTestcasesReadyCheck,
	},
	{
		ID: claimRetiredOutput, What: "no component still emits for a fragment supersession retired",
		Stages: []string{gateStageCode, gateStageDone}, Blocking: true,
		Branches: []string{branchRetiredEmitted, branchRetiredShared, branchRetiredUnaccounted, branchSubjectUnreadable, branchSubjectMissing},
		Check:    claimRetiredOutputCheck,
	},
	{
		ID: claimGeneratedState, What: "generated code matches what was recorded",
		Stages: []string{gateStageDone}, Blocking: true,
		Branches: []string{branchSubjectUnreadable, branchModified, branchSubjectRemoved, branchNotGenerated, branchAdopted},
		Check:    claimGeneratedStateCheck,
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

// claimRetiredOutputCheck is the deletion half of cross-feature supersession.
//
// The composition signature forces a REBUILD when a sibling retires one of this
// feature's fragments; it cannot say what the rebuild owes. Without this claim a
// rebuilt buildfile may keep emitting and routing the superseded component, so
// the racing pair supersedes: exists to prevent arrives one build later rather
// than immediately.
func claimRetiredOutputCheck(cfg *config.Context, slug, featurePath string) claimResult {
	var r claimResult
	for _, f := range checkRetiredContributions(cfg, slug) {
		b := gateBlocker{Code: f.Code, Message: f.Message, Fix: f.Fix}
		if f.Severity == "warning" {
			r.Warnings = append(r.Warnings, b)
			continue
		}
		r.Blockers = append(r.Blockers, b)
	}
	return r
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
