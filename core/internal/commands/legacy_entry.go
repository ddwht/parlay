package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// legacyEntry is one exemption in the retired review, with the identity a
// disposition must carry to answer exactly it.
type legacyEntry struct {
	Suite       string `json:"suite,omitempty"`
	Ref         string `json:"ref"`
	Text        string `json:"criterion_text,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Duplicate   int    `json:"duplicate"`
}

// loadLegacyEntries reads the retired review and assigns each entry its exact
// identity, along with the hash of the file they were read from.
//
// Both writers go through this rather than each deriving identity themselves.
// A disposition written under one notion of identity and checked under another
// is the failure this whole binding exists to prevent, and two copies of the
// derivation is how that happens.
func loadLegacyEntries(cfg *config.Context, slug string) (entries []legacyEntry, fileHash string, err error) {
	path := filepath.Join(cfg.BuildPath(slug), "coverage-review.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read the retired coverage review: %w", err)
	}
	legacy, err := parser.ParseCoverageReviewBytes(path, content)
	if err != nil {
		return nil, "", err
	}
	if legacy == nil {
		return nil, legacyFileHash(content), nil
	}
	seen := map[string]int{}
	for _, ex := range legacy.Exemptions {
		if ex.Item == "" {
			continue
		}
		fp := legacyExemptionFingerprint(ex)
		dup := seen[fp]
		seen[fp]++
		entries = append(entries, legacyEntry{
			Suite: ex.Suite, Ref: ex.Item, Text: ex.CriterionText,
			Reason: ex.Reason, Fingerprint: fp, Duplicate: dup,
		})
	}
	return entries, legacyFileHash(content), nil
}

// requireUnchangedLegacyFile refuses a write whose subject moved since the
// reviewer saw it.
//
// Recomputing identity against whatever file exists at confirmation time is not
// enough: the person approves the subject the report showed them, and between
// the report and the write the file may have gained, lost or reworded an entry.
// The command would then record a disposition against a NEWER occurrence than
// the one that was judged. The report's hash is therefore an
// optimistic-concurrency token, not a convenience.
func requireUnchangedLegacyFile(expected, actual string) error {
	if expected == "" {
		return fmt.Errorf("--legacy-file-hash is required: it is the version of the retired review you were shown, and without it this records a judgment against whatever the file happens to say now")
	}
	if expected != actual {
		return fmt.Errorf("the retired coverage review changed after it was listed, so the entry you reviewed may not be the one this would answer. Re-run migrate-coverage-exceptions and confirm against the current file")
	}
	return nil
}

// findLegacyEntry locates the entry a disposition names.
//
// An unmatched fingerprint is refused rather than recorded: a disposition that
// answers no entry marks nothing reconciled while reading in the ledger as
// though somebody had done the work.
func findLegacyEntry(entries []legacyEntry, fingerprint string, duplicate int) (legacyEntry, error) {
	for _, e := range entries {
		if e.Fingerprint == fingerprint && e.Duplicate == duplicate {
			return e, nil
		}
	}
	return legacyEntry{}, fmt.Errorf(
		"no exemption in the retired coverage review has fingerprint %q (copy %d). Run `parlay internal migrate-coverage-exceptions` to list the entries and their fingerprints; if the file changed since you listed them, list it again — an identity from an older version of the file answers an entry that may no longer be there",
		fingerprint, duplicate)
}
