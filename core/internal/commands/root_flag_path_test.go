package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkRoot creates a minimal parlay project at dir.
func mkRoot(t *testing.T, dir string) string {
	t.Helper()
	cfg := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(cfg, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "config.yaml"), []byte("ai_agent: Claude Code\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRootFlagAcceptsAPath covers P2-3: every skill and agent brief in the
// tree writes `--root <root>`, and an agent reasonably puts a path there.
// Requiring a registered child-root name made that form unusable on a
// standalone project, whose roots index is empty — so the documented
// invocation could never work, and the error listed child-root names the
// caller had never heard of.
func TestRootFlagAcceptsAPath(t *testing.T) {
	root := mkRoot(t, t.TempDir())

	res, resolved, err := resolveRootFlagAsPath(root)
	if err != nil {
		t.Fatalf("resolveRootFlagAsPath: %v", err)
	}
	if !resolved {
		t.Fatal("a path to a real parlay project was not accepted")
	}
	got, _ := filepath.EvalSymlinks(res.ActiveRoot.Path)
	want, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Fatalf("resolved to %s, want %s", got, want)
	}
}

// A path that is not a parlay project must not resolve — it falls through
// to the child-root-name lookup, which is what keeps the name form working.
func TestRootFlagRejectsANonProjectPath(t *testing.T) {
	_, resolved, err := resolveRootFlagAsPath(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved {
		t.Fatal("a directory with no .parlay/config.yaml was accepted as a root")
	}
}

// A bare name must never be treated as a path, even when a directory of
// that name exists in cwd: it is ambiguous with a registered child-root
// name, and the name lookup has to get the chance to win.
func TestBareNameIsNotUnambiguouslyAPath(t *testing.T) {
	for _, s := range []string{"core", "studio", "my-root"} {
		if unambiguouslyPath(s) {
			t.Errorf("%q treated as unambiguously a path — it collides with a child-root name", s)
		}
	}
	for _, s := range []string{"./core", "../x", "/abs/path", "~/proj", ".", ".."} {
		if !unambiguouslyPath(s) {
			t.Errorf("%q should be recognized as a path", s)
		}
	}
}

// The ~ form is the one people type by hand; without expansion it would be
// the single place the path form silently failed.
func TestRootFlagExpandsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	got := expandHome("~/x/y")
	if !strings.HasPrefix(got, home) {
		t.Fatalf("expandHome(~/x/y) = %q, want a path under %q", got, home)
	}
	if expandHome("plain") != "plain" {
		t.Fatal("expandHome rewrote a value with no ~ prefix")
	}
}
