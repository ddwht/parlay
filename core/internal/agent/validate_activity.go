// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: activity-declaration

package agent

import (
	"github.com/ddwht/parlay/core/internal/parser"
)

// ValidateActivity checks one activity.yaml.
//
// Two passes with different owners, and the split matters to every caller:
//
//   - PARSE refuses what makes derivation unsafe — an unknown
//     schema_version, an event outside the closed vocabulary, unknown
//     fields, more than one document. A file failing here has no
//     derivable state at all.
//   - SHAPE reports what makes the record poor rather than unreadable —
//     an empty history, a parking with no reason, missing attribution, a
//     timestamp nobody can parse.
//
// Parse-safe is not authoring-valid. A caller that parses successfully and
// goes straight to deriving state will happily report `unclassified` for
// an empty history, which is a declaration that declares nothing wearing
// the filename of one that does. Callers must run this before Current or
// ResolveActivityState — see requireValidActivity in the commands package.
// ActivityCodeNotParseable is the code a parse refusal carries. A parse
// refusal is a different fact from a shape fault — the file could not be
// read at all — and it is the one code that does not come from a typed
// problem kind, because parsing fails before kinds exist.
const ActivityCodeNotParseable = "activity-declaration-not-parseable"

// ActivityFixNotParseable is the remedy for a parse refusal.
//
// Exported alongside the code because every surface that reports the
// fault owes the reader the same remedy. Embedding it in one call site
// and letting the others invent their own is how two commands end up
// telling somebody two different things about one file.
// The remedy names the SUPPORTED range, not the authoring version. A v1
// declaration is valid — it reads through the migrator chain and
// validates clean — so telling its author to rewrite it would be telling
// them to fix something that is not broken.
const ActivityFixNotParseable = "an activity declaration is one YAML document with a supported schema_version (1 or 2) and a history of events that version admits — parked and unparked in either, activated in 2"

func ValidateActivity(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	activity, err := parser.ParseActivityBytes(path, content)
	if err != nil {
		return []ValidationOutcome{outcomeWith(mode,
			ActivityCodeNotParseable, err.Error(), path, ActivityFixNotParseable)}
	}

	var out []ValidationOutcome
	for _, problem := range parser.ValidateActivityShape(activity) {
		code, fix := ActivityDiagnostic(problem.Kind)
		out = append(out, outcomeWith(mode, code, problem.Message, path, fix))
	}
	return out
}

// ActivityUnmappedCode is what a problem kind this switch does not know
// reports.
//
// It exists because Go cannot check a switch over a string-backed enum
// for exhaustiveness, so a kind added to the parser and forgotten here
// would otherwise fall through to whatever the default said. Sending it
// to activity-declaration-incomplete — a real, published code — would be
// the worst outcome: a mis-routed finding that looks correct to everyone
// downstream. A loud internal code is a bug report; a plausible wrong one
// is a lie. TestActivityDiagnostic_EveryRegisteredKindIsMapped fails before a
// user can ever see this.
const ActivityUnmappedCode = "activity-problem-unmapped"

// ActivityDiagnostic maps a structural fault to its published code and
// remedy. Switching on the KIND rather than on the message is the whole
// point: the diagnostic contract must not be a function of English.
//
// Exported so every surface that reports activity faults — `parlay
// validate --type activity`, `gate --all`, the review command — routes
// through ONE mapping. A second copy in another package would drift, and
// the first symptom of the drift would be two commands publishing
// different codes for the same file.
func ActivityDiagnostic(kind parser.ActivityProblemKind) (code, fix string) {
	switch kind {
	case parser.ProblemSchemaVersion:
		// Reached only by a programmatically built Activity: a parsed
		// one has already been migrated to the current version. So this
		// names the version the tool WRITES, not the range it reads.
		return "activity-declaration-incomplete",
			"an activity declaration is written at schema_version 2"
	case parser.ProblemEmptyHistory:
		return "activity-declaration-incomplete",
			"declare something, or delete the file — an empty declaration is the state it exists to replace"
	case parser.ProblemUnknownEvent:
		return "activity-declaration-not-parseable",
			"event must be `parked`, `unparked` or `activated`"
	case parser.ProblemMissingAt:
		return "activity-declaration-incomplete",
			"add `at:` as an RFC3339 timestamp, e.g. 2026-09-01T11:00:00Z"
	case parser.ProblemTimestampInvalid:
		return "activity-timestamp-not-rfc3339",
			"write `at` as RFC3339, e.g. 2026-09-01T11:00:00Z"
	case parser.ProblemMissingBy:
		return "activity-declaration-incomplete",
			"add `by:` — record who decided, or the next reader learns nothing"
	case parser.ProblemParkedWithoutReason:
		return "activity-parked-without-reason",
			"record why the work was paused — `parlay park @<feature> --reason ...`"
	case parser.ProblemUntilOnUnparked:
		// The code keeps its historical name; the meaning is broader
		// than it, and the remedy has to be accurate rather than
		// matching the name. `until` describes what would end a pause,
		// so it belongs on the only event that starts one.
		return "activity-until-on-unparked",
			"remove `until:` from the non-parked event; it describes what would end a pause, so it belongs only on `parked`"
	default:
		return ActivityUnmappedCode,
			"this is a bug in parlay: a validation problem kind has no published diagnostic"
	}
}
