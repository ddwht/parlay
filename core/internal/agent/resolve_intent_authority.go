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
	// Mode is the transition that ended the lineage. Carried rather than
	// dropped: normalisation is safe only if it PRESERVES the uncertainty it
	// normalises, and a consumer must be able to tell a known retire from a
	// legacy_supersession whose semantics nobody recorded.
	Mode parser.IntentMode
	// Applied distinguishes a retirement in force from one merely proposed.
	// Under AppliedAuthority every entry here is applied; under
	// ProspectiveAuthority the tail appears with Applied false.
	Applied bool
}

// PendingIntentTransition is a transition authored but not yet applied. Under
// AppliedAuthority its intent is still Active, and this is what a phase
// boundary refuses to advance past.
//
// Named for the general case rather than for retirement: an unapplied extend or
// revise withdraws nothing, and a type called PendingSupersession invited every
// consumer to say so.
type PendingIntentTransition struct {
	Intent      string
	ByAmendment string
	Seq         int
	// Mode is the transition awaiting application. Without it every pending
	// transition renders as a retirement, so an unapplied extend or revise
	// would be reported to an operator as a promise about to be withdrawn.
	Mode parser.IntentMode
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
	// Pending are authored, unapplied intent transitions of any mode. Non-empty
	// means a phase boundary must not advance.
	Pending []PendingIntentTransition
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

// HasPending reports whether any intent transition is authored but unapplied.
func (r *IntentResolution) HasPending() bool { return len(r.Pending) > 0 }

// PendingSummary renders the unapplied transitions for a boundary message.
//
// By mode. Saying "retires intent" for an unapplied revision would tell an
// operator a promise is about to be withdrawn when it is about to be reworded,
// which is the opposite of what the vocabulary exists to distinguish.
func (r *IntentResolution) PendingSummary() string {
	var parts []string
	for _, p := range r.Pending {
		parts = append(parts, p.ByAmendment+" "+pendingVerb(p.Mode)+" intent "+p.Intent)
	}
	return strings.Join(parts, "; ")
}

func pendingVerb(m parser.IntentMode) string {
	switch m {
	case parser.IntentExtend:
		return "extends"
	case parser.IntentRevise:
		return "revises"
	case parser.IntentNarrow:
		return "narrows"
	case parser.IntentRetire:
		return "retires"
	case parser.IntentLegacySupersession:
		return "supersedes (legacy, mode unrecorded)"
	}
	return "changes"
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
		by   string
		seq  int
		mode parser.IntentMode
	}
	// Tracked separately so an unapplied later amendment cannot resurrect a
	// retirement already in force: a promise withdrawn by an applied decision
	// stays withdrawn even if a newer, unapplied decision also names it.
	appliedClaim := map[string]claim{}
	latestClaim := map[string]claim{}

	// Revisions, tracked alongside retirements. A lineage that was REVISED is
	// still active; what changes is its text. Keyed the same way and by the
	// same max-sequence rule, and separated into applied and latest for the
	// same reason: an unapplied later revision must not change what the code
	// currently promises.
	appliedRevision := map[string]intentRevision{}
	latestRevision := map[string]intentRevision{}

	for _, a := range amendments {
		for _, tr := range a.IntentTransitions() {
			if tr.Mode.EndsLineage() {
				continue // handled by the retirement pass below
			}
			s := strings.TrimSpace(tr.Intent)
			if s == "" || strings.ContainsAny(s, "@/") {
				continue
			}
			r := intentRevision{by: a.FileSlug, seq: a.Seq, mode: tr.Mode, version: tr.Version}
			if prev, ok := latestRevision[s]; !ok || r.seq > prev.seq {
				latestRevision[s] = r
			}
			if a.Seq <= lastApplied {
				if prev, ok := appliedRevision[s]; !ok || r.seq > prev.seq {
					appliedRevision[s] = r
				}
			}
		}
	}

	for _, a := range amendments {
		for _, tr := range a.IntentTransitions() {
			if !tr.Mode.EndsLineage() {
				continue
			}
			s := strings.TrimSpace(tr.Intent)
			// Shape findings — empty entries, qualified cross-feature refs —
			// belong to ValidateAmendment. Skipping them here means a
			// malformed record cannot retire anything, which is the safe
			// direction: it keeps the promise rather than dropping it on the
			// strength of a record already reported as wrong.
			if s == "" || strings.ContainsAny(s, "@/") {
				continue
			}
			c := claim{by: a.FileSlug, seq: a.Seq, mode: tr.Mode}
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
				Intent: currentVersionOf(in, appliedRevision), ByAmendment: latest.by,
				Seq: latest.seq, Mode: latest.mode, Applied: latest.seq <= lastApplied,
			})
			continue
		}
		if isApplied {
			out.Superseded = append(out.Superseded, SupersededIntent{
				Intent: currentVersionOf(in, appliedRevision), ByAmendment: applied.by,
				Seq: applied.seq, Mode: applied.mode, Applied: true,
			})
			continue
		}
		if !isClaimed {
			// Not retired. It may still have been REVISED, in which case what
			// the feature currently promises is the revised text — the whole
			// point of the vocabulary. Same authority rule as retirement: an
			// unapplied revision does not change what the code promises.
			out.Active = append(out.Active, applyRevision(in, appliedRevision, latestRevision, mode, lastApplied, &out))
			continue
		}

		// Applied authority: the promise stands until the decision is applied,
		// and the boundary refuses to advance while it is not.
		// The promise in force is the latest APPLIED version, not the founding
		// text. A pending retirement says the promise is about to end; it does
		// not un-apply a revision that already happened, and answering with
		// founding text here would describe a system nobody is running.
		out.Pending = append(out.Pending, PendingIntentTransition{
			Intent: in.Slug, ByAmendment: latest.by, Seq: latest.seq, Mode: latest.mode,
		})
		out.Active = append(out.Active, currentVersionOf(in, appliedRevision))
	}

	return out
}

// intentRevision is a lineage's latest non-terminal transition.
type intentRevision struct {
	by      string
	seq     int
	mode    parser.IntentMode
	version *parser.IntentVersion
}

// applyRevision returns the promise as it currently reads.
//
// The founding text is never rewritten on disk — it is history, and history is
// what makes the frozen document readable rather than contradictory. This
// returns the CURRENT version of the proposition, which is what a consumer
// asking "what does this feature promise" is actually asking for.
//
// An unapplied revision is reported as pending and leaves the founding text
// standing, exactly as an unapplied retirement does: the artifacts and the
// generated code still make the old promise, so answering with the new one
// would describe a system nobody has built yet.
func applyRevision(in parser.Intent, applied, latest map[string]intentRevision, mode IntentAuthority, lastApplied int, out *IntentResolution) parser.Intent {
	a, isApplied := applied[in.Slug]
	l, isRevised := latest[in.Slug]

	chosen, ok := a, isApplied
	if mode == ProspectiveAuthority && isRevised {
		chosen, ok = l, true
	}
	if !ok {
		if isRevised {
			out.Pending = append(out.Pending, PendingIntentTransition{
				Intent: in.Slug, ByAmendment: l.by, Seq: l.seq, Mode: l.mode,
			})
		}
		return in
	}

	return materialise(in.Slug, chosen.version)
}

// materialise builds the current promise from its immutable lineage slug and a
// complete version snapshot.
//
// Snapshot semantics, deliberately: every field comes from the version, so a
// field omitted there is ABSENT rather than inherited. Only the slug survives,
// because attribution binds to it and a transition may never change it.
func materialise(slug string, v *parser.IntentVersion) parser.Intent {
	if v == nil {
		return parser.Intent{Slug: slug}
	}
	return parser.Intent{
		Slug:        slug,
		Title:       v.Title,
		Goal:        v.Goal,
		Persona:     v.Persona,
		Priority:    v.Priority,
		Context:     v.Context,
		Action:      v.Action,
		Objects:     v.Objects,
		Constraints: v.Constraints,
		Verify:      v.Verify,
		Questions:   v.Questions,
	}
}

// currentVersionOf returns the promise as the applied ledger leaves it.
func currentVersionOf(in parser.Intent, applied map[string]intentRevision) parser.Intent {
	if r, ok := applied[in.Slug]; ok {
		return materialise(in.Slug, r.version)
	}
	return in
}
