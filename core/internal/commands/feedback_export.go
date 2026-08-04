// parlay-feature: parlay-tool/feedback-mode
// parlay-component: export
//
// Bundles the local logs into one file a person can read and then send.
//
// The bundle exists even though capture is already safe, for two reasons.
// A user asked to "send your feedback log" needs one path, not a directory
// of day files. And the bundle is where the version floor is enforced:
// logs written before the sanitising redesign are refused outright, which
// is a mechanical guarantee rather than a warning someone may not read.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/feedback"
	"github.com/spf13/cobra"
)

var (
	feedbackExportOut   string
	feedbackExportSince string
)

var feedbackExportCmd = &cobra.Command{
	Use:   "feedback-export",
	Short: "Bundle the feedback logs into one reviewable file to send",
	Args:  cobra.NoArgs,
	RunE:  runFeedbackExport,
}

func init() {
	feedbackExportCmd.Flags().StringVar(&feedbackExportOut, "out", "",
		"Where to write the bundle (default: parlay-feedback-<date>.jsonl in the current directory)")
	feedbackExportCmd.Flags().StringVar(&feedbackExportSince, "since", "",
		"Only include events on or after this date (YYYY-MM-DD)")
}

func runFeedbackExport(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	dir := feedback.LogDir(cfg.Root.Path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("no feedback logs at %s — turn the mode on with `feedback: true` in .parlay/config.yaml, reproduce the problem, then export", dir)
	}

	var since time.Time
	if feedbackExportSince != "" {
		since, err = time.Parse("2006-01-02", feedbackExportSince)
		if err != nil {
			return fmt.Errorf("--since %q: expected YYYY-MM-DD", feedbackExportSince)
		}
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		// .salt is excluded by construction, not by filter order. It is the
		// one file in this directory that must never leave: with it, every
		// hash in the bundle becomes reversible by dictionary.
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if !since.IsZero() {
			day, perr := time.Parse("2006-01-02", strings.TrimSuffix(name, ".jsonl"))
			if perr != nil || day.Before(since) {
				continue
			}
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no feedback logs matched — nothing to export")
	}

	kept, skipped, subjects, err := collectExportable(files)
	if err != nil {
		return err
	}
	if len(kept) == 0 {
		return fmt.Errorf("every event was written by a version that predates the current format (%d skipped). Those logs are never exported; remove them with `parlay feedback-prune --legacy` and reproduce the problem again", skipped)
	}

	out := feedbackExportOut
	if out == "" {
		out = fmt.Sprintf("parlay-feedback-%s.jsonl", time.Now().UTC().Format("2006-01-02"))
	}

	var b strings.Builder
	b.WriteString(exportPreamble(len(kept), skipped, len(subjects)))
	for _, line := range kept {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := atomicfile.WriteAtomic(out, []byte(b.String())); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", out)
	fmt.Fprintf(cmd.OutOrStdout(), "  events:   %d\n", len(kept))
	if skipped > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  skipped:  %d (written before the current format — never exported)\n", skipped)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  subjects: %d, replaced with subject-1..%d\n", len(subjects), len(subjects))
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Read it before you send it — it is plain JSON, one event per line.")
	return nil
}

// collectExportable filters to the current format and replaces subject
// hashes with ordinals.
//
// The ordinal step is not extra safety — the hash is already salted and
// un-reversible without .salt. It is readability: an investigating agent
// reasoning about "subject-1 failed four times" does better than one
// staring at 9f2ac41b8e07, and an ordinal has no relationship to the
// input at all, so it also removes the salt file's importance.
func collectExportable(files []string) (kept []string, skipped int, subjects map[string]string, err error) {
	subjects = map[string]string{}
	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, 0, nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var ev feedback.Event
			if json.Unmarshal([]byte(line), &ev) != nil {
				// Unparseable lines are dropped rather than passed
				// through: an export must never carry bytes nothing
				// understood.
				skipped++
				continue
			}
			// The version floor. This is what makes pre-redesign logs —
			// the ones holding argv, paths and message text — impossible
			// to send, rather than merely discouraged.
			if ev.V < feedback.SchemaVersion {
				skipped++
				continue
			}
			if s := ev.Data["subject"]; s != "" {
				if _, seen := subjects[s]; !seen {
					subjects[s] = fmt.Sprintf("subject-%d", len(subjects)+1)
				}
				ev.Data["subject"] = subjects[s]
			}
			encoded, mErr := json.Marshal(ev)
			if mErr != nil {
				skipped++
				continue
			}
			kept = append(kept, string(encoded))
		}
	}
	return kept, skipped, subjects, nil
}

// exportPreamble states the data contract inside the bundle, so the person
// sending it and the agent reading it see the same claim. A promise made
// only in documentation is one neither party has in front of them.
func exportPreamble(events, skipped, subjects int) string {
	var b strings.Builder
	b.WriteString("# parlay feedback bundle\n")
	b.WriteString("#\n")
	b.WriteString("# WHAT IS IN HERE: parlay's own error codes, phase and command names,\n")
	b.WriteString("# coarse timings, and opaque labels (subject-1, subject-2) standing in for\n")
	b.WriteString("# feature and operation names.\n")
	b.WriteString("#\n")
	b.WriteString("# WHAT IS NOT: file paths, message text, argv, error strings, or any content\n")
	b.WriteString("# from spec files. None of it is captured, so none of it can appear here.\n")
	b.WriteString("#\n")
	b.WriteString(fmt.Sprintf("# events: %d   subjects: %d   skipped (pre-format): %d\n", events, subjects, skipped))
	b.WriteString("# Every line below is one JSON event.\n")
	return b.String()
}

// feedbackPruneCmd deletes logs. Parlay never removes these on its own —
// they are the user's file — so this is the explicit way to do it.
var feedbackPruneLegacy bool

var feedbackPruneCmd = &cobra.Command{
	Use:   "feedback-prune",
	Short: "Delete feedback logs (all of them, or only those predating the current format)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := mustContext(cmd)
		if err != nil {
			return err
		}
		dir := feedback.LogDir(cfg.Root.Path)
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), "No feedback logs to remove.")
			return nil
		}

		removed := 0
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			path := filepath.Join(dir, name)
			if feedbackPruneLegacy && !isLegacyLog(path) {
				continue
			}
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %d log file(s) from %s\n", removed, dir)
		return nil
	},
}

func init() {
	feedbackPruneCmd.Flags().BoolVar(&feedbackPruneLegacy, "legacy", false,
		"Only remove logs containing events written before the current format")
}

// isLegacyLog reports whether a file holds any event below the current
// schema version. Conservative: a file is legacy if ANY line is, because a
// day file can span an upgrade and the old lines are the unsafe ones.
func isLegacyLog(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev feedback.Event
		if json.Unmarshal([]byte(line), &ev) != nil || ev.V < feedback.SchemaVersion {
			return true
		}
	}
	return false
}

// CountLegacyLogs reports how many log files under a root predate the
// current format. Used by `upgrade` to warn without deleting anything.
func CountLegacyFeedbackLogs(rootPath string) int {
	entries, err := os.ReadDir(feedback.LogDir(rootPath))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if isLegacyLog(filepath.Join(feedback.LogDir(rootPath), e.Name())) {
			n++
		}
	}
	return n
}
