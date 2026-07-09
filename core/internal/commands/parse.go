// parlay-feature: parlay-tool/multi-root
// parlay-component: cross-root-reference-validation-error

package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var parseCmd = &cobra.Command{
	Use:   "parse --type <type> <path>",
	Short: "Parse a file and output structured JSON",
	Args:  cobra.ExactArgs(1),
	RunE:  runParse,
}

var parseType string
var parseJSON bool

func init() {
	parseCmd.Flags().StringVar(&parseType, "type", "", "File type: intents, dialogs, surface")
	parseCmd.MarkFlagRequired("type")
	parseCmd.Flags().BoolVar(&parseJSON, "json", true, "Output as JSON (default true)")
}

func runParse(cmd *cobra.Command, args []string) error {
	path := args[0]

	// Multi-root cross-root reference guard: any file we are about to
	// parse as authored content (intents, dialogs) is checked for
	// `web:@feat`-style cross-root prefixes. Cross-root references in
	// authored content are not supported in v1; surface as a validation
	// error before parsing proceeds.
	if parseType == "intents" || parseType == "dialogs" {
		if body, err := os.ReadFile(path); err == nil {
			if errs := parser.ValidateNoCrossRootRefsInContent(path, body); len(errs) > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "[ERR] cross-root references in intent content are not supported in v1")
				for _, e := range errs {
					fmt.Fprintf(cmd.ErrOrStderr(), "  at %s:%d (%s)\n", e.File, e.Line, e.Ref)
				}
				return fmt.Errorf("cross-root references found in %s", path)
			}
		}
	}

	var result interface{}
	var err error

	switch parseType {
	case "intents":
		result, err = parser.ParseIntentsFile(path)
	case "dialogs":
		result, err = parser.ParseDialogsFile(path)
	case "surface":
		result, err = parser.ParseSurfaceFile(path)
	default:
		return fmt.Errorf("unknown type %q — supported: intents, dialogs, surface", parseType)
	}

	if err != nil {
		return fmt.Errorf("parse failed: %w", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON encoding failed: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
