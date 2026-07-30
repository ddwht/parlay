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

var (
	lockPageOwner  string
	lockPageYes    bool
	lockPageRelock bool
	// lockPageTTYOverride lets tests fix the interactivity answer without a
	// fake PTY, matching the studio_hook.go convention.
	lockPageTTYOverride *bool
)

func init() {
	// The command had no flags at all, which is what made the prompt
	// unavoidable and therefore made a piped invocation write a manifest with
	// an empty Owner: line and exit 0.
	lockPageCmdImpl.Flags().StringVar(&lockPageOwner, "owner", "",
		"Who owns this page (skips the prompt; required when stdin is not a terminal)")
	lockPageCmdImpl.Flags().BoolVar(&lockPageYes, "yes", false,
		"Do not prompt; requires --owner")
	lockPageCmdImpl.Flags().BoolVar(&lockPageRelock, "relock", false,
		"Re-derive an existing manifest whose Status: is not locked")
}

func runLockPage(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	// Data input: page-name from command-argument
	pageName := args[0]
	manifestPath := filepath.Join(cfg.PagesPath(), pageName+".page.md")

	// Create-only, unless the manifest is explicitly re-derivable.
	//
	// A locked manifest is a decision someone made and must not be silently
	// replaced by whatever the surfaces currently say. A draft one is not —
	// refusing to re-derive it meant the only way to pick up a new fragment
	// was to delete the file, which loses the Owner: with it.
	if _, err := os.Stat(manifestPath); err == nil {
		if !lockPageRelock {
			return fmt.Errorf("page manifest already exists at %s (pass --relock to re-derive it, if its Status: is not locked)", manifestPath)
		}
		if status := manifestStatus(manifestPath); strings.EqualFold(status, "locked") {
			return fmt.Errorf("page manifest at %s is Status: locked — change the status by hand if you really mean to re-derive it", manifestPath)
		}
	}

	// Layout-validation precheck gate (parlay-cross-cutting-id:
	// layout-precheck-contract): a standalone *.layout.yaml already on
	// disk for this page must pass precheck before the (legacy,
	// region-based) manifest below is generated and locked. Pages
	// without a layout artifact are unaffected — the manifest-exists
	// check above already covers the embedded-layout case (a manifest
	// with an embedded Layout section is, by definition, an existing
	// manifest, and the check above already refuses to proceed).
	if verdict := layoutPrecheckGate(cfg, pageName); verdict.Code != "ok" {
		return refuseOnPrecheckVerdict(cmd, verdict)
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
	//
	// Three defects lived in the four lines this replaces. The prompt read
	// process-global os.Stdin rather than cmd.InOrStdin(), so it was
	// untestable and ignored any injected input. It discarded the read error.
	// And it had no TTY check — so under a pipe the read failed instantly,
	// owner came back empty, and the command wrote an OWNERLESS manifest and
	// exited 0. A manifest whose whole purpose is to record a layout decision
	// silently recorded nobody as having made it.
	owner := strings.TrimSpace(lockPageOwner)
	if owner == "" {
		if lockPageYes {
			return fmt.Errorf("--yes requires --owner: a manifest with no owner records a decision nobody made")
		}
		if !ttyInteractive(lockPageTTYOverride) {
			return fmt.Errorf("stdin is not a terminal; pass --owner <name> (with --yes) instead of relying on the prompt")
		}
		fmt.Fprint(cmd.OutOrStdout(), "Who should own this page? > ")
		reader := bufio.NewReader(cmd.InOrStdin())
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return fmt.Errorf("could not read an owner: %w", err)
		}
		owner = strings.TrimSpace(line)
	}
	if owner == "" {
		return fmt.Errorf("no owner given — refusing to write an ownerless manifest")
	}

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

// manifestStatus reads the Status: line out of an existing manifest. Returns
// "" when the file cannot be read or declares none — an unreadable manifest
// is not a locked one, and the caller's refusal message is clearer than a
// swallowed error here.
func manifestStatus(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// Both spellings: generateManifest writes the bolded markdown form, and a
	// hand-written manifest may use the plain one. Matching only one is how a
	// locked manifest gets treated as a draft.
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"**Status**:", "Status:"} {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				return strings.TrimSpace(rest)
			}
		}
	}
	return ""
}
