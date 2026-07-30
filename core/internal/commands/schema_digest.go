package commands

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/embedded"
	"github.com/spf13/cobra"
)

// schemaDigestCmd emits the derived schema digest.
var schemaDigestCmd = &cobra.Command{
	Use:   "schema-digest",
	Short: "Emit the derived schema digest — diagnostics and closed vocabularies (JSON or Markdown)",
	Args:  cobra.NoArgs,
	RunE:  runSchemaDigest,
}

var schemaDigestFormat string

func init() {
	schemaDigestCmd.Flags().StringVar(&schemaDigestFormat, "format", "md", "Output format: md or json")
}

func runSchemaDigest(cmd *cobra.Command, args []string) error {
	d, err := embedded.BuildSchemaDigest()
	if err != nil {
		return fmt.Errorf("build schema digest: %w", err)
	}
	switch schemaDigestFormat {
	case "json":
		buf, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	case "md":
		fmt.Fprint(cmd.OutOrStdout(), embedded.RenderSchemaDigestMarkdown(d))
	default:
		return fmt.Errorf("unknown format %q — supported: md, json", schemaDigestFormat)
	}
	return nil
}

// writeSchemaDigest moved to embedded.WriteSchemaDigest, beside the builder and
// renderer it calls. It was the only deployment write outside the packages
// TestNoDirectWritePrimitives scans, and stayed unconditional after the rest were
// converted — so a no-op upgrade rewrote DIGEST.md while reporting no changes.

// schemasPathFor returns the deployed schema directory for a root.
func schemasPathFor(root string) string {
	return filepath.Join(root, config.ParlayDir, config.SchemasDir)
}
