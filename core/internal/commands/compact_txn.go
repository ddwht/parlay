package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The compaction transaction record.
//
// An in-memory list of what was moved supports rollback after a returned
// error. It does not survive the process dying between two renames, and a
// half-compacted ledger is exactly the state nothing else can classify: some
// records active, some archived, and no way to tell whether that was the
// intended end state or the middle of an interrupted run.
//
// So the intent is written down before the first rename and removed only after
// the restored state has been verified. The journal is DATA, never
// instructions: it records amendment FILENAMES, and recovery derives every
// path itself from the feature it was asked about. An earlier version stored
// absolute from/to paths and renamed whatever they named, which made a damaged
// or hand-edited journal into a way to move an unrelated file — a recovery
// path that trusts its own input is not a recovery path.

const compactJournalFile = ".compact-journal.json"

type compactJournal struct {
	// Feature is checked against the slug being recovered. A journal is
	// evidence about one feature and must not be applied to another.
	Feature string `json:"feature"`
	// BeforeProjection is the canonical authority projection captured before
	// the first move. Recovery must reproduce it exactly.
	BeforeProjection string `json:"before_projection"`
	// Amendments are exact ledger filenames, in move order. Never paths.
	Amendments []string `json:"amendments"`
}

// compactionInFlight reports whether a feature has an unfinished compaction.
//
// While one exists the ledger may be half-moved, so nothing may record
// authority over it: a save or a governance application would be writing
// against a state nobody intended and that recovery is about to undo.
//
// FAIL CLOSED. An earlier version returned `err == nil`, which turned every
// failure — a permission error, an I/O error — into "no transaction in
// flight", so an unreadable transaction marker authorised the very mutations
// it exists to block. Only a genuine absence means absent; anything else is an
// unknown that callers must refuse on.
//
// Lstat, not Stat: any directory entry at that exact path is a barrier. A
// broken symlink reads as ENOENT through Stat while being plainly visible in
// the directory, and "there is something here I cannot follow" is not the same
// claim as "there is nothing here".
func compactionInFlight(cfg *config.Context, slug string) (bool, error) {
	path := compactJournalPath(cfg, slug)
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("the compaction transaction marker at %s cannot be read (%w), so "+
			"whether a compaction is in flight is unknown", path, err)
	}
	return true, nil
}

func compactJournalPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), compactJournalFile)
}

func writeCompactJournal(path string, j compactJournal) error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicfile.WriteAtomic(path, data)
}

func loadCompactJournal(path string) (*compactJournal, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j compactJournal
	if err := json.Unmarshal(data, &j); err != nil {
		// A corrupt journal is evidence a run died, not something to discard:
		// discarding it would strand a half-compacted ledger with nothing
		// recording that a compaction was ever in flight.
		return nil, fmt.Errorf("the compaction journal at %s is unreadable (%v). A compaction was "+
			"in flight and its record is damaged; resolve by hand rather than starting another",
			path, err)
	}
	return &j, nil
}

// validateCompactJournal refuses a journal that could name anything but this
// feature's own ledger records.
func validateCompactJournal(j *compactJournal, slug string) error {
	if j.Feature != slug {
		return fmt.Errorf("the compaction journal here records feature %q, not %q — a journal is "+
			"evidence about one feature and must never be applied to another", j.Feature, slug)
	}
	if len(j.Amendments) == 0 {
		return fmt.Errorf("the compaction journal names no records")
	}
	seen := map[string]bool{}
	for _, name := range j.Amendments {
		if name != filepath.Base(name) || strings.ContainsRune(name, os.PathSeparator) ||
			strings.Contains(name, "..") {
			return fmt.Errorf("the compaction journal names %q, which is a path rather than a "+
				"ledger filename — recovery derives its own paths and accepts none", name)
		}
		if !parser.AmendmentFileNameValid(name) {
			return fmt.Errorf("the compaction journal names %q, which is not an amendment "+
				"filename", name)
		}
		if seen[name] {
			return fmt.Errorf("the compaction journal names %q twice", name)
		}
		seen[name] = true
	}
	return nil
}

// amendmentLocations reports where a record currently is, distinguishing
// genuine absence from an unreadable answer. A permission or I/O error is not
// "the file is not there", and treating it as absence picks the wrong
// recovery action.
func amendmentLocations(featDir, name string) (activePath, archivePath string, active, archived bool, err error) {
	activePath = filepath.Join(featDir, "amendments", name)
	archivePath = filepath.Join(featDir, "amendments", "archive", name)

	if _, sErr := os.Stat(activePath); sErr == nil {
		active = true
	} else if !os.IsNotExist(sErr) {
		return activePath, archivePath, false, false, fmt.Errorf("%s in the ledger: %w", name, sErr)
	}
	if _, sErr := os.Stat(archivePath); sErr == nil {
		archived = true
	} else if !os.IsNotExist(sErr) {
		return activePath, archivePath, false, false, fmt.Errorf("%s in archive/: %w", name, sErr)
	}
	return activePath, archivePath, active, archived, nil
}

// recoverCompaction returns any interrupted run to its before state.
//
// Recovery prefers rollback: the before state is the one we have a recorded
// description of, and finishing an interrupted compaction would mean asserting
// its projection held without ever having compared it.
//
// Restoring the FILES is only half of it. The window between the crash and now
// is unbounded, so the bytes may have changed — moving an altered record back
// and calling it recovered would launder a mutation into the active ledger.
// So the restored state must reproduce the recorded projection AND every
// restored record must still verify against the capsule's stored hash before
// the journal is discharged.
func recoverCompaction(cfg *config.Context, slug string) error {
	path := compactJournalPath(cfg, slug)
	j, err := loadCompactJournal(path)
	if err != nil {
		return err
	}
	if j == nil {
		return nil
	}
	if err := validateCompactJournal(j, slug); err != nil {
		return fmt.Errorf("refusing to recover the compaction of %s: %w", slug, err)
	}

	featDir := cfg.FeaturePath(slug)
	var problems []string
	for i := len(j.Amendments) - 1; i >= 0; i-- {
		name := j.Amendments[i]
		activePath, archivePath, active, archived, locErr := amendmentLocations(featDir, name)
		switch {
		case locErr != nil:
			problems = append(problems, locErr.Error())
		case active && archived:
			problems = append(problems, fmt.Sprintf("%s exists in BOTH the ledger and archive/ — "+
				"refusing to choose which is history", name))
		case !active && !archived:
			problems = append(problems, fmt.Sprintf("%s is in neither the ledger nor archive/ — "+
				"the record is missing and cannot be recovered here", name))
		case archived:
			if err := os.Rename(archivePath, activePath); err != nil {
				problems = append(problems, fmt.Sprintf("restore %s: %v", name, err))
			}
		}
	}
	if len(problems) > 0 {
		return incompleteRecovery(slug, path, problems)
	}

	// Locations restored. The BYTES come first, before anything is asked to
	// interpret them.
	//
	// Ordering matters here and used to be the other way round. Current-state
	// resolution now refuses a record whose bytes do not match the recorded
	// evidence, so recomputing the projection first meant this check could
	// never be reached: the refusal was still correct, but it came from a
	// generic resolver rather than from the operation that had just moved the
	// file, and "moving it back would launder a change into the active ledger"
	// is the thing somebody recovering a compaction needs to be told.
	//
	// The projection carries the baseline's STORED evidence, so it could not
	// notice a record whose content changed in the window anyway; only hashing
	// what is now on disk can.
	capsule, capErr := observeAppliedAuthority(cfg, slug)
	if capErr != nil {
		return incompleteRecovery(slug, path, []string{
			fmt.Sprintf("the applied-authority capsule could not be read: %v", capErr)})
	}
	for _, name := range j.Amendments {
		stored, recorded := capsule.Hashes[name]
		if !recorded {
			// A journal is only ever created for TRUSTED APPLIED records, and
			// trust requires a stored hash under this exact filename. Its
			// absence now means the authority state changed under the
			// interrupted run, or the journal is not one this tool wrote.
			// Skipping it would discharge the journal while verifying nothing.
			return incompleteRecovery(slug, path, []string{
				fmt.Sprintf("%s has no recorded evidence in the applied-authority capsule, but a "+
					"compaction journal is only written for records that had some — the authority "+
					"state changed while the run was interrupted, or this journal is not ours", name)})
		}
		actual, ok := hashWholeFile(filepath.Join(featDir, "amendments", name))
		if !ok {
			return incompleteRecovery(slug, path, []string{
				fmt.Sprintf("%s could not be hashed after restoration", name)})
		}
		if actual != stored {
			return incompleteRecovery(slug, path, []string{
				fmt.Sprintf("%s was restored but its bytes no longer match the evidence recorded "+
					"for it — moving it back would launder a change into the active ledger", name)})
		}
	}

	// Bytes verified. Now the harder question: is this the state the journal
	// described?
	after, projErr := computeAuthorityProjectionTx(cfg, slug, true)
	if projErr != nil {
		return incompleteRecovery(slug, path, []string{
			fmt.Sprintf("the authority projection could not be recomputed: %v", projErr)})
	}
	if after.canonical() != j.BeforeProjection {
		return incompleteRecovery(slug, path, []string{
			"the restored ledger does not reproduce the projection recorded before the " +
				"interrupted run, so something else changed while it was interrupted"})
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("recovered the interrupted compaction of %s but could not clear its "+
			"journal at %s: %v — a stale journal would block the next run", slug, path, err)
	}
	return nil
}

// incompleteRecovery reports honestly and LEAVES the journal, so the next run
// still knows a compaction was in flight.
func incompleteRecovery(slug, path string, problems []string) error {
	return fmt.Errorf("an interrupted compaction of %s could not be fully recovered, so nothing "+
		"further will run against it. Its journal at %s is left in place:\n  - %s",
		slug, path, joinLines(problems))
}

func joinLines(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += "\n  - "
		}
		out += s
	}
	return out
}

// rollbackMoves returns moved records in the live process, using the same
// location check recovery uses.
//
// It reports every failure rather than discarding them: a caller that says
// "rolled back" while residue remains has stated a false terminal condition.
// And it never overwrites a source that has unexpectedly reappeared — turning
// a concurrent or ambiguous state into a silent replacement is worse than
// stopping and saying so.
func rollbackMoves(featDir string, names []string) []string {
	var problems []string
	for i := len(names) - 1; i >= 0; i-- {
		name := names[i]
		activePath, archivePath, active, archived, err := amendmentLocations(featDir, name)
		switch {
		case err != nil:
			problems = append(problems, err.Error())
		case active && archived:
			problems = append(problems, fmt.Sprintf("%s reappeared in the ledger while also in "+
				"archive/ — refusing to overwrite it", name))
		case !active && !archived:
			problems = append(problems, fmt.Sprintf("%s is in neither place", name))
		case archived:
			if rErr := os.Rename(archivePath, activePath); rErr != nil {
				problems = append(problems, fmt.Sprintf("restore %s: %v", name, rErr))
			}
		}
	}
	return problems
}
