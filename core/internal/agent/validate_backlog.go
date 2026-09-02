// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: backlog-item

package agent

import (
	"github.com/ddwht/parlay/core/internal/parser"
)

// BacklogCodeNotParseable is the code a parse refusal carries. Like
// activity's, it is the one code that does not come from a typed problem
// kind — parsing fails before kinds exist.
const BacklogCodeNotParseable = "backlog-item-not-parseable"

// BacklogFixNotParseable names the supported shape.
const BacklogFixNotParseable = "a backlog item is one YAML document with schema_version 1, a kind of defect/gap/debt/idea, a title, and a captured block"

// BacklogUnmappedCode is what a problem kind this switch does not know
// reports.
//
// Loud on purpose. Sending an unknown kind to a real published code would
// be the worst outcome: a mis-routed finding that looks correct to
// everyone downstream. A loud internal code is a bug report; a plausible
// wrong one is a lie.
const BacklogUnmappedCode = "backlog-problem-unmapped"

// Codes for the two immutability guarantees, published here beside the
// rest so a consumer has ONE place to look.
//
// They are not emitted by the single-file validator and cannot be: a
// validator reading one current file has no prior value to compare
// against, so it can never see that `captured` or `history` was edited.
// They are emitted by the MUTATION COMMANDS, which do have the prior
// value, and that is the whole extent of the guarantee — the schema is
// explicit that retrospective tamper detection for direct hand edits is
// deliberately not built.
const (
	CodeBacklogCapturedUpdateForbidden = "backlog-captured-update-forbidden"
	CodeBacklogHistoryUpdateForbidden  = "backlog-history-update-forbidden"
	CodeActivityHistoryUpdateForbidden = "activity-history-update-forbidden"
	// Emitted by next-activity-review, not by a file validator: an
	// undeclared feature has no file to diagnose.
	CodeActivityUndeclared = "activity-undeclared"
)

// ValidateBacklog checks one backlog item.
//
// Two passes with different owners, the same split activity uses. Parse
// refuses what makes the record unreadable — unknown fields, an unknown
// version, a kind or event outside the closed vocabularies, more than one
// document. Shape reports what makes it poor rather than unreadable.
func ValidateBacklog(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	item, err := parser.ParseBacklogBytes(path, content)
	if err != nil {
		return []ValidationOutcome{outcomeWith(mode,
			BacklogCodeNotParseable, err.Error(), path, BacklogFixNotParseable)}
	}
	var out []ValidationOutcome
	for _, p := range parser.ValidateBacklogShape(item) {
		code, fix := BacklogDiagnostic(p.Kind)
		out = append(out, outcomeWith(mode, code, p.Message, path, fix))
	}
	return out
}

// BacklogDiagnostic maps a structural fault to its published code and
// remedy.
//
// Exported so every surface that reports a backlog fault — validate, the
// listing, the review command — routes through ONE mapping. A second copy
// would drift, and the first symptom would be two commands publishing
// different codes for the same file.
func BacklogDiagnostic(kind parser.BacklogProblemKind) (code, fix string) {
	switch kind {
	case parser.BacklogProblemSchemaVersion:
		return "backlog-item-incomplete", "a backlog item is written at schema_version 1"
	case parser.BacklogProblemMissingID:
		return "backlog-item-incomplete", "add `id:` — items are addressed by it"
	case parser.BacklogProblemMissingTitle:
		return "backlog-item-incomplete", "add a one-line `title:` a reader can recognise in a listing"
	case parser.BacklogProblemMissingKind:
		return "backlog-item-incomplete", "set `kind:` to defect, gap, debt or idea"
	case parser.BacklogProblemCaptureMissing:
		return "backlog-item-capture-incomplete", "`captured.at` and `captured.by` are required — an observation nobody can attribute is one nobody can follow up"
	case parser.BacklogProblemTimestamp:
		return "backlog-timestamp-not-rfc3339", "write timestamps as RFC3339, e.g. 2026-09-01T11:00:00Z"
	case parser.BacklogProblemPriority:
		return "backlog-item-priority-invalid", "`priority:` is P0, P1 or P2, or omit it — absent means untriaged"
	case parser.BacklogProblemBecomesMissing:
		return "backlog-disposition-incomplete", "promoted, amended and folded must name what the work became in `becomes:`"
	case parser.BacklogProblemBecomesUnwanted:
		return "backlog-disposition-incomplete", "declined and obsolete name nothing the work became — remove `becomes:`"
	case parser.BacklogProblemReasonMissing:
		return "backlog-disposition-incomplete", "record why — a disposition nobody can review later is not one"
	case parser.BacklogProblemTerminalNotLast:
		return "backlog-terminal-event-not-last", "a terminal event closes the item; no event may follow it"
	case parser.BacklogProblemAboutRef:
		return "backlog-about-ref-invalid", "`about:` holds parlay refs — @feature, @initiative/feature, or @feature/<kind>:<name> with kind operation, surface, infrastructure or domain"
	case parser.BacklogProblemEventAttribution:
		return "backlog-item-capture-incomplete", "every history event needs `at` and `by`"
	default:
		return BacklogUnmappedCode, "this is a bug in parlay: a validation problem kind has no published diagnostic"
	}
}
