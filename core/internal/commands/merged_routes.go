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
	"regexp"
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

// ---------------------------------------------------------------------
// The cross-cutting index.
// ---------------------------------------------------------------------

var crossCuttingIndexCmd = &cobra.Command{
	Use:   "cross-cutting-index",
	Short: "Index every feature's cross-cutting entries by id and target, without their transform prose (JSON)",
	Args:  cobra.NoArgs,
	RunE:  runCrossCuttingIndex,
}

// crossCuttingIndexTargets filters the index to entries touching the named
// paths. The unfiltered index is ~70 KB on a mature project — better than the
// 238 KB of prose it stands in for, but still not something to load every
// run. The actual question a caller has is narrow ("I regenerated these three
// files; whose merges did I overwrite?"), and answering exactly that question
// usually returns nothing at all.
var crossCuttingIndexTargets []string

func init() {
	crossCuttingIndexCmd.Flags().StringArrayVar(&crossCuttingIndexTargets, "target", nil,
		"Only report entries whose targets include this path (repeatable). Without it, the whole index is emitted.")
}

// crossCuttingIndexEntry is one entry's identity and footprint.
type crossCuttingIndexEntry struct {
	ID      string `json:"id"`
	Feature string `json:"feature"`
	Source  string `json:"source,omitempty"`
	// Targets is target-files + target-creates, the paths this entry writes
	// or merges into. TargetPattern is reported separately because it
	// resolves against the source tree at generation time, not here.
	Targets       []string `json:"targets,omitempty"`
	TargetPattern string   `json:"target_pattern,omitempty"`
	// Buildfile is where to read this entry's transform prose from, when the
	// caller decides it needs it. The whole point of the index is that this
	// is usually not needed.
	Buildfile string `json:"buildfile"`
}

type crossCuttingIndexOutput struct {
	Entries []crossCuttingIndexEntry `json:"entries"`
	// ByTarget maps a target path to the entry ids that write it — the
	// lookup a caller actually performs: "I am about to regenerate this
	// file; whose merges land in it?"
	ByTarget     map[string][]string `json:"by_target,omitempty"`
	FeaturesRead int                 `json:"features_read"`
}

func runCrossCuttingIndex(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	features, err := cfg.AllFeatures()
	if err != nil {
		return fmt.Errorf("discover features: %w", err)
	}
	sort.Strings(features)

	wanted := map[string]bool{}
	for _, t := range crossCuttingIndexTargets {
		wanted[filepath.ToSlash(filepath.Clean(t))] = true
	}

	out := crossCuttingIndexOutput{Entries: []crossCuttingIndexEntry{}, ByTarget: map[string][]string{}}
	for _, slug := range features {
		path := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out.FeaturesRead++
		entries, err := agent.ResolveBuildfileCrossCutting(data)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(cfg.Root.Path, path)
		if relErr != nil {
			rel = path
		}
		for _, e := range entries {
			targets := append(append([]string{}, e.TargetFiles...), e.TargetCreates...)
			sort.Strings(targets)
			if len(wanted) > 0 &&
				!targetsIntersect(targets, wanted) &&
				!patternMatchesAny(cfg, e.TargetPattern, wanted) {
				continue
			}
			out.Entries = append(out.Entries, crossCuttingIndexEntry{
				ID:            e.ID,
				Feature:       slug,
				Source:        e.Source,
				Targets:       targets,
				TargetPattern: e.TargetPattern,
				Buildfile:     filepath.ToSlash(rel),
			})
			for _, t := range targets {
				if len(wanted) > 0 && !wanted[filepath.ToSlash(filepath.Clean(t))] {
					continue
				}
				out.ByTarget[t] = append(out.ByTarget[t], e.ID)
			}
		}
	}
	for t := range out.ByTarget {
		sort.Strings(out.ByTarget[t])
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

// targetsIntersect reports whether any of an entry's targets is one of the
// paths the caller asked about. Compared on cleaned slash-form so
// "./src/app.go" and "src/app.go" are the same file.
func targetsIntersect(targets []string, wanted map[string]bool) bool {
	for _, t := range targets {
		if wanted[filepath.ToSlash(filepath.Clean(t))] {
			return true
		}
	}
	return false
}

// patternMatchesAny resolves a cross-cutting entry's target-pattern against
// the specific files the caller asked about.
//
// The pattern selects files by CONTENT (`rootCmd.AddCommand\(`), not by path,
// so it cannot be decided from the index alone — and leaving every
// pattern-carrying entry in the filtered result made the filter useless: on
// the dogfood root it returned 30-odd entries for a question with one real
// answer. Resolving it here costs reading the handful of files the caller
// named, and returns an exact answer instead of a haystack.
//
// Unreadable file or uncompilable pattern → true, the safe direction: an
// entry the index cannot rule out is one the caller should look at, and the
// cost of an extra look is a read, while the cost of a wrong exclusion is a
// silently dropped merge.
func patternMatchesAny(cfg *config.Context, pattern string, wanted map[string]bool) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return true
	}
	for path := range wanted {
		data, err := os.ReadFile(filepath.Join(cfg.Root.Path, filepath.FromSlash(path)))
		if err != nil {
			// The file may be about to be created by this run rather than
			// already on disk; the entry stays in scope.
			return true
		}
		if re.Match(data) {
			return true
		}
	}
	return false
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
