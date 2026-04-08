package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var scanGeneratedCmd = &cobra.Command{
	Use:   "scan-generated <source-root>",
	Short: "List files containing parlay-component markers (JSON output for agent consumption)",
	Long: `Walk a directory recursively and emit a JSON list of every file
containing a parlay marker. Used by /parlay-generate-code to find which
files belong to which buildfile components, so incremental rebuilds know
which files to update or delete without re-deriving filenames from the
adapter naming convention.

Files without a parlay marker are user-owned and excluded from the output.`,
	Args: cobra.ExactArgs(1),
	RunE: runScanGenerated,
}

type scanOutput struct {
	SourceRoot string          `json:"source_root"`
	Files      []parser.Marker `json:"files"`
}

func runScanGenerated(cmd *cobra.Command, args []string) error {
	root := args[0]

	if _, err := os.Stat(root); err != nil {
		// Root doesn't exist yet — emit empty result rather than error.
		// This is the first-generation case where source-root has not
		// been created.
		return emitScanJSON(&scanOutput{SourceRoot: root, Files: []parser.Marker{}})
	}

	markers, err := parser.ScanGenerated(root)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	if markers == nil {
		markers = []parser.Marker{}
	}
	return emitScanJSON(&scanOutput{SourceRoot: root, Files: markers})
}

func emitScanJSON(output *scanOutput) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
