// parlay-feature: parlay-tool/intent-supersession
// parlay-component: amendment-artifact
// parlay-artifact: test

package agent

import (
	"strings"
	"testing"
)

func amendmentCodes(outcomes []ValidationOutcome) []string {
	var out []string
	for _, o := range outcomes {
		out = append(out, o.Code)
	}
	return out
}

func amendmentHasCode(outcomes []ValidationOutcome, code string) bool {
	for _, o := range outcomes {
		if o.Code == code {
			return true
		}
	}
	return false
}

// governanceAmendment is a supersession: no affects:, an intent named, and both
// body sections a retirement requires.
const governanceAmendment = `---
amendment: mechanical-readiness-replaces-blanket-review
date: 2026-08-26
trigger: "the gate it mandates is being removed"
supersedes_intents:
  - insert-a-coverage-review-step-between-the-build-and-code-phase-groups
---

## Change
Clean mechanical readiness replaces the blanket coverage review at the build to
code boundary.

## Why
The review it mandates approves suite names with no cases, criteria or diff in
view, so it records that a person clicked rather than that a person looked.

## Acceptance
- A multi-target feature with clean mechanical readiness enters codegen with no
  review artifact and no person present.
`

func TestValidateAmendment_GovernanceAmendmentNeedsNoAffects(t *testing.T) {
	got := ValidateAmendment(ModeBuild, "amendments/001-x.md", []byte(governanceAmendment))
	if amendmentHasCode(got, "amendment-affects-missing") {
		t.Errorf("a supersession names a promise instead of a contract entry and must not require affects:; got %v", amendmentCodes(got))
	}
	if len(got) != 0 {
		t.Errorf("expected a clean governance amendment, got %v", amendmentCodes(got))
	}
}

func TestValidateAmendment_NeitherAffectsNorSupersessionIsNothing(t *testing.T) {
	// The relaxation is a disjunction, not a removal: an amendment naming no
	// contract entry AND no superseded promise has nothing for apply or
	// scoping to act on.
	src := strings.Replace(governanceAmendment,
		"supersedes_intents:\n  - insert-a-coverage-review-step-between-the-build-and-code-phase-groups\n", "", 1)
	got := ValidateAmendment(ModeBuild, "amendments/001-x.md", []byte(src))
	if !amendmentHasCode(got, "amendment-affects-missing") {
		t.Errorf("expected amendment-affects-missing when both are empty, got %v", amendmentCodes(got))
	}
}

func TestValidateAmendment_CannotSupersedeAnotherFeaturesIntent(t *testing.T) {
	// The same-feature rule is enforced at the syntax layer so a qualified ref
	// is refused rather than quietly resolved somewhere else.
	// A bare @ opens a reserved YAML scalar, so a qualified ref is written
	// quoted in practice; both quoted forms must be refused on meaning
	// rather than on syntax.
	for _, ref := range []string{
		`"@parlay-tool/loop-coverage-review-phase/some-intent"`,
		`"loop-coverage-review-phase/some-intent"`,
	} {
		src := strings.Replace(governanceAmendment,
			"  - insert-a-coverage-review-step-between-the-build-and-code-phase-groups",
			"  - "+ref, 1)
		got := ValidateAmendment(ModeBuild, "amendments/001-x.md", []byte(src))
		if !amendmentHasCode(got, "amendment-supersedes-intent-foreign") {
			t.Errorf("ref %q: one feature must not retire another's founding promise; got %v", ref, amendmentCodes(got))
		}
	}
}

func TestValidateAmendment_SupersessionWithoutSuccessorIsDeletion(t *testing.T) {
	src := governanceAmendment[:strings.Index(governanceAmendment, "## Acceptance")]
	got := ValidateAmendment(ModeBuild, "amendments/001-x.md", []byte(src))
	if !amendmentHasCode(got, "amendment-supersession-no-successor") {
		t.Errorf("retiring a promise with nothing in its place is deletion; got %v", amendmentCodes(got))
	}
}

func TestValidateAmendment_SupersessionWithoutRationaleIsRefused(t *testing.T) {
	src := strings.Replace(governanceAmendment, `## Why
The review it mandates approves suite names with no cases, criteria or diff in
view, so it records that a person clicked rather than that a person looked.

`, "", 1)
	got := ValidateAmendment(ModeBuild, "amendments/001-x.md", []byte(src))
	if !amendmentHasCode(got, "amendment-supersession-no-rationale") {
		t.Errorf("the frozen intent cannot record why it stopped being true, so the amendment must; got %v", amendmentCodes(got))
	}
}

// The negative controls. The stricter bar belongs to supersession alone —
// if these fire on an ordinary amendment, the check is matching something
// other than what it claims to.
func TestValidateAmendment_OrdinaryAmendmentKeepsItsExemptions(t *testing.T) {
	ordinary := `---
amendment: tighten-create
date: 2026-08-26
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
Rename the operation's input field.
`
	got := ValidateAmendment(ModeBuild, "amendments/001-x.md", []byte(ordinary))
	if amendmentHasCode(got, "amendment-supersession-no-successor") {
		t.Errorf("a rename with no Acceptance is the case the exemption exists for; got %v", amendmentCodes(got))
	}
	if amendmentHasCode(got, "amendment-supersession-no-rationale") {
		t.Errorf("Why stays encouraged rather than required off the supersession path; got %v", amendmentCodes(got))
	}
	if !amendmentHasCode(got, "amendment-missing-acceptance") {
		t.Errorf("the ordinary warning should still be reported; got %v", amendmentCodes(got))
	}
}

func TestValidateAmendment_EmptySupersedesEntryIsMalformed(t *testing.T) {
	src := strings.Replace(governanceAmendment,
		"  - insert-a-coverage-review-step-between-the-build-and-code-phase-groups",
		`  - ""`, 1)
	got := ValidateAmendment(ModeBuild, "amendments/001-x.md", []byte(src))
	if !amendmentHasCode(got, "amendment-supersedes-intent-malformed") {
		t.Errorf("expected malformed entry to be named, got %v", amendmentCodes(got))
	}
}
