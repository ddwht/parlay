// parlay-feature: parlay-tool
// parlay-component: IntentValidationResult

// intents.md and dialogs.md are the two artifacts a designer writes by
// hand, which makes them the likeliest in the pipeline to be malformed —
// and until now the only ones nothing could check. Both schemas shipped
// and deployed; neither type was accepted by `parlay validate`.
//
// Nothing here is a new rule. The intent rules were already implemented
// inside `check-readiness` as a phase gate, in a different finding shape;
// this is the same set, stated once, in the shape every other validator
// uses. check-readiness now calls it rather than carrying a second copy —
// two implementations of "an intent needs a Goal" is how they drift.

package agent

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

// validPriorities is the closed set intent.schema.md documents. An
// omitted Priority is valid — the schema says it defaults to P1.
var validPriorities = map[string]bool{"P0": true, "P1": true, "P2": true}

// ValidateIntentsDeep checks an intents.md against intent.schema.md.
//
// The signature matches the mode-aware validator shape so the CLI can
// wrap it with wrapOutcomeValidator like every other multi-outcome
// validator. content is accepted for signature conformance; the parse
// goes through parser.ParseIntentsFile(path), the same entry point
// check-coverage, diff, sync and check-drift already use, so there is
// exactly one definition of what an intent block is.
func ValidateIntentsDeep(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	intents, err := parser.ParseIntentsFile(path)
	if err != nil {
		return []ValidationOutcome{outcomeWith(mode, "intents-not-readable",
			fmt.Sprintf("cannot parse intents.md: %s", err), path,
			"ensure the file exists and is valid markdown with ## intent headings")}
	}

	if len(intents) == 0 {
		// A freshly scaffolded intents.md has a feature header and no
		// intent blocks yet. That is a legitimate authoring state and a
		// hard build failure, which is exactly the split the mode enum
		// exists for.
		return []ValidationOutcome{outcomeWith(mode, "no-intents",
			"intents.md has no intent blocks", path,
			"add at least one intent (## Title with **Goal**: and **Persona**:)")}
	}

	var outcomes []ValidationOutcome
	seen := map[string]string{}

	for _, intent := range intents {
		where := fmt.Sprintf("%s ## %s", path, intent.Title)

		// Uniqueness is compared on the slug, not the title: the slug is
		// what `@feature/intent-slug` references resolve against, so two
		// titles that differ only in punctuation or case still collide
		// where it matters.
		if first, dup := seen[intent.Slug]; dup {
			outcomes = append(outcomes, outcomeWith(mode, "duplicate-intent-title",
				fmt.Sprintf("intent %q resolves to the same slug %q as %q, so @-references to it are ambiguous",
					intent.Title, intent.Slug, first),
				where,
				"rename one of them so each intent title is unique within the feature"))
		} else {
			seen[intent.Slug] = intent.Title
		}

		if intent.Goal == "" {
			outcomes = append(outcomes, outcomeWith(mode, "missing-goal",
				fmt.Sprintf("intent %q has no Goal", intent.Title), where,
				"add a **Goal**: line stating what the user is trying to accomplish"))
		}

		if intent.Persona == "" {
			outcomes = append(outcomes, outcomeWith(mode, "missing-persona",
				fmt.Sprintf("intent %q has no Persona", intent.Title), where,
				"add a **Persona**: line naming the role performing the action"))
		}

		if p := strings.TrimSpace(intent.Priority); p != "" && !validPriorities[p] {
			outcomes = append(outcomes, outcomeWith(mode, "invalid-priority",
				fmt.Sprintf("intent %q has Priority %q — must be P0, P1 or P2", intent.Title, p), where,
				"set **Priority**: to P0, P1 or P2, or omit the line to default to P1"))
		}
	}

	return outcomes
}

// outcomeWith is NewOutcome plus the two fields a person needs in order
// to act on the finding: where it is and what to do about it.
func outcomeWith(mode ValidationMode, code, message, context, fix string) ValidationOutcome {
	o := NewOutcome(mode, code, message)
	o.Context = context
	o.Fix = fix
	return o
}
