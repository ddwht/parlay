package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSchemas_WritesAllSchemas(t *testing.T) {
	dir := t.TempDir()

	if err := WriteSchemas(dir); err != nil {
		t.Fatalf("WriteSchemas failed: %v", err)
	}

	expected := []string{
		"adapter.schema.md",
		"blueprint.schema.md",
		"buildfile.schema.md",
		"design-spec.schema.md",
		"dialog.schema.md",
		"feature-structure.schema.md",
		"intent.schema.md",
		"page.schema.md",
		"surface.schema.md",
		"testcases.schema.md",
	}

	for _, name := range expected {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			t.Errorf("schema not written: %s", name)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("schema is empty: %s", name)
		}
	}
}

// TestSchemaNames_ReturnsAll asserts that SchemaNames() reports every file
// the //go:embed schemas/*.schema.md glob picks up — and nothing else. The
// list is data-driven on purpose: adding a schema file no longer requires
// bumping a hardcoded count.
func TestSchemaNames_ReturnsAll(t *testing.T) {
	names, err := SchemaNames()
	if err != nil {
		t.Fatal(err)
	}

	// Confirm the expected core set is present. Adding more schemas is
	// fine; removing one of these is a regression.
	mustExist := []string{
		"adapter.schema.md",
		"buildfile.schema.md",
		"surface.schema.md",
		"testcases.schema.md",
		"intent.schema.md",
		"dialog.schema.md",
		"blueprint.schema.md",
		"domain-model.schema.md",
		"feature-structure.schema.md",
	}
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n] = true
	}
	for _, n := range mustExist {
		if !have[n] {
			t.Errorf("expected schema %q to be present; got %v", n, names)
		}
	}

	// Confirm we didn't accidentally pick up non-schema files.
	for _, n := range names {
		if !endsWithSchemaSuffix(n) {
			t.Errorf("schema name %q does not match the *.schema.md glob — embed glob may have drifted", n)
		}
	}
}

func endsWithSchemaSuffix(name string) bool {
	const suffix = ".schema.md"
	return len(name) >= len(suffix) && name[len(name)-len(suffix):] == suffix
}
