package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// writeTestcases drops a testcases.yaml beside a feature's buildfile, for the
// route-suite inference path.
func writeTestcases(t *testing.T, cfg *config.Context, feature, body string) {
	t.Helper()
	dir := cfg.BuildPath(feature)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testcases.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func deriveSeedForTest(t *testing.T, cfg *config.Context) seedOutput {
	t.Helper()
	features, err := cfg.AllFeatures()
	if err != nil {
		t.Fatal(err)
	}
	return deriveSeed(cfg, features)
}

// Two features that both know about the same employee, each carrying the
// fields it needed, must produce one record holding both. This is the whole
// point of the composed seed: before it, the app booted from whichever
// feature's fixture hydrated first, so the same person had a name on one
// screen and a department on another.
func TestSeedUnionsAgreeingContributors(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"submit-expense": `feature: submit-expense
fixtures:
  draft:
    composes: true
    data:
      Employee:
        - id: emp-1
          name: Dana
          role: employee
`,
		"approvals/review-queue": `feature: review-queue
fixtures:
  queue:
    composes: true
    data:
      Employee:
        - id: emp-1
          name: Dana
          department: Field Ops
        - id: emp-2
          name: Sam
`,
	})

	out := deriveSeedForTest(t, cfg)
	if !out.Derivable {
		t.Fatalf("expected a derivable seed, got findings: %+v", out.Findings)
	}
	if len(out.Records) != 2 {
		t.Fatalf("expected 2 records, got %d: %+v", len(out.Records), out.Records)
	}

	var dana *seedRecord
	for i := range out.Records {
		if out.Records[i].ID == "emp-1" {
			dana = &out.Records[i]
		}
	}
	if dana == nil {
		t.Fatal("emp-1 missing from the seed")
	}
	for _, field := range []string{"name", "role", "department"} {
		if _, ok := dana.Fields[field]; !ok {
			t.Errorf("emp-1 lost field %q in the union: %+v", field, dana.Fields)
		}
	}
	if len(dana.From) != 2 {
		t.Errorf("emp-1 should credit both contributors, got %v", dana.From)
	}
}

// A field two features disagree on stops the derivation. There is one
// runtime, so "submitted" and "rejected" cannot both be true of one report —
// and quietly picking a winner would hide exactly the defect the seed exists
// to expose. The author is asked; the tool does not guess.
func TestSeedRefusesOnDisagreement(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"expenses-list": `feature: expenses-list
fixtures:
  mine:
    composes: true
    data:
      ExpenseReport:
        - id: rpt-1
          status: submitted
`,
		"approvals/review-queue": `feature: review-queue
fixtures:
  queue:
    composes: true
    data:
      ExpenseReport:
        - id: rpt-1
          status: rejected
`,
	})

	out := deriveSeedForTest(t, cfg)
	if out.Derivable {
		t.Fatal("a contradiction must make the seed underivable")
	}
	found := false
	for _, f := range out.Findings {
		if f.Code == "composition-fixture-contradiction" {
			found = true
			for _, want := range []string{"rpt-1", "status", "submitted", "rejected"} {
				if !strings.Contains(f.Message, want) {
					t.Errorf("message should name %q: %s", want, f.Message)
				}
			}
			if len(f.Sites) != 2 {
				t.Errorf("both contributors should be named as sites, got %v", f.Sites)
			}
		}
	}
	if !found {
		t.Fatalf("expected composition-fixture-contradiction, got %+v", out.Findings)
	}
}

// The canonical-output guarantee. Every consumer diffs this output to decide
// whether the seed changed, so map iteration order leaking into it would show
// phantom drift on every run and make real drift unreadable.
func TestSeedOrderingIsStableAcrossRuns(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"submit-expense": `feature: submit-expense
fixtures:
  draft:
    composes: true
    data:
      Employee:
        - id: emp-3
          name: Ash
        - id: emp-1
          name: Dana
      ExpenseReport:
        - id: rpt-2
          status: draft
        - id: rpt-1
          status: submitted
`,
		"approvals/review-queue": `feature: review-queue
fixtures:
  queue:
    composes: true
    data:
      Employee:
        - id: emp-2
          name: Sam
      ExpenseReport:
        - id: rpt-1
          owner: emp-1
`,
	})

	first, err := json.Marshal(deriveSeedForTest(t, cfg))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		next, err := json.Marshal(deriveSeedForTest(t, cfg))
		if err != nil {
			t.Fatal(err)
		}
		if string(next) != string(first) {
			t.Fatalf("run %d differs from run 1:\n %s\n %s", i+2, first, next)
		}
	}

	// And the order is the documented one, not merely repeatable.
	out := deriveSeedForTest(t, cfg)
	var got []string
	for _, r := range out.Records {
		got = append(got, r.Entity+"/"+r.ID)
	}
	want := []string{
		"Employee/emp-1", "Employee/emp-2", "Employee/emp-3",
		"ExpenseReport/rpt-1", "ExpenseReport/rpt-2",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("records not sorted by entity then id:\n got %v\nwant %v", got, want)
	}
}

// With no `composes:` marker, the fixture named by the feature's route suite
// is the deterministic answer — a route suite is by definition "everything
// this route renders", which is the same question the seed asks.
func TestSeedInfersComposingFixtureFromRouteSuite(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"expenses-list": `feature: expenses-list
fixtures:
  empty-state:
    data:
      ExpenseReport: []
  three-reports:
    data:
      ExpenseReport:
        - id: rpt-1
          status: submitted
`,
	})
	writeTestcases(t, cfg, "expenses-list", `suites:
  - kind: presentation
    scope: component
    name: row
    fixture: empty-state
  - kind: presentation
    scope: route
    name: expenses-route
    fixture: three-reports
`)

	out := deriveSeedForTest(t, cfg)
	if !out.Derivable {
		t.Fatalf("expected derivable, got %+v", out.Findings)
	}
	if got := out.Contributors["expenses-list"]; got != "three-reports" {
		t.Errorf("expected the route suite's fixture, got %q", got)
	}
	if len(out.Records) != 1 {
		t.Errorf("expected only the route fixture's rows, got %+v", out.Records)
	}
}

// A feature's fixtures are SUPPOSED to disagree — an empty state and a
// populated one are different scenarios over the same ids. So when nothing
// says which one boots the app, the tool must ask rather than union them,
// which would manufacture contradictions out of correct authoring.
func TestSeedAmbiguityIsReportedNotGuessed(t *testing.T) {
	cases := map[string]struct {
		buildfile string
		testcases string
		want      string
	}{
		"two fixtures both marked": {
			buildfile: `feature: f
fixtures:
  a:
    composes: true
    data: {}
  b:
    composes: true
    data: {}
`,
			want: "exactly one fixture boots the app",
		},
		"route suites disagree": {
			buildfile: `feature: f
fixtures:
  a:
    data: {}
  b:
    data: {}
`,
			testcases: `suites:
  - scope: route
    name: one
    fixture: a
  - scope: route
    name: two
    fixture: b
`,
			want: "mark one `composes: true`",
		},
		"no marker and no route suite": {
			buildfile: `feature: f
fixtures:
  a:
    data: {}
`,
			testcases: `suites:
  - scope: component
    name: one
    fixture: a
`,
			want: "no `scope: route` suite to infer one from",
		},
		"route suite names an undeclared fixture": {
			buildfile: `feature: f
fixtures:
  a:
    data: {}
`,
			testcases: `suites:
  - scope: route
    name: one
    fixture: ghost
`,
			want: "which the buildfile does not declare",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := setupCompositionProject(t, map[string]string{"f": tc.buildfile})
			if tc.testcases != "" {
				writeTestcases(t, cfg, "f", tc.testcases)
			}
			out := deriveSeedForTest(t, cfg)
			if out.Derivable {
				t.Fatal("ambiguity must make the seed underivable")
			}
			if len(out.Findings) != 1 || out.Findings[0].Code != "composition-seed-ambiguous" {
				t.Fatalf("expected one composition-seed-ambiguous, got %+v", out.Findings)
			}
			if !strings.Contains(out.Findings[0].Message, tc.want) {
				t.Errorf("message should say %q: %s", tc.want, out.Findings[0].Message)
			}
		})
	}
}

// An unbuilt feature contributes nothing and is not an error here.
// check-composition is where "this feature has no build output" is reported;
// duplicating it would make the seed refuse on a project that is merely
// mid-pipeline.
func TestSeedSkipsUnbuiltFeatures(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"dashboard": "",
		"submit-expense": `feature: submit-expense
fixtures:
  draft:
    composes: true
    data:
      Employee:
        - id: emp-1
          name: Dana
`,
	})

	out := deriveSeedForTest(t, cfg)
	if !out.Derivable {
		t.Fatalf("an unbuilt feature must not break the seed: %+v", out.Findings)
	}
	if _, ok := out.Contributors["dashboard"]; ok {
		t.Error("dashboard has no buildfile and cannot contribute")
	}
	if len(out.Records) != 1 {
		t.Errorf("expected the built feature's row only, got %+v", out.Records)
	}
}

// Fields whose values are lists or maps are merged without being compared,
// the same rule check-composition applies: two features legitimately load
// different depths of the same record, and calling that a contradiction would
// fire on correct fixtures.
func TestSeedDoesNotCompareNonScalarFields(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"expenses-list": `feature: expenses-list
fixtures:
  a:
    composes: true
    data:
      ExpenseReport:
        - id: rpt-1
          lineItems: []
`,
		"approvals/review-queue": `feature: review-queue
fixtures:
  b:
    composes: true
    data:
      ExpenseReport:
        - id: rpt-1
          lineItems:
            - id: li-1
`,
	})

	out := deriveSeedForTest(t, cfg)
	if !out.Derivable {
		t.Fatalf("non-scalar difference must not refuse the seed: %+v", out.Findings)
	}
}

// A refusal must withhold the data, not merely flag it.
//
// The records were emitted alongside derivable:false, and they embodied exactly
// the last-writer-wins the design forbids: the contradicting field held
// whichever contributor sorted last. A consumer reading `records` without
// first checking `derivable` got a silently reconciled seed — the defect the
// refusal exists to prevent, reintroduced one field over.
func TestRefusedSeedEmitsNoRecords(t *testing.T) {
	cfg := setupCompositionProject(t, map[string]string{
		"expenses-list": `feature: expenses-list
fixtures:
  mine:
    composes: true
    data:
      ExpenseReport:
        - id: rpt-1
          status: submitted
      Employee:
        - id: emp-1
          name: Dana
`,
		"approvals/review-queue": `feature: review-queue
fixtures:
  queue:
    composes: true
    data:
      ExpenseReport:
        - id: rpt-1
          status: rejected
`,
	})

	out := deriveSeedForTest(t, cfg)
	if out.Derivable {
		t.Fatal("expected a refusal")
	}
	if len(out.Records) != 0 {
		t.Fatalf("a refused derivation must emit no records, got %d: %+v", len(out.Records), out.Records)
	}
	// The uncontradicted records are withheld too — a seed missing the rows the
	// contradiction touched is not a seed, it is a different dataset.
	for _, r := range out.Records {
		if r.Entity == "Employee" {
			t.Error("no partial seed: uncontradicted rows must not ship either")
		}
	}
}
