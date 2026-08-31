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
// re-hash every STAGED member and require it to equal what the manifest
// records, verify the manifest by reading it back, and promote by rename
// only when all of that holds. Verification reads the archived bytes —
// the source hashes were taken during the pre-copy walk, so a manifest
// nobody checked against the copy would describe the source rather than
// the archive. A failure at any point leaves the project exactly as it
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

// verifyStagedArchive re-reads the STAGED copy and requires every
// member's bytes to hash to the value the manifest records.
//
// The manifest's hashes are computed on the source during the
// pre-copy walk, which is the only moment escape and readability can
// be judged before anything is written. That leaves a window: the
// source can change between the walk and the copy, and a copy can
// itself go wrong. A manifest describing bytes nobody re-read is a
// claim about the source, not about the archive — and the whole point
// of this operation is that the preserved copy is verifiable. So the
// archived bytes are hashed here, from the staging directory, and any
// disagreement with the manifest aborts the run while the live tree is
// still untouched.
//
// Counting members is not verification and is not what this does: a
// member list of the right length can still hold the wrong bytes.
func verifyStagedArchive(contentsDir string, members []memberEntry) error {
	if err := retirementEvent("verify-archive"); err != nil {
		return err
	}
	listed := make([]ManifestMember, 0, len(members))
	for _, m := range members {
		listed = append(listed, ManifestMember{Path: m.RelPath, SHA256: m.SHA256})
	}
	return verifyArchivedMembers(contentsDir, listed)
}

// verifyArchivedMembers re-reads every member the manifest names from
// the directory holding the archived copy and requires its bytes to
// hash to the recorded value.
//
// It is used at two moments, for the same reason each time. Before an
// archive is promoted, it establishes that the copy matches the
// manifest written beside it. Before a resumed run acts on an archive
// it did not make, it establishes the same thing again — because the
// manifest's self-covering hash proves only that the LIST is internally
// consistent, which a list invented from nothing satisfies perfectly.
// Hashing the members is what makes the manifest a statement about
// files rather than about itself.
func verifyArchivedMembers(contentsDir string, members []ManifestMember) error {
	for _, m := range members {
		archived := filepath.Join(contentsDir, filepath.FromSlash(m.Path))
		info, err := os.Lstat(archived)
		if err != nil {
			return fmt.Errorf("verify archived member %s: %w", m.Path, err)
		}
		var got string
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(archived)
			if err != nil {
				return fmt.Errorf("verify archived symlink %s: %w", m.Path, err)
			}
			got = hashBytes([]byte(target))
		} else {
			sum, err := archiveHashFile(archived)
			if err != nil {
				return fmt.Errorf("verify archived member %s: %w", m.Path, err)
			}
			got = sum
		}
		if got != m.SHA256 {
			return fmt.Errorf("verify archived member %s: the preserved bytes hash to %s but the manifest records %s — the archive does not match what it claims to preserve, so nothing is retired",
				m.Path, got, m.SHA256)
		}
	}
	return nil
}

// manifestDigest is the hash of the manifest FILE's bytes — distinct
// from the manifest's own self-covering hash over its member list.
//
// The self-covering hash travels with the list, so rewriting the list
// and recomputing the hash leaves a manifest that still verifies. This
// digest is recorded somewhere else (in the journal, when the journal
// is written), which is what lets a later run notice that the manifest
// it is reading is not the manifest the run that wrote the journal
// produced.
func manifestDigest(manifestPath string) (string, error) {
	sum, err := archiveHashFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("digest archive manifest %s: %w", manifestPath, err)
	}
	return "sha256:" + sum, nil
}

// archivePreservesLiveContents requires every byte still standing in
// the root's directory to be provably preserved in the archive.
//
// This is the invariant a retirement can actually keep, and it is worth
// being exact about why it is this one. Every artifact in the chain —
// the journal, the manifest, the record, the digest — is writable by
// whoever runs the tool, so no comparison among them can establish that
// they came from a genuine run. Consistency is checkable; AUTHENTICITY
// is not, absent a trust anchor outside the repository. What IS
// checkable is the property that actually matters before a delete: that
// nothing about to be destroyed is being lost.
//
// So while the removal step is outstanding, every live file is hashed
// and required to equal the hash the manifest records for that same
// path. Combined with verifyArchivedMembers — which establishes that the
// ARCHIVED bytes hash to those same values — the chain is
// live == manifest == archived, and the deletion proceeds only over
// content that demonstrably exists in the preserved copy. Comparing
// filenames alone would not do it: an archive holding the right paths
// and the wrong bytes would authorize destroying the real ones.
//
// A mismatch refuses, and that includes the honest case of a file
// legitimately edited while the retirement was interrupted. Refusing is
// the correct answer there, not an inconvenience to work around: the
// archive is stale, the edit is not in it, and completing the run would
// destroy the newer bytes. The remedy is to start the retirement again
// so the archive is taken from what is actually there, and the refusal
// says so.
func archivePreservesLiveContents(childDir string, members []ManifestMember) error {
	archived := make(map[string]string, len(members))
	for _, m := range members {
		archived[m.Path] = m.SHA256
	}
	resolvedRoot, err := filepath.EvalSymlinks(childDir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", childDir, err)
	}
	var unpreserved []string
	note := func(rel, why string) error {
		unpreserved = append(unpreserved, rel+" ("+why+")")
		if len(unpreserved) > 4 {
			return fs.SkipAll
		}
		return nil
	}
	walkErr := filepath.WalkDir(resolvedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		isLink := d.Type()&fs.ModeSymlink != 0
		if d.IsDir() && !isLink {
			return nil
		}
		rel, relErr := filepath.Rel(resolvedRoot, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		want, named := archived[relSlash]
		if !named {
			return note(relSlash, "not named by the archive at all")
		}
		var got string
		if isLink {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				// Unreadable is not preserved-and-verified.
				return note(relSlash, "cannot be read: "+linkErr.Error())
			}
			got = hashBytes([]byte(target))
		} else {
			sum, hashErr := archiveHashFile(path)
			if hashErr != nil {
				return note(relSlash, "cannot be read: "+hashErr.Error())
			}
			got = sum
		}
		if got != want {
			return note(relSlash, "holds different bytes from the archived copy")
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if len(unpreserved) > 0 {
		sort.Strings(unpreserved)
		return fmt.Errorf("the archive does not preserve %s — a retirement destroys the root's contents only once every one of them is provably in the preserved copy, so this run is refused; if the archive is simply out of date, remove it and its journal and run the retirement again against what is actually there",
			strings.Join(unpreserved, ", "))
	}
	return nil
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

	// Verification hashes the ARCHIVED bytes, not the source and not the
	// member count: every staged member is re-read and must hash to what
	// the manifest records, before the manifest is written or the
	// archive promoted.
	if err := verifyStagedArchive(contentsDir, members); err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(stagingDir, "manifest.yaml")
	if err := WriteManifest(manifest, manifestPath); err != nil {
		return nil, err
	}
	// And the manifest a later integrity check will trust must itself
	// round-trip — its self-covering hash re-derived from the member
	// list it carries — before the archive is promoted.
	verified, err := ReadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	if len(verified.Members) != len(manifest.Members) {
		return nil, fmt.Errorf("verify archive manifest: read-back lists %d members, wrote %d", len(verified.Members), len(manifest.Members))
	}
	return verified, nil
}
