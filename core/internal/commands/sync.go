// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-sync

package commands

// Generated from buildfile component: coverage-report
// Type: report | Widget: sectioned-output | Layout: report-output

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var syncCmdImpl = &cobra.Command{
	Use:   "sync <@feature>",
	Short: "Check intent-dialog coverage",
	Args:  cobra.ExactArgs(1),
	RunE:  runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	// Data input: feature-ref from command-argument (strip @ prefix)
	slug := parser.FeatureSlug(args[0])
	featurePath := cfg.FeaturePath(slug)

	if err := refuseOnUnit(cfg, slug,
		"sync reconciles intents against dialogs, and a unit has no dialogs to reconcile against"); err != nil {
		return err
	}

	// Reconciliation is about what the feature currently promises, so it reads
	// the resolved set. A promise an applied amendment withdrew must not still
	// be reported as needing a dialog.
	res, err := resolveActiveIntents(cfg, slug)
	if err != nil {
		return fmt.Errorf("failed to read intents: %w", err)
	}
	intents := res.Active

	// Operation: read-file dialogs.md, parse using dialog-schema
	dialogsPath := filepath.Join(featurePath, "dialogs.md")
	dialogs, err := parser.ParseDialogsFile(dialogsPath)
	if err != nil {
		return fmt.Errorf("failed to read dialogs: %w", err)
	}

	// Computed: match intents to dialogs by title, slug, and word overlap
	type coverageMatch struct {
		IntentTitle string
		DialogTitle string
	}

	var covered []coverageMatch
	var uncoveredIntents []parser.Intent
	matchedDialogs := make(map[string]bool)

	for _, intent := range intents {
		found := false
		for _, dialog := range dialogs {
			if matchesIntent(intent, dialog) {
				covered = append(covered, coverageMatch{intent.Title, dialog.Title})
				matchedDialogs[dialog.Slug] = true
				found = true
				break
			}
		}
		if !found {
			uncoveredIntents = append(uncoveredIntents, intent)
		}
	}

	// Claim dialogs belonging to superseded intents before the orphan walk.
	// matchedDialogs is populated from the intent loop above, so a filtered
	// intent set would otherwise drop every retired intent's dialog into the
	// orphan list and present preserved history as cleanup debt.
	var retiredDialogs []parser.Dialog
	for _, sup := range res.Superseded {
		for _, dialog := range dialogs {
			if matchesIntent(sup.Intent, dialog) && !matchedDialogs[dialog.Slug] {
				matchedDialogs[dialog.Slug] = true
				retiredDialogs = append(retiredDialogs, dialog)
			}
		}
	}

	var orphanDialogs []parser.Dialog
	for _, dialog := range dialogs {
		if !matchedDialogs[dialog.Slug] {
			orphanDialogs = append(orphanDialogs, dialog)
		}
	}

	// Element: all-clear (visible-when: uncovered == 0 && orphans == 0)
	// The retired list is history worth showing even when nothing is wrong;
	// gating the all-clear on it too would otherwise return before the section
	// below ever prints, so a clean feature never learned what it had retired.
	if len(uncoveredIntents) == 0 && len(orphanDialogs) == 0 && len(retiredDialogs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "All intents are covered. No orphan dialogs.")
		return nil
	}

	// Element: covered-header + covered-list (visible-when: covered.length > 0)
	if len(covered) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Covered intents:")
		for _, m := range covered {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — matched by %s\n", m.IntentTitle, m.DialogTitle)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Element: uncovered-header + uncovered-list (visible-when: uncovered.length > 0)
	if len(uncoveredIntents) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Intents without dialogs:")
		for _, intent := range uncoveredIntents {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — no matching dialog found\n", intent.Title)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Element: orphan-header + orphan-list (visible-when: orphan.length > 0)
	if len(retiredDialogs) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Retired dialogs (their intent was superseded — history, not debt):")
		for _, dialog := range retiredDialogs {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", dialog.Title)
		}
	}

	if len(orphanDialogs) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Orphan dialogs (no matching intent):")
		for _, dialog := range orphanDialogs {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — doesn't trace to any intent\n", dialog.Title)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Action: template-prompt (selection → lettered-prompt, visible-when: uncovered.length > 0)
	if len(uncoveredIntents) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Generate dialog templates for uncovered intents?")
		fmt.Fprintln(cmd.OutOrStdout(), "  A: Yes, generate templates for all")
		fmt.Fprintln(cmd.OutOrStdout(), "  B: Let me pick which ones")
		fmt.Fprintln(cmd.OutOrStdout(), "  C: No, just the report")
		fmt.Fprint(cmd.OutOrStdout(), "> ")

		reader := bufio.NewReader(os.Stdin)
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToUpper(choice))

		switch choice {
		case "A":
			// Action: generate-all (file-operation, enabled-when: selected == A)
			if err := appendDialogTemplates(cmd, uncoveredIntents, dialogsPath); err != nil {
				return err
			}

		case "B":
			// Action: pick-specific (selection → lettered-prompt, enabled-when: selected == B)
			selected := promptForSelection(cmd, uncoveredIntents, reader)
			if len(selected) > 0 {
				if err := appendDialogTemplates(cmd, selected, dialogsPath); err != nil {
					return err
				}
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "No intents selected.")
			}

		case "C":
			// Action: dismiss (enabled-when: selected == C) — no-op
		}
	}

	// No editor hand-off here. This hook offered to open Studio "to reconcile
	// this drift visually" (spec/intents/studio-support/studio-cli-hooks/
	// intents.md:54) — a surface that was designed and never built. Because
	// `reconcile` was never a real subcommand, accepting the prompt made this
	// command exit non-zero after having successfully done its work.
	_ = cfg
	return nil
}

func promptForSelection(cmd *cobra.Command, intents []parser.Intent, reader *bufio.Reader) []parser.Intent {
	fmt.Fprintln(cmd.OutOrStdout(), "Which intents should I generate templates for?")
	for i, intent := range intents {
		letter := string(rune('A' + i))
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", letter, intent.Title)
	}
	fmt.Fprint(cmd.OutOrStdout(), "Enter letters (e.g., A,C): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToUpper(input))

	var selected []parser.Intent
	for _, ch := range strings.Split(input, ",") {
		ch = strings.TrimSpace(ch)
		if len(ch) == 1 {
			idx := int(ch[0] - 'A')
			if idx >= 0 && idx < len(intents) {
				selected = append(selected, intents[idx])
			}
		}
	}
	return selected
}

func appendDialogTemplates(cmd *cobra.Command, intents []parser.Intent, dialogsPath string) error {
	f, err := os.OpenFile(dialogsPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open dialogs.md: %w", err)
	}
	defer f.Close()

	for _, intent := range intents {
		tmpl := generateDialogTemplate(intent)
		if _, err := f.WriteString(tmpl); err != nil {
			return err
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added %d dialog templates to dialogs.md.\n", len(intents))
	return nil
}

func matchesIntent(intent parser.Intent, dialog parser.Dialog) bool {
	if strings.EqualFold(intent.Title, dialog.Title) {
		return true
	}
	if intent.Slug == dialog.Slug {
		return true
	}

	intentWords := significantWords(intent.Title)
	dialogWords := significantWords(dialog.Title)
	overlap := wordOverlap(intentWords, dialogWords)

	if len(intentWords) > 0 && len(dialogWords) > 0 {
		minLen := len(intentWords)
		if len(dialogWords) < minLen {
			minLen = len(dialogWords)
		}
		if float64(overlap)/float64(minLen) >= 0.6 {
			return true
		}
	}

	return false
}

func significantWords(s string) []string {
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"for": true, "to": true, "in": true, "on": true, "of": true,
		"with": true, "from": true, "by": true, "is": true, "it": true,
	}
	var words []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		if !stop[w] {
			words = append(words, w)
		}
	}
	return words
}

func wordOverlap(a, b []string) int {
	set := make(map[string]bool)
	for _, w := range a {
		set[w] = true
	}
	count := 0
	for _, w := range b {
		if set[w] {
			count++
		}
	}
	return count
}
