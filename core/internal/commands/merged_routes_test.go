// parlay-feature: parlay-tool/multi-adapter
// parlay-component: merged-route-table
// parlay-artifact: test

package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func routedBuildfile(feature string, paths ...string) string {
	var b strings.Builder
	b.WriteString("feature: " + feature + "\nadapter: go-cli\nroutes:\n")
	for _, p := range paths {
		b.WriteString("  - path: " + p + "\n    component: view\n")
	}
	b.WriteString("plan:\n  creates:\n    - path: src/" + feature + ".go\n      sources: [\"component:view\"]\n")
	return b.String()
}

func runMergedRoutes_(t *testing.T) mergedRoutesOutput {
	t.Helper()
	cmd := testCommandWithContext(t, testContext(t))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runMergedRoutes(cmd, nil); err != nil {
		t.Fatalf("merged-routes: %v", err)
	}
	var out mergedRoutesOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("decode merged-routes %q: %v", buf.String(), err)
	}
	return out
}

func TestMergedRoutes_MergesWithProvenance(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "expense-list")
	featureWithIntents(t, dir, "approvals")
	writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml", routedBuildfile("expense-list", "/expenses"))
	writeFixtureFile(t, dir, ".parlay/build/approvals/buildfile.yaml", routedBuildfile("approvals", "/approvals", "/approvals/:id"))

	out := runMergedRoutes_(t)

	if len(out.Routes) != 3 {
		t.Fatalf("expected 3 merged routes, got %+v", out.Routes)
	}
	byPath := map[string]string{}
	for _, r := range out.Routes {
		byPath[r.Path] = r.Feature
	}
	if byPath["/expenses"] != "expense-list" || byPath["/approvals"] != "approvals" {
		t.Errorf("routes lost their provenance: %+v", out.Routes)
	}
	if out.FeaturesRead != 2 {
		t.Errorf("features_read = %d, want 2", out.FeaturesRead)
	}
	// Sorted output keeps the table diffable run to run.
	if out.Routes[0].Path != "/approvals" {
		t.Errorf("routes must be path-sorted for determinism, got %+v", out.Routes)
	}
}

// TestMergedRoutes_ReportsConflictsRatherThanPicking pins the refusal: two
// features claiming one path is a composition question with an owner, and
// silently picking a winner would hide it exactly when codegen depends on
// the answer.
func TestMergedRoutes_ReportsConflictsRatherThanPicking(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "expense-list")
	featureWithIntents(t, dir, "approvals")
	writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml", routedBuildfile("expense-list", "/shared"))
	writeFixtureFile(t, dir, ".parlay/build/approvals/buildfile.yaml", routedBuildfile("approvals", "/shared"))

	out := runMergedRoutes_(t)

	if len(out.Conflicts) != 1 || out.Conflicts[0] != "/shared" {
		t.Errorf("expected /shared reported as a conflict, got %v", out.Conflicts)
	}
	if len(out.Routes) != 1 {
		t.Errorf("a conflicted path still contributes exactly one row, got %+v", out.Routes)
	}
}

func TestMergedRoutes_JoinsBlueprintNavigation(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "expense-list")
	writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml", routedBuildfile("expense-list", "/expenses"))
	writeFixtureFile(t, dir, ".parlay/blueprint.yaml", `app: demo
navigation:
  strategy: browser
  default-route: /expenses
  routes:
    - path: /expenses
      shell: main
      guard: authed
`)

	out := runMergedRoutes_(t)

	if out.Strategy != "browser" || out.DefaultRoute != "/expenses" {
		t.Errorf("navigation strategy/default lost: %+v", out)
	}
	if len(out.Routes) != 1 || out.Routes[0].Shell != "main" || out.Routes[0].Guard != "authed" {
		t.Errorf("blueprint join did not reach the route row: %+v", out.Routes)
	}
}

// TestMergedRoutes_NoBlueprintIsNotAnError — the documented
// backwards-compatible path: no blueprint means default shell, no guard.
func TestMergedRoutes_NoBlueprintIsNotAnError(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "expense-list")
	writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml", routedBuildfile("expense-list", "/expenses"))

	out := runMergedRoutes_(t)
	if len(out.Routes) != 1 || out.Routes[0].Shell != "" {
		t.Errorf("expected an unjoined route row, got %+v", out.Routes)
	}
}

// TestProjectDiff_ReportsMissingPlan pins the field that lets codegen run
// its plan:-presence gate without opening a single buildfile.
func TestProjectDiff_ReportsMissingPlan(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "expense-list")
	featureWithIntents(t, dir, "approvals")
	// One buildfile with a plan, one without.
	writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml", routedBuildfile("expense-list", "/expenses"))
	writeFixtureFile(t, dir, ".parlay/build/approvals/buildfile.yaml",
		"feature: approvals\nadapter: go-cli\nroutes:\n  - path: /approvals\n    component: view\n")

	cmd := testCommandWithContext(t, testContext(t))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runDiff(cmd, nil); err != nil {
		t.Fatalf("project diff: %v", err)
	}
	var out projectDiffOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("decode project diff: %v", err)
	}

	if len(out.MissingPlan) != 1 || out.MissingPlan[0] != "approvals" {
		t.Errorf("missing_plan = %v, want [approvals]", out.MissingPlan)
	}
}

// TestProjectDiff_UnbuiltFeatureIsNotMissingPlan — a feature with no
// buildfile is unbuilt, which has_buildfile already says; calling it
// "missing plan" would name the wrong repair.
func TestProjectDiff_UnbuiltFeatureIsNotMissingPlan(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "expense-list")

	cmd := testCommandWithContext(t, testContext(t))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runDiff(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var out projectDiffOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.MissingPlan) != 0 {
		t.Errorf("an unbuilt feature must not appear in missing_plan, got %v", out.MissingPlan)
	}
}

// TestBuildfileDeclaresPlan_MultiTargetShape pins the v2 fallback: plan rows
// nested under plan.targets.<kind> are a declared plan, and a reader that
// only checked the top-level lists would report the wrong answer.
func TestBuildfileDeclaresPlan_MultiTargetShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "buildfile.yaml")
	if err := os.WriteFile(path, []byte(`feature: x
adapter-set: default
plan:
  targets:
    presentation:
      creates:
        - path: apps/web/list.tsx
          sources: ["component:list"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !buildfileDeclaresPlan(path) {
		t.Error("per-target plan rows must count as a declared plan")
	}

	empty := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(empty, []byte("feature: x\nplan:\n  creates: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if buildfileDeclaresPlan(empty) {
		t.Error("an empty plan: is not a declared plan")
	}
}
