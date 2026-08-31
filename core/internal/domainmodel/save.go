// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/compare-and-swap-save

package domainmodel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// ConflictError carries the etag pair from a failed compare-and-swap: the
// token the caller presented and the one the file actually holds now.
//
// It used to be server.ConflictError, defined in the HTTP harness that mapped
// it to a 409 envelope. The harness is gone and the compare-and-swap is not an
// HTTP concept, so the error belongs to the package that raises it. Both etags
// stay on it: a caller that cannot say what it presented and what it collided
// with cannot offer reload-and-reapply.
type ConflictError struct {
	CurrentETag   string
	AttemptedETag string
}

func (e *ConflictError) Error() string { return "conflict" }

// LockBusyError reports that the model lock was still held when the bounded
// wait ran out, so the save never got as far as comparing anything.
//
// It is deliberately NOT a ConflictError. A conflict names two tokens and
// invites the caller to reload and reapply; a lock timeout has no tokens to
// name, because nothing was read. Filling ConflictError's two fields with
// invented values to reuse the type would hand the caller a reload-and-reapply
// affordance built on etags that never described anything on disk. The honest
// answer to this error is to retry the same save, not to rebase it.
type LockBusyError struct {
	// Path is the lock file whose holder never let go.
	Path string
	// Waited is how long the acquisition was given before giving up.
	Waited time.Duration
}

func (e *LockBusyError) Error() string {
	return fmt.Sprintf("domainmodel: model lock %s still held after %s", e.Path, e.Waited)
}

// modelLockWait bounds how long a writer waits for the model lock, and
// modelLockRetry how often it re-attempts within that window. They are
// variables rather than constants so the contention test can collapse the
// wait; nothing outside tests reassigns them.
var (
	modelLockWait  = 5 * time.Second
	modelLockRetry = 25 * time.Millisecond
)

// modelLockName is the lock file, kept beside the project's other tool state
// rather than next to domain-model.yaml so a stray lock file never looks like
// part of the spec.
const modelLockName = "domain-model.lock"

// withModelLock runs fn holding the project's model lock, so the read, the
// etag compare, and the rename that follows them are one indivisible step for
// every writer that takes the lock.
//
// The lock is ADVISORY and coordinates parlay's own writers only. Nothing
// stops a text editor, a script, or a second tool from rewriting
// domain-model.yaml while this lock is held — the operating system does not
// enforce it, and this package does not pretend otherwise. What that costs is
// bounded and worth stating plainly: a non-cooperating writer is still caught
// by the etag compare, because the compare reads the file's current bytes
// rather than anything cached, so a save whose file moved under it fails
// rather than clobbering. The residue is the window between that compare and
// the rename. The lock does not close that window against a writer who never
// took the lock; it closes it against every writer who did, which is the whole
// of parlay. Hand-editors racing each other remain git's problem.
//
// flock is not re-entrant: a second acquisition from the same process on the
// same file blocks on the first. Nothing in this package nests withModelLock,
// and nothing should start.
//
// On os.Root: retire-root routes its mutations through a rooted handle because
// the relative path it deletes comes from user input, and an intermediate
// component could be swapped for a symlink between the containment check and
// the delete. Neither half of that applies here — the model path is the
// configured root joined with two fixed names, so there is no attacker-chosen
// component to swap and no check-then-act gap to widen — and flock opens its
// file by path, so a rooted handle could not cover the lock anyway. Adopting
// os.Root here would be half a defense against a threat this path does not
// have.
func withModelLock(root string, fn func(tx *modelTx) error) error {
	dir := filepath.Join(root, ".parlay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("domainmodel: mkdir %s: %w", dir, err)
	}
	lockPath := filepath.Join(dir, modelLockName)

	lock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), modelLockWait)
	defer cancel()

	locked, err := lock.TryLockContext(ctx, modelLockRetry)
	if err != nil || !locked {
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			return &LockBusyError{Path: lockPath, Waited: modelLockWait}
		}
		return fmt.Errorf("domainmodel: acquire model lock %s: %w", lockPath, err)
	}
	defer func() { _ = lock.Unlock() }()

	return fn(&modelTx{path: resolveModelPath(root)})
}

// modelTx is the handle a withModelLock body works through. It exists only
// while the lock is held, and it is package-internal on purpose: readCurrent
// and saveLocked are safe exactly because of the lock around them, so handing
// either to a caller that could invoke it unlocked would sell the guarantee
// without the thing that provides it.
type modelTx struct {
	path string
}

// readCurrent returns the model file's current bytes and their etag. A file
// that does not exist yet is not an error: it reports no bytes and the empty
// sentinel, which is the same token the load bootstrap hands out, so the
// compare in saveLocked treats "no file" as an ordinary current state.
func (tx *modelTx) readCurrent() ([]byte, Etag, error) {
	current, err := os.ReadFile(tx.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, SentinelEmpty, nil
	case err != nil:
		return nil, "", fmt.Errorf("domainmodel: read model: %w", err)
	}
	return current, computeEtag(current), nil
}

// saveLocked is the compare-and-swap itself, run with the lock held. When the
// presented token matches the current on-disk bytes it writes the file
// atomically; when it does not it writes NOTHING and returns a ConflictError
// carrying both etags. It never merges.
func (tx *modelTx) saveLocked(model Model, presented Etag) (Etag, error) {
	current, currentEtag, err := tx.readCurrent()
	if err != nil {
		return "", err
	}
	if presented != currentEtag {
		// Stale etag — write nothing. Against a missing file this is a save
		// presenting something other than the empty sentinel.
		return "", &ConflictError{
			CurrentETag:   string(currentEtag),
			AttemptedETag: string(presented),
		}
	}
	if current != nil {
		// Carry the deprecated operations block through from the current
		// on-disk file byte-for-byte.
		model.rawOperations = captureOperationsBlock(current)
	}
	return writeModel(tx.path, model)
}

// Save is the compare-and-swap write path. A save must present the token from
// its originating load. When the presented token matches the current on-disk
// bytes, the save writes the file atomically (write-temp + fsync + rename).
// When it does not match — a second writer, a hand-edit, or a CLI
// regeneration changed the file since load — the save writes NOTHING and
// fails with a ConflictError carrying both current and attempted etags, so
// the caller can prompt reload-and-reapply. A save never merges.
//
// The compare and the write happen under the model lock (see withModelLock),
// so two parlay writers cannot both read the same current bytes and both
// write; the second to arrive sees the first's bytes and conflicts. A save
// that cannot get the lock within the bounded wait returns a LockBusyError
// having read and written nothing.
//
// A first save against the empty-model sentinel ("empty") creates the file.
// The deprecated operations block is carried through from the current on-disk
// file unchanged (see Serialize / captureOperationsBlock).
func Save(ctx context.Context, root string, model Model, presented Etag) (Etag, error) {
	var written Etag
	err := withModelLock(root, func(tx *modelTx) error {
		var err error
		written, err = tx.saveLocked(model, presented)
		return err
	})
	if err != nil {
		return "", err
	}
	return written, nil
}

// writeModel serializes the model and writes it atomically, returning the
// etag of the bytes that landed on disk.
func writeModel(path string, model Model) (Etag, error) {
	out, err := Serialize(model)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, out); err != nil {
		return "", err
	}
	return computeEtag(out), nil
}

// writeFileAtomic writes data to path via write-temp + fsync + rename so a
// crash mid-write leaves the original target intact. The parent directory is
// created if it does not yet exist.
//
// The temp file gets a unique name from os.CreateTemp rather than a fixed
// path+".tmp". A fixed name is a second, unlocked write path onto a shared
// filename: two writers land in the same temp file, and the one that renames
// first leaves the other writing into a file that is no longer there — or
// worse, publishing bytes that are a splice of both. Uniqueness makes the
// rename the only step two writers can contend on, and rename is atomic.
//
// The directory is fsynced after the rename. The file's own fsync makes its
// contents durable; only the directory fsync makes the name pointing at those
// contents durable, and without it a crash can leave the rename unrecorded
// with the temp file's data safely on disk under a name nobody looks up.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("domainmodel: mkdir %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".domain-model-*.tmp")
	if err != nil {
		return fmt.Errorf("domainmodel: create temp: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("domainmodel: write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("domainmodel: fsync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("domainmodel: close temp: %w", err)
	}
	// os.CreateTemp makes the file 0o600; the model file has always been
	// world-readable, and the mode has to be right before the rename so no
	// reader ever sees the target under the stricter one.
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("domainmodel: chmod temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("domainmodel: rename temp: %w", err)
	}
	// Past this point the new bytes ARE the model file: the error below says
	// the rename could not be confirmed durable, never that it did not happen.
	if err := fsyncDir(dir); err != nil {
		return fmt.Errorf("domainmodel: fsync dir %s after rename (the model file was replaced; only its durability is unconfirmed): %w", dir, err)
	}
	return nil
}

// fsyncDir flushes a directory's own metadata so a rename into it survives a
// crash.
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
