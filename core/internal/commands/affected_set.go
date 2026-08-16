// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: affected-set-probe
//
// Read-only probe: given a feature, emit the set of features whose builds
// could be affected by a change to it — the feature itself plus every
// feature whose buildfile references it (composition fixtures, operation
// bindings, flow declarations all write `@<feature>/...` refs into the
// consuming buildfile). In a ledger project the declared dirty set from
// check-amendments is included for scoping.
//
// This is the mechanical half of affected-test selection. It deliberately
// does NOT change what refine runs: the full suite remains the default
// gate, and narrowing the interactive run to the affected set is an
// explicit opt-in with an unconditional full-suite backstop in CI — see
// refine.skill.md. A probe that merely answers "who could this touch"
// blocks nothing and blesses nothing.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var affectedSetCmd = &cobra.Command{
	Use:   "affected-set <@feature>",
	Short: "Emit the features whose builds a change to this feature could affect (JSON)",
	Args:  cobra.ExactArgs(1),
	RunE:  runAffectedSet,
}

type affectedSetOutput struct {
	Feature string `json:"feature"`
	// DirtySet is check-amendments' declared set when the feature has a
	// ledger; empty otherwise.
	DirtySet []string `json:"dirty_set,omitempty"`
	// Dependents are other features whose buildfiles reference this one.
	Dependents []string `json:"dependents"`
	// Affected is the closure: the feature itself plus its dependents —
	// the suites to run when narrowing an interactive test run.
	Affected []string `json:"affected"`
}

func runAffectedSet(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	out := affectedSetOutput{Feature: slug, Dependents: []string{}, Affected: []string{}}

	// Declared dirty set, when a ledger exists. Best-effort: an unreadable
	// ledger is check-amendments' finding to report, not this probe's.
	//
	// Scoped to the unapplied tail (Seq > baseline last-applied-amendment), the
	// same as check-amendments' dirty_set (L7). The comment on the field claims
	// these two are the same set; they must actually be, or the divergence this
	// probe exists to avoid reappears one command over. A missing/unreadable
	// baseline reads as 0 — every amendment counts as unapplied.
	lastApplied := 0
	if blData, readErr := os.ReadFile(baselinePath(cfg, slug)); readErr == nil {
		var baseline Baseline
		if yaml.Unmarshal(blData, &baseline) == nil {
			lastApplied = baseline.LastAppliedAmendment
		}
	}
	if amendments, err := parser.LoadFeatureAmendments(cfg.FeaturePath(slug)); err == nil {
		for _, a := range amendments {
			if a.Seq <= lastApplied {
				continue
			}
			for _, raw := range a.Affects {
				out.DirtySet = appendUniqueRef(out.DirtySet, raw)
			}
		}
	}

	// Every feature whose buildfile mentions this feature by ref. The
	// buildfile is the right place to look: cross-feature coupling always
	// lands there as a normalized `@<feature>/...` reference, whatever
	// artifact it started in.
	features, err := cfg.AllFeatures()
	if err != nil {
		return fmt.Errorf("enumerate features: %w", err)
	}
	needle := "@" + slug + "/"
	for _, other := range features {
		if other == slug {
			continue
		}
		bfPath := filepath.Join(cfg.BuildPath(other), "buildfile.yaml")
		content, readErr := os.ReadFile(bfPath)
		if readErr != nil {
			continue // unbuilt feature; nothing can depend through it
		}
		if strings.Contains(string(content), needle) {
			out.Dependents = append(out.Dependents, other)
		}
	}
	sort.Strings(out.Dependents)
	out.Affected = append([]string{slug}, out.Dependents...)

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
