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
	if len(a.Affects) == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "amendment-affects-missing",
			"affects: is empty — an amendment that names no contract entry cannot be applied or scoped; name the operations/fragments it changes"))
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

	return outcomes
}
