// parlay-feature: parlay-tool/criterion-authority
// parlay-component: criteria-authority-cli

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/parser"
)

var (
	authorizeCriteriaMode string
	approveCriteriaBy     string
	approveCriteriaID     string
)

// machineAuthorizationMode is the only accepted value, spelled out rather than
// implied by a bare boolean. `--authorize-criteria=machine` says what is being
// authorized and by what; `--yes` or `--approve-all` would say neither, and a
// governance waiver should not read like a convenience switch.
const machineAuthorizationMode = "machine"

var criteriaAuthorityCmd = &cobra.Command{
	Use:   "criteria-authority <@feature>",
	Short: "Report who approved the criteria a feature is graded against (JSON)",
	Long: `Report the standard a feature is graded against, and who approved it.

A feature's acceptance criteria are what its tests must discharge. They are
produced by rewriting each intent's Verify bullets into atomic claims and
routing each to a fragment or an operation — a lossy transformation that runs
after the designer's only choice and is otherwise never shown.

This reports the current criterion set, whether a person has approved it, and
what changed if they approved a different one.`,
	Args: cobra.ExactArgs(1),
	RunE: runCriteriaAuthority,
}

var approveCriteriaCmd = &cobra.Command{
	Use:   "approve-criteria <@feature>",
	Short: "Record that the criteria a feature is graded against were approved",
	Long: `Record approval of a feature's current criterion set.

--by names what accepted the standard and is required: it comes from the
decision channel that asked, never from the environment. Reading the
environment for a reviewer is how the artifact this replaces came to record a
background process as a person, and an identity the tool invents is evidence of
nothing.`,
	Args: cobra.ExactArgs(1),
	RunE: runApproveCriteria,
}

func init() {
	approveCriteriaCmd.Flags().StringVar(&approveCriteriaBy, "by", "",
		"what accepted the standard, supplied by the decision channel (required)")
	approveCriteriaCmd.Flags().StringVar(&approveCriteriaID, "decision-id", "",
		"identifier of the interaction that produced this approval, so it can be traced rather than merely asserted")
	criteriaAuthorityCmd.Flags().StringVar(&authorizeCriteriaMode, "authorize-criteria", "",
		"set to \"machine\" to preview whether an advancing run would be permitted to proceed without human "+
			"approval. This command reports only — the waiver is exercised, and recorded, by the boundary that "+
			"actually advances (parlay internal gate --authorize-criteria=machine)")
}

type criteriaAuthorityOutput struct {
	Feature  string                `json:"feature"`
	Criteria []AuthorizedCriterion `json:"criteria"`
	Hash     string                `json:"criteria_hash"`
	// Authorized reports whether this invocation may proceed.
	Authorized bool `json:"authorized"`
	// Machine is true when it may proceed only because this run waived the
	// separation, which is not the same as having met it.
	Machine bool                  `json:"machine_authorized"`
	Reason  string                `json:"reason"`
	Added   []AuthorizedCriterion `json:"added,omitempty"`
	Removed []AuthorizedCriterion `json:"removed,omitempty"`
}

func runCriteriaAuthority(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	if authorizeCriteriaMode != "" && authorizeCriteriaMode != machineAuthorizationMode {
		return fmt.Errorf("--authorize-criteria=%q is not a mode; the only value is %q", authorizeCriteriaMode, machineAuthorizationMode)
	}
	machine := authorizeCriteriaMode == machineAuthorizationMode

	verdict, current, err := CheckCriteriaAuthority(cfg, slug, machine)
	if err != nil {
		return err
	}

	// This command REPORTS; it does not advance anything, so it records
	// nothing. Appending here logged a waiver for a run that never proceeded,
	// while the boundary that actually advanced refused — it was handed
	// machineFlag=false by a caller with no way to pass anything else. The
	// audit event now belongs to the gate, which is the run that goes on to
	// generate code.

	out := criteriaAuthorityOutput{
		Feature: slug, Criteria: current, Hash: CriteriaHash(current),
		Authorized: verdict.Proceed, Machine: verdict.Machine, Reason: verdict.Reason,
		Added: verdict.Added, Removed: verdict.Removed,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	if !verdict.Proceed {
		return NewExitCodeError(1)
	}
	return nil
}

func runApproveCriteria(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	if approveCriteriaBy == "" {
		return fmt.Errorf("--by is required: an approval that cannot say what accepted the standard is the forgery this record exists to avoid")
	}

	current, err := CurrentCriteria(cfg, slug)
	if err != nil {
		return err
	}
	if len(current) == 0 {
		return fmt.Errorf("%s declares no criteria; there is no standard to approve", slug)
	}
	if err := RecordHumanApproval(cfg, slug, current,
		time.Now().UTC().Format(time.RFC3339), approveCriteriaBy, approveCriteriaID); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Approved %d criteria for %s (%s)\n", len(current), slug, CriteriaHash(current))
	return nil
}

// runIdentity names this invocation for the audit trail.
//
// Prefers a CI-supplied run identifier, because that is a fact about the run
// that something outside parlay can corroborate. Falls back to a hostname and
// process id, which is weak but honest — unlike $USER, it does not claim to
// name a person.
func runIdentity() string {
	for _, key := range []string{"GITHUB_RUN_ID", "CI_JOB_ID", "BUILD_ID", "PARLAY_RUN_ID"} {
		if v := os.Getenv(key); v != "" {
			return key + "=" + v
		}
	}
	host, _ := os.Hostname()
	return fmt.Sprintf("local:%s/%d", host, os.Getpid())
}
