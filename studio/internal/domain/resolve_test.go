// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-read-path-resolution
// parlay-artifact: test

package domain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveModelPathTargetsYAMLOnly asserts the resolver produces only the
// resolved root's domain-model.yaml — never a .md path.
func TestResolveModelPathTargetsYAMLOnly(t *testing.T) {
	got := resolveModelPath("/project")
	want := filepath.Join("/project", "domain-model.yaml")
	if got != want {
		t.Fatalf("resolveModelPath = %q, want %q", got, want)
	}
	if strings.HasSuffix(got, ".md") {
		t.Fatalf("resolveModelPath must never target a .md file, got %q", got)
	}
}

// TestLegacyMarkdownNeverRead asserts that a project holding only a legacy
// domain-model.md (and no domain-model.yaml) loads the empty-model bootstrap
// rather than parsing, merging, or consulting the .md as a fallback.
func TestLegacyMarkdownNeverRead(t *testing.T) {
	root := t.TempDir()
	// Seed a legacy markdown file that MUST be ignored. Its content is
	// deliberately non-empty and non-YAML so any accidental parse would fail
	// loudly rather than silently succeed.
	mdPath := filepath.Join(root, "domain-model.md")
	if err := os.WriteFile(mdPath, []byte("# Legacy model\n\nEntity: Customer\n"), 0o644); err != nil {
		t.Fatalf("seed legacy md: %v", err)
	}

	if p := resolveModelPath(root); strings.HasSuffix(p, ".md") {
		t.Fatalf("resolver targeted the legacy .md: %q", p)
	}

	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load with only a legacy .md present: %v", err)
	}
	if etag != SentinelEmpty {
		t.Fatalf("expected empty-model bootstrap etag %q, got %q", SentinelEmpty, etag)
	}
	if len(model.Entities) != 0 {
		t.Fatalf("legacy .md must not populate the model; got %d entities", len(model.Entities))
	}
}
