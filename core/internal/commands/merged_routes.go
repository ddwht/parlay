// parlay-feature: parlay-tool/multi-adapter
// parlay-component: merged-route-table
//
// The merged route dispatch table, computed in Go instead of by reading
// every buildfile into an agent's context.
//
// generate-code step 4 used to say "Load ALL buildfiles — read
// .parlay/build/*/buildfile.yaml for every feature that has been built",
// and step 5 then merged their routes: sections by hand. On a 22-feature
// project that is ~540 KB of context to produce a table of paths — and it
// is the reason a one-feature change cost the same as a whole-project one.
//
// The two things that whole-file read actually bought are both mechanical:
// the merged route table (here) and the plan:-presence gate (a field on
// project diff). Everything else step 8's diff already told the agent to
// ignore. Same move as scaffold-seed and check-composition: cross-feature
// coherence is computed deterministically and emitted as names.
//
// Route rows resolve through agent.ResolveBuildfileRoutes — the one
// v2-aware reader — so a multi-target buildfile whose routes relocated under
// targets.presentation cannot read as "no routes" here while the deep
// validator sees them (the BP1 divergence).

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var mergedRoutesCmd = &cobra.Command{
	Use:   "merged-routes",
	Short: "Emit the merged route dispatch table across every built feature, with provenance (JSON)",
	Args:  cobra.NoArgs,
	RunE:  runMergedRoutes,
}

// mergedRoute is one row of the dispatch table.
type mergedRoute struct {
	Path string `json:"path"`
	// Feature is the provenance the agent needs to attribute a route back
	// to the buildfile that declared it — and, when a route needs changing,
	// to know which single buildfile to open.
	Feature string `json:"feature"`

	// The blueprint join (step 6's work, done here because it is a
	// deterministic lookup on path). Empty when the blueprint declares
	// nothing for this route: the module's existing rule applies — default
	// shell, no guard.
	Shell string `json:"shell,omitempty"`
	Guard string `json:"guard,omitempty"`
}

type mergedRoutesOutput struct {
	Routes []mergedRoute `json:"routes"`

	// Conflicts names paths declared by more than one feature. Reported
	// rather than resolved: two features claiming one path is a
	// composition question with an owner (check-composition), and silently
	// picking a winner here would hide it at exactly the moment codegen
	// depends on the answer.
	Conflicts []string `json:"conflicts,omitempty"`

	// Strategy and DefaultRoute come from the blueprint's navigation block
	// and drive the router component and root redirect. Empty when no
	// blueprint exists — the documented backwards-compatible path.
	Strategy     string `json:"strategy,omitempty"`
	DefaultRoute string `json:"default_route,omitempty"`

	// FeaturesRead counts the buildfiles this command opened, so a skill can
	// say in its report what it did NOT have to load.
	FeaturesRead int `json:"features_read"`
}

func runMergedRoutes(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	out, err := computeMergedRoutes(cfg)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func computeMergedRoutes(cfg *config.Context) (*mergedRoutesOutput, error) {
	features, err := cfg.AllFeatures()
	if err != nil {
		return nil, fmt.Errorf("discover features: %w", err)
	}
	sort.Strings(features)

	out := &mergedRoutesOutput{Routes: []mergedRoute{}}
	seen := map[string]string{} // path → first feature that declared it
	conflicts := map[string]bool{}

	for _, slug := range features {
		data, err := os.ReadFile(filepath.Join(cfg.BuildPath(slug), "buildfile.yaml"))
		if err != nil {
			continue // unbuilt feature contributes no routes
		}
		out.FeaturesRead++
		routes, err := agent.ResolveBuildfileRoutes(data)
		if err != nil {
			// A malformed buildfile is check-buildfile's finding to report,
			// not this command's to fail on: emitting the table for every
			// feature that parses is more useful than refusing all of them.
			continue
		}
		for _, r := range routes {
			if r.Path == "" {
				continue
			}
			if first, dup := seen[r.Path]; dup {
				if first != slug {
					conflicts[r.Path] = true
				}
				continue
			}
			seen[r.Path] = slug
			out.Routes = append(out.Routes, mergedRoute{Path: r.Path, Feature: slug})
		}
	}

	sort.Slice(out.Routes, func(i, j int) bool { return out.Routes[i].Path < out.Routes[j].Path })
	for path := range conflicts {
		out.Conflicts = append(out.Conflicts, path)
	}
	sort.Strings(out.Conflicts)

	applyBlueprintNavigation(cfg, out)
	return out, nil
}

// applyBlueprintNavigation joins the blueprint's navigation block onto the
// merged table. A missing or unparseable blueprint leaves the table intact —
// the module's documented "proceed without it" path.
func applyBlueprintNavigation(cfg *config.Context, out *mergedRoutesOutput) {
	data, err := os.ReadFile(cfg.BlueprintPath())
	if err != nil {
		return
	}
	var bp struct {
		Navigation *struct {
			Strategy     string `yaml:"strategy"`
			DefaultRoute string `yaml:"default-route"`
			Routes       []struct {
				Path  string `yaml:"path"`
				Shell string `yaml:"shell"`
				Guard string `yaml:"guard"`
			} `yaml:"routes"`
		} `yaml:"navigation"`
	}
	if yaml.Unmarshal(data, &bp) != nil || bp.Navigation == nil {
		return
	}
	out.Strategy = bp.Navigation.Strategy
	out.DefaultRoute = bp.Navigation.DefaultRoute

	byPath := map[string]struct{ shell, guard string }{}
	for _, r := range bp.Navigation.Routes {
		byPath[r.Path] = struct{ shell, guard string }{r.Shell, r.Guard}
	}
	for i := range out.Routes {
		if join, ok := byPath[out.Routes[i].Path]; ok {
			out.Routes[i].Shell = join.shell
			out.Routes[i].Guard = join.guard
		}
	}
}
