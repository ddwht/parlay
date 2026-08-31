// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/mutation-order-rollback-resumable-journal
//
// The resumable journal for root retirement, and the ordering of the
// retirement's mutations around it.
//
// The order: the archive and the retirement record are complete before
// the project's registration of the root changes, and the registration
// changes last. A failure at any point either restores exactly the
// prior state (pre-archive failures: the staged archive directory is
// removed and nothing else was written) or leaves an explicit resumable
// journal naming the outstanding steps, in the order they must happen.
//
// The journal lives at <parent>/.parlay/retired/<root>.journal.yaml — a
// SIBLING of the destination directory, so its presence never collides
// with the destination-must-not-exist precondition of a fresh run. It
// exists only between a post-archive failure and a resumed completion.
//
// It is also found by SCANNING that location, before the registration is
// consulted — see FindRetirementJournal. The terminal step removes the
// directory, then the registration, then the journal, so the state a
// resume most needs to reach is one the registration can no longer
// describe. Resolving the target through the registration first would
// make that state unreachable, which is the difference between a journal
// that records an interruption and one that can finish it.
//
// The step vocabulary is closed at two: write-record and deregister-root
// (decision: journal-vocabulary-owns-directory-removal).
// The archive is complete before any journal exists, by the ordering
// above, so no journal ever names an archive step. deregister-root is
// the terminal step and owns the invariant pair the testcases pin: the
// root's directory is removed BEFORE the registration stops naming it
// (so no interruption leaves the registration missing the root while
// its contents are still in place), and the journal is removed in the
// same final step (so whenever the contents have moved and the root is
// still registered, the journal exists).
//
// Every part of that step is idempotent — a missing directory is skipped,
// an already-deregistered root is skipped, an absent journal file is
// tolerated — so a resumed run completes whatever remains however much of
// the step already succeeded. That is what makes the registration removal
// the last mutation whose failure matters: the only thing that can fail
// after it is the journal removal, and re-running finishes exactly that.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
)

// Journal step vocabulary — closed at two members.
const (
	journalStepWriteRecord    = "write-record"
	journalStepDeregisterRoot = "deregister-root"
)

// RetirementJournal is the on-disk record of a part-finished retirement:
// the root concerned and the outstanding steps in the order they must
// happen, in terms a person can act on and a resumed run can execute.
type RetirementJournal struct {
	// Root is the registered name of the root being retired.
	Root string `yaml:"root"`
	// RelativePath is the root's registered path relative to the parent,
	// kept so the deregister-root step can find the directory to remove
	// even though a resumed run must not re-read the (already archived)
	// contents.
	RelativePath string `yaml:"relative-path"`
	// Outstanding lists the steps not yet completed, in execution order.
	// Closed vocabulary: write-record, deregister-root.
	Outstanding []string `yaml:"outstanding"`
}

// retiredRootsDir is the parent-level directory holding every retired
// root's archive. It sits under the .parlay dot-directory, which every
// discovery walk already skips — invisibility to discovery is
// structural.
func retiredRootsDir(parentPath string) string {
	return filepath.Join(parentPath, config.ParlayDir, "retired")
}

// retirementDestination is where a retired root's archive lives.
func retirementDestination(parentPath, rootName string) string {
	return filepath.Join(retiredRootsDir(parentPath), rootName)
}

// retirementJournalPath is the journal's location — sibling of the
// destination directory, never inside it.
func retirementJournalPath(parentPath, rootName string) string {
	return filepath.Join(retiredRootsDir(parentPath), rootName+".journal.yaml")
}

// LoadRetirementJournal reads the journal for the named root. Returns
// (nil, nil) when no journal exists — the normal state. Any other read
// or parse failure is an error: a journal that cannot be read cannot be
// distinguished from one that names outstanding steps.
func LoadRetirementJournal(parentPath, rootName string) (*RetirementJournal, error) {
	path := retirementJournalPath(parentPath, rootName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read retirement journal %s: %w", path, err)
	}
	var j RetirementJournal
	if err := yaml.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("parse retirement journal %s: %w", path, err)
	}
	return &j, nil
}

// journalFileSuffix is the extension of every in-flight retirement
// journal, and the only thing a resume needs in order to find one.
const journalFileSuffix = ".journal.yaml"

// FindRetirementJournal looks for an in-flight retirement matching the
// operator's argument WITHOUT consulting the root registration.
//
// This is the resumability boundary. The final step of a retirement
// removes the root's directory, deregisters the root, and then removes
// the journal, so a failure between the last two leaves a journal for a
// root the registration no longer names. Resolving the target through
// the registration first would make exactly that state unreachable —
// the run that must be resumed is the one whose registration has
// already gone. So the journal location is scanned first, by filename,
// and a match resumes on the journal's own contents; only when nothing
// is in flight does the run fall back to resolving a registered target.
//
// The argument matches a journal by the root name it records or by the
// registered path it records, mirroring resolveRetirementTarget's
// name-or-relative-path rule. A journal that cannot be read or parsed
// is an error rather than a miss: "cannot tell" is not "nothing in
// flight", and starting a fresh destructive run over a part-finished
// one is the failure this refuses.
func FindRetirementJournal(parentPath, name string) (*RetirementJournal, error) {
	dir := retiredRootsDir(parentPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan retirement journals in %s: %w", dir, err)
	}
	cleanName := filepath.ToSlash(filepath.Clean(name))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), journalFileSuffix) {
			continue
		}
		root := strings.TrimSuffix(e.Name(), journalFileSuffix)
		j, err := LoadRetirementJournal(parentPath, root)
		if err != nil {
			return nil, err
		}
		if j == nil {
			continue
		}
		if j.Root == "" {
			return nil, fmt.Errorf("retirement journal %s names no root — a part-finished retirement that cannot say what it was retiring is refused rather than passed over", filepath.Join(dir, e.Name()))
		}
		if j.Root == name || filepath.ToSlash(filepath.Clean(j.RelativePath)) == cleanName {
			return j, nil
		}
	}
	return nil, nil
}

// WriteRetirementJournal persists the journal for the named root.
func WriteRetirementJournal(parentPath string, j *RetirementJournal) error {
	data, err := yaml.Marshal(j)
	if err != nil {
		return fmt.Errorf("marshal retirement journal: %w", err)
	}
	path := retirementJournalPath(parentPath, j.Root)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write retirement journal: %w", err)
	}
	return nil
}

// retirementHook is the fault-injection seam at the retirement's
// mutation boundaries. Tests substitute it to record event ordering or
// to abort the run at a named boundary; in production it is nil and
// every event is a no-op. Events fired, in order of a full run:
//
//	enumerate-features, sweep, archive-walk, stage-archive,
//	archive-copy (per member), verify-archive, promote, write-journal,
//	write-record, remove-contents, deregister-index, remove-journal
var retirementHook func(event string) error

// retirementEvent fires the hook for one named boundary. A non-nil
// return aborts the step it gates, exactly as the real failure it
// stands in for would.
func retirementEvent(event string) error {
	if retirementHook == nil {
		return nil
	}
	return retirementHook(event)
}

// completeJournalStep removes the finished step from the front of the
// journal and persists the shrunk record, so an interruption between
// steps resumes with only what remains. The journal file itself is
// removed by the deregister-root step, not here.
func completeJournalStep(parentPath string, j *RetirementJournal) error {
	if len(j.Outstanding) > 0 {
		j.Outstanding = j.Outstanding[1:]
	}
	if len(j.Outstanding) == 0 {
		return nil
	}
	return WriteRetirementJournal(parentPath, j)
}

// executeJournal runs the outstanding steps of a retirement, in order.
// Completed steps are never repeated — their preconditions were
// consumed (the destination exists, the contents have moved) — which is
// what makes the same function serve both the fresh run (with the full
// step list) and a resumed one (with whatever remains).
func executeJournal(parentPath string, idx *config.RootsIndex, j *RetirementJournal, record *retirementRecord) error {
	for len(j.Outstanding) > 0 {
		step := j.Outstanding[0]
		switch step {
		case journalStepWriteRecord:
			if err := retirementEvent("write-record"); err != nil {
				return err
			}
			if err := writeRetirementRecord(retirementDestination(parentPath, j.Root), record); err != nil {
				return err
			}
			if err := completeJournalStep(parentPath, j); err != nil {
				return err
			}
		case journalStepDeregisterRoot:
			// The root's directory is removed before the registration
			// stops naming it: at no point is the root deregistered
			// while its directory is still in place.
			// The journal's path is re-validated for containment on every
			// resumed run: the deletion target is derived from it, and a
			// journal is an on-disk file like any other.
			childDir, pathErr := resolveContainedChildDir(parentPath, j.RelativePath)
			if pathErr != nil {
				return fmt.Errorf("retirement journal for %q names a path that is not inside the project: %w", j.Root, pathErr)
			}
			if _, err := os.Lstat(childDir); err == nil {
				if err := retirementEvent("remove-contents"); err != nil {
					return err
				}
				if err := os.RemoveAll(childDir); err != nil {
					return fmt.Errorf("remove retired root directory %s: %w", childDir, err)
				}
			}
			if err := retirementEvent("deregister-index"); err != nil {
				return err
			}
			if _, ok := idx.Lookup(j.Root); ok {
				if _, err := config.RemoveRootFromIndex(idx, j.Root); err != nil {
					return fmt.Errorf("deregister root %q: %w", j.Root, err)
				}
			}
			// The run is complete only when the registration no longer
			// names the root; the journal is removed in the same final
			// step.
			if err := retirementEvent("remove-journal"); err != nil {
				return err
			}
			if err := os.Remove(retirementJournalPath(parentPath, j.Root)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove retirement journal: %w", err)
			}
			j.Outstanding = nil
		default:
			return fmt.Errorf("retirement journal for %q names unknown step %q (known: %s, %s)",
				j.Root, step, journalStepWriteRecord, journalStepDeregisterRoot)
		}
	}
	return nil
}
