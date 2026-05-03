package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBuildfileDeep_ValidBuildfile(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: go-cli
models:
  Cluster:
    properties:
      name:
        type: string
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

	errors := ValidateBuildfileDeep(path, "")
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}
}

func TestValidateBuildfileDeep_InvalidModelRef(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: go-cli
models:
  Cluster:
    properties:
      name:
        type: string
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

	errors := ValidateBuildfileDeep(path, "")
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
models: {}
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

	errors := ValidateBuildfileDeep(path, "")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
}

func TestValidateBuildfileDeep_InvalidFixtureModel(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: go-cli
models:
  Cluster:
    properties:
      name:
        type: string
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

	errors := ValidateBuildfileDeep(path, "")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
}

func TestValidateBuildfileDeep_InvalidChildRef(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: go-cli
models: {}
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

	errors := ValidateBuildfileDeep(path, "")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
}

func TestValidateBuildfileDeep_AdapterVocabulary(t *testing.T) {
	dir := t.TempDir()

	buildfile := `feature: test-feature
adapter: test-adapter
models: {}
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

	errors := ValidateBuildfileDeep(bfPath, adPath)
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

	buildfile := `feature: test-feature
adapter: go-cli
models: {}
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

	errors := ValidateBuildfileDeep(path, "")
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
models: {}
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
models: {}
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
models: {}
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
	found := false
	for _, e := range errors {
		if e.Code == "cross-cutting-target-not-in-plan" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cross-cutting-target-not-in-plan error, got: %+v", errors)
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
models: {}
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
models: {}
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
models: {}
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
		if e.Code == "missing-plan" || e.Code == "component-not-in-plan" || e.Code == "cross-cutting-not-in-plan" || e.Code == "cross-cutting-target-not-in-plan" {
			t.Errorf("plan validation incorrectly errored on a valid plan: %+v", e)
		}
	}
}
