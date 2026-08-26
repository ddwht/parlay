// parlay-feature: parlay-tool/intent-supersession
// parlay-component: active-specification-resolver
//
// The semantic half of intent supersession: given a feature's raw intents, its
// ledger, and how far that ledger has been applied, which promises does the
// feature currently make?
//
// Pure. No filesystem, no config, no baseline type — the caller reads those and
// passes the three facts in. That keeps the rule testable without a project on
// disk, and keeps this package free of the I/O layer it validates for.
//
// One resolver rather than a filter per consumer. Founding intents are read by
// coverage, question collection, verify routing, dialog generation and
// intent/dialog reconciliation; if each applied supersession itself they would
// drift, and the drift would surface as a contradiction between phases rather
// than as a bug in any one of them. The ledger already answers the same problem
// one level down the same way — check-amendments computes superseded_by once,
// and says an amendment carrying one "is history, not specification".
//
// What this does NOT serve is the integrity reads. Baseline hashing, diff,
// ledger freezing and file-shape validation keep reading the raw file, because
// supersession grants no exemption from byte integrity: the frozen document is
// never written to, so an edit to one must still be caught. Filtering there
// would manufacture exactly the hash exemption the mechanism promises not to
// create.

package agent

import (
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

// IntentAuthority selects which decisions count as current.
//
// Two modes, named rather than a boolean, because the difference is a safety
// property and not a convenience. With only one view, either an authored
// amendment changes behaviour before anyone applies it, or the apply workflow
// cannot see the decision it is applying.
type IntentAuthority int

const (
	// AppliedAuthority counts only supersessions the baseline records as
	// applied. This is what every ordinary consumer uses. An authored but
	// unapplied supersession leaves its intent ACTIVE here on purpose: the
	// artifacts and the generated code still make that promise, so dropping it
	// would stop anything checking a promise the code still keeps.
	AppliedAuthority IntentAuthority = iota

	// ProspectiveAuthority additionally counts the unapplied tail, and reports
	// the newest claim rather than the one in force: a caller applying a
	// decision must see THAT decision, not a predecessor the ledger already
	// replaced.
	//
	// RESERVED FOR the apply workflow; not yet used by it. Nothing under
	// core/internal selects this mode outside tests, because refine currently
	// reads back the amendment it just wrote rather than asking the resolver.
	// Stated plainly because an unused safety mode drifts: when the apply path
	// does route through here, this comment is the contract it must honour, and
	// until then no claim is being made that it already does. Ordinary callers
	// must never select it — there is deliberately no flag, environment
	// variable or config that turns it on.
	ProspectiveAuthority
)

// SupersededIntent is a founding intent a supersession replaced, carried rather
// than dropped: what was promised, and which decision replaced it, is what
// makes the frozen document readable as history instead of as a contradiction.
type SupersededIntent struct {
	Intent      parser.Intent
	ByAmendment string
	Seq         int
	// Applied distinguishes a retirement in force from one merely proposed.
	// Under AppliedAuthority every entry here is applied; under
	// ProspectiveAuthority the tail appears with Applied false.
	Applied bool
}

// PendingSupersession is a supersession authored but not yet applied. Under
// AppliedAuthority its intent is still Active, and this is what a phase
// boundary refuses to advance past.
type PendingSupersession struct {
	Intent      string
	ByAmendment string
	Seq         int
}

// IntentResolution is a feature's promise set, split by standing.
type IntentResolution struct {
	// Active are the promises in force — what coverage must cover, what
	// verify routing may route, what dialog generation may generate for.
	Active []parser.Intent
	// Superseded are the promises replaced. Consumers that match artifacts
	// against intents need these as a separate identity rather than simply
	// absent: a dialog belonging to a superseded intent is HISTORY, and
	// reporting it as an orphan would turn preserved history into cleanup
	// debt — the opposite of what supersession promises.
	Superseded []SupersededIntent
	// Pending are authored, unapplied supersessions. Non-empty means a phase
	// boundary must not advance.
	Pending []PendingSupersession
}

// IsSuperseded reports whether an intent slug has been replaced.
func (r *IntentResolution) IsSuperseded(slug string) bool {
	for _, s := range r.Superseded {
		if s.Intent.Slug == slug {
			return true
		}
	}
	return false
}

// HasPending reports whether any supersession is authored but unapplied.
func (r *IntentResolution) HasPending() bool { return len(r.Pending) > 0 }

// PendingSummary renders the unapplied supersessions for a boundary message.
func (r *IntentResolution) PendingSummary() string {
	var parts []string
	for _, p := range r.Pending {
		parts = append(parts, p.ByAmendment+" retires intent "+p.Intent)
	}
	return strings.Join(parts, "; ")
}

// ResolveIntentAuthority splits a feature's intents into active and superseded.
//
// A feature with no supersession in its ledger resolves to every intent active,
// which is exactly what every caller saw before this existed.
func ResolveIntentAuthority(raw []parser.Intent, amendments []parser.Amendment, lastApplied int, mode IntentAuthority) IntentResolution {
	var out IntentResolution

	// Later amendments win. When two claim the same intent the ledger has
	// already reported the fork; resolving to the latest keeps this consistent
	// with the retiring amendment check-amendments names.
	type claim struct {
		by  string
		seq int
	}
	// Tracked separately so an unapplied later amendment cannot resurrect a
	// retirement already in force: a promise withdrawn by an applied decision
	// stays withdrawn even if a newer, unapplied decision also names it.
	appliedClaim := map[string]claim{}
	latestClaim := map[string]claim{}

	for _, a := range amendments {
		for _, rawRef := range a.SupersedesIntents {
			s := strings.TrimSpace(rawRef)
			// Shape findings — empty entries, qualified cross-feature refs —
			// belong to ValidateAmendment. Skipping them here means a
			// malformed record cannot retire anything, which is the safe
			// direction: it keeps the promise rather than dropping it on the
			// strength of a record already reported as wrong.
			if s == "" || strings.ContainsAny(s, "@/") {
				continue
			}
			c := claim{by: a.FileSlug, seq: a.Seq}
			// Keyed on max sequence rather than on iteration order. The
			// doc comment says later amendments win; taking the last write
			// would have made that silently depend on the caller sorting its
			// input, which this package cannot see and should not assume.
			if prev, ok := latestClaim[s]; !ok || c.seq > prev.seq {
				latestClaim[s] = c
			}
			if a.Seq <= lastApplied {
				if prev, ok := appliedClaim[s]; !ok || c.seq > prev.seq {
					appliedClaim[s] = c
				}
			}
		}
	}

	for _, in := range raw {
		applied, isApplied := appliedClaim[in.Slug]
		latest, isClaimed := latestClaim[in.Slug]

		// Which claim stands depends on the mode, and the difference is the
		// reason the modes exist. Applied authority names the decision in
		// FORCE. Prospective names the decision being APPLIED — when a newer
		// amendment supersedes the one that retired this promise, the apply
		// workflow must see the newer one, or it acts on a predecessor the
		// ledger has already replaced.
		if isClaimed && mode == ProspectiveAuthority {
			out.Superseded = append(out.Superseded, SupersededIntent{
				Intent: in, ByAmendment: latest.by, Seq: latest.seq,
				Applied: latest.seq <= lastApplied,
			})
			continue
		}
		if isApplied {
			out.Superseded = append(out.Superseded, SupersededIntent{
				Intent: in, ByAmendment: applied.by, Seq: applied.seq, Applied: true,
			})
			continue
		}
		if !isClaimed {
			out.Active = append(out.Active, in)
			continue
		}

		// Applied authority: the promise stands until the decision is applied,
		// and the boundary refuses to advance while it is not.
		out.Pending = append(out.Pending, PendingSupersession{
			Intent: in.Slug, ByAmendment: latest.by, Seq: latest.seq,
		})
		out.Active = append(out.Active, in)
	}

	return out
}
