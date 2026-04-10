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
`
	path := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(path, []byte(buildfile), 0644)

	errors := ValidateBuildfileDeep(path, "")
	// Should catch: missing route component, invalid model ref, invalid child, invalid fixture model
	if len(errors) != 4 {
		t.Errorf("expected 4 errors, got %d: %v", len(errors), errors)
	}
}
