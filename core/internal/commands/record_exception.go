// parlay-feature: parlay-tool/criterion-authority
// parlay-component: exception-authoring
//
// The way a coverage exception gets written.
//
// There was none. saveCoverageExceptions existed and every caller was a test,
// so R1 shipped a record the tool could read, validate, bind for freshness and
// block on — and that a person could only produce by hand-editing YAML against
// a shape documented nowhere. A decision artifact with no authoring path is one
// nobody makes deliberately.
//
// This is that path, and it is deliberately NOT legacy-only. R2 needs a
// migration for exemptions stranded in the retired coverage-review.yaml, but
// building only that would leave every NEW exception hand-edited. The migration
// is one mode of this command rather than a command of its own.
//
// Nothing here is silent. Every write records who decided and why, and the
// migration converts one exemption at a time into a fresh decision or a
// recorded deliberate deletion — never a copy. Whether an old judgment still
// holds is exactly what nobody but its author can say, and a bulk copy would
// assert it for all of them at once.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
)

var dropLegacyCmd = &cobra.Command{
	Use:   "drop-legacy-exemption <@feature>",
	Short: "Record that a stranded exemption no longer applies",
	Long: `Record a deliberate decision that a stranded exemption is not carried forward.

Dropping is a decision and is recorded as one. A judgment abandoned without a
trace is indistinguishable from one nobody noticed, which is the failure this
reconciliation exists to prevent — and until every stranded exemption is either
re-recorded or dropped, the boundary keeps saying so.`,
	Args: cobra.ExactArgs(1),
	RunE: runDropLegacyExemption,
}

var (
	dropLegacyRef             string
	dropLegacyText            string
	dropLegacyReason          string
	dropLegacyBy              string
	recordExceptionFromLegacy bool
	recordExceptionLegacyFP   string
	recordExceptionLegacyDup  int
	recordExceptionLegacyHash string
	dropLegacyHash            string
	dropLegacyFP              string
	dropLegacyDup             int
)

var (
	recordExceptionRef    string
	recordExceptionText   string
	recordExceptionKind   string
	recordExceptionReason string
	recordExceptionBy     string
	recordExceptionSuite  string
	recordExceptionCase   string
)

var recordExceptionCmd = &cobra.Command{
	Use:   "record-exception <@feature>",
	Short: "Record a coverage exception — a criterion deliberately left untested, or a weaker observation accepted",
	Long: `Record one coverage exception for a feature.

Two kinds are supported. ` + "`waived`" + ` says a criterion genuinely needs no
generated test; it excuses that criterion. ` + "`state-only`" + ` says one case
observes its criterion more weakly than the criterion states — it excuses
nothing, because the criterion is still discharged, and binds to the case it
accepts.

--reason and --by are required. An exception nobody can review later is not one,
and a record that cannot say what decided it is the forgery this artifact exists
to avoid.`,
	Args: cobra.ExactArgs(1),
	RunE: runRecordException,
}

var migrateExceptionsCmd = &cobra.Command{
	Use:   "migrate-coverage-exceptions <@feature>",
	Short: "List exemptions stranded in a retired coverage-review.yaml, one decision at a time",
	Long: `Report the exemptions stranded in a feature's retired coverage-review.yaml.

The tool no longer reads that file, so a judgment recorded there is doing
nothing while its criterion goes uncovered. This names each one with the
criterion it excused and whether that criterion still exists, so a person can
decide per exemption whether it still holds.

It writes nothing. Each surviving judgment is re-recorded with
` + "`record-exception`" + `, and each abandoned one is deleted deliberately —
because whether an old judgment still applies is the one thing nobody but its
author can say, and copying them in bulk would assert it for all of them at
once.`,
	Args: cobra.ExactArgs(1),
	RunE: runMigrateExceptions,
}

func init() {
	recordExceptionCmd.Flags().StringVar(&recordExceptionRef, "ref", "", "contract entry the exception is about (required)")
	recordExceptionCmd.Flags().StringVar(&recordExceptionText, "criterion", "", "the criterion's exact text; omit for an entry-wide exception, which is broader and warned")
	recordExceptionCmd.Flags().StringVar(&recordExceptionKind, "kind", string(ExceptionWaived), "waived | state-only")
	recordExceptionCmd.Flags().StringVar(&recordExceptionReason, "reason", "", "why (required) — an exception nobody can review later is not one")
	recordExceptionCmd.Flags().StringVar(&recordExceptionBy, "by", "", "what decided this, from the decision channel (required)")
	recordExceptionCmd.Flags().StringVar(&recordExceptionSuite, "suite", "", "state-only: the suite whose case observes weakly")
	recordExceptionCmd.Flags().StringVar(&recordExceptionCase, "case", "", "state-only: the case whose observation is accepted")
	recordExceptionCmd.Flags().StringVar(&recordExceptionLegacyFP, "legacy-fingerprint", "", "with --from-legacy: the exact entry being answered, from migrate-coverage-exceptions")
	recordExceptionCmd.Flags().StringVar(&recordExceptionLegacyHash, "legacy-file-hash", "", "with --from-legacy: the version of the retired review you were shown")
	recordExceptionCmd.Flags().IntVar(&recordExceptionLegacyDup, "legacy-duplicate", 0, "with --from-legacy: which copy, when entries are identical in every field")
	recordExceptionCmd.Flags().BoolVar(&recordExceptionFromLegacy, "from-legacy", false,
		"mark the matching stranded exemption in the retired coverage-review.yaml as answered by this decision")
	dropLegacyCmd.Flags().StringVar(&dropLegacyFP, "fingerprint", "", "the exact entry's fingerprint, from migrate-coverage-exceptions (required)")
	dropLegacyCmd.Flags().StringVar(&dropLegacyHash, "legacy-file-hash", "", "the version of the retired review you were shown, from migrate-coverage-exceptions (required)")
	dropLegacyCmd.Flags().IntVar(&dropLegacyDup, "duplicate", 0, "which copy, when entries are identical in every field")
	dropLegacyCmd.Flags().StringVar(&dropLegacyRef, "ref", "", "deprecated: identity comes from --fingerprint")
	dropLegacyCmd.Flags().StringVar(&dropLegacyText, "criterion", "", "deprecated: identity comes from --fingerprint")
	dropLegacyCmd.Flags().StringVar(&dropLegacyReason, "reason", "", "why it no longer applies (required)")
	dropLegacyCmd.Flags().StringVar(&dropLegacyBy, "by", "", "what decided this, from the decision channel (required)")
}

func runRecordException(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	for name, v := range map[string]string{"--ref": recordExceptionRef, "--reason": recordExceptionReason, "--by": recordExceptionBy} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	kind := ExceptionKind(recordExceptionKind)
	switch kind {
	case ExceptionWaived:
	case ExceptionStateOnly:
		if recordExceptionSuite == "" || recordExceptionCase == "" {
			return fmt.Errorf("--suite and --case are required for state-only: the decision accepts ONE case's weaker observation, and one naming no case would accept every weakening of that criterion including ones nobody saw")
		}
		if recordExceptionText == "" {
			return fmt.Errorf("--criterion is required for state-only: the decision is about a specific criterion being observed weakly, not about the entry")
		}
	default:
		return fmt.Errorf("--kind %q is not supported; waived and state-only are, hand-authored is reserved", recordExceptionKind)
	}

	// The exception must be about a criterion that exists. Recording one
	// against a ref the contract does not declare produces a ledger that reads
	// as working and excuses nothing.
	current, err := CurrentCriteria(cfg, slug)
	if err != nil {
		return fmt.Errorf("read the criteria this exception would be about: %w", err)
	}
	var entryBullets []AuthorizedCriterion
	for _, c := range current {
		if c.Ref == recordExceptionRef {
			entryBullets = append(entryBullets, c)
		}
	}
	if len(entryBullets) == 0 {
		return fmt.Errorf("%s declares no criteria on %s, so there is nothing there to excuse", slug, recordExceptionRef)
	}

	// A state-only decision must be bound to the case it is about. Writing
	// suite and case NAMES alone records an approval that keeps matching after
	// the body is replaced, so the reviewer ends up on record approving an
	// observation they never saw. Resolving here also refuses the three ways a
	// decision can be about nothing: a case that does not exist, one that is
	// not state-only, and one that discharges a different criterion.
	caseHash := ""
	if kind == ExceptionStateOnly {
		tcPath := filepath.Join(cfg.BuildPath(slug), "testcases.yaml")
		content, err := os.ReadFile(tcPath)
		if err != nil {
			return fmt.Errorf("read the cases this decision would be about: %w", err)
		}
		cases, err := resolveCases(content)
		if err != nil {
			return err
		}
		var match *resolvedCase
		for i := range cases {
			if cases[i].Suite == recordExceptionSuite && cases[i].Name == recordExceptionCase {
				match = &cases[i]
				break
			}
		}
		if match == nil {
			return fmt.Errorf("%s declares no case %q in suite %q — a decision naming a case that does not exist excuses nothing and reads in the ledger as though it did", tcPath, recordExceptionCase, recordExceptionSuite)
		}
		if match.Coverage != "state-only" {
			return fmt.Errorf("suite %q case %q has coverage %q, not state-only — there is no weakened observation here to accept", recordExceptionSuite, recordExceptionCase, match.Coverage)
		}
		if match.Ref != recordExceptionRef || agent.CanonicalCriterionText(match.Text) != agent.CanonicalCriterionText(recordExceptionText) {
			return fmt.Errorf("suite %q case %q discharges %q, not %q — approving one case's weakening says nothing about a criterion it does not observe", recordExceptionSuite, recordExceptionCase, match.Ref, recordExceptionRef)
		}
		caseHash = match.Fingerprint
	}

	ex := CoverageException{
		Ref: recordExceptionRef, Text: recordExceptionText, Kind: kind,
		Reason: recordExceptionReason, Suite: recordExceptionSuite, Case: recordExceptionCase,
		CaseHash: caseHash,
	}
	if recordExceptionText == "" {
		ex.EntryHash = entryBulletsHash(entryBullets)
	} else {
		wanted := agent.CanonicalCriterionText(recordExceptionText)
		found := false
		for _, c := range entryBullets {
			if agent.CanonicalCriterionText(c.Text) == wanted {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s declares no criterion %q — a text matching no bullet excuses nothing and looks like it does", recordExceptionRef, recordExceptionText)
		}
	}

	rec, err := loadCoverageExceptions(cfg, slug)
	if err != nil {
		return fmt.Errorf("read existing exceptions: %w — refusing to overwrite a ledger that could not be read", err)
	}
	if rec == nil {
		rec = &CoverageExceptions{Feature: slug}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if rec.GrantedAt == "" {
		rec.GrantedAt = now
	}
	rec.CriteriaHash = CriteriaHash(current)
	ex.At, ex.By = now, recordExceptionBy
	rec.Exceptions = append(rec.Exceptions, ex)

	if recordExceptionFromLegacy {
		entries, fileHash, lErr := loadLegacyEntries(cfg, slug)
		if lErr != nil {
			return lErr
		}
		if hErr := requireUnchangedLegacyFile(recordExceptionLegacyHash, fileHash); hErr != nil {
			return hErr
		}
		entry, fErr := findLegacyEntry(entries, recordExceptionLegacyFP, recordExceptionLegacyDup)
		if fErr != nil {
			return fErr
		}
		rec.LegacyFileHash = fileHash
		rec.ReconciledLegacy = append(rec.ReconciledLegacy, LegacyDisposition{
			Ref: entry.Ref, Text: entry.Text,
			Fingerprint: entry.Fingerprint, Duplicate: entry.Duplicate,
			Disposition: "recorded", Reason: recordExceptionReason,
			At: now, By: recordExceptionBy,
		})
	}

	if err := saveCoverageExceptions(cfg, slug, rec); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s exception for %s (%d total)\n", kind, recordExceptionRef, len(rec.Exceptions))
	return nil
}

type strandedExemption struct {
	Ref string `json:"ref"`
	// Text is empty for the entry-wide legacy shape, which is how every
	// exemption written before bullet identity has to be read.
	Text string `json:"criterion_text,omitempty"`
	// StillDeclared says whether the contract still carries what this excused.
	// An exemption for a criterion that no longer exists needs deleting, not
	// re-deciding.
	StillDeclared bool `json:"still_declared"`
	// Fingerprint and Duplicate are what a disposition must name to answer
	// exactly this entry. They are emitted here because this listing is where
	// a reviewer sees the entries, and an identity they have to derive
	// themselves is one they can derive differently.
	Fingerprint string `json:"fingerprint"`
	Duplicate   int    `json:"duplicate"`
	Reason      string `json:"recorded_reason"`
}

type migrateExceptionsOutput struct {
	Feature string `json:"feature"`
	Legacy  string `json:"legacy_file"`
	// LegacyFileHash is the version of the retired review these entries were
	// read from. A writer must pass it back, so a decision recorded after the
	// file moved is refused rather than silently answering a newer occurrence
	// than the one the reviewer saw.
	LegacyFileHash string `json:"legacy_file_hash"`
	// Status is the shared projection. The walkthrough reads its counts and
	// occurrences rather than deriving its own, so discovery and the tool that
	// performs the migration can never disagree about what is left.
	Status  *CoverageMigrationStatus `json:"status"`
	Pending []strandedExemption      `json:"stranded"`
	Note    string                   `json:"note"`
}

func runMigrateExceptions(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	legacyPath := filepath.Join(cfg.BuildPath(slug), "coverage-review.yaml")

	if _, statErr := os.Stat(legacyPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return fmt.Errorf("%s has no coverage-review.yaml; nothing is stranded", slug)
		}
		return fmt.Errorf("read %s: %w", legacyPath, statErr)
	}
	legacy, err := parser.ParseCoverageReview(legacyPath)
	if err != nil {
		return fmt.Errorf("parse %s: %w — repair it or delete it deliberately; it may hold judgments nobody can now recover", legacyPath, err)
	}
	if legacy == nil || len(legacy.Exemptions) == 0 {
		return fmt.Errorf("%s's coverage-review.yaml records no exemptions — only suite approvals, which proved nothing and are gone on purpose. Delete the file", slug)
	}

	current, err := CurrentCriteria(cfg, slug)
	if err != nil {
		return fmt.Errorf("read current criteria: %w", err)
	}
	// Bullet-level, not ref-level. An entry can still exist while the bullet
	// this exemption excused was reworded or removed, and reporting that as
	// "still declared" sends a person to re-record a judgment about something
	// the contract no longer says.
	declaredEntry := map[string]bool{}
	declaredBullet := map[string]bool{}
	for _, c := range current {
		declaredEntry[c.Ref] = true
		declaredBullet[legacyKey(c.Ref, c.Text)] = true
	}

	entries, legacyHash, lErr := loadLegacyEntries(cfg, slug)
	if lErr != nil {
		return lErr
	}
	status, sErr := CollectCoverageMigrationStatus(cfg, slug)
	if sErr != nil {
		return sErr
	}

	out := migrateExceptionsOutput{
		Feature: slug, Legacy: legacyPath, LegacyFileHash: legacyHash,
		Status: status,
		Note: "This command writes nothing. Re-record each judgment that still holds with `parlay internal record-exception --from-legacy --legacy-fingerprint <fingerprint> --legacy-file-hash <legacy_file_hash>`, " +
			"and drop the rest with `parlay internal drop-legacy-exemption --fingerprint <fingerprint> --legacy-file-hash <legacy_file_hash>`. The boundary keeps reporting these until every one is answered. " +
			"Whether an old judgment still applies is the one thing nobody but its author can say, " +
			"so copying them in bulk would assert it for all of them at once. Each fingerprint identifies ONE entry by its whole content, " +
			"so answering one never answers another that happens to share a ref and criterion. " +
			"Pass the FULL fingerprint from status.occurrences[].fingerprint; the short label is for a person to select by, not to type.",
	}
	for _, e := range entries {
		still := declaredEntry[e.Ref]
		if strings.TrimSpace(e.Text) != "" {
			still = declaredBullet[legacyKey(e.Ref, e.Text)]
		}
		out.Pending = append(out.Pending, strandedExemption{
			Ref: e.Ref, Text: e.Text,
			StillDeclared: still, Reason: e.Reason,
			Fingerprint: e.Fingerprint, Duplicate: e.Duplicate,
		})
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func runDropLegacyExemption(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	for name, v := range map[string]string{"--fingerprint": dropLegacyFP, "--reason": dropLegacyReason, "--by": dropLegacyBy} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}

	// The exemption must actually be stranded, and the disposition must name
	// the EXACT one. Recording against (ref, text) alone answered every entry
	// that shared them, so a second judgment on the same bullet — written for
	// a different reason by a different person — was marked reconciled by a
	// decision nobody made about it.
	entries, fileHash, lErr := loadLegacyEntries(cfg, slug)
	if lErr != nil {
		return lErr
	}
	if hErr := requireUnchangedLegacyFile(dropLegacyHash, fileHash); hErr != nil {
		return hErr
	}
	entry, fErr := findLegacyEntry(entries, dropLegacyFP, dropLegacyDup)
	if fErr != nil {
		return fErr
	}

	rec, err := loadCoverageExceptions(cfg, slug)
	if err != nil {
		return fmt.Errorf("read existing exceptions: %w — refusing to overwrite a ledger that could not be read", err)
	}
	if rec == nil {
		rec = &CoverageExceptions{Feature: slug, GrantedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	rec.LegacyFileHash = fileHash
	rec.ReconciledLegacy = append(rec.ReconciledLegacy, LegacyDisposition{
		Ref: entry.Ref, Text: entry.Text,
		Fingerprint: entry.Fingerprint, Duplicate: entry.Duplicate,
		Disposition: "dropped", Reason: dropLegacyReason,
		At: time.Now().UTC().Format(time.RFC3339), By: dropLegacyBy,
	})
	if err := saveCoverageExceptions(cfg, slug, rec); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Recorded that %s no longer applies\n", describeStranded(entry.Ref, entry.Text))
	return nil
}
