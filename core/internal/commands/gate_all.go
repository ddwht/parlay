// parlay-feature: parlay-tool/phase-gates
// parlay-component: gate-sweep
//
// `parlay gate --all` — the repo-level backstop. Where the deploy-time-injected
// Step 0 gate (skills.go) catches an agent that follows its module's
// instructions, this sweep catches everything else: for every feature in every
// root it computes the feature's phase, runs the corresponding stage gate, and
// exits non-zero on any blocker. Wire it as a pre-commit hook and a CI job and
// the gate becomes genuinely unskippable.
//
// This is a TOP-LEVEL command on purpose. CI must depend on a stable surface,
// not on `internal` shapes that follow the skills that read them. The verdict
// (exit code) and the table are the contract; the per-finding JSON lives under
// `internal gate`.
//
// It is also, verbatim, the merge gate a future worktree-per-run model needs:
// "gate --all green" becomes the merge condition, and frozen-doc enforcement
// (reject modified files under spec/intents/<feature>/ post-first-build; admit
// only new files under amendments/) joins this same sweep. That extension is
// out of scope here, but the --check flag is shaped for it.

package commands

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var gateAllCmd = &cobra.Command{
	Use:   "gate",
	Short: "Sweep every feature in every root through its phase gate (CI/pre-commit backstop)",
	Long: `Compute each feature's pipeline phase, run the gate for the boundary it sits
at, and exit non-zero if any feature has a blocker. This is the unskippable
layer: the deploy-time-injected Step 0 gate catches agents that follow their
module instructions; this sweep catches everything else.

Run it as a pre-commit hook or a CI job. The exit code is the contract — zero
when every gated feature passes, non-zero on the first blocker.`,
	Args: cobra.NoArgs,
	RunE: runGateAll,
}

var (
	gateAllFlag    bool
	gateAllChecks  []string
	gateAllVerbose bool
)

// gateAllSupportedChecks is the closed set of sweeps this command knows how to
// run. Only phase-gates is implemented today; frozen-docs is reserved for the
// worktree-merge extension so the flag's vocabulary is stable before the check
// behind it exists.
var gateAllSupportedChecks = map[string]bool{
	"phase-gates": true,
	"frozen-docs": false, // recognized, not yet implemented
}

func init() {
	gateAllCmd.Flags().BoolVar(&gateAllFlag, "all", false,
		"Sweep every feature in every root (required — the command runs no per-feature form)")
	gateAllCmd.Flags().StringSliceVar(&gateAllChecks, "check", []string{"phase-gates"},
		"Which sweeps to run (default phase-gates; frozen-docs is reserved for the worktree-merge model)")
	gateAllCmd.Flags().BoolVar(&gateAllVerbose, "verbose", false,
		"List each blocker under its feature, not just the count")
}

// gateSweepRow is one feature's line in the sweep table.
type gateSweepRow struct {
	Root     string
	Feature  string
	Phase    string
	Stage    string
	Passed   bool
	Skipped  bool // too early in the pipeline, or a hand-authored unit — no boundary to gate
	Blockers []gateBlocker
	Err      string

	// Activity is carried on EVERY row, not only skipped ones.
	//
	// An earlier cut attached it to skipped rows alone, reasoning that a
	// gated feature's pass or block already answers the question gate
	// asks. That reasoning was right about the verdict and wrong about
	// staleness, and the mistake made ActivityStale dead code: a stale
	// parking is one with observed pipeline activity, observed activity
	// is what earns a boundary, so a stale parking is ALWAYS on a gated
	// row and never on a skipped one. Gate could not have reported the
	// condition it was supposed to report.
	//
	// So the verdict column still belongs to the boundary — pass and
	// BLOCKED are not diluted — and staleness is appended to it, because
	// a parked feature that has since acquired artifacts is asserting a
	// disposition nobody currently holds.
	Activity       string
	ActivityDetail string
	ActivityStale  bool

	// ActivityFindings are published diagnostics about the declaration
	// itself, kept SEPARATE from Blockers.
	//
	// Separate because they answer a different question. Blockers are
	// what the phase boundary refused; these are what is wrong with the
	// record of whether the feature is being worked on at all. Folding
	// them into Blockers would inflate "BLOCKED (3)" with findings the
	// boundary never made, and a count that means two things means
	// neither.
	//
	// They still make the sweep exit non-zero. Both codes are errors in
	// the schema, and a CI run that stays green over a committed
	// declaration parlay itself refuses to mutate is a gate reporting
	// health it did not verify.
	ActivityFindings []gateBlocker
}

func runGateAll(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	if !gateAllFlag {
		return fmt.Errorf("parlay gate needs --all: it runs no per-feature form (use `parlay internal gate @<feature> --stage <boundary>` for one feature)")
	}
	for _, c := range gateAllChecks {
		impl, known := gateAllSupportedChecks[c]
		if !known {
			return fmt.Errorf("unknown --check %q; supported: phase-gates (frozen-docs is reserved, not yet implemented)", c)
		}
		if !impl {
			fmt.Fprintf(cmd.ErrOrStderr(), "note: --check %s is reserved for the worktree-merge model and does nothing yet; skipping\n", c)
		}
	}
	// Only phase-gates is implemented; the loop above already warned about any
	// reserved value, so there is nothing more to branch on here.

	var rows []gateSweepRow
	rows = append(rows, sweepRoot(cfg, activeRootLabel(cfg))...)

	// Multi-root: a parent sweeps its children too, so a single `gate --all` at
	// the repo-level root is the whole-repo verdict CI wants.
	if cfg.Root.Kind == config.RootKindParent && cfg.Index != nil && len(cfg.Index.Children) > 0 {
		walkChildRoots(cfg, func(name, path string, childCtx *config.Context, childFeatures []string, unavailable error) {
			if unavailable != nil {
				rows = append(rows, gateSweepRow{Root: name, Feature: "—", Err: unavailable.Error()})
				return
			}
			rows = append(rows, sweepFeatures(childCtx, name, childFeatures)...)
		})
	}

	printGateSweep(cmd, rows)

	blocked := false
	for _, r := range rows {
		// Activity findings count. Both codes are errors in the schema,
		// and a sweep that stayed green over a committed declaration
		// parlay itself refuses to mutate would be reporting health it
		// never verified.
		if len(r.Blockers) > 0 || r.Err != "" || len(r.ActivityFindings) > 0 {
			blocked = true
			break
		}
	}
	if blocked {
		return NewExitCodeError(1)
	}
	return nil
}

// sweepRoot enumerates and gates every feature under a single root context.
func sweepRoot(cfg *config.Context, rootLabel string) []gateSweepRow {
	features, err := cfg.AllFeatures()
	if err != nil {
		return []gateSweepRow{{Root: rootLabel, Feature: "—", Err: err.Error()}}
	}
	return sweepFeatures(cfg, rootLabel, features)
}

func sweepFeatures(cfg *config.Context, rootLabel string, features []string) []gateSweepRow {
	rows := make([]gateSweepRow, 0, len(features))
	for _, slug := range features {
		phase := ComputeFeaturePhase(cfg, slug)
		stage, gated := phaseToGateStage(phase)
		// Read the declaration for every feature. On a skipped row it
		// becomes the verdict — that is the bucket the activity axis
		// exists for, and it was seventeen identical lines here. On a
		// gated row it is where staleness lives.
		reading := readActivity(cfg.FeaturePath(slug))
		observed := HasObservedPipelineActivity(phase)
		activity := reading.Resolve(observed)
		detail := reading.Detail()
		stale := reading.ParkingIsStale(observed)

		findings := activityFindings(reading, stale)

		if !gated {
			rows = append(rows, gateSweepRow{
				Root: rootLabel, Feature: slug, Phase: string(phase), Stage: "—",
				Skipped: true, Passed: true,
				Activity: activity, ActivityDetail: detail, ActivityStale: stale,
				ActivityFindings: findings,
			})
			continue
		}
		out, err := computeGate(cfg, slug, stage)
		if err != nil {
			rows = append(rows, gateSweepRow{
				Root: rootLabel, Feature: slug, Phase: string(phase), Stage: stage, Err: err.Error(),
				Activity: activity, ActivityDetail: detail, ActivityStale: stale,
				ActivityFindings: findings,
			})
			continue
		}
		rows = append(rows, gateSweepRow{
			Root: rootLabel, Feature: slug, Phase: string(phase), Stage: stage,
			Passed: out.Passed, Blockers: out.Blockers,
			Activity: activity, ActivityDetail: detail, ActivityStale: stale,
			ActivityFindings: findings,
		})
	}
	return rows
}

// phaseToGateStage maps a feature's current phase to the boundary gate that
// verifies it. A feature sits at the boundary it has most recently crossed:
// artifacts-present gates the build boundary, a buildfile gates the code
// boundary, a complete build gates done. Features earlier than artifacts have
// no boundary these gates cover, and hand-authored units have none at all.
func phaseToGateStage(phase FeaturePhase) (stage string, gated bool) {
	switch phase {
	case PhaseArtifacts:
		return gateStageBuild, true
	case PhaseBuild:
		return gateStageCode, true
	case PhaseDone:
		return gateStageDone, true
	default:
		return "", false
	}
}

func activeRootLabel(cfg *config.Context) string {
	if cfg.Root.Name != "" {
		return cfg.Root.Name
	}
	return "."
}

func printGateSweep(cmd *cobra.Command, rows []gateSweepRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Root != rows[j].Root {
			return rows[i].Root < rows[j].Root
		}
		return rows[i].Feature < rows[j].Feature
	})

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ROOT\tFEATURE\tPHASE\tGATE\tVERDICT")
	passed, blocked, staleParkings, unusableDeclarations := 0, 0, 0, 0
	// The skipped bucket, split. Pass and block semantics for gated
	// features are untouched: this only says what the ungated ones are.
	byActivity := map[string]int{}
	for _, r := range rows {
		verdict := ""
		switch {
		case r.Err != "":
			verdict = "ERROR: " + r.Err
			blocked++
		case r.Skipped:
			verdict = gateActivityVerdict(r)
			// Disposition buckets only. An unusable declaration is
			// counted through its FINDINGS below, on every row alike —
			// counting it here as well printed the same phrase twice,
			// once per counter, and a Contains-only test could not see
			// it.
			if r.Activity != ActivityUnavailable {
				byActivity[r.Activity]++
			}
		case r.Passed:
			verdict = "pass"
			passed++
		default:
			verdict = fmt.Sprintf("BLOCKED (%d)", len(r.Blockers))
			blocked++
		}
		// The boundary owns the verdict; activity facts are APPENDED to
		// it, never folded into it. On a skipped row the verdict already
		// IS the activity, so appending would repeat it.
		if !r.Skipped {
			for _, f := range r.ActivityFindings {
				verdict += "  [" + f.Code + ": " + firstLine(f.Message) + "]"
			}
		}
		// Counted across every row, gated or not, and exactly once.
		// Counted across every row, gated or not, and exactly once —
		// findings are the single owner of this number.
		var countedUnusable bool
		for _, f := range r.ActivityFindings {
			if f.Code == codeParkedFeatureAdvanced {
				staleParkings++
				continue
			}
			// A file with three shape faults is one unusable
			// declaration, not three.
			if !countedUnusable {
				unusableDeclarations++
				countedUnusable = true
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Root, r.Feature, r.Phase, r.Stage, verdict)
		if gateAllVerbose {
			for _, b := range r.Blockers {
				fmt.Fprintf(w, "\t  ↳ %s\t\t\t%s\n", b.Code, b.Message)
			}
			for _, f := range r.ActivityFindings {
				fmt.Fprintf(w, "\t  ↳ %s\t\t\t%s\n", f.Code, f.Message)
			}
		}
	}
	w.Flush()

	fmt.Fprint(cmd.OutOrStdout(), "\n"+gateSummaryLine(passed, blocked, staleParkings, unusableDeclarations, byActivity)+"\n")
	if blocked > 0 && !gateAllVerbose {
		fmt.Fprintln(cmd.OutOrStdout(), "re-run with --verbose to list each blocker, or `parlay internal gate @<feature> --stage <boundary>` for one feature")
	}
}

// gateActivityVerdict renders the verdict cell for an ungated feature.
//
// "— (no boundary yet)" said only that gate had nothing to judge, which
// is true of a deliberate placeholder and an abandoned one alike. The
// verdict now says which.
func gateActivityVerdict(r gateSweepRow) string {
	switch r.Activity {
	case string(parser.ActivityParked):
		v := "parked"
		if r.ActivityStale {
			v = "parked (stale — has artifacts)"
		}
		if r.ActivityDetail != "" {
			v += " — " + firstLine(r.ActivityDetail)
		}
		return v
	case ActivityUnavailable:
		// The published code, on the default output rather than only
		// under --verbose. A CI log grepped for a diagnostic code should
		// find it whatever kind of row it landed on; making that depend
		// on a flag means the greps that matter are the ones nobody ran
		// with it.
		v := "unavailable"
		for _, f := range r.ActivityFindings {
			v += "  [" + f.Code + ": " + firstLine(f.Message) + "]"
		}
		if len(r.ActivityFindings) == 0 {
			v += " — " + firstLine(r.ActivityDetail)
		}
		return v
	case string(parser.ActivityActive):
		// Declared active but with no boundary yet: in progress, and
		// somebody said so.
		return "active (no boundary yet)"
	default:
		return "unclassified — no disposition recorded"
	}
}

// gateSummaryLine reports the ungated features by disposition rather than
// as one undifferentiated count.
//
// The order is deliberate: unclassified last, because it is the only
// bucket that is a call to action. Parked features are accounted for and
// need nobody; unclassified ones are the pile a person has to work
// through.
func gateSummaryLine(passed, blocked, staleParkings, unusableDeclarations int, byActivity map[string]int) string {
	parts := []string{
		fmt.Sprintf("%d passed", passed),
		fmt.Sprintf("%d blocked", blocked),
	}
	for _, k := range []struct {
		key   string
		label string
	}{
		{string(parser.ActivityActive), "active"},
		{string(parser.ActivityParked), "parked"},
		{ActivityUnavailable, "with an unusable declaration"},
		{string(parser.ActivityUnclassified), "unclassified"},
	} {
		if n := byActivity[k.key]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k.label))
		}
	}
	if unusableDeclarations > 0 {
		parts = append(parts, fmt.Sprintf("%d with an unusable declaration", unusableDeclarations))
	}
	if staleParkings > 0 {
		// Last, and phrased as work rather than as a count, because it is
		// the one line here that names something actively wrong: a
		// parking that no longer describes its feature.
		parts = append(parts, fmt.Sprintf("%d with a stale parking", staleParkings))
	}
	return strings.Join(parts, ", ")
}

// The two published codes this sweep can raise about a declaration. Both
// are errors in activity.schema.md, not warnings.
const codeParkedFeatureAdvanced = "activity-parked-feature-advanced"

// activityFindings turns the activity reading into published diagnostics.
//
// Prose decoration was the earlier design and it was not enough: a built
// feature with a broken activity.yaml printed a plain `pass`, contributed
// to no count, and left the fault invisible to CI — a committed
// declaration that parlay itself refuses to mutate, passing a gate. And
// `activity-parked-feature-advanced` was promised in the proposal and
// emitted nowhere, so an invalid lifecycle claim could not fail anything.
//
// Both are now real findings, carried on the row and counted, while the
// verdict column still belongs to the boundary.
func activityFindings(reading activityReading, stale bool) []gateBlocker {
	var out []gateBlocker
	// EVERY fault, each under its own published code. An earlier cut
	// collapsed them all into "not parseable", which mislabelled a file
	// that had parsed perfectly and failed a shape rule — and threw away
	// the typed routing the validator had just gained.
	for _, d := range reading.Diagnostics() {
		out = append(out, gateBlocker{Code: d.Code, Message: d.Message, Fix: d.Fix})
	}
	if stale {
		out = append(out, gateBlocker{
			Code:    codeParkedFeatureAdvanced,
			Message: "parked, but the feature has since acquired artifacts — the parking no longer describes it",
			// The remedy belongs in Fix, not smuggled into the message.
			// A display that shows only Message would otherwise be the
			// only place the reader could learn what to do.
			//
			// And it must be EXECUTABLE. An earlier wording offered
			// "unpark it, or park it again with a current reason" as
			// alternatives, and neither half of that was reachable as
			// written: park refuses a repeated park while the feature is
			// still parked, so unparking is not an alternative but a
			// prerequisite — and once build outputs exist park refuses
			// even after unparking, because parking is a pre-build act.
			// A remedy the tool would reject is worse than none, because
			// the reader spends the attempt before learning that.
			Fix: "unpark it first (`parlay unpark @<feature> --by <who>`); if it is still pre-build you may then park it again with a current reason, but a feature with a buildfile or testcases must be retired through an amendment rather than parked",
		})
	}
	return out
}
