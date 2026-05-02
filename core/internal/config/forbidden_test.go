package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateChildRootForbiddenDirectories_Clean(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ParlayDir, AdaptersDir), 0755); err != nil {
		t.Fatal(err)
	}
	child := Root{Path: tmp, Kind: RootKindChild}
	v := ValidateChildRootForbiddenDirectories(child, []string{".claude/skills", ".cursor/agents"})
	if v != nil {
		t.Errorf("clean child should have no violation, got %+v", v)
	}
}

func TestValidateChildRootForbiddenDirectories_SchemasInChild(t *testing.T) {
	tmp := t.TempDir()
	schemasDir := filepath.Join(tmp, ParlayDir, SchemasDir)
	if err := os.MkdirAll(schemasDir, 0755); err != nil {
		t.Fatal(err)
	}
	child := Root{Path: tmp, Kind: RootKindChild}
	v := ValidateChildRootForbiddenDirectories(child, nil)
	if v == nil {
		t.Fatal("expected violation")
	}
	if v.Rule != RuleSchemasParentOnly {
		t.Errorf("rule: want %q, got %q", RuleSchemasParentOnly, v.Rule)
	}
	if v.Error() == "" {
		t.Error("Error() returned empty")
	}
}

func TestValidateChildRootForbiddenDirectories_AgentSurfaceInChild(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".claude", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	child := Root{Path: tmp, Kind: RootKindChild}
	v := ValidateChildRootForbiddenDirectories(child, []string{".claude/skills"})
	if v == nil {
		t.Fatal("expected violation")
	}
	if v.Rule != RuleAgentSurfaceParentOnly {
		t.Errorf("rule: want %q, got %q", RuleAgentSurfaceParentOnly, v.Rule)
	}
}

func TestValidateChildRootForbiddenDirectories_FileBasedSurface(t *testing.T) {
	tmp := t.TempDir()
	// AGENT_INSTRUCTIONS.md is a file, not a directory.
	instr := filepath.Join(tmp, "AGENT_INSTRUCTIONS.md")
	if err := os.WriteFile(instr, []byte("instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	child := Root{Path: tmp, Kind: RootKindChild}
	v := ValidateChildRootForbiddenDirectories(child, []string{"AGENT_INSTRUCTIONS.md"})
	if v == nil {
		t.Fatal("expected violation for file-based surface")
	}
}

func TestValidateChildRootForbiddenDirectories_SkipsNonChild(t *testing.T) {
	tmp := t.TempDir()
	// Stand up a forbidden dir but mark the root as standalone.
	if err := os.MkdirAll(filepath.Join(tmp, ParlayDir, SchemasDir), 0755); err != nil {
		t.Fatal(err)
	}
	parent := Root{Path: tmp, Kind: RootKindParent}
	if v := ValidateChildRootForbiddenDirectories(parent, nil); v != nil {
		t.Errorf("parent should be skipped, got %+v", v)
	}
	standalone := Root{Path: tmp, Kind: RootKindStandalone}
	if v := ValidateChildRootForbiddenDirectories(standalone, nil); v != nil {
		t.Errorf("standalone should be skipped, got %+v", v)
	}
}
