// parlay-feature: parlay-tool/criterion-authority
// parlay-component: transitional-severity-graduation
// parlay-artifact: test

package agent

import (
	"fmt"
	"testing"
)

// The promise the transitional comments made and could not keep: a file a
// current generator produced is one where these facts COULD have been
// recorded, so omitting them is an error rather than a warning.
func TestGraduatedSeverity_NewShapeBlocks(t *testing.T) {
	current := ArtifactRevisions{Testcases: TestcasesGraduationVersion, Capabilities: CapabilitiesGraduationVersion}
	for code := range GraduatingCodes() {
		if got := GraduatedSeverity(code, ModeBuild, current); got != SeverityError {
			t.Errorf("%s should block at the current shape, got %q", code, got)
		}
	}
}

// The reason they were warnings at all. Every artifact in existence predates
// the field, and erroring would have failed every project at once over a fact
// none of them could have recorded.
func TestGraduatedSeverity_LegacyShapeStillWarns(t *testing.T) {
	for _, revs := range []ArtifactRevisions{
		{},                              // nothing declared
		{Testcases: 2, Capabilities: 1}, // the shapes that predate the fields
	} {
		for code := range GraduatingCodes() {
			if got := GraduatedSeverity(code, ModeBuild, revs); got != SeverityWarning {
				t.Errorf("%s at %+v should still warn, got %q", code, revs, got)
			}
		}
	}
}

// Each code graduates on ITS OWN artifact. A rebuilt testcases file says
// nothing about whether capabilities recorded source:.
func TestGraduatedSeverity_EachCodeFollowsItsOwnArtifact(t *testing.T) {
	onlyTestcases := ArtifactRevisions{Testcases: TestcasesGraduationVersion, Capabilities: 1}
	if GraduatedSeverity("verify-criterion-uncovered", ModeBuild, onlyTestcases) != SeverityError {
		t.Error("a current testcases file graduates the testcases codes")
	}
	if GraduatedSeverity("capabilities-source-missing", ModeBuild, onlyTestcases) != SeverityWarning {
		t.Error("rebuilding testcases does not make capabilities record source:")
	}
}

// The set is closed, and these three are outside it on purpose: two are
// judgment calls where only the aggregate blocks, and one reports a defect in
// the contract rather than in the file being validated.
func TestGraduatedSeverity_DeliberateWarningsAreNotSweptUp(t *testing.T) {
	current := ArtifactRevisions{Testcases: 99, Capabilities: 99}
	for _, code := range []string{
		"surface-fragment-no-criteria",
		"capability-operation-no-criteria",
		"verify-criterion-duplicate",
	} {
		if got := GraduatedSeverity(code, ModeBuild, current); got != SeverityWarning {
			t.Errorf("%s is a deliberate warning and must not graduate, got %q", code, got)
		}
	}
}

// Authoring is where a file is being written and half-finished is the normal
// state. The boundary that matters is where it is handed downstream.
func TestGraduatedSeverity_AuthoringDoesNotGraduate(t *testing.T) {
	current := ArtifactRevisions{Testcases: TestcasesGraduationVersion, Capabilities: CapabilitiesGraduationVersion}
	if got := GraduatedSeverity("verify-criterion-uncovered", ModeAuthoring, current); got != SeverityWarning {
		t.Errorf("authoring should stay a warning, got %q", got)
	}
}

// A code that is already an error stays one, and an unrelated code is
// untouched — the table only ever raises members of its own closed set.
func TestGraduatedSeverity_LeavesEverythingElseAlone(t *testing.T) {
	current := ArtifactRevisions{Testcases: 99, Capabilities: 99}
	if got := GraduatedSeverity("feature-surface-no-criteria", ModeBuild, current); got != SeverityError {
		t.Errorf("already an error in build mode: %q", got)
	}
	if got := GraduatedSeverity("some-code-that-does-not-exist", ModeBuild, current); got != SeverityError {
		t.Errorf("unlisted codes default to error, unchanged: %q", got)
	}
}

// --- reached, not merely correct -----------------------------------------
//
// The table above is a pure function, and a pure function nothing calls is the
// failure this codebase has shipped repeatedly. These run through the real
// validator with real file content.

func testcasesAt(version int) []byte {
	head := ""
	if version > 0 {
		head = fmt.Sprintf("schema_version: %d\n", version)
	}
	return []byte(head + `feature: f
suites:
  - name: Thing List
    kind: presentation
    source_refs:
      - "@f/fragment:thing-list"
    file: src/ThingList.test.tsx
    cases:
      - name: renders
        exercises: ["@f/fragment:thing-list"]
        observes: ["@f/fragment:thing-list"]
        steps:
          - { type: render, target: "@f/fragment:thing-list" }
        expectations:
          - { type: shows, target: "@f/fragment:thing-list" }
`)
}

func severityOf(outcomes []ValidationOutcome, code string) Severity {
	for _, o := range outcomes {
		if o.Code == code {
			return o.Severity
		}
	}
	return ""
}

// An undischarged criterion is the case the whole transition is about.
func TestGraduation_ReachesTheValidator(t *testing.T) {
	criteria := []CriterionRef{{Ref: "@f/fragment:thing-list", Text: "the list shows every thing"}}

	legacy := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{
		Path: "testcases.yaml", Content: testcasesAt(2),
		ContractResolved: true, Criteria: criteria,
	})
	if got := severityOf(legacy, "verify-criterion-uncovered"); got != SeverityWarning {
		t.Errorf("a file predating criterion identity keeps its warning, got %q", got)
	}

	current := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{
		Path: "testcases.yaml", Content: testcasesAt(TestcasesGraduationVersion),
		ContractResolved: true, Criteria: criteria,
	})
	if got := severityOf(current, "verify-criterion-uncovered"); got != SeverityError {
		t.Errorf("a file a current generator produced could have recorded this, so it blocks; got %q", got)
	}
}

// The revision comes from the file itself, which is what makes the trigger real
// rather than something a caller has to remember to supply.
func TestGraduation_ReadsTheRevisionFromTheFile(t *testing.T) {
	criteria := []CriterionRef{{Ref: "@f/fragment:thing-list", Text: "the list shows every thing"}}
	out := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{
		Path: "testcases.yaml", Content: testcasesAt(TestcasesGraduationVersion),
		ContractResolved: true, Criteria: criteria,
		// Caller supplies nothing.
	})
	if got := severityOf(out, "verify-criterion-uncovered"); got != SeverityError {
		t.Errorf("the file declares its own shape; got %q", got)
	}
}
