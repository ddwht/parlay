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

func TestSchemaNames_ReturnsAll(t *testing.T) {
	names, err := SchemaNames()
	if err != nil {
		t.Fatal(err)
	}

	if len(names) != 10 {
		t.Errorf("expected 10 schemas, got %d: %v", len(names), names)
	}
}
