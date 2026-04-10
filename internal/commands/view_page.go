package commands

// Generated from buildfile component: page-assembly-view
// Type: report | Widget: sectioned-output | Layout: report-output

import (
	"fmt"
	"sort"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
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

	// Operation: scan-files "spec/intents/*/surface.md", parse using surface-schema
	allFragments, err := parser.ScanAllSurfaces(config.SpecDir)
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
		fmt.Printf("No fragments target page %q.\n", pageName)
		if len(unplaced) > 0 {
			fmt.Printf("%d fragments have no page target.\n", len(unplaced))
		}
		return nil
	}

	// Computed: regions = group targeted by region, sort by order
	// Computed: conflicts = fragments with same region + same order
	regions, conflicts := assembleRegions(targeted)

	// Element: page-header (text-output → fmt.Println)
	fmt.Printf("Assembled view: %s\n\n", pageName)

	// Element: region-blocks (grouped-output → headed-section)
	for _, region := range regions {
		fmt.Printf("**%s**:\n", region.Name)
		for i, frag := range region.Fragments {
			fmt.Printf("  %d. @%s/%s\n", i+1, frag.Feature, parser.Slugify(frag.Name))
		}
		fmt.Println()
	}

	// Element: conflict-warnings (data-list → bulleted-list, visible-when: conflicts.length > 0)
	if len(conflicts) > 0 {
		fmt.Printf("Conflicts (%d):\n", len(conflicts))
		for _, c := range conflicts {
			fmt.Printf("  %s and %s both target %s with order %d\n",
				fmt.Sprintf("@%s/%s", c.Fragments[0].Feature, parser.Slugify(c.Fragments[0].Name)),
				fmt.Sprintf("@%s/%s", c.Fragments[1].Feature, parser.Slugify(c.Fragments[1].Name)),
				c.Region, c.Order)
		}
		fmt.Println()
	}

	// Element: unplaced-header + unplaced-list (visible-when: unplaced.length > 0)
	if len(unplaced) > 0 {
		fmt.Printf("Unplaced fragments (%d):\n", len(unplaced))
		for _, f := range unplaced {
			fmt.Printf("  %s from @%s — %s\n", f.Name, f.Feature, truncate(f.Shows, 50))
		}
		fmt.Println()
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
