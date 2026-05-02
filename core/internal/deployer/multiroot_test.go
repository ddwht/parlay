package deployer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/embedded"
)

// writeRootsIndex sets up a parent root with two child entries.
func writeRootsIndex(t *testing.T, parent string, children []config.Root) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(parent, config.ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	idx := &config.RootsIndex{ParentPath: parent, Children: children}
	if err := config.SaveRootsIndex(idx); err != nil {
		t.Fatal(err)
	}
}

func TestRenderMultiRootSection_NoChildren(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, config.ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	got := renderMultiRootSection(tmp)
	if got != "" {
		t.Errorf("expected empty section for no children, got: %q", got)
	}
}

func TestRenderMultiRootSection_WithChildren(t *testing.T) {
	tmp := t.TempDir()
	writeRootsIndex(t, tmp, []config.Root{
		{Name: "web", RelativePath: "apps/web", Description: "Frontend"},
		{Name: "api", RelativePath: "apps/api"},
	})
	got := renderMultiRootSection(tmp)
	if !strings.Contains(got, "## Multi-Root Layout") {
		t.Errorf("missing header: %s", got)
	}
	if !strings.Contains(got, "**web**") {
		t.Errorf("missing web entry: %s", got)
	}
	if !strings.Contains(got, "Frontend") {
		t.Errorf("missing description: %s", got)
	}
	if !strings.Contains(got, "**api**") {
		t.Errorf("missing api entry: %s", got)
	}
}

func TestClaudeDeployer_MultiRootInCLAUDEmd(t *testing.T) {
	root := t.TempDir()
	writeRootsIndex(t, root, []config.Root{
		{Name: "web", RelativePath: "apps/web"},
	})

	d := &ClaudeDeployer{}
	if err := d.Deploy(root, []embedded.SkillEntry{
		{Name: "sync", Content: []byte("# sync")},
	}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "## Multi-Root Layout") {
		t.Errorf("CLAUDE.md missing multi-root section:\n%s", body)
	}
	if !strings.Contains(body, "apps/web") {
		t.Errorf("CLAUDE.md missing child path:\n%s", body)
	}
	// Section must sit between markers.
	begin := strings.Index(body, parlayMarkerBegin)
	end := strings.Index(body, parlayMarkerEnd)
	mr := strings.Index(body, "## Multi-Root Layout")
	if begin < 0 || end < 0 || mr < begin || mr > end {
		t.Errorf("multi-root section not bracketed by markers (begin=%d, mr=%d, end=%d)", begin, mr, end)
	}
}

func TestClaudeDeployer_MultiRootSectionDisappearsAfterChildRemoval(t *testing.T) {
	root := t.TempDir()
	// First deploy with one child.
	writeRootsIndex(t, root, []config.Root{
		{Name: "web", RelativePath: "apps/web"},
	})
	d := &ClaudeDeployer{}
	if err := d.Deploy(root, []embedded.SkillEntry{}); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(first), "## Multi-Root Layout") {
		t.Fatal("setup invariant: section should be present after first deploy")
	}

	// Remove all children, redeploy.
	if err := os.Remove(filepath.Join(root, config.ParlayDir, config.RootsIndexFile)); err != nil {
		t.Fatal(err)
	}
	if err := d.Deploy(root, []embedded.SkillEntry{}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if strings.Contains(string(second), "## Multi-Root Layout") {
		t.Errorf("section should disappear after children removed:\n%s", second)
	}
}

func TestGenericDeployer_MultiRootInAgentInstructions(t *testing.T) {
	root := t.TempDir()
	writeRootsIndex(t, root, []config.Root{
		{Name: "web", RelativePath: "apps/web"},
	})
	d := &GenericDeployer{}
	if err := d.Deploy(root, []embedded.SkillEntry{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENT_INSTRUCTIONS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Multi-Root Layout") {
		t.Errorf("AGENT_INSTRUCTIONS.md missing multi-root section")
	}
}

func TestCursorDeployer_MultiRootInRule(t *testing.T) {
	root := t.TempDir()
	writeRootsIndex(t, root, []config.Root{
		{Name: "web", RelativePath: "apps/web"},
	})
	d := &CursorDeployer{}
	if err := d.Deploy(root, []embedded.SkillEntry{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "parlay.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Multi-Root Layout") {
		t.Errorf(".cursor/rules/parlay.mdc missing multi-root section")
	}
}
