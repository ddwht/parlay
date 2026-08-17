// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: refine-preflight
//
// The write half of the refine journal. check_applied.go owns the type and
// the read path (the pre-flight reports an in-flight run); this owns the
// stamping a running refinement does as it completes each step, and the
// clear that ends it.
//
// Why a command rather than the skill writing YAML directly: the step
// vocabulary is closed, and a journal with an invented step name resumes at
// the wrong place — worse than no journal, because it looks authoritative.
// Routing writes through here makes an unknown step an error at the moment
// it is written rather than a wrong answer on the next run.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var refineJournalCmd = &cobra.Command{
	Use:   "refine-journal <@feature>",
	Short: "Record or clear a refinement's step progress so an interrupted run can resume (JSON)",
	Args:  cobra.ExactArgs(1),
	RunE:  runRefineJournal,
}

var (
	refineJournalStep      string
	refineJournalAmendment int
	refineJournalAsk       string
	refineJournalClear     bool
)

func init() {
	refineJournalCmd.Flags().StringVar(&refineJournalStep, "step", "",
		"Mark this step complete. One of: "+strings.Join(refineJournalSteps, ", "))
	refineJournalCmd.Flags().IntVar(&refineJournalAmendment, "amendment", 0,
		"Record the amendment sequence number this run wrote (set alongside --step amendment-written)")
	refineJournalCmd.Flags().StringVar(&refineJournalAsk, "ask", "",
		"Record the refinement prose this run is executing, so a resumed run can confirm it is the same job")
	refineJournalCmd.Flags().BoolVar(&refineJournalClear, "clear", false,
		"Delete the journal — the refinement finished (step 9's re-baseline) or was abandoned")
}

func runRefineJournal(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	path := refineJournalPath(cfg, slug)

	if refineJournalClear {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clear refine journal: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "refine journal cleared for %s\n", slug)
		return nil
	}

	journal, err := loadRefineJournal(cfg, slug)
	if err != nil {
		return err
	}
	if journal == nil {
		journal = &refineJournal{
			Feature:   slug,
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			Completed: []string{},
		}
	}
	if refineJournalAsk != "" {
		journal.Ask = refineJournalAsk
	}
	if refineJournalAmendment > 0 {
		journal.Amendment = refineJournalAmendment
	}

	if refineJournalStep != "" {
		if !knownRefineStep(refineJournalStep) {
			return fmt.Errorf("unknown refine step %q — the vocabulary is closed: %s",
				refineJournalStep, strings.Join(refineJournalSteps, ", "))
		}
		// Idempotent: re-stamping a completed step is a no-op rather than a
		// duplicate entry, so a retried CLI call cannot corrupt the order.
		already := false
		for _, s := range journal.Completed {
			if s == refineJournalStep {
				already = true
				break
			}
		}
		if !already {
			journal.Completed = append(journal.Completed, refineJournalStep)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create build directory: %w", err)
	}
	data, err := yaml.Marshal(journal)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write refine journal: %w", err)
	}

	out := struct {
		Feature   string   `json:"feature"`
		Amendment int      `json:"amendment,omitempty"`
		Completed []string `json:"completed"`
		NextStep  string   `json:"next_step,omitempty"`
	}{
		Feature:   journal.Feature,
		Amendment: journal.Amendment,
		Completed: journal.Completed,
		NextStep:  journal.NextRefineStep(),
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
	return nil
}

func knownRefineStep(name string) bool {
	for _, s := range refineJournalSteps {
		if s == name {
			return true
		}
	}
	return false
}
