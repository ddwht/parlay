// parlay-feature: parlay-tool/intent-supersession
// parlay-component: governance-amendment-apply
//
// Applying a ledger record that changes no artifact.
//
// Every other amendment is applied by splicing its delta into a contract
// artifact and re-baselining, which is what /parlay-refine does. A governance
// amendment has no delta: it supersedes a founding intent, or retires a
// feature, and its effect is entirely on what the project promises. There was
// nothing to splice and therefore nothing to apply it — so an authored
// supersession stayed pending forever, blocking every boundary it touched,
// with no command that could resolve it.
//
// The gap was specified rather than missed: intent-supersession's own
// infrastructure says "a feature carrying no contract artifact still requires a
// real completion step to apply a record, rather than an automatic advance of
// the applied marker." This is that step. It surfaced the moment the mechanism
// was used on a real feature instead of a fixture.
//
// "Real" is the operative word. This does not simply advance a number: it
// refuses unless the ledger is otherwise sound and the record genuinely has no
// artifact delta to apply, because an amendment that DOES name contract entries
// has a splice somebody still owes.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

var applyGovernanceConfirmed bool

var applyGovernanceCmd = &cobra.Command{
	Use:   "apply-governance @<feature>",
	Short: "Apply a governance amendment — one that changes what a feature promises rather than any artifact",
	Long: `Apply the feature's unapplied governance amendments.

A governance amendment supersedes a founding intent or retires a feature. It
carries no affects:, so there is no splice to perform and /parlay-refine has
nothing to do with it — but until it is applied the feature still makes the
promise the amendment withdraws, and every advancing boundary blocks.

Refuses when the ledger has other problems, and refuses any amendment that
names contract entries, because those have a splice somebody still owes.`,
	Args: cobra.ExactArgs(1),
	RunE: runApplyGovernance,
}

func init() {
	applyGovernanceCmd.Flags().BoolVar(&applyGovernanceConfirmed, "confirm", false,
		"required: applying a governance amendment changes what this feature promises, and there is no safe default for that")
}

type applyGovernanceOutput struct {
	Feature string `json:"feature"`
	// Applied names the amendments this run put into force.
	Applied []string `json:"applied"`
	// LastApplied is where the baseline now sits.
	LastApplied int      `json:"last_applied_amendment"`
	Retired     bool     `json:"feature_retired"`
	Issues      []string `json:"issues,omitempty"`
}

func runApplyGovernance(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featDir := cfg.FeaturePath(slug)

	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		return fmt.Errorf("read ledger: %w", err)
	}
	lastApplied := lastAppliedAmendment(cfg, slug)

	var pending []parser.Amendment
	for _, a := range amendments {
		if a.Seq > lastApplied {
			pending = append(pending, a)
		}
	}
	if len(pending) == 0 {
		return fmt.Errorf("%s has no unapplied amendments", slug)
	}

	// Every pending record must be governance. One that names contract entries
	// has a delta somebody still owes, and advancing past it would mark a
	// splice applied that never happened — which is the drift the ledger's
	// whole applied-tail model exists to make visible.
	for _, a := range pending {
		if len(a.Affects) > 0 {
			return fmt.Errorf("%03d-%s names %d contract entr(y/ies) in affects:, so it has a splice to apply — run /parlay-refine for it rather than marking it applied here",
				a.Seq, a.FileSlug, len(a.Affects))
		}
	}

	// The ledger must be otherwise sound. Applying over a fork, an unknown
	// intent ref or an unaccounted contract entry would put a decision into
	// force that check-amendments is refusing.
	ca := computeCheckAmendments(cfg, slug)
	var issues []string
	for _, iss := range ca.Issues {
		if iss.Severity == "error" {
			issues = append(issues, fmt.Sprintf("[%s] %s", iss.Code, iss.Message))
		}
	}
	if len(issues) > 0 {
		out := applyGovernanceOutput{Feature: slug, LastApplied: lastApplied, Issues: issues}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		return fmt.Errorf("%s: the ledger has %d unresolved error(s); applying would put a decision into force that check-amendments refuses", slug, len(issues))
	}

	if !applyGovernanceConfirmed {
		var names []string
		retires := false
		for _, a := range pending {
			names = append(names, fmt.Sprintf("%03d-%s", a.Seq, a.FileSlug))
			if a.RetiresFeature {
				retires = true
			}
		}
		what := "change what this feature promises"
		if retires {
			what = "RETIRE this feature"
		}
		return fmt.Errorf("applying %s would %s. Re-run with --confirm; there is no safe default for a decision that withdraws scope",
			joinNames(names), what)
	}

	highest := lastApplied
	var applied []string
	retired := false
	for _, a := range pending {
		if a.Seq > highest {
			highest = a.Seq
		}
		applied = append(applied, fmt.Sprintf("%03d-%s", a.Seq, a.FileSlug))
		if a.RetiresFeature {
			retired = true
		}
	}

	if err := advanceAppliedMarker(cfg, slug, highest, amendments); err != nil {
		return err
	}

	out := applyGovernanceOutput{Feature: slug, Applied: applied, LastApplied: highest, Retired: retired}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// advanceAppliedMarker moves the baseline's applied tail and records the
// ledger file hashes alongside it.
//
// The hashes matter as much as the number: the integrity check compares them,
// so advancing without them would mark the ledger applied while leaving the
// tool unable to notice a later edit to a file it had already honoured.
func advanceAppliedMarker(cfg *config.Context, slug string, seq int, amendments []parser.Amendment) error {
	path := baselinePath(cfg, slug)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read baseline for %s: %w — a governance amendment applies to a feature that has one", slug, err)
	}
	var baseline Baseline
	if err := yaml.Unmarshal(data, &baseline); err != nil {
		return fmt.Errorf("parse baseline for %s: %w", slug, err)
	}

	baseline.LastAppliedAmendment = seq
	if baseline.Sources == nil {
		baseline.Sources = &HashedSources{}
	}
	if baseline.Sources.Amendments == nil {
		baseline.Sources.Amendments = map[string]string{}
	}
	for _, a := range amendments {
		if a.Seq > seq {
			continue
		}
		if hash, ok := hashWholeFile(a.Path); ok {
			baseline.Sources.Amendments[filepath.Base(a.Path)] = hash
		}
	}
	baseline.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	out, err := yaml.Marshal(&baseline)
	if err != nil {
		return err
	}
	return atomicfile.WriteAtomic(path, out)
}

func joinNames(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	s := ""
	for i, n := range names {
		if i > 0 {
			s += ", "
		}
		s += n
	}
	return s
}
