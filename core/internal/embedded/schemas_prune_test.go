package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPruneStaleSchemas covers the counterpart PruneStaleModules had and the
// schema writer did not. WriteSchemas only adds and overwrites, so before this
// existed, retiring a schema left it deployed in every project on disk —
// authoritative-looking documentation for something that no longer shipped.
func TestPruneStaleSchemas(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteSchemas(dir); err != nil {
		t.Fatalf("WriteSchemas: %v", err)
	}
	if _, err := WriteSchemaDigest(dir); err != nil {
		t.Fatalf("WriteSchemaDigest: %v", err)
	}

	// A schema core used to ship, and a file that is not a schema at all.
	stale := filepath.Join(dir, "design-loop-result.schema.md")
	if err := os.WriteFile(stale, []byte("retired"), 0o644); err != nil {
		t.Fatalf("seed stale: %v", err)
	}
	bystander := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(bystander, []byte("mine"), 0o644); err != nil {
		t.Fatalf("seed bystander: %v", err)
	}

	removed, err := PruneStaleSchemas(dir)
	if err != nil {
		t.Fatalf("PruneStaleSchemas: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d, want exactly 1", removed)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("retired schema survived the prune: %v", err)
	}
	// DIGEST.md is generated beside the schemas, not one of them.
	if _, err := os.Stat(filepath.Join(dir, "DIGEST.md")); err != nil {
		t.Fatalf("prune removed DIGEST.md: %v", err)
	}
	if _, err := os.Stat(bystander); err != nil {
		t.Fatalf("prune removed a non-schema file: %v", err)
	}
	// A shipped schema must survive.
	if _, err := os.Stat(filepath.Join(dir, "intent.schema.md")); err != nil {
		t.Fatalf("prune removed a shipped schema: %v", err)
	}

	// Idempotent.
	again, err := PruneStaleSchemas(dir)
	if err != nil {
		t.Fatalf("second prune: %v", err)
	}
	if again != 0 {
		t.Fatalf("second prune removed %d; want 0", again)
	}
}
