// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-loader-reuse
// parlay-artifact: test

package domain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testPopulatedYAML mirrors the populated-model fixture: two entities
// (Customer, Order with a ref to Customer and an OrderStatus enum field), one
// enum (OrderStatus), and a populated deprecated operations block that must
// round-trip untouched.
const testPopulatedYAML = `schema_version: 1
enums:
  - name: OrderStatus
    values:
      - value: pending
        label: Pending
        tone: warning
      - value: paid
        tone: success
entities:
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
      - name: name
        type: string
        required: true
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
      - name: status
        type: OrderStatus
        enum: OrderStatus
        required: true
      - name: customer_id
        type: ref
        target: Customer
        required: true
operations:
  - entity: Order
    name: cancel-order
    input:
      - Order.id
    effects:
      - "set Order.status to cancelled"
`

// writeTempModel writes content to <dir>/domain-model.yaml and returns the dir.
func writeTempModel(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, modelFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write temp model: %v", err)
	}
	return dir
}

// TestLoadEmptyBootstrap asserts a project with no domain-model.yaml returns
// an empty model at the current schema_version with the sentinel etag, never
// a not-found.
func TestLoadEmptyBootstrap(t *testing.T) {
	root := t.TempDir()
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load empty project: %v", err)
	}
	if etag != SentinelEmpty {
		t.Fatalf("etag = %q, want %q", etag, SentinelEmpty)
	}
	if model.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", model.SchemaVersion, CurrentSchemaVersion)
	}
}

// TestLoadPopulated asserts the populated fixture parses into the expected
// shape and yields a content-derived etag.
func TestLoadPopulated(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load populated: %v", err)
	}
	if etag == SentinelEmpty || etag == "" {
		t.Fatalf("expected a content etag, got %q", etag)
	}
	if len(model.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(model.Entities))
	}
	if len(model.Enums) != 1 {
		t.Fatalf("enums = %d, want 1", len(model.Enums))
	}
	if len(model.Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(model.Operations))
	}
}

// TestSchemaVersionGate is the table-driven schema-version gate: older
// migrates in memory to the binary version, equal loads as-is, newer fails
// with the actionable sentinel, missing fails with its own sentinel.
func TestSchemaVersionGate(t *testing.T) {
	cases := []struct {
		name           string
		yaml           string
		binaryVersion  int
		wantErr        error
		wantServedVer  int
		wantMigrated   bool
	}{
		{
			name:          "older migrated in memory",
			yaml:          testPopulatedYAML, // schema_version: 1
			binaryVersion: 2,
			wantServedVer: 2,
			wantMigrated:  true,
		},
		{
			name:          "equal loads as-is",
			yaml:          testPopulatedYAML,
			binaryVersion: 1,
			wantServedVer: 1,
		},
		{
			name:          "newer than binary fails actionably",
			yaml:          "schema_version: 3\nentities: []\n",
			binaryVersion: 2,
			wantErr:       ErrSchemaVersionNewer,
		},
		{
			name:          "missing schema_version fails",
			yaml:          "entities: []\n",
			binaryVersion: 2,
			wantErr:       ErrMissingSchemaVersion,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, err := decodeAndMigrate([]byte(tc.yaml), tc.binaryVersion)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeAndMigrate: %v", err)
			}
			if model.SchemaVersion != tc.wantServedVer {
				t.Fatalf("served schema_version = %d, want %d", model.SchemaVersion, tc.wantServedVer)
			}
		})
	}
}

// TestOlderMigrationLeavesDiskUntouched asserts an in-memory migration does
// not rewrite the on-disk file (the file is untouched until a save).
func TestOlderMigrationLeavesDiskUntouched(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	path := filepath.Join(root, modelFileName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if _, err := decodeAndMigrate(before, 2); err != nil {
		t.Fatalf("decodeAndMigrate: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("on-disk file changed during an in-memory migration")
	}
}
