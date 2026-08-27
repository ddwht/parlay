package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/parser"
)

// Deferring is what a reviewer does when they have looked at a stranded
// judgment and genuinely cannot say whether it still holds.
//
// It exists so that "I cannot say" has somewhere to go OTHER than drop. Without
// it the only way past an entry you do not understand is to withdraw it, which
// means the pressure to keep a pipeline moving is also pressure to silently
// remove protections — and the ledger would record that as a decision somebody
// made. Recording the attempt keeps the entry unanswered while making the
// difficulty visible.
var deferLegacyCmd = &cobra.Command{
	Use:   "defer-legacy-exemption <@feature>",
	Short: "Record that a reviewer looked at a stranded exemption and could not decide",
	Long: `Record a review attempt that reached no decision.

This does NOT answer the exemption. The boundary keeps reporting it, and it
still needs a real decision — re-record it if it still holds, or drop it
deliberately if it does not. What this adds is that somebody looked, and who,
and why they could not say, so the next reviewer starts from that rather than
from nothing.

Attempts accumulate. Two people independently unable to decide is a different
fact from one attempt overwritten twice, and it is worth being able to see.`,
	Args: cobra.ExactArgs(1),
	RunE: runDeferLegacyExemption,
}

var (
	deferLegacyFP     string
	deferLegacyDup    int
	deferLegacyHash   string
	deferLegacyReason string
	deferLegacyBy     string
)

func init() {
	f := deferLegacyCmd.Flags()
	f.StringVar(&deferLegacyFP, "fingerprint", "", "the exact entry, from migrate-coverage-exceptions (required)")
	f.IntVar(&deferLegacyDup, "duplicate", 0, "which copy, when entries are identical in every field")
	f.StringVar(&deferLegacyHash, "legacy-file-hash", "", "the version of the retired review you were shown (required)")
	f.StringVar(&deferLegacyReason, "reason", "", "required: what you could not determine")
	f.StringVar(&deferLegacyBy, "by", "", "required: who looked")
}

type deferLegacyOutput struct {
	Feature string `json:"feature"`
	Ref     string `json:"ref"`
	// Recorded is false when this attempt was already on record. The entry is
	// in the same state either way, which is what makes the retry safe.
	Recorded bool `json:"recorded"`
	Attempts int  `json:"attempts_on_this_entry"`
	// StillUnanswered is always true. It is emitted rather than implied so a
	// caller reading this JSON cannot mistake a successful write for progress
	// through the migration.
	StillUnanswered bool   `json:"still_unanswered"`
	Note            string `json:"note"`
}

func runDeferLegacyExemption(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	for name, v := range map[string]string{
		"--fingerprint": deferLegacyFP,
		"--reason":      deferLegacyReason,
		"--by":          deferLegacyBy,
	} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required: a deferral nobody can attribute, about nothing in particular, tells the next reviewer nothing they did not already know", name)
		}
	}

	entries, fileHash, lErr := loadLegacyEntries(cfg, slug)
	if lErr != nil {
		return lErr
	}
	// The same token discipline as the deciding writers. A deferral is a
	// weaker statement but it is still a statement about a specific
	// occurrence, and one recorded against an entry the reviewer never saw is
	// as misleading as a decision would be.
	if hErr := requireUnchangedLegacyFile(deferLegacyHash, fileHash); hErr != nil {
		return hErr
	}
	entry, fErr := findLegacyEntry(entries, deferLegacyFP, deferLegacyDup)
	if fErr != nil {
		return fErr
	}

	rec, err := loadCoverageExceptions(cfg, slug)
	if err != nil {
		return fmt.Errorf("read existing decisions: %w — refusing to overwrite a ledger that could not be read", err)
	}
	if rec == nil {
		rec = &CoverageExceptions{Feature: slug, GrantedAt: time.Now().UTC().Format(time.RFC3339)}
	}

	// An entry already answered does not need deferring, and saying so is more
	// useful than silently filing an attempt against a closed question.
	for _, d := range rec.ReconciledLegacy {
		if d.Fingerprint == entry.Fingerprint && d.Duplicate == entry.Duplicate {
			return fmt.Errorf("%s was already answered (%s) — there is nothing left to defer. To revisit it, retire that decision first", entry.Ref, d.Disposition)
		}
	}

	attempt := LegacyDeferral{
		Ref: entry.Ref, Text: entry.Text,
		Fingerprint: entry.Fingerprint, Duplicate: entry.Duplicate,
		SourceHash: fileHash,
		Reason:     deferLegacyReason, By: deferLegacyBy,
		At: time.Now().UTC().Format(time.RFC3339),
	}

	recorded := true
	for _, existing := range rec.DeferredLegacy {
		if existing.sameAttempt(attempt) {
			recorded = false
			break
		}
	}
	if recorded {
		rec.LegacyFileHash = fileHash
		rec.DeferredLegacy = append(rec.DeferredLegacy, attempt)
		if err := saveCoverageExceptions(cfg, slug, rec); err != nil {
			return err
		}
	}

	attempts := 0
	for _, d := range rec.DeferredLegacy {
		if d.Fingerprint == entry.Fingerprint && d.Duplicate == entry.Duplicate {
			attempts++
		}
	}

	note := "This entry is still unanswered and the boundary will keep reporting it."
	if !recorded {
		note = "This attempt was already on record; nothing was added. The entry is still unanswered."
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(deferLegacyOutput{
		Feature: slug, Ref: entry.Ref, Recorded: recorded,
		Attempts: attempts, StillUnanswered: true, Note: note,
	})
}
