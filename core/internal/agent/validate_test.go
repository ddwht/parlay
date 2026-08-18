package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deepBuildfileMessages flattens the structured findings to their messages,
// which is what these tests assert on.
//
// This mapping used to be a production function, ValidateBuildfileDeep, with
// seven callers — all of them in this file and none in the tool. So the tests
// reported confidence about a wrapper the CLI never executed, while the path it
// does execute (ValidateBuildfileDeepStructured) was covered only indirectly.
// The mapping belongs here: the tests want strings, the tool wants findings.
func deepBuildfileMessages(buildfilePath, adapterPath string) []string {
	var msgs []string
	for _, e := range ValidateBuildfileDeepStructured(buildfilePath, adapterPath) {
		msgs = append(msgs, e.Message)
	}
	return msgs
}

func TestValidateBuildfileDeep_ValidBuildfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "domain-model.yaml"), []byte("schema_version: 1\nentities:\n  - name: Cluster\n    fields:\n      - name: name\n        type: string\n        required: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bfDir := filepath.Join(dir, ".parlay", "build", "test-feature")
	if err := os.MkdirAll(bfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dir = bfDir

	buildfile := `feature: test-feature
adapter: go-cli
fixtures:
  default:
    data:
      Cluster:
        - name: "prod-1"
routes:
  - path: main
    regions:
      main:
        components: [cluster-view]
components:
  cluster-view:
    source: "@test-feature/cluster-list"
    data:
      inputs:
        - model: Cluster
          field: name
plan:
  creates:
    - path: cmd/cluster_view.go
      sources: [component/cluster-view]
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := deepBuildfileMessages(path, "")
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}
}

func TestValidateBuildfileDeep_InvalidModelRef(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: go-cli
components:
  cluster-view:
    source: "@test-feature/cluster-list"
    data:
      inputs:
        - model: NonExistentModel
          field: name
plan:
  creates:
    - path: cmd/cluster_view.go
      sources: [component/cluster-view]
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := deepBuildfileMessages(path, "")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
	if errors[0] != `component "cluster-view" references model "NonExistentModel" which is not defined` {
		t.Errorf("unexpected error: %s", errors[0])
	}
}

func TestValidateBuildfileDeep_InvalidRouteComponent(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: go-cli
routes:
  - path: main
    regions:
      main:
        components: [missing-component]
components:
  real-component:
    source: "@test-feature/something"
plan:
  creates:
    - path: cmd/real_component.go
      sources: [component/real-component]
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := deepBuildfileMessages(path, "")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
}

func TestValidateBuildfileDeep_InvalidFixtureModel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "domain-model.yaml"), []byte("schema_version: 1\nentities:\n  - name: Cluster\n    fields:\n      - name: name\n        type: string\n        required: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bfDir := filepath.Join(dir, ".parlay", "build", "test-feature")
	if err := os.MkdirAll(bfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dir = bfDir

	buildfile := `feature: test-feature
adapter: go-cli
fixtures:
  default:
    data:
      NonExistentModel:
        - name: "test"
components: {}
plan:
  creates: []
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := deepBuildfileMessages(path, "")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
}

func TestValidateBuildfileDeep_InvalidChildRef(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: go-cli
components:
  parent:
    source: "@test-feature/parent"
    children:
      - ghost-child
plan:
  creates:
    - path: cmd/parent.go
      sources: [component/parent]
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := deepBuildfileMessages(path, "")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
}

func TestValidateBuildfileDeep_AdapterVocabulary(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: test-adapter
components:
  my-comp:
    source: "@test-feature/frag"
    widget: unknown-widget
plan:
  creates:
    - path: cmd/my_comp.go
      sources: [component/my-comp]
`
	adapter := `name: test-adapter
framework: Test
shows:
  data-value:
    widget: span
  data-list:
    widget: ul
actions:
  invoke:
    widget: button
flows:
  guided-flow:
    pattern: form-wizard
`
	bfPath := filepath.Join(dir, "buildfile.yaml")
	adPath := filepath.Join(dir, "test-adapter.adapter.yaml")
	os.WriteFile(bfPath, []byte(buildfile), 0644)
	os.WriteFile(adPath, []byte(adapter), 0644)

	errors := deepBuildfileMessages(bfPath, adPath)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error for unknown-widget, got %d: %v", len(errors), errors)
	}
}

func TestValidateBlueprint_Valid(t *testing.T) {
	blueprint := `app: my-app

shells:
  main:
    description: Main app shell with sidebar
    chrome:
      - region: sidebar
        widget: Sider
        content: primary navigation
    wraps: [dashboard, tasks, settings]
  auth:
    description: Centered auth layout
    chrome: []
    wraps: [login, register]

navigation:
  strategy: browser
  default-route: /dashboard
  routes:
    - path: /dashboard
      shell: main
      guard: require-auth
    - path: /login
      shell: auth
      guard: none

authorization:
  strategy: role-based
  guards:
    require-auth:
      requires: user
      redirect: /login
`
	err := ValidateBlueprint("test.yaml", []byte(blueprint))
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateBlueprint_InvalidStrategy(t *testing.T) {
	blueprint := `app: my-app
navigation:
  strategy: invalid-thing
`
	err := ValidateBlueprint("test.yaml", []byte(blueprint))
	if err == nil {
		t.Error("expected error for invalid strategy")
	}
}

func TestValidateBlueprint_MissingShellRef(t *testing.T) {
	blueprint := `app: my-app
shells:
  main:
    description: Main shell
    wraps: all
navigation:
  strategy: browser
  routes:
    - path: /dashboard
      shell: nonexistent
`
	err := ValidateBlueprint("test.yaml", []byte(blueprint))
	if err == nil {
		t.Error("expected error for missing shell reference")
	}
}

func TestValidateBlueprint_MissingGuardRef(t *testing.T) {
	blueprint := `app: my-app
navigation:
  strategy: browser
  routes:
    - path: /dashboard
      guard: require-auth
`
	err := ValidateBlueprint("test.yaml", []byte(blueprint))
	if err == nil {
		t.Error("expected error for missing guard reference")
	}
}

func TestValidateBlueprint_DuplicateRoutes(t *testing.T) {
	blueprint := `app: my-app
navigation:
  strategy: browser
  routes:
    - path: /dashboard
    - path: /dashboard
`
	err := ValidateBlueprint("test.yaml", []byte(blueprint))
	if err == nil {
		t.Error("expected error for duplicate route paths")
	}
}

func TestValidateBlueprint_Minimal(t *testing.T) {
	blueprint := `app: ""
navigation:
  strategy: cli-subcommands
`
	err := ValidateBlueprint("test.yaml", []byte(blueprint))
	if err != nil {
		t.Errorf("expected no error for minimal blueprint, got: %v", err)
	}
}

func TestValidateBuildfileDeep_MultipleErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "domain-model.yaml"), []byte("schema_version: 1\nentities:\n  - name: Cluster\n    fields:\n      - name: name\n        type: string\n        required: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bfDir := filepath.Join(dir, ".parlay", "build", "test-feature")
	if err := os.MkdirAll(bfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dir = bfDir

	buildfile := `feature: test-feature
adapter: go-cli
fixtures:
  default:
    data:
      Ghost:
        - name: test
routes:
  - path: main
    regions:
      main:
        components: [missing]
components:
  real:
    source: "@test-feature/frag"
    data:
      inputs:
        - model: AlsoGhost
    children:
      - phantom
plan:
  creates:
    - path: cmd/real.go
      sources: [component/real]
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := deepBuildfileMessages(path, "")
	// Should catch: missing route component, invalid model ref, invalid child, invalid fixture model
	if len(errors) != 4 {
		t.Errorf("expected 4 errors, got %d: %v", len(errors), errors)
	}
}

// --- plan: section validation tests ---

func TestValidatePlan_MissingPlanFails(t *testing.T) {
	dir := t.TempDir()
	buildfile := `feature: test-feature
adapter: go-cli
components: {}
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := ValidateBuildfileDeepStructured(path, "")
	if len(errors) != 1 || errors[0].Code != "missing-plan" {
		t.Fatalf("expected single missing-plan error, got: %+v", errors)
	}
}

func TestValidatePlan_ComponentNotInPlan(t *testing.T) {
	dir := t.TempDir()
	buildfile := `feature: test-feature
adapter: go-cli
components:
  orphan-comp:
    source: "@test-feature/frag"
plan:
  creates: []
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := ValidateBuildfileDeepStructured(path, "")
	found := false
	for _, e := range errors {
		if e.Code == "component-not-in-plan" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected component-not-in-plan error, got: %+v", errors)
	}
}

func TestValidatePlan_CrossCuttingTargetNotInPlan(t *testing.T) {
	dir := t.TempDir()
	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: my-cc
    source: "@test-feature/intent"
    target-files:
      - some/existing/file.go
    transform: "do something"
plan:
  modifies: []
  creates: []
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := ValidateBuildfileDeepStructured(path, "")
	// Under the kind-aware routing introduced by
	// parlay-tool/cross-cutting-target-paths, the legacy
	// "cross-cutting-target-not-in-plan" code is replaced by per-kind
	// codes. With no rootDir (buildfile outside .parlay/build/), the
	// classifier defaults to modifies-only; the per-target error is
	// "cross-cutting-target-not-in-modifies" plus the entry-level
	// "cross-cutting-not-in-plan" since no plan rows are sourced.
	wantCodes := map[string]bool{
		"cross-cutting-target-not-in-modifies": false,
		"cross-cutting-not-in-plan":            false,
	}
	for _, e := range errors {
		if _, ok := wantCodes[e.Code]; ok {
			wantCodes[e.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Fatalf("expected error code %q, got: %+v", code, errors)
		}
	}
}

func TestValidatePlan_DiskShapeChecks(t *testing.T) {
	// Set up a layout matching the real <root>/.parlay/build/<feature>/buildfile.yaml shape.
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)

	// One file exists in the source root, one doesn't.
	srcDir := filepath.Join(root, "internal", "config")
	os.MkdirAll(srcDir, 0755)
	existing := filepath.Join(srcDir, "config.go")
	os.WriteFile(existing, []byte("package config\n"), 0644)

	buildfile := `feature: test-feature
adapter: go-cli
components:
  comp-a:
    source: "@test-feature/frag"
cross-cutting:
  - id: cc-real
    source: "@test-feature/intent"
    target-files:
      - internal/config/config.go
    transform: "edit existing config"
  - id: cc-missing
    source: "@test-feature/intent"
    target-files:
      - internal/missing/file.go
    transform: "edit nonexistent file"
plan:
  modifies:
    - path: internal/config/config.go
      sources: [cross-cutting/cc-real]
    - path: internal/missing/file.go
      sources: [cross-cutting/cc-missing]
  creates:
    - path: internal/config/config.go
      sources: [component/comp-a]
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := ValidateBuildfileDeepStructured(path, "")
	wantCodes := map[string]bool{
		"plan-modify-target-missing": false,
		"plan-create-collision":      false,
	}
	for _, e := range errors {
		if _, ok := wantCodes[e.Code]; ok {
			wantCodes[e.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Errorf("expected error code %q, did not see it. all errors: %+v", code, errors)
		}
	}
}

func TestValidatePlan_DiskShapeChecks_QualifiedSlug(t *testing.T) {
	// Regression: initiative-nested features have qualified slugs like
	// <root>/.parlay/build/<initiative>/<feature>/buildfile.yaml. The root
	// resolver must land on <root>, not on <root>/.parlay, so existing
	// plan targets are recognized as existing.
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "my-initiative", "test-feature")
	os.MkdirAll(buildDir, 0755)

	srcDir := filepath.Join(root, "internal", "config")
	os.MkdirAll(srcDir, 0755)
	existing := filepath.Join(srcDir, "config.go")
	os.WriteFile(existing, []byte("package config\n"), 0644)

	buildfile := `feature: my-initiative/test-feature
adapter: go-cli
cross-cutting:
  - id: cc-real
    source: "@my-initiative/test-feature/intent"
    target-files:
      - internal/config/config.go
    transform: "edit existing config"
plan:
  modifies:
    - path: internal/config/config.go
      sources: [cross-cutting/cc-real]
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := ValidateBuildfileDeepStructured(path, "")
	for _, e := range errors {
		if e.Code == "plan-modify-target-missing" {
			t.Errorf("qualified-slug buildfile spuriously reports plan-modify-target-missing: %+v", e)
		}
	}
}

func TestValidatePlan_HappyPath(t *testing.T) {
	dir := t.TempDir()
	buildfile := `feature: test-feature
adapter: go-cli
components:
  my-comp:
    source: "@test-feature/frag"
cross-cutting:
  - id: my-cc
    source: "@test-feature/intent"
    target-files:
      - some/file.go
    transform: "do something"
plan:
  modifies:
    - path: some/file.go
      sources: [cross-cutting/my-cc]
  creates:
    - path: cmd/my_comp.go
      sources: [component/my-comp]
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := ValidateBuildfileDeepStructured(path, "")
	for _, e := range errors {
		switch e.Code {
		case "missing-plan",
			"component-not-in-plan",
			"cross-cutting-not-in-plan",
			"cross-cutting-target-not-in-plan",
			"cross-cutting-target-not-in-modifies",
			"cross-cutting-target-not-in-creates",
			"cross-cutting-mixed-target-kinds",
			"cross-cutting-target-creates-not-in-plan",
			"cross-cutting-target-double-listed",
			"cross-cutting-pattern-empty":
			t.Errorf("plan validation incorrectly errored on a valid plan: %+v", e)
		}
	}
}

// --- kind-aware cross-cutting routing tests ---
//
// parlay-feature: parlay-tool/cross-cutting-target-paths
// parlay-component: validate (extends validator-classify-entry-kind-and-route)
// parlay-component: validate (extends validator-resolve-target-pattern-at-validation-time)
// parlay-component: validate (extends validator-target-creates-and-two-kinded-entries)
// parlay-component: validate (extends project-pass-validation-and-cli-flag)

func TestValidatePlan_CrossCutting_PurelyIntroducing_RoutesToCreates(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)

	// Path absent on disk -> classifier returns "purely-introducing".
	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: introduce-pkg
    source: "@test-feature/intent"
    target-files:
      - internal/newpkg/file.go
    transform: "introduce a new package"
plan:
  modifies: []
  creates:
    - path: internal/newpkg/file.go
      sources: [cross-cutting/introduce-pkg]
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	for _, e := range errors {
		switch e.Code {
		case "cross-cutting-target-not-in-creates",
			"cross-cutting-target-not-in-modifies",
			"cross-cutting-mixed-target-kinds":
			t.Errorf("purely-introducing entry incorrectly errored: %+v", e)
		}
	}
}

func TestValidatePlan_CrossCutting_PurelyIntroducing_MissingFromCreates(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)

	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: introduce-pkg
    source: "@test-feature/intent"
    target-files:
      - internal/newpkg/file.go
    transform: "introduce a new package"
plan:
  modifies:
    - path: internal/newpkg/file.go
      sources: [cross-cutting/introduce-pkg]
  creates: []
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	found := false
	for _, e := range errors {
		if e.Code == "cross-cutting-target-not-in-creates" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cross-cutting-target-not-in-creates, got: %+v", errors)
	}
}

func TestValidatePlan_CrossCutting_ModifiesOnly_NoRegression(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)
	srcDir := filepath.Join(root, "internal", "config")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "config.go"), []byte("package config\n"), 0644)

	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: edit-config
    source: "@test-feature/intent"
    target-files:
      - internal/config/config.go
    transform: "extend config"
plan:
  modifies:
    - path: internal/config/config.go
      sources: [cross-cutting/edit-config]
  creates: []
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	for _, e := range errors {
		switch e.Code {
		case "cross-cutting-target-not-in-creates",
			"cross-cutting-target-not-in-modifies",
			"cross-cutting-mixed-target-kinds":
			t.Errorf("modifies-only entry incorrectly errored: %+v", e)
		}
	}
}

func TestValidatePlan_CrossCutting_MixedKinds_FailsLoudly(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)
	srcDir := filepath.Join(root, "internal", "exists")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "old.go"), []byte("package exists\n"), 0644)

	// Two paths in target-files: one exists on disk, one doesn't.
	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: split-me
    source: "@test-feature/intent"
    target-files:
      - internal/exists/old.go
      - internal/missing/new.go
    transform: "ambiguously do something"
plan:
  modifies:
    - path: internal/exists/old.go
      sources: [cross-cutting/split-me]
    - path: internal/missing/new.go
      sources: [cross-cutting/split-me]
  creates: []
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	found := false
	for _, e := range errors {
		if e.Code == "cross-cutting-mixed-target-kinds" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cross-cutting-mixed-target-kinds, got: %+v", errors)
	}
}

func TestValidatePlan_CrossCutting_TwoKinded_HappyPath(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)
	srcDir := filepath.Join(root, "internal", "config")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "config.go"), []byte("package config\n"), 0644)

	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: extend-and-add
    source: "@test-feature/intent"
    target-files:
      - internal/config/config.go
    target-creates:
      - internal/newhelper/helper.go
    transform: "extend config and add a helper"
plan:
  modifies:
    - path: internal/config/config.go
      sources: [cross-cutting/extend-and-add]
  creates:
    - path: internal/newhelper/helper.go
      sources: [cross-cutting/extend-and-add]
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	for _, e := range errors {
		switch e.Code {
		case "cross-cutting-target-not-in-creates",
			"cross-cutting-target-not-in-modifies",
			"cross-cutting-target-creates-not-in-plan",
			"cross-cutting-mixed-target-kinds",
			"cross-cutting-target-double-listed":
			t.Errorf("two-kinded entry incorrectly errored: %+v", e)
		}
	}
}

func TestValidatePlan_CrossCutting_TargetCreatesMissingFromPlan(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)
	srcDir := filepath.Join(root, "internal", "config")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "config.go"), []byte("package config\n"), 0644)

	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: forgot-the-create
    source: "@test-feature/intent"
    target-files:
      - internal/config/config.go
    target-creates:
      - internal/newhelper/helper.go
    transform: "forgot to add helper to plan.creates"
plan:
  modifies:
    - path: internal/config/config.go
      sources: [cross-cutting/forgot-the-create]
  creates: []
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	found := false
	for _, e := range errors {
		if e.Code == "cross-cutting-target-creates-not-in-plan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cross-cutting-target-creates-not-in-plan, got: %+v", errors)
	}
}

func TestValidatePlan_CrossCutting_DoubleListed_FailsLoudly(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)
	srcDir := filepath.Join(root, "internal", "config")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "config.go"), []byte("package config\n"), 0644)

	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: double-listed
    source: "@test-feature/intent"
    target-files:
      - internal/config/config.go
    target-creates:
      - internal/config/config.go
    transform: "same path in both"
plan:
  modifies:
    - path: internal/config/config.go
      sources: [cross-cutting/double-listed]
  creates:
    - path: internal/config/config.go
      sources: [cross-cutting/double-listed]
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	found := false
	for _, e := range errors {
		if e.Code == "cross-cutting-target-double-listed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cross-cutting-target-double-listed, got: %+v", errors)
	}
}

func TestValidatePlan_CrossCutting_TargetPattern_EmptyResolution(t *testing.T) {
	root := t.TempDir()
	buildDir := filepath.Join(root, ".parlay", "build", "test-feature")
	os.MkdirAll(buildDir, 0755)
	// No matching files anywhere under root.

	buildfile := `feature: test-feature
adapter: go-cli
components: {}
cross-cutting:
  - id: pattern-miss
    source: "@test-feature/intent"
    target-pattern: "internal/nope/*.go"
    transform: "fan out across nonexistent dir"
plan:
  modifies: []
  creates: []
`
	path := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)
	errors := ValidateBuildfileDeepStructured(path, "")
	found := false
	for _, e := range errors {
		if e.Code == "cross-cutting-pattern-empty" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cross-cutting-pattern-empty, got: %+v", errors)
	}
}

func TestResolveTargetPattern_Determinism(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"a.go", "b.go", "c.txt"} {
		os.WriteFile(filepath.Join(root, p), []byte("x"), 0644)
	}
	out1 := resolveTargetPattern("*.go", root, nil)
	out2 := resolveTargetPattern("*.go", root, nil)
	if len(out1) != 2 || out1[0] != "a.go" || out1[1] != "b.go" {
		t.Fatalf("expected sorted [a.go b.go], got %v", out1)
	}
	if len(out2) != len(out1) {
		t.Fatalf("repeated calls returned different results: %v vs %v", out1, out2)
	}
	for i := range out1 {
		if out1[i] != out2[i] {
			t.Fatalf("non-deterministic resolution: %v vs %v", out1, out2)
		}
	}
}

func TestResolveTargetPattern_RecursiveStarDoesNotMatch(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "x.go"), []byte("x"), 0644)

	// filepath.Match has no recursive ** support; pattern resolves to zero.
	out := resolveTargetPattern("**/*.go", root, nil)
	if len(out) != 0 {
		t.Fatalf("expected zero matches for recursive **, got %v", out)
	}
}

// --- project-pass tests ---
//
// parlay-feature: parlay-tool/cross-cutting-target-paths
// parlay-component: validate (extends project-pass-validation-and-cli-flag)

func TestProjectPass_TwoFeatureHappyPath(t *testing.T) {
	root := t.TempDir()
	// Feature A creates internal/foo/foo.go.
	dirA := filepath.Join(root, ".parlay", "build", "feat-a")
	os.MkdirAll(dirA, 0755)
	os.WriteFile(filepath.Join(dirA, "buildfile.yaml"), []byte(`feature: feat-a
adapter: go-cli
components: {}
cross-cutting:
  - id: create-foo
    source: "@feat-a/intent"
    target-files:
      - internal/foo/foo.go
    transform: "introduce foo"
plan:
  creates:
    - path: internal/foo/foo.go
      sources: [cross-cutting/create-foo]
`), 0644)

	// Feature B modifies internal/foo/foo.go (which doesn't exist on disk
	// yet — feat-a creates it). In single-feature mode this fails with
	// plan-modify-target-missing; in project-pass mode it passes.
	dirB := filepath.Join(root, ".parlay", "build", "feat-b")
	os.MkdirAll(dirB, 0755)
	os.WriteFile(filepath.Join(dirB, "buildfile.yaml"), []byte(`feature: feat-b
adapter: go-cli
components: {}
cross-cutting:
  - id: extend-foo
    source: "@feat-b/intent"
    target-files:
      - internal/foo/foo.go
    transform: "extend foo"
plan:
  modifies:
    - path: internal/foo/foo.go
      sources: [cross-cutting/extend-foo]
`), 0644)

	verdicts, err := ValidateBuildfilesProjectStructured(root)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(verdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d: %+v", len(verdicts), verdicts)
	}
	for _, v := range verdicts {
		for _, e := range v.Errors {
			if e.Code == "plan-modify-target-missing" {
				t.Errorf("project-pass should relax modify-existence via sibling create, but %s reported: %+v", v.Feature, e)
			}
		}
	}
}

func TestProjectPass_SingleVsProjectRegression(t *testing.T) {
	root := t.TempDir()
	dirB := filepath.Join(root, ".parlay", "build", "feat-b")
	os.MkdirAll(dirB, 0755)
	bfPath := filepath.Join(dirB, "buildfile.yaml")
	os.WriteFile(bfPath, []byte(`feature: feat-b
adapter: go-cli
components: {}
cross-cutting:
  - id: extend-foo
    source: "@feat-b/intent"
    target-files:
      - internal/foo/foo.go
    transform: "extend foo"
plan:
  modifies:
    - path: internal/foo/foo.go
      sources: [cross-cutting/extend-foo]
`), 0644)

	// Single-feature mode: file is missing -> plan-modify-target-missing fires.
	singleErrors := ValidateBuildfileDeepStructured(bfPath, "")
	foundMissing := false
	for _, e := range singleErrors {
		if e.Code == "plan-modify-target-missing" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("single-feature mode should fire plan-modify-target-missing when file is absent and no sibling promises it, got: %+v", singleErrors)
	}
}

func TestProjectPass_TwoFeatureCycleDetection(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, ".parlay", "build", "feat-a")
	dirB := filepath.Join(root, ".parlay", "build", "feat-b")
	os.MkdirAll(dirA, 0755)
	os.MkdirAll(dirB, 0755)
	// A creates foo.go, modifies bar.go. B creates bar.go, modifies foo.go.
	os.WriteFile(filepath.Join(dirA, "buildfile.yaml"), []byte(`feature: feat-a
adapter: go-cli
components: {}
cross-cutting:
  - id: a-create
    source: "@feat-a/i1"
    target-files:
      - internal/foo.go
    transform: "create foo"
  - id: a-modify
    source: "@feat-a/i2"
    target-files:
      - internal/bar.go
    transform: "modify bar"
plan:
  creates:
    - path: internal/foo.go
      sources: [cross-cutting/a-create]
  modifies:
    - path: internal/bar.go
      sources: [cross-cutting/a-modify]
`), 0644)
	os.WriteFile(filepath.Join(dirB, "buildfile.yaml"), []byte(`feature: feat-b
adapter: go-cli
components: {}
cross-cutting:
  - id: b-create
    source: "@feat-b/i1"
    target-files:
      - internal/bar.go
    transform: "create bar"
  - id: b-modify
    source: "@feat-b/i2"
    target-files:
      - internal/foo.go
    transform: "modify foo"
plan:
  creates:
    - path: internal/bar.go
      sources: [cross-cutting/b-create]
  modifies:
    - path: internal/foo.go
      sources: [cross-cutting/b-modify]
`), 0644)
	verdicts, err := ValidateBuildfilesProjectStructured(root)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	cycleCount := 0
	for _, v := range verdicts {
		for _, e := range v.Errors {
			if e.Code == "plan-create-modify-cycle" {
				cycleCount++
			}
		}
	}
	if cycleCount < 2 {
		t.Fatalf("expected at least 2 plan-create-modify-cycle errors (one per side), got %d. verdicts: %+v", cycleCount, verdicts)
	}
}

func TestProjectPass_EmptyProject(t *testing.T) {
	root := t.TempDir()
	// No .parlay/build at all.
	verdicts, err := ValidateBuildfilesProjectStructured(root)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(verdicts) != 0 {
		t.Fatalf("expected zero verdicts on empty project, got %d", len(verdicts))
	}
}

// TestValidateSurface_DispatchesOnFormat guards the defect where a
// surface.yaml was routed into the legacy markdown heading probe and always
// failed with "surface.md has no fragment headings (## )" — leaving the
// format the pipeline actually produces with no passing validation path.
func TestValidateSurface_DispatchesOnFormat(t *testing.T) {
	validYAML := []byte(`feature: expense-list

fragments:
  - name: My reports grid
    shows: data-table
    source: "@expense-list/browse"
    page: expenses
`)
	if err := ValidateSurface("spec/intents/expense-list/surface.yaml", validYAML); err != nil {
		t.Errorf("valid surface.yaml rejected: %v", err)
	}

	validMD := []byte("# F — Surface\n\n## A fragment\n\n**Shows**: data-value\n")
	if err := ValidateSurface("spec/intents/f/surface.md", validMD); err != nil {
		t.Errorf("valid surface.md rejected: %v", err)
	}

	// The YAML path must still catch real problems.
	for name, body := range map[string]string{
		"no fragments":   "feature: f\nfragments: []\n",
		"missing shows":  "feature: f\nfragments:\n  - name: A\n    source: \"@f/x\"\n    page: p\n",
		"missing source": "feature: f\nfragments:\n  - name: A\n    shows: data-value\n    page: p\n",
		"missing page":   "feature: f\nfragments:\n  - name: A\n    shows: data-value\n    source: \"@f/x\"\n",
	} {
		if err := ValidateSurface("s/surface.yaml", []byte(body)); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}

// TestValidateBlueprint_ClosedVocabularyGate guards P1-1: blueprint.schema.md
// documents data.fetching as a closed set and defines
// blueprint-strategy-unknown for out-of-vocabulary values, but nothing
// enforced it on the path the CLI uses — a typo'd or invented strategy
// validated clean and surfaced, if at all, only during codegen.
func TestValidateBlueprint_ClosedVocabularyGate(t *testing.T) {
	valid := []byte(`app: x
navigation:
  strategy: browser
data:
  fetching: on-mount
  caching:
    strategy: in-memory
`)
	if err := ValidateBlueprint("bp.yaml", valid); err != nil {
		t.Errorf("valid blueprint rejected: %v", err)
	}

	bad := []byte(`app: x
navigation:
  strategy: browser
data:
  fetching: in-memory-store
`)
	err := ValidateBlueprint("bp.yaml", bad)
	if err == nil {
		t.Fatal("out-of-vocabulary data.fetching accepted; blueprint-strategy-unknown never fires")
	}
	if !strings.Contains(err.Error(), "blueprint-strategy-unknown") {
		t.Errorf("error does not carry the documented code: %v", err)
	}
	if !strings.Contains(err.Error(), "on-mount") {
		t.Errorf("error should list the allowed values so the fix is obvious: %v", err)
	}
}

// TestValidateBlueprint_CachingVocabularyReconciled replaces
// TestValidateBlueprint_CachingVocabularyNotGated, which existed to hold the
// key open while two vocabularies competed for it: a Go set of
// {none, per-route, shared} (cache scope) against the schema's
// {none, in-memory, local-storage, service-worker} (cache location). It ended
// with "resolve the vocabulary conflict first, then gate."
//
// Resolved: the rival set is deleted, the schema's wins, and the key is gated.
// Both halves are asserted here — a schema-shaped value passes, and an invented
// one is now rejected rather than sailing through to codegen.
func TestValidateBlueprint_CachingVocabularyReconciled(t *testing.T) {
	schemaShaped := []byte(`app: x
data:
  fetching: on-mount
  caching:
    strategy: in-memory
`)
	if err := ValidateBlueprint("bp.yaml", schemaShaped); err != nil {
		t.Errorf("a blueprint authored straight from the schema table was rejected: %v", err)
	}

	// The old Go vocabulary. It must NOT be accepted now — if it is, the
	// rival set has grown back.
	oldGoVocab := []byte(`app: x
data:
  caching:
    strategy: per-route
`)
	err := ValidateBlueprint("bp.yaml", oldGoVocab)
	if err == nil {
		t.Fatal("data.caching.strategy: per-route accepted — the retired cache-scope vocabulary is back")
	}
	if !strings.Contains(err.Error(), "blueprint-strategy-unknown") {
		t.Errorf("error does not carry the documented code: %v", err)
	}
}

// TestValidateBlueprint_RetryVocabularyIsTheStrategyNotTheScope pins the other
// half of the same mistake. The retired ClosedSetErrorsRetry held
// {none, reads, writes, all} — the vocabulary of errors.retry.applies-to —
// while the key it claimed to gate, errors.retry.strategy, takes
// {none, exponential-backoff, immediate-once}. Had it ever run, it would have
// rejected every legal strategy and accepted none.
func TestValidateBlueprint_RetryVocabularyIsTheStrategyNotTheScope(t *testing.T) {
	schemaShaped := []byte(`app: x
errors:
  retry:
    strategy: exponential-backoff
    applies-to: writes
`)
	if err := ValidateBlueprint("bp.yaml", schemaShaped); err != nil {
		t.Errorf("schema-shaped retry block rejected: %v", err)
	}

	swapped := []byte(`app: x
errors:
  retry:
    strategy: writes
`)
	if err := ValidateBlueprint("bp.yaml", swapped); err == nil {
		t.Fatal("errors.retry.strategy: writes accepted — applies-to's vocabulary is gating strategy again")
	}
}

// TestValidateBlueprint_StrategyGateSurvivesACompleteBlueprint is the
// regression test for why three wrong vocabularies went unnoticed for so long.
// The retired gate decoded data.caching and errors.retry as strings; the schema
// documents both as maps. A blueprint that filled in either section failed the
// unmarshal, and an `err == nil` guard then skipped every strategy check in
// silence — including the one that was correct. Completeness must not disable
// validation.
func TestValidateBlueprint_StrategyGateSurvivesACompleteBlueprint(t *testing.T) {
	complete := []byte(`app: x
navigation:
  strategy: browser
authorization:
  strategy: role-based
data:
  fetching: telepathy
  caching:
    strategy: in-memory
  offline:
    strategy: read-only-cache
errors:
  retry:
    strategy: exponential-backoff
    applies-to: all
`)
	err := ValidateBlueprint("bp.yaml", complete)
	if err == nil {
		t.Fatal("data.fetching: telepathy accepted in a fully-populated blueprint — the strategy gate switched itself off again")
	}
	if !strings.Contains(err.Error(), "blueprint-strategy-unknown") {
		t.Errorf("wrong failure: %v", err)
	}
}
