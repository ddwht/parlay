package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

// makeProjectRoot writes a fully-populated parent root (config.yaml with
// agent + spec/intents/) at path. Used by add-root tests.
func makeProjectRoot(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, config.ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte("ai-agent: Generic\nsdd-framework: None\nprototype-framework: Go CLI\n")
	if err := os.WriteFile(filepath.Join(path, config.ParlayDir, config.ConfigFile), body, 0644); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{config.IntentsDir, config.HandoffDir, config.PagesDir} {
		if err := os.MkdirAll(filepath.Join(path, config.SpecDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

// withCtx returns a cobra.Command pre-loaded with a config.Context.
// Bypasses PersistentPreRunE so tests can drive runAddRoot etc. directly.
func withCtx(t *testing.T, root config.Root, idx *config.RootsIndex) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "fake"}
	cmd.SetContext(config.WithCtx(context.Background(), config.NewContext(
		&config.ResolutionResult{ActiveRoot: root},
		idx,
	)))
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	return cmd
}

func TestAddRoot_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	root := config.Root{Name: filepath.Base(tmp), Path: tmp, Kind: RootKindFromTopology(t, tmp)}
	cmd := withCtx(t, root, nil)
	if err := runAddRoot(cmd, []string{"apps/web"}); err != nil {
		t.Fatalf("runAddRoot: %v", err)
	}

	childPath := filepath.Join(tmp, "apps", "web")
	if _, err := os.Stat(filepath.Join(childPath, config.ParlayDir, config.ConfigFile)); err != nil {
		t.Errorf("child config not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(childPath, config.SpecDir, config.IntentsDir)); err != nil {
		t.Errorf("child intents dir not created: %v", err)
	}

	idx, err := config.LoadRootsIndex(tmp)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(idx.Children) != 1 || idx.Children[0].Name != "web" {
		t.Errorf("index entry: %+v", idx.Children)
	}
	if idx.Children[0].RelativePath != filepath.Join("apps", "web") {
		t.Errorf("relative path: %s", idx.Children[0].RelativePath)
	}

	// Parent pointer round-trip.
	got, err := os.ReadFile(filepath.Join(childPath, config.ParlayDir, config.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "parent:") {
		t.Errorf("child config missing parent: %s", got)
	}
}

func TestAddRoot_RefuseExistingParlayDir(t *testing.T) {
	tmp := t.TempDir()
	makeProjectRoot(t, tmp)
	subdir := filepath.Join(tmp, "apps", "web")
	if err := os.MkdirAll(filepath.Join(subdir, config.ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	err := runAddRoot(cmd, []string{"apps/web"})
	if err == nil || !strings.Contains(err.Error(), "already contains") {
		t.Errorf("expected 'already contains' error, got %v", err)
	}
}

func TestAddRoot_RefuseSubdirOutsideRoot(t *testing.T) {
	tmp := t.TempDir()
	makeProjectRoot(t, tmp)

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	err := runAddRoot(cmd, []string{"../escape"})
	if err == nil || !strings.Contains(err.Error(), "not inside") {
		t.Errorf("expected 'not inside' error, got %v", err)
	}
}

func TestAddRoot_RefuseNestedChild(t *testing.T) {
	tmp := t.TempDir()
	makeProjectRoot(t, tmp)

	// Pretend the active root is itself a child.
	root := config.Root{Path: tmp, Kind: config.RootKindChild}
	cmd := withCtx(t, root, nil)
	err := runAddRoot(cmd, []string{"apps/web"})
	if err == nil || !strings.Contains(err.Error(), "nested") {
		t.Errorf("expected 'nested' refusal, got %v", err)
	}
}

func TestAddRoot_RefuseDuplicateName(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	// Pre-existing index entry for "web" at a different path.
	idx := &config.RootsIndex{
		ParentPath: tmp,
		Children:   []config.Root{{Name: "web", RelativePath: "different/web"}},
	}
	if err := config.SaveRootsIndex(idx); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, idx)
	err := runAddRoot(cmd, []string{"apps/web"})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("expected duplicate-name refusal, got %v", err)
	}
}

// RootKindFromTopology returns the right RootKind for a freshly-created
// project root used in tests — Standalone for parents with no children.
func RootKindFromTopology(t *testing.T, _ string) config.RootKind {
	t.Helper()
	return config.RootKindStandalone
}
