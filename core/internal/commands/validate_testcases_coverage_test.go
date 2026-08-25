// parlay-feature: parlay-tool
// parlay-component: validate
// parlay-artifact: test
//
// The coverage inputs for `parlay validate --type testcases` are derived from
// the feature's contract artifacts, resolved from the testcases.yaml's own build
// path. Before this the only caller passed nil, so the operation-coverage walker
// had never fired against a real feature; these tests prove the derivation and
// that the walker fires once it is fed.

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
)

func writeFileForTest(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rootedContext builds a standalone *config.Context whose active root is dir,
// without chdir — the coverage derivation resolves everything from the passed
// path relative to the root's own BuildRoot().
func rootedContext(dir string) *config.Context {
	return config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{
			Name: filepath.Base(dir),
			Path: dir,
			Kind: config.RootKindStandalone,
		},
		Source: config.SourceCwdWalkUp,
	}, nil)
}

func TestTestcasesCoverageInputs_DerivesFromContractArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeFileForTest(t, filepath.Join(dir, "spec", "intents", "expenses", "capabilities.yaml"), `schema_version: 1
feature: expenses
operations:
  - id: report.submit
    kind: command
    subject:
      entity: ExpenseReport
    verify:
      - "the report moves to submitted"
  - id: report.list
    kind: query
    subject:
      entity: ExpenseReport
`)
	writeFileForTest(t, filepath.Join(dir, "spec", "intents", "expenses", "surface.yaml"), `feature: expenses
fragments:
  - name: submit-form
    shows: the submission form
    verify:
      - "the form validates before submit"
  - name: list-row
    shows: a report row
`)

	tcPath := filepath.Join(dir, ".parlay", "build", "expenses", "testcases.yaml")
	writeFileForTest(t, tcPath, "schema_version: 2\nfeature: expenses\nsuites: []\n")

	cmd := testCommandWithContext(t, rootedContext(dir))
	in := testcasesCoverageInputs(cmd, tcPath)

	// Every declared operation is a canonical-operation subject.
	wantOps := map[string]bool{
		"@expenses/operation:report.submit": false,
		"@expenses/operation:report.list":   false,
	}
	for _, op := range in.CanonicalOperations {
		if _, ok := wantOps[op]; ok {
			wantOps[op] = true
		}
	}
	for op, seen := range wantOps {
		if !seen {
			t.Errorf("canonical operation %q not derived; got %+v", op, in.CanonicalOperations)
		}
	}

	// Only entries carrying verify: become criteria: the submit operation and
	// the submit-form fragment, not the verify-less list operation or list-row
	// fragment.
	gotCrit := map[string]bool{}
	for _, c := range in.Criteria {
		gotCrit[c.Ref] = true
	}
	for _, want := range []string{"@expenses/operation:report.submit", "@expenses/fragment:submit-form"} {
		if !gotCrit[want] {
			t.Errorf("criterion %q not derived; got %+v", want, in.Criteria)
		}
	}
	for _, notWant := range []string{"@expenses/operation:report.list", "@expenses/fragment:list-row"} {
		if gotCrit[notWant] {
			t.Errorf("criterion %q was derived but its entry carries no verify:", notWant)
		}
	}
}

// The payoff: fed real operations, the coverage walker fires for a testcases.yaml
// that covers none of them — the check that had never run against a real feature.
func TestTestcasesCoverageInputs_WalkerFiresWithRealOps(t *testing.T) {
	dir := t.TempDir()
	writeFileForTest(t, filepath.Join(dir, "spec", "intents", "expenses", "capabilities.yaml"), `schema_version: 1
feature: expenses
operations:
  - id: report.submit
    kind: command
    subject:
      entity: ExpenseReport
    verify:
      - "the report moves to submitted"
`)

	tcPath := filepath.Join(dir, ".parlay", "build", "expenses", "testcases.yaml")
	// A presentation-only suite: no operation suite covers report.submit, and no
	// case discharges its criterion.
	tcBody := `schema_version: 2
feature: expenses
suites:
  - name: form
    kind: presentation
    file: src/form.spec.ts
    component: SubmitForm
    source_refs: ["@expenses/submit-form"]
    cases: []
`
	writeFileForTest(t, tcPath, tcBody)

	cmd := testCommandWithContext(t, rootedContext(dir))
	in := testcasesCoverageInputs(cmd, tcPath)
	in.Content = []byte(tcBody)

	outcomes := agent.ValidateTestcasesV2(agent.ModeBuild, in)
	if !findOutcomeCode(outcomes, "testcases-operation-uncovered") {
		t.Errorf("operation walker did not fire for an uncovered real operation; got %+v", outcomes)
	}
	if !findOutcomeCode(outcomes, "verify-criterion-uncovered") {
		t.Errorf("criterion walker did not fire for an undischarged real criterion; got %+v", outcomes)
	}
}

// A testcases.yaml handed to the CLI by a path outside any build tree resolves
// no feature, so the coverage inputs are empty — the walkers go quiet rather
// than reporting everything as covered.
func TestTestcasesCoverageInputs_NoFeatureOutsideBuildTree(t *testing.T) {
	dir := t.TempDir()
	stray := filepath.Join(dir, "loose", "testcases.yaml")
	writeFileForTest(t, stray, "schema_version: 2\nfeature: x\nsuites: []\n")

	cmd := testCommandWithContext(t, rootedContext(dir))
	in := testcasesCoverageInputs(cmd, stray)
	if len(in.CanonicalOperations) != 0 || len(in.Criteria) != 0 {
		t.Errorf("expected empty coverage inputs for a path outside the build tree; got ops=%+v criteria=%+v", in.CanonicalOperations, in.Criteria)
	}
}

func findOutcomeCode(outcomes []agent.ValidationOutcome, code string) bool {
	for _, o := range outcomes {
		if o.Code == code {
			return true
		}
	}
	return false
}
