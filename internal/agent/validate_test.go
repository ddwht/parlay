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
    type: data-display
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
    type: unknown-type
`
	adapter := `name: test-adapter
framework: Test
component-types:
  data-display:
    widget: div
  interactive-wizard:
    widget: form
`
	bfPath := filepath.Join(dir, "buildfile.yaml")
	adPath := filepath.Join(dir, "test-adapter.adapter.yaml")
	os.WriteFile(bfPath, []byte(buildfile), 0644)
	os.WriteFile(adPath, []byte(adapter), 0644)

	errors := ValidateBuildfileDeep(bfPath, adPath)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
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
