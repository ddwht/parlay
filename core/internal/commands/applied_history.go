package commands

import (
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/parser"
)

// Trusted applied history (WP3).
//
// The affects resolver used to require EVERY amendment's refs to resolve
// against the current contract, forever. That deadlocked retirement: the
// resolver demanded a feature's operations and fragments exist in perpetuity,
// while feature-retirement-has-output demanded those very artifacts be gone
// before the feature could be retired. Delete them and history unresolves;
// keep them and retirement refuses.
//
// The resolution is to treat an APPLIED record's refs as history. What makes
// that safe is that "applied" stops being a claim and becomes a checkable
// fact: a record is trusted only when the baseline's marker covers it AND the
// baseline's stored hash for it still matches the bytes retained on disk. A
// forged tail — a marker moved by hand with no evidence behind it — buys
// nothing, which is exactly the property WP1 and WP2 exist to keep true.
//
// The trade, stated rather than discovered: after trusted application the tool
// keeps FILE-level detection of a contract artifact being deleted or mutated
// (the baseline's whole-file hashes, and source-signatures as the hard
// emission gate) but relinquishes ENTRY-level historical attribution — it will
// report that capabilities.yaml changed, not that operation X, which amendment
// 002 once edited, is gone. That is the price of letting retirement dispose of
// feature-local contracts at all, and it is deliberately paid.

// retainedAmendmentHash hashes an amendment file wherever history retains it.
//
// Compaction moves applied records to amendments/archive/, which is the
// schema's documented post-compaction shape. Both the integrity check and the
// trust predicate must agree on where a record lives, so they share this
// lookup rather than each deciding for itself — a fork here would let a
// compacted ledger read as intact to one and erased to the other.
//
// The ACTIVE copy wins when both exist. A stale archive copy must never stand
// in for a mutated active one: that would let an edit to a live record hide
// behind history that still matches.
func retainedAmendmentHash(featurePath, name string) (string, bool) {
	if hash, ok := hashWholeFile(filepath.Join(featurePath, "amendments", name)); ok {
		return hash, true
	}
	return hashWholeFile(filepath.Join(featurePath, "amendments", "archive", name))
}

// amendmentTrustedApplied evaluates ONE record against an already-acquired
// authority capsule.
//
// Deliberately separate from observeAppliedAuthority, which acquires the
// capsule and answers nothing about trust. Keeping acquisition and evaluation
// apart is what lets check-amendments report an unreadable baseline as its own
// explicit finding instead of silently degrading to "nothing is applied".
//
// The key is the full amendment FILENAME, never the sequence alone: a stored
// hash map keyed by name is what ties the marker to specific bytes, and a
// sequence-only check would trust a record whose evidence was never recorded.
func amendmentTrustedApplied(capsule appliedAuthority, featurePath string, a parser.Amendment) bool {
	if a.Seq > capsule.Through {
		return false // still pending: its refs must resolve
	}
	name := filepath.Base(a.Path)
	stored, recorded := capsule.Hashes[name]
	if !recorded {
		return false // marker covers it, but nothing proves it was honoured
	}
	actual, found := retainedAmendmentHash(featurePath, name)
	if !found {
		return false // history erased, not retained
	}
	return actual == stored
}

// historicalRefTolerated reports whether an unresolvable ref may stand because
// the record that declared it is trusted history.
//
// Scope is deliberately narrow. Only the three FEATURE-LOCAL kinds deadlock:
// operation resolves against the feature's capabilities.yaml, surface against
// its surface artifact, and infrastructure against its infrastructure.md — all
// files retirement deletes. A domain ref is root-scoped, so it outlives its
// own feature's retirement and never had the problem; a cross-feature ref
// resolves against another feature's contract, whose disposal is that
// feature's drift responsibility. Neither gains an exemption here.
func historicalRefTolerated(ref parser.AmendmentRef, owningFeature string, trusted bool) bool {
	if !trusted {
		return false
	}
	if ref.Feature != owningFeature {
		return false
	}
	switch ref.Kind {
	case "operation", "surface", "infrastructure":
		return true
	default:
		return false
	}
}
