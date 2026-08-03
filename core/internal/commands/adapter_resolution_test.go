// parlay-feature: parlay-tool/adapter-authoring
// parlay-artifact: test
//
// Adapter resolution across roots. Two properties matter: a child root
// inherits the parent's adapters (which is what the generated CLAUDE.md has
// always told users), and an ambiguous single-target project is refused rather
// than resolved by filename order.

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func writeAdapterAt(t *testing.T, root, slug, body string) {
	t.Helper()
	dir := filepath.Join(root, config.ParlayDir, config.AdaptersDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug+".adapter.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func childCtx(t *testing.T, child, parent string) *config.Context {
	t.Helper()
	return config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "child", Path: child, Kind: config.RootKindChild, ParentPath: parent},
		Source:     config.SourceCwdWalkUp,
	}, nil)
}

func TestSoleAdapterFile_ChildInheritsParent(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "apps", "web")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAdapterAt(t, parent, "react-antd", "name: react-antd\nkind: presentation\n")

	got, err := soleAdapterFile(childCtx(t, child, parent))
	if err != nil {
		t.Fatalf("a child with no adapters should inherit the parent's: %v", err)
	}
	if !strings.HasPrefix(got, parent) || strings.HasPrefix(got, child) {
		t.Errorf("expected the parent's adapter, got %q", got)
	}
}

func TestSoleAdapterFile_ChildLocalOverridesParent(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "apps", "web")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAdapterAt(t, parent, "react-antd", "name: react-antd\nkind: presentation\n")
	writeAdapterAt(t, child, "react-antd", "name: react-antd\nkind: presentation\n")

	got, err := soleAdapterFile(childCtx(t, child, parent))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, child) {
		t.Errorf("a child-local adapter must win over the parent's, got %q", got)
	}
}

// The hazard the predecessor created: firstAdapterFile returned the
// lexically-first match, so a react-nest-prisma project resolved
// nestjs-application — a widget-less backend adapter — wherever a presentation
// one was wanted. Refusing is the fix; a wrong adapter yields plausible,
// wrong paths.
func TestSoleAdapterFile_RefusesToGuessAmongSeveral(t *testing.T) {
	root := t.TempDir()
	writeAdapterAt(t, root, "nestjs-application", "name: nestjs-application\nkind: application\n")
	writeAdapterAt(t, root, "react-antd", "name: react-antd\nkind: presentation\n")

	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "p", Path: root, Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)

	got, err := soleAdapterFile(cfg)
	if err == nil {
		t.Fatalf("expected a refusal among several adapters; got %q", got)
	}
	if !strings.Contains(err.Error(), "adapter-set.yaml") {
		t.Errorf("the error should point at the fix (pin an adapter-set); got %v", err)
	}
}
