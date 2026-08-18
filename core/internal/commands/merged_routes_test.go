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

// ---------------------------------------------------------------------
// The cross-cutting index — the guard that keeps a scoped read-set from
// silently dropping another feature's merge.
// ---------------------------------------------------------------------

func crossCuttingBuildfile(feature string, entries string) string {
	return "feature: " + feature + "\nadapter: go-cli\n" +
		"plan:\n  creates:\n    - path: src/" + feature + ".go\n      sources: [\"component:view\"]\n" +
		"cross-cutting:\n" + entries
}

func runCrossCuttingIndex_(t *testing.T, targets ...string) crossCuttingIndexOutput {
	t.Helper()
	resetFlagsAfterTest(t, crossCuttingIndexCmd.Flags())
	// StringArrayVar APPENDS on Parse, so a value left by an earlier test
	// would accumulate here and silently widen (or, when it survives into an
	// unfiltered call, narrow) the filter. Clear it explicitly.
	crossCuttingIndexTargets = nil
	var flags []string
	for _, tg := range targets {
		flags = append(flags, "--target", tg)
	}
	if err := crossCuttingIndexCmd.Flags().Parse(flags); err != nil {
		t.Fatal(err)
	}
	cmd := testCommandWithContext(t, testContext(t))
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runCrossCuttingIndex(cmd, nil); err != nil {
		t.Fatalf("cross-cutting-index: %v", err)
	}
	var out crossCuttingIndexOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("decode index %q: %v", buf.String(), err)
	}
	return out
}

// TestCrossCuttingIndex_FindsWhoTargetsARegeneratedFile is the scenario the
// index exists for: feature A merged into a shared file, feature B's change
// regenerates it, and A's merge must be re-applied even though this run never
// loaded A's buildfile.
func TestCrossCuttingIndex_FindsWhoTargetsARegeneratedFile(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "middleware-feature")
	featureWithIntents(t, dir, "unrelated")
	writeFixtureFile(t, dir, ".parlay/build/middleware-feature/buildfile.yaml",
		crossCuttingBuildfile("middleware-feature",
			"  - id: auth-middleware\n    source: \"@middleware-feature/protect\"\n    target-files: [\"src/entrypoint.go\"]\n    transform: \"wrap the router in auth\"\n"))
	writeFixtureFile(t, dir, ".parlay/build/unrelated/buildfile.yaml",
		crossCuttingBuildfile("unrelated",
			"  - id: log-setup\n    source: \"@unrelated/log\"\n    target-files: [\"src/logging.go\"]\n    transform: \"add a logger\"\n"))

	out := runCrossCuttingIndex_(t, "src/entrypoint.go")

	if len(out.Entries) != 1 || out.Entries[0].ID != "auth-middleware" {
		t.Fatalf("expected exactly the entry targeting the regenerated file, got %+v", out.Entries)
	}
	if out.Entries[0].Feature != "middleware-feature" {
		t.Errorf("entry must name the feature to widen to, got %q", out.Entries[0].Feature)
	}
	if out.Entries[0].Buildfile == "" {
		t.Error("entry must name the buildfile holding its transform prose")
	}
	if ids := out.ByTarget["src/entrypoint.go"]; len(ids) != 1 || ids[0] != "auth-middleware" {
		t.Errorf("by_target lookup = %v", ids)
	}
	if _, leaked := out.ByTarget["src/logging.go"]; leaked {
		t.Error("by_target must be filtered to the asked-about paths")
	}
}

// TestCrossCuttingIndex_CarriesNoTransformProse pins the index's whole
// economy: 238 KB of transform prose across the dogfood root is exactly what
// it must not emit.
func TestCrossCuttingIndex_CarriesNoTransformProse(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "verbose")
	writeFixtureFile(t, dir, ".parlay/build/verbose/buildfile.yaml",
		crossCuttingBuildfile("verbose",
			"  - id: big\n    source: \"@verbose/x\"\n    target-files: [\"src/a.go\"]\n    transform: \"UNIQUE_PROSE_MARKER a very long description of the merge\"\n"))

	out := runCrossCuttingIndex_(t)
	blob, _ := json.Marshal(out)
	if strings.Contains(string(blob), "UNIQUE_PROSE_MARKER") {
		t.Error("the index must carry identity and targets only — transform prose is what it exists to avoid loading")
	}
	if len(out.Entries) != 1 {
		t.Fatalf("unfiltered index must list every entry, got %d", len(out.Entries))
	}
}

// TestCrossCuttingIndex_ResolvesTargetPatternAgainstTheFile pins the filter's
// precision. A target-pattern selects files by CONTENT, so leaving every
// pattern-carrying entry in the result made the filter useless — it returned
// a haystack for a question with one answer.
func TestCrossCuttingIndex_ResolvesTargetPatternAgainstTheFile(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "matcher")
	featureWithIntents(t, dir, "nonmatcher")
	writeFixtureFile(t, dir, "src/entrypoint.go", "package main\n\nfunc register() { rootCmd.AddCommand(x) }\n")
	writeFixtureFile(t, dir, ".parlay/build/matcher/buildfile.yaml",
		crossCuttingBuildfile("matcher",
			"  - id: hits\n    source: \"@matcher/x\"\n    target-pattern: \"rootCmd\\\\.AddCommand\\\\(\"\n    transform: \"register\"\n"))
	writeFixtureFile(t, dir, ".parlay/build/nonmatcher/buildfile.yaml",
		crossCuttingBuildfile("nonmatcher",
			"  - id: misses\n    source: \"@nonmatcher/x\"\n    target-pattern: \"NoSuchSymbolAnywhere\"\n    transform: \"nothing\"\n"))

	out := runCrossCuttingIndex_(t, "src/entrypoint.go")

	var ids []string
	for _, e := range out.Entries {
		ids = append(ids, e.ID)
	}
	if len(ids) != 1 || ids[0] != "hits" {
		t.Errorf("pattern must be resolved against the file's content; got %v", ids)
	}
}

// TestCrossCuttingIndex_UnreadableTargetKeepsPatternEntries — a file this run
// is about to CREATE is not on disk yet, so a pattern cannot be ruled out.
// Erring toward reporting costs a read; erring the other way drops a merge.
func TestCrossCuttingIndex_UnreadableTargetKeepsPatternEntries(t *testing.T) {
	dir := setupTestDir(t)
	featureWithIntents(t, dir, "matcher")
	writeFixtureFile(t, dir, ".parlay/build/matcher/buildfile.yaml",
		crossCuttingBuildfile("matcher",
			"  - id: hits\n    source: \"@matcher/x\"\n    target-pattern: \"anything\"\n    transform: \"t\"\n"))

	out := runCrossCuttingIndex_(t, "src/not-created-yet.go")
	if len(out.Entries) != 1 {
		t.Errorf("a pattern entry must survive when the target cannot be read yet, got %+v", out.Entries)
	}
}
