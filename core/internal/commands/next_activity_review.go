// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: activity-declaration

package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// next-activity-review hands a person ONE feature to decide about.
//
// The shape is lifted from next-legacy-review, and for the same reason it
// exists there: seventeen undifferentiated lines is not a list somebody
// works through, it is a list somebody scrolls past. A wall of findings
// converts into action at roughly the rate of zero, while one question
// with its evidence attached converts at the rate people actually answer
// questions.
//
// It is READ-ONLY. It selects and presents; the decision is made by
// `parlay park`, `parlay activate` or `parlay unpark`, which are the
// commands that carry the attribution. A review command that also wrote
// would be a command that could record a decision nobody made.
var nextActivityReviewCmd = &cobra.Command{
	Use:   "next-activity-review",
	Short: "Emit the next feature whose activity nobody has declared",
	Long: `Select one feature with no activity disposition and emit what is needed to decide it.

Nothing is written. The answer is given with ` + "`parlay park`" + `,
` + "`parlay activate`" + ` or ` + "`parlay unpark`" + `, which record who decided
and why; this command only chooses the next question and assembles its evidence.

An undeclared feature is answered with park (the pause was deliberate) or
activate (it is being worked on). ` + "`unpark`" + ` is for ending a recorded
pause and is offered only where one exists — activate writes its own event
kind, so a feature that was never parked never gains a history claiming a
pause ended.

Order is deliberate. Features with a BROKEN declaration come first — somebody
already tried to say something and the record cannot be read, which is a
smaller and more urgent pile than the features nobody has considered. Stale
parkings come next: a declaration that has quietly stopped being true.
Undeclared features come last, oldest phase first, because a feature that
never got past planning is the cheapest kind to decide about.

Pass --exclude for each feature already handled in this sitting. Nothing is
persisted, so without exclusions a run would eventually hand back a feature
somebody has just looked at and chosen not to decide.`,
	Args: cobra.NoArgs,
	RunE: runNextActivityReview,
}

var nextActivityExclude []string

func init() {
	nextActivityReviewCmd.Flags().StringArrayVar(&nextActivityExclude, "exclude", nil,
		"feature already handled this sitting (repeatable)")
}

// activityReviewSubject is one feature awaiting a decision.
type activityReviewSubject struct {
	Feature string `json:"feature"`
	// Root is the root this feature lives in. Emitted because the same
	// slug can name different work in a parent and a child, so a subject
	// without it is ambiguous the moment a project has children.
	Root string `json:"root"`
	// Exclude is the exact token to pass back as --exclude. Emitted
	// rather than left for the caller to build: a caller reconstructing a
	// root-qualified key is a caller who can get the separator wrong and
	// silently exclude nothing.
	Exclude string `json:"exclude"`
	Phase   string `json:"phase"`
	// Why is the reason this feature needs a person: the fault in a
	// broken declaration, the staleness, or simply that nothing has been
	// said.
	Why string `json:"why"`
	// Kind is that same reason as a token, so ordering and any caller's
	// grouping switch on a value rather than on the prose. The prose is
	// for a person; this is for the code that arranges it.
	Kind string `json:"kind"`
	// Findings are the published diagnostics, with their fixes. Carried
	// so the reviewer is told how to resolve the fault rather than only
	// that one exists.
	Findings []ActivityDiagnostic `json:"findings,omitempty"`
	// Options are the decisions available, as commands to run. Emitted
	// rather than described so the caller builds a chooser from them
	// instead of composing invocations itself and getting a flag wrong.
	Options []activityReviewOption `json:"options"`
}

// activityReviewOption is one decision available to the reviewer.
//
// Command and Path are alternatives, and which one is set says how the
// caller should present the entry. Command is an exact invocation to run;
// Path names a file the person must act on themselves. Nothing carries
// both, and an option with neither is a choice that needs no tool.
type activityReviewOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Command is an exact, runnable invocation. Present only when the
	// tool can actually accept it — an option the caller runs and gets
	// refused costs them the attempt before they learn it was never
	// available.
	Command string `json:"command,omitempty"`
	// Argv is the same invocation, already split into arguments. A
	// caller that re-derives arguments by splitting Command on
	// whitespace gets it wrong for any value carrying a space or a
	// quote — a root name, a reason — so the structured form is emitted
	// alongside rather than left to be reconstructed.
	Argv []string `json:"argv,omitempty"`
	// Path is the file the person must edit or remove. Present instead
	// of Command when no exact invocation exists.
	Path string `json:"path,omitempty"`
}

type activityReviewOutput struct {
	Root string `json:"root"`
	// RootsExamined names every root walked, so a reader can tell an
	// empty review from one that never looked at the children.
	RootsExamined []string `json:"roots_examined"`
	// RootErrors names roots that could not be enumerated. A child that
	// failed must not vanish into a clean summary — "nothing left to
	// review" while a child holds unclassified work is false.
	RootErrors []string `json:"root_errors,omitempty"`
	// Summary is the same projection `gate --all` reports, emitted here
	// so a reviewer sees one coherent snapshot rather than correlating
	// two calls that may have loaded different states.
	Summary activityReviewSummary `json:"summary"`
	// Subject is absent when there is nothing left to decide, which is
	// how a caller knows the sitting is over.
	Subject *activityReviewSubject `json:"subject,omitempty"`
	Note    string                 `json:"note"`
}

type activityReviewSummary struct {
	Total        int `json:"total"`
	Active       int `json:"active"`
	Parked       int `json:"parked"`
	Unclassified int `json:"unclassified"`
	Unavailable  int `json:"unavailable"`
	StaleParking int `json:"stale_parking"`
	Remaining    int `json:"remaining"`
}

func runNextActivityReview(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	// A bare parent — every feature living in child roots — is a
	// supported topology, and status renders it without complaint. A
	// review command that errored there would refuse to run in a project
	// whose only fault is that its work lives one level down.
	excluded := map[string]bool{}
	for _, e := range nextActivityExclude {
		excluded[e] = true
	}

	out := activityReviewOutput{Root: cfg.Root.Path}
	var summary activityReviewSummary
	var pending []activityReviewSubject

	// Walk the active root AND every child. A parent review reporting
	// "every feature has a disposition" while a child holds unclassified
	// work is false — and doctor invokes this from the parent, so
	// refusing with routing instructions would only move the problem.
	for _, target := range activityReviewRoots(cfg) {
		// Named whether or not it could be read. A root missing from the
		// examined list is indistinguishable from one that does not
		// exist.
		out.RootsExamined = append(out.RootsExamined, target.name)

		if target.err != nil {
			// Surfaced, never swallowed: a child that could not be read
			// might hold anything, and a clean summary would assert it
			// holds nothing.
			out.RootErrors = append(out.RootErrors, fmt.Sprintf("%s: %v", target.name, target.err))
			continue
		}
		summary.Total += len(target.features)

		for _, slug := range target.features {
			phase := ComputeFeaturePhase(target.ctx, slug)
			reading := readActivity(target.ctx.FeaturePath(slug))
			observed := HasObservedPipelineActivity(phase)
			state := reading.Resolve(observed)
			stale := reading.ParkingIsStale(observed)

			switch state {
			case string(parser.ActivityActive):
				summary.Active++
			case string(parser.ActivityParked):
				summary.Parked++
			case ActivityUnavailable:
				summary.Unavailable++
			default:
				summary.Unclassified++
			}
			if stale {
				summary.StaleParking++
			}

			needs, kind, why := activityNeedsReview(state, stale)
			if !needs {
				continue
			}
			summary.Remaining++
			token := activityExcludeToken(target.name, slug)
			if excluded[token] || excluded[slug] && target.isActive {
				continue
			}
			findings := reading.Diagnostics()
			// The undeclared case has no file to diagnose, so the
			// declaration validator can never emit it — but it IS the
			// finding, and a reviewer keying on codes would otherwise
			// see this one subject arrive with none. A WARNING, because
			// an undeclared feature is not malformed; nobody has said
			// anything about it yet, which is the fact this whole axis
			// exists to make visible rather than a fault to fix.
			if state == string(parser.ActivityUnclassified) {
				findings = append(findings, ActivityDiagnostic{
					Code:    agent.CodeActivityUndeclared,
					Message: "no declaration, no pipeline evidence — nobody has said whether this is active or paused",
					Fix:     "declare it: `parlay activate` if work is under way, `parlay park --reason` if it is paused on purpose",
				})
			}
			pending = append(pending, activityReviewSubject{
				Feature:  slug,
				Root:     target.name,
				Exclude:  token,
				Phase:    string(phase),
				Why:      why,
				Kind:     kind,
				Findings: findings,
				Options: activityReviewOptions(slug, target.rootName,
					parser.ActivityPath(target.ctx.FeaturePath(slug)), state, stale),
			})
		}
	}

	sortActivityReviewSubjects(pending)
	out.Summary = summary
	switch {
	case len(pending) > 0:
		out.Subject = &pending[0]
		out.Note = "Nothing has been written. Answer with one of the commands in options, then re-run with --exclude " + pending[0].Exclude + " to move on."
	case summary.Remaining > 0:
		out.Note = "Every remaining feature was excluded this sitting. Re-run without --exclude to revisit them."
	case len(out.RootErrors) > 0:
		// Never "everything has a disposition" while a root could not be
		// read. What that root holds is unknown, and unknown is not
		// clean.
		out.Note = "Nothing to review in the roots that could be read, but " +
			fmt.Sprintf("%d root(s) could not be enumerated — see root_errors. Their features have not been checked.", len(out.RootErrors))
	default:
		out.Note = "Every feature has an activity disposition. Nothing left to review."
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// activityNeedsReview decides whether a feature belongs in the queue, and
// says why in the words the reviewer will read.
//
// `active` and a healthy `parked` need nobody: both are answered
// questions. Everything else is a question somebody still owes.
func activityNeedsReview(state string, stale bool) (needs bool, kind, why string) {
	switch {
	case state == ActivityUnavailable:
		return true, reviewKindUnavailable, "the activity declaration exists but cannot be read"
	case stale:
		return true, reviewKindStale, "parked, but the feature has since acquired artifacts — the parking no longer describes it"
	case state == string(parser.ActivityUnclassified):
		return true, reviewKindUndeclared, "nobody has said whether this pause was chosen"
	default:
		return false, "", ""
	}
}

// The reasons a feature can be in the queue, as tokens.
const (
	reviewKindUnavailable = "unavailable"
	reviewKindStale       = "stale-parking"
	reviewKindUndeclared  = "undeclared"
)

// activityReviewOptions emits the decisions available as commands.
//
// There is deliberately no "skip" option and no default. Skipping is
// --exclude, which is the caller's business and leaves no record; a
// default would let an unattended run answer a question about somebody's
// intent, which is precisely the judgment this command exists to route to
// a person.
func activityReviewOptions(slug, rootName, declarationPath, state string, stale bool) []activityReviewOption {
	ref := "@" + slug

	// Argv alongside Command for EVERY option that carries one.
	//
	// activityReviewOption is a shared type. When backlog's options
	// started emitting Argv and these did not, a consumer taught to
	// prefer the structured form got a runnable invocation for backlog
	// and an empty slice here — nothing to run, and no error saying so.
	// A field on a shared type is a promise made on behalf of every
	// producer of that type, so either all of them keep it or it does
	// not belong on the shared type.
	build := func(verb string, tail ...string) ([]string, string) {
		argv := []string{"parlay"}
		human := "parlay "
		if rootName != "" {
			argv = append(argv, "--root", rootName)
			human += "--root " + shellQuote(rootName) + " "
		}
		argv = append(argv, verb, ref)
		argv = append(argv, tail...)
		human += verb + " " + ref + " " + strings.Join(quoteAll(tail), " ")
		return argv, strings.TrimSpace(human)
	}

	parkArgv, parkCmd := build("park", "--reason", "<why>", "--by", "<who>")
	activateArgv, activateCmd := build("activate", "--by", "<who>")
	unparkArgv, unparkCmd := build("unpark", "--by", "<who>")

	park := activityReviewOption{
		ID:      "park",
		Label:   "The pause was deliberate — record why",
		Command: parkCmd,
		Argv:    parkArgv,
	}
	activate := activityReviewOption{
		ID:      "activate",
		Label:   "Work is active — declare it, and the feature leaves this queue",
		Command: activateCmd,
		Argv:    activateArgv,
	}
	unpark := activityReviewOption{
		ID:      "unpark",
		Label:   "Work is active — clear the parking",
		Command: unparkCmd,
		Argv:    unparkArgv,
	}

	// EVERY option here must be a command the tool will actually accept.
	// An option the caller runs and gets refused costs them the attempt
	// before they learn it was never available — the same failure as a
	// remedy that names an unreachable command.
	switch {
	case state == ActivityUnavailable:
		// No park, activate or unpark: all three refuse to append to a
		// declaration they cannot read.
		//
		// Repair is NOT emitted as a command, because there is no exact
		// command to emit. The file is colocated in the feature
		// directory rather than the caller's cwd, `$EDITOR` may carry
		// arguments, and "fix it" and "delete it and start over" are two
		// different decisions with the same tool. An options list whose
		// contract is "run this" must not contain an entry that is
		// really a suggestion — so these carry the PATH and say what to
		// do with it, and the caller presents them as instructions.
		return []activityReviewOption{
			{
				ID:    "repair",
				Label: "Edit the declaration to fix the fault reported above",
				Path:  declarationPath,
			},
			{
				ID:    "discard",
				Label: "Delete the declaration and start over — the feature returns to unclassified",
				Path:  declarationPath,
			},
		}
	case stale:
		// Parked already, so park would be refused as a no-op
		// transition. Unpark is the only move.
		return []activityReviewOption{unpark}
	default:
		// Undeclared. `unpark` is NOT offered — there is no parking to
		// clear and the command refuses it — but `activate` is, and it
		// has to be: an earlier cut offered "leave it undeclared" here,
		// which resolved nothing. The feature stayed unclassified,
		// remained in the remaining count, and returned next sitting. A
		// one-time triage whose only options are "park it" and "do
		// nothing" can never reduce its own backlog.
		return []activityReviewOption{park, activate}
	}
}

// sortActivityReviewSubjects orders the queue.
//
// Broken declarations first: somebody already tried to say something and
// the record cannot be read, which is both more urgent and a much smaller
// pile than the features nobody has considered. Stale parkings next — a
// record that has quietly stopped being true. Undeclared last, earliest
// phase first, because a feature that never got past planning is the
// cheapest kind to decide about and clearing cheap ones first is how a
// sitting builds momentum.
func sortActivityReviewSubjects(subjects []activityReviewSubject) {
	rank := func(s activityReviewSubject) int {
		switch s.Kind {
		case reviewKindUnavailable:
			return 0
		case reviewKindStale:
			return 1
		default:
			return 2
		}
	}
	phaseRank := map[string]int{
		string(PhasePlanned): 0, string(PhaseIntents): 1, string(PhaseDialogs): 2,
		string(PhaseArtifacts): 3, string(PhaseBuild): 4, string(PhaseDone): 5,
	}
	sort.SliceStable(subjects, func(i, j int) bool {
		if ri, rj := rank(subjects[i]), rank(subjects[j]); ri != rj {
			return ri < rj
		}
		if pi, pj := phaseRank[subjects[i].Phase], phaseRank[subjects[j].Phase]; pi != pj {
			return pi < pj
		}
		if subjects[i].Root != subjects[j].Root {
			return subjects[i].Root < subjects[j].Root
		}
		return subjects[i].Feature < subjects[j].Feature
	})
}

// rootTarget is one root to walk, and the shape every multi-root read in
// this package uses. Named generically because a second copy of this walk
// is a second place for the child-error handling to be got wrong — which
// it already was once, when failed children were dropped before they
// could be reported.
type rootTarget = activityReviewTarget

// activityReviewTarget is one root to walk.
type activityReviewTarget struct {
	name     string
	ctx      *config.Context
	isActive bool
	// features is what the walk already enumerated, so a healthy child is
	// not listed twice.
	features []string
	// err is a child that could not be enumerated. Carried as a TARGET
	// rather than dropped: an earlier cut returned early on the failure,
	// so the child vanished from the walk entirely — absent from
	// RootsExamined, absent from RootErrors, and absent from a summary
	// that then claimed everything had a disposition. A child that could
	// not be read might hold anything.
	err error
	// rootName is the bare root to target, empty for the active root.
	// ONE value, from which both the pasteable Command and the
	// structured Argv are built. It used to be stored pre-rendered as
	// "--root 'name' " and the argv builder reverse-parsed the quoting
	// back out — a shell decoder that existed only because the
	// presentation string predated the structured form.
	rootName string
	// rootFlag is the `--root <name>` fragment an emitted command needs,
	// empty for the active root. Emitted rather than left to the caller:
	// a command that omits it runs against the wrong root and reports
	// success.
	rootFlag string
}

// activityReviewRoots is the active root plus every registered child.
//
// Same walk `gate --all` does, and for the same reason: doctor invokes
// this from the parent, so a review that looked only at the active root
// would tell somebody standing in a bare parent that a project full of
// undeclared features has nothing to review.
func activityReviewRoots(cfg *config.Context) []activityReviewTarget {
	active := activityReviewTarget{name: activeRootLabel(cfg), ctx: cfg, isActive: true}
	active.features, active.err = cfg.AllFeatures()
	if active.err != nil && errors.Is(active.err, os.ErrNotExist) {
		// A bare parent — every feature in child roots — is a supported
		// topology that status renders without complaint.
		active.err = nil
		active.features = nil
	}
	targets := []activityReviewTarget{active}

	if cfg.Root.Kind != config.RootKindParent || cfg.Index == nil {
		return targets
	}
	walkChildRoots(cfg, func(name, path string, childCtx *config.Context, childFeatures []string, unavailable error) {
		t := activityReviewTarget{
			name:     name,
			ctx:      childCtx,
			features: childFeatures,
			err:      unavailable,
			rootName: name,
			rootFlag: fmt.Sprintf("--root %s ", shellQuote(name)),
		}
		if t.err == nil && childCtx == nil {
			t.err = fmt.Errorf("child root %s could not be resolved", name)
		}
		targets = append(targets, t)
	})
	return targets
}

// activityExcludeToken is the exact --exclude value for one subject.
//
// Root-qualified, because two roots may hold the same slug and an
// unqualified exclusion would silently skip the wrong one — or both.
func activityExcludeToken(root, slug string) string {
	return root + ":" + slug
}

// shellQuote wraps a value in single quotes when it needs them, so an
// emitted command survives being pasted into a shell. A root name with a
// space that arrives unquoted becomes two arguments and a confusing
// error.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '/' {
			continue
		}
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
