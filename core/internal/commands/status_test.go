package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestStatus_StandaloneNoFeatures(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' for empty features, got: %s", out)
	}
	if !strings.Contains(out, tmp) {
		t.Errorf("expected root path in output, got: %s", out)
	}
}

func TestStatus_BareParentListsChildren(t *testing.T) {
	// Updated for parlay-tool/status-feature-phases:
	// status now renders one section per registered child (in
	// roots.yaml slice order) instead of a single "child roots:"
	// table. Both children appear; the bare parent reports
	// "(none)" for its own features.
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
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "web") || !strings.Contains(out, "api") {
		t.Errorf("expected both children listed, got: %s", out)
	}
	// Registration-order, NOT alphabetized: web should appear before api.
	if strings.Index(out, "web") > strings.Index(out, "api") {
		t.Errorf("expected web before api (registration order), got: %s", out)
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
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "foo") {
		t.Errorf("expected feature 'foo' in output: %s", out)
	}
	if !strings.Contains(out, "features: 1") {
		t.Errorf("expected 'features: 1', got: %s", out)
	}
	if strings.Contains(out, "orphaned build dirs") {
		t.Errorf("expected no orphaned-build-dirs anomaly line for a clean project, got: %s", out)
	}
}

// TestStatus_FlagsOrphanedBuildDir confirms status surfaces a
// .parlay/build/<feature>/ directory that carries build artifacts but
// has no matching spec/intents/<feature>/intents.md, instead of
// silently omitting it the way it did before this check existed.
func TestStatus_FlagsOrphanedBuildDir(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	// A real feature, in sync across both trees.
	realDir := filepath.Join(tmp, config.SpecDir, config.IntentsDir, "real-feature")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "intents.md"), []byte("# Real Feature"), 0644); err != nil {
		t.Fatal(err)
	}

	// A ghost feature: build artifact survives, intents.md is gone.
	ghostBuildDir := filepath.Join(tmp, config.ParlayDir, "build", "ghost-feature")
	if err := os.MkdirAll(ghostBuildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghostBuildDir, "buildfile.yaml"), []byte("feature: ghost-feature\n"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "orphaned build dirs: 1") {
		t.Errorf("expected orphaned build dirs anomaly to be flagged, got: %s", out)
	}
	if !strings.Contains(out, "ghost-feature") {
		t.Errorf("expected ghost-feature named in the anomaly, got: %s", out)
	}
	if strings.Contains(out, "real-feature") == false {
		t.Errorf("expected real-feature still listed as a normal feature, got: %s", out)
	}
}

// TestStatus_JSON_IncludesOrphanedBuildDirs confirms the JSON envelope
// carries the same anomaly, always as a present (never nil) array.
func TestStatus_JSON_IncludesOrphanedBuildDirs(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	ghostBuildDir := filepath.Join(tmp, config.ParlayDir, "build", "ghost-feature")
	if err := os.MkdirAll(ghostBuildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghostBuildDir, "testcases.yaml"), []byte("feature: ghost-feature\n"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	pctx := config.NewContext(&config.ResolutionResult{ActiveRoot: root}, nil)
	env := buildStatusEnvelope(pctx)

	if env.Root.OrphanedBuildDirs == nil {
		t.Fatal("OrphanedBuildDirs must never be nil — JSON contract requires the array always present")
	}
	if len(env.Root.OrphanedBuildDirs) != 1 || env.Root.OrphanedBuildDirs[0] != "ghost-feature" {
		t.Errorf("OrphanedBuildDirs = %v, want [ghost-feature]", env.Root.OrphanedBuildDirs)
	}
}
