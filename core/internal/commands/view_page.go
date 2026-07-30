package commands

// Generated from buildfile component: page-assembly-view
// Type: report | Widget: sectioned-output | Layout: report-output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var viewPageCmdImpl = &cobra.Command{
	Use:   "view-page <page-name>",
	Short: "Assemble and display a page view",
	Args:  cobra.ExactArgs(1),
	RunE:  runViewPage,
}

type regionView struct {
	Name      string
	Fragments []parser.Fragment
}

type conflict struct {
	Region    string
	Order     int
	Fragments []parser.Fragment
}

func runViewPage(cmd *cobra.Command, args []string) error {
	// Data input: page-name from command-argument
	pageName := args[0]

	// Layout-validation precheck gate (parlay-cross-cutting-id:
	// layout-precheck-contract): a page with a layout artifact on disk
	// must pass precheck before its view is assembled. Pages without a
	// layout artifact are unaffected — the gate is a no-op for the
	// existing region-based flow below.
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if verdict := layoutPrecheckGate(cfg, pageName); verdict.Code != "ok" {
		return refuseOnPrecheckVerdict(cmd, verdict)
	}

	// Operation: scan-files "spec/intents/*/surface.md", parse using surface-schema
	//
	// Joined to the resolved root. This passed the bare relative constant
	// "spec", so view-page only worked when cwd happened to BE the active
	// root and silently ignored --root and PARLAY_ROOT — against any other
	// root it scanned a "spec" directory relative to the shell's cwd, found
	// nothing, and reported "No fragments target page X". lock-page in the
	// same package already joins correctly.
	allFragments, err := parser.ScanAllSurfaces(filepath.Join(cfg.Root.Path, config.SpecDir))
	if err != nil {
		return fmt.Errorf("failed to scan surfaces: %w", err)
	}

	// Computed: targeted = fragments where page == page-name
	// Computed: unplaced = fragments where page is empty
	var targeted []parser.Fragment
	var unplaced []parser.Fragment

	for _, f := range allFragments {
		if f.Page == pageName {
			targeted = append(targeted, f)
		} else if f.Page == "" {
			unplaced = append(unplaced, f)
		}
	}

	if len(targeted) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No fragments target page %q.\n", pageName)
		if len(unplaced) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "%d fragments have no page target.\n", len(unplaced))
		}
		return nil
	}

	// Computed: regions = group targeted by region, sort by order
	// Computed: conflicts = fragments with same region + same order
	regions, conflicts := assembleRegions(targeted)

	// Apply the page manifest, when one exists.
	//
	// lock-page has always written spec/pages/<page>.page.md and view-page
	// has never read it — a manifest could be reordered, have fragments
	// removed, have a region invented and be marked `locked`, and the output
	// did not change by a byte. All three behaviours page.schema.md documents
	// (manifest order overrides surface order; unlisted fragments follow the
	// listed ones; drift is reported) were unimplemented, so the manifest was
	// a write-only artifact and lock-page locked nothing.
	manifestRegions, drift := applyPageManifest(cfg, pageName, regions)
	if manifestRegions != nil {
		regions = manifestRegions
	}

	// Element: page-header (text-output → fmt.Println)
	fmt.Fprintf(cmd.OutOrStdout(), "Assembled view: %s\n\n", pageName)

	// Element: region-blocks (grouped-output → headed-section)
	for _, region := range regions {
		fmt.Fprintf(cmd.OutOrStdout(), "**%s**:\n", region.Name)
		for i, frag := range region.Fragments {
			fmt.Fprintf(cmd.OutOrStdout(), "  %d. @%s/%s\n", i+1, frag.Feature, parser.Slugify(frag.Name))
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Element: manifest-drift (visible-when: drift.length > 0)
	//
	// page.schema.md: "Tool warns on drift (new/removed fragments)". Without
	// this a manifest silently rots — a fragment added to a surface after the
	// lock never appears in the locked order, and one removed leaves a
	// reference to something that no longer exists, with nothing to say so.
	if len(drift) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Manifest drift (%d):\n", len(drift))
		for _, d := range drift {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", d)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Element: conflict-warnings (data-list → bulleted-list, visible-when: conflicts.length > 0)
	if len(conflicts) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Conflicts (%d):\n", len(conflicts))
		for _, c := range conflicts {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s and %s both target %s with order %d\n",
				fmt.Sprintf("@%s/%s", c.Fragments[0].Feature, parser.Slugify(c.Fragments[0].Name)),
				fmt.Sprintf("@%s/%s", c.Fragments[1].Feature, parser.Slugify(c.Fragments[1].Name)),
				c.Region, c.Order)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Element: unplaced-header + unplaced-list (visible-when: unplaced.length > 0)
	if len(unplaced) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Unplaced fragments (%d):\n", len(unplaced))
		for _, f := range unplaced {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s from @%s — %s\n", f.Name, f.Feature, truncate(f.Shows, 50))
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	return nil
}

func assembleRegions(fragments []parser.Fragment) ([]regionView, []conflict) {
	regionMap := make(map[string][]parser.Fragment)
	for _, f := range fragments {
		region := f.Region
		if region == "" {
			region = "main"
		}
		regionMap[region] = append(regionMap[region], f)
	}

	// Sort region names in conventional order
	order := map[string]int{
		"header": 1, "toolbar": 2, "main": 3, "sidebar": 4, "footer": 5, "dialog": 6,
	}

	regionNames := make([]string, 0, len(regionMap))
	for name := range regionMap {
		regionNames = append(regionNames, name)
	}
	sort.Slice(regionNames, func(i, j int) bool {
		oi, ok1 := order[regionNames[i]]
		oj, ok2 := order[regionNames[j]]
		if ok1 && ok2 {
			return oi < oj
		}
		if ok1 {
			return true
		}
		if ok2 {
			return false
		}
		return regionNames[i] < regionNames[j]
	})

	var regions []regionView
	var conflicts []conflict

	for _, name := range regionNames {
		frags := regionMap[name]
		sort.Slice(frags, func(i, j int) bool {
			return frags[i].Order < frags[j].Order
		})

		for i := 0; i < len(frags)-1; i++ {
			if frags[i].Order == frags[i+1].Order && frags[i].Order > 0 {
				conflicts = append(conflicts, conflict{
					Region:    name,
					Order:     frags[i].Order,
					Fragments: []parser.Fragment{frags[i], frags[i+1]},
				})
			}
		}

		regions = append(regions, regionView{Name: name, Fragments: frags})
	}

	return regions, conflicts
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// applyPageManifest reorders the assembled regions to match
// spec/pages/<page>.page.md, and reports drift between the two.
//
// Returns (nil, nil) when no manifest exists — the derived view is the
// default and a page without a manifest is the normal case.
//
// The three behaviours are page.schema.md's, and none of them existed before:
//
//   - "Order overrides feature surface Order values" — the manifest's list
//     position wins over the fragment's own order:.
//   - Unlisted fragments targeting this page appear AFTER the manifest-ordered
//     ones, rather than being dropped. A manifest is a layout decision, not an
//     allowlist; silently hiding a fragment a feature declares would make the
//     view disagree with the surfaces that produce it.
//   - Drift is reported both ways: a fragment the manifest lists that no
//     surface produces, and one the surfaces produce that the manifest does
//     not list.
func applyPageManifest(cfg *config.Context, pageName string, derived []regionView) ([]regionView, []string) {
	path := filepath.Join(cfg.Root.Path, config.SpecDir, "pages", pageName+".page.md")
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	page, err := parser.ParsePageFile(path)
	if err != nil || page == nil || len(page.Regions) == 0 {
		// An unreadable or region-less manifest must not silently replace the
		// derived view with nothing. validate --type page is where a
		// malformed manifest gets reported.
		return nil, nil
	}

	// Index every derived fragment by its @feature/fragment reference.
	type placed struct {
		frag   parser.Fragment
		region string
	}
	byRef := map[string]placed{}
	for _, r := range derived {
		for _, f := range r.Fragments {
			byRef[fmt.Sprintf("@%s/%s", f.Feature, parser.Slugify(f.Name))] = placed{f, r.Name}
		}
	}

	var out []regionView
	var drift []string
	listed := map[string]bool{}

	for _, mr := range page.Regions {
		rv := regionView{Name: mr.Name}
		for _, ref := range mr.Components {
			p, ok := byRef[ref]
			if !ok {
				drift = append(drift, fmt.Sprintf("%s is listed in the manifest but no surface produces it", ref))
				continue
			}
			listed[ref] = true
			rv.Fragments = append(rv.Fragments, p.frag)
		}
		out = append(out, rv)
	}

	// Unlisted fragments follow, in their derived region and order.
	byName := map[string]int{}
	for i, r := range out {
		byName[r.Name] = i
	}
	for _, r := range derived {
		for _, f := range r.Fragments {
			ref := fmt.Sprintf("@%s/%s", f.Feature, parser.Slugify(f.Name))
			if listed[ref] {
				continue
			}
			drift = append(drift, fmt.Sprintf("%s targets this page but the manifest does not list it", ref))
			if i, ok := byName[r.Name]; ok {
				out[i].Fragments = append(out[i].Fragments, f)
				continue
			}
			byName[r.Name] = len(out)
			out = append(out, regionView{Name: r.Name, Fragments: []parser.Fragment{f}})
		}
	}

	sort.Strings(drift)
	return out, drift
}
