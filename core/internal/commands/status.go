package commands

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the active parlay root, its features, and any registered child roots",
	Long: `Print a short summary of the resolved parlay state for the current
invocation: the active root path and kind, the features authored under
spec/intents/, and (for parent roots) any registered child roots.

A parent root with an empty spec/intents/ — i.e. all features live in
children — is a normal supported state. status reports zero parent
features and lists the children, with no warning.`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		return fmt.Errorf("no active parlay root")
	}

	fmt.Printf("root:     %s\n", pctx.Root.Path)
	fmt.Printf("kind:     %s\n", pctx.Root.Kind)
	if pctx.Root.Kind == config.RootKindChild && pctx.Root.ParentPath != "" {
		fmt.Printf("parent:   %s\n", pctx.Root.ParentPath)
	}
	if pctx.Resolution != nil {
		fmt.Printf("source:   %s\n", pctx.Resolution.Source)
	}

	// Features at this root. Treat a missing intents/ tree as zero
	// features — this is the bare-parent topology and must not error.
	features, err := scanFeaturesAt(pctx.IntentsRoot())
	if err != nil {
		return err
	}
	if len(features) == 0 {
		fmt.Printf("features: (none)\n")
	} else {
		fmt.Printf("features: %d\n", len(features))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, f := range features {
			fmt.Fprintf(w, "  - %s\n", f)
		}
		w.Flush()
	}

	if pctx.Index != nil && len(pctx.Index.Children) > 0 {
		fmt.Printf("child roots:\n")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, c := range pctx.Index.Children {
			desc := ""
			if c.Description != "" {
				desc = "\t— " + c.Description
			}
			fmt.Fprintf(w, "  - %s\t(%s)%s\n", c.Name, c.RelativePath, desc)
		}
		w.Flush()
	}
	return nil
}

// scanFeaturesAt enumerates feature identifiers under the given intents
// tree root. A missing tree root returns (nil, nil) — bare-parent
// topology.
func scanFeaturesAt(intentsRoot string) ([]string, error) {
	if _, err := os.Stat(intentsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return config.ScanFeatureTree(intentsRoot)
}
