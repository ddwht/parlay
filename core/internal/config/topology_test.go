// parlay-feature: parlay-tool/multi-root
// parlay-cross-cutting: topology-validator
// parlay-artifact: test

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestScanTopology_CleanSingleRoot verifies that a single-root project
// with all three fields produces zero mismatches.
func TestScanTopology_CleanSingleRoot(t *testing.T) {
	tmp := t.TempDir()
	parlayDir := filepath.Join(tmp, ParlayDir)
	if err := os.MkdirAll(parlayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parlayDir, ConfigFile),
		[]byte("ai-agent: Claude Code\nsdd-framework: parlay-spec\nprototype-framework: go-cli\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	root := &Root{Path: tmp, Kind: RootKindStandalone}
	mismatches, err := ScanTopology(root)
	if err != nil {
		t.Fatalf("ScanTopology: %v", err)
	}
	if len(mismatches) != 0 {
		t.Fatalf("expected 0 mismatches for clean single-root; got %v", mismatches)
	}
}

// TestScanTopology_BareParent surfaces a bare-parent mismatch when the
// parent has roots.yaml but no config.yaml.
func TestScanTopology_BareParent(t *testing.T) {
	tmp := t.TempDir()
	parlayDir := filepath.Join(tmp, ParlayDir)
	if err := os.MkdirAll(parlayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parlayDir, RootsIndexFile),
		[]byte("children: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := &Root{Path: tmp, Kind: RootKindParent}
	mismatches, err := ScanTopology(root)
	if err != nil {
		t.Fatalf("ScanTopology: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch; got %d (%v)", len(mismatches), mismatches)
	}
	if mismatches[0].Kind != MismatchBareParent {
		t.Fatalf("expected bare-parent kind; got %s", mismatches[0].Kind)
	}
}

// TestScanTopology_AgentAtChild surfaces agent-at-child when a child
// config carries ai-agent and the parent does not.
func TestScanTopology_AgentAtChild(t *testing.T) {
	tmp := t.TempDir()

	// Parent: config without ai-agent + roots.yaml registering one child.
	parentParlay := filepath.Join(tmp, ParlayDir)
	if err := os.MkdirAll(parentParlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentParlay, ConfigFile),
		[]byte("sdd-framework: parlay-spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentParlay, RootsIndexFile),
		[]byte("children:\n  - name: web\n    relative-path: apps/web\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Child: config carrying ai-agent.
	childParlay := filepath.Join(tmp, "apps", "web", ParlayDir)
	if err := os.MkdirAll(childParlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childParlay, ConfigFile),
		[]byte("ai-agent: Cursor\nparent: ../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := &Root{Path: tmp, Kind: RootKindParent}
	mismatches, err := ScanTopology(root)
	if err != nil {
		t.Fatalf("ScanTopology: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch; got %d: %+v", len(mismatches), mismatches)
	}
	if mismatches[0].Kind != MismatchAgentAtChild {
		t.Fatalf("expected agent-at-child; got %s", mismatches[0].Kind)
	}
}

// TestScanTopology_BothHaveAgent surfaces both-have-agent when parent
// and child both declare ai-agent (regardless of values matching).
func TestScanTopology_BothHaveAgent(t *testing.T) {
	tmp := t.TempDir()

	parentParlay := filepath.Join(tmp, ParlayDir)
	if err := os.MkdirAll(parentParlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentParlay, ConfigFile),
		[]byte("ai-agent: Claude Code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentParlay, RootsIndexFile),
		[]byte("children:\n  - name: web\n    relative-path: apps/web\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	childParlay := filepath.Join(tmp, "apps", "web", ParlayDir)
	if err := os.MkdirAll(childParlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childParlay, ConfigFile),
		[]byte("ai-agent: Claude Code\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := &Root{Path: tmp, Kind: RootKindParent}
	mismatches, err := ScanTopology(root)
	if err != nil {
		t.Fatalf("ScanTopology: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch; got %d: %+v", len(mismatches), mismatches)
	}
	if mismatches[0].Kind != MismatchBothHaveAgent {
		t.Fatalf("expected both-have-agent; got %s", mismatches[0].Kind)
	}
}

// TestValidateAgentIdentitySingleSource_DuplicatedRejected ensures the
// load-time validator hard-errors when both parent and child declare
// ai-agent — even when values match.
func TestValidateAgentIdentitySingleSource_DuplicatedRejected(t *testing.T) {
	parent := &ProjectConfig{AIAgent: "Claude Code"}
	child := &ProjectConfig{AIAgent: "Claude Code"}
	err := ValidateAgentIdentitySingleSource(parent, child, "/p/.parlay/config.yaml", "/p/c/.parlay/config.yaml")
	if err == nil {
		t.Fatal("expected error for both-have-agent; got nil")
	}
	if !errors.Is(err, ErrAgentIdentityDuplicated) {
		t.Fatalf("expected ErrAgentIdentityDuplicated; got %v", err)
	}
}

// TestValidateAgentIdentitySingleSource_AtChildRejected ensures the
// validator hard-errors when only the child declares ai-agent.
func TestValidateAgentIdentitySingleSource_AtChildRejected(t *testing.T) {
	parent := &ProjectConfig{} // no ai-agent
	child := &ProjectConfig{AIAgent: "Cursor"}
	err := ValidateAgentIdentitySingleSource(parent, child, "/p/.parlay/config.yaml", "/p/c/.parlay/config.yaml")
	if err == nil {
		t.Fatal("expected error for agent-at-child; got nil")
	}
	if !errors.Is(err, ErrAgentIdentityAtChild) {
		t.Fatalf("expected ErrAgentIdentityAtChild; got %v", err)
	}
}

// TestResolveEffectiveConfig_ChildInheritsFromParent verifies the
// per-field child-first / parent-fallback rule.
func TestResolveEffectiveConfig_ChildInheritsFromParent(t *testing.T) {
	tmp := t.TempDir()

	parentParlay := filepath.Join(tmp, ParlayDir)
	if err := os.MkdirAll(parentParlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentParlay, ConfigFile),
		[]byte("ai-agent: Claude Code\nsdd-framework: parlay-spec\nprototype-framework: parlay-prototype\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	childPath := filepath.Join(tmp, "apps", "web")
	childParlay := filepath.Join(childPath, ParlayDir)
	if err := os.MkdirAll(childParlay, 0o755); err != nil {
		t.Fatal(err)
	}
	// Child only overrides prototype-framework; sdd-framework comes from parent.
	if err := os.WriteFile(filepath.Join(childParlay, ConfigFile),
		[]byte("prototype-framework: react\nparent: ../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	child := &Root{Path: childPath, ParentPath: tmp, Kind: RootKindChild}
	eff, err := ResolveEffectiveConfig(child)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfig: %v", err)
	}
	if eff.AIAgent.Value != "Claude Code" || eff.AIAgent.Origin != OriginFrom {
		t.Errorf("AIAgent: want Claude Code from parent (OriginFrom); got %+v", eff.AIAgent)
	}
	if eff.SDDFramework.Value != "parlay-spec" || eff.SDDFramework.Origin != OriginInheritedFrom {
		t.Errorf("SDDFramework: want parlay-spec inherited; got %+v", eff.SDDFramework)
	}
}
