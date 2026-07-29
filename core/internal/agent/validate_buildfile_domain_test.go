package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuildfileModels_ResolvesFromDomainModel guards the contradiction where
// buildfile.schema.md and the build-feature skill both stated that a
// non-empty top-level models: is deprecated and that entities belong in
// domain-model.yaml — while the deep validator resolved model references
// ONLY against models:, so a buildfile written the documented way failed
// with missing-model-reference whose fix text prescribed the opposite.
func TestBuildfileModels_ResolvesFromDomainModel(t *testing.T) {
	root := t.TempDir()
	bfDir := filepath.Join(root, ".parlay", "build", "submit-expense")
	if err := os.MkdirAll(bfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "domain-model.yaml"), []byte(
		"schema_version: 1\nentities:\n  - name: ExpenseReport\n  - name: LineItem\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The documented shape: no models: block at all.
	bfPath := filepath.Join(bfDir, "buildfile.yaml")
	if err := os.WriteFile(bfPath, []byte(`feature: submit-expense
schema_version: 1
adapter: angular-clarity

components:
  wizard-step:
    widget: ClrInput
    data:
      inputs:
        - name: report
          model: ExpenseReport

fixtures:
  a-fixture:
    data:
      LineItem: []
`), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, e := range ValidateBuildfileDeepStructured(bfPath, "") {
		if e.Code == "missing-model-reference" || e.Code == "missing-fixture-model" {
			t.Errorf("%s fired for an entity declared in domain-model.yaml: %s", e.Code, e.Message)
		}
	}
}

// TestBuildfileModels_DeprecationWarns asserts the documented code actually
// fires now — as a warning, so the buildfiles that exist today (all of
// which carry models:, because the validator used to require it) stay
// valid rather than failing en masse.
func TestBuildfileModels_DeprecationWarns(t *testing.T) {
	root := t.TempDir()
	bfDir := filepath.Join(root, ".parlay", "build", "f")
	if err := os.MkdirAll(bfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "domain-model.yaml"), []byte(
		"schema_version: 1\nentities:\n  - name: Widget\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bfPath := filepath.Join(bfDir, "buildfile.yaml")
	if err := os.WriteFile(bfPath, []byte(`feature: f
schema_version: 1
adapter: a

models:
  Widget:
    properties: {}

components: {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var found *ValidationError
	for i, e := range ValidateBuildfileDeepStructured(bfPath, "") {
		if e.Code == "buildfile-models-deprecated" {
			found = &ValidateBuildfileDeepStructured(bfPath, "")[i]
			break
		}
	}
	if found == nil {
		t.Fatal("buildfile-models-deprecated did not fire for a non-empty models: block")
	}
	if found.Severity != string(SeverityWarning) {
		t.Errorf("severity = %q, want warning — blocking would fail every existing buildfile at once", found.Severity)
	}
}

// TestProjectRootFromBuildfilePath covers both layouts.
func TestProjectRootFromBuildfilePath(t *testing.T) {
	cases := map[string]string{
		"/p/.parlay/build/feat/buildfile.yaml":      "/p",
		"/p/.parlay/build/init/feat/buildfile.yaml": "/p",
		"/a/b/c/.parlay/build/feat/buildfile.yaml":  "/a/b/c",
	}
	for in, want := range cases {
		if got := projectRootFromBuildfilePath(in); got != want {
			t.Errorf("projectRootFromBuildfilePath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := projectRootFromBuildfilePath("/no/parlay/here.yaml"); got != "" {
		t.Errorf("expected empty root for a path with no .parlay component, got %q", got)
	}
}
