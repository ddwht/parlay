// parlay-section: cross-cutting

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ErrNoRootFound is returned when neither PARLAY_ROOT nor walk-up finds
// a usable root.
var ErrNoRootFound = errors.New("no parlay root found")

// ErrParlayRootInvalid is returned when PARLAY_ROOT is set but does not
// point at a directory containing .parlay/.
var ErrParlayRootInvalid = errors.New("PARLAY_ROOT is set but does not contain .parlay/")

// ErrParentRootNotFound is returned when a child root's parent pointer
// cannot be resolved.
var ErrParentRootNotFound = errors.New("parent root not found")

// ResolveActiveRoot picks the active root for an invocation. Resolution
// order:
//  1. PARLAY_ROOT env var (must be absolute and contain .parlay/);
//  2. walk upward from cwd to the first .parlay/, stopping at a .git/
//     boundary or the filesystem root.
//
// Disambiguation (when no .parlay/ is found at cwd but candidate roots
// exist below or in a parent's roots.yaml) is layered above this in the
// cobra entry point — this function returns ErrNoRootFound on miss.
//
// On success, the returned ResolutionResult's ActiveRoot has Kind populated
// from the on-disk config (parent / child / standalone).
func ResolveActiveRoot(cwd string, env map[string]string) (*ResolutionResult, error) {
	if env == nil {
		env = map[string]string{}
	}

	if envPath, ok := env["PARLAY_ROOT"]; ok && envPath != "" {
		if !filepath.IsAbs(envPath) {
			return nil, fmt.Errorf("%w: not an absolute path: %s", ErrParlayRootInvalid, envPath)
		}
		if !hasParlayDir(envPath) {
			return nil, fmt.Errorf("%w: %s", ErrParlayRootInvalid, envPath)
		}
		return makeResult(envPath, SourceParlayRootEnv), nil
	}

	rootPath, ok := walkUp(cwd)
	if !ok {
		return nil, ErrNoRootFound
	}
	return makeResult(rootPath, SourceCwdWalkUp), nil
}

// makeResult assembles a ResolutionResult, classifying the root by
// reading its on-disk config (parent: pointer present → child).
func makeResult(rootPath string, source ResolutionSource) *ResolutionResult {
	root := Root{
		Name: filepath.Base(rootPath),
		Path: rootPath,
		Kind: RootKindStandalone,
	}
	if parent, err := readParentPointer(rootPath); err == nil && parent != "" {
		root.ParentPath = parent
		root.Kind = RootKindChild
		if rel, err := filepath.Rel(parent, rootPath); err == nil {
			root.RelativePath = rel
		}
	}
	return &ResolutionResult{
		ActiveRoot:           root,
		Source:               source,
		AnnouncementRequired: false,
	}
}

// hasParlayDir returns true iff path/.parlay/ is a directory.
func hasParlayDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ParlayDir))
	return err == nil && info.IsDir()
}

// walkUp walks dir upward, looking for the first directory that contains
// .parlay/. Stops at a .git/ boundary or the filesystem root. Returns
// the found root path and true on hit; ("", false) on miss.
func walkUp(dir string) (string, bool) {
	cur := dir
	for {
		if hasParlayDir(cur) {
			return cur, true
		}
		// Stop at .git boundary (do NOT cross into a parent repo).
		if info, err := os.Stat(filepath.Join(cur, ".git")); err == nil && info.IsDir() {
			return "", false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// ApplyRootFlagToResolution implements the --root flag override: when set
// it must agree with any prefix on parsed; mismatches return an error.
// Returns the chosen root name (empty string when neither flag nor prefix
// is set), or an error on conflict.
func ApplyRootFlagToResolution(flag, prefix string) (string, error) {
	if flag == "" {
		return prefix, nil
	}
	if prefix != "" && prefix != flag {
		return "", fmt.Errorf("--root %s disagrees with prefix %s in feature reference", flag, prefix)
	}
	return flag, nil
}

// SearchChildrenForFeature returns the registered children whose disk
// layout contains the named feature slug under spec/intents/.
func SearchChildrenForFeature(parent string, idx *RootsIndex, slug string) []Root {
	if idx == nil {
		return nil
	}
	var out []Root
	for _, child := range idx.Children {
		path := child.Path
		if path == "" {
			path = filepath.Join(parent, child.RelativePath)
		}
		featurePath := filepath.Join(path, SpecDir, IntentsDir, slug)
		if _, err := os.Stat(featurePath); err == nil {
			out = append(out, child)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CandidatesFromIndex converts a roots index into the candidate-root
// shape used by the disambiguation prompt.
func CandidatesFromIndex(idx *RootsIndex, reason CandidateReason) []Candidate {
	if idx == nil {
		return nil
	}
	out := make([]Candidate, 0, len(idx.Children))
	for _, c := range idx.Children {
		out = append(out, Candidate{
			Name:         c.Name,
			RelativePath: c.RelativePath,
			Reason:       reason,
		})
	}
	return out
}

// DiscoverRootsBelow walks the directory tree under cwd looking for
// .parlay/ markers that aren't on the walk-up path. Returns at most one
// candidate per discovered root; a child of an already-discovered root
// is skipped. Used when walk-up fails but the user might still mean a
// child somewhere below cwd.
func DiscoverRootsBelow(cwd string, maxDepth int) []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Skip dot dirs like .git, .claude, but allow .parlay detection below.
			if len(name) > 0 && name[0] == '.' {
				continue
			}
			child := filepath.Join(dir, name)
			if hasParlayDir(child) && !seen[child] {
				rel, _ := filepath.Rel(cwd, child)
				seen[child] = true
				out = append(out, Candidate{
					Name:         filepath.Base(child),
					RelativePath: rel,
					Reason:       ReasonDiscoveredBelowCwd,
				})
				// Don't recurse into a discovered root.
				continue
			}
			walk(child, depth+1)
		}
	}
	walk(cwd, 0)
	return out
}

// ValidateParentPointer verifies that a child root's recorded parent path
// resolves to a valid parlay root (a directory with .parlay/ and no
// parent: of its own). Returns ErrParentRootNotFound wrapped with the
// missing path on failure.
func ValidateParentPointer(child Root) error {
	if child.Kind != RootKindChild || child.ParentPath == "" {
		return nil
	}
	if !hasParlayDir(child.ParentPath) {
		return fmt.Errorf("%w at %s", ErrParentRootNotFound, child.ParentPath)
	}
	// Refuse if the parent itself has a parent: pointer (no nesting).
	if grand, err := readParentPointer(child.ParentPath); err == nil && grand != "" {
		return fmt.Errorf("%w: parent %s itself has a parent pointer; nested children are not supported",
			ErrParentRootNotFound, child.ParentPath)
	}
	return nil
}
