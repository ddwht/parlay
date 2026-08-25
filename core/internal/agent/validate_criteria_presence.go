// parlay-feature: parlay-tool/multi-adapter
// parlay-component: criteria-presence
//
// Reports contract entries that carry no acceptance criteria at all.
//
// This is the complement of the criterion-coverage walker in
// validate_testcases_v2.go, and it exists because that walker structurally
// cannot see this condition. `verify-criterion-uncovered` fires when an entry
// CARRYING verify: has no case discharging it. An entry carrying none
// discharges nothing, demands nothing, and so draws no diagnostic: it is
// coverage-complete by vacancy. The absence of criteria is invisible; only
// their non-discharge is visible.
//
// That is what let an artifacts phase hand the build phase an untestable
// contract and have the build phase take the blame — 21 warnings about tests
// reporting a fact about the artifacts, one phase earlier. This walker runs at
// the designer->build boundary, before a testcases.yaml exists for the coverage
// walkers to read.

package agent

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/parser"
)

// CriteriaPresenceInput is a feature's contract, as much of it as resolved.
//
// HasSurface is separate from len(Fragments) deliberately: a feature with no
// surface artifact at all is a normal state (a CLI, a TUI, a pure backend
// feature), and is a different condition from a surface artifact that parsed to
// zero fragments. Only the latter is worth a word.
type CriteriaPresenceInput struct {
	Feature    string
	HasSurface bool
	Fragments  []parser.Fragment
	Operations []parser.CapabilityOperation
}

// ValidateCriteriaPresence reports entries with no verify: list, and the
// aggregate case where a feature's whole surface is vacant.
func ValidateCriteriaPresence(mode ValidationMode, in CriteriaPresenceInput) []ValidationOutcome {
	var outcomes []ValidationOutcome

	withCriteria := 0
	for _, f := range in.Fragments {
		if f.Name == "" {
			continue
		}
		if len(f.Verify) > 0 {
			withCriteria++
			continue
		}
		outcomes = append(outcomes, NewOutcome(mode, "surface-fragment-no-criteria",
			fmt.Sprintf("fragment %q carries no verify: — nothing states what it must do, so every presentation case written against it will cite nothing; add the owning intent's presentation claims (see create-artifacts § Routing acceptance criteria)", f.Name)))
	}

	for _, op := range in.Operations {
		if op.ID == "" {
			continue
		}
		if len(op.Verify) > 0 {
			continue
		}
		outcomes = append(outcomes, NewOutcome(mode, "capability-operation-no-criteria",
			fmt.Sprintf("operation %q carries no verify: — nothing states its contract, so its operation suite has nothing to discharge; add the owning intent's contract claims (input validation, state change, output shape, allowed errors)", op.ID)))
	}

	// The aggregate. Named for what it detects — every fragment vacant — rather
	// than for the whole contract: a feature whose operations all carry criteria
	// and whose fragments carry none is exactly the shape that produced the
	// benchmark's 21 criterion-less presentation cases, and an aggregate keyed
	// on "no criteria anywhere in the feature" would not have fired on it.
	//
	// The per-fragment warnings above locate PARTIAL vacancy. This one says the
	// presentation contract is empty as a whole, which is the condition that
	// guarantees every presentation case the build phase writes cites nothing.
	if in.HasSurface && len(in.Fragments) > 0 && withCriteria == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "feature-surface-no-criteria",
			fmt.Sprintf("no fragment in this feature's surface carries verify: (%d fragments, 0 with criteria) — the presentation contract is empty, so the build phase has nothing to derive presentation cases from", len(in.Fragments))))
	}

	return outcomes
}
