package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/config"
	"github.com/spf13/cobra"
)

// makeRoot writes a minimal .parlay/config.yaml to mark a directory as
// a parlay root. When parentRel is set, also writes the parent: pointer.
func makeRoot(t *testing.T, path, parentRel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, config.ParlayDir), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	body := []byte{}
	if parentRel != "" {
		body = []byte("parent: " + parentRel + "\n")
	}
	if err := os.WriteFile(filepath.Join(path, config.ParlayDir, config.ConfigFile), body, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// chdir switches to dir for the duration of the test using t.Chdir,
// which restores cwd automatically and handles cwd in a deleted dir
// (caused by other tests' temp-dir cleanup) gracefully.
func chdir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}

// resetFlags resets package-level flag vars so tests don't leak state.
func resetFlags(t *testing.T) {
	t.Helper()
	prevRoot := rootFlag
	prevVerbose := verboseFlag
	rootFlag = ""
	verboseFlag = false
	t.Cleanup(func() {
		rootFlag = prevRoot
		verboseFlag = prevVerbose
	})
}

// runPreRun invokes persistentPreRun against a fake cobra.Command and
// returns the resulting Context plus stderr capture.
func runPreRun(t *testing.T, cmd *cobra.Command) (*config.Context, string, error) {
	t.Helper()
	if cmd == nil {
		cmd = &cobra.Command{Use: "fake"}
	}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	err := persistentPreRun(cmd, nil)
	return config.FromCtx(cmd.Context()), stderr.String(), err
}

func TestPreRun_StandaloneRoot(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	// Resolve symlinks (macOS /var → /private/var) so comparisons match cwd.
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeRoot(t, tmp, "")
	chdir(t, tmp)

	c, stderr, err := runPreRun(t, nil)
	if err != nil {
		t.Fatalf("PreRun failed: %v", err)
	}
	if c == nil {
		t.Fatal("Context not set")
	}
	if c.Root.Path != tmp {
		t.Errorf("path: want %s, got %s", tmp, c.Root.Path)
	}
	if c.Root.Kind != config.RootKindStandalone {
		t.Errorf("kind: want %q, got %q", config.RootKindStandalone, c.Root.Kind)
	}
	if stderr != "" {
		t.Errorf("verbose disabled but stderr non-empty: %q", stderr)
	}
}

func TestPreRun_VerbosePrintsResolution(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeRoot(t, tmp, "")
	chdir(t, tmp)
	verboseFlag = true

	_, stderr, err := runPreRun(t, nil)
	if err != nil {
		t.Fatalf("PreRun failed: %v", err)
	}
	if !strings.Contains(stderr, "resolved root:") {
		t.Errorf("verbose stderr missing resolution line: %q", stderr)
	}
	if !strings.Contains(stderr, tmp) {
		t.Errorf("verbose stderr missing root path: %q", stderr)
	}
}

func TestPreRun_NoRootFoundErrors(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	// .git boundary so walk-up stops here.
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	chdir(t, tmp)

	_, _, err := runPreRun(t, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPreRun_SkipAnnotation(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	chdir(t, tmp)

	cmd := &cobra.Command{
		Use:         "init",
		Annotations: map[string]string{annotationSkipResolution: "true"},
	}
	c, _, err := runPreRun(t, cmd)
	if err != nil {
		t.Errorf("skip annotation should suppress resolution error, got: %v", err)
	}
	if c != nil {
		t.Errorf("skip annotation should leave Context nil, got %+v", c)
	}
}

func TestPreRun_ParlayRootEnv(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeRoot(t, tmp, "")
	t.Setenv("PARLAY_ROOT", tmp)
	// cwd somewhere unrelated.
	other := t.TempDir()
	chdir(t, other)

	c, _, err := runPreRun(t, nil)
	if err != nil {
		t.Fatalf("PreRun failed: %v", err)
	}
	if c.Resolution.Source != config.SourceParlayRootEnv {
		t.Errorf("source: want %q, got %q", config.SourceParlayRootEnv, c.Resolution.Source)
	}
}

func TestPreRun_RootFlagOverridesIntoChild(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "apps", "web")
	makeRoot(t, parent, "")
	makeRoot(t, child, filepath.Join("..", ".."))

	idx := &config.RootsIndex{
		ParentPath: parent,
		Children:   []config.Root{{Name: "web", RelativePath: filepath.Join("apps", "web")}},
	}
	if err := config.SaveRootsIndex(idx); err != nil {
		t.Fatal(err)
	}

	chdir(t, parent)
	rootFlag = "web"

	c, _, err := runPreRun(t, nil)
	if err != nil {
		t.Fatalf("PreRun: %v", err)
	}
	if c.Root.Kind != config.RootKindChild {
		t.Errorf("kind: want child, got %q", c.Root.Kind)
	}
	// The child's recorded path comes back resolved via the resolver.
	if !strings.HasSuffix(c.Root.Path, filepath.Join("apps", "web")) {
		t.Errorf("path: want suffix apps/web, got %s", c.Root.Path)
	}
	if c.Resolution.Source != config.SourceRootFlag {
		t.Errorf("source: want %q, got %q", config.SourceRootFlag, c.Resolution.Source)
	}
}

func TestPreRun_RootFlagUnknownErrors(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeRoot(t, tmp, "")
	chdir(t, tmp)
	rootFlag = "unknown"

	_, _, err := runPreRun(t, nil)
	if err == nil {
		t.Fatal("expected unknown-root error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("err message should mention 'unknown': %v", err)
	}
}

func TestPreRun_OrphanedChild(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	// Create a child whose parent: pointer points at a non-existent dir.
	child := filepath.Join(tmp, "child")
	makeRoot(t, child, filepath.Join("..", "missing"))
	chdir(t, child)

	_, _, err := runPreRun(t, nil)
	if err == nil {
		t.Fatal("expected ErrParentRootNotFound")
	}
	if !strings.Contains(err.Error(), "parent root not found") {
		t.Errorf("err message: %v", err)
	}
}

func TestPreRun_ForbiddenDirectoryInChild(t *testing.T) {
	resetFlags(t)
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "apps", "web")
	makeRoot(t, parent, "")
	makeRoot(t, child, filepath.Join("..", ".."))
	// Forbidden: child has its own .parlay/schemas/.
	if err := os.MkdirAll(filepath.Join(child, config.ParlayDir, config.SchemasDir), 0755); err != nil {
		t.Fatal(err)
	}
	chdir(t, child)

	_, _, err := runPreRun(t, nil)
	if err == nil {
		t.Fatal("expected forbidden-directory violation")
	}
	if !strings.Contains(err.Error(), "schemas live at the parent root only") {
		t.Errorf("err message: %v", err)
	}
}
