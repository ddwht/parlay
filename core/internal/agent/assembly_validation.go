// parlay-feature: parlay-tool/page-assembly-derivation
// parlay-component: assembly-validation
//
// Validates an authored assembly suite by DIFFING it against the derivation,
// rather than by re-implementing the rules it should satisfy.
//
// The five enforcement rules — one assembly suite per contributed page, exact
// case identities, no criterion on assembly cases, assembly cases excluded
// from criterion coverage, no contract case inside the assembly suite — are
// not five checks here. They fall out of comparing the authored suite to the
// one DeriveAssemblySuite computes. Implementing them separately is how a
// validator drifts from the emitter it is supposed to police.
package agent

import (
	"fmt"
	"sort"
	"strings"
)

// AuthoredCase is the subset of a testcases case this validation reads.
type AuthoredCase struct {
	Name string
	// CriterionRef and CriterionText are the contract citation, empty on a
	// correctly-derived assembly case.
	CriterionRef  string
	CriterionText string
	// Coverage is the `coverage:` stamp: "full", "state-only", or empty.
	Coverage string
	// Derivation is the machine identity an assembly case carries instead of
	// a citation.
	Derivation *AuthoredDerivation
	// Steps are the case's actual mechanics. Checked, not assumed: a case
	// carrying the right derivation: block and empty or unrelated steps passes
	// every identity check while asserting nothing, which is the false-citation
	// defect rebuilt as a true label over a false test.
	Steps []AssemblyStep
}

// AuthoredPending mirrors one `pending_assertions:` row — capability debt as
// the file records it.
type AuthoredPending struct {
	Page      string
	Subject   string
	Assertion string
	Needs     string
}

// ID matches AssemblyAssertion.ID so recorded debt and derived debt are
// directly comparable.
func (p AuthoredPending) ID() string {
	return fmt.Sprintf("%s|%s|%s", p.Page, p.Subject, p.Assertion)
}

// AuthoredDerivation mirrors `derivation: {kind, page, subject, assertion}`.
type AuthoredDerivation struct {
	Kind      string
	Page      string
	Subject   string
	Assertion string
}

// ID matches AssemblyAssertion.ID so the two sides are directly comparable.
func (d AuthoredDerivation) ID() string {
	return fmt.Sprintf("%s|%s|%s", d.Page, d.Subject, d.Assertion)
}

// AuthoredSuite is one suite as written in testcases.yaml.
type AuthoredSuite struct {
	Name string
	Kind string
	// Scope is the organizational field {component, route, flow}. It is NOT
	// what marks a suite as derived: route suites may carry real acceptance
	// tests, so exempting `scope: route` from criterion accounting would drop
	// genuine coverage. Origin is what marks it, below.
	Scope string
	// Origin is "page-assembly" on the derived suite and empty on an authored
	// one. Explicit, because the alternative is inferring intent from a field
	// that does not mean what the inference needs it to mean.
	Origin string
	Page   string
	Cases  []AuthoredCase
	// PendingAssertions is the capability debt the file records. Diffed, not
	// trusted: the schema and the skill both claim this is an explicit record
	// of what the adapter cannot execute, and an unchecked field makes that
	// claim false — the file could omit or invent debt and readiness would say
	// the same thing either way.
	PendingAssertions []AuthoredPending
}

// AssemblyGraduationVersion is the revision at which a missing assembly suite
// stops warning and starts blocking.
const AssemblyGraduationVersion = 3

// AssemblyKindPageAssembly is the only derived origin this release defines.
const AssemblyKindPageAssembly = "page-assembly"

// ValidateAssemblySuites diffs the authored suites against the derivation for
// every page the feature actively contributes to.
//
// expected is keyed by page. Pages with no expected assembly (a feature that
// contributes no fragments to any page) yield no findings.
func ValidateAssemblySuites(expected map[string]AssemblySuite, authored []AuthoredSuite, revision int) []ValidationError {
	var errs []ValidationError

	byPage := map[string][]AuthoredSuite{}
	for _, s := range authored {
		if s.Origin == AssemblyKindPageAssembly {
			byPage[s.Page] = append(byPage[s.Page], s)
		}
	}

	pages := make([]string, 0, len(expected))
	for p := range expected {
		pages = append(pages, p)
	}
	sort.Strings(pages)

	for _, page := range pages {
		want := expected[page]
		got := byPage[page]

		// Rule 1: exactly one derived suite per contributed page.
		//
		// REVISION-GATED, not unconditionally soft. `origin:` is new, so a
		// released v2 artifact could not have recorded it and gets a rebuild
		// window rather than a refusal — the graduation trap this release
		// exists to repair was exactly a diagnostic turned fatal while the
		// producer still emitted the shape that triggered it.
		//
		// But an unconditional warning is its own failure: build-feature now
		// writes v3, so a v3 file is one where the fact COULD have been
		// recorded, and leaving it advisory there would let the whole
		// mechanism be omitted forever behind a warning nobody must act on.
		// Same discipline as criterion_graduation.go, and the same trigger.
		if len(got) == 0 {
			severity := "warning"
			if revision >= AssemblyGraduationVersion {
				severity = "error"
			}
			errs = append(errs, ValidationError{
				Code:     "assembly-suite-missing",
				Message:  fmt.Sprintf("page %q has no `origin: page-assembly` suite, so the composition defects no per-component suite can see (an unmounted component, a sibling eating input) are unchecked", page),
				Context:  "testcases",
				Fix:      "regenerate the derived assembly suite for this page; it is computed, not authored",
				Severity: severity,
			})
			continue
		}
		if len(got) > 1 {
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.Name)
			}
			sort.Strings(names)
			errs = append(errs, ValidationError{
				Code:     "assembly-suite-duplicated",
				Message:  fmt.Sprintf("page %q has %d assembly suites (%s); the derivation produces exactly one", page, len(got), strings.Join(names, ", ")),
				Context:  "testcases",
				Fix:      "keep the single derived suite for this page and delete the rest",
				Severity: "error",
			})
			continue
		}

		suite := got[0]

		// Rule 2 and 3: exact identities, and no contract citation on any of
		// them. Both are read off the same walk.
		expectedIDs := map[string]AssemblyAssertion{}
		for _, a := range want.Supported {
			expectedIDs[a.ID()] = a
		}

		seen := map[string]bool{}
		for _, c := range suite.Cases {
			// Rule 5: a contract-origin case must not live inside the derived
			// suite. Its criterion would be counted as covered by a case the
			// derivation is free to rewrite on the next build.
			if c.Derivation == nil {
				errs = append(errs, ValidationError{
					Code:     "assembly-suite-foreign-case",
					Message:  fmt.Sprintf("case %q in the derived assembly suite for %q carries no derivation: — a contract case cannot live in a suite the tool regenerates", c.Name, page),
					Context:  "testcases",
					Fix:      "move this case into an authored suite; the assembly suite holds only derived composition facts",
					Severity: "error",
				})
				continue
			}
			if c.Derivation.Kind != AssemblyKindPageAssembly {
				errs = append(errs, ValidationError{
					Code:     "assembly-derivation-unknown-kind",
					Message:  fmt.Sprintf("case %q declares derivation kind %q; the closed set is {%s}", c.Name, c.Derivation.Kind, AssemblyKindPageAssembly),
					Context:  "testcases",
					Fix:      "use kind: page-assembly, or move the case out of the derived suite",
					Severity: "error",
				})
				continue
			}

			// Rule 3: derived cases discharge nothing, so a citation on one is
			// a false coverage claim. This is the exact shape the archives
			// carried seven times over — a mount assertion citing a criterion
			// about table columns.
			if strings.TrimSpace(c.CriterionRef) != "" || strings.TrimSpace(c.CriterionText) != "" {
				errs = append(errs, ValidationError{
					Code:     "assembly-case-cites-criterion",
					Message:  fmt.Sprintf("derived case %q cites %s — an assembly assertion discharges no contract criterion, so the citation claims coverage the case does not provide", c.Name, c.CriterionRef),
					Context:  "testcases",
					Fix:      "remove the criterion: from this case; if the criterion genuinely needs a test, write one in an authored suite",
					Severity: "error",
				})
			}

			// `state-only` records that a CONTRACT criterion was observed
			// weakly. A derived fact has no criterion to weaken, and stamping
			// it produced a blocker with an empty ref that record-exception
			// refuses to accept — undischargeable by anyone.
			if strings.TrimSpace(c.Coverage) == "state-only" {
				errs = append(errs, ValidationError{
					Code:     "assembly-case-state-only",
					Message:  fmt.Sprintf("derived case %q is stamped `coverage: state-only`, which records a weakened observation of a contract criterion — a composition fact has none to weaken", c.Name),
					Context:  "testcases",
					Fix:      "drop the coverage: stamp; if the adapter cannot execute this assertion it belongs in pending_assertions as capability debt",
					Severity: "error",
				})
			}

			id := c.Derivation.ID()
			if seen[id] {
				errs = append(errs, ValidationError{
					Code:     "assembly-assertion-duplicated",
					Message:  fmt.Sprintf("assertion %s appears more than once in the assembly suite for %q", id, page),
					Context:  "testcases",
					Fix:      "the derivation emits each (page, subject, assertion) once; regenerate the suite",
					Severity: "error",
				})
				continue
			}
			seen[id] = true

			want, ok := expectedIDs[id]
			if !ok {
				errs = append(errs, ValidationError{
					Code:     "assembly-assertion-unexpected",
					Message:  fmt.Sprintf("assembly suite for %q asserts %s, which the surface and page manifest do not derive", page, id),
					Context:  "testcases",
					Fix:      "regenerate the suite; assembly assertions are computed from the composed page, never invented",
					Severity: "error",
				})
				continue
			}

			// The mechanics, not just the label. An identity-only diff let a
			// correctly-labelled case carry empty or unrelated steps and pass —
			// coverage counted, nothing asserted.
			if got, expect := FingerprintSteps(c.Steps), want.StepFingerprint(); got != expect {
				detail := "carries no steps at all"
				if got != "" {
					detail = fmt.Sprintf("carries %s", got)
				}
				errs = append(errs, ValidationError{
					Code:     "assembly-case-steps-mismatch",
					Message:  fmt.Sprintf("derived case %q for %s %s, but the derivation requires %s — a case with the right label and the wrong mechanics asserts nothing while counting as checked", c.Name, id, detail, expect),
					Context:  "testcases",
					Fix:      "regenerate the suite with `parlay internal emit-assembly --write`; the steps of a derived case are computed, not authored",
					Severity: "error",
				})
			}
		}

		// Rule 2, other direction: a derived assertion the suite omits.
		missing := make([]string, 0)
		for id := range expectedIDs {
			if !seen[id] {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		for _, id := range missing {
			errs = append(errs, ValidationError{
				Code:     "assembly-assertion-missing",
				Message:  fmt.Sprintf("assembly suite for %q omits derived assertion %s", page, id),
				Context:  "testcases",
				Fix:      "regenerate the suite so it carries every assertion the composed page derives",
				Severity: "error",
			})
		}

		// Capability debt: DIFFED against the derivation, then reported.
		//
		// The schema and the skill both say pending_assertions is an explicit
		// record of what the adapter cannot execute. An unvalidated field made
		// that claim false — the file could omit the debt entirely, or invent
		// rows for assertions that are executable, and readiness said the same
		// thing either way. So the recorded set must match the derived set
		// exactly, in both directions, before it is reported.
		recorded := map[string]AuthoredPending{}
		for _, p := range suite.PendingAssertions {
			recorded[p.ID()] = p
		}
		derivedPending := map[string]AssemblyAssertion{}
		for _, a := range want.Pending {
			derivedPending[a.ID()] = a
		}

		for _, a := range want.Pending {
			rec, ok := recorded[a.ID()]
			if !ok {
				errs = append(errs, ValidationError{
					Code:     "assembly-pending-unrecorded",
					Message:  fmt.Sprintf("assertion %s cannot be executed by the presentation adapter, but the assembly suite for %q does not record it in pending_assertions — the debt is unrecorded rather than accepted", a.ID(), page),
					Context:  "testcases",
					Fix:      "regenerate the suite so every unexecutable assertion is recorded as capability debt",
					Severity: "error",
				})
				continue
			}
			if strings.TrimSpace(rec.Needs) != a.RequiredCapability {
				errs = append(errs, ValidationError{
					Code:     "assembly-pending-wrong-capability",
					Message:  fmt.Sprintf("pending assertion %s records needs_capability %q; it requires %q", a.ID(), rec.Needs, a.RequiredCapability),
					Context:  "testcases",
					Fix:      "regenerate the suite; the capability a pending assertion waits on is derived, not chosen",
					Severity: "error",
				})
			}
		}

		var invented []string
		for id := range recorded {
			if _, ok := derivedPending[id]; !ok {
				invented = append(invented, id)
			}
		}
		sort.Strings(invented)
		for _, id := range invented {
			errs = append(errs, ValidationError{
				Code:     "assembly-pending-not-derived",
				Message:  fmt.Sprintf("the assembly suite for %q records %s as capability debt, but the derivation does not — it is either executable or not derived at all, and recording it as debt excuses an assertion nothing asked for", page, id),
				Context:  "testcases",
				Fix:      "regenerate the suite; capability debt is computed from the adapter's declared capabilities",
				Severity: "error",
			})
		}

		// Reported and NOT blocking. It cannot block while the vocabulary is
		// young: no bundled adapter declares render-support or hit-testing
		// today, so every assembly assertion is pending, and a blocking
		// severity here would refuse codegen on every page-bearing feature —
		// reconstructing the very outage this work removes, under a new code.
		// It may graduate once adapters can declare both terms and the bundled
		// ones do so truthfully.
		for _, a := range want.Pending {
			errs = append(errs, ValidationError{
				Code:     "assembly-assertion-unsupported",
				Message:  fmt.Sprintf("assertion %s needs adapter capability %q, which the presentation adapter does not declare, so it is recorded as capability debt rather than executed", a.ID(), a.RequiredCapability),
				Context:  "testcases",
				Fix:      fmt.Sprintf("declare %q on the presentation adapter once the framework can assert it; until then the composition defect this would catch stays unchecked", a.RequiredCapability),
				Severity: "warning",
			})
		}
	}

	// A derived suite for a page the feature does not contribute to.
	var orphanPages []string
	for page := range byPage {
		if _, ok := expected[page]; !ok {
			orphanPages = append(orphanPages, page)
		}
	}
	sort.Strings(orphanPages)
	for _, page := range orphanPages {
		errs = append(errs, ValidationError{
			Code:     "assembly-suite-orphaned",
			Message:  fmt.Sprintf("an assembly suite targets page %q, which this feature contributes no active fragment to — supersession may have retired its contribution", page),
			Context:  "testcases",
			Fix:      "regenerate the testcases; a page the feature no longer reaches has no assembly suite",
			Severity: "error",
		})
	}

	return errs
}

// FindAssemblyAssertionsInContractSuites reports contract-origin cases whose
// steps duplicate a derived assembly assertion.
//
// Keyed on the canonical (page, subject, assertion) identity rather than on
// case names or step syntax, so renaming a case or reordering its steps does
// not evade it. Without this, the run-2 shape survives beside a valid derived
// suite: a contract case named "customers-table is mounted" citing a criterion
// about table columns, discharging nothing while counting as coverage.
func FindAssemblyAssertionsInContractSuites(expected map[string]AssemblySuite, authored []AuthoredSuite) []ValidationError {
	derived := map[string]bool{}
	for _, suite := range expected {
		for _, a := range suite.Supported {
			derived[a.ID()] = true
		}
		for _, a := range suite.Pending {
			derived[a.ID()] = true
		}
	}

	var errs []ValidationError
	for _, s := range authored {
		if s.Origin == AssemblyKindPageAssembly {
			continue
		}
		for _, c := range s.Cases {
			if c.Derivation == nil {
				continue
			}
			id := c.Derivation.ID()
			if !derived[id] {
				continue
			}
			errs = append(errs, ValidationError{
				Code:     "assembly-assertion-outside-derived-suite",
				Message:  fmt.Sprintf("case %q in authored suite %q restates derived assembly assertion %s while citing a contract criterion — the assertion discharges no criterion, so this counts coverage the case does not provide", c.Name, s.Name, id),
				Context:  "testcases",
				Fix:      "delete this case; the derived assembly suite already carries the assertion, and the criterion it cites needs a test that actually observes what it states",
				Severity: "error",
			})
		}
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Message < errs[j].Message })
	return errs
}
