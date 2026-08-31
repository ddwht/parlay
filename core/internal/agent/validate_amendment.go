// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: amendment-artifact
//
// Single-file validation for one amendment. Ledger-level checks — sequence
// integrity, supersedes resolution, affects resolution against the contract
// artifacts — need the whole feature in view and live in
// `parlay internal check-amendments`; this validator covers everything one
// file can be wrong about on its own.

package agent

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

// ValidateAmendment checks one amendment file's shape: frontmatter fields,
// affects: ref syntax, and the presence of the body sections that make the
// record worth keeping.
func ValidateAmendment(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	var outcomes []ValidationOutcome

	a, err := parser.ParseAmendmentBytes(path, content)
	if err != nil {
		return []ValidationOutcome{NewOutcome(mode, "amendment-not-parseable", err.Error())}
	}

	for _, problem := range a.ValidateIntentTransitions() {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-intent-transition-malformed", problem))
	}

	if a.Slug == "" {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-frontmatter-incomplete",
			"missing amendment: — the slug is the amendment's identity and what supersedes: refs point at"))
	}
	if a.Date == "" {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-frontmatter-incomplete",
			"missing date: — a ledger entry without a date cannot be read as history"))
	}
	// affects: may be empty ONLY for a governance amendment — one that
	// supersedes a founding intent. A feature owning no contract artifact has
	// nothing for affects: to resolve against, which is why 18 of 27
	// parlay-tool features had no legal amendment target at all before
	// supersedes_intents: existed. An amendment declaring neither is still
	// nothing: it names no contract entry to splice and no promise to
	// replace, so there is nothing for apply or scoping to act on.
	if len(a.Affects) == 0 && len(a.SupersedesIntents) == 0 && len(a.AmendsIntents) == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-affects-missing",
			"affects: is empty — an amendment that names no contract entry cannot be applied or scoped; name the operations/fragments it changes, or amends_intents: the founding promise this decision changes"))
	}
	for _, raw := range a.Affects {
		if _, err := parser.ParseAmendmentRef(raw); err != nil {
			outcomes = append(outcomes, NewOutcome(mode, "amendment-affects-malformed",
				fmt.Sprintf("affects entry %s", err.Error())))
		}
	}
	if a.Change == "" {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-missing-change",
			"no ## Change section — the record of what changed is the amendment's reason to exist"))
	}
	if len(a.Acceptance) == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-missing-acceptance",
			"no ## Acceptance bullets — a behavior change without acceptance criteria leaves the affected verify: entries unchanged; fine for renames and pure-prose changes, wrong for anything testable"))
	}

	outcomes = append(outcomes, validateIntentSupersession(mode, a)...)
	outcomes = append(outcomes, validateFeatureRetirement(mode, a)...)

	return outcomes
}

// validateIntentSupersession holds an intent-superseding amendment to a higher
// bar than an ordinary one, because it does something no other ledger entry
// does: it withdraws a promise the feature made.
//
// The rename and pure-prose exemptions that make amendment-missing-acceptance a
// warning do not apply here. An ordinary amendment with no Acceptance leaves the
// affected verify: entries alone, which is a real and harmless case. A
// supersession with no Acceptance retires a promise and puts nothing in its
// place — that is not a lighter version of the same thing, it is the failure
// mode the field is named against. supersedes_intents: rather than retires:
// says a commitment may be replaced, not deleted, and this is where that is
// enforced rather than merely implied.
//
// Why is required for the same reason and is otherwise only encouraged: the
// reasoning is the entire content of a decision that removes scope, and the
// frozen intent it supersedes cannot record why it stopped being true.
//
// Structural, not textual, by design. A free-text justification can be produced
// on demand by any author, human or agent, so it cannot carry the weight of
// retiring a promise. What is checked here is that a successor EXISTS; whether
// it is a good one is a question for the decision protocol, which refuses to
// answer it when nobody is present.
//
// Cross-feature and resolution checks are deliberately absent: they need the
// whole feature (and its contract artifacts) in view and live in
// check-amendments, alongside the sequence and supersedes walks.
func validateIntentSupersession(mode ValidationMode, a *parser.Amendment) []ValidationOutcome {
	// Obligations attach to lineage-ENDING BEHAVIOUR, not to which spelling
	// expressed it. What carries over is the SAFETY obligation — say why, and
	// say what is observably true afterwards — not the old spelling's semantic
	// fiction.
	//
	// The distinction matters. Legacy supersession assumed something always
	// replaces a withdrawn promise, because withdrawal was the only verb the
	// vocabulary had. `mode: retire` means the opposite by definition: the
	// lineage ends and nothing takes the promise over. Telling a retire author
	// that "retiring a promise without stating what replaces it is deletion,
	// not supersession" would contradict the mode they explicitly chose, and
	// would rebuild inside the validator exactly the conflation this stage
	// exists to remove.
	if !endsAnyLineage(a) {
		return nil
	}

	var outcomes []ValidationOutcome
	for _, raw := range a.SupersedesIntents {
		if strings.TrimSpace(raw) == "" {
			outcomes = append(outcomes, NewOutcome(mode, "amendment-supersedes-intent-malformed",
				"supersedes_intents: has an empty entry — name the intent slug this decision replaces"))
			continue
		}
		// An intent ref here is a bare slug in this feature's own intents.md.
		// A qualified @feature/slug is the shape that would let one feature
		// retire another's founding promise, which is exactly what the
		// same-feature rule forbids; reject it at the syntax layer so the
		// intent is refused rather than silently resolved somewhere else.
		if strings.ContainsAny(raw, "@/") {
			outcomes = append(outcomes, NewOutcome(mode, "amendment-supersedes-intent-foreign",
				fmt.Sprintf("supersedes_intents entry %q is qualified — an amendment may only supersede an intent in its own feature, so name the bare intent slug. Cross-feature pressure belongs in trigger:", raw)))
		}
	}

	// Acceptance: required by every lineage-ending transition, but for
	// different reasons, so the diagnostic is mode-aware.
	if len(a.Acceptance) == 0 {
		if endsAnyLineageKnowingly(a) {
			outcomes = append(outcomes, NewOutcome(mode, "amendment-supersession-no-successor",
				"this amendment retires a founding promise but has no ## Acceptance bullets — a promise may end with nothing taking it over, and that is what mode: retire means, but what is observably true afterwards still has to be stated. The rename/pure-prose exemption does not apply"))
		} else {
			outcomes = append(outcomes, NewOutcome(mode, "amendment-supersession-no-successor",
				"this amendment supersedes a founding intent but has no ## Acceptance bullets — retiring a promise without stating what replaces it is deletion, not supersession. The rename/pure-prose exemption does not apply: name the criteria that now hold instead"))
		}
	}
	if strings.TrimSpace(a.Why) == "" {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-supersession-no-rationale",
			"this amendment ends a founding promise but has no ## Why — the frozen intent cannot record why it stopped being true, so this is the only place that reasoning will ever exist"))
	}

	return outcomes
}

// endsAnyLineageKnowingly reports whether every lineage this record ends was
// ended by an explicitly declared mode rather than by the legacy spelling.
//
// The split exists so a KNOWN retire is never told something must replace the
// promise: it declared that nothing does. A legacy record keeps the older
// wording, because its author's intent was never recorded and the compatibility
// message is the honest one for an unknown.
func endsAnyLineageKnowingly(a *parser.Amendment) bool {
	knowing := false
	for _, tr := range a.IntentTransitions() {
		if !tr.Mode.EndsLineage() {
			continue
		}
		if tr.Mode == parser.IntentLegacySupersession {
			return false
		}
		knowing = true
	}
	return knowing
}

// RetirementOutcomes is the closed vocabulary for how a feature ended.
//
// Two values and no third, because the difference between them is the entire
// content of the decision: "replaced" sends a later reader somewhere, and
// "obsolete" tells them there is nowhere to go. Silence cannot express either,
// which is why the field is required rather than encouraged.
var RetirementOutcomes = map[string]bool{
	"replaced": true,
	"obsolete": true,
}

// validateFeatureRetirement checks the shape of a terminal retirement record.
//
// Only the half one file can answer. Whether the named intents are exactly the
// live ones, whether the replacement exists and still stands, whether the
// ledger has an unapplied tail, and whether anything still references the
// feature all need more than this file and live in check-amendments.
func validateFeatureRetirement(mode ValidationMode, a *parser.Amendment) []ValidationOutcome {
	var outcomes []ValidationOutcome

	replacement := strings.TrimSpace(a.ReplacementFeature)
	outcome := strings.TrimSpace(a.Outcome)

	if !a.RetiresFeature {
		// The fields are meaningless without the marker, and quietly ignoring
		// them would let an author believe they had retired a feature.
		if outcome != "" || replacement != "" {
			outcomes = append(outcomes, NewOutcome(mode, "amendment-retirement-fields-without-marker",
				"outcome:/replacement_feature: are set but retires_feature: is not — these describe how a feature ended and mean nothing on an ordinary amendment; set retires_feature: true, or drop them"))
		}
		return outcomes
	}

	if !endsAnyLineage(a) {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-retirement-no-intents",
			"retires_feature: is set but the record ends no promise — retiring a feature retires every promise it makes, so name them in amends_intents: with mode: retire (or the legacy supersedes_intents:); check-amendments verifies the set is exactly the live ones"))
	}

	switch {
	case outcome == "":
		outcomes = append(outcomes, NewOutcome(mode, "amendment-retirement-outcome-missing",
			"retires_feature: is set but outcome: is not — record whether the work moved (replaced, with replacement_feature:) or the need is gone (obsolete). A reader months from now cannot recover which from silence"))
	case !RetirementOutcomes[outcome]:
		outcomes = append(outcomes, NewOutcome(mode, "amendment-retirement-outcome-unknown",
			fmt.Sprintf("outcome: %q is outside the closed set {obsolete, replaced}", outcome)))
	case outcome == "replaced" && replacement == "":
		outcomes = append(outcomes, NewOutcome(mode, "amendment-retirement-replacement-missing",
			"outcome: replaced requires replacement_feature: — saying the work moved without saying where leaves a reader exactly where saying nothing would"))
	case outcome == "obsolete" && replacement != "":
		outcomes = append(outcomes, NewOutcome(mode, "amendment-retirement-replacement-unexpected",
			fmt.Sprintf("outcome: obsolete forbids replacement_feature: (%q) — the two say opposite things about whether the work still exists somewhere", replacement)))
	}

	// Self-replacement is NOT checked here: an amendment does not know which
	// feature's ledger it sits in — the feature comes from the path — so that
	// comparison needs the caller's slug and lives in check-amendments beside
	// the other cross-feature resolution.

	return outcomes
}

// endsAnyLineage reports whether a record ends at least one founding promise,
// in either vocabulary.
func endsAnyLineage(a *parser.Amendment) bool {
	for _, tr := range a.IntentTransitions() {
		if tr.Mode.EndsLineage() {
			return true
		}
	}
	return false
}
