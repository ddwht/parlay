package agent

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func interactiveFalse() *bool { b := false; return &b }

func allCaps() AdapterCapabilities {
	return AdapterCapabilities{RenderSupport: map[string]bool{CapRenderMounted: true}, HitTesting: true}
}

func noCaps() AdapterCapabilities {
	return AdapterCapabilities{RenderSupport: map[string]bool{}}
}

func idsOf(as []AssemblyAssertion) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID())
	}
	return out
}

func containsID(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// The three assertion kinds, derived from the surface alone with nothing
// invented — and, critically, NO criterion on any of them.
func TestDeriveAssemblySuite_ThreeKinds(t *testing.T) {
	plain := frag("catalog", "Customers Table", "Customers", "main", 1)
	plain.Actions = "invoke"

	static := frag("catalog", "Banner", "Customers", "main", 2)
	static.Interactive = interactiveFalse()

	inert := frag("catalog", "Spacer", "Customers", "main", 3)

	suite := DeriveAssemblySuite("Customers", []parser.Fragment{plain, static, inert}, allCaps())

	got := idsOf(suite.Supported)
	for _, want := range []string{
		"Customers|customers-table|mounted",
		"Customers|customers-table|hit-reachable",
		"Customers|banner|mounted",
		"Customers|banner|not-hit-reachable",
		"Customers|spacer|mounted",
	} {
		if !containsID(got, want) {
			t.Errorf("missing derived assertion %s; got %v", want, got)
		}
	}
	// A fragment with no actions and no interactive:false gets no
	// reachability assertion — there is nothing to assert about.
	if containsID(got, "Customers|spacer|hit-reachable") {
		t.Error("a fragment with no actions must not derive a hit-reachable assertion")
	}
	// interactive:false is a positive NOT-reachable claim, not the absence of
	// the reachable one.
	if containsID(got, "Customers|banner|hit-reachable") {
		t.Error("an interactive:false fragment must not derive hit-reachable")
	}
}

// Today's default: no adapter declares either capability, so every assertion
// is capability debt rather than an executable case. This is what makes the
// unsupported severity load-bearing — if it blocked, every page-bearing
// feature would refuse codegen.
func TestDeriveAssemblySuite_NoCapabilitiesMakesEverythingPending(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	f.Actions = "invoke"

	suite := DeriveAssemblySuite("Customers", []parser.Fragment{f}, noCaps())

	if len(suite.Supported) != 0 {
		t.Errorf("no adapter capability must leave nothing executable; got %v", idsOf(suite.Supported))
	}
	if len(suite.Pending) != 2 {
		t.Fatalf("expected mounted + hit-reachable as debt; got %v", idsOf(suite.Pending))
	}
}

// mounted and hit-testing are SEPARATE capabilities. An adapter that can see a
// mount point has not thereby shown a pointer can reach it, and collapsing the
// two would let render-support stand in as proof of reachability.
func TestDeriveAssemblySuite_MountedDoesNotImplyHitTesting(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	f.Actions = "invoke"

	caps := AdapterCapabilities{RenderSupport: map[string]bool{CapRenderMounted: true}}
	suite := DeriveAssemblySuite("Customers", []parser.Fragment{f}, caps)

	if !containsID(idsOf(suite.Supported), "Customers|table|mounted") {
		t.Error("declared mounted support must make the mount assertion executable")
	}
	if !containsID(idsOf(suite.Pending), "Customers|table|hit-reachable") {
		t.Error("hit-reachable must stay debt when only mounted is declared")
	}
}

// The emitter and the validator compare outputs, so the order must be fixed.
func TestDeriveAssemblySuite_IsDeterministic(t *testing.T) {
	a := frag("catalog", "Alpha", "P", "main", 1)
	a.Actions = "invoke"
	b := frag("catalog", "Beta", "P", "main", 2)

	first := DeriveAssemblySuite("P", []parser.Fragment{a, b}, allCaps())
	second := DeriveAssemblySuite("P", []parser.Fragment{b, a}, allCaps())

	f, s := idsOf(first.Supported), idsOf(second.Supported)
	if len(f) != len(s) {
		t.Fatalf("lengths differ: %v vs %v", f, s)
	}
	for i := range f {
		if f[i] != s[i] {
			t.Fatalf("order differs at %d: %v vs %v", i, f, s)
		}
	}
}

func errCodesOf(errs []ValidationError) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Code)
	}
	return out
}

func derivedSuite(page string, ids ...[3]string) AuthoredSuite {
	s := AuthoredSuite{Name: page + "-assembly", Kind: "presentation", Scope: "route",
		Origin: AssemblyKindPageAssembly, Page: page}
	for _, id := range ids {
		// Steps come from the derivation itself: the validator diffs mechanics,
		// not just labels, so a fixture that omitted them would be asserting
		// the very defect the check exists to catch.
		a := AssemblyAssertion{Page: id[0], Subject: id[1], Kind: id[2]}
		s.Cases = append(s.Cases, AuthoredCase{
			Name: id[1] + " " + id[2],
			Derivation: &AuthoredDerivation{
				Kind: AssemblyKindPageAssembly, Page: id[0], Subject: id[1], Assertion: id[2],
			},
			Steps: a.Steps(),
		})
	}
	return s
}

// A correctly derived suite validates clean.
func TestValidateAssemblySuites_CleanRoundTrip(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}
	authored := []AuthoredSuite{derivedSuite("Customers", [3]string{"Customers", "table", "mounted"})}

	if errs := ValidateAssemblySuites(expected, authored, 3); len(errs) != 0 {
		t.Fatalf("a suite matching the derivation must validate clean; got %v", errs)
	}
}

// The exact defect from the archives: a derived case citing a contract
// criterion it does not discharge.
func TestValidateAssemblySuites_CitingCriterionIsRefused(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}
	authored := []AuthoredSuite{derivedSuite("Customers", [3]string{"Customers", "table", "mounted"})}
	authored[0].Cases[0].CriterionRef = "@catalog/fragment:Table"
	authored[0].Cases[0].CriterionText = "The customers table shows name, email, ship-to and created columns."

	errs := ValidateAssemblySuites(expected, authored, 3)
	if !hasActiveCode(errs, "assembly-case-cites-criterion") {
		t.Fatalf("a derived case citing a criterion must be refused; got %v", errCodesOf(errs))
	}
}

// The other archived shape: state-only on a derived fact, which produced a
// blocker with an empty criterion that record-exception refuses to accept.
func TestValidateAssemblySuites_StateOnlyIsRefused(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}
	authored := []AuthoredSuite{derivedSuite("Customers", [3]string{"Customers", "table", "mounted"})}
	authored[0].Cases[0].Coverage = "state-only"

	if errs := ValidateAssemblySuites(expected, authored, 3); !hasActiveCode(errs, "assembly-case-state-only") {
		t.Fatalf("state-only on a derived fact must be refused; got %v", errCodesOf(errs))
	}
}

func TestValidateAssemblySuites_InventedAndMissingAssertions(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}

	invented := []AuthoredSuite{derivedSuite("Customers",
		[3]string{"Customers", "table", "mounted"},
		[3]string{"Customers", "ghost", "mounted"})}
	if errs := ValidateAssemblySuites(expected, invented, 3); !hasActiveCode(errs, "assembly-assertion-unexpected") {
		t.Errorf("an assertion the page does not derive must be refused; got %v", errCodesOf(errs))
	}

	empty := []AuthoredSuite{derivedSuite("Customers")}
	if errs := ValidateAssemblySuites(expected, empty, 3); !hasActiveCode(errs, "assembly-assertion-missing") {
		t.Errorf("an omitted derived assertion must be reported; got %v", errCodesOf(errs))
	}
}

// A contract case inside the regenerated suite would have its criterion
// counted as covered by a case the tool rewrites on the next build.
func TestValidateAssemblySuites_ForeignCaseIsRefused(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}
	authored := []AuthoredSuite{derivedSuite("Customers", [3]string{"Customers", "table", "mounted"})}
	authored[0].Cases = append(authored[0].Cases, AuthoredCase{
		Name: "an authored acceptance test", CriterionRef: "@catalog/fragment:Table", CriterionText: "shows columns",
	})

	if errs := ValidateAssemblySuites(expected, authored, 3); !hasActiveCode(errs, "assembly-suite-foreign-case") {
		t.Fatalf("a contract case in the derived suite must be refused; got %v", errCodesOf(errs))
	}
}

// Capability debt is reported and must NOT block — provided the file records
// it. No bundled adapter declares either term, so a blocking severity on the
// debt itself would refuse codegen on every page-bearing feature.
func TestValidateAssemblySuites_UnsupportedIsWarningOnly(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, noCaps()),
	}
	authored := []AuthoredSuite{derivedSuite("Customers")}
	for _, a := range expected["Customers"].Pending {
		authored[0].PendingAssertions = append(authored[0].PendingAssertions, AuthoredPending{
			Page: a.Page, Subject: a.Subject, Assertion: a.Kind, Needs: a.RequiredCapability,
		})
	}

	errs := ValidateAssemblySuites(expected, authored, 3)
	sawUnsupported := false
	for _, e := range errs {
		if e.Code == "assembly-assertion-unsupported" {
			sawUnsupported = true
			if e.Severity != "warning" {
				t.Fatalf("capability debt must be a warning, got %q — a blocker here rebuilds the outage", e.Severity)
			}
		}
		if e.Severity == "error" {
			t.Fatalf("nothing may block while every assertion is recorded capability debt; got %+v", e)
		}
	}
	if !sawUnsupported {
		t.Fatal("capability debt must be reported, not silently dropped")
	}
}

// The schema and the skill both claim pending_assertions is an explicit record
// of what the adapter cannot execute. Unvalidated, that claim was false: the
// file could omit the debt entirely and readiness said the same thing.
func TestValidateAssemblySuites_PendingDebtIsDiffed(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, noCaps()),
	}

	omitted := []AuthoredSuite{derivedSuite("Customers")}
	if errs := ValidateAssemblySuites(expected, omitted, 3); !hasActiveCode(errs, "assembly-pending-unrecorded") {
		t.Errorf("omitted capability debt must be refused; got %v", errCodesOf(errs))
	}

	invented := []AuthoredSuite{derivedSuite("Customers")}
	invented[0].PendingAssertions = []AuthoredPending{
		{Page: "Customers", Subject: "ghost", Assertion: "mounted", Needs: "mounted"},
	}
	if errs := ValidateAssemblySuites(expected, invented, 3); !hasActiveCode(errs, "assembly-pending-not-derived") {
		t.Errorf("invented capability debt must be refused; got %v", errCodesOf(errs))
	}
}

// A correctly-labelled case with the wrong mechanics asserts nothing while
// counting as checked — the false-citation defect rebuilt as a true label over
// a false test. Identity alone could not see it.
func TestValidateAssemblySuites_StepsAreDiffed(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}

	hollow := []AuthoredSuite{derivedSuite("Customers", [3]string{"Customers", "table", "mounted"})}
	hollow[0].Cases[0].Steps = nil
	if errs := ValidateAssemblySuites(expected, hollow, 3); !hasActiveCode(errs, "assembly-case-steps-mismatch") {
		t.Errorf("a derived case with no steps must be refused; got %v", errCodesOf(errs))
	}

	wrong := []AuthoredSuite{derivedSuite("Customers", [3]string{"Customers", "table", "mounted"})}
	wrong[0].Cases[0].Steps = []AssemblyStep{{Action: "render", Target: "something-else"}}
	if errs := ValidateAssemblySuites(expected, wrong, 3); !hasActiveCode(errs, "assembly-case-steps-mismatch") {
		t.Errorf("a derived case whose steps target something else must be refused; got %v", errCodesOf(errs))
	}
}

// Revision gating: a released v2 artifact predates origin: and gets a rebuild
// window; a v3 file is one where the fact could have been recorded, so leaving
// it advisory there would let the whole mechanism be omitted forever.
func TestValidateAssemblySuites_MissingSuiteIsRevisionGated(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}

	for _, tc := range []struct {
		revision int
		want     string
	}{{2, "warning"}, {0, "warning"}, {3, "error"}} {
		errs := ValidateAssemblySuites(expected, nil, tc.revision)
		found := false
		for _, e := range errs {
			if e.Code == "assembly-suite-missing" {
				found = true
				if e.Severity != tc.want {
					t.Errorf("revision %d: severity %q, want %q", tc.revision, e.Severity, tc.want)
				}
			}
		}
		if !found {
			t.Errorf("revision %d: no assembly-suite-missing reported", tc.revision)
		}
	}
}

// The run-2 shape surviving beside a valid derived suite: a contract case
// restating a derived assertion while citing a criterion it cannot discharge.
// Keyed on identity, so renaming the case does not evade it.
func TestFindAssemblyAssertionsInContractSuites(t *testing.T) {
	f := frag("catalog", "Table", "Customers", "main", 1)
	expected := map[string]AssemblySuite{
		"Customers": DeriveAssemblySuite("Customers", []parser.Fragment{f}, allCaps()),
	}

	authored := []AuthoredSuite{
		derivedSuite("Customers", [3]string{"Customers", "table", "mounted"}),
		{
			Name: "customers-contract", Kind: "presentation",
			Cases: []AuthoredCase{{
				Name:          "a name that reveals nothing",
				CriterionRef:  "@catalog/fragment:Table",
				CriterionText: "The customers table shows name, email, ship-to and created columns.",
				Derivation: &AuthoredDerivation{
					Kind: AssemblyKindPageAssembly, Page: "Customers", Subject: "table", Assertion: "mounted",
				},
			}},
		},
	}

	errs := FindAssemblyAssertionsInContractSuites(expected, authored)
	if !hasActiveCode(errs, "assembly-assertion-outside-derived-suite") {
		t.Fatalf("a contract case restating a derived assertion must be refused; got %v", errCodesOf(errs))
	}
}
