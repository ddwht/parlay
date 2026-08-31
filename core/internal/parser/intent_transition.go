package parser

import (
	"fmt"
	"strings"
)

// Intent transitions — the evolution algebra.
//
// A founding promise used to have exactly one possible transition: death.
// `supersedes_intents:` said a promise was GONE, and nothing could say it now
// READS DIFFERENTLY. So every evolution was modelled as a retirement, every
// retirement orphaned the contract entries the promise justified, and the
// accounting rule existed to make a human clean up after an orphaning the tool
// itself induced.
//
// `amends_intents:` is the replacement vocabulary. The slug is a durable
// decision LINEAGE, never reused; an amendment creates the next version of it.
// Attribution binds to the lineage, so an entry sourced to a promise stays
// ATTRIBUTED across a revision — whether it stays JUSTIFIED is a human
// judgement the tool cannot make, and says so.

// IntentMode is the closed set of transitions a promise can undergo.
type IntentMode string

const (
	// IntentExtend adds to a promise without withdrawing any of it. Prior
	// entries stay attributed and the author attests their support survives.
	IntentExtend IntentMode = "extend"
	// IntentRevise replaces the promise text. Scope may move in either
	// direction, so it needs the promise delta approved.
	IntentRevise IntentMode = "revise"
	// IntentNarrow weakens the promise. Some entries may lose justification,
	// so at least one narrowing consequence must be declared.
	IntentNarrow IntentMode = "narrow"
	// IntentRetire ends the lineage. Nothing takes the promise over.
	IntentRetire IntentMode = "retire"
	// IntentLegacySupersession is what a pre-vocabulary `supersedes_intents:`
	// entry reads as.
	//
	// NOT "retire". Executing it as retirement is operationally safe and
	// faithful to what the old resolver did; it is not necessarily faithful to
	// what the AUTHOR meant, because retirement was the only available
	// spelling and an author intending a revision had no way to say so.
	// Relabelling it would rewrite history to match a vocabulary that did not
	// exist when it was written.
	IntentLegacySupersession IntentMode = "legacy_supersession"
)

// KnownIntentMode reports whether a mode may be WRITTEN by a new record.
// legacy_supersession is derived on read and is never authored.
func KnownIntentMode(m IntentMode) bool {
	switch m {
	case IntentExtend, IntentRevise, IntentNarrow, IntentRetire:
		return true
	}
	return false
}

// EndsLineage reports whether a mode takes the promise out of force.
func (m IntentMode) EndsLineage() bool {
	return m == IntentRetire || m == IntentLegacySupersession
}

// CarriesNewText reports whether a mode supplies a replacement proposition.
func (m IntentMode) CarriesNewText() bool {
	return m == IntentExtend || m == IntentRevise || m == IntentNarrow
}

// IntentVersion is a COMPLETE new version of a promise.
//
// A snapshot, not a patch. The design chose snapshots precisely to avoid patch
// algebra, so omission means ABSENT — a field left out of a version is cleared,
// not inherited. An earlier draft replaced only goal and verify and silently
// inherited everything else, which was neither a full snapshot nor a defined
// patch, and could not clear a field at all.
//
// The lineage slug is deliberately OUTSIDE this struct: it is the one field a
// transition may never change, because attribution binds to it. Everything a
// consumer treats as the promise's current state lives here — including Title,
// so a human-facing name can change without breaking identity, which is the
// whole point of having a stable slug.
type IntentVersion struct {
	Title       string   `yaml:"title"`
	Goal        string   `yaml:"goal"`
	Persona     string   `yaml:"persona"`
	Priority    string   `yaml:"priority"`
	Context     string   `yaml:"context"`
	Action      string   `yaml:"action"`
	Objects     []string `yaml:"objects"`
	Constraints []string `yaml:"constraints"`
	Verify      []string `yaml:"verify"`
	Questions   []string `yaml:"questions"`
}

// IntentAmendment is one authored transition of one lineage.
type IntentAmendment struct {
	Intent string     `yaml:"intent"`
	Mode   IntentMode `yaml:"mode"`
	// Version is the promise's complete new text. A pointer so that "no
	// version block" is distinguishable from "an empty one": the first is
	// required by the modes that end a lineage, the second would be a promise
	// that says nothing.
	Version *IntentVersion `yaml:"version"`
}

// AttestationFor returns the plain-language claim a human is being asked to
// make for a mode.
//
// Recorded in the approval payload verbatim, because the receipt must say what
// was asserted rather than merely that something was. Phrased as a first-person
// claim, never as a result the tool checked: neither statement is verifiable
// from prose, and the ceremony must not imply otherwise.
func AttestationFor(m IntentMode) string {
	switch m {
	case IntentExtend:
		return "I confirm the new version adds to this promise and removes or weakens none of it."
	case IntentRevise:
		return "I approve this before/after replacement and the stated downstream scope impact."
	case IntentNarrow:
		return "I approve this narrowing and the dispositions of every entry that loses support."
	case IntentRetire:
		return "I approve ending this promise, with nothing taking it over."
	}
	return ""
}

// IntentTransition is the normalised view a consumer reads, merging the
// authored `amends_intents:` with the legacy `supersedes_intents:` spelling so
// no consumer has to know there were ever two fields.
//
// ONE discriminant. An earlier draft carried both a legacy_supersession mode
// and a separate Legacy bool — redundant state that can disagree. The mode
// already states both the operational behaviour and the epistemic status.
type IntentTransition struct {
	Intent  string
	Mode    IntentMode
	Version *IntentVersion
}

// IsLegacy reports whether this transition came from the pre-vocabulary
// spelling, and therefore has UNKNOWN semantics.
func (t IntentTransition) IsLegacy() bool { return t.Mode == IntentLegacySupersession }

// IntentTransitions normalises both spellings, in a stable order: authored
// transitions first in file order, then legacy ones.
func (a *Amendment) IntentTransitions() []IntentTransition {
	var out []IntentTransition
	for _, ia := range a.AmendsIntents {
		out = append(out, IntentTransition{Intent: ia.Intent, Mode: ia.Mode, Version: ia.Version})
	}
	for _, slug := range a.SupersedesIntents {
		out = append(out, IntentTransition{Intent: slug, Mode: IntentLegacySupersession})
	}
	return out
}

// ValidateIntentTransitions checks the shape of a record's transitions.
//
// Shape only. Whether a declared mode is SEMANTICALLY honest — whether prose
// labelled `extend` actually narrows the promise — is not decidable here or
// anywhere, and no check in this file should be read as claiming otherwise.
func (a *Amendment) ValidateIntentTransitions() []string {
	var problems []string
	seen := map[string]IntentMode{}

	for _, ia := range a.AmendsIntents {
		if ia.Intent == "" {
			problems = append(problems, "an amends_intents entry names no intent")
			continue
		}
		if !KnownIntentMode(ia.Mode) {
			if ia.Mode == IntentLegacySupersession {
				problems = append(problems, fmt.Sprintf(
					"amends_intents %q declares mode %q, which is derived when reading an older "+
						"record and may not be authored", ia.Intent, ia.Mode))
			} else {
				problems = append(problems, fmt.Sprintf(
					"amends_intents %q declares mode %q; the vocabulary is closed: extend, "+
						"revise, narrow, retire", ia.Intent, ia.Mode))
			}
			continue
		}
		if ia.Mode.CarriesNewText() {
			if ia.Version == nil {
				problems = append(problems, fmt.Sprintf(
					"amends_intents %q is a %s but supplies no version: — a promise that now "+
						"reads differently has to say how", ia.Intent, ia.Mode))
			} else {
				problems = append(problems, ia.Version.problems(ia.Intent)...)
			}
		}
		if ia.Mode.EndsLineage() && ia.Version != nil {
			problems = append(problems, fmt.Sprintf(
				"amends_intents %q ends the lineage but also supplies a version: — a promise "+
					"that is over does not also read differently", ia.Intent))
		}
		if strings.ContainsAny(ia.Intent, "@/") {
			problems = append(problems, fmt.Sprintf(
				"amends_intents %q is qualified; a transition names a founding promise in its "+
					"OWN feature by bare slug, because one feature may never change another's "+
					"promise", ia.Intent))
		}
		if prev, dup := seen[ia.Intent]; dup {
			problems = append(problems, fmt.Sprintf(
				"amends_intents names %q twice (%s then %s); one record states one transition "+
					"per lineage", ia.Intent, prev, ia.Mode))
		}
		seen[ia.Intent] = ia.Mode
	}

	for _, slug := range a.SupersedesIntents {
		if _, dup := seen[slug]; dup {
			problems = append(problems, fmt.Sprintf(
				"%q appears in both amends_intents and supersedes_intents; a record must state "+
					"one transition for a lineage, in one vocabulary", slug))
		}
	}
	return problems
}

// validPriorities mirrors the founding-intent rule. Omitted defaults to P1.
var validPriorities = map[string]bool{"P0": true, "P1": true, "P2": true}

// problems holds a version snapshot to the SAME minimum validity as the
// founding intent it replaces.
//
// Without this, "omission means cleared" silently turns required identity and
// presentation fields into clearable ones: a version could carry a goal and
// nothing else, and materialise would make a titleless, personaless promise the
// feature's current one. A projected current version must never be structurally
// weaker than the founding version it replaces.
//
// The list fields stay clearable on purpose — being able to remove a
// constraint, an object or an answered question is the point of a snapshot.
func (v *IntentVersion) problems(lineage string) []string {
	var out []string
	if strings.TrimSpace(v.Title) == "" {
		out = append(out, fmt.Sprintf(
			"amends_intents %q supplies a version with no title — a founding intent gets its "+
				"title from its heading, so a version must state one rather than clear it", lineage))
	}
	if strings.TrimSpace(v.Goal) == "" {
		out = append(out, fmt.Sprintf(
			"amends_intents %q supplies a version with no goal — the version is a complete "+
				"snapshot, so an omitted goal clears the promise rather than keeping the old one",
			lineage))
	}
	if strings.TrimSpace(v.Persona) == "" {
		out = append(out, fmt.Sprintf(
			"amends_intents %q supplies a version with no persona — a promise is made to "+
				"somebody, and a founding intent may not omit it either", lineage))
	}
	if p := strings.TrimSpace(v.Priority); p != "" && !validPriorities[p] {
		out = append(out, fmt.Sprintf(
			"amends_intents %q supplies a version with priority %q — must be P0, P1 or P2, or "+
				"omitted to default to P1", lineage, p))
	}
	return out
}

// ScopeImpact is the author's declaration about what a transition does to the
// contract entries attributed to the lineages it changes.
//
// Exception-plus-closure, not enumeration. The declaration says "every entry
// attributed to these lineages that I have not listed remains semantically
// supported by the new promise", and then lists only the entries whose
// relationship changes. Enumerating the whole attributed population was the
// pain the old vocabulary imposed; enumerating only the exceptions is not that
// pain under a new name.
//
// Declared rather than inferred from the mode. Inferring it would mean the
// ceremony manufacturing a preservation claim the amendment itself never made,
// and the whole point of this field is that a human made it.
type ScopeImpact struct {
	// Version is the declaration's shape version, so a later shape cannot be
	// read as this one.
	Version int `yaml:"version"`
	// PreservesUnlisted is the closure assertion. It is the human's, and no
	// check anywhere establishes it: an entry whose lineage is alive and
	// resolving can still have lost the support of a revised promise, and that
	// mismatch hides behind a valid edge.
	PreservesUnlisted bool `yaml:"preserves_unlisted"`
	// Exceptions name the entries whose relationship changes.
	Exceptions []ScopeException `yaml:"exceptions"`
}

// ScopeException is one entry the closure assertion does NOT cover.
type ScopeException struct {
	// Intent is the lineage this consequence belongs to. Explicit, because a
	// REMOVED entry cannot be assigned to a lineage from current attribution —
	// it is gone. Without it, one exception on one lineage silently satisfies
	// the obligations of another in the same record.
	Intent      string `yaml:"intent"`
	Ref         string `yaml:"ref"`
	Disposition string `yaml:"disposition"`
	// ReplacedBy names where the work went. Required by replaced-by and
	// forbidden by every other disposition — the same rule the feature-level
	// retirement outcome follows, and for the same reason: a reader months
	// later cannot recover from silence whether the work moved or stopped
	// mattering.
	ReplacedBy string `yaml:"replaced_by,omitempty"`
}

// ScopeDispositions is the closed set an exception may declare.
var ScopeDispositions = map[string]bool{
	"retained": true, "revised": true, "removed": true, "replaced-by": true,
}

// ValidateScopeImpact checks the declaration's shape.
func (a *Amendment) ValidateScopeImpact() []string {
	si := a.ScopeImpact
	if si == nil {
		return nil
	}
	var problems []string
	if si.Version != 1 {
		problems = append(problems, fmt.Sprintf(
			"scope_impact declares version %d; this build understands version 1", si.Version))
	}
	// The closure assertion is MODE-AWARE, because it is a claim about a
	// promise that still exists.
	//
	// preserves_unlisted says: the entries this record does not list remain
	// supported by the changed promise. A retirement leaves no changed promise,
	// so on a retire-only record the assertion is not merely unnecessary — it is
	// false, and requiring it meant every retirement carried a false statement
	// into the amendment and into the digest that signs it. Hiding it from the
	// printed ceremony did not remove it from what was signed.
	living, retiring := 0, 0
	for _, tr := range a.IntentTransitions() {
		switch {
		case tr.Mode == IntentRetire:
			retiring++
		case KnownIntentMode(tr.Mode):
			living++
		}
	}
	switch {
	case living == 0 && retiring > 0:
		if si.PreservesUnlisted {
			problems = append(problems,
				"scope_impact asserts preserves_unlisted, but every transition in this record "+
					"ends a promise — there is no promise left to keep supporting the entries "+
					"the record does not list, so the assertion cannot be true. Omit it; the "+
					"exceptions are the complete account")
		}
	case living > 0:
		if !si.PreservesUnlisted {
			problems = append(problems,
				"scope_impact does not assert preserves_unlisted — the declaration is a closure "+
					"over the entries it does not list, so without that assertion it states "+
					"nothing about them and the transition cannot be approved against an "+
					"inventory")
		}
	}
	seen := map[string]bool{}
	for _, ex := range si.Exceptions {
		if strings.TrimSpace(ex.Ref) == "" {
			problems = append(problems, "a scope_impact exception names no ref")
			continue
		}
		if strings.TrimSpace(ex.Intent) == "" {
			problems = append(problems, fmt.Sprintf(
				"scope_impact exception %q names no intent — a consequence belongs to one "+
					"lineage, and a removed entry cannot be assigned to one from what is left in "+
					"the contract", ex.Ref))
		}
		// Canonicalise before comparing anything. A malformed ref would
		// otherwise flow into the absent paths and be accepted as removed, and
		// a non-canonical spelling would make exact comparisons lie.
		canon, cerr := CanonicalScopeRef(ex.Ref)
		if cerr != nil {
			problems = append(problems, fmt.Sprintf("scope_impact exception %q: %v", ex.Ref, cerr))
			continue
		}
		// Identity is (lineage, entry), not entry alone. One contract entry may
		// name several source intents, so a shared entry that disappears owes a
		// consequence under EACH promise that justified it — keying on the ref
		// would reject the second as a duplicate and make the completeness
		// requirement unsatisfiable.
		key := strings.TrimSpace(ex.Intent) + "\x00" + canon
		if seen[key] {
			problems = append(problems, fmt.Sprintf(
				"scope_impact dispositions %s under %q twice; one entry has one fate per lineage",
				canon, strings.TrimSpace(ex.Intent)))
		}
		seen[key] = true
		if !ScopeDispositions[ex.Disposition] {
			problems = append(problems, fmt.Sprintf(
				"scope_impact exception %q declares disposition %q; the set is closed: retained, "+
					"revised, removed, replaced-by", canon, ex.Disposition))
		}
		if strings.TrimSpace(ex.ReplacedBy) != "" {
			if _, rerr := CanonicalScopeRef(ex.ReplacedBy); rerr != nil {
				problems = append(problems, fmt.Sprintf(
					"scope_impact exception %q names replacement %q: %v", canon, ex.ReplacedBy, rerr))
			}
		}
	}
	return problems
}

// CanonicalScopeRef parses a scope ref and returns its canonical spelling, so
// two spellings of one entry cannot compare unequal and a malformed one cannot
// be silently treated as an absent entry.
func CanonicalScopeRef(raw string) (string, error) {
	ref, err := ParseAmendmentRef(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("not a contract entry reference: %w", err)
	}
	return fmt.Sprintf("@%s/%s:%s", ref.Feature, ref.Kind, ref.Name), nil
}
