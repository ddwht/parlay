// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: activity-declaration

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/ledger"
	"github.com/ddwht/parlay/core/internal/parser"
)

// Parking is what a designer does when work stops on purpose.
//
// Before this existed the only way to pause a feature was to stop touching
// it, which is indistinguishable on disk from forgetting about it. `parlay
// status` reported both as the same phase with no disposition, so a
// deliberate placeholder and an abandoned one read identically — and a
// listing that cannot separate them reports a permanent non-problem, which
// is how a status line stops being read.
var parkCmd = &cobra.Command{
	Use:   "park <@feature>",
	Short: "Declare that work on a feature is paused on purpose",
	Long: `Record that a feature is deliberately not being worked on.

The declaration is appended to spec/intents/<feature>/activity.yaml, beside
the feature it describes, and it is append-only: unparking does not erase
the parking, it records the reversal. Two people pausing a feature for
different reasons months apart is two facts, not one overwritten twice.

--reason and --by are required. A pause with no stated reason is
indistinguishable from neglect, which is the state this record exists to
replace, and a declaration nobody can attribute tells the next reader
nothing they did not already know.`,
	Args: cobra.ExactArgs(1),
	RunE: runPark,
}

// Activating is somebody looking at a feature nobody has said anything
// about and declaring it live.
//
// It writes an `activated` event, NOT an `unparked` one. Unparking ends a
// pause, and a feature that was never parked has no pause to end — a
// history saying otherwise would put a false statement in the one record
// whose value is being literally true months later.
//
// It also exists because without it the triage cannot finish. "Leave it
// undeclared" resolves nothing: the feature stays unclassified, returns
// next sitting, and a one-time review that can never reduce its own
// backlog is not a review.
var activateCmd = &cobra.Command{
	Use:   "activate <@feature>",
	Short: "Declare that a feature nobody has classified is being worked on",
	Long: `Record that a feature is active.

For a feature nobody has declared anything about. Unparking is for ending a
pause; this is for confirming there was never one, and the history keeps the
two apart.

Refused when the feature is already parked — unpark it instead — and when it
has already been declared active, because an append that changes nothing still
changes the file.`,
	Args: cobra.ExactArgs(1),
	RunE: runActivate,
}

var unparkCmd = &cobra.Command{
	Use:   "unpark <@feature>",
	Short: "Declare that a parked feature is active again",
	Long: `Record that work has resumed on a parked feature.

Appends an unparked event; the parking that preceded it stays in the
history, because what a reader wants months later is that the pause
happened and ended, not a file that reads as though it never did.`,
	Args: cobra.ExactArgs(1),
	RunE: runUnpark,
}

var (
	parkReason string
	parkUntil  string
	parkBy     string
	unparkBy   string
	activateBy string
)

func init() {
	parkCmd.Flags().StringVar(&parkReason, "reason", "", "why the work is paused (required)")
	parkCmd.Flags().StringVar(&parkUntil, "until", "", "the condition that would end the pause, e.g. \"after adapter-set v2 lands\"")
	parkCmd.Flags().StringVar(&parkBy, "by", "", "who decided (required)")
	unparkCmd.Flags().StringVar(&unparkBy, "by", "", "who decided (required)")
	activateCmd.Flags().StringVar(&activateBy, "by", "", "who decided (required)")
}

func runPark(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	for name, v := range map[string]string{"--reason": parkReason, "--by": parkBy} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required: a pause nobody can attribute, for no stated reason, is indistinguishable from the neglect this record exists to replace", name)
		}
	}
	return appendActivity(cmd, cfg, args[0], parser.ActivityEvent{
		Event:  parser.EventParked,
		Reason: strings.TrimSpace(parkReason),
		Until:  strings.TrimSpace(parkUntil),
		By:     strings.TrimSpace(parkBy),
	})
}

func runUnpark(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if strings.TrimSpace(unparkBy) == "" {
		return fmt.Errorf("--by is required: resuming work is a decision, and one nobody can attribute tells the next reader nothing")
	}
	return appendActivity(cmd, cfg, args[0], parser.ActivityEvent{
		Event: parser.EventUnparked,
		By:    strings.TrimSpace(unparkBy),
	})
}

// appendActivity is the one write path for both commands.
//
// Everything that makes the write safe lives here rather than in either
// caller, because two copies of a lock-read-append-publish sequence is two
// chances to get the ordering wrong.
func runActivate(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if strings.TrimSpace(activateBy) == "" {
		return fmt.Errorf("--by is required: declaring a feature active is a decision, and one nobody can attribute tells the next reader nothing")
	}
	return appendActivity(cmd, cfg, args[0], parser.ActivityEvent{
		Event: parser.EventActivated,
		By:    strings.TrimSpace(activateBy),
	})
}

func appendActivity(cmd *cobra.Command, cfg *config.Context, ref string, event parser.ActivityEvent) error {
	slug := parser.FeatureSlug(ref)
	featurePath := cfg.FeaturePath(slug)

	// Scope first. A feature that does not exist under the ACTIVE root is
	// not a feature to park: in a multi-root project the same slug can
	// name different work in a parent and a child, and writing to
	// whichever one the path happened to resolve to is how a declaration
	// lands on the wrong project.
	info, err := os.Stat(featurePath)
	switch {
	case os.IsNotExist(err):
		return fmt.Errorf("no feature %q under %s — check the reference, or pass --root", slug, cfg.Root.Path)
	case err != nil:
		return fmt.Errorf("cannot read %s: %w", featurePath, err)
	case !info.IsDir():
		// A same-named regular file is not a feature. Saying so here is
		// better than the I/O error the activity path would produce three
		// steps later, which describes a symptom rather than the mistake.
		return fmt.Errorf("%s is not a feature directory: %s is a file", slug, featurePath)
	}

	// Parking is a PRE-BUILD act. Offering it on a built feature would
	// present a reversible pause where the honest options are an
	// amendment or nothing: the promises are frozen and the contract is
	// live, so "not now" is no longer a thing anybody can truthfully say
	// about it.
	if event.Event == parser.EventParked {
		if built, why := featureIsBuilt(cfg, slug); built {
			return fmt.Errorf(
				"%s is already built (%s) — parking is a pre-build act. To close a built feature, retire it through an amendment with retires_feature:",
				slug, why)
		}
	}

	// An ADVISORY read, for a better message. It decides nothing.
	//
	// Every check it could make is remade under the lock, because a
	// decision taken here is a decision about a file that may have
	// changed by the time the write happens: two `parlay park` processes
	// could both read `active` here and both append `parked`. What this
	// buys is a refusal that can quote the reason in force, which the
	// locked path cannot always do as legibly.
	if reading := readActivity(featurePath); !reading.unusable() {
		if event.Event == parser.EventParked && reading.Resolve(false) == string(parser.ActivityParked) {
			return fmt.Errorf("%s is already parked: %s\nto change the stated reason, unpark it and park it again — both are recorded", slug, reading.Detail())
		}
	}

	event.At = time.Now().UTC().Format(time.RFC3339)

	if err := appendEventToStore(cfg, featurePath, event); err != nil {
		switch {
		case errors.Is(err, errAlreadyParked):
			return fmt.Errorf("%s is already parked — to change the stated reason, unpark it and park it again", slug)
		case errors.Is(err, errNotParked):
			return fmt.Errorf("%s is not parked, so there is nothing to unpark — if nobody has classified it, `parlay activate` is the command that declares it live", slug)
		case errors.Is(err, errAlreadyActive):
			return fmt.Errorf("%s is already declared active", slug)
		case errors.Is(err, errParkedNotUndeclared):
			return fmt.Errorf("%s is parked, so activating it would skip the record of the pause ending — `parlay unpark %s --by <who>` instead", slug, "@"+slug)
		case errors.Is(err, errUnusableDeclaration):
			return fmt.Errorf(
				"%s has an activity declaration that cannot be used: %w\nrefusing to append to it — fix %s by hand, or delete it to start over",
				slug, err, parser.ActivityPath(featurePath))
		}
		return err
	}

	verb := map[parser.ActivityEventKind]string{
		parser.EventParked:    "parked",
		parser.EventUnparked:  "unparked",
		parser.EventActivated: "declared active",
	}[event.Event]
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", verb, slug)
	if event.Reason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  reason: %s\n", event.Reason)
	}
	if event.Until != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  until:  %s\n", event.Until)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  by:     %s\n", event.By)
	return nil
}

// The refusals the locked append can raise. Sentinels rather than
// message matching, so a caller distinguishes them by identity.
var (
	errAlreadyParked       = errors.New("already parked")
	errNotParked           = errors.New("not parked")
	errAlreadyActive       = errors.New("already declared active")
	errParkedNotUndeclared = errors.New("parked, so unpark rather than activate")
	errUnusableDeclaration = errors.New("activity declaration cannot be used")
)

// appendEventToStore is the AUTHORITATIVE write, and every rule the
// command promises is enforced here rather than before the lock.
//
// An earlier cut checked usability and the transition outside the lock
// and only re-parsed inside it. That is not enough, and the gap is not
// theoretical: two `parlay park` processes could both read `active`,
// both pass their checks, and both append a `parked` event, leaving a
// history that violates the one invariant this command exists to
// maintain — that history holds transitions, not repetitions. The same
// window lets a declaration be hand-edited into a parseable-but-invalid
// state between the read and the write, which the append would then
// republish while the command claimed to refuse it.
//
// So the bytes supplied to this callback are the only evidence used.
// Three checks, in the order that matters:
//
//  1. the CURRENT declaration must be usable — parseable and valid;
//  2. the transition must be a transition against that current history;
//  3. the RESULT must itself validate, so a bug here cannot publish a
//     declaration the tool would refuse to read back.
func appendEventToStore(cfg *config.Context, featurePath string, event parser.ActivityEvent) error {
	path := parser.ActivityPath(featurePath)
	store := ledger.New(cfg.Root.Path, path)

	return store.Update(func(current []byte, exists bool) ([]byte, bool, error) {
		activity := &parser.Activity{SchemaVersion: parser.ActivitySchemaVersion}
		if exists {
			parsed, perr := parser.ParseActivityBytes(path, current)
			if perr != nil {
				return nil, false, fmt.Errorf("%w: %v", errUnusableDeclaration, perr)
			}
			if problems := parser.ValidateActivityShape(parsed); len(problems) > 0 {
				return nil, false, fmt.Errorf("%w: %s", errUnusableDeclaration, problems[0].Message)
			}
			activity = parsed
		}
		// Snapshot the LOCKED INPUT, here — not immediately before the
		// append below. Taking it there made the guard compare the
		// append against itself: any mutation between parsing the
		// locked bytes and that line would be inside the baseline and
		// therefore blessed. The schema says this guard protects
		// against future callers, and a baseline taken after those
		// callers would have run protects against nobody.
		wasHistory := append([]parser.ActivityEvent(nil), activity.History...)

		// Injection seam standing in for a future caller's code, placed
		// where such code would actually go: between the parse and the
		// append, alongside the transition checks. Nil in production.
		//
		// It has to sit HERE and not beside the append. Placed there it
		// fell inside the protected region no matter where the snapshot
		// was taken, so the control that moved the snapshot back to its
		// buggy position still passed — the test proved nothing.
		if activityMutateBeforeAppend != nil {
			activityMutateBeforeAppend(activity)
		}

		// The transition, judged against the locked history.
		//
		// activate is the narrow one: it declares a feature nobody has
		// classified, so it is refused both when a pause is in force
		// (unpark is the honest record there) and when the feature has
		// already been declared active (an append that changes nothing
		// still changes the file).
		switch state := activity.Current(); {
		case event.Event == parser.EventParked && state == parser.ActivityParked:
			return nil, false, errAlreadyParked
		case event.Event == parser.EventUnparked && state != parser.ActivityParked:
			return nil, false, errNotParked
		case event.Event == parser.EventActivated && state == parser.ActivityParked:
			return nil, false, errParkedNotUndeclared
		case event.Event == parser.EventActivated && state == parser.ActivityActive:
			return nil, false, errAlreadyActive
		}

		// Same enforcement as the backlog's history: the ONLY legal
		// change is an append. Here it is currently unreachable —
		// there is one `append` and nothing else touches the slice —
		// and that is exactly the situation the backlog side was in
		// before somebody could have added a second line. An invariant
		// that depends on every future author noticing it is not
		// enforced, it is hoped for.
		activity.History = append(activity.History, event)
		if err := activityHistoryOnlyAppended(wasHistory, activity.History); err != nil {
			return nil, false, fmt.Errorf("%s: %w", agent.CodeActivityHistoryUpdateForbidden, err)
		}

		// Never publish something the tool would refuse to read back.
		if problems := parser.ValidateActivityShape(activity); len(problems) > 0 {
			return nil, false, fmt.Errorf("refusing to write a declaration that would not validate: %s", problems[0].Message)
		}

		out, merr := yaml.Marshal(activity)
		if merr != nil {
			return nil, false, fmt.Errorf("serialise declaration: %w", merr)
		}
		return out, true, nil
	})
}

// featureIsBuilt reports whether a feature has build outputs, and names
// the one it found. Presence of either output is enough: a half-built
// feature is past the point where "not now" describes it.
func featureIsBuilt(cfg *config.Context, slug string) (bool, string) {
	for _, name := range []string{"buildfile.yaml", "testcases.yaml"} {
		if fileExistsAt(filepath.Join(cfg.BuildPath(slug), name)) {
			return true, "it has " + name
		}
	}
	return false, ""
}

// activityHistoryOnlyAppended is the activity twin of
// historyOnlyAppended: every existing event survives byte-identically
// and in order, and the only legal change is new events at the end.
func activityHistoryOnlyAppended(before, after []parser.ActivityEvent) error {
	if len(after) < len(before) {
		return fmt.Errorf("history is append-only: %d event(s) were removed", len(before)-len(after))
	}
	for i, was := range before {
		if after[i] != was {
			return fmt.Errorf("history is append-only: event %d (%s, recorded %s by %s) was rewritten",
				i, was.Event, was.At, was.By)
		}
	}
	return nil
}

// activityMutateBeforeAppend is the seam above; nil by default.
var activityMutateBeforeAppend func(*parser.Activity)
