package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSchemas_WritesAllSchemas(t *testing.T) {
	dir := t.TempDir()

	n, err := WriteSchemas(dir)
	if err != nil {
		t.Fatalf("WriteSchemas failed: %v", err)
	}
	if n == 0 {
		t.Fatal("WriteSchemas reported zero writes into an empty directory")
	}

	expected := []string{
		"adapter.schema.md",
		"blueprint.schema.md",
		"buildfile.schema.md",
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

// TestWriteSchemas_SecondRunWritesNothing pins the content-hash skip at the
// schema writer. Salvaged behaviour from the editor's deployer: an upgrade over
// unchanged sources must rewrite nothing, so a changed mtime on a deployed
// schema means changed content rather than merely a command having been run.
func TestWriteSchemas_SecondRunWritesNothing(t *testing.T) {
	dir := t.TempDir()

	first, err := WriteSchemas(dir)
	if err != nil {
		t.Fatalf("first WriteSchemas: %v", err)
	}
	if first == 0 {
		t.Fatal("first run wrote nothing")
	}

	second, err := WriteSchemas(dir)
	if err != nil {
		t.Fatalf("second WriteSchemas: %v", err)
	}
	if second != 0 {
		t.Fatalf("second run rewrote %d schema(s); the content-hash skip did not fire", second)
	}

	// One perturbed file must come back, and only that one.
	victim := filepath.Join(dir, "intent.schema.md")
	if err := os.WriteFile(victim, []byte("clobbered"), 0o644); err != nil {
		t.Fatalf("perturb: %v", err)
	}
	third, err := WriteSchemas(dir)
	if err != nil {
		t.Fatalf("third WriteSchemas: %v", err)
	}
	if third != 1 {
		t.Fatalf("after perturbing one schema, wrote %d; want exactly 1", third)
	}
}

// TestWriteModules_SecondRunWritesNothing is the same property at the module
// writer, which deploys the phase instructions the loop's subagents read.
func TestWriteModules_SecondRunWritesNothing(t *testing.T) {
	dir := t.TempDir()

	first, err := WriteModules(dir)
	if err != nil {
		t.Fatalf("first WriteModules: %v", err)
	}
	if first == 0 {
		t.Fatal("first run wrote no modules")
	}

	second, err := WriteModules(dir)
	if err != nil {
		t.Fatalf("second WriteModules: %v", err)
	}
	if second != 0 {
		t.Fatalf("second run rewrote %d module(s); the content-hash skip did not fire", second)
	}
}
