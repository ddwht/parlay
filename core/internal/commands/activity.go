// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: activity-declaration

package commands

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
)

// ActivityUnavailable is the display token for a feature whose activity
// declaration exists but cannot be trusted.
//
// It is deliberately NOT `unclassified`. Unclassified means the project
// has said nothing, which is an ordinary state a person can resolve by
// deciding. A declaration that is present and broken is a different fact
// with a different fix, and collapsing the two would hide a corrupt file
// inside the pile of features nobody has got to yet — the one pile
// guaranteed not to be read carefully.
//
// It is also not the feature's last known state. A file that cannot be
// parsed cannot be said to mean what it meant yesterday.
const ActivityUnavailable = "unavailable"

// activityReading is the result of reading one feature's declaration.
//
// The fields are unexported and the state is only reachable through
// Resolve, which is the whole point: parse-safe is not authoring-valid,
// and a caller holding a *parser.Activity can call Current on a
// declaration whose history is empty and get a confident `unclassified`
// out of a file that declares nothing. Making the valid path the only
// path is cheaper than remembering to validate at four call sites.
type activityReading struct {
	activity *parser.Activity
	declared bool

	// parseErr is set when the file exists but could not be read or
	// parsed at all. Distinct from problems: a parse refusal means the
	// declaration has no structure to fault, which is a different report
	// from a file that parsed and then failed its authoring rules.
	parseErr error

	// problems are the TYPED shape faults, all of them.
	//
	// An earlier cut kept only the first problem's English message. That
	// threw away exactly the typing we had just built: every unusable
	// declaration collapsed into "not parseable", so a gated file with an
	// empty history was reported as unparseable when it had parsed
	// perfectly well and failed a different rule with its own published
	// code. Keeping the kinds means every surface can publish the same
	// codes `parlay validate --type activity` does.
	problems []parser.ActivityProblem
}

// readActivity loads and fully validates a feature's declaration.
//
// Both passes run here — parse, which refuses what makes derivation
// unsafe, and shape, which refuses what makes the record meaningless.
// Callers never see the gap between them.
func readActivity(featurePath string) activityReading {
	path := parser.ActivityPath(featurePath)

	activity, declared, err := parser.ParseActivityFile(path)
	if err != nil {
		return activityReading{declared: true, parseErr: err}
	}
	if !declared {
		return activityReading{}
	}
	return activityReading{
		activity: activity,
		declared: true,
		problems: parser.ValidateActivityShape(activity),
	}
}

// unusable reports whether the declaration cannot be trusted.
func (r activityReading) unusable() bool {
	return r.parseErr != nil || len(r.problems) > 0
}

// Resolve reports the activity state to display.
//
// observedBoundary is whether the pipeline has been seen doing anything
// with this feature — a passed or blocked gate. It resolves the case the
// file cannot answer alone: no declaration plus observed work is `active`,
// because reporting a missing disposition for work whose activity is
// already evident is a permanent non-problem, and a status line that
// prints those stops being read.
//
// An unusable declaration short-circuits both. It is never resolved
// against observation, because the question "is this parked" has no
// answer here — not a default one, and not yesterday's.
func (r activityReading) Resolve(observedBoundary bool) string {
	if r.unusable() {
		return ActivityUnavailable
	}
	return string(parser.ResolveActivityState(r.activity, r.declared, observedBoundary))
}

// Detail is the short human suffix a status line prints after the state:
// the parking reason, or the reason the declaration cannot be used.
//
// A state without its reason is half a record. The whole complaint that
// started this feature is that seventeen lines said `dialogs` and none of
// them said why.
func (r activityReading) Detail() string {
	if r.parseErr != nil {
		return r.parseErr.Error()
	}
	if len(r.problems) > 0 {
		return r.problems[0].Message
	}
	if r.activity == nil {
		return ""
	}
	parking, ok := r.activity.LatestParking()
	if !ok {
		return ""
	}
	switch {
	case parking.Until != "":
		return fmt.Sprintf("%s (until %s)", parking.Reason, parking.Until)
	default:
		return parking.Reason
	}
}

// ParkingIsStale reports a parked feature that has since acquired
// pipeline evidence.
//
// The state itself is unaffected — a declaration outranks observation, so
// this feature is still `parked`, because somebody said so and the
// artifacts did not. But a parking made before work resumed is a record
// that has quietly stopped being true, and a listing that prints it as a
// clean `parked` line is asserting a disposition nobody currently holds.
// So the state stays and the staleness is surfaced alongside it, which is
// the only combination that is honest about both facts.
func (r activityReading) ParkingIsStale(observedActivity bool) bool {
	return observedActivity && !r.unusable() && r.Resolve(false) == string(parser.ActivityParked)
}

// Unusable reports whether the declaration exists but cannot be trusted,
// and why. Commands that must refuse to act on a broken record — park
// appending to a history it cannot read, for one — ask this first.
func (r activityReading) Unusable() (string, bool) {
	if !r.unusable() {
		return "", false
	}
	return r.Detail(), true
}

// Diagnostics returns the published code and message for every fault in
// this declaration.
//
// Routed through agent.ActivityDiagnostic, the SAME mapping
// `parlay validate --type activity` uses, so gate and validate can never
// publish different codes for one file. A parse refusal is the one case
// that does not come from a typed kind — parsing fails before kinds
// exist — and it carries the parse code directly.
//
// Every shape fault is returned, not just the first. A file with three
// problems has three findings, because a reader who fixes the one they
// were shown and re-runs should not discover the next one at the same
// cost as the first.
func (r activityReading) Diagnostics() []ActivityDiagnostic {
	if r.parseErr != nil {
		return []ActivityDiagnostic{{
			Code:    agent.ActivityCodeNotParseable,
			Message: r.parseErr.Error(),
			Fix:     agent.ActivityFixNotParseable,
		}}
	}
	out := make([]ActivityDiagnostic, 0, len(r.problems))
	for _, p := range r.problems {
		code, fix := agent.ActivityDiagnostic(p.Kind)
		out = append(out, ActivityDiagnostic{Code: code, Message: p.Message, Fix: fix})
	}
	return out
}

// ActivityDiagnostic is one published finding about a declaration: what
// is wrong, in this instance, and what to do about it.
//
// Fix is carried rather than derived at each display site. A finding that
// names a problem and not its remedy sends the reader back to the schema
// to work out what the tool already knew — and the review command in
// particular exists to hand somebody a decision, which it cannot do while
// only able to label the fault.
type ActivityDiagnostic struct {
	Code    string
	Message string
	Fix     string
}

// validateActivityContent runs the published validator over raw bytes.
// Kept here so the commands package has one route to the same answer the
// CLI's `--type activity` gives, rather than a second opinion.
func validateActivityContent(path string, content []byte) []agent.ValidationOutcome {
	return agent.ValidateActivity(agent.ModeAuthoring, path, content)
}

// activityCell renders one feature's activity for the human listing.
//
// `active` prints as nothing. It is the ordinary case and by far the most
// common, and a column that repeats the unremarkable answer forty times
// buries the four lines somebody needs to act on. The states worth a
// reader's attention are the ones that print.
func activityCell(e featureEntry) string {
	switch e.Activity {
	case string(parser.ActivityActive), "":
		return ""
	case ActivityUnavailable:
		return "unavailable — " + firstLine(e.ActivityDetail)
	case string(parser.ActivityParked):
		cell := "parked"
		if e.ActivityStale {
			// Named, not silently tolerated: this parking was made before
			// work resumed and no longer describes the feature.
			cell = "parked (stale — has artifacts)"
		}
		if e.ActivityDetail != "" {
			cell += " — " + firstLine(e.ActivityDetail)
		}
		return cell
	default:
		return e.Activity
	}
}

// firstLine keeps a multi-line fault from breaking the tabwriter's
// columns. The full text is in the JSON and in the file.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
