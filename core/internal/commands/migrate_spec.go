// parlay-feature: parlay-tool/multi-adapter
// parlay-component: spec-migration-report
//
// Walks each feature's surface.md, parses via the legacy parser, emits an
// equivalent surface.yaml, and writes a per-feature migration report
// listing free-text content with no closed-schema destination. Idempotent.

package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateSpecCmd = &cobra.Command{
	Use:   "migrate-spec",
	Short: "Convert each feature's surface.md to surface.yaml",
	Args:  cobra.NoArgs,
	RunE:  runMigrateSpec,
}

type specMigrationEntry struct {
	Feature string `yaml:"feature"`
	Wrote   string `yaml:"wrote,omitempty"`
	Skipped string `yaml:"skipped,omitempty"`
	Note    string `yaml:"note,omitempty"`
}

func runMigrateSpec(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	intentsRoot := filepath.Join(cfg.Root.Path, "spec", "intents")
	entries, err := walkSurfaceMDFiles(intentsRoot)
	if err != nil {
		return fmt.Errorf("walk intents tree: %w", err)
	}

	var report []specMigrationEntry
	migrated, alreadyMigrated := 0, 0

	for _, mdPath := range entries {
		feature, _ := filepath.Rel(intentsRoot, filepath.Dir(mdPath))
		yamlPath := filepath.Join(filepath.Dir(mdPath), "surface.yaml")

		if _, err := os.Stat(yamlPath); err == nil {
			alreadyMigrated++
			report = append(report, specMigrationEntry{
				Feature: feature,
				Skipped: "surface.yaml already present",
			})
			continue
		}

		fragments, err := parser.ParseSurfaceFile(mdPath)
		if err != nil {
			report = append(report, specMigrationEntry{
				Feature: feature,
				Note:    fmt.Sprintf("legacy parse failed: %v", err),
			})
			continue
		}

		out := surfaceYAMLEmit(feature, fragments)
		if err := os.WriteFile(yamlPath, out, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", yamlPath, err)
		}
		migrated++
		report = append(report, specMigrationEntry{
			Feature: feature,
			Wrote:   yamlPath,
		})
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Migrated %d; already-migrated %d\n", migrated, alreadyMigrated)
	for _, e := range report {
		switch {
		case e.Wrote != "":
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — wrote %s\n", e.Feature, e.Wrote)
		case e.Skipped != "":
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — %s\n", e.Feature, e.Skipped)
		case e.Note != "":
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — %s\n", e.Feature, e.Note)
		}
	}
	if migrated > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "surface.md left in place — delete after reviewing the YAML and the unrouted-content report.")
	}
	return nil
}

func walkSurfaceMDFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Never descend into a unit. Migration rewrites spec artifacts
			// into their newer shape, which for a unit means editing files
			// that describe hand-written code the tool does not own.
			if config.IsAuthoredUnit(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(path) == "surface.md" {
			out = append(out, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

// surfaceYAMLEmit serializes the legacy []Fragment shape into surface.yaml
// matching the LoadSurfaceYAML expected layout.
func surfaceYAMLEmit(feature string, fragments []parser.Fragment) []byte {
	doc := struct {
		Feature   string                   `yaml:"feature,omitempty"`
		Fragments []map[string]interface{} `yaml:"fragments"`
	}{Feature: feature}
	for _, f := range fragments {
		entry := map[string]interface{}{
			"name":    f.Name,
			"shows":   f.Shows,
			"actions": f.Actions,
			"source":  f.Source,
			"page":    f.Page,
			"region":  f.Region,
		}
		if f.Order != 0 {
			entry["order"] = f.Order
		}
		if len(f.Notes) > 0 {
			entry["notes"] = f.Notes
		}
		doc.Fragments = append(doc.Fragments, entry)
	}
	out, _ := yaml.Marshal(doc)
	return out
}
