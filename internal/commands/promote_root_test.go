package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/config"
)

func TestPromoteRoot_OrphanedChild(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	child := filepath.Join(tmp, "child")
	if err := os.MkdirAll(filepath.Join(child, config.ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, config.ParlayDir, config.ConfigFile),
		[]byte("parent: ../missing\nai-agent: Generic\n"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{
		Path:       child,
		ParentPath: filepath.Join(tmp, "missing"),
		Kind:       config.RootKindChild,
	}
	cmd := withCtx(t, root, nil)
	if err := runPromoteRoot(cmd, nil); err != nil {
		t.Fatalf("promote: %v", err)
	}

	// Verify parent: pointer removed but other fields preserved.
	data, _ := os.ReadFile(filepath.Join(child, config.ParlayDir, config.ConfigFile))
	if strings.Contains(string(data), "parent:") {
		t.Errorf("parent: still in config: %s", data)
	}
	if !strings.Contains(string(data), "ai-agent: Generic") {
		t.Errorf("ai-agent field stripped: %s", data)
	}
}

func TestPromoteRoot_RefuseWhenParentResolves(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "child")
	for _, p := range []string{parent, child} {
		if err := os.MkdirAll(filepath.Join(p, config.ParlayDir), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, config.ParlayDir, config.ConfigFile), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.WriteParentPointer(child, parent); err != nil {
		t.Fatal(err)
	}
	root := config.Root{
		Path:       child,
		ParentPath: parent,
		Kind:       config.RootKindChild,
	}
	cmd := withCtx(t, root, nil)
	err := runPromoteRoot(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "still resolves") {
		t.Errorf("expected refusal, got %v", err)
	}
}

func TestPromoteRoot_RefuseWhenNotChild(t *testing.T) {
	tmp := t.TempDir()
	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	err := runPromoteRoot(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "not a child") {
		t.Errorf("expected refusal, got %v", err)
	}
}
