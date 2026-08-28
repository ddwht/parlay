// parlay-feature: parlay-tool/page-assembly-derivation
// parlay-component: producer-readiness-probe
//
// Lets the PRODUCER see what the code boundary will see, before it hands off.
//
// Uncovered criteria were being discovered at generate-code, one phase after
// the only phase that can fix them. build-feature derives the cases; if it
// emits a testcases.yaml that does not discharge the standard, the run gets as
// far as codegen and stops there, having spent a whole phase to learn
// something its producer could have been told immediately.
//
// This is deliberately CheckTestcasesReadiness itself, not a reimplementation
// of it. A second "producer-side" readiness computation that drifted from the
// boundary's would be worse than none: build-feature would hand off on a green
// probe and be refused by a check asking a different question.
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/parser"
)

var checkTestcasesReadyJSON bool

func init() {
	checkTestcasesReadyCmd.Flags().BoolVar(&checkTestcasesReadyJSON, "json", true,
		"emit machine-readable JSON (default; --json=false for prose)")
}

var checkTestcasesReadyCmd = &cobra.Command{
	Use:   "check-testcases-ready <@feature>",
	Short: "Report whether this feature's testcases discharge its standard, as the code boundary will judge it",
	Long: `Run the code boundary's testcase-readiness computation now, in the build phase.

This is the same check generate-code's boundary runs, called early so the phase
that DERIVES the cases is the phase that repairs them. Every blocker it prints
names a criterion with no case discharging it, a case citing something that
does not exist, or a derived assembly suite that disagrees with the composed
page — all of them things build-feature can fix by deriving again, and none of
them things a downstream waiver should paper over.

Exits non-zero when blockers remain, so the build phase can refuse to hand off
work the code phase is guaranteed to reject.`,
	Args: cobra.ExactArgs(1),
	RunE: runCheckTestcasesReady,
}

type testcasesReadyOutput struct {
	Feature  string   `json:"feature"`
	Ready    bool     `json:"ready"`
	Blockers []string `json:"blockers"`
	Warnings []string `json:"warnings"`
}

func runCheckTestcasesReady(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	r := CheckTestcasesReadiness(cfg, slug)
	out := testcasesReadyOutput{
		Feature:  slug,
		Ready:    len(r.Blockers) == 0,
		Blockers: r.Blockers,
		Warnings: r.Warnings,
	}
	if out.Blockers == nil {
		out.Blockers = []string{}
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}

	if checkTestcasesReadyJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
	} else {
		if out.Ready {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: testcases discharge the standard.\n", slug)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d blocker(s) the code boundary will refuse on:\n", slug, len(out.Blockers))
			for _, b := range out.Blockers {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", b)
			}
		}
		for _, w := range out.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "  (warning) %s\n", w)
		}
	}

	if !out.Ready {
		// Non-zero so the build phase cannot hand off work the code phase is
		// guaranteed to reject. SilenceUsage: this is a finding, not misuse.
		cmd.SilenceUsage = true
		return fmt.Errorf("%s: testcases do not discharge the standard (%d blockers)", slug, len(out.Blockers))
	}
	return nil
}
