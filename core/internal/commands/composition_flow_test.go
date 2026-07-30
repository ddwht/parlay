package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// setupFlowProject writes a buildfile and testcases per feature and returns
// the context. A feature whose testcases entry is "" gets no testcases.yaml.
func setupFlowProject(t *testing.T, buildfiles, testcases map[string]string) *config.Context {
	t.Helper()
	cfg := setupCompositionProject(t, buildfiles)
	for feature, body := range testcases {
		if body == "" {
			continue
		}
		dir := cfg.BuildPath(feature)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "testcases.yaml"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

const reviewBuildfile = `feature: review-queue
routes:
  - path: /review
plan:
  creates:
    - path: src/app/features/review-queue/review-queue.component.ts
`

const expensesBuildfile = `feature: expenses-list
routes:
  - path: /expenses
plan:
  creates:
    - path: src/app/features/expenses-list/expenses-list.component.ts
`

// The flow this whole item exists for: approve on one feature's route, then
// assert the report reads approved on another's.
const crossFeatureStateFlow = `suites:
  - kind: presentation
    scope: flow
    name: approved-report-appears-in-the-owners-expense-list
    flow:
      - /review
      - /expenses
    cases:
      - name: after approval the report reads approved for its owner
        steps:
          - action: navigate
            target: /review
          - action: click
            target: approve-report
          - action: navigate
            target: /expenses
          - verify: state
            target: ExpenseReport.status
            expected: approved
`

func flowFindingsFor(t *testing.T, cfg *config.Context, storePath string) []compositionFinding {
	t.Helper()
	features, err := cfg.AllFeatures()
	if err != nil {
		t.Fatal(err)
	}
	return findUnsatisfiableFlows(cfg, features, storePath)
}

// With no shared store anywhere, the assertion cannot hold however the code
// is written. This is the finding the first regression run never produced:
// the generating agent weakened the assertion instead and explained itself in
// a comment, so the suite went green and nothing upstream learned the journey
// was broken.
func TestCrossFeatureStateAssertionIsReportedWhenNoStoreExists(t *testing.T) {
	cfg := setupFlowProject(t,
		map[string]string{"review-queue": reviewBuildfile, "expenses-list": expensesBuildfile},
		map[string]string{"review-queue": crossFeatureStateFlow})

	got := flowFindingsFor(t, cfg, "")
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %#v", got)
	}
	if got[0].Code != "composition-flow-unsatisfiable" {
		t.Errorf("code = %q", got[0].Code)
	}
	for _, want := range []string{"approved-report-appears", "paths.store", "review-queue", "expenses-list"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message should name %q: %s", want, got[0].Message)
		}
	}
}

// Navigating from one feature into another and checking you arrived needs no
// shared runtime — nothing written in the first feature has to be visible in
// the second. Reporting these would bury the one case that matters under two
// that do not, which is how the real finding went unnoticed for a full run.
func TestPureNavigationAcrossFeaturesIsNotReported(t *testing.T) {
	const navFlow = `suites:
  - kind: presentation
    scope: flow
    name: empty-state-to-submit-wizard
    flow:
      - /expenses
      - /review
    cases:
      - name: the call to action opens the other feature
        steps:
          - action: navigate
            target: /expenses
          - verify: visible
            target: no-reports-placeholder
            expected: true
          - action: click
            target: submit-first-expense
          - verify: route
            expected: /review
`
	cfg := setupFlowProject(t,
		map[string]string{"review-queue": reviewBuildfile, "expenses-list": expensesBuildfile},
		map[string]string{"expenses-list": navFlow})

	if got := flowFindingsFor(t, cfg, ""); len(got) != 0 {
		t.Errorf("a navigation-only flow needs no shared runtime: %#v", got)
	}
}

// A state assertion inside one feature is ordinary and always satisfiable.
// The crossing is what makes it a composition question.
func TestStateAssertionWithinOneFeatureIsNotReported(t *testing.T) {
	const sameFeature = `suites:
  - kind: presentation
    scope: flow
    name: approve-from-the-queue
    flow:
      - /review
      - /review
    cases:
      - name: approving updates the report
        steps:
          - action: navigate
            target: /review
          - action: click
            target: approve-report
          - verify: state
            target: ExpenseReport.status
            expected: approved
`
	cfg := setupFlowProject(t,
		map[string]string{"review-queue": reviewBuildfile, "expenses-list": expensesBuildfile},
		map[string]string{"review-queue": sameFeature})

	if got := flowFindingsFor(t, cfg, ""); len(got) != 0 {
		t.Errorf("a state assertion within one feature is satisfiable: %#v", got)
	}
}

// The store exists and every participant wires it — the journey works, and
// nothing is reported. This is the state Stage 3C is trying to reach, and
// asserting it is what stops the check from being a permanent complaint.
func TestNothingIsReportedWhenEveryParticipantWiresTheStore(t *testing.T) {
	const store = "src/app/core/state/domain.store.ts"
	withStore := func(bf string) string {
		return bf + "    - path: " + store + "\n"
	}
	cfg := setupFlowProject(t,
		map[string]string{
			"review-queue":  withStore(reviewBuildfile),
			"expenses-list": withStore(expensesBuildfile),
		},
		map[string]string{"review-queue": crossFeatureStateFlow})

	if got := flowFindingsFor(t, cfg, store); len(got) != 0 {
		t.Errorf("the journey is satisfiable once both features share the store: %#v", got)
	}
}

// The store exists but one feature does not wire it. That is a different
// claim from "this framework has no shared runtime" — the mechanism is there
// and this feature is not using it, so its writes stay local.
func TestFeatureThatSkipsTheDeclaredStoreIsReported(t *testing.T) {
	const store = "src/app/core/state/domain.store.ts"
	cfg := setupFlowProject(t,
		map[string]string{
			"review-queue":  reviewBuildfile + "    - path: " + store + "\n",
			"expenses-list": expensesBuildfile, // does not wire it
		},
		map[string]string{"review-queue": crossFeatureStateFlow})

	got := flowFindingsFor(t, cfg, store)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %#v", got)
	}
	if !strings.Contains(got[0].Message, "expenses-list does not plan to create or modify it") {
		t.Errorf("the message must name the feature that skipped the store: %s", got[0].Message)
	}
	if strings.Contains(got[0].Message, "review-queue does not plan") {
		t.Errorf("review-queue wires the store and must not be blamed: %s", got[0].Message)
	}
}

// A route no feature declares — the unbuilt dashboard — is not a crossing
// into anything. Treating it as one would make every redirect-away-from-a-
// guarded-route flow look like a composition failure.
func TestRouteOwnedByNoFeatureIsNotACrossing(t *testing.T) {
	const redirectFlow = `suites:
  - kind: presentation
    scope: flow
    name: employee-redirected-away-from-the-queue
    flow:
      - /review
      - /dashboard
    cases:
      - name: an employee is sent to the dashboard
        steps:
          - action: navigate
            target: /review
          - verify: route
            expected: /dashboard
          - verify: state
            target: Employee.role
            expected: employee
`
	cfg := setupFlowProject(t,
		map[string]string{"review-queue": reviewBuildfile, "expenses-list": expensesBuildfile},
		map[string]string{"review-queue": redirectFlow})

	if got := flowFindingsFor(t, cfg, ""); len(got) != 0 {
		t.Errorf("/dashboard belongs to no feature, so nothing was crossed: %#v", got)
	}
}
