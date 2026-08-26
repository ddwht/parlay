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
	if len(a.Affects) == 0 && len(a.SupersedesIntents) == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-affects-missing",
			"affects: is empty — an amendment that names no contract entry cannot be applied or scoped; name the operations/fragments it changes, or supersedes_intents: the founding intent this decision replaces"))
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
	if len(a.SupersedesIntents) == 0 {
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

	if len(a.Acceptance) == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-supersession-no-successor",
			"this amendment supersedes a founding intent but has no ## Acceptance bullets — retiring a promise without stating what replaces it is deletion, not supersession. The rename/pure-prose exemption does not apply: name the criteria that now hold instead"))
	}
	if strings.TrimSpace(a.Why) == "" {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-supersession-no-rationale",
			"this amendment supersedes a founding intent but has no ## Why — the frozen intent cannot record why it stopped being true, so this is the only place that reasoning will ever exist"))
	}

	return outcomes
}
