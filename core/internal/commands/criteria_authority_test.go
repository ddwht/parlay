// parlay-feature: parlay-tool/criterion-authority
// parlay-component: criteria-authority-record
// parlay-artifact: test

package commands

import (
	"strings"
	"testing"
)

func crit(ref, text string) AuthorizedCriterion { return AuthorizedCriterion{Ref: ref, Text: text} }

func twoCriteria() []AuthorizedCriterion {
	return []AuthorizedCriterion{
		crit("@f/operation:archive", "archiving a customer with unpaid invoices is rejected"),
		crit("@f/fragment:customer-detail", "the archive button is disabled while invoices are unpaid"),
	}
}

func approvedFor(cs []AuthorizedCriterion) *CriteriaAuthority {
	return &CriteriaAuthority{
		Feature:  "f",
		Approved: &HumanApproval{At: "2026-08-27T00:00:00Z", CriteriaHash: CriteriaHash(cs), Criteria: cs},
	}
}

func TestCriteriaAuthority_ApprovedCriteriaProceedEverywhere(t *testing.T) {
	v := EvaluateCriteriaAuthority(approvedFor(twoCriteria()), twoCriteria(), false, false)
	if !v.Proceed || v.Machine {
		t.Errorf("a person approved this exact standard; %+v", v)
	}
}

// The friction the old gate was most disliked for: one full re-approval per
// regeneration. What is approved here is the standard, so regenerating
// testcases against unchanged criteria asks nothing.
func TestCriteriaAuthority_ReorderingAndReformattingDoNotReapprove(t *testing.T) {
	cs := twoCriteria()
	rec := approvedFor(cs)

	shuffled := []AuthorizedCriterion{cs[1], cs[0]}
	if !EvaluateCriteriaAuthority(rec, shuffled, false, false).Proceed {
		t.Error("order is not identity; reordering must not invalidate an approval")
	}

	respaced := []AuthorizedCriterion{
		crit(cs[0].Ref, "  archiving a customer with unpaid invoices is rejected  "),
		cs[1],
	}
	if !EvaluateCriteriaAuthority(rec, respaced, false, false).Proceed {
		t.Error("whitespace is not identity either")
	}
}

func TestCriteriaAuthority_ChangedStandardRefusesAndShowsTheChange(t *testing.T) {
	rec := approvedFor(twoCriteria())
	changed := []AuthorizedCriterion{
		twoCriteria()[0],
		crit("@f/fragment:customer-detail", "the archive button is HIDDEN while invoices are unpaid"),
	}
	v := EvaluateCriteriaAuthority(rec, changed, false, false)
	if v.Proceed {
		t.Fatal("a rewritten standard is not the approved one")
	}
	if len(v.Added) != 1 || len(v.Removed) != 1 {
		t.Errorf("a stale record should show WHAT changed, not only that something did: %+v", v)
	}
}

// Both switches or neither counts, and a refusal names the missing half.
func TestCriteriaAuthority_MachineAuthorizationNeedsBothSwitches(t *testing.T) {
	cs := twoCriteria()

	if v := EvaluateCriteriaAuthority(nil, cs, true, false); v.Proceed {
		t.Error("a flag alone must not waive the separation; the project has not opted in")
	}
	if v := EvaluateCriteriaAuthority(nil, cs, false, true); v.Proceed {
		t.Error("an opt-in alone must not make every run self-authorizing")
	}
	v := EvaluateCriteriaAuthority(nil, cs, true, true)
	if !v.Proceed || !v.Machine {
		t.Fatalf("project permits it and the run asked; %+v", v)
	}
	// The record must not read as approval. It is a waiver.
	if !strings.Contains(v.Reason, "WAIVED") {
		t.Errorf("machine mode records that nobody looked, not that anybody did: %q", v.Reason)
	}
}

// One unattended escape must not permanently answer the question for everyone
// who comes after.
func TestCriteriaAuthority_APastMachineRunDoesNotAuthorizeALaterOne(t *testing.T) {
	cs := twoCriteria()
	rec := &CriteriaAuthority{
		Feature:     "f",
		MachineRuns: []MachineRun{{At: "2026-08-26T00:00:00Z", CriteriaHash: CriteriaHash(cs), Reason: "ci"}},
	}
	v := EvaluateCriteriaAuthority(rec, cs, false, true)
	if v.Proceed {
		t.Fatal("an audit event is not standing authority")
	}
	if !strings.Contains(v.Reason, "nobody looked") {
		t.Errorf("the refusal should say what the earlier run actually recorded: %q", v.Reason)
	}
}
