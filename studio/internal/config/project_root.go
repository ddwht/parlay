// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/studio-project-root-resolution

package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Stable error codes for project-root resolution.

// ErrProjectRootNotFound (studio-config-project-root-not-found) — no source
// resolved a project root. The wrapped message names every source that was
// tried, in precedence order, and points at the setup doc.
var ErrProjectRootNotFound = errors.New("studio-config-project-root-not-found")

// ErrProjectRootInvalid (studio-config-project-root-invalid) — an explicit
// override (--project or STUDIO_PROJECT_ROOT) pointed at a path that does
// not exist or that does not directly contain a .parlay/ subdirectory.
// The explicit-override branch never walks up the filesystem.
var ErrProjectRootInvalid = errors.New("studio-config-project-root-invalid")

// SourceFlagProject is the precise label used in operator-facing messages
// for the --project flag. It is not part of the Source enum because it is a
// project-root-specific provenance label, not a config-key source layer.
const (
	srcLabelFlag    = "--project flag"
	srcLabelEnv     = "STUDIO_PROJECT_ROOT"
	srcLabelWalkup  = "cwd-walkup"
)

// ResolveProjectRoot resolves the active project root from the three
// supported sources, in precedence order:
//
//  1. --project <path> CLI flag (relative paths resolved against cwd)
//  2. STUDIO_PROJECT_ROOT environment variable (same relative-path rule)
//  3. cwd walk-up: ancestor whose direct .parlay/ subdirectory exists
//
// Strict-root: branches 1 and 2 MUST point at a directory that DIRECTLY
// contains .parlay/. They never walk up, even when the parent is a real
// parlay project — explicit invocations should be unambiguous.
//
// Walk-up termination:
//   - When cwd is INSIDE home at entry, the walk stops at home (it never
//     crosses into /home or /).
//   - When cwd is OUTSIDE home at entry, the walk proceeds all the way to /.
//
// On success, returns the absolute path, the Source label, and a nil error.
// On failure, returns "", "", and a wrapped stable code.
func ResolveProjectRoot(args []string, env map[string]string, cwd, home string) (string, Source, error) {
	return resolveProjectRoot(args, env, cwd, home, defaultProjectRootIO())
}

// projectRootIO abstracts the two filesystem capabilities ResolveProjectRoot
// needs. Production uses defaultProjectRootIO; tests inject fakes.
type projectRootIO struct {
	stat    func(path string) (os.FileInfo, error)
	absPath func(path string) (string, error)
}

func defaultProjectRootIO() projectRootIO {
	return projectRootIO{
		stat:    os.Stat,
		absPath: filepath.Abs,
	}
}

func resolveProjectRoot(args []string, env map[string]string, cwd, home string, io projectRootIO) (string, Source, error) {
	// --- branch 1: --project flag (strict, no walk-up) ---
	if v, ok := flagValue(args, "project"); ok && v != "" {
		path := resolveRelative(v, cwd)
		if err := validateExplicitRoot(path, srcLabelFlag, io); err != nil {
			return "", "", err
		}
		return path, SourceFlag, nil
	}

	// --- branch 2: STUDIO_PROJECT_ROOT env var (strict, no walk-up) ---
	if v, ok := env["STUDIO_PROJECT_ROOT"]; ok && v != "" {
		path := resolveRelative(v, cwd)
		if err := validateExplicitRoot(path, srcLabelEnv, io); err != nil {
			return "", "", err
		}
		return path, SourceEnv, nil
	}

	// --- branch 3: cwd walk-up ---
	root, stopped := walkUpForParlay(cwd, home, io)
	if root != "" {
		return root, sourceWalkup, nil
	}

	// Failure: name every source that was tried.
	msg := fmt.Sprintf("%s: no project root resolved. Tried (in precedence order): %s, %s, %s",
		ErrProjectRootNotFound.Error(),
		srcLabelFlag,
		srcLabelEnv,
		srcLabelWalkup,
	)
	if stopped != "" {
		msg += fmt.Sprintf(" (walk stopped at %s)", stopped)
	}
	msg += ". See studio/docs/figma-mcp-setup.md"
	return "", "", fmt.Errorf("%w: %s", ErrProjectRootNotFound, msg)
}

// sourceWalkup is the Source label for cwd-walkup resolutions. Defined
// alongside the resolver because the walk-up branch's vocabulary is
// project-root-specific (the loader's Source values cover layered config
// keys; project-root has its own precedence ladder).
const sourceWalkup Source = "cwd-walkup"

// resolveRelative makes path absolute. Relative paths are resolved against
// cwd; absolute paths pass through unchanged.
func resolveRelative(path, cwd string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

// validateExplicitRoot enforces the strict-root invariant for branches 1
// and 2: the path must exist AND must directly contain a .parlay/
// subdirectory. Pointing at a subdirectory of a real project fails; this
// branch never walks up.
func validateExplicitRoot(path, sourceLabel string, io projectRootIO) error {
	info, err := io.stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s pointed at %s but the path does not exist",
				ErrProjectRootInvalid, sourceLabel, path)
		}
		return fmt.Errorf("%w: %s pointed at %s: %v",
			ErrProjectRootInvalid, sourceLabel, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s pointed at %s but the path is not a directory",
			ErrProjectRootInvalid, sourceLabel, path)
	}
	parlayDir := filepath.Join(path, ".parlay")
	parlayInfo, err := io.stat(parlayDir)
	if err != nil || !parlayInfo.IsDir() {
		return fmt.Errorf("%w: %s pointed at %s which does not directly contain .parlay/ (explicit overrides never walk up)",
			ErrProjectRootInvalid, sourceLabel, path)
	}
	return nil
}

// walkUpForParlay walks from cwd toward the filesystem root looking for the
// first ancestor whose direct .parlay/ subdirectory exists. Returns the
// matching ancestor and the empty string for "stopped at"; or returns "" and
// the path the walk terminated at when no ancestor qualifies.
//
// Termination rules:
//   - When cwd was INSIDE home at entry, the walk stops at home (inclusive
//     check on the way up — home itself is the last directory inspected).
//   - When cwd was OUTSIDE home at entry, the walk proceeds to / and the
//     home-terminator does not engage.
func walkUpForParlay(cwd, home string, io projectRootIO) (string, string) {
	if cwd == "" {
		return "", ""
	}
	insideHome := home != "" && cwdInsideHome(cwd, home)
	current := filepath.Clean(cwd)
	for {
		parlay := filepath.Join(current, ".parlay")
		if info, err := io.stat(parlay); err == nil && info.IsDir() {
			return current, ""
		}
		if insideHome && current == filepath.Clean(home) {
			return "", current
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached the filesystem root.
			return "", current
		}
		current = parent
	}
}

// cwdInsideHome reports whether cwd is a descendant of home (or equals
// home). Both arguments are expected to be cleaned absolute paths.
func cwdInsideHome(cwd, home string) bool {
	cwd = filepath.Clean(cwd)
	home = filepath.Clean(home)
	if cwd == home {
		return true
	}
	prefix := home
	if prefix == "/" {
		// Every absolute path is under "/"; only treat as "inside home" when
		// home is a non-root directory.
		return false
	}
	if len(cwd) <= len(prefix) {
		return false
	}
	return cwd[:len(prefix)] == prefix && cwd[len(prefix)] == filepath.Separator
}

// LogResolvedRoot writes the one-line INFO record for a successful project
// root resolution. The line shape is asserted by tests:
//
//	project root: <abs-path> (source: <source-label>)
//
// The source labels come from the srcLabel* constants above.
func LogResolvedRoot(w io.Writer, root string, source Source) {
	if w == nil {
		w = os.Stderr
	}
	label := labelForSource(source)
	fmt.Fprintf(w, "project root: %s (source: %s)\n", root, label)
}

func labelForSource(s Source) string {
	switch s {
	case SourceFlag:
		return srcLabelFlag
	case SourceEnv:
		return srcLabelEnv
	case sourceWalkup:
		return srcLabelWalkup
	default:
		return string(s)
	}
}
