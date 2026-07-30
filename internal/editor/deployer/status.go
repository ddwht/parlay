// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

// Package deployer owns Parlay Studio's init and upgrade subcommands. The
// deployer fans the embedded source surface (studio/internal/embedded) out
// to a target parlay project's agent surfaces (Claude Code via .claude/,
// Cursor via .cursor/, Generic CLI via .parlay/cli/).
//
// status.go declares the feature-stable four-status enum that external
// tooling parsing parlay-studio's stdout MAY rely on, and the PrintSummary
// helper that emits one line per file in manifest order.
package deployer

import (
	"errors"
	"fmt"
	"io"
)

// FileStatus is the per-file outcome of a deployer run. The four values
// are feature-stable — adding a fifth status is a breaking change to the
// deployer's stdout contract.
type FileStatus int

const (
	// StatusWritten — the deployer wrote (or overwrote) the file because
	// the on-disk content hash did not match the embedded source hash.
	StatusWritten FileStatus = iota
	// StatusUnchanged — the on-disk content hash matched the embedded
	// source hash; no write was performed.
	StatusUnchanged
	// StatusOrphan — the file matches the parlay-* naming convention but
	// is NOT on the current manifest. The deployer leaves it on disk; the
	// operator decides whether to remove it.
	StatusOrphan
	// StatusFailed — the deployer attempted a write but the atomic-write
	// helper returned an error. The original file (if any) is intact;
	// the .tmp sibling will be cleaned up at the next run.
	StatusFailed
)

// String returns the lowercase enum label printed in stdout summaries.
// External tooling that parses parlay-studio's stdout matches against
// these literals.
func (s FileStatus) String() string {
	switch s {
	case StatusWritten:
		return "written"
	case StatusUnchanged:
		return "unchanged"
	case StatusOrphan:
		return "orphan"
	case StatusFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown-status-%d", int(s))
	}
}

// FileStatusEntry pairs a target path with its outcome. The Source field
// names the embedded skill slug for written/unchanged entries; for orphan
// entries it is empty (or carries a prior-version label when discoverable
// from the file's header). For failed entries, Err is populated with the
// underlying error.
type FileStatusEntry struct {
	Path   string
	Status FileStatus
	Source string
	Err    error
}

// PrintSummary writes one line per entry to w in manifest order followed
// by the orphan entries. The line shape is tooling-parseable:
//
//	<status>: <path> (source: <slug>)
//
// Failed entries append the error text after the source clause:
//
//	failed: <path> (source: <slug>) — <error>
//
// An empty entries slice produces a single "no files to report" line.
func PrintSummary(w io.Writer, entries []FileStatusEntry) error {
	if w == nil {
		return errors.New("deployer.PrintSummary: writer is nil")
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, "no files to report")
		return err
	}
	for _, e := range entries {
		line := fmt.Sprintf("%s: %s (source: %s)", e.Status.String(), e.Path, e.Source)
		if e.Status == StatusFailed && e.Err != nil {
			line += fmt.Sprintf(" — %v", e.Err)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
