// parlay-feature: parlay-tool/multi-adapter
// parlay-component: criteria-presence
// parlay-artifact: test

package agent

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func presenceCodes(outcomes []ValidationOutcome) map[string]int {
	counts := map[string]int{}
	for _, o := range outcomes {
		counts[o.Code]++
	}
	return counts
}

// The shape the benchmark actually produced: every intent covered by an
// operation, so under the pre-v0.5.x routing rule every criterion went to the
// operations and all four fragments were left vacant. Nothing reported it, and
// the 21 criterion-less presentation cases that followed were read as a defect
// in the build phase.
func TestValidateCriteriaPresence_OperationsOnlyIsTheObservedShape(t *testing.T) {
	in := CriteriaPresenceInput{
		Feature:    "catalog/manage-customers",
		HasSurface: true,
		Fragments: []parser.Fragment{
			{Name: "Customers list"}, {Name: "Create customer form"},
			{Name: "Customer detail"}, {Name: "Edit customer form"},
		},
		Operations: []parser.CapabilityOperation{
			{ID: "customer.create", Verify: []string{"rejects a duplicate email"}},
			{ID: "customer.list", Verify: []string{"returns customers in name order"}},
			{ID: "customer.get", Verify: []string{"404s on an unknown id"}},
			{ID: "customer.update", Verify: []string{"rejects an unknown field"}},
		},
	}
	got := presenceCodes(ValidateCriteriaPresence(ModeBuild, in))
	if got["surface-fragment-no-criteria"] != 4 {
		t.Errorf("surface-fragment-no-criteria = %d, want 4", got["surface-fragment-no-criteria"])
	}
	if got["feature-surface-no-criteria"] != 1 {
		t.Errorf("feature-surface-no-criteria = %d, want 1 — the whole surface is vacant", got["feature-surface-no-criteria"])
	}
	if got["capability-operation-no-criteria"] != 0 {
		t.Errorf("operations all carry criteria, but %d were reported", got["capability-operation-no-criteria"])
	}
}

// Partial vacancy: the per-fragment code locates it, the aggregate stays quiet.
// This is the distinction that makes the aggregate worth having — an aggregate
// keyed on "no criteria anywhere in the feature" would not have fired on the
// observed run at all, since its operations all carried criteria.
func TestValidateCriteriaPresence_PartialVacancyIsNotAggregate(t *testing.T) {
	in := CriteriaPresenceInput{
		HasSurface: true,
		Fragments: []parser.Fragment{
			{Name: "Customers list", Verify: []string{"shows each customer's name and email"}},
			{Name: "Customer detail"},
		},
	}
	got := presenceCodes(ValidateCriteriaPresence(ModeBuild, in))
	if got["surface-fragment-no-criteria"] != 1 {
		t.Errorf("surface-fragment-no-criteria = %d, want 1", got["surface-fragment-no-criteria"])
	}
	if got["feature-surface-no-criteria"] != 0 {
		t.Error("the aggregate fired while one fragment still carries criteria")
	}
}

// A fully-populated contract is silent.
func TestValidateCriteriaPresence_PopulatedContractIsSilent(t *testing.T) {
	in := CriteriaPresenceInput{
		HasSurface: true,
		Fragments:  []parser.Fragment{{Name: "Customers list", Verify: []string{"shows each customer's name"}}},
		Operations: []parser.CapabilityOperation{{ID: "customer.list", Verify: []string{"returns customers in name order"}}},
	}
	if out := ValidateCriteriaPresence(ModeBuild, in); len(out) != 0 {
		t.Errorf("a fully-populated contract reported %d findings: %+v", len(out), out)
	}
}

// A feature with no surface artifact at all — a CLI, a TUI, a pure backend
// feature — has observable output and no fragment to carry it. Its output
// claims live on the operation, and the aggregate must not fire on the absence
// of a surface it was never supposed to have.
func TestValidateCriteriaPresence_NoSurfaceIsNotVacancy(t *testing.T) {
	in := CriteriaPresenceInput{
		HasSurface: false,
		Operations: []parser.CapabilityOperation{{ID: "report.render", Verify: []string{"writes one row per expense"}}},
	}
	if out := ValidateCriteriaPresence(ModeBuild, in); len(out) != 0 {
		t.Errorf("a feature with no surface reported %d findings: %+v", len(out), out)
	}
}

// A surface that parsed to zero fragments is a different condition from having
// no surface, and neither is the aggregate's subject: with no fragments there
// is no presentation contract to call empty.
func TestValidateCriteriaPresence_EmptySurfaceDoesNotAggregate(t *testing.T) {
	in := CriteriaPresenceInput{HasSurface: true}
	if out := ValidateCriteriaPresence(ModeBuild, in); len(out) != 0 {
		t.Errorf("an empty surface reported %d findings: %+v", len(out), out)
	}
}

// Operations are checked independently of the surface side.
func TestValidateCriteriaPresence_OperationWithoutCriteria(t *testing.T) {
	in := CriteriaPresenceInput{
		Operations: []parser.CapabilityOperation{
			{ID: "customer.create", Verify: []string{"rejects a duplicate email"}},
			{ID: "customer.purge"},
		},
	}
	out := ValidateCriteriaPresence(ModeBuild, in)
	if presenceCodes(out)["capability-operation-no-criteria"] != 1 {
		t.Fatalf("expected 1 operation finding, got %+v", out)
	}
	if !strings.Contains(out[0].Message, "customer.purge") {
		t.Errorf("finding names the wrong operation: %q", out[0].Message)
	}
}

// All three are warnings. Grading the aggregate an error would make readiness
// convert it into a gate blocker, stopping every project authored under the old
// routing rule before its owner had any way forward.
func TestValidateCriteriaPresence_AllWarnings(t *testing.T) {
	in := CriteriaPresenceInput{
		HasSurface: true,
		Fragments:  []parser.Fragment{{Name: "Customers list"}},
		Operations: []parser.CapabilityOperation{{ID: "customer.purge"}},
	}
	out := ValidateCriteriaPresence(ModeBuild, in)
	if len(out) == 0 {
		t.Fatal("expected findings")
	}
	for _, o := range out {
		if o.Severity != SeverityWarning {
			t.Errorf("%s severity = %q, want warning", o.Code, o.Severity)
		}
	}
}
