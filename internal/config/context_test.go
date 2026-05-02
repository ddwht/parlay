package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestContext_PathsForStandaloneRoot(t *testing.T) {
	tmp := t.TempDir()
	c := NewContext(&ResolutionResult{
		ActiveRoot: Root{Name: "p", Path: tmp, Kind: RootKindStandalone},
		Source:     SourceCwdWalkUp,
	}, nil)

	if got, want := c.RepoRoot(), tmp; got != want {
		t.Errorf("RepoRoot: want %s, got %s", want, got)
	}
	if got, want := c.SchemasPath(), filepath.Join(tmp, ParlayDir, SchemasDir); got != want {
		t.Errorf("SchemasPath: want %s, got %s", want, got)
	}
	if got, want := c.FeaturePath("foo"), filepath.Join(tmp, SpecDir, IntentsDir, "foo"); got != want {
		t.Errorf("FeaturePath: want %s, got %s", want, got)
	}
	if got, want := c.FeaturePath("init/foo"), filepath.Join(tmp, SpecDir, IntentsDir, "init", "foo"); got != want {
		t.Errorf("FeaturePath qualified: want %s, got %s", want, got)
	}
	if got, want := c.BuildPath("init/foo"), filepath.Join(tmp, ParlayDir, BuildDir, "init", "foo"); got != want {
		t.Errorf("BuildPath: want %s, got %s", want, got)
	}
	if got, want := c.HandoffPath("foo"), filepath.Join(tmp, SpecDir, HandoffDir, "foo"); got != want {
		t.Errorf("HandoffPath: want %s, got %s", want, got)
	}
	if got, want := c.ProjectBuildPath(), filepath.Join(tmp, ParlayDir, BuildDir, "_project"); got != want {
		t.Errorf("ProjectBuildPath: want %s, got %s", want, got)
	}
}

func TestContext_RepoRootForChildPointsAtParent(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "apps", "web")
	c := NewContext(&ResolutionResult{
		ActiveRoot: Root{
			Path:       child,
			ParentPath: parent,
			Kind:       RootKindChild,
		},
	}, nil)
	if got := c.RepoRoot(); got != parent {
		t.Errorf("RepoRoot: want %s, got %s", parent, got)
	}
	// Schemas come from the parent regardless of active root.
	if got, want := c.SchemasPath(), filepath.Join(parent, ParlayDir, SchemasDir); got != want {
		t.Errorf("SchemasPath: want %s, got %s", want, got)
	}
	// Features still live under the active (child) root.
	if got, want := c.FeaturePath("foo"), filepath.Join(child, SpecDir, IntentsDir, "foo"); got != want {
		t.Errorf("FeaturePath: want %s, got %s", want, got)
	}
}

func TestContext_WithCtxFromCtx(t *testing.T) {
	c := &Context{Root: Root{Name: "x"}}
	ctx := WithCtx(context.Background(), c)
	if got := FromCtx(ctx); got != c {
		t.Errorf("round trip: want %p, got %p", c, got)
	}
	// Empty context returns nil.
	if got := FromCtx(context.Background()); got != nil {
		t.Errorf("missing ctx: want nil, got %+v", got)
	}
	// Nil context returns nil cleanly.
	if got := FromCtx(nil); got != nil {
		t.Errorf("nil ctx: want nil, got %+v", got)
	}
}

func TestContext_ResolveAdapterChildFirst(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "apps", "web")

	for _, dir := range []string{
		filepath.Join(parent, ParlayDir, AdaptersDir),
		filepath.Join(child, ParlayDir, AdaptersDir),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	parentAdapter := filepath.Join(parent, ParlayDir, AdaptersDir, "react.adapter.yaml")
	if err := os.WriteFile(parentAdapter, []byte("from-parent\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c := NewContext(&ResolutionResult{
		ActiveRoot: Root{Path: child, ParentPath: parent, Kind: RootKindChild},
	}, nil)

	// No child override → falls back to parent.
	got, src := c.ResolveAdapter("react")
	if got != parentAdapter {
		t.Errorf("fallback: want %s, got %s", parentAdapter, got)
	}
	if src != ProvidedByParent {
		t.Errorf("fallback source: want %q, got %q", ProvidedByParent, src)
	}

	// Add child override → child wins.
	childAdapter := filepath.Join(child, ParlayDir, AdaptersDir, "react.adapter.yaml")
	if err := os.WriteFile(childAdapter, []byte("from-child\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, src = c.ResolveAdapter("react")
	if got != childAdapter {
		t.Errorf("override: want %s, got %s", childAdapter, got)
	}
	if src != ProvidedByChild {
		t.Errorf("override source: want %q, got %q", ProvidedByChild, src)
	}

	// Missing adapter → empty.
	got, _ = c.ResolveAdapter("nonexistent")
	if got != "" {
		t.Errorf("missing: want empty, got %s", got)
	}
}

