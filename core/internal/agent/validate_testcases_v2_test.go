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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, []string{"@f/operation:task.delete"})
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
	if !findCode(outcomes, "testcases-source-refs-missing") {
		t.Errorf("missing testcases-source-refs-missing; got %+v", outcomes)
	}
}

func TestValidateTestcasesV2_LegacyV1Warning(t *testing.T) {
	content := []byte(`schema_version: 2
feature: f
suites:
  - name: a
    component: x
    intent: legacy intent
`)
	outcomes := ValidateTestcasesV2(ModeAuthoring, "test", content, nil)
	if !findCode(outcomes, "testcases-source-refs-missing-legacy") {
		t.Errorf("missing testcases-source-refs-missing-legacy; got %+v", outcomes)
	}
	for _, o := range outcomes {
		if o.Code == "testcases-source-refs-missing-legacy" && o.Severity != SeverityWarning {
			t.Errorf("legacy warning should be SeverityWarning; got %v", o.Severity)
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, []string{"@f/operation:task.create"})
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
			outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
		if findCode(ValidateTestcasesV2(ModeBuild, "test", base(ok), nil), "testcases-coverage-unknown") {
			t.Errorf("coverage %q wrongly rejected", ok)
		}
	}
	if !findCode(ValidateTestcasesV2(ModeBuild, "test", base("partial"), nil), "testcases-coverage-unknown") {
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
		if findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-unknown-term") {
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
		if findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-unknown-term") {
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
	if !findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-unknown-term") {
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
	if !findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-unknown-term") {
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
	outcomes := ValidateTestcasesV2(ModeBuild, "testcases.yaml", content, nil)
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
	outcomes := ValidateTestcasesV2(ModeBuild, "testcases.yaml", content, nil)
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
	outcomes := ValidateTestcasesV2(ModeBuild, "testcases.yaml", content, nil)
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
	if !findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-case-criterion-missing") {
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
	if findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-case-criterion-missing") {
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
	if findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-case-vacuous") {
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
	outcomes := ValidateTestcasesV2(ModeBuild, "test", content, nil)
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
	if findCode(ValidateTestcasesV2(ModeBuild, "test", content, nil), "testcases-case-claims-unmet") {
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
	if findCode(ValidateTestcasesV2(ModeBuild, "test", noObserves, nil), "testcases-case-claims-unmet") {
		t.Error("claims-unmet fired on a case that declared no observes:")
	}
}
