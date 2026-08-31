// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/compare-and-swap-save

package domainmodel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// Save is the compare-and-swap write path. A save must present the token from
// its originating load. When the presented token matches the current on-disk
// bytes, the save writes the file atomically (write-temp + fsync + rename).
// When it does not match — a second writer, a hand-edit, or a CLI
// regeneration changed the file since load — the save writes NOTHING and
// fails with a ConflictError carrying both current and attempted etags, so
// the caller can prompt reload-and-reapply. A save never merges.
//
// A first save against the empty-model sentinel ("empty") creates the file.
// The deprecated operations block is carried through from the current on-disk
// file unchanged (see Serialize / captureOperationsBlock).
func Save(ctx context.Context, root string, model Model, presented Etag) (Etag, error) {
	path := resolveModelPath(root)
	current, err := os.ReadFile(path)

	switch {
	case errors.Is(err, os.ErrNotExist):
		// No file yet. Only a save presenting the empty sentinel may create
		// it; any other presented token is a stale-load conflict.
		if presented != SentinelEmpty {
			return "", &ConflictError{
				CurrentETag:   string(SentinelEmpty),
				AttemptedETag: string(presented),
			}
		}
		return writeModel(path, model)

	case err != nil:
		return "", fmt.Errorf("domainmodel: read model: %w", err)
	}

	// File exists: compare presented token against current on-disk bytes.
	currentEtag := computeEtag(current)
	if presented != currentEtag {
		// Stale etag — write nothing.
		return "", &ConflictError{
			CurrentETag:   string(currentEtag),
			AttemptedETag: string(presented),
		}
	}

	// Carry the deprecated operations block through from the current on-disk
	// file byte-for-byte.
	model.rawOperations = captureOperationsBlock(current)
	return writeModel(path, model)
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
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("domainmodel: mkdir %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("domainmodel: create temp: %w", err)
	}
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
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("domainmodel: rename temp: %w", err)
	}
	return nil
}
