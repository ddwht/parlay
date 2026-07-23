// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/compare-and-swap-save
// parlay-artifact: test

package domain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parlay-tool/parlay/studio/internal/server"
)

// TestSaveMatchingEtagWrites asserts a save presenting the load-time etag
// writes the file and the edit lands on disk.
func TestSaveMatchingEtagWrites(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	model.Entities = append(model.Entities, Entity{
		Name:   "Widget",
		Fields: []Field{{Name: "id", Type: "uuid", Required: true}},
	})

	newEtag, err := Save(context.Background(), root, model, etag)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if newEtag == etag {
		t.Fatal("etag did not advance after a content-changing save")
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !strings.Contains(string(after), "Widget") {
		t.Fatalf("edit not reflected on disk:\n%s", after)
	}
}

// TestSaveStaleEtagConflictWritesNothing asserts a save presenting a stale etag
// returns the conflict envelope carrying both etags and leaves the on-disk file
// unchanged.
func TestSaveStaleEtagConflictWritesNothing(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, _, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = Save(context.Background(), root, model, Etag("sha256:9f2c-stale"))
	var cerr *server.ConflictError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected *server.ConflictError, got %v", err)
	}
	if cerr.AttemptedETag != "sha256:9f2c-stale" {
		t.Fatalf("attempted etag = %q, want the presented stale token", cerr.AttemptedETag)
	}
	if cerr.CurrentETag == "" || cerr.CurrentETag == cerr.AttemptedETag {
		t.Fatalf("conflict must carry the differing current etag, got %q", cerr.CurrentETag)
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != testPopulatedYAML {
		t.Fatal("on-disk file changed despite a stale-etag conflict")
	}
}

// TestSaveEmptyModelSentinelCreatesFile asserts a first save presenting the
// empty sentinel creates domain-model.yaml, and a non-sentinel token against a
// missing file is a conflict.
func TestSaveEmptyModelSentinelCreatesFile(t *testing.T) {
	root := t.TempDir()
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if etag != SentinelEmpty {
		t.Fatalf("bootstrap etag = %q, want %q", etag, SentinelEmpty)
	}

	model.Entities = append(model.Entities, Entity{
		Name:   "Customer",
		Fields: []Field{{Name: "id", Type: "uuid", Required: true}},
	})
	if _, err := Save(context.Background(), root, model, etag); err != nil {
		t.Fatalf("first save against empty sentinel: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, modelFileName)); err != nil {
		t.Fatalf("first save did not create the file: %v", err)
	}

	// A non-sentinel token against a still-missing file is a conflict.
	root2 := t.TempDir()
	_, err = Save(context.Background(), root2, model, Etag("sha256:not-empty"))
	var cerr *server.ConflictError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected conflict for a stale token against a missing file, got %v", err)
	}
}

// TestSavePersistsCurrentSchemaVersion asserts a saved model carries the
// binary's current schema_version on disk.
func TestSavePersistsCurrentSchemaVersion(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	model.SchemaVersion = CurrentSchemaVersion
	if _, err := Save(context.Background(), root, model, etag); err != nil {
		t.Fatalf("Save: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !strings.Contains(string(after), "schema_version: 1") {
		t.Fatalf("saved file missing current schema_version:\n%s", after)
	}
}
