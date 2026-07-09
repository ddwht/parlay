package commands

// Generated from buildfile component: page-lock-confirmation
// Type: interactive-prompt | Widget: bufio-prompt | Layout: command-with-confirmation

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var lockPageCmdImpl = &cobra.Command{
	Use:   "lock-page <page-name>",
	Short: "Lock a page layout into a manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runLockPage,
}

func runLockPage(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	// Data input: page-name from command-argument
	pageName := args[0]
	manifestPath := filepath.Join(cfg.PagesPath(), pageName+".page.md")

	if _, err := os.Stat(manifestPath); err == nil {
		return fmt.Errorf("page manifest already exists at %s", manifestPath)
	}

	// Data input: assembled-regions from reuse page-assembly-view logic
	allFragments, err := parser.ScanAllSurfaces(filepath.Join(cfg.Root.Path, config.SpecDir))
	if err != nil {
		return fmt.Errorf("failed to scan surfaces: %w", err)
	}

	var targeted []parser.Fragment
	for _, f := range allFragments {
		if f.Page == pageName {
			targeted = append(targeted, f)
		}
	}

	if len(targeted) == 0 {
		return fmt.Errorf("no fragments target page %q — nothing to lock", pageName)
	}

	// Operation: assemble page view
	regions, _ := assembleRegions(targeted)

	// Element: layout-preview (grouped-output → headed-section)
	fmt.Fprintf(cmd.OutOrStdout(), "Layout to lock for %q:\n\n", pageName)
	for _, region := range regions {
		fmt.Fprintf(cmd.OutOrStdout(), "**%s**:\n", region.Name)
		for i, frag := range region.Fragments {
			fmt.Fprintf(cmd.OutOrStdout(), "  %d. @%s/%s\n", i+1, frag.Feature, parser.Slugify(frag.Name))
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Element: manifest-path (path-reference → path-line)
	fmt.Fprintf(cmd.OutOrStdout(), "Will create %s\n", manifestPath)

	// Element: owner-prompt (text-output → fmt.Println)
	// Action: read-owner (text-input → text-prompt)
	fmt.Fprint(cmd.OutOrStdout(), "Who should own this page? > ")
	reader := bufio.NewReader(os.Stdin)
	owner, _ := reader.ReadString('\n')
	owner = strings.TrimSpace(owner)

	// Operation: create-directory "spec/pages/"
	if err := os.MkdirAll(cfg.PagesPath(), 0755); err != nil {
		return fmt.Errorf("failed to create pages directory: %w", err)
	}

	// Operation: create-file manifest using template
	manifest := generateManifest(pageName, owner, regions)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", manifestPath)
	fmt.Fprintln(cmd.OutOrStdout(), "Status: draft")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Set the status to \"reviewed\" or \"locked\" when you're satisfied with the layout.")

	return nil
}

func generateManifest(pageName, owner string, regions []regionView) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", toTitleCase(pageName)))

	if owner != "" {
		b.WriteString(fmt.Sprintf("**Owner**: %s\n", owner))
	}
	b.WriteString("**Status**: draft\n\n")

	for _, region := range regions {
		b.WriteString(fmt.Sprintf("## %s\n\n", region.Name))
		for i, frag := range region.Fragments {
			b.WriteString(fmt.Sprintf("%d. @%s/%s\n", i+1, frag.Feature, parser.Slugify(frag.Name)))
		}
		b.WriteString("\n")
	}

	return b.String()
}
