package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/parser"
)

// parlay internal compact — move applied ledger history into archive/.
//
// Compaction was the workaround for a deadlock that no longer exists: the
// affects resolver demanded a feature's contract entries resolve forever while
// retirement demanded those artifacts be gone, and archiving was the only way
// out. Applied-history-aware resolution closed that, so this command is ledger
// HYGIENE — shortening a long active ledger — not an escape hatch.
//
// It is also not a file mover. The loader ignores subdirectories, so archiving
// a record removes its supersedes_intents claim and its supersedes: edges from
// the ledger's view. Archive the wrong record and a retired founding intent
// silently comes back, or an active record is left naming one that no longer
// exists. This repository's own studio-cli-hooks compaction was sound only
// because both ends of a supersedes edge happened to move together — by
// construction, not by any check.
//
// So: the gate is projection equivalence, the move is transactional, and a
// post-move mismatch rolls back.

var (
	compactThrough int
	compactConfirm bool
)

var compactCmd = &cobra.Command{
	Use:   "compact @<feature>",
	Short: "Move applied ledger history into amendments/archive/, proving authority is unchanged",
	Long: `Archive applied amendments, leaving the feature's authority identical.

Only records that are TRUSTED APPLIED may move: the baseline's marker must
cover them and its stored hash must match the bytes on disk. A pending record,
one with no recorded evidence, or one whose hash no longer matches is refused —
compaction must never be a way to retire a record nobody applied.

The authority projection (active intents, superseded-intent heads, retirement
head, pending tail, supersession edges, and the applied-authority capsule) is
captured before the move and compared after. Any difference rolls the move
back.

What compaction DOES change, by design: archived records leave the semantic
ledger walk, so the amendment listing and all_affects shrink. That is what
compaction is for. The authority fields above are what it guarantees.`,
	Args: cobra.ExactArgs(1),
	RunE: runCompact,
}

func init() {
	compactCmd.Flags().IntVar(&compactThrough, "through", 0,
		"Archive applied records at or below this sequence (default: every trusted applied record)")
	compactCmd.Flags().BoolVar(&compactConfirm, "confirm", false,
		"required: compaction rewrites where history lives")
}

type compactOutput struct {
	Feature  string   `json:"feature"`
	Archived []string `json:"archived"`
	Refused  []string `json:"refused,omitempty"`
}

func runCompact(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if compactThrough < 0 {
		return fmt.Errorf("--through %d is not a sequence; a negative threshold silently reads "+
			"as the default and would archive everything", compactThrough)
	}
	slug := parser.FeatureSlug(args[0])
	featDir := cfg.FeaturePath(slug)

	// An interrupted run is recovered before anything else. Starting a second
	// compaction over a half-compacted ledger would compound it, and the
	// selection logic would be reasoning about a state nobody intended.
	if err := recoverCompaction(cfg, slug); err != nil {
		return err
	}

	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		return fmt.Errorf("read ledger: %w", err)
	}
	capsule, err := observeAppliedAuthority(cfg, slug)
	if err != nil {
		return fmt.Errorf("read applied authority for %s: %w — compaction must not move history "+
			"it cannot prove was applied", slug, err)
	}

	candidates, refusals := selectCompactionSet(featDir, amendments, capsule, compactThrough)
	if len(refusals) > 0 {
		sort.Strings(refusals)
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(compactOutput{Feature: slug, Refused: refusals})
		return fmt.Errorf("compaction refused; nothing moved:\n  - %s", strings.Join(refusals, "\n  - "))
	}
	if len(candidates) == 0 {
		return fmt.Errorf("%s has no trusted applied records to compact", slug)
	}

	if !compactConfirm {
		return fmt.Errorf("compacting %s would archive %d record(s): %s. This rewrites where "+
			"history lives; re-run with --confirm", slug, len(candidates),
			joinNames(identities(candidates)))
	}

	before, err := computeAuthorityProjection(cfg, slug)
	if err != nil {
		return fmt.Errorf("capture authority projection: %w", err)
	}
	if len(before.Errors) > 0 {
		return fmt.Errorf("%s has %d unresolved ledger error(s), so its authority projection is "+
			"not sound to preserve; nothing moved:\n  - %s", slug, len(before.Errors),
			strings.Join(before.Errors, "\n  - "))
	}

	// Write the intent down before the first rename. A process death after
	// this point leaves a record the next invocation recovers from; without
	// it, a half-compacted ledger is indistinguishable from an intended one.
	journalPath := compactJournalPath(cfg, slug)
	txn := compactJournal{Feature: slug, BeforeProjection: before.canonical()}
	for _, a := range candidates {
		txn.Amendments = append(txn.Amendments, filepath.Base(a.Path))
	}
	if err := writeCompactJournal(journalPath, txn); err != nil {
		return fmt.Errorf("record the compaction intent: %w — refusing to move history without a "+
			"way to recover an interrupted run", err)
	}

	moved, err := archiveRecords(featDir, candidates)
	if err != nil {
		return abortCompaction(featDir, journalPath, moved, fmt.Errorf("archiving %s: %w", slug, err))
	}

	after, projErr := computeAuthorityProjectionTx(cfg, slug, true)
	switch {
	case projErr != nil:
		return abortCompaction(featDir, journalPath, moved,
			fmt.Errorf("the authority projection could not be recomputed after the move: %v", projErr))
	case len(after.Errors) > 0:
		return abortCompaction(featDir, journalPath, moved,
			fmt.Errorf("compaction introduced %d ledger error(s):\n  - %s",
				len(after.Errors), strings.Join(after.Errors, "\n  - ")))
	case before.canonical() != after.canonical():
		return abortCompaction(featDir, journalPath, moved,
			fmt.Errorf("compaction would have changed what %s promises.\n--- before ---\n%s--- after ---\n%s",
				slug, before.canonical(), after.canonical()))
	}

	// The capsule must still verify against the bytes in their new home,
	// through the same retained-hash lookup the integrity check uses.
	for name, stored := range capsule.Hashes {
		if actual, ok := retainedAmendmentHash(featDir, name); !ok || actual != stored {
			return abortCompaction(featDir, journalPath, moved,
				fmt.Errorf("after the move, %s no longer verifies against its recorded evidence", name))
		}
	}

	// Verified. Only now is the intent discharged.
	if err := os.Remove(journalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("compaction of %s succeeded and verified, but its journal at %s could "+
			"not be cleared (%v) — the next run would try to recover a completed compaction; "+
			"remove it by hand", slug, journalPath, err)
	}

	out := compactOutput{Feature: slug, Archived: identities(candidates)}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// selectCompactionSet returns the records that may move, or the reasons none may.
//
// Two gates, in order. Trust first: a record is a candidate only if the marker
// covers it AND its stored hash matches — "seq <= marker" alone would let a
// hand-moved marker retire records nobody applied. Then projection closure: a
// candidate whose supersedes edge or intent-supersession authority would be
// split by the archive boundary refuses rather than silently dangling.
func selectCompactionSet(featDir string, amendments []parser.Amendment, capsule appliedAuthority, through int) ([]parser.Amendment, []string) {
	var refusals []string
	archiving := map[string]bool{} // amendment slug -> being archived
	var candidates []parser.Amendment

	for _, a := range amendments {
		if through > 0 && a.Seq > through {
			continue
		}
		if a.Seq > capsule.Through {
			continue // pending: not history yet
		}
		if !amendmentTrustedApplied(capsule, featDir, a) {
			refusals = append(refusals, fmt.Sprintf("%s is at or below the applied marker but is "+
				"not trusted applied — its recorded evidence is missing or no longer matches. "+
				"Compaction may not retire a record nobody can show was applied",
				amendmentIdentity(a)))
			continue
		}
		candidates = append(candidates, a)
		archiving[a.Slug] = true
	}
	if len(refusals) > 0 {
		return nil, refusals
	}

	// Projection closure. An ACTIVE record naming an archived one in
	// supersedes: would be left dangling, reported as
	// amendment-supersedes-unknown.
	for _, a := range amendments {
		if archiving[a.Slug] {
			continue
		}
		for _, sup := range a.Supersedes {
			if archiving[sup] {
				refusals = append(refusals, fmt.Sprintf("%s stays active and supersedes %q, "+
					"which this run would archive — archiving one end of a supersession edge "+
					"leaves the other naming a record no longer in the ledger. Extend the set to "+
					"include %s, or compact no further than it",
					amendmentIdentity(a), sup, amendmentIdentity(a)))
			}
		}
	}

	// An archived record's supersedes_intents claim leaves the ledger's view,
	// which would reactivate a founding intent it had retired — unless an
	// active record restates the same claim.
	restated := map[string]bool{}
	for _, a := range amendments {
		if archiving[a.Slug] {
			continue
		}
		for _, in := range a.SupersedesIntents {
			restated[in] = true
		}
	}
	for _, a := range candidates {
		for _, in := range a.SupersedesIntents {
			if !restated[in] {
				refusals = append(refusals, fmt.Sprintf("%s retires the founding intent %q and no "+
					"active record restates it — archiving it would bring that promise back into "+
					"force. A retired promise must not return because its record moved",
					amendmentIdentity(a), in))
			}
		}
		if a.RetiresFeature {
			refusals = append(refusals, fmt.Sprintf("%s is the terminal retirement record — "+
				"archiving it would make the feature read as live again", amendmentIdentity(a)))
		}
	}
	if len(refusals) > 0 {
		return nil, refusals
	}
	return candidates, nil
}

// archiveRecords moves each record, preflighting every source and destination
// before touching anything and reporting what it moved so a caller can roll
// back. Returns the moved pairs in the order performed.
func archiveRecords(featDir string, records []parser.Amendment) ([]string, error) {
	archive := filepath.Join(featDir, "amendments", "archive")

	// Preflight everything first: a partial move is the failure mode this
	// avoids, and an unwritable archive dir or a colliding destination is
	// knowable before the first rename.
	for _, a := range records {
		if _, err := os.Stat(a.Path); err != nil {
			return nil, fmt.Errorf("source %s: %w", filepath.Base(a.Path), err)
		}
		dest := filepath.Join(archive, filepath.Base(a.Path))
		if _, err := os.Stat(dest); err == nil {
			return nil, fmt.Errorf("%s already exists in archive/ — history is written once and "+
				"never overwritten; refusing rather than replacing it", filepath.Base(a.Path))
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("destination %s: %w", filepath.Base(a.Path), err)
		}
	}
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return nil, fmt.Errorf("create archive dir: %w", err)
	}

	var moved []string
	for _, a := range records {
		name := filepath.Base(a.Path)
		if err := os.Rename(a.Path, filepath.Join(archive, name)); err != nil {
			return moved, fmt.Errorf("move %s: %w", name, err)
		}
		moved = append(moved, name)
	}
	return moved, nil
}

// abortCompaction rolls back and reports honestly.
//
// A caller that prints "rolled back" while residue remains has stated a false
// terminal condition — the operator would believe the ledger was intact and
// move on. Every restore failure is surfaced, and the journal is deliberately
// LEFT in place when any remains, so the next invocation still knows a
// compaction was in flight.
func abortCompaction(featDir, journalPath string, moved []string, cause error) error {
	problems := rollbackMoves(featDir, moved)
	if len(problems) == 0 {
		if err := os.Remove(journalPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%w — rolled back, but the compaction journal at %s could not be "+
				"cleared: %v", cause, journalPath, err)
		}
		return fmt.Errorf("%w — rolled back, nothing moved", cause)
	}
	return fmt.Errorf("%w — AND THE ROLLBACK DID NOT COMPLETE, so the ledger is part-compacted. "+
		"The journal at %s is left in place and the next run will recover from it:\n  - %s",
		cause, journalPath, joinLines(problems))
}
