package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
)

// CoverageMigrationStatus is the ONE projection of how far a feature's legacy
// reconciliation has got.
//
// Discovery and the walkthrough both read it. If each computed its own, the two
// could disagree about what is left — and a migration where the diagnosis and
// the tool that performs it tell different stories is worse than one with no
// diagnosis, because the reader has no way to know which to believe.
type CoverageMigrationStatus struct {
	Feature string `json:"feature"`
	// Counts are CURRENT STATE and mutually exclusive. An occurrence somebody
	// deferred and later decided counts only as answered; its attempts stay in
	// DeferralAttempts as history.
	Answered          int `json:"answered"`
	PendingUnreviewed int `json:"pending_unreviewed"`
	PendingDeferred   int `json:"pending_deferred"`
	PendingTotal      int `json:"pending_total"`
	// DeferralAttempts counts ATTEMPTS, not occurrences: several may belong to
	// one entry, and "twenty attempts across six questions" is a different
	// situation from twenty questions each looked at once.
	DeferralAttempts int `json:"deferral_attempts"`

	Occurrences []MigrationOccurrence `json:"occurrences"`
}

// MigrationOccurrence is one legacy entry and where it stands.
type MigrationOccurrence struct {
	// Label is a SHORT, stable, proven-unique handle for a person to select by.
	// It is not a token: the writers take the full fingerprint, which the skill
	// carries from this JSON so nobody ever copies either value by hand.
	Label string `json:"label"`
	// Fingerprint and Duplicate are the full identity a writer requires.
	Fingerprint string `json:"fingerprint"`
	Duplicate   int    `json:"duplicate"`

	Ref  string `json:"ref"`
	Text string `json:"criterion_text,omitempty"`
	// GrantedBecause is the original reason. It is what distinguishes two
	// entries sharing a ref and criterion text to a PERSON, where the
	// fingerprint distinguishes them to the writer.
	GrantedBecause string `json:"granted_because,omitempty"`

	// State is "answered", "deferred", or "unreviewed".
	State string `json:"state"`
	// Disposition is set when State is "answered".
	Disposition string           `json:"disposition,omitempty"`
	Attempts    []LegacyDeferral `json:"deferral_attempts,omitempty"`
}

// shortestUniquePrefixes returns the shortest prefix of each value that is
// unique across the set, never shorter than min.
//
// A fixed-length prefix is only safe if something proves it unique, and nothing
// did. Exposing a short handle that usually works and occasionally collides is
// worse than exposing none: the collision surfaces as two entries that look
// interchangeable at exactly the moment somebody is choosing between them.
func shortestUniquePrefixes(values []string, min int) map[string]string {
	out := make(map[string]string, len(values))
	for _, v := range values {
		n := min
		if n > len(v) {
			n = len(v)
		}
		for ; n < len(v); n++ {
			collides := false
			for _, other := range values {
				if other != v && len(other) >= n && other[:n] == v[:n] {
					collides = true
					break
				}
			}
			if !collides {
				break
			}
		}
		if n > len(v) {
			n = len(v)
		}
		out[v] = v[:n]
	}
	return out
}

// CollectCoverageMigrationStatus builds the projection for one feature.
func CollectCoverageMigrationStatus(cfg *config.Context, slug string) (*CoverageMigrationStatus, error) {
	entries, _, err := loadLegacyEntries(cfg, slug)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return &CoverageMigrationStatus{Feature: slug}, nil
	}

	rec, err := loadCoverageExceptions(cfg, slug)
	if err != nil {
		return nil, err
	}

	answered := map[string]LegacyDisposition{}
	attempts := map[string][]LegacyDeferral{}
	if rec != nil {
		for _, d := range rec.ReconciledLegacy {
			answered[legacyDispositionKey(d.Fingerprint, d.Duplicate)] = d
		}
		for _, d := range rec.DeferredLegacy {
			k := legacyDispositionKey(d.Fingerprint, d.Duplicate)
			attempts[k] = append(attempts[k], d)
		}
	}

	// Labels are computed over the fingerprints ACTUALLY PRESENT, so a label is
	// unique in the set the reader is looking at.
	fps := make([]string, 0, len(entries))
	for _, e := range entries {
		fps = append(fps, e.Fingerprint)
	}
	short := shortestUniquePrefixes(fps, 8)

	st := &CoverageMigrationStatus{Feature: slug}
	for _, e := range entries {
		k := legacyDispositionKey(e.Fingerprint, e.Duplicate)
		label := short[e.Fingerprint]
		if e.Duplicate > 0 {
			// Byte-identical copies share a fingerprint, so the copy index is
			// the only thing that separates them.
			label = fmt.Sprintf("%s.%d", label, e.Duplicate)
		}
		occ := MigrationOccurrence{
			Label: label, Fingerprint: e.Fingerprint, Duplicate: e.Duplicate,
			Ref: e.Ref, Text: e.Text, GrantedBecause: e.Reason,
			Attempts: attempts[k],
		}
		switch {
		case answered[k].Disposition != "":
			occ.State, occ.Disposition = "answered", answered[k].Disposition
			st.Answered++
		case len(attempts[k]) > 0:
			occ.State = "deferred"
			st.PendingDeferred++
		default:
			occ.State = "unreviewed"
			st.PendingUnreviewed++
		}
		st.DeferralAttempts += len(attempts[k])
		st.Occurrences = append(st.Occurrences, occ)
	}
	st.PendingTotal = st.PendingUnreviewed + st.PendingDeferred
	return st, nil
}

// NextToReview returns the occurrence a walkthrough should offer next.
//
// Never-reviewed entries come before deferred ones. A deferred entry is one
// somebody already found hard, and putting it at the front of every subsequent
// sitting means the hardest question blocks all the easy ones — the traversal
// traps on it, and the migration stalls where it should have made progress.
func (s *CoverageMigrationStatus) NextToReview() *MigrationOccurrence {
	for _, want := range []string{"unreviewed", "deferred"} {
		for i := range s.Occurrences {
			if s.Occurrences[i].State == want {
				return &s.Occurrences[i]
			}
		}
	}
	return nil
}
