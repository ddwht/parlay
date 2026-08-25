// parlay-feature: parlay-tool/multi-adapter
// parlay-component: testcases-v2
// parlay-artifact: test

package agent

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/embedded"
)

func TestValidateTestcasesV2_OperationUncovered(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: presentation
    component: x
    source_refs: ["@f/x"]
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content, CanonicalOperations: []string{"@f/operation:task.delete"}})
	if !findCode(outcomes, "testcases-operation-uncovered") {
		t.Errorf("missing testcases-operation-uncovered; got %+v", outcomes)
	}
}

func TestValidateTestcasesV2_SourceRefsMissing(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: presentation
    component: x
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-source-refs-missing") {
		t.Errorf("missing testcases-source-refs-missing; got %+v", outcomes)
	}
}

// The v1 shape (a suite without kind:) stopped being accepted in v0.3 — the
// policy has always been regenerate, so the only correct response is a hard
// error pointing at build-feature.
func TestValidateTestcasesV2_V1ShapeUnsupported(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    component: x
    intent: legacy intent
`)
	outcomes := ValidateTestcasesV2(ModeAuthoring, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-v1-unsupported") {
		t.Errorf("missing testcases-v1-unsupported; got %+v", outcomes)
	}
	for _, o := range outcomes {
		if o.Code == "testcases-v1-unsupported" && o.Severity != SeverityError {
			t.Errorf("v1 shape must be an error since v0.3; got %v", o.Severity)
		}
	}
}

func TestValidateTestcasesV2_UnknownKind(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: integration
    source_refs: ["@f/x"]
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-suite-kind-unknown") {
		t.Errorf("missing testcases-suite-kind-unknown; got %+v", outcomes)
	}
}

func TestValidateTestcasesV2_OperationCoverageWalker(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    kind: operation
    operation: "@f/operation:task.create"
    source_refs: ["@f/operation:task.create"]
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content, CanonicalOperations: []string{"@f/operation:task.create"}})
	if findCode(outcomes, "testcases-operation-uncovered") {
		t.Errorf("operation suite should cover the canonical op; got %+v", outcomes)
	}
}

// Suite: cases[] step vocabulary (D10)
//
// Cases decoded into []map[string]yaml.Node and nothing looked inside, so the two
// closed sets testcases.schema.md documents for cases[].steps[] were unchecked.
// A suite could say `action: hover` and validate cleanly; the agent generating
// tests from it then has to guess whether the term means something, which is the
// failure this closes. An unknown verb is not obviously wrong to a reader — only
// to the runner.

func TestValidateTestcasesV2_UnknownStepActionRejected(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: form renders
    kind: presentation
    source_refs: ["@expenses/submit"]
    cases:
      - name: hovering the field
        steps:
          - action: hover
            target: amount
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-unknown-term") {
		t.Fatalf("action \"hover\" was accepted; got %+v", outcomes)
	}
	// The message must carry the offending term, the case it is in, and the
	// allowed set — a closed-set error without the alternatives makes the author
	// guess at a vocabulary they cannot extend.
	if !findMessage(outcomes, "hover") {
		t.Error("diagnostic omits the offending term or the allowed set")
	}
	if !findMessage(outcomes, "hovering the field") {
		t.Error("diagnostic does not name the case the bad step is in")
	}
}

func TestValidateTestcasesV2_UnknownVerifyRejected(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: totals
    kind: presentation
    source_refs: ["@expenses/submit"]
    cases:
      - name: total shown
        steps:
          - verify: colour
            target: total
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-unknown-term") {
		t.Fatalf("verify \"colour\" was accepted; got %+v", outcomes)
	}
}

// An appears step at a known level is accepted; the level lives in value:.
func TestValidateTestcasesV2_AppearsStepAccepted(t *testing.T) {
	content := []byte(`schema_version: 2
feature: viewer
suites:
  - name: mesh renders
    kind: presentation
    source_refs: ["@viewer/render"]
    cases:
      - name: mesh reaches the renderer
        steps:
          - action: render
            target: MeshViewport
          - action: appears
            target: MeshViewport
            value: content
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if findCode(outcomes, "testcases-unknown-term") {
		t.Errorf("appears action rejected as unknown term: %+v", outcomes)
	}
	if findCode(outcomes, "testcases-appears-level-unknown") {
		t.Errorf("appears value \"content\" flagged as unknown level: %+v", outcomes)
	}
}

// An appears step with a bad or missing level cannot run — the runner does not
// know what depth to assert.
func TestValidateTestcasesV2_AppearsLevelUnknownRejected(t *testing.T) {
	for _, tc := range []struct {
		name, value string
	}{
		{"bad level", "value: painted\n"},
		{"missing level", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := []byte(`schema_version: 2
feature: viewer
suites:
  - name: mesh renders
    kind: presentation
    source_refs: ["@viewer/render"]
    cases:
      - name: mesh appears
        steps:
          - action: appears
            target: MeshViewport
            ` + tc.value)
			outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
			if !findCode(outcomes, "testcases-appears-level-unknown") {
				t.Fatalf("expected testcases-appears-level-unknown, got %+v", outcomes)
			}
		})
	}
}

// coverage: is the state-only honesty marker; a known value is clean, an unknown
// one hides whether the claim was downgraded.
func TestValidateTestcasesV2_CoverageValidated(t *testing.T) {
	base := func(cov string) []byte {
		return []byte(`schema_version: 2
feature: viewer
suites:
  - name: mesh renders
    kind: presentation
    source_refs: ["@viewer/render"]
    cases:
      - name: mesh shown
        coverage: ` + cov + `
        steps:
          - action: render
            target: MeshViewport
`)
	}
	for _, ok := range []string{"full", "state-only"} {
		if findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: base(ok)}), "testcases-coverage-unknown") {
			t.Errorf("coverage %q wrongly rejected", ok)
		}
	}
	if !findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: base("partial")}), "testcases-coverage-unknown") {
		t.Errorf("coverage \"partial\" should be rejected")
	}
}

// Every documented term must pass. An over-strict check would reject valid
// generated testcases, which is worse than the gap it closes.
func TestValidateTestcasesV2_AllDocumentedTermsAccepted(t *testing.T) {
	for _, action := range []string{"render", "click", "input", "select", "navigate", "wait"} {
		content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: s
    kind: presentation
    source_refs: ["@expenses/submit"]
    cases:
      - name: c
        steps:
          - action: ` + action + `
            target: x
`)
		if findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-unknown-term") {
			t.Errorf("documented action %q was rejected", action)
		}
	}
	for _, verb := range []string{"element", "state", "route", "count", "text", "visible", "hidden", "enabled", "disabled"} {
		content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: s
    kind: presentation
    source_refs: ["@expenses/submit"]
    cases:
      - name: c
        steps:
          - verify: ` + verb + `
            target: x
`)
		if findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-unknown-term") {
			t.Errorf("documented verify %q was rejected", verb)
		}
	}
}

// A step is either an action or an assertion. One that is neither carries no
// instruction at all, and silently generating nothing for it is how a suite ends
// up with fewer assertions than it appears to have.
func TestValidateTestcasesV2_StepWithNeitherActionNorVerifyRejected(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: s
    kind: presentation
    source_refs: ["@expenses/submit"]
    cases:
      - name: c
        steps:
          - target: amount
            value: "10"
`)
	if !findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-unknown-term") {
		t.Error("a step with neither action: nor verify: was accepted")
	}
}

func TestValidateTestcasesV2_UnnamedCaseRejected(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: s
    kind: presentation
    source_refs: ["@expenses/submit"]
    cases:
      - steps:
          - action: render
            target: form
      - name: "   "
        steps:
          - action: render
            target: form
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-case-unnamed") {
		t.Fatalf("an unnamed case was accepted; got %+v", outcomes)
	}
	// Both the absent name and the whitespace-only one must be caught: a name
	// that is present but blank reports just as anonymously as a missing one.
	count := 0
	for _, o := range outcomes {
		if o.Code == "testcases-case-unnamed" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("want 2 unnamed-case findings (absent and whitespace-only), got %d", count)
	}
}

// The cases vocabulary is version-independent, so the check must reach legacy v1
// suites too — they are most of what exists in projects today. Putting it behind
// the v2 discriminator would have exempted exactly the suites most likely to
// carry hand-written steps.
func TestValidateTestcasesV2_LegacySuiteCasesStillChecked(t *testing.T) {
	content := []byte(`schema_version: 1
feature: expenses
suites:
  - name: legacy
    intent: "@expenses/submit"
    cases:
      - name: c
        steps:
          - action: teleport
            target: x
`)
	if !findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-unknown-term") {
		t.Error("a legacy v1 suite's bad step vocabulary was not checked")
	}
}

// TestCaseStepVocabulariesMatchTheSchemaTables holds the Go closed sets against
// the tables that document them.
//
// This exists because of a specific mistake. testcases.schema.md carried two
// lists of verifies — a short inline example and a longer Verifications table —
// and they had drifted apart: each contained terms the other lacked, and the
// generator emitted from both. The first version of this validator took the
// example's list, and rejected seven steps in a real generated testcases.yaml.
// Reading the table means the code cannot disagree with the documentation about
// what the vocabulary is; it can only be wrong together with it, which is a
// failure someone reviewing the schema would see.
func TestCaseStepVocabulariesMatchTheSchemaTables(t *testing.T) {
	data, err := embedded.ReadSchema("testcases.schema.md")
	if err != nil {
		t.Fatalf("read testcases.schema.md: %v", err)
	}

	actions := schemaTableTerms(string(data), "### Actions")
	verifies := schemaTableTerms(string(data), "### Verifications")

	if len(actions) == 0 || len(verifies) == 0 {
		t.Fatalf("parsed no terms (actions=%d verifies=%d) — the table format has drifted from this parser", len(actions), len(verifies))
	}

	assertSetsAgree(t, "action", closedSetCaseActions, actions)
	assertSetsAgree(t, "verify", closedSetCaseVerbs, verifies)
}

// schemaTableTerms collects the first-column term of every row in the markdown
// table following the given heading, stopping at the next heading.
func schemaTableTerms(doc, heading string) map[string]bool {
	terms := map[string]bool{}
	idx := strings.Index(doc, heading)
	if idx < 0 {
		return terms
	}
	for _, line := range strings.Split(doc[idx+len(heading):], "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			break // next section
		}
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) == 0 {
			continue
		}
		term := strings.TrimSpace(cells[0])
		// Skip the header row and the |---| separator.
		if term == "" || strings.HasPrefix(term, "---") ||
			term == "Action" || term == "Verify" {
			continue
		}
		if strings.ContainsAny(term, " `*") {
			continue // prose row, not a term row
		}
		terms[term] = true
	}
	return terms
}

func assertSetsAgree(t *testing.T, label string, code, schema map[string]bool) {
	t.Helper()
	for term := range schema {
		if !code[term] {
			t.Errorf("%s %q is documented in the schema table but rejected by the validator", label, term)
		}
	}
	for term := range code {
		if !schema[term] {
			t.Errorf("%s %q is accepted by the validator but absent from the schema table — add the row or drop the term", label, term)
		}
	}
}

// A suite that says nothing about where its code goes leaves the decision
// to whoever writes the file, and generate-code — the step that reaches it
// — has no adapter conventions in view. Two components' tests then land in
// two different places in the same project.
func TestTestcasesV2_SuiteWithoutFileIsReported(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - kind: presentation
    name: expense-list-renders
    component: expense-list
    source_refs:
      - "@expenses/expense-list"
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "testcases.yaml", Content: content})
	if !hasOutcomeCode(outcomes, "testcases-file-missing") {
		t.Errorf("expected testcases-file-missing, got %v", codesOf(outcomes))
	}
	// A warning, not an error: every testcases.yaml predates the field, so
	// erroring would fail every project at once over a fact none of them
	// could have recorded.
	for _, o := range outcomes {
		if o.Code == "testcases-file-missing" && o.Severity != SeverityWarning {
			t.Errorf("severity = %q, want warning while the field lands", o.Severity)
		}
	}
}

func TestTestcasesV2_SuiteWithFileIsClean(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - kind: presentation
    name: expense-list-renders
    file: src/components/expense-list.spec.ts
    component: expense-list
    source_refs:
      - "@expenses/expense-list"
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "testcases.yaml", Content: content})
	if hasOutcomeCode(outcomes, "testcases-file-missing") {
		t.Errorf("a suite declaring file: must not be reported: %v", codesOf(outcomes))
	}
}

// Legacy v1 suites predate the field entirely and are exempt — they exit
// the loop at the discriminator check before file: is examined.
func TestTestcasesV2_LegacySuiteIsNotAskedForFile(t *testing.T) {
	content := []byte(`feature: expenses
suites:
  - name: expense-list-renders
    component: expense-list
    intent: "@expenses/see-my-reports"
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "testcases.yaml", Content: content})
	if hasOutcomeCode(outcomes, "testcases-file-missing") {
		t.Errorf("a legacy v1 suite must not be asked for file:, got %v", codesOf(outcomes))
	}
}

// Criterion-driven cases (WP4, Theme 1 A+B+E).
//
// A case exists because a criterion demands it, states its claim checkably, and
// admits what it could not express. Three per-case checks make that structural.

// A v2 case with no criterion: draws the transition warning — nothing records
// why the case exists. It must be a warning, not an error: every testcases.yaml
// predates the field.
func TestValidateTestcasesV2_CaseCriterionMissingWarns(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: totals
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: total shown
        steps:
          - verify: text
            target: total
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-case-criterion-missing") {
		t.Fatalf("a v2 case without criterion: was not flagged; got %v", codesOf(outcomes))
	}
	for _, o := range outcomes {
		if o.Code == "testcases-case-criterion-missing" && o.Severity != SeverityWarning {
			t.Errorf("criterion-missing must be a warning during transition; got %v", o.Severity)
		}
	}
}

// A criterion: block with an empty ref is as uninformative as none at all.
func TestValidateTestcasesV2_CaseCriterionEmptyRefWarns(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: totals
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: total shown
        criterion:
          text: the total is shown
        steps:
          - verify: text
            target: total
`)
	if !findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-case-criterion-missing") {
		t.Error("a criterion: with no ref was accepted")
	}
}

// Legacy v1 suites are exempt from the criterion warning — they exit before the
// v2 discriminator, and their own legacy code already tells the designer to
// regenerate.
func TestValidateTestcasesV2_LegacyCaseNotAskedForCriterion(t *testing.T) {
	content := []byte(`feature: expenses
suites:
  - name: legacy
    intent: "@expenses/submit"
    cases:
      - name: c
        steps:
          - verify: text
            target: total
`)
	if findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-case-criterion-missing") {
		t.Error("a legacy v1 case was asked for a criterion it predates")
	}
}

// A case that declares exercises but whose steps touch none of them acts on
// nothing it claims to — the vacuous ceremony test the coverage count would
// otherwise reward.
func TestValidateTestcasesV2_CaseVacuousRejected(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
        exercises: ["submit-button"]
        steps:
          - action: render
            target: form
          - verify: text
            target: total
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-case-vacuous") {
		t.Fatalf("a case exercising a target no step touches was accepted; got %v", codesOf(outcomes))
	}
	if !findMessage(outcomes, "submit-button") {
		t.Error("the vacuous diagnostic does not name the untouched exercise target")
	}
}

// A case whose steps do touch a declared exercise is not vacuous.
func TestValidateTestcasesV2_CaseExercisesTouchedIsClean(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
        exercises: ["submit-button"]
        steps:
          - action: click
            target: submit-button
          - verify: state
            target: ExpenseReport.status
`)
	if findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-case-vacuous") {
		t.Error("a case whose step touches its exercise target was wrongly called vacuous")
	}
}

// A verify step reading a target the case does not declare it observes is a
// claim the declaration does not admit.
func TestValidateTestcasesV2_CaseClaimsUnmetRejected(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
        observes: ["ExpenseReport.status"]
        steps:
          - action: click
            target: submit-button
          - verify: text
            target: sneaky-total
`)
	outcomes := ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content})
	if !findCode(outcomes, "testcases-case-claims-unmet") {
		t.Fatalf("an assertion outside observes: was accepted; got %v", codesOf(outcomes))
	}
	if !findMessage(outcomes, "sneaky-total") {
		t.Error("the claims-unmet diagnostic does not name the undeclared read")
	}
}

// A verify step reading a declared observes target is clean; a case with no
// observes: declared is not checked at all (nothing to hold it against).
func TestValidateTestcasesV2_CaseObservesSatisfiedIsClean(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
        observes: ["ExpenseReport.status"]
        steps:
          - verify: state
            target: ExpenseReport.status
`)
	if findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: content}), "testcases-case-claims-unmet") {
		t.Error("a verify reading a declared observes target was wrongly flagged")
	}

	// No observes: declared → not checked.
	noObserves := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
        steps:
          - verify: text
            target: anything
`)
	if findCode(ValidateTestcasesV2(ModeBuild, TestcasesV2Input{Path: "test", Content: noObserves}), "testcases-case-claims-unmet") {
		t.Error("claims-unmet fired on a case that declared no observes:")
	}
}

// The criterion walker is the complement of the operation walker: it holds the
// contract's verify: entries against the cases, firing verify-criterion-uncovered
// for a criterion no case discharges.
func TestValidateTestcasesV2_CriterionUncovered(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
          text: "the title reads Submit"
        steps:
          - verify: text
            target: title
`)
	// The contract declares a criterion for an operation no case cites.
	in := TestcasesV2Input{
		Path:    "test",
		Content: content,
		Criteria: []CriterionRef{
			{Ref: "@expenses/operation:report.submit", Text: "submitting stores the report"},
			{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"},
		},
	}
	outcomes := ValidateTestcasesV2(ModeBuild, in)
	if !findCode(outcomes, "verify-criterion-uncovered") {
		t.Fatalf("expected verify-criterion-uncovered for the uncited operation criterion; got %+v", outcomes)
	}
	// The fragment criterion is discharged by the case, so only the operation
	// criterion should be reported — exactly one finding, at warning severity.
	var reported []ValidationOutcome
	for _, o := range outcomes {
		if o.Code == "verify-criterion-uncovered" {
			reported = append(reported, o)
		}
	}
	if len(reported) != 1 {
		t.Fatalf("expected exactly one uncovered criterion, got %d: %+v", len(reported), reported)
	}
	if !strings.Contains(reported[0].Message, "@expenses/operation:report.submit") {
		t.Errorf("finding names the wrong criterion: %q", reported[0].Message)
	}
	if reported[0].Severity != SeverityWarning {
		t.Errorf("verify-criterion-uncovered severity = %q, want warning", reported[0].Severity)
	}
}

// A criterion every contract entry states and every case discharges draws
// nothing — the covered path.
func TestValidateTestcasesV2_CriterionCovered(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
          text: "the title reads Submit"
        steps:
          - verify: text
            target: title
`)
	in := TestcasesV2Input{
		Path:     "test",
		Content:  content,
		Criteria: []CriterionRef{{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"}},
	}
	if findCode(ValidateTestcasesV2(ModeBuild, in), "verify-criterion-uncovered") {
		t.Error("verify-criterion-uncovered fired on a criterion a case discharges")
	}
}

// An exemption recorded in coverage-review.yaml (surfaced here as ExemptCriteria)
// excuses a criterion from needing a case, the same way the coverage-review gate
// honors it.
func TestValidateTestcasesV2_CriterionExempted(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases: []
`)
	in := TestcasesV2Input{
		Path:     "test",
		Content:  content,
		Criteria: []CriterionRef{{Ref: "@expenses/operation:report.submit", Text: "submitting stores the report"}},
		ExemptCriteria: ExemptedCriteria{
			Entries: map[string]bool{"@expenses/operation:report.submit": true},
		},
	}
	if findCode(ValidateTestcasesV2(ModeBuild, in), "verify-criterion-uncovered") {
		t.Error("verify-criterion-uncovered fired on an exempted criterion")
	}
}

// ---------------------------------------------------------------------------
// Criterion identity (WS 0). Coverage used to be counted per contract ENTRY:
// one ref for an operation with five verify: bullets, so a single citing case
// marked all five discharged. These hold the walker to bullet granularity.
// ---------------------------------------------------------------------------

// criterionFixture is a testcases.yaml whose one case cites one criterion.
func criterionFixture(ref, text string) []byte {
	return []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "` + ref + `"
          text: "` + text + `"
        steps:
          - verify: text
            target: title
`)
}

// The defect this workstream exists for: an entry with several bullets and one
// citing case leaves the other bullets uncovered. Under the old per-entry model
// this reported nothing at all.
func TestValidateTestcasesV2_CoverageIsPerBulletNotPerEntry(t *testing.T) {
	in := TestcasesV2Input{
		Path:    "test",
		Content: criterionFixture("@expenses/fragment:submit", "the title reads Submit"),
		Criteria: []CriterionRef{
			{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"},
			{Ref: "@expenses/fragment:submit", Text: "the button is disabled while saving"},
			{Ref: "@expenses/fragment:submit", Text: "an error banner names the failing field"},
		},
	}
	var uncovered []ValidationOutcome
	for _, o := range ValidateTestcasesV2(ModeBuild, in) {
		if o.Code == "verify-criterion-uncovered" {
			uncovered = append(uncovered, o)
		}
	}
	if len(uncovered) != 2 {
		t.Fatalf("expected 2 uncovered bullets on the cited entry, got %d: %+v", len(uncovered), uncovered)
	}
	for _, o := range uncovered {
		if strings.Contains(o.Message, "the title reads Submit") {
			t.Errorf("the discharged bullet was reported uncovered: %q", o.Message)
		}
	}
}

// A case citing a ref no contract entry declares. Previously invisible: an
// unknown ref simply marked nothing covered, so it read as a gap elsewhere.
func TestValidateTestcasesV2_CriterionRefUnknown(t *testing.T) {
	in := TestcasesV2Input{
		Path:     "test",
		Content:  criterionFixture("@expenses/fragment:typo", "the title reads Submit"),
		Criteria: []CriterionRef{{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"}},
	}
	outcomes := ValidateTestcasesV2(ModeBuild, in)
	if !findCode(outcomes, "testcases-criterion-ref-unknown") {
		t.Fatalf("expected testcases-criterion-ref-unknown; got %+v", outcomes)
	}
}

// A case citing a known entry with text matching none of its current bullets.
// This is the check testcases.schema.md has always described criterion.text as
// performing, and which it did not perform: the field was decoded and dropped.
func TestValidateTestcasesV2_CriterionTextDrift(t *testing.T) {
	in := TestcasesV2Input{
		Path:     "test",
		Content:  criterionFixture("@expenses/fragment:submit", "the heading reads Submit"),
		Criteria: []CriterionRef{{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"}},
	}
	outcomes := ValidateTestcasesV2(ModeBuild, in)
	if !findCode(outcomes, "testcases-criterion-text-drift") {
		t.Fatalf("expected testcases-criterion-text-drift; got %+v", outcomes)
	}
	if findCode(outcomes, "testcases-criterion-ref-unknown") {
		t.Error("drift on a known ref was reported as an unknown ref")
	}
}

// A case citing a ref with no text at all — what every pre-WS-0 testcases.yaml
// looks like. Reported distinctly from drift so the fix ("rebuild") differs
// from the fix ("the contract was reworded").
func TestValidateTestcasesV2_CriterionTextMissing(t *testing.T) {
	content := []byte(`schema_version: 2
feature: expenses
suites:
  - name: submit
    kind: presentation
    file: src/x.spec.ts
    source_refs: ["@expenses/submit"]
    cases:
      - name: submitting the report
        criterion:
          ref: "@expenses/fragment:submit"
        steps:
          - verify: text
            target: title
`)
	in := TestcasesV2Input{
		Path:     "test",
		Content:  content,
		Criteria: []CriterionRef{{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"}},
	}
	outcomes := ValidateTestcasesV2(ModeBuild, in)
	if !findCode(outcomes, "testcases-criterion-text-missing") {
		t.Fatalf("expected testcases-criterion-text-missing; got %+v", outcomes)
	}
	if findCode(outcomes, "testcases-criterion-text-drift") {
		t.Error("a missing text was reported as drift; the two have different fixes")
	}
}

// A feature with no resolvable contract supplies no criteria. Every citation
// would then look unknown, which is a fact about the missing input rather than
// about the file — the same rule the empty-input walkers already follow.
func TestValidateTestcasesV2_NoContractSuppressesCitationChecks(t *testing.T) {
	in := TestcasesV2Input{
		Path:    "test",
		Content: criterionFixture("@expenses/fragment:submit", "the title reads Submit"),
	}
	outcomes := ValidateTestcasesV2(ModeBuild, in)
	if findCode(outcomes, "testcases-criterion-ref-unknown") {
		t.Errorf("citation checks ran with no contract loaded: %+v", outcomes)
	}
}

// Normalization is narrow by design: trim and line endings only. Leading
// whitespace must not defeat identity, and a case difference must not silently
// unify two bullets.
func TestValidateTestcasesV2_CriterionTextNormalization(t *testing.T) {
	in := TestcasesV2Input{
		Path:     "test",
		Content:  criterionFixture("@expenses/fragment:submit", "the title reads Submit"),
		Criteria: []CriterionRef{{Ref: "@expenses/fragment:submit", Text: "  the title reads Submit  "}},
	}
	if findCode(ValidateTestcasesV2(ModeBuild, in), "verify-criterion-uncovered") {
		t.Error("surrounding whitespace defeated criterion identity")
	}

	// Case differences are preserved: two bullets differing only in
	// capitalization are two claims, not one.
	in.Criteria = []CriterionRef{{Ref: "@expenses/fragment:submit", Text: "The title reads Submit"}}
	if !findCode(ValidateTestcasesV2(ModeBuild, in), "verify-criterion-uncovered") {
		t.Error("normalization lowercased, merging two materially distinct bullets")
	}
}

// Two identical bullets on one entry cannot be discharged separately, so they
// are reported as an authoring defect rather than given an invented index.
func TestValidateTestcasesV2_DuplicateCriterion(t *testing.T) {
	in := TestcasesV2Input{
		Path:    "test",
		Content: criterionFixture("@expenses/fragment:submit", "the title reads Submit"),
		Criteria: []CriterionRef{
			{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"},
			{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"},
		},
	}
	var dupes []ValidationOutcome
	for _, o := range ValidateTestcasesV2(ModeBuild, in) {
		if o.Code == "verify-criterion-duplicate" {
			dupes = append(dupes, o)
		}
	}
	if len(dupes) != 1 {
		t.Fatalf("expected exactly one duplicate finding (per pair, not per occurrence), got %d: %+v", len(dupes), dupes)
	}
}

// An exemption naming a text excuses exactly that bullet and leaves the entry's
// other bullets still requiring a case.
func TestValidateTestcasesV2_BulletSpecificExemption(t *testing.T) {
	in := TestcasesV2Input{
		Path:    "test",
		Content: criterionFixture("@expenses/fragment:submit", "the title reads Submit"),
		Criteria: []CriterionRef{
			{Ref: "@expenses/fragment:submit", Text: "the title reads Submit"},
			{Ref: "@expenses/fragment:submit", Text: "the button is disabled while saving"},
			{Ref: "@expenses/fragment:submit", Text: "an error banner names the failing field"},
		},
		ExemptCriteria: ExemptedCriteria{
			Bullets: map[CriterionRef]bool{
				{Ref: "@expenses/fragment:submit", Text: "the button is disabled while saving"}: true,
			},
		},
	}
	var uncovered []ValidationOutcome
	for _, o := range ValidateTestcasesV2(ModeBuild, in) {
		if o.Code == "verify-criterion-uncovered" {
			uncovered = append(uncovered, o)
		}
	}
	if len(uncovered) != 1 {
		t.Fatalf("expected 1 uncovered bullet (one discharged, one exempted), got %d: %+v", len(uncovered), uncovered)
	}
	if !strings.Contains(uncovered[0].Message, "an error banner") {
		t.Errorf("wrong bullet reported: %q", uncovered[0].Message)
	}
}

// The compatibility rule: an exemption with no text is entry-wide. Every
// exemption written before bullet-level coverage omits it and none could have
// recorded one, so reading absence as "excuses nothing" would silently revoke
// exemptions a human already granted.
func TestValidateTestcasesV2_LegacyExemptionStaysEntryWide(t *testing.T) {
	in := TestcasesV2Input{
		Path:    "test",
		Content: criterionFixture("@expenses/fragment:submit", "the title reads Submit"),
		Criteria: []CriterionRef{
			{Ref: "@expenses/operation:report.submit", Text: "stores the report"},
			{Ref: "@expenses/operation:report.submit", Text: "rejects a duplicate"},
			{Ref: "@expenses/operation:report.submit", Text: "emits an audit row"},
		},
		ExemptCriteria: ExemptedCriteria{
			Entries: map[string]bool{"@expenses/operation:report.submit": true},
		},
	}
	if findCode(ValidateTestcasesV2(ModeBuild, in), "verify-criterion-uncovered") {
		t.Error("a legacy ref-only exemption stopped excusing its entry's bullets")
	}
}
