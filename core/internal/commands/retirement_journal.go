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
	"bytes"
	"fmt"
	"io"
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
	// ManifestDigest is the hash of the archive manifest FILE as it
	// stood when this journal was written. It binds the journal to one
	// specific archive.
	//
	// Without it the two artifacts only have to be individually
	// plausible: a manifest verifies its own member list, and a journal
	// verifies its own shape, and nothing says they came from the same
	// run. Recording the digest here means a manifest that has been
	// rewritten — however consistently — no longer matches the journal
	// that authorizes acting on it.
	ManifestDigest string `yaml:"manifest-digest"`
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

// journalStepOrder is the canonical order of the journal's steps. A
// persisted journal's outstanding list is always a non-empty SUFFIX of
// it, because steps are completed from the front and the file is
// removed when none remain.
var journalStepOrder = []string{journalStepWriteRecord, journalStepDeregisterRoot}

// authenticateJournal establishes what can be established about a file
// found in the journal location before a resumed run acts on it.
//
// Be exact about what that is, because the obvious framing is wrong.
// A journal is an INSTRUCTION TO DELETE: a resumed run reads a root name
// and a relative path out of it and removes both the directory and the
// registration they name, with no preflight, no sweep and no disposition
// record, since the run that wrote the journal performed all of those.
// It is tempting to call the checks below "authentication" in the sense
// of proving the file's origin. They are not, and cannot be. The
// journal, the manifest, the retirement record and the digest are all
// writable by whoever runs this tool; a party who can forge one can
// forge the set, consistently. Establishing origin needs a trust anchor
// outside the repository — a signature, a key, an append-only log —
// and this operation has none. Any claim of provenance here would be a
// claim the code cannot keep.
//
// What these checks DO deliver, which is the part that matters:
//
//   - CONSISTENCY. Corruption, partial writes, stale artifacts and
//     honest operator mistakes are caught, loudly, before anything is
//     destroyed. This is the common case by an enormous margin.
//   - INTEGRITY, which is the real safety property. Nothing is deleted
//     unless it is provably present in the archive — see
//     authenticateJournalProgress, where every live byte must hash to
//     what the manifest records and the archived bytes must hash to the
//     same. A forged chain that satisfies this has, by construction,
//     preserved the content it is about to remove.
//
// Authorization is separate and is not the journal's to give: a resumed
// run passes the same human confirmation a fresh one does (see
// resumeRetirement), so the residual case — a chain forged by someone
// with write access — reduces to a person being asked, at a terminal,
// to confirm destroying bytes that are verifiably archived and
// recoverable.
//
// What is checked, each of which an inconsistent file fails:
//
//   - The decode is strict. An unknown key is refused rather than
//     dropped, so a file whose real content sits under keys this shape
//     does not define cannot present as a sparse valid journal.
//   - The recorded root is a plain slug, and it EQUALS the filename
//     stem. The name addresses the destination and the journal file
//     itself, so a journal named one thing and claiming another is
//     claiming authority over a root it is not filed under.
//   - The recorded path stays inside the project, judged on the
//     resolved path.
//   - The outstanding steps are a non-empty suffix of the canonical
//     order: known steps only, no duplicates, no reordering, and
//     nothing after the terminal step.
//   - The archive the journal is the tail of actually exists at the
//     canonical destination for that name, with a manifest that reads
//     back and verifies, naming at least one member, and holding
//     archived bytes that hash to what it records.
//
// Any failure is loud. Refusing is the safe answer: the cost is a
// person reading a message, against a deletion that may destroy
// something no copy holds.
func authenticateJournal(parentPath, fileStem string, j *RetirementJournal) error {
	where := retirementJournalPath(parentPath, fileStem)
	if err := validateRootName(j.Root); err != nil {
		return fmt.Errorf("retirement journal %s: %w", where, err)
	}
	if j.Root != fileStem {
		return fmt.Errorf("retirement journal %s records root %q but is filed under %q — a journal names the root it is filed under, and one claiming another root is claiming authority over a retirement it is not the record of",
			where, j.Root, fileStem)
	}
	if _, err := resolveContainedChildDir(parentPath, j.RelativePath); err != nil {
		return fmt.Errorf("retirement journal %s names a path that is not inside the project: %w", where, err)
	}
	if err := validateJournalSteps(where, j.Outstanding); err != nil {
		return err
	}
	// The archive this journal is the tail of must be there, and must
	// verify. A journal only ever exists after a promoted archive.
	dest := retirementDestination(parentPath, j.Root)
	if info, err := os.Stat(dest); err != nil || !info.IsDir() {
		return fmt.Errorf("retirement journal %s claims a part-finished retirement of %q, but no archive stands at %s — a journal is written only after the archive is complete, so one without an archive records a state this operation never produces and is refused rather than resumed",
			where, j.Root, dest)
	}
	manifestPath := filepath.Join(dest, "manifest.yaml")
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("retirement journal %s claims a part-finished retirement of %q, but the archive at %s has no manifest that reads back and verifies (%v) — the run this journal would resume deletes the root's directory, and it does that only against a preserved copy that can be shown to be whole",
			where, j.Root, dest, err)
	}
	// A manifest naming nothing is not an archive of a root. Every root
	// carries at least its own .parlay configuration, so an empty member
	// list describes a state this operation cannot produce — and it is
	// the cheapest possible forgery, since a self-covering hash over an
	// empty list verifies perfectly.
	if len(manifest.Members) == 0 {
		return fmt.Errorf("retirement journal %s claims a part-finished retirement of %q, but the manifest at %s names no members — a root preserves at least its own configuration, so a manifest naming nothing is not that root's archive",
			where, j.Root, manifestPath)
	}
	// The journal names the archive it belongs to, by the digest of the
	// manifest file as it stood when the journal was written. This is
	// what makes the two artifacts one record instead of two plausible
	// ones: a rewritten manifest re-covers its own member list, and only
	// a value recorded somewhere else notices.
	if strings.TrimSpace(j.ManifestDigest) == "" {
		return fmt.Errorf("retirement journal %s records no manifest digest — every journal this operation writes names the archive it belongs to, and one that names none cannot be shown to belong to the archive at %s",
			where, dest)
	}
	digest, err := manifestDigest(manifestPath)
	if err != nil {
		return fmt.Errorf("retirement journal %s: %w", where, err)
	}
	if digest != j.ManifestDigest {
		return fmt.Errorf("retirement journal %s records manifest digest %s but the manifest at %s digests to %s — the journal and the archive are not from the same run, and a journal is authority to delete only over the archive it was written beside",
			where, j.ManifestDigest, manifestPath, digest)
	}
	// The manifest describes files; establish that those files are
	// there and hash to what it says, rather than trusting a list that
	// verifies only against itself.
	if err := verifyArchivedMembers(filepath.Join(dest, "contents"), manifest.Members); err != nil {
		return fmt.Errorf("retirement journal %s claims a part-finished retirement of %q, but its archive does not hold what its manifest describes: %w",
			where, j.Root, err)
	}
	return authenticateJournalProgress(parentPath, where, j, dest, manifest)
}

// authenticateJournalProgress cross-checks what the journal says has
// already happened against what the filesystem shows.
//
// The outstanding list is a claim about progress: every step NOT in it
// is a step the journal asserts is finished. Each finished step left
// something behind, and each unfinished one has not yet had its effect
// — so the two can be compared, and a journal whose claimed progress
// contradicts the state it sits in is refused.
//
// The second half of this function carries the operation's real safety
// property, and it is a property about CONTENT rather than about
// origin. While the removal step is outstanding the contents are still
// there, so they are hashed against the manifest: the run may destroy
// them only once every one of them is provably in the preserved copy.
// A forged chain does not defeat that — it either preserves the bytes,
// in which case the deletion is recoverable from the archive, or it
// does not, in which case this refuses. That is the honest guarantee,
// and it is stronger than a provenance claim this trust model cannot
// support.
func authenticateJournalProgress(parentPath, where string, j *RetirementJournal, dest string, manifest *ArchiveManifest) error {
	outstanding := map[string]bool{}
	for _, step := range j.Outstanding {
		outstanding[step] = true
	}

	// write-record claimed finished: the record it writes must be there,
	// and must name this same retirement.
	if !outstanding[journalStepWriteRecord] {
		recordPath := filepath.Join(dest, retirementRecordFile)
		data, err := os.ReadFile(recordPath)
		if err != nil {
			return fmt.Errorf("retirement journal %s says the record step is finished, but no record stands at %s (%v) — a journal that claims progress the archive does not show was not written by the run it describes",
				where, recordPath, err)
		}
		var record retirementRecord
		if err := yaml.Unmarshal(data, &record); err != nil {
			return fmt.Errorf("retirement journal %s says the record step is finished, but the record at %s does not parse: %v", where, recordPath, err)
		}
		if record.Root != j.Root || filepath.ToSlash(filepath.Clean(record.RelativePath)) != filepath.ToSlash(filepath.Clean(j.RelativePath)) {
			return fmt.Errorf("retirement journal %s records %q at %q, but the retirement record beside its archive records %q at %q — the journal and the record describe different retirements",
				where, j.Root, j.RelativePath, record.Root, record.RelativePath)
		}
	}

	// deregister-root still outstanding: the contents have not been
	// removed yet, so whatever is still there must already be in the
	// archive. This is the one check a forger cannot satisfy without
	// having genuinely archived the root.
	if outstanding[journalStepDeregisterRoot] {
		childDir := filepath.Join(parentPath, filepath.FromSlash(j.RelativePath))
		if _, err := os.Lstat(childDir); err == nil {
			if err := archivePreservesLiveContents(childDir, manifest.Members); err != nil {
				return fmt.Errorf("retirement journal %s would remove %s on the authority of the archive at %s, but %w",
					where, childDir, dest, err)
			}
		}
	}
	return nil
}

// validateJournalSteps requires the outstanding list to be a non-empty
// suffix of the canonical order. That single rule covers every way the
// list can be wrong — an unknown step, a duplicate, a reordering, a step
// after the terminal one, an empty list that would complete a
// retirement by doing nothing — and it is the only shape the writer
// produces.
func validateJournalSteps(where string, outstanding []string) error {
	if len(outstanding) == 0 {
		return fmt.Errorf("retirement journal %s names no outstanding steps — a journal exists only while steps remain, and one naming none would complete a retirement by performing nothing", where)
	}
	for _, step := range outstanding {
		known := false
		for _, k := range journalStepOrder {
			if step == k {
				known = true
			}
		}
		if !known {
			return fmt.Errorf("retirement journal %s names unknown step %q (known: %s)", where, step, strings.Join(journalStepOrder, ", "))
		}
	}
	start := len(journalStepOrder) - len(outstanding)
	if start < 0 {
		return fmt.Errorf("retirement journal %s names %d steps, more than the %d this operation has — a repeated or invented step is refused rather than executed",
			where, len(outstanding), len(journalStepOrder))
	}
	for i, step := range outstanding {
		if step != journalStepOrder[start+i] {
			return fmt.Errorf("retirement journal %s names its steps as %s, which is not a tail of the order they must happen in (%s) — steps are completed from the front, so any other list has been rewritten",
				where, strings.Join(outstanding, ", "), strings.Join(journalStepOrder, ", "))
		}
	}
	return nil
}

// LoadRetirementJournal reads and AUTHENTICATES the journal filed under
// the given root name. Returns (nil, nil) when no journal exists — the
// normal state. Every other outcome is an error: a journal that cannot
// be read, cannot be parsed, or cannot be shown to be this operation's
// own record is not distinguishable from one that names outstanding
// steps, and the difference decides whether a directory is deleted.
func LoadRetirementJournal(parentPath, rootName string) (*RetirementJournal, error) {
	path := retirementJournalPath(parentPath, rootName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read retirement journal %s: %w", path, err)
	}
	// Strict decode: a key this shape does not define is refused, not
	// dropped. A dropped key is how a file that is mostly something else
	// presents as a sparse valid journal.
	var j RetirementJournal
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&j); err != nil && err != io.EOF {
		return nil, fmt.Errorf("parse retirement journal %s: %w", path, err)
	}
	var extra RetirementJournal
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("retirement journal %s carries more than one YAML document — a journal is one record of one part-finished run", path)
	} else if err != io.EOF {
		return nil, fmt.Errorf("parse retirement journal %s: %w", path, err)
	}
	if err := authenticateJournal(parentPath, rootName, &j); err != nil {
		return nil, err
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
		stem := strings.TrimSuffix(e.Name(), journalFileSuffix)
		// The filename stem is itself a root name, and it is the key the
		// journal is looked up by. A stem that is not a plain slug was
		// not filed by this operation, so it is refused here rather than
		// carried into a lookup that would trust what it says.
		if err := validateRootName(stem); err != nil {
			return nil, fmt.Errorf("%s sits in the retirement journal location but is not named for a root: %w", filepath.Join(dir, e.Name()), err)
		}
		// LoadRetirementJournal authenticates: strict decode, the
		// recorded root equal to this stem, a contained path, a valid
		// step tail, and a verified archive standing behind it.
		j, err := LoadRetirementJournal(parentPath, stem)
		if err != nil {
			return nil, err
		}
		if j == nil {
			continue
		}
		if j.Root == name || filepath.ToSlash(filepath.Clean(j.RelativePath)) == cleanName {
			return j, nil
		}
	}
	return nil, nil
}

// WriteRetirementJournal persists the journal for the named root,
// through a handle rooted at the project. The journal is the record a
// later run acts on destructively, so where it lands matters as much as
// what it says: a path-based write would follow a .parlay/retired
// replaced with a link out of the project, and leave the real location
// with no journal at all — the half-retired state the whole ordering
// exists to prevent.
func WriteRetirementJournal(parentPath string, j *RetirementJournal) error {
	if err := validateRootName(j.Root); err != nil {
		return fmt.Errorf("write retirement journal: %w", err)
	}
	data, err := yaml.Marshal(j)
	if err != nil {
		return fmt.Errorf("marshal retirement journal: %w", err)
	}
	if err := mutateUnderParent(parentPath, func(root *os.Root) error {
		return root.WriteFile(retirementJournalRel(j.Root), data, 0o644)
	}); err != nil {
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

// assertContentsPreserved re-establishes live == manifest == archived
// at the destructive boundary itself.
//
// Every other check in this operation is an admission check: it decides
// whether a run may proceed. This one decides whether THIS DELETION may
// happen, and it is deliberately the last thing between the decision and
// the act. The distinction is not academic — between admitting a run and
// removing its contents sits a confirmation prompt, which waits on a
// human, which is an unbounded interval in which the directory can
// change.
//
// It runs on every path that reaches the removal, fresh run and resume
// alike, because the window exists on both: a fresh run authenticates
// nothing but still asks for confirmation after its archive was taken.
//
// Both halves are re-run. The archived side may have been altered in the
// same window as the live side, and half a chain proves nothing.
func assertContentsPreserved(parentPath, rootName, childDir string) error {
	dest := retirementDestination(parentPath, rootName)
	manifest, err := ReadManifest(filepath.Join(dest, "manifest.yaml"))
	if err != nil {
		return fmt.Errorf("refusing to remove %s: its archive's manifest at %s no longer reads back and verifies (%w) — nothing is destroyed on the authority of an archive that cannot be shown to be whole",
			childDir, dest, err)
	}
	if err := verifyArchivedMembers(filepath.Join(dest, "contents"), manifest.Members); err != nil {
		return fmt.Errorf("refusing to remove %s: %w", childDir, err)
	}
	if err := archivePreservesLiveContents(childDir, manifest.Members); err != nil {
		return fmt.Errorf("refusing to remove %s: %w", childDir, err)
	}
	return nil
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
			if err := writeRetirementRecord(parentPath, j.Root, record); err != nil {
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
				// The preservation check runs HERE, immediately before
				// the removal, and not only when the journal was
				// admitted.
				//
				// Admission happens before the operator is asked to
				// confirm, and a person takes as long as a person takes.
				// A file written or edited in that interval — by an
				// editor left open, a build, another terminal — was
				// never in the archive, and destroying it would break
				// the one promise this operation makes. Checking at
				// admission establishes that the archive WAS complete;
				// checking here establishes that it IS complete, at the
				// only moment that decides anything.
				if err := assertContentsPreserved(parentPath, j.Root, childDir); err != nil {
					return err
				}
				// Through a handle rooted at the project, not through
				// the resolved path: the resolution above is a check
				// made a moment earlier, and an intermediate directory
				// swapped for a symlink in between would carry an
				// ordinary RemoveAll out of the project. The rooted
				// handle resolves and deletes in one sequence, so there
				// is no interval to exploit.
				if err := removeUnderParent(parentPath, filepath.FromSlash(j.RelativePath)); err != nil {
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
			if err := mutateUnderParent(parentPath, func(root *os.Root) error {
				return root.Remove(retirementJournalRel(j.Root))
			}); err != nil && !os.IsNotExist(err) {
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
