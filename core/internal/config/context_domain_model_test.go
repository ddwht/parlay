// parlay-feature: studio-support/domain-model-yaml-migration
// parlay-component: extract-domain-model-output
// parlay-artifact: test
//
// Tests for (*Context).DomainModelPath() and (*Context).LoadDomainModel().
// These methods are the single enforcement point for the read-path
// precedence rule:
//
//   - one canonical domain-model.yaml per active root (never per-feature)
//   - YAML is the only live source; legacy .md is never consulted

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDomainModelPath_AtActiveRoot(t *testing.T) {
	dir := t.TempDir()
	c := &Context{Root: Root{Path: dir, Kind: RootKindStandalone}}

	got := c.DomainModelPath()
	want := filepath.Join(dir, "domain-model.yaml")
	if got != want {
		t.Errorf("DomainModelPath = %q, want %q", got, want)
	}
}

func TestDomainModelPath_NilContext(t *testing.T) {
	var c *Context
	if got := c.DomainModelPath(); got != "" {
		t.Errorf("nil Context should return empty path, got %q", got)
	}
}

func TestDomainModelPath_ChildRootIsIndependent(t *testing.T) {
	// A child root's domain-model.yaml lives at the child's path, NOT
	// the parent's. One child's model never bleeds into another's.
	parent := t.TempDir()
	childA := filepath.Join(parent, "child-a")
	childB := filepath.Join(parent, "child-b")
	for _, p := range []string{childA, childB} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}

	cA := &Context{Root: Root{Path: childA, Kind: RootKindChild, ParentPath: parent}}
	cB := &Context{Root: Root{Path: childB, Kind: RootKindChild, ParentPath: parent}}

	pathA := cA.DomainModelPath()
	pathB := cB.DomainModelPath()

	if pathA == pathB {
		t.Errorf("two children should have distinct domain-model paths; both = %q", pathA)
	}
	if filepath.Dir(pathA) != childA {
		t.Errorf("child-a's model should live under %q, got %q", childA, pathA)
	}
	if filepath.Dir(pathB) != childB {
		t.Errorf("child-b's model should live under %q, got %q", childB, pathB)
	}
}

func TestLoadDomainModel_MissingFile(t *testing.T) {
	dir := t.TempDir()
	c := &Context{Root: Root{Path: dir, Kind: RootKindStandalone}}

	_, err := c.LoadDomainModel()
	if !errors.Is(err, ErrNoDomainModel) {
		t.Errorf("expected ErrNoDomainModel for missing file, got %v", err)
	}
}

func TestLoadDomainModel_LegacyMdNotConsulted(t *testing.T) {
	// The read-path precedence rule: a project with only .md (no .yaml)
	// is treated as having no domain model. The .md is never parsed,
	// never merged, never consulted as a fallback.
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "domain-model.md")
	if err := os.WriteFile(mdPath, []byte("# Domain Model\n\nOrders have items.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Context{Root: Root{Path: dir, Kind: RootKindStandalone}}
	_, err := c.LoadDomainModel()
	if !errors.Is(err, ErrNoDomainModel) {
		t.Errorf("legacy .md must NOT be consulted as a fallback; expected ErrNoDomainModel, got %v", err)
	}
}

func TestLoadDomainModel_YamlOnly(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "domain-model.yaml")
	yaml := `schema_version: 1
enums: []
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
relationships: []
operations: []
`
	if err := os.WriteFile(yamlPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Context{Root: Root{Path: dir, Kind: RootKindStandalone}}
	a, err := c.LoadDomainModel()
	if err != nil {
		t.Fatalf("LoadDomainModel: %v", err)
	}
	if a.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", a.SchemaVersion)
	}
	if len(a.Entities) != 1 || a.Entities[0].Name != "Order" {
		t.Errorf("entities not parsed: %+v", a.Entities)
	}
}

func TestLoadDomainModel_YamlWinsOverMd(t *testing.T) {
	// When BOTH files exist, the YAML is the sole live source.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "domain-model.md"),
		[]byte("# Stale\n\nThis should be ignored.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "domain-model.yaml"),
		[]byte("schema_version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Context{Root: Root{Path: dir, Kind: RootKindStandalone}}
	a, err := c.LoadDomainModel()
	if err != nil {
		t.Fatalf("LoadDomainModel: %v", err)
	}
	if a.SchemaVersion != 1 {
		t.Errorf("YAML should win; got schema_version %d", a.SchemaVersion)
	}
}

func TestLoadDomainModel_PerProcessCache(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "domain-model.yaml")
	if err := os.WriteFile(yamlPath, []byte("schema_version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c := &Context{Root: Root{Path: dir, Kind: RootKindStandalone}}

	first, err := c.LoadDomainModel()
	if err != nil {
		t.Fatalf("first LoadDomainModel: %v", err)
	}
	// Mutate the file on disk; cached read should return the original.
	if err := os.WriteFile(yamlPath, []byte("schema_version: 99\n"), 0644); err != nil {
		t.Fatal(err)
	}

	second, err := c.LoadDomainModel()
	if err != nil {
		t.Fatalf("second LoadDomainModel: %v", err)
	}
	if first != second {
		t.Errorf("expected cached pointer to match across calls; first=%p second=%p", first, second)
	}
	if second.SchemaVersion != 1 {
		t.Errorf("cache should have preserved schema_version 1, got %d", second.SchemaVersion)
	}
}

func TestLoadDomainModel_BadYamlSurfacesError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "domain-model.yaml"),
		[]byte("schema_version: 1\nentities: [\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c := &Context{Root: Root{Path: dir, Kind: RootKindStandalone}}
	_, err := c.LoadDomainModel()
	if err == nil {
		t.Fatal("expected parse error for malformed YAML")
	}
	if errors.Is(err, ErrNoDomainModel) {
		t.Errorf("malformed YAML should NOT collapse to ErrNoDomainModel; got %v", err)
	}
}
