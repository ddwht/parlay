// Package ledger guards the read-modify-write of a single append-only
// file.
//
// WHY THIS EXISTS. `atomicfile` prevents torn files and nothing more:
// WriteAtomic is fixed-`.tmp` plus fsync plus rename, and WriteIfChanged
// hashes only to skip an identical write. Neither holds anything across
// the read/decide/write sequence, so two processes appending an event to
// the same file both load the same bytes and the second silently discards
// the first's event. Measured, that is not a marginal loss: twenty
// concurrent append-one-event writers through atomicfile.WriteAtomic land
// ONE of twenty events. The failure corrupts nothing, which is what makes
// it dangerous — the file still parses, every field is well-formed, and
// nineteen records are gone with nothing reporting it.
//
// WHAT THIS IS. The pattern `core/internal/domainmodel` already runs,
// lifted to any path: an advisory flock acquired BEFORE the read and held
// through the directory fsync, plus a uniquely-named O_EXCL temp so
// concurrent writers to one target never share a temp name.
//
// THE STORE OWNS ALL I/O. There is no transaction handle to hold. An
// earlier cut passed a *Tx into the callback, which read well and did not
// work: a caller could capture that pointer and read or write through it
// after the callback returned, entirely unlocked. A guarantee the API
// merely asks for is not a guarantee. So the callback receives BYTES and
// returns BYTES, and every file operation happens inside this package
// with the lock provably held.
//
// Deliberately NOT an etag compare-and-swap, which domainmodel also has.
// An etag earns its place when a stale object crosses a boundary and
// comes back — an HTTP client presenting a version it fetched a request
// ago. Update's read-modify-write happens wholly inside one lock in one
// short-lived command, so on the supported mutation path nothing stale
// can cross anything and a presented-etag parameter would have no failure
// left to catch.
//
// That claim is about Update, and only Update. Read exists because
// inventory listings need it, and its result is a SNAPSHOT: informational,
// already potentially stale by the time the caller looks at it, and never
// a basis for a write. A caller that Reads, keeps the bytes, then calls
// Update and ignores the `current` argument has reintroduced exactly the
// lost update this package prevents — the lock cannot stop that, because
// the staleness was created outside it. Republish only what the callback
// hands you.
package ledger

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// Bounded wait, same shape and reasoning as domainmodel's: a writer that
// blocks forever on a lock turns a stuck process into a stuck project,
// and a subsystem whose whole job is "never lose a record" must not
// acquire the power to hang the command that calls it.
//
// Vars rather than consts so a contention test can pin the typed error
// contract without spending the full wait — domainmodel does the same, for
// the same reason.
var (
	lockWait  = 5 * time.Second
	lockRetry = 25 * time.Millisecond
)

// Test seams for the two publish steps that cannot be provoked from
// outside. Production leaves them alone.
var (
	publishLink   = os.Link
	publishRename = os.Rename
	removeFile    = os.Remove
	syncFile      = func(f *os.File) error { return f.Sync() }
	syncDir       = fsyncDir
)

// BusyError reports that the lock was still held when the bounded wait
// ran out. A distinct type because the caller's right response differs
// from every other failure here: this one is worth retrying, and the
// others are not.
type BusyError struct {
	Path   string
	Waited time.Duration
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("ledger: %s is being written by another process (waited %s)", e.Path, e.Waited)
}

// Store guards exactly one target file.
//
// Lock scope is that single file, which is what keeps unrelated records
// from contending: two features' activity declarations, or two backlog
// items, never wait on each other. A single project-wide lock would
// serialise every write in the project behind whichever one is slowest,
// and would do it invisibly.
type Store struct {
	// target is the RESOLVED absolute path, and it is the same path the
	// lock name was derived from. Holding one identity for locking and a
	// different one for I/O is how a lock ends up guarding a file nobody
	// is writing: New used to hash the absolute path while storing the
	// caller's relative one, so a cwd change between New and the write
	// moved the I/O and left the lock behind.
	target   string
	lockPath string
}

// New returns a Store for target, with its lock under rootPath/.parlay/locks/.
//
// The lock lives in `.parlay/` rather than beside the target because a
// lock is tool machinery, not design intent, and `spec/` is a zone a
// person reads and commits. A `.lock` file appearing next to a designer's
// activity.yaml would be committed by someone eventually, and the zone
// rules exist precisely to stop that. domainmodel sets the same
// precedent with `.parlay/<name>.lock`.
//
// The target is canonicalised before it is hashed: absolute, with any
// symlinks in its PARENT resolved. Two lexical spellings of one real file
// must not acquire two different locks, which would leave both writers
// believing they held it. The parent rather than the file itself, because
// the file legitimately may not exist yet.
func New(rootPath, target string) *Store {
	resolved := canonicalise(target)
	sum := sha256.Sum256([]byte(resolved))
	return &Store{
		target:   resolved,
		lockPath: filepath.Join(rootPath, ".parlay", "locks", hex.EncodeToString(sum[:16])+".lock"),
	}
}

// canonicalise resolves target to an absolute path with symlinks in its
// parent directory expanded.
//
// Resolution happens ONCE, at construction. A caller that later replaces
// an unresolved parent directory with a symlink to somewhere else can
// therefore derive a different lock from the same spelling, because the
// first Store resolved against a directory that no longer means what it
// did. Parlay's paths are fixed project directories, so this does not
// arise in practice; it is a precondition rather than a defended case.
//
// Every failure degrades to the best path so far rather than erroring: a
// Store that cannot be built is worse than one whose lock identity is
// merely lexical, and the degraded case is exactly what the old code did
// unconditionally.
func canonicalise(target string) string {
	abs, err := filepath.Abs(target)
	if err != nil {
		return target
	}
	parent, base := filepath.Split(abs)
	realParent, err := filepath.EvalSymlinks(filepath.Clean(parent))
	if err != nil {
		return abs
	}
	return filepath.Join(realParent, base)
}

// Target is the resolved absolute path this Store guards.
func (s *Store) Target() string { return s.target }

// Read returns the target's current bytes under the lock.
//
// A missing file is not an error: an activity declaration that has never
// been written is the undeclared state, and forcing every caller to
// distinguish os.IsNotExist from a real read failure is how one of them
// eventually treats a permissions error as "no file yet" and writes over
// a file it could not read.
func (s *Store) Read() (data []byte, exists bool, err error) {
	err = s.withLock(func() error {
		data, exists, err = readFile(s.target)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return data, exists, nil
}

// Update runs fn under the lock and publishes what it returns.
//
// fn receives the target's current bytes and whether it exists, and
// returns the bytes to write. Returning write=false leaves the file
// untouched — the honest way to say "having read it, there is nothing to
// change" without writing identical bytes and touching the mtime.
//
// fn must not perform its own file I/O on the target. It cannot: it is
// handed bytes and returns bytes, and no handle to the file escapes this
// package.
func (s *Store) Update(fn func(current []byte, exists bool) (next []byte, write bool, err error)) error {
	return s.withLock(func() error {
		current, exists, err := readFile(s.target)
		if err != nil {
			return err
		}
		next, write, err := fn(current, exists)
		if err != nil || !write {
			return err
		}
		return replaceFile(s.target, next)
	})
}

// Create publishes data only if the target does not already exist,
// returning an error wrapping os.ErrExist otherwise.
//
// Crash-atomic, which O_EXCL on the final path is not. Opening the
// canonical path and writing into it directly means a crash — or a failed
// write, or a failed fsync — leaves a truncated record at the name a
// reader trusts, and every retry afterwards sees ErrExist and refuses to
// repair it. So the bytes are written and synced to a temp first and
// published by hard link, which creates the name atomically and fails
// rather than replacing if somebody won the race.
//
// New-record creation uses this rather than Update even though ids are
// collision-safe by construction. A probable-unique id is a statement
// about how often collisions happen, not a guarantee that a collision
// cannot silently overwrite somebody's record — and the difference
// matters exactly once, on the occasion nobody is watching.
func (s *Store) Create(data []byte) error {
	return s.withLock(func() error {
		dir := filepath.Dir(s.target)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ledger: mkdir %s: %w", dir, err)
		}
		tmp, err := writeTemp(dir, data)
		if err != nil {
			return err
		}
		// Best-effort cleanup for the failure paths only. The success
		// path removes the temp explicitly below and clears this, because
		// an unchecked deferred remove would report success while leaving
		// a .ledger-*.tmp in a directory a person reads — and would leave
		// it unfsynced, so a crash could resurrect the entry after the
		// call returned cleanly.
		cleaned := false
		defer func() {
			if !cleaned {
				_ = removeFile(tmp)
			}
		}()

		if err := publishLink(tmp, s.target); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("ledger: %s already exists: %w", s.target, os.ErrExist)
			}
			// A filesystem without hard links cannot publish a name
			// atomically. Say so rather than falling back to a direct
			// O_EXCL write, which would quietly drop the crash guarantee
			// this package otherwise makes.
			return fmt.Errorf("ledger: publish %s by link (the filesystem may not support hard links): %w", s.target, err)
		}
		if err := removeFile(tmp); err != nil {
			// The record IS published; only the temp survived. Say both
			// halves, because "create failed" would be false and would
			// send a caller to retry something already done.
			return fmt.Errorf("ledger: %s was published, but its temp %s could not be removed: %w", s.target, tmp, err)
		}
		cleaned = true
		// One fsync AFTER both metadata operations, so the link and the
		// unlink are made durable together.
		//
		// Past the link the record EXISTS. A raw error here would read as
		// "creation failed" and send the caller to retry straight into
		// ErrExist — the same ambiguity the removal branch above avoids,
		// and the one replaceFile already spells out on its own rename.
		if err := syncDir(dir); err != nil {
			return fmt.Errorf("ledger: %s was published and its temp removed, but the directory could not be fsynced, so only durability is unconfirmed — do not retry the create: %w", s.target, err)
		}
		return nil
	})
}

// withLock acquires the advisory lock, runs fn, and releases.
//
// Not re-entrant: flock blocks a second acquisition from the same
// process, so a nested call deadlocks until the wait expires and then
// reports BusyError.
func (s *Store) withLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.lockPath), 0o755); err != nil {
		return fmt.Errorf("ledger: mkdir lock dir: %w", err)
	}

	lock := flock.New(s.lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), lockWait)
	defer cancel()

	locked, err := lock.TryLockContext(ctx, lockRetry)
	if err != nil || !locked {
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			return &BusyError{Path: s.target, Waited: lockWait}
		}
		return fmt.Errorf("ledger: acquire lock %s: %w", s.lockPath, err)
	}
	defer func() { _ = lock.Unlock() }()

	return fn()
}

func readFile(path string) (data []byte, exists bool, err error) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		return b, true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("ledger: read %s: %w", path, err)
	}
}

// replaceFile swaps the target's contents atomically: unique O_EXCL temp,
// fsync, rename, fsync of the directory.
func replaceFile(target string, data []byte) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ledger: mkdir %s: %w", dir, err)
	}
	tmp, err := writeTemp(dir, data)
	if err != nil {
		return err
	}
	if err := publishRename(tmp, target); err != nil {
		// Rename did not happen, so any existing target is untouched.
		// The temp removal is best-effort: if it also fails the residue
		// is named in the error rather than left for the caller to
		// discover.
		if rmErr := removeFile(tmp); rmErr != nil {
			return fmt.Errorf("ledger: rename temp onto %s failed (%w) and the temp %s could not be removed either: %v", target, err, tmp, rmErr)
		}
		return fmt.Errorf("ledger: rename temp onto %s: %w", target, err)
	}
	// Past this point the new bytes ARE the file. The error below says
	// the rename could not be confirmed durable, never that it did not
	// happen.
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("ledger: fsync %s after rename (the file was replaced; only its durability is unconfirmed): %w", dir, err)
	}
	return nil
}

// writeTemp writes data to a uniquely-named, fsynced temp file in dir and
// returns its path.
//
// On failure it tries to remove the temp and, when that also fails, names
// the survivor in the returned error. Cleanup is best-effort by nature —
// a filesystem that just refused a write may equally refuse an unlink —
// so the guarantee offered is that a residue is always NAMED, never that
// it cannot happen. A caller reading ".ledger-*.tmp" in an error has
// somewhere to start; a caller reading a swallowed one does not.
func writeTemp(dir string, data []byte) (string, error) {
	f, tmp, err := createExclusiveTemp(dir)
	if err != nil {
		return "", fmt.Errorf("ledger: create temp in %s: %w", dir, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", cleanupTemp(tmp, fmt.Errorf("ledger: write temp: %w", err))
	}
	if err := syncFile(f); err != nil {
		_ = f.Close()
		return "", cleanupTemp(tmp, fmt.Errorf("ledger: fsync temp: %w", err))
	}
	if err := f.Close(); err != nil {
		return "", cleanupTemp(tmp, fmt.Errorf("ledger: close temp: %w", err))
	}
	return tmp, nil
}

// cleanupTemp removes a temp after a failed write and folds any removal
// failure into the error being returned, so the stranded path is named
// exactly once and never silently.
func cleanupTemp(tmp string, cause error) error {
	if err := removeFile(tmp); err != nil {
		return fmt.Errorf("%w (and the temp %s could not be removed either: %v)", cause, tmp, err)
	}
	return cause
}

// createExclusiveTemp opens a uniquely-named temp file in dir with
// O_EXCL, retrying on the astronomically unlikely name collision. Unique
// rather than fixed: a shared temp name is a second collision point on
// the same target, and it is the one atomicfile has.
func createExclusiveTemp(dir string) (*os.File, string, error) {
	for range 10 {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, "", err
		}
		tmp := filepath.Join(dir, fmt.Sprintf(".ledger-%x.tmp", suffix))
		f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, tmp, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not find a free temp name in %s", dir)
}

// fsyncDir flushes a directory's own metadata so a rename or link into it
// survives a crash.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}
