// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/complete-directory-archive-with-manifest
// parlay-extends: parlay-tool/root-retirement/cross-cutting/escaping-paths-unreadable-members-fail-closed
//
// The archive engine for root retirement: preserve the complete child
// directory byte for byte — configuration, adapters, and all build state
// included, never a curated subset — with a manifest that names every
// preserved member and covers itself.
//
// Two fail-closed refusals govern the walk, evaluated for every member
// BEFORE any content is copied:
//
//   - Escape: a member whose resolved path lands outside the child
//     directory — a symlink resolving out, or any traversal by another
//     route — aborts the run. Escape is judged on the RESOLVED path
//     (filepath.EvalSymlinks + containment), never on the textual form
//     of the name. Following the escape would preserve content the root
//     does not own; skipping it would produce a copy claiming a
//     completeness it does not have. Neither wrong answer is taken.
//   - Unreadable member: a member that cannot be read aborts the run.
//     There is no partial archive, because a copy silently missing a
//     file nobody could read is indistinguishable from a complete one.
//
// The archive is assembled complete before any other part of the project
// changes: stage into a temporary directory sibling of the destination,
// verify the manifest by reading it back, and promote by rename only
// when complete. A failure at any point leaves the project exactly as it
// was — complete-or-absent is the invariant.

package commands

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// errEscapingMember and errUnreadableMember are the two fail-closed
// refusals of the archive walk. Both wrap a message naming the member
// that caused them.
var (
	errEscapingMember   = fmt.Errorf("member resolves outside the child directory")
	errUnreadableMember = fmt.Errorf("member cannot be read")
)

// memberEntry is one validated member of the child directory: a regular
// file or a symlink whose target resolves inside the directory.
// Directories are represented implicitly by their members; empty
// directories are collected separately so the copy preserves them.
type memberEntry struct {
	// RelPath is the member's path relative to the child directory root,
	// in slash form.
	RelPath string
	// IsSymlink marks a symbolic link member; LinkTarget carries the
	// link's literal (unresolved) target so the copy can recreate it.
	IsSymlink  bool
	LinkTarget string
	// SHA256 is the hex content hash — of the file bytes for a regular
	// file, of the literal link target for a symlink.
	SHA256 string
	Mode   fs.FileMode
}

// ArchiveManifest lists every preserved member with its content hash,
// plus a hash over its own sorted member list — so a reader can
// establish both that the contents are unchanged and that the LIST of
// contents is unchanged.
type ArchiveManifest struct {
	Members []ManifestMember `yaml:"members"`
	// ManifestHash is sha256 over the sorted member list (path and hash
	// of every member, in path order). It covers the manifest itself.
	ManifestHash string `yaml:"manifest-hash"`
}

// ManifestMember is one line of the manifest: a preserved path and the
// sha256 of its content.
type ManifestMember struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

// computeManifestHash derives the self-covering hash from the sorted
// member list. Any change to the set of members or any member's hash
// changes this value.
func computeManifestHash(members []ManifestMember) string {
	h := sha256.New()
	for _, m := range members {
		h.Write([]byte(m.Path))
		h.Write([]byte{0})
		h.Write([]byte(m.SHA256))
		h.Write([]byte{'\n'})
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// WriteManifest writes the manifest to path, computing the self-covering
// hash from the member list. Members are sorted by path first so the
// hash is stable whatever order the walk produced.
func WriteManifest(m *ArchiveManifest, path string) error {
	sort.Slice(m.Members, func(i, j int) bool { return m.Members[i].Path < m.Members[j].Path })
	m.ManifestHash = computeManifestHash(m.Members)
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal archive manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write archive manifest: %w", err)
	}
	return nil
}

// ReadManifest reads a manifest back and verifies its self-covering
// hash against the member list it carries.
func ReadManifest(path string) (*ArchiveManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read archive manifest: %w", err)
	}
	var m ArchiveManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse archive manifest: %w", err)
	}
	if got := computeManifestHash(m.Members); got != m.ManifestHash {
		return nil, fmt.Errorf("verify archive manifest %s: member list does not match its recorded manifest-hash", path)
	}
	return &m, nil
}

// archiveHashFile computes the sha256 of a file's content. A read
// failure is an unreadable-member refusal.
func archiveHashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// hashBytes computes the sha256 of a byte slice (used for symlink
// targets, so links have a manifest entry like any other member).
func hashBytes(b []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// pathWithin reports whether resolved is childRoot itself or inside it.
// Both arguments must already be symlink-resolved absolute paths.
func pathWithin(childRoot, resolved string) bool {
	if resolved == childRoot {
		return true
	}
	return strings.HasPrefix(resolved, childRoot+string(filepath.Separator))
}

// validateArchiveWalk performs the full pre-copy walk of the child
// directory: every member is checked for resolved-path containment and
// readability BEFORE anything is copied, so both refusals abort with
// the project untouched. Returns the validated members (files and
// internal symlinks, hashed) and the empty directories to preserve.
func validateArchiveWalk(childPath string) ([]memberEntry, []string, error) {
	if err := retirementEvent("archive-walk"); err != nil {
		return nil, nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(childPath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve child directory %s: %w", childPath, err)
	}

	var members []memberEntry
	var emptyDirs []string
	walkErr := filepath.WalkDir(resolvedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%w: %s: %v", errUnreadableMember, path, err)
		}
		rel, relErr := filepath.Rel(resolvedRoot, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("%w: %s: %v", errUnreadableMember, relSlash, infoErr)
		}

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			// A symbolic link is followed only to judge escape: its
			// resolved target must land inside the child directory.
			// The judgment is on the resolved path, never the textual
			// form of the link target.
			resolved, evalErr := filepath.EvalSymlinks(path)
			if evalErr != nil {
				// A link that cannot be resolved (dangling, cyclic)
				// cannot be established as internal — cannot tell is
				// treated as escape, not as none.
				return fmt.Errorf("%w: %s: unresolvable symlink: %v", errEscapingMember, relSlash, evalErr)
			}
			if !pathWithin(resolvedRoot, resolved) {
				return fmt.Errorf("%w: %s -> %s", errEscapingMember, relSlash, resolved)
			}
			target, readErr := os.Readlink(path)
			if readErr != nil {
				return fmt.Errorf("%w: %s: %v", errUnreadableMember, relSlash, readErr)
			}
			members = append(members, memberEntry{
				RelPath:    relSlash,
				IsSymlink:  true,
				LinkTarget: target,
				SHA256:     hashBytes([]byte(target)),
				Mode:       info.Mode(),
			})
			if d.IsDir() {
				// An internal directory symlink is preserved as a link;
				// its contents are already covered by the walk of the
				// real directory it points at.
				return filepath.SkipDir
			}
		case d.IsDir():
			// Belt and braces: WalkDir never emits traversal segments,
			// but the containment invariant is on the resolved path of
			// every member, so check it anyway.
			resolved, evalErr := filepath.EvalSymlinks(path)
			if evalErr != nil {
				return fmt.Errorf("%w: %s: %v", errUnreadableMember, relSlash, evalErr)
			}
			if !pathWithin(resolvedRoot, resolved) {
				return fmt.Errorf("%w: %s -> %s", errEscapingMember, relSlash, resolved)
			}
			entries, readErr := os.ReadDir(path)
			if readErr != nil {
				return fmt.Errorf("%w: %s: %v", errUnreadableMember, relSlash, readErr)
			}
			if len(entries) == 0 {
				emptyDirs = append(emptyDirs, relSlash)
			}
		default:
			resolved, evalErr := filepath.EvalSymlinks(path)
			if evalErr != nil {
				return fmt.Errorf("%w: %s: %v", errUnreadableMember, relSlash, evalErr)
			}
			if !pathWithin(resolvedRoot, resolved) {
				return fmt.Errorf("%w: %s -> %s", errEscapingMember, relSlash, resolved)
			}
			sum, hashErr := archiveHashFile(path)
			if hashErr != nil {
				return fmt.Errorf("%w: %s: %v", errUnreadableMember, relSlash, hashErr)
			}
			members = append(members, memberEntry{
				RelPath: relSlash,
				SHA256:  sum,
				Mode:    info.Mode(),
			})
		}
		return nil
	})
	if walkErr != nil {
		return nil, nil, walkErr
	}
	return members, emptyDirs, nil
}

// copyFileByte copies src to dst byte for byte, preserving the mode bits.
func copyFileByte(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// archiveRoot assembles the complete archive of childPath into
// stagingDir: contents/ holds the byte-for-byte copy, manifest.yaml the
// member list with hashes and the self-covering hash. The walk is fully
// validated first (fragment: escaping-paths-unreadable-members-fail-
// closed), then copied, then the manifest is written and verified by
// reading it back. The caller promotes stagingDir to its final
// destination by rename only when this returns nil — the atomicity
// mechanism behind complete-or-absent.
//
// Placeholder baselines and empty build state are copied exactly as they
// are: emptiness is never read as a signal.
func archiveRoot(childPath, stagingDir string) (*ArchiveManifest, error) {
	members, emptyDirs, err := validateArchiveWalk(childPath)
	if err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(childPath)
	if err != nil {
		return nil, fmt.Errorf("resolve child directory %s: %w", childPath, err)
	}

	contentsDir := filepath.Join(stagingDir, "contents")
	if err := os.MkdirAll(contentsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive staging directory: %w", err)
	}
	for _, rel := range emptyDirs {
		if err := os.MkdirAll(filepath.Join(contentsDir, filepath.FromSlash(rel)), 0o755); err != nil {
			return nil, fmt.Errorf("preserve directory %s: %w", rel, err)
		}
	}
	manifest := &ArchiveManifest{}
	for _, m := range members {
		if err := retirementEvent("archive-copy"); err != nil {
			return nil, err
		}
		dst := filepath.Join(contentsDir, filepath.FromSlash(m.RelPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("preserve %s: %w", m.RelPath, err)
		}
		if m.IsSymlink {
			if err := os.Symlink(m.LinkTarget, dst); err != nil {
				return nil, fmt.Errorf("preserve symlink %s: %w", m.RelPath, err)
			}
		} else {
			src := filepath.Join(resolvedRoot, filepath.FromSlash(m.RelPath))
			if err := copyFileByte(src, dst, m.Mode); err != nil {
				return nil, fmt.Errorf("%w: %s: %v", errUnreadableMember, m.RelPath, err)
			}
		}
		manifest.Members = append(manifest.Members, ManifestMember{Path: m.RelPath, SHA256: m.SHA256})
	}

	manifestPath := filepath.Join(stagingDir, "manifest.yaml")
	if err := WriteManifest(manifest, manifestPath); err != nil {
		return nil, err
	}
	// Verify by reading back: the manifest a later integrity check will
	// trust must round-trip before the archive is promoted.
	verified, err := ReadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	if len(verified.Members) != len(manifest.Members) {
		return nil, fmt.Errorf("verify archive manifest: read-back lists %d members, wrote %d", len(verified.Members), len(manifest.Members))
	}
	return verified, nil
}
