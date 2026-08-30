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

package commands

import (
	"fmt"
	"os"
	"path/filepath"

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
//	archive-copy (per member), promote, write-journal, write-record,
//	remove-contents, deregister-index, remove-journal
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
			childDir := filepath.Join(parentPath, filepath.FromSlash(j.RelativePath))
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
