// parlay-feature: parlay-tool/multi-adapter
// parlay-component: config-migration-result
//
// Converts the legacy .parlay/config.yaml `prototype-framework:` field into
// a single-target presentation .parlay/adapter-set.yaml. The legacy field
// remains parseable in v1 with a prototype-framework-deprecated warning;
// outright removal is owned by a separate deprecation feature.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateConfigCmd = &cobra.Command{
	Use:   "migrate-config",
	Short: "Convert legacy prototype-framework: into a single-target presentation adapter-set",
	Args:  cobra.NoArgs,
	RunE:  runMigrateConfig,
}

func runMigrateConfig(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	configPath := cfg.ConfigPath()
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config %s: %w", configPath, err)
	}

	var legacy struct {
		PrototypeFramework string `yaml:"prototype-framework"`
		SDDFramework       string `yaml:"sdd-framework"`
		Parent             string `yaml:"parent,omitempty"`
	}
	if err := yaml.Unmarshal(configContent, &legacy); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if legacy.PrototypeFramework == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "no legacy fields detected; nothing to migrate")
		return nil
	}

	adapterSlug := slugifyFramework(legacy.PrototypeFramework)
	adapterSetPath := filepath.Join(filepath.Dir(configPath), "adapter-set.yaml")

	if _, err := os.Stat(adapterSetPath); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "adapter-set.yaml already exists at %s; not overwriting\n", adapterSetPath)
		return nil
	}

	out := fmt.Sprintf(`# parlay-feature: parlay-tool/multi-adapter
# parlay-component: config-migration-result
#
# Migrated from legacy prototype-framework: %s

name: migrated
targets:
  presentation:
    adapter: %s
    root: %s
`, legacy.PrototypeFramework, adapterSlug, defaultRootForFramework(adapterSlug))

	if err := os.WriteFile(adapterSetPath, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write adapter-set: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Conversion: %s -> adapter-set with %s\n", legacy.PrototypeFramework, adapterSlug)
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", adapterSetPath)
	fmt.Fprintln(cmd.OutOrStdout(), "prototype-framework is deprecated; outright removal is owned by a separate deprecation feature")
	return nil
}

// slugifyFramework maps a free-text framework label into an adapter slug.
// Handles the common shipped frameworks; falls back to a kebab-case
// transformation of the label.
func slugifyFramework(framework string) string {
	switch strings.ToLower(strings.TrimSpace(framework)) {
	case "go cli", "go cli + ai agent", "go-cli":
		return "go-cli"
	case "react + ant design", "react antd", "react-antd":
		return "react-antd"
	case "angular + clarity", "angular clarity", "angular-clarity":
		return "angular-clarity"
	case "angular + material", "angular material", "angular-material":
		return "angular-material"
	case "none (register adapter later)", "none", "":
		// The init menu's opt-out. Kebab-casing the label produced the slug
		// "none-(register-adapter-later)", which names no adapter and no file.
		return ""
	default:
		s := strings.ToLower(framework)
		s = strings.ReplaceAll(s, " ", "-")
		s = strings.ReplaceAll(s, "+", "")
		s = strings.ReplaceAll(s, "--", "-")
		return strings.Trim(s, "-")
	}
}

// defaultRootForFramework picks a sensible default source-root per framework
// slug, mirroring each bundled adapter's own file-conventions.source-root.
//
// The previous values did not match: it returned "internal/commands" for
// go-cli (the value this repo's own dogfood adapter uses, not the bundled
// one's "cmd/"), "src" for angular-clarity (whose adapter declares "src/app/"),
// and had no case for angular-material at all.
func defaultRootForFramework(slug string) string {
	switch slug {
	case "go-cli":
		return "cmd"
	case "react-antd":
		return "src"
	case "angular-clarity", "angular-material":
		return "src/app"
	default:
		return "src"
	}
}
