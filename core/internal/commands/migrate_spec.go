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

// migrateSpecRetireMD deletes a feature's surface.md once surface.yaml covers
// it. Off by default: retirement is a deliberate, separate step from
// conversion, because a dual-surface feature keeps WORKING (ResolveSurfacePath
// prefers the .yaml) — what it does not keep is truthfulness, since nothing
// maintains the .md once the .yaml is canonical. The WP10 benchmark measured
// that cost three replicates out of three: an abandoned surface.md that still
// described retired behavior. In ledger projects retirement is a
// prerequisite, not an option — a frozen-looking prose artifact that
// contradicts the contract is exactly the misleading document the ledger
// model exists to prevent.
var migrateSpecRetireMD bool

func init() {
	migrateSpecCmd.Flags().BoolVar(&migrateSpecRetireMD, "retire-md", false,
		"Delete surface.md where surface.yaml exists and covers every fragment; refuses per-feature when the .md carries fragments the .yaml lacks")
}

type specMigrationEntry struct {
	Feature string `yaml:"feature"`
	Wrote   string `yaml:"wrote,omitempty"`
	Skipped string `yaml:"skipped,omitempty"`
	Retired string `yaml:"retired,omitempty"`
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
	migrated, alreadyMigrated, retired := 0, 0, 0

	for _, mdPath := range entries {
		feature, _ := filepath.Rel(intentsRoot, filepath.Dir(mdPath))
		yamlPath := filepath.Join(filepath.Dir(mdPath), "surface.yaml")

		if _, err := os.Stat(yamlPath); err == nil {
			if migrateSpecRetireMD {
				entry := retireSurfaceMD(feature, mdPath, yamlPath)
				if entry.Retired != "" {
					retired++
				}
				report = append(report, entry)
				continue
			}
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

	fmt.Fprintf(cmd.OutOrStdout(), "Migrated %d; already-migrated %d; retired %d\n", migrated, alreadyMigrated, retired)
	for _, e := range report {
		switch {
		case e.Wrote != "":
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — wrote %s\n", e.Feature, e.Wrote)
		case e.Retired != "":
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — retired %s\n", e.Feature, e.Retired)
		case e.Skipped != "":
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — %s\n", e.Feature, e.Skipped)
		case e.Note != "":
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — %s\n", e.Feature, e.Note)
		}
	}
	if migrated > 0 && !migrateSpecRetireMD {
		fmt.Fprintln(cmd.OutOrStdout(), "surface.md left in place — delete after reviewing the YAML, or re-run with --retire-md.")
	}
	if retired > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Retired surface.md files were signed into some buildfiles' source-signatures. Re-stamp each affected feature: parlay internal scaffold-signatures @{feature} — a signature naming a now-missing artifact fails the codegen freshness gate.")
	}
	return nil
}

// retireSurfaceMD deletes a feature's surface.md when its surface.yaml covers
// every fragment the .md declares (matched by slugified name — the .yaml is
// canonical, so content differences are expected and fine; a fragment the
// .yaml lacks entirely is not). Refusal is per-feature and names the missing
// fragments, so a partial project retires everything safe and reports the
// rest.
func retireSurfaceMD(feature, mdPath, yamlPath string) specMigrationEntry {
	mdFragments, err := parser.ParseSurfaceFile(mdPath)
	if err != nil {
		return specMigrationEntry{Feature: feature, Note: fmt.Sprintf("retire skipped: legacy parse failed: %v", err)}
	}
	yamlFragments, err := parser.ParseSurfaceFile(yamlPath)
	if err != nil {
		return specMigrationEntry{Feature: feature, Note: fmt.Sprintf("retire skipped: yaml parse failed: %v", err)}
	}
	inYAML := make(map[string]bool, len(yamlFragments))
	for _, f := range yamlFragments {
		inYAML[parser.Slugify(f.Name)] = true
	}
	var missing []string
	for _, f := range mdFragments {
		if !inYAML[parser.Slugify(f.Name)] {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) > 0 {
		return specMigrationEntry{Feature: feature, Note: fmt.Sprintf("retire refused: surface.md carries %d fragment(s) surface.yaml lacks: %v — reconcile first", len(missing), missing)}
	}
	if err := os.Remove(mdPath); err != nil {
		return specMigrationEntry{Feature: feature, Note: fmt.Sprintf("retire failed: %v", err)}
	}
	return specMigrationEntry{Feature: feature, Retired: mdPath}
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
