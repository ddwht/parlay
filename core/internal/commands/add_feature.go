package commands

// parlay-feature: parlay-tool/authoring
// parlay-component: FeatureScaffoldConfirmation
// parlay-extends: initiatives/FeatureCreationResult

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var initiativeFlag string

// The --authored mode's inputs. A unit is declared, not authored into
// existence: the code already exists, so the only thing being created is
// the declaration that makes it visible to parlay.
var (
	authoredFlag        bool
	authoredSourcesFlag []string
	authoredTestsFlag   []string
	authoredSummaryFlag string
)

var addFeatureCmd = &cobra.Command{
	Use:   "add-feature <name>",
	Short: "Create a new feature folder with intents.md and dialogs.md",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runAddFeature,
}

func init() {
	addFeatureCmd.Flags().StringVar(&initiativeFlag, "initiative", "", "Create the feature inside this initiative (auto-creates the initiative if needed)")
	addFeatureCmd.Flags().BoolVar(&authoredFlag, "authored", false,
		"Declare a hand-authored unit instead of a feature: writes authored.yaml, no dialogs.md, and no handoff directory")
	addFeatureCmd.Flags().StringArrayVar(&authoredSourcesFlag, "sources", nil,
		"Root-relative glob naming the unit's sources; repeatable. Required with --authored")
	addFeatureCmd.Flags().StringArrayVar(&authoredTestsFlag, "tests", nil,
		"Root-relative glob naming the unit's own tests; repeatable")
	addFeatureCmd.Flags().StringVar(&authoredSummaryFlag, "summary", "",
		"One line stating what the unit is; defaults to the display name")
}

// unitTreeRoots is threeTreeRoots minus the handoff tree.
//
// A unit produces no engineering handoff — the handoff tree carries
// specifications for code about to be written, and a unit's code is
// already written. Creating the directory anyway would leave an empty
// twin that `repair` reports and then "fixes" forever.
func unitTreeRoots(cfg *config.Context) []string {
	return []string{cfg.IntentsRoot(), cfg.BuildRoot()}
}

// writeAuthoredUnit writes the two files that constitute a unit.
func writeAuthoredUnit(unitPath, slug, displayName string) error {
	summary := strings.TrimSpace(authoredSummaryFlag)
	if summary == "" {
		summary = displayName
	}

	intentsContent := scaffoldedIntents(displayName)
	if err := os.WriteFile(filepath.Join(unitPath, "intents.md"), []byte(intentsContent), 0644); err != nil {
		return fmt.Errorf("creating intents.md: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "schema_version: %d\n", agent.AuthoredUnitSchemaVersion)
	fmt.Fprintf(&b, "unit: %s\n", slug)
	fmt.Fprintf(&b, "summary: %q\n", summary)
	fmt.Fprintln(&b, "sources:")
	for _, s := range authoredSourcesFlag {
		fmt.Fprintf(&b, "  - %q\n", strings.TrimSpace(s))
	}
	if len(authoredTestsFlag) > 0 {
		fmt.Fprintln(&b, "tests:")
		for _, t := range authoredTestsFlag {
			fmt.Fprintf(&b, "  - %q\n", strings.TrimSpace(t))
		}
	}
	if err := os.WriteFile(filepath.Join(unitPath, config.AuthoredFile), []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("creating %s: %w", config.AuthoredFile, err)
	}
	return nil
}

// reportAuthoredUnit prints what was created and what to do next.
func reportAuthoredUnit(cmd *cobra.Command, unitPath string) {
	fmt.Fprintf(cmd.OutOrStdout(), "Declared hand-authored unit at %s/\n", unitPath)
	fmt.Fprintln(cmd.OutOrStdout(), "  intents.md")
	fmt.Fprintln(cmd.OutOrStdout(), "  "+config.AuthoredFile)
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "No dialogs.md and no handoff directory: a unit's code is already written, so there is no pipeline phase to feed and nothing to hand off.")
	fmt.Fprintln(cmd.OutOrStdout())
	if strings.TrimSpace(authoredSummaryFlag) == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "The summary defaulted to the display name — replace it with what the unit actually does; the phases that refuse to generate into it quote that line.")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Next: parlay validate --type authored %s\n", filepath.Join(unitPath, config.AuthoredFile))
}

// validateAuthoredFlags rejects the flag combinations that would produce a
// declaration the validator immediately refuses.
func validateAuthoredFlags() error {
	if !authoredFlag {
		if len(authoredSourcesFlag) > 0 || len(authoredTestsFlag) > 0 {
			return fmt.Errorf("--sources/--tests only apply with --authored; a feature's files come from its buildfile plan, not from a declaration")
		}
		return nil
	}
	if len(authoredSourcesFlag) == 0 {
		return fmt.Errorf("--authored requires at least one --sources glob: a unit owning no files declares nothing, and the declaration would fail validation with authored-field-missing")
	}
	return nil
}

func runAddFeature(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	name := strings.Join(args, " ")
	slug := parser.Slugify(name)

	if err := validateAuthoredFlags(); err != nil {
		return err
	}

	if initiativeFlag != "" {
		return runAddFeatureWithInitiative(cmd, cfg, name, slug, initiativeFlag)
	}

	featurePath := cfg.FeaturePath(slug)

	if _, err := os.Stat(featurePath); err == nil {
		return fmt.Errorf("feature %q already exists at %s", slug, featurePath)
	}

	displayName := toTitleCase(name)

	roots := threeTreeRoots(cfg)
	if authoredFlag {
		roots = unitTreeRoots(cfg)
	}
	for _, root := range roots {
		if mkErr := os.MkdirAll(filepath.Join(root, slug), 0755); mkErr != nil {
			return fmt.Errorf("creating feature directory in %s: %w", root, mkErr)
		}
	}

	if authoredFlag {
		if err := writeAuthoredUnit(featurePath, slug, displayName); err != nil {
			return err
		}
		reportAuthoredUnit(cmd, featurePath)
		return nil
	}

	intentsContent := scaffoldedIntents(displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "intents.md"), []byte(intentsContent), 0644); err != nil {
		return fmt.Errorf("creating intents.md: %w", err)
	}

	dialogsContent := fmt.Sprintf("# %s — Dialogs\n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "dialogs.md"), []byte(dialogsContent), 0644); err != nil {
		return fmt.Errorf("creating dialogs.md: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created feature at %s/\n", featurePath)
	fmt.Fprintln(cmd.OutOrStdout(), "  intents.md")
	fmt.Fprintln(cmd.OutOrStdout(), "  dialogs.md")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "Start with intents.md. When ready, run: parlay create-dialogs @%s\n", slug)

	return nil
}

// parlay-feature: initiatives
// parlay-component: FeatureCreationResult
func runAddFeatureWithInitiative(cmd *cobra.Command, cfg *config.Context, name, featureSlug, initiativeName string) error {
	initiativeSlug := parser.Slugify(initiativeName)

	intentsRoot := cfg.IntentsRoot()
	initiativePath := filepath.Join(intentsRoot, initiativeSlug)

	if config.HasIntentsMd(initiativePath) {
		return fmt.Errorf("`%s` exists at the top level as a feature, not an initiative. A feature and an initiative can't share a top-level slug. Either pick a different initiative name, or first move the existing `%s` feature into an initiative with parlay move-feature", initiativeSlug, initiativeSlug)
	}

	featurePath := filepath.Join(initiativePath, featureSlug)
	if _, err := os.Stat(featurePath); err == nil {
		return fmt.Errorf("feature `%s` already exists inside initiative `%s` at %s/. Pick a different feature name, or move the existing feature somewhere else first", featureSlug, initiativeSlug, featurePath)
	}

	initiativeCreated := false
	if _, err := os.Stat(initiativePath); os.IsNotExist(err) {
		for _, root := range threeTreeRoots(cfg) {
			if mkErr := os.MkdirAll(filepath.Join(root, initiativeSlug), 0755); mkErr != nil {
				return fmt.Errorf("creating initiative directory in %s: %w", root, mkErr)
			}
		}
		initiativeCreated = true
	}

	// The initiative directory itself is created in all three trees above:
	// it may later hold ordinary features, which do need a handoff twin.
	// Only the unit's own directory skips it.
	childRoots := threeTreeRoots(cfg)
	if authoredFlag {
		childRoots = unitTreeRoots(cfg)
	}
	for _, root := range childRoots {
		if mkErr := os.MkdirAll(filepath.Join(root, initiativeSlug, featureSlug), 0755); mkErr != nil {
			if initiativeCreated {
				fmt.Fprintf(cmd.OutOrStdout(), "[WARN] Created initiative %s (in deferred classification — no features yet), but couldn't create feature %s inside it: %v. Re-run the same command after fixing the issue — it's idempotent.\n", initiativeSlug, featureSlug, mkErr)
				return nil
			}
			return fmt.Errorf("creating feature directory in %s: %w", root, mkErr)
		}
	}

	displayName := toTitleCase(name)

	if authoredFlag {
		if err := writeAuthoredUnit(featurePath, featureSlug, displayName); err != nil {
			return err
		}
		if initiativeCreated {
			fmt.Fprintf(cmd.OutOrStdout(), "Initiative %s created.\n", initiativeSlug)
		}
		reportAuthoredUnit(cmd, featurePath)
		return nil
	}

	intentsContent := scaffoldedIntents(displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "intents.md"), []byte(intentsContent), 0644); err != nil {
		return fmt.Errorf("creating intents.md: %w", err)
	}
	dialogsContent := fmt.Sprintf("# %s — Dialogs\n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "dialogs.md"), []byte(dialogsContent), 0644); err != nil {
		return fmt.Errorf("creating dialogs.md: %w", err)
	}

	if initiativeCreated {
		fmt.Fprintf(cmd.OutOrStdout(), "Initiative %s created.\n", initiativeSlug)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Feature %s added to initiative %s at %s/.\n", featureSlug, initiativeSlug, featurePath)
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "Start with intents.md. When ready, run: parlay create-dialogs @%s/%s\n", initiativeSlug, featureSlug)

	return nil
}

func threeTreeRoots(cfg *config.Context) []string {
	return []string{
		cfg.IntentsRoot(),
		cfg.HandoffRoot(),
		cfg.BuildRoot(),
	}
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// scaffoldedIntents is the starting text of a new feature's intents.md.
//
// It used to be a heading, an empty blockquote and a rule — an author opened a
// blank document with no prompt of any kind. That is where drift starts: with
// nothing to write against, the easiest thing to describe is the screen you
// already have in mind, and intents.md freezes at the first green build with
// whatever that produced.
//
// The commented block is a template, not an example: it names each field and
// the distinction that field most often gets wrong, so the prompt travels with
// the file rather than living only in a module the author may never open.
// Commented because a scaffold must parse as an intents.md with ZERO intents —
// `no-intents` warns while authoring and errors at build, and a fake example
// intent would satisfy that check falsely.
func scaffoldedIntents(displayName string) string {
	return fmt.Sprintf(`# %s

> 

---

<!--
Write one intent per `+"`## `"+` heading. Delete this comment when you have one.

## <What the user wants, not what the product has>

**Goal**: <the user-world outcome and why it matters — not the operation the system performs>
**Persona**: <the role doing THIS job — "a person sending the tax report", not "accountant">
**Priority**: <P0 | P1 | P2 — cost of leaving the USER OUTCOME unmet, not build order>
**Context**: <the situation that makes this task arise — not "the user opens the X page">
**Action**: <the task-level act — "send the report to the authority", not "click Upload">
**Objects**: <domain concepts this touches — "tax report, tax number", not "the modal">

**Constraints**:
- <a limit the world imposes; an implementation limit belongs in infrastructure.md>

**Verify**:
- <independently testable evidence the outcome happened — one claim per bullet>

**Questions**:
- <a design choice genuinely still open>

Full guidance, including what to do when your domain IS software:
.parlay/schemas/intent.schema.md, section "Soft boundaries".
-->
`, displayName)
}
