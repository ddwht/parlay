package commands

import (
	"encoding/json"
	"fmt"
	"os"
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

// writeSchemaDigest materializes DIGEST.md alongside the deployed schemas.
// Called from init and upgrade so the digest can never be staler than the
// schemas it summarizes — a stale cheat sheet is worse than none, because it
// is trusted.
func writeSchemaDigest(schemasPath string) error {
	d, err := embedded.BuildSchemaDigest()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(schemasPath, "DIGEST.md"),
		[]byte(embedded.RenderSchemaDigestMarkdown(d)), 0644)
}

// schemasPathFor returns the deployed schema directory for a root.
func schemasPathFor(root string) string {
	return filepath.Join(root, config.ParlayDir, config.SchemasDir)
}
