package commands

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// captureStdout temporarily redirects os.Stdout, runs fn, and returns
// what was written. Used for status' fmt.Printf output that doesn't go
// through cobra.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	w.Close()
	os.Stdout = prev
	return <-done
}

func TestStatus_StandaloneNoFeatures(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	out := captureStdout(t, func() {
		if err := runStatus(cmd, nil); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' for empty features, got: %s", out)
	}
	if !strings.Contains(out, tmp) {
		t.Errorf("expected root path in output, got: %s", out)
	}
}

func TestStatus_BareParentListsChildren(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	idx := &config.RootsIndex{
		ParentPath: tmp,
		Children: []config.Root{
			{Name: "web", RelativePath: "apps/web", Description: "Frontend"},
			{Name: "api", RelativePath: "apps/api"},
		},
	}
	if err := config.SaveRootsIndex(idx); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindParent}
	cmd := withCtx(t, root, idx)
	out := captureStdout(t, func() {
		if err := runStatus(cmd, nil); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if !strings.Contains(out, "child roots:") {
		t.Errorf("expected child roots section, got: %s", out)
	}
	if !strings.Contains(out, "web") || !strings.Contains(out, "api") {
		t.Errorf("expected both children listed, got: %s", out)
	}
	if !strings.Contains(out, "Frontend") {
		t.Errorf("expected description for web, got: %s", out)
	}
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' for parent features, got: %s", out)
	}
}

func TestStatus_ListsFeatures(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)
	// Drop a feature into spec/intents/foo/intents.md.
	featDir := filepath.Join(tmp, config.SpecDir, config.IntentsDir, "foo")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte("# Foo"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	out := captureStdout(t, func() {
		if err := runStatus(cmd, nil); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if !strings.Contains(out, "foo") {
		t.Errorf("expected feature 'foo' in output: %s", out)
	}
	if !strings.Contains(out, "features: 1") {
		t.Errorf("expected 'features: 1', got: %s", out)
	}
}
