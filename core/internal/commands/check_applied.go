// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: refine-preflight
//
// The cheap question refine asks before it reads anything else: has this ask
// already been done?
//
// The benchmark measured what its absence costs (F12): re-run agents found
// their ask already implemented and each improvised a different no-op path,
// every one of them having first paid the full skill + module + schema load
// to discover there was nothing to do. F13 is the same wound from the other
// side — an interrupted refine had no resume story, so a resumed session
// re-dispatched completed steps.
//
// This command answers, in one call and without loading any amendment
// bodies: what does drift say, which amendments exist (frontmatter only),
// what is the unapplied tail, and is a refine already in flight. Whether the
// ASK matches one of those amendments stays a judgment — but it becomes a
// judgment over a few hundred bytes of frontmatter instead of over a fully
// loaded refine context.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var checkAppliedCmd = &cobra.Command{
	Use:   "check-applied <@feature>",
	Short: "Report whether a refinement may already be applied: drift verdict, ledger index, and any in-flight run (JSON)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckApplied,
}

// ledgerIndexEntry is one amendment as the pre-flight reports it: the
// frontmatter identity and nothing else. Deliberately NOT the body — the
// whole point is to let the agent recognise "I have already done this"
// without ingesting the ledger. ~200 bytes per amendment.
type ledgerIndexEntry struct {
	Seq     int      `json:"seq"`
	Slug    string   `json:"slug"`
	Date    string   `json:"date,omitempty"`
	Trigger string   `json:"trigger,omitempty"`
	Affects []string `json:"affects,omitempty"`
	Applied bool     `json:"applied"`
	// Path is relative to the project root so the agent can open exactly
	// this one file when it needs the body, without a directory walk.
	Path string `json:"path"`
}

type checkAppliedOutput struct {
	Feature string `json:"feature"`

	// CleanState is the one-line verdict a skill branches on: no drift of
	// any kind, no unapplied tail, no interrupted run. When true, the
	// project is in a settled state and any matching amendment in the index
	// below is genuinely already applied — not half-applied, not pending.
	CleanState bool `json:"clean_state"`

	HasDrift             bool     `json:"has_drift"`
	LedgerIntegrity      []string `json:"ledger_integrity,omitempty"`
	UnappliedAmendments  []string `json:"unapplied_amendments,omitempty"`
	SharedSourcesChanged []string `json:"shared_sources_changed,omitempty"`
	LastAppliedAmendment int      `json:"last_applied_amendment"`

	// Amendments is the ledger index, oldest first.
	Amendments []ledgerIndexEntry `json:"amendments"`

	// InFlight describes an interrupted refine, when one is recorded. Nil
	// means no journal on disk — the normal state.
	InFlight *refineJournal `json:"in_flight,omitempty"`

	// HasBaseline is false for a feature that has never been built. Refine
	// has nothing to re-baseline there and the loop owns the feature.
	HasBaseline bool `json:"has_baseline"`
}

func runCheckApplied(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featureDir := cfg.FeaturePath(slug)

	out := checkAppliedOutput{Feature: slug, Amendments: []ledgerIndexEntry{}}

	if _, err := os.Stat(baselinePath(cfg, slug)); err == nil {
		out.HasBaseline = true
	}

	// Reuse detectDrift rather than re-deriving: the pre-flight must agree
	// with check-drift by construction, or a skill could exit "already
	// applied" on a state check-drift calls dirty.
	//
	// Inherited from that reuse, and correct here: detectDrift fails when a
	// file it must scan cannot be read, so this fails too. A pre-flight is
	// exactly where an unreadable file must not be answered — "already
	// applied" and "not applied" are both claims, and neither is safe to make
	// on a state nobody could look at.
	drift, err := detectDrift(cfg, slug, featureDir)
	if err != nil {
		return err
	}
	out.HasDrift = drift.HasDrift
	out.LedgerIntegrity = drift.LedgerIntegrity
	out.UnappliedAmendments = drift.UnappliedAmendments
	out.SharedSourcesChanged = drift.SharedSourcesChanged

	if blData, readErr := os.ReadFile(baselinePath(cfg, slug)); readErr == nil {
		var baseline Baseline
		if yaml.Unmarshal(blData, &baseline) == nil {
			out.LastAppliedAmendment = baseline.LastAppliedAmendment
		}
	}

	amendments, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		return fmt.Errorf("read amendments: %w", err)
	}
	for _, a := range amendments {
		rel, relErr := filepath.Rel(cfg.Root.Path, a.Path)
		if relErr != nil {
			rel = a.Path
		}
		out.Amendments = append(out.Amendments, ledgerIndexEntry{
			Seq:     a.Seq,
			Slug:    a.FileSlug,
			Date:    a.Date,
			Trigger: a.Trigger,
			Affects: a.Affects,
			Applied: a.Seq <= out.LastAppliedAmendment,
			Path:    filepath.ToSlash(rel),
		})
	}

	journal, err := loadRefineJournal(cfg, slug)
	if err != nil {
		return err
	}
	out.InFlight = journal

	out.CleanState = !out.HasDrift && journal == nil

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

// ---------------------------------------------------------------------
// The refine journal.
// ---------------------------------------------------------------------

// refineJournalFile is the per-feature journal name. It lives in the
// tool-internals zone beside .baseline.yaml — same ownership, same
// never-user-facing rule, same precedent as the .emitted manifest.
const refineJournalFile = ".refine-journal.yaml"

// refineJournal records how far an in-flight refinement got.
//
// It exists because a refine that dies mid-flight leaves the project in a
// state no probe can classify: an amendment file may be on disk with its
// splice unapplied, or the splice applied with nothing regenerated. The
// baseline cannot answer this — it records the last GREEN state, and the
// whole point is that this run never got there.
//
// Steps are recorded as they complete, so the resume point is "the first
// step not listed". The amendment sequence is recorded with the first step
// precisely so a resumed run amends the file it already wrote instead of
// minting a duplicate 002 for the same ask — the double-write cousin of
// L16's double-apply.
type refineJournal struct {
	// Feature is stored so a journal read out of context is self-describing.
	Feature string `yaml:"feature" json:"feature"`
	// Ask is the refinement prose this run is executing, so a resumed run
	// can confirm it is finishing the same job rather than a new one.
	Ask string `yaml:"ask,omitempty" json:"ask,omitempty"`
	// Amendment is the sequence number this run wrote (0 before step 3.5).
	Amendment int `yaml:"amendment,omitempty" json:"amendment,omitempty"`
	// StartedAt is an RFC3339 stamp, for the report only.
	StartedAt string `yaml:"started-at,omitempty" json:"started_at,omitempty"`
	// Completed lists the completed step names, in order. The vocabulary is
	// closed — see refineJournalSteps.
	Completed []string `yaml:"completed" json:"completed"`

	// ScopeBefore is the contract entries attributed to each lineage this
	// run's amendment changes, captured BEFORE the splice mutates anything.
	//
	// Without it there is no evidence of the prior subject population at all.
	// Scope derived after the splice cannot see a removed entry, so a
	// disposition naming any plausible absent ref would pass — the record
	// would be evidence that somebody claimed a consequence, never that the
	// promise ever justified the thing they claimed it about.
	ScopeBefore []lineageScope `yaml:"scope-before,omitempty" json:"scope_before,omitempty"`
	// ScopeBeforeAmendment, ScopeBeforeFile and ScopeBeforeHash identify the
	// EXACT record the inventory is evidence about.
	//
	// A sequence alone is not identity. Amendments are append-only, but an
	// append-only violation or an outright replacement of NNN-slug.md keeps
	// the sequence while changing the lineages and the meaning — and the
	// ceremony would then treat the old capture as evidence for the new bytes.
	// The amendment hash in the later approval payload does not repair the
	// provenance of an earlier capture; only the capture carrying its own
	// binding does.
	ScopeBeforeAmendment int      `yaml:"scope-before-amendment,omitempty" json:"scope_before_amendment,omitempty"`
	ScopeBeforeFile      string   `yaml:"scope-before-file,omitempty" json:"scope_before_file,omitempty"`
	ScopeBeforeHash      string   `yaml:"scope-before-hash,omitempty" json:"scope_before_hash,omitempty"`
	ScopeBeforeLineages  []string `yaml:"scope-before-lineages,omitempty" json:"scope_before_lineages,omitempty"`
	// ScopeBeforeDigest is a canonical hash of the captured inventory, so a
	// partially written or altered capture is detectable rather than trusted.
	ScopeBeforeDigest string `yaml:"scope-before-digest,omitempty" json:"scope_before_digest,omitempty"`
}

// refineJournalSteps is the closed vocabulary of journal step names, in
// pipeline order. A closed set is what makes "resume at the first step not
// listed" a decidable question rather than a string-matching guess.
var refineJournalSteps = []string{
	"amendment-written",
	"splice-applied",
	"rebuilt",
	"emitted",
	"tested",
	"re-baselined",
}

// NextRefineStep returns the first step this journal has not completed, and
// "" when every step is done (a journal that should have been cleared).
func (j *refineJournal) NextRefineStep() string {
	done := map[string]bool{}
	for _, s := range j.Completed {
		done[s] = true
	}
	for _, s := range refineJournalSteps {
		if !done[s] {
			return s
		}
	}
	return ""
}

func refineJournalPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), refineJournalFile)
}

// loadRefineJournal reads a feature's journal. A missing file is (nil, nil)
// — the normal state, not an error.
func loadRefineJournal(cfg *config.Context, slug string) (*refineJournal, error) {
	data, err := os.ReadFile(refineJournalPath(cfg, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read refine journal: %w", err)
	}
	var j refineJournal
	if err := yaml.Unmarshal(data, &j); err != nil {
		// A corrupt journal is reported, never silently discarded: it is
		// evidence that a run died, and dropping it would strand the
		// half-finished state it describes.
		return nil, fmt.Errorf("invalid refine journal %s: %w", refineJournalPath(cfg, slug), err)
	}
	return &j, nil
}
