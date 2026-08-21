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
	"text/tabwriter"

	"github.com/ddwht/parlay/core/internal/config"
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
		if len(r.Blockers) > 0 || r.Err != "" {
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
		if !gated {
			rows = append(rows, gateSweepRow{
				Root: rootLabel, Feature: slug, Phase: string(phase), Stage: "—", Skipped: true, Passed: true,
			})
			continue
		}
		out, err := computeGate(cfg, slug, stage)
		if err != nil {
			rows = append(rows, gateSweepRow{
				Root: rootLabel, Feature: slug, Phase: string(phase), Stage: stage, Err: err.Error(),
			})
			continue
		}
		rows = append(rows, gateSweepRow{
			Root: rootLabel, Feature: slug, Phase: string(phase), Stage: stage,
			Passed: out.Passed, Blockers: out.Blockers,
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
	passed, blocked, skipped := 0, 0, 0
	for _, r := range rows {
		verdict := ""
		switch {
		case r.Err != "":
			verdict = "ERROR: " + r.Err
			blocked++
		case r.Skipped:
			verdict = "— (no boundary yet)"
			skipped++
		case r.Passed:
			verdict = "pass"
			passed++
		default:
			verdict = fmt.Sprintf("BLOCKED (%d)", len(r.Blockers))
			blocked++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Root, r.Feature, r.Phase, r.Stage, verdict)
		if gateAllVerbose {
			for _, b := range r.Blockers {
				fmt.Fprintf(w, "\t  ↳ %s\t\t\t%s\n", b.Code, b.Message)
			}
		}
	}
	w.Flush()

	fmt.Fprintf(cmd.OutOrStdout(), "\n%d passed, %d blocked, %d not yet gated\n", passed, blocked, skipped)
	if blocked > 0 && !gateAllVerbose {
		fmt.Fprintln(cmd.OutOrStdout(), "re-run with --verbose to list each blocker, or `parlay internal gate @<feature> --stage <boundary>` for one feature")
	}
}
