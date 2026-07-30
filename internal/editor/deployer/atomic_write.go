// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

// atomic_write.go is the SOLE write primitive the deployer is allowed to
// use. Every output file goes through writeAtomic; direct os.WriteFile,
// ioutil.WriteFile, or os.Create calls in other deployer files are
// forbidden and rejected by a build-time guardrail test (see
// atomic_write_test.go's TestNoDirectWritePrimitives).
//
// The atomic write pattern is write-temp + fsync + rename: a crash between
// the temp-file creation and the rename leaves the original target intact
// and the .tmp sibling on disk; the next Run cleans up the orphan .tmp
// before any new write.
package deployer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// renamer abstracts the rename step so tests can inject a failing rename
// without monkey-patching os.Rename. Production code uses defaultRenamer.
type renamer func(oldPath, newPath string) error

func defaultRenamer(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// writeAtomic writes content to path via the write-temp + fsync + rename
// pattern. The temp file is path + ".tmp". The directory containing path
// is created via mkdir -p if it does not yet exist (file mode 0755 on the
// directory, 0644 on the file).
//
// Failure modes:
//   - mkdir of the parent directory failed → returns wrapped error;
//     no .tmp file created.
//   - temp file create or write failed → temp file is removed; returns
//     wrapped error.
//   - fsync failed → temp file is removed; returns wrapped error.
//   - rename failed → temp file is LEFT on disk (it will be cleaned up by
//     cleanupOrphanTmpFiles on the next Run); the original target is
//     untouched; returns wrapped error.
func writeAtomic(path string, content []byte) error {
	return writeAtomicWith(path, content, defaultRenamer)
}

// writeAtomicWith is the renamer-injection variant of writeAtomic used by
// the test suite. Production callers use writeAtomic.
func writeAtomicWith(path string, content []byte, rename renamer) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("writeAtomic: mkdir %s: %w", dir, err)
	}
	tmpPath := path + ".tmp"
	// Open with O_CREATE|O_TRUNC|O_WRONLY so a leftover .tmp from a
	// previous crashed run is overwritten cleanly. cleanupOrphanTmpFiles
	// at run start is the primary defense; this is belt-and-suspenders.
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("writeAtomic: open temp %s: %w", tmpPath, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writeAtomic: write temp %s: %w", tmpPath, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writeAtomic: fsync temp %s: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writeAtomic: close temp %s: %w", tmpPath, err)
	}
	if err := rename(tmpPath, path); err != nil {
		// Intentional: leave the .tmp on disk for cleanup-at-next-run.
		// The original target (if any) is untouched.
		return fmt.Errorf("writeAtomic: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// cleanupOrphanTmpFiles removes any *.tmp file under dir whose stripped
// (non-tmp) path is on the provided manifest. .tmp files NOT corresponding
// to manifest paths are left alone — they belong to some other producer
// the deployer does not own.
//
// The cleanup runs before any new write in the per-agent step so the
// previous run's crash debris does not block the new run's writes.
func cleanupOrphanTmpFiles(dir string, manifestPaths map[string]struct{}) error {
	if dir == "" {
		return nil
	}
	// Walk dir; we cannot assume the .tmp file is a direct child because
	// manifest paths live at .claude/skills/parlay-<slug>/SKILL.md (one
	// level deeper than the parent directory passed to this helper).
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".tmp") {
			return nil
		}
		stripped := strings.TrimSuffix(p, ".tmp")
		if _, ok := manifestPaths[stripped]; !ok {
			return nil
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cleanupOrphanTmpFiles: remove %s: %w", p, err)
		}
		return nil
	})
}
