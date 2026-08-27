package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
)

// A decision can outlive what it was about. The case it approved gets renamed,
// strengthened to full coverage, or deleted; the criterion it waived stops
// being declared. Reporting those is only half an answer — without a way to
// retire one, the only route forward is hand-editing the ledger, which is
// exactly the unreviewable change the ledger exists to prevent.
//
// Retiring is itself a decision and is recorded as one. The entry is not
// deleted: an approval that silently vanishes is indistinguishable from one
// nobody made, and the question "who decided this no longer applies, and why"
// has to stay answerable.
var retireDecisionCmd = &cobra.Command{
	Use:   "retire-decision <@feature>",
	Short: "Record that a coverage decision no longer applies",
	Long: `Retire a coverage decision whose subject has changed or gone.

Use this when a state-only approval is reported stale or orphaned, or when a
waiver's criterion is no longer declared. The decision is moved to the retired
list with a reason and attribution rather than removed, so the ledger still
answers who decided it no longer applies.

To re-approve a case whose content drifted, retire the old decision and record
a fresh one against the case as it now stands. Editing the old decision in
place would make one review look like two.`,
	Args: cobra.ExactArgs(1),
	RunE: runRetireDecision,
}

var (
	retireDecisionRef    string
	retireDecisionText   string
	retireDecisionSuite  string
	retireDecisionCase   string
	retireDecisionReason string
	retireDecisionBy     string
)

func init() {
	f := retireDecisionCmd.Flags()
	f.StringVar(&retireDecisionRef, "ref", "", "the decision's criterion ref")
	f.StringVar(&retireDecisionText, "criterion", "", "the decision's criterion text")
	f.StringVar(&retireDecisionSuite, "suite", "", "state-only: the suite the decision named")
	f.StringVar(&retireDecisionCase, "case", "", "state-only: the case the decision named")
	f.StringVar(&retireDecisionReason, "reason", "", "required: why this decision no longer applies")
	f.StringVar(&retireDecisionBy, "by", "", "required: what decided this")
}

type retireDecisionOutput struct {
	Feature   string `json:"feature"`
	Retired   string `json:"retired"`
	Remaining int    `json:"remaining_decisions"`
}

func runRetireDecision(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	if strings.TrimSpace(retireDecisionReason) == "" || strings.TrimSpace(retireDecisionBy) == "" {
		return fmt.Errorf("--reason and --by are required: retiring a decision is itself a decision, and one recorded without a reason or attribution cannot be reviewed")
	}
	if strings.TrimSpace(retireDecisionRef) == "" {
		return fmt.Errorf("--ref is required: it names which decision to retire")
	}

	rec, err := loadCoverageExceptions(cfg, slug)
	if err != nil {
		return err
	}
	if rec == nil || len(rec.Exceptions) == 0 {
		return fmt.Errorf("%s has no recorded coverage decisions", slug)
	}

	wantText := agent.CanonicalCriterionText(retireDecisionText)
	var kept []CoverageException
	var found *CoverageException
	for i := range rec.Exceptions {
		ex := rec.Exceptions[i]
		match := ex.Ref == retireDecisionRef &&
			agent.CanonicalCriterionText(ex.Text) == wantText
		// A state-only decision is identified by its case too, so retiring one
		// must not silently take a sibling that shares the criterion.
		if match && ex.Kind == ExceptionStateOnly {
			match = ex.Suite == retireDecisionSuite && ex.Case == retireDecisionCase
		}
		if match && found == nil {
			found = &ex
			continue
		}
		kept = append(kept, ex)
	}
	if found == nil {
		return fmt.Errorf("%s has no decision for %s%s — nothing to retire. Check the ref, criterion text, and for a downgrade the suite and case, exactly as the ledger records them",
			slug, retireDecisionRef, describeCaseSelector())
	}

	rec.Exceptions = kept
	rec.RetiredDecisions = append(rec.RetiredDecisions, RetiredDecision{
		Ref: found.Ref, Text: found.Text, Kind: found.Kind,
		Suite: found.Suite, Case: found.Case,
		OriginalReason: found.Reason, OriginalBy: found.By, OriginalAt: found.At,
		Reason: retireDecisionReason, By: retireDecisionBy, At: time.Now().UTC().Format(time.RFC3339),
	})

	if err := saveCoverageExceptions(cfg, slug, rec); err != nil {
		return err
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(retireDecisionOutput{
		Feature: slug, Retired: retireDecisionRef, Remaining: len(rec.Exceptions),
	})
}

func describeCaseSelector() string {
	if retireDecisionSuite == "" && retireDecisionCase == "" {
		return ""
	}
	return fmt.Sprintf(" in suite %q case %q", retireDecisionSuite, retireDecisionCase)
}
