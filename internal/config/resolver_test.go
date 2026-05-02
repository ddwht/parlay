package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeRoot creates a minimal parlay root (a .parlay/ directory) at path.
// When parentRel is non-empty, also writes a parent: pointer to the
// child's config.yaml.
func makeRoot(t *testing.T, path, parentRel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ParlayDir), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	cfgPath := filepath.Join(path, ParlayDir, ConfigFile)
	if parentRel == "" {
		// Empty config file is enough to mark the root.
		if err := os.WriteFile(cfgPath, []byte{}, 0644); err != nil {
			t.Fatalf("write %s: %v", cfgPath, err)
		}
		return
	}
	body := "parent: " + parentRel + "\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", cfgPath, err)
	}
}

func TestResolveActiveRoot_WalkUpHit(t *testing.T) {
	tmp := t.TempDir()
	makeRoot(t, tmp, "")
	// Walk-up from a nested subfolder finds the root.
	deep := filepath.Join(tmp, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	res, err := ResolveActiveRoot(deep, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ActiveRoot.Path != tmp {
		t.Errorf("path: want %s, got %s", tmp, res.ActiveRoot.Path)
	}
	if res.Source != SourceCwdWalkUp {
		t.Errorf("source: want %q, got %q", SourceCwdWalkUp, res.Source)
	}
	if res.ActiveRoot.Kind != RootKindStandalone {
		t.Errorf("kind: want %q, got %q", RootKindStandalone, res.ActiveRoot.Kind)
	}
}

func TestResolveActiveRoot_NoRootFound(t *testing.T) {
	tmp := t.TempDir()
	// Place a .git boundary so walk-up stops there.
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(tmp, "a", "b")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveActiveRoot(deep, nil)
	if !errors.Is(err, ErrNoRootFound) {
		t.Errorf("want ErrNoRootFound, got %v", err)
	}
}

func TestResolveActiveRoot_GitBoundary(t *testing.T) {
	tmp := t.TempDir()
	// .git at tmp; .parlay/ ABOVE the .git boundary should NOT be found.
	parent := filepath.Dir(tmp)
	makeRoot(t, parent, "")
	defer os.RemoveAll(filepath.Join(parent, ParlayDir))
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(tmp, "a")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveActiveRoot(deep, nil)
	if !errors.Is(err, ErrNoRootFound) {
		t.Errorf("walk-up should stop at .git boundary; got %v", err)
	}
}

func TestResolveActiveRoot_ParlayRootEnv(t *testing.T) {
	tmp := t.TempDir()
	makeRoot(t, tmp, "")
	res, err := ResolveActiveRoot("/tmp/somewhere-else", map[string]string{"PARLAY_ROOT": tmp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ActiveRoot.Path != tmp {
		t.Errorf("path: want %s, got %s", tmp, res.ActiveRoot.Path)
	}
	if res.Source != SourceParlayRootEnv {
		t.Errorf("source: want %q, got %q", SourceParlayRootEnv, res.Source)
	}
}

func TestResolveActiveRoot_ParlayRootEnvInvalid(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want error
	}{
		{"not absolute", "relative/path", ErrParlayRootInvalid},
		{"missing parlay dir", t.TempDir(), ErrParlayRootInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveActiveRoot("/", map[string]string{"PARLAY_ROOT": tc.env})
			if !errors.Is(err, tc.want) {
				t.Errorf("want %v, got %v", tc.want, err)
			}
		})
	}
}

func TestResolveActiveRoot_ChildKindFromConfig(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "apps", "web")
	makeRoot(t, parent, "")
	makeRoot(t, child, "../..")

	res, err := ResolveActiveRoot(child, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ActiveRoot.Kind != RootKindChild {
		t.Errorf("kind: want %q, got %q", RootKindChild, res.ActiveRoot.Kind)
	}
	if res.ActiveRoot.ParentPath != parent {
		t.Errorf("parent path: want %s, got %s", parent, res.ActiveRoot.ParentPath)
	}
	if !strings.HasSuffix(res.ActiveRoot.RelativePath, filepath.Join("apps", "web")) {
		t.Errorf("relative path: %q does not end with apps/web", res.ActiveRoot.RelativePath)
	}
}

func TestApplyRootFlagToResolution(t *testing.T) {
	cases := []struct {
		name, flag, prefix, want string
		wantErr                  bool
	}{
		{"both empty", "", "", "", false},
		{"flag only", "web", "", "web", false},
		{"prefix only", "", "web", "web", false},
		{"agree", "web", "web", "web", false},
		{"disagree", "web", "api", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyRootFlagToResolution(tc.flag, tc.prefix)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil; result=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestValidateParentPointer(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	makeRoot(t, parent, "")

	// Standalone — no validation needed.
	if err := ValidateParentPointer(Root{Kind: RootKindStandalone}); err != nil {
		t.Errorf("standalone: %v", err)
	}

	// Child with valid parent — clean.
	child := Root{
		Kind:       RootKindChild,
		ParentPath: parent,
	}
	if err := ValidateParentPointer(child); err != nil {
		t.Errorf("valid child: %v", err)
	}

	// Child with missing parent — error.
	orphan := Root{
		Kind:       RootKindChild,
		ParentPath: filepath.Join(tmp, "does-not-exist"),
	}
	if err := ValidateParentPointer(orphan); !errors.Is(err, ErrParentRootNotFound) {
		t.Errorf("orphan: want ErrParentRootNotFound, got %v", err)
	}
}
