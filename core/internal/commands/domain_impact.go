// parlay-feature: parlay-tool/domain-document-api
// parlay-component: domain-impact

package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/domainmodel"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

// ContributionFile is the per-feature domain-model file name. It is the same
// filename as the root model and the same artifact shape — a contribution is
// a domain model holding only what one feature proposes.
const ContributionFile = config.DomainModelFile

var domainImpactCmd = &cobra.Command{
	Use:   "domain-impact <@feature>",
	Short: "Report what a feature's domain-model contribution proposes and who it affects (JSON output)",
	Args:  cobra.ExactArgs(1),
	RunE:  runDomainImpact,
}

var domainImpactApply bool

func init() {
	domainImpactCmd.Flags().BoolVar(&domainImpactApply, "apply", false,
		"Merge the contribution's additions into the root domain model. Refuses and writes nothing when the contribution conflicts.")
}

// domainImpactOutput is the JSON envelope. The impact itself is embedded
// rather than nested so a consumer reads `additions` and `affects` at the top
// level, where the loop's decision prompt needs them.
type domainImpactOutput struct {
	agent.ContributionImpact
	// Contributed is false when the feature has no domain-model.yaml of its
	// own. A project that authors only the root model is the normal case, not
	// an error, and the loop needs to tell "nothing proposed" from "nothing
	// checked".
	Contributed bool `json:"contributed"`
	// Applied reports whether --apply wrote the root model.
	Applied bool `json:"applied"`
}

func runDomainImpact(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	contributionPath := filepath.Join(cfg.FeaturePath(slug), ContributionFile)
	contribution, err := domainmodel.LoadFile(contributionPath)
	if errors.Is(err, domainmodel.ErrNoContribution) {
		out := domainImpactOutput{
			ContributionImpact: agent.ContributionImpact{
				Feature: slug, Path: contributionPath, Applicable: true,
			},
			Contributed: false,
		}
		return emitJSON(cmd, out)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", contributionPath, err)
	}

	root, _, err := domainmodel.Load(cmd.Context(), cfg.Root.Path)
	if err != nil {
		return fmt.Errorf("read the project domain model: %w", err)
	}

	facts, err := collectProjectFacts(cfg)
	if err != nil {
		return err
	}

	out := domainImpactOutput{
		ContributionImpact: agent.Impact(slug, contributionPath, root, contribution, facts),
		Contributed:        true,
	}

	if domainImpactApply {
		if _, applyErr := domainmodel.ApplyContribution(cmd.Context(), cfg.Root.Path, contribution); applyErr != nil {
			// The report is still emitted — a refusal the caller cannot see
			// the reason for is a refusal they will work around. The conflict
			// detail is in out.Conflicts.
			_ = emitJSON(cmd, out)
			return fmt.Errorf("apply contribution: %w", applyErr)
		}
		out.Applied = true
	}

	if err := emitJSON(cmd, out); err != nil {
		return err
	}
	if !out.Applicable {
		return NewExitCodeError(1)
	}
	return nil
}

func emitJSON(cmd *cobra.Command, v any) error {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	return nil
}

// collectProjectFacts gathers the two things the impact walk needs: which
// features reference which entities, and which fixtures hold records of them.
//
// Both halves are filters over data parlay already collects — capabilities
// files are already parsed for the entity cross-reference, and
// collectFixtureRecords already yields entity, id and fields per feature and
// fixture. Neither is a new traversal.
func collectProjectFacts(cfg *config.Context) (agent.ProjectFacts, error) {
	facts := agent.ProjectFacts{EntityUsers: map[string][]string{}}

	features, err := cfg.AllFeatures()
	if err != nil {
		return facts, fmt.Errorf("enumerate features: %w", err)
	}

	for _, slug := range features {
		capPath := filepath.Join(cfg.FeaturePath(slug), "capabilities.yaml")
		content, readErr := os.ReadFile(capPath)
		if readErr != nil {
			continue
		}
		for _, entity := range agent.CapabilityEntities(capPath, content) {
			facts.EntityUsers[entity] = append(facts.EntityUsers[entity], slug)
		}
	}
	for entity := range facts.EntityUsers {
		sort.Strings(facts.EntityUsers[entity])
	}

	records, _, _ := collectFixtureRecords(cfg, features)
	seen := map[agent.FixtureEntity]bool{}
	for _, r := range records {
		key := agent.FixtureEntity{Feature: r.Feature, Fixture: r.Fixture, Entity: r.Entity}
		if seen[key] {
			continue
		}
		seen[key] = true
		facts.Fixtures = append(facts.Fixtures, key)
	}
	sort.Slice(facts.Fixtures, func(i, j int) bool {
		a, b := facts.Fixtures[i], facts.Fixtures[j]
		if a.Feature != b.Feature {
			return a.Feature < b.Feature
		}
		if a.Fixture != b.Fixture {
			return a.Fixture < b.Fixture
		}
		return a.Entity < b.Entity
	})

	return facts, nil
}

// loadContributions reads every feature's contribution under the active root.
// Used to answer "is this entity proposed somewhere" — the question that turns
// capabilities-entity-undeclared into capabilities-entity-pending.
//
// A contribution that cannot be decoded is skipped rather than fatal: the
// domain-model validator reports a broken model against the file itself, and
// failing an unrelated feature's capabilities check because a third feature's
// contribution is malformed would be the wrong place to learn about it.
func loadContributions(cfg *config.Context) map[string]domainmodel.Model {
	features, err := cfg.AllFeatures()
	if err != nil {
		return nil
	}
	out := map[string]domainmodel.Model{}
	for _, slug := range features {
		m, loadErr := domainmodel.LoadFile(filepath.Join(cfg.FeaturePath(slug), ContributionFile))
		if loadErr != nil {
			continue
		}
		out[slug] = m
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
