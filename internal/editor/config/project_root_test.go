// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/studio-project-root-resolution
// parlay-artifact: test

// Covers every dialog branch of project-root resolution:
//   - --project flag wins over env and cwd
//   - PARLAY_ROOT wins over cwd
//   - cwd walk-up finds the nearest ancestor with .parlay/
//   - walk-up failure returns studio-config-project-root-not-found
//   - $HOME terminates the walk-up when cwd was inside $HOME at entry
//   - cwd outside $HOME walks all the way to /
//   - explicit --project at a subdirectory is rejected (no walk-up)
//   - explicit --project at a non-existent path is rejected
//   - PARLAY_ROOT at a subdirectory is rejected (strict-root)
//   - relative --project resolves against cwd
//   - successful resolution emits the INFO log line

package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fakeFSStat is the projectRootIO fake. It accepts a set of "directories
// that exist on disk" and a set of "files that exist on disk" — anything
// else returns os.ErrNotExist.
type fakeFSStat struct {
	dirs  map[string]bool
	files map[string]bool
}

func (f fakeFSStat) stat(path string) (os.FileInfo, error) {
	clean := filepath.Clean(path)
	if f.dirs[clean] {
		return rootStat{name: filepath.Base(clean), dir: true}, nil
	}
	if f.files[clean] {
		return rootStat{name: filepath.Base(clean), dir: false}, nil
	}
	return nil, os.ErrNotExist
}

type rootStat struct {
	name string
	dir  bool
}

func (s rootStat) Name() string       { return s.name }
func (s rootStat) Size() int64        { return 0 }
func (s rootStat) Mode() os.FileMode  { return 0 }
func (s rootStat) ModTime() time.Time { return time.Time{} }
func (s rootStat) IsDir() bool        { return s.dir }
func (s rootStat) Sys() any           { return nil }

func mkIO(dirs ...string) projectRootIO {
	f := fakeFSStat{dirs: map[string]bool{}, files: map[string]bool{}}
	for _, d := range dirs {
		f.dirs[filepath.Clean(d)] = true
		// Mark every parent as a directory too so stat(parent) succeeds
		// during the walk-up.
		parent := filepath.Dir(filepath.Clean(d))
		for parent != "/" && parent != "." {
			f.dirs[parent] = true
			parent = filepath.Dir(parent)
		}
		if parent == "/" {
			f.dirs["/"] = true
		}
	}
	return projectRootIO{
		stat: f.stat,
		absPath: func(p string) (string, error) {
			if filepath.IsAbs(p) {
				return p, nil
			}
			return filepath.Clean(p), nil
		},
	}
}

// --- Suite: studio-project-root-resolution ---

func TestProjectFlagHasHighestPrecedence(t *testing.T) {
	io := mkIO("/tmp/has-parlay/.parlay")
	root, src, err := resolveProjectRoot(
		[]string{"--project", "/tmp/has-parlay"},
		map[string]string{"PARLAY_ROOT": "/tmp/other-parlay"},
		"/tmp/yet-another-parlay",
		"/home/dev",
		io,
	)
	if err != nil {
		t.Fatalf("resolveProjectRoot: %v", err)
	}
	if root != "/tmp/has-parlay" {
		t.Fatalf("root = %q, want /tmp/has-parlay", root)
	}
	if src != SourceFlag {
		t.Fatalf("source = %q, want %q", src, SourceFlag)
	}
	if labelForSource(src) != srcLabelFlag {
		t.Fatalf("label = %q, want %q", labelForSource(src), srcLabelFlag)
	}
}

func TestStudioProjectRootUsedWhenNoFlag(t *testing.T) {
	io := mkIO("/tmp/has-parlay/.parlay")
	root, src, err := resolveProjectRoot(
		nil,
		map[string]string{"PARLAY_ROOT": "/tmp/has-parlay"},
		"/tmp/some-other-dir",
		"/home/dev",
		io,
	)
	if err != nil {
		t.Fatalf("resolveProjectRoot: %v", err)
	}
	if root != "/tmp/has-parlay" {
		t.Fatalf("root = %q, want /tmp/has-parlay", root)
	}
	if src != SourceEnv {
		t.Fatalf("source = %q, want %q", src, SourceEnv)
	}
	if labelForSource(src) != srcLabelEnv {
		t.Fatalf("label = %q, want %q", labelForSource(src), srcLabelEnv)
	}
}

func TestWalkUpFindsNearestAncestor(t *testing.T) {
	io := mkIO("/home/dev/myapp/.parlay")
	root, src, err := resolveProjectRoot(
		nil,
		map[string]string{},
		"/home/dev/myapp/some/deeply/nested/subdir",
		"/home/dev",
		io,
	)
	if err != nil {
		t.Fatalf("resolveProjectRoot: %v", err)
	}
	if root != "/home/dev/myapp" {
		t.Fatalf("root = %q, want /home/dev/myapp", root)
	}
	if src != sourceWalkup {
		t.Fatalf("source = %q, want %q", src, sourceWalkup)
	}
	if labelForSource(src) != srcLabelWalkup {
		t.Fatalf("label = %q, want %q", labelForSource(src), srcLabelWalkup)
	}
}

func TestWalkUpFailureProducesNotFound(t *testing.T) {
	io := mkIO() // no .parlay/ dirs anywhere
	_, _, err := resolveProjectRoot(
		nil,
		map[string]string{},
		"/tmp/scratch",
		"/home/dev",
		io,
	)
	if !errors.Is(err, ErrProjectRootNotFound) {
		t.Fatalf("expected %v, got %v", ErrProjectRootNotFound, err)
	}
	// Message names every source tried, in precedence order.
	for _, want := range []string{srcLabelFlag, srcLabelEnv, srcLabelWalkup} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q; got %v", want, err)
		}
	}
}

func TestHomeTerminatesWalkUp(t *testing.T) {
	// cwd inside $HOME, no .parlay/ anywhere → walk stops at $HOME.
	io := mkIO()
	_, _, err := resolveProjectRoot(
		nil,
		map[string]string{},
		"/home/dev/scratch",
		"/home/dev",
		io,
	)
	if !errors.Is(err, ErrProjectRootNotFound) {
		t.Fatalf("expected %v, got %v", ErrProjectRootNotFound, err)
	}
	if !strings.Contains(err.Error(), "/home/dev") {
		t.Fatalf("error should mention walk-stop point /home/dev; got %v", err)
	}
}

func TestCwdOutsideHomeWalksAllTheWay(t *testing.T) {
	// cwd is outside $HOME; walk goes all the way to /.
	io := mkIO("/var/lib/parlay-projects/team-app/.parlay")
	root, src, err := resolveProjectRoot(
		nil,
		map[string]string{},
		"/var/lib/parlay-projects/team-app/some-subdir",
		"/home/dev",
		io,
	)
	if err != nil {
		t.Fatalf("resolveProjectRoot: %v", err)
	}
	if root != "/var/lib/parlay-projects/team-app" {
		t.Fatalf("root = %q, want /var/lib/parlay-projects/team-app", root)
	}
	if src != sourceWalkup {
		t.Fatalf("source = %q, want %q", src, sourceWalkup)
	}
}

func TestExplicitProjectAtSubdirRejected(t *testing.T) {
	// .parlay/ exists at /home/dev/myapp/.parlay/ but --project points at
	// a subdirectory. Strict-root: explicit branches do NOT walk up.
	io := mkIO("/home/dev/myapp/.parlay", "/home/dev/myapp/some/subdir")
	_, _, err := resolveProjectRoot(
		[]string{"--project", "/home/dev/myapp/some/subdir"},
		map[string]string{},
		"/tmp",
		"/home/dev",
		io,
	)
	if !errors.Is(err, ErrProjectRootInvalid) {
		t.Fatalf("expected %v, got %v", ErrProjectRootInvalid, err)
	}
}

func TestExplicitProjectAtNonExistentRejected(t *testing.T) {
	io := mkIO()
	_, _, err := resolveProjectRoot(
		[]string{"--project", "/nonexistent/path"},
		map[string]string{},
		"/tmp",
		"/home/dev",
		io,
	)
	if !errors.Is(err, ErrProjectRootInvalid) {
		t.Fatalf("expected %v, got %v", ErrProjectRootInvalid, err)
	}
}

func TestStudioProjectRootAtSubdirRejected(t *testing.T) {
	io := mkIO("/home/dev/myapp/.parlay", "/home/dev/myapp/some/subdir")
	_, _, err := resolveProjectRoot(
		nil,
		map[string]string{"PARLAY_ROOT": "/home/dev/myapp/some/subdir"},
		"/tmp",
		"/home/dev",
		io,
	)
	if !errors.Is(err, ErrProjectRootInvalid) {
		t.Fatalf("expected %v, got %v", ErrProjectRootInvalid, err)
	}
}

func TestRelativeProjectResolvedAgainstCwd(t *testing.T) {
	io := mkIO("/home/dev/myapp/.parlay")
	root, _, err := resolveProjectRoot(
		[]string{"--project", "./myapp"},
		map[string]string{},
		"/home/dev",
		"/home/dev",
		io,
	)
	if err != nil {
		t.Fatalf("resolveProjectRoot: %v", err)
	}
	if root != "/home/dev/myapp" {
		t.Fatalf("root = %q, want /home/dev/myapp", root)
	}
}

func TestSuccessfulResolutionLoggedWithSource(t *testing.T) {
	var buf bytes.Buffer
	LogResolvedRoot(&buf, "/home/dev/myapp", SourceFlag)
	re := regexp.MustCompile(`project root: /home/dev/myapp \(source: --project flag\)`)
	if !re.MatchString(buf.String()) {
		t.Fatalf("log line missing expected shape %v; got %q", re, buf.String())
	}
}

// cwdInsideHome is exercised through resolveProjectRoot above; add a
// focused micro-test so a regression points right at the function.
func TestCwdInsideHomeBoundaryCases(t *testing.T) {
	cases := []struct {
		cwd, home string
		want      bool
	}{
		{"/home/dev/work", "/home/dev", true},
		{"/home/dev", "/home/dev", true},
		{"/home/developer/work", "/home/dev", false}, // prefix-match false-positive guard
		{"/tmp", "/home/dev", false},
		{"/var/anything", "", false},
	}
	for _, c := range cases {
		got := cwdInsideHome(c.cwd, c.home)
		if got != c.want {
			t.Errorf("cwdInsideHome(%q, %q) = %v, want %v", c.cwd, c.home, got, c.want)
		}
	}
}
