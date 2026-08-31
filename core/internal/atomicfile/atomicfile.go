// parlay-feature: parlay-tool/atomic-file-writes
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

// Package atomicfile is the sole write primitive parlay's deployment paths are
// allowed to use. Every deployed file goes through WriteIfChanged (or
// WriteAtomic); direct os.WriteFile, ioutil.WriteFile, or os.Create calls in the
// deployment packages are forbidden and rejected by TestNoDirectWritePrimitives.
//
// The write pattern is write-temp + fsync + rename. A crash between the temp
// file's creation and the rename leaves the original target intact and a .tmp
// sibling on disk; the .tmp is opened O_TRUNC so the next run overwrites it
// rather than tripping over it. What this buys is that a deployed file is never
// observed half-written — an agent reading a SKILL.md mid-deploy sees the old
// one or the new one, and a truncated skill is not a state a project can reach.
//
// WriteIfChanged adds the content-hash skip: hash the bytes about to be written
// against what is on disk and do nothing when they match. That is what makes a
// second `parlay upgrade` over unchanged sources write zero files, which in turn
// is what makes mtimes mean something — a changed timestamp indicates changed
// content rather than merely a command having been run.
//
// This code was salvaged from the editor tree's deployer, ahead of that tree's
// deletion. It was the better-engineered of the two deployers on exactly this
// point: core's paths did unconditional os.WriteFile,
// rewriting every file on every upgrade. Porting it before the deletion, rather
// than after, is deliberate — the alternative is deleting it and hoping to
// remember.
//
// It lives in its own package rather than inside core/internal/deployer because
// both deployment packages need it and one cannot import the other:
// core/internal/deployer imports core/internal/embedded, and the schema and
// module writers are in embedded. A helper reachable from only one of them would
// leave two thirds of an upgrade's writes unconditional, and the idempotency
// claim is about the whole upgrade.
package atomicfile

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// Renamer abstracts the rename step so tests can inject a failing rename
// without monkey-patching os.Rename. Production code uses DefaultRenamer.
type Renamer func(oldPath, newPath string) error

// DefaultRenamer is the production rename.
func DefaultRenamer(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// WriteIfChanged writes content to path unless the file already holds exactly
// these bytes, and reports whether it wrote.
//
// A read error other than not-exist is returned rather than treated as "differs".
// Writing over a file that cannot be read is how an unrelated permissions
// problem turns into data loss.
func WriteIfChanged(path string, content []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		want := sha256.Sum256(content)
		got := sha256.Sum256(existing)
		if bytes.Equal(want[:], got[:]) {
			return false, nil
		}
	case os.IsNotExist(err):
		// Expected on a first deploy.
	default:
		return false, fmt.Errorf("atomicfile: read %s: %w", path, err)
	}
	if err := WriteAtomic(path, content); err != nil {
		return false, err
	}
	return true, nil
}

// WriteAtomic writes content to path via write-temp + fsync + rename. The temp
// file is path + ".tmp". The parent directory is created if absent (0755 on the
// directory, 0644 on the file).
//
// Failure modes:
//   - mkdir of the parent failed → wrapped error; no .tmp created.
//   - temp create or write failed → temp removed; wrapped error.
//   - fsync failed → temp removed; wrapped error.
//   - rename failed → temp LEFT on disk; the original target is untouched;
//     wrapped error.
func WriteAtomic(path string, content []byte) error {
	return WriteAtomicWith(path, content, DefaultRenamer)
}

// WriteAtomicWith is the renamer-injection variant, for tests that need the
// rename to fail. Production callers use WriteAtomic.
func WriteAtomicWith(path string, content []byte, rename Renamer) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomicfile: mkdir %s: %w", dir, err)
	}
	tmpPath := path + ".tmp"
	// O_TRUNC so a leftover .tmp from a previously crashed run is overwritten
	// cleanly rather than appended to.
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("atomicfile: open temp %s: %w", tmpPath, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomicfile: write temp %s: %w", tmpPath, err)
	}
	// fsync before rename: rename is atomic with respect to the directory
	// entry, not with respect to the data behind it. Without this a crash can
	// leave the new name pointing at a zero-length file, which is worse than
	// either outcome the atomic write is supposed to guarantee.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomicfile: fsync temp %s: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomicfile: close temp %s: %w", tmpPath, err)
	}
	if err := rename(tmpPath, path); err != nil {
		// Intentional: leave the .tmp for the next run to overwrite. The
		// original target is untouched, which is the point.
		return fmt.Errorf("atomicfile: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
