package commands

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/parlay/internal/parser"
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

	fmt.Println(string(data))
	return nil
}
