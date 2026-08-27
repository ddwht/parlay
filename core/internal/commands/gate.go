// parlay-feature: parlay-tool/phase-gates
// parlay-component: gate-aggregator
//
// `parlay internal gate @{feature} --stage <boundary>` — the phase-transition
// gate. It answers one question at a phase-group boundary: may this feature
// advance to the next stage? "Agent proposes, CLI disposes" — the agent drives
// the pipeline, this recomputes whether a boundary is actually clear.
//
// Purity contract (the same one ComputeFeaturePhase documents, and for the same
// reason): the gate is a PURE RECOMPUTATION. It writes nothing, stamps nothing,
// and persists no "gate passed" marker. A stored pass-stamp goes stale the
// moment a spec file changes and then becomes the stale-state problem the whole
// plan exists to remove. Passage is re-derived from disk at every boundary.
//
// The gate does not reimplement any checker — it AGGREGATES the existing ones
// in-process (direct calls to the compute* cores, never a subprocess) and
// merges their findings into one verdict with a stable ordering.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var gateCmd = &cobra.Command{
	Use:   "gate <@feature>",
	Short: "Aggregate a phase-boundary's checks into one advance/block verdict (JSON)",
	Args:  cobra.ExactArgs(1),
	RunE:  runInternalGate,
}

var (
	gateStage             string
	gateAuthorizeCriteria string
)

func init() {
	gateCmd.Flags().StringVar(&gateStage, "stage", "",
		"Boundary to gate, aligned with the FeaturePhase ladder: build (designer->build), code (build->code), done (code->complete)")
	gateCmd.MarkFlagRequired("stage")
	gateCmd.Flags().StringVar(&gateAuthorizeCriteria, "authorize-criteria", "",
		"set to \"machine\" to advance this boundary without human approval of the criteria. Requires the project to "+
			"have opted in with parlay.criterion-authority.allow-machine; records that the separation between authoring "+
			"a standard and grading against it was WAIVED for this run, not satisfied")
}

// gateBlocker is one finding the gate surfaces. A blocker fails the gate; a
// warning is reported but does not. Every finding names its own fix, because
// the driver decides what to do about it and needs the fix to tell the user.
type gateBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

type gateOutput struct {
	// PendingWaiver is a machine authorization this boundary would exercise,
	// set only when the whole boundary passed. Persisting it is the advancing
	// COMMAND's job: computeGate stays a pure evaluation, so a caller that
	// merely asks what a boundary would say does not leave a record claiming a
	// run happened.
	PendingWaiver *pendingMachineRun `json:"-"`

	Feature  string        `json:"feature"`
	Stage    string        `json:"stage"`
	Passed   bool          `json:"passed"`
	Blockers []gateBlocker `json:"blockers"`
	Warnings []gateBlocker `json:"warnings"`
}

// The three stage tokens, aligned with the FeaturePhase ladder rather than
// check-readiness's legacy create-surface/build-feature naming. The gate maps
// to the readiness stage internally (see gateBuild).
const (
	gateStageBuild = "build"
	gateStageCode  = "code"
	gateStageDone  = "done"
)

func runInternalGate(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if err := validateAuthorizeCriteriaFlag(gateAuthorizeCriteria); err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	out, err := computeGate(cfg, slug, gateStage)
	if err != nil {
		return err
	}
	if err := commitPendingWaiver(cfg, slug, gateStage, out); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if !out.Passed {
		return NewExitCodeError(1)
	}
	return nil
}

// machineRunRecordedFor reports whether the audit trail already holds a machine
// authorization written by THIS execution against exactly this standard.
func machineRunRecordedFor(cfg *config.Context, slug string, criteria []AuthorizedCriterion, runID string) (bool, error) {
	rec, err := loadCriteriaAuthority(cfg, slug)
	if err != nil {
		return false, err
	}
	if rec == nil {
		return false, nil
	}
	// Both halves are required. The hash alone says an identical standard was
	// waived at some point by somebody; the run id is what says it was waived
	// by THIS execution, which is the only thing that makes today's crossing
	// already audited.
	want := CriteriaHash(criteria)
	for _, r := range rec.MachineRuns {
		if r.CriteriaHash == want && r.RunID != "" && r.RunID == runID {
			return true, nil
		}
	}
	return false, nil
}

// computeGate is the pure core: given a resolved context, a feature slug and a
// stage token, it aggregates the stage's checkers and returns the verdict. No
// stdout, no exit — both the internal command and `parlay gate --all` call it.
func computeGate(cfg *config.Context, slug, stage string) (gateOutput, error) {
	out := gateOutput{
		Feature:  slug,
		Stage:    stage,
		Blockers: []gateBlocker{},
		Warnings: []gateBlocker{},
	}

	featurePath := cfg.FeaturePath(slug)

	// A hand-authored unit has no buildfile and nothing generates its code, so
	// none of the three boundaries applies to it. Gating one would block a
	// correctly-declared unit forever — the same permanent-false-failure trap
	// check-readiness and check-buildfile already sidestep.
	if config.IsAuthoredUnit(featurePath) {
		out.Passed = true
		return out, nil
	}

	switch stage {
	case gateStageBuild, gateStageCode, gateStageDone:
	default:
		return out, fmt.Errorf("unknown gate stage %q — supported: %s, %s, %s",
			stage, gateStageBuild, gateStageCode, gateStageDone)
	}

	// Assembled from the registry rather than by a per-stage function calling
	// checkers in sequence. A checker not registered as a claim is not
	// reachable from a boundary at all, which is what makes the completeness
	// conformance non-circular: an unwitnessed path cannot hide by simply not
	// being registered, because not being registered means not running.
	blockers, warnings, pending := runStageClaims(cfg, slug, featurePath, stage)

	out.Blockers = dedupeGateFindings(blockers)
	out.Warnings = dedupeGateFindings(warnings)
	out.Passed = len(out.Blockers) == 0

	// A waiver is exercised only by a boundary that actually advances. Held
	// until every check has spoken, because one subcheck permitting it says
	// nothing about the aggregate — and an audit trail recording that a refused
	// run "advanced" lies about the single fact it exists to hold.
	if out.Passed && pending != nil {
		out.PendingWaiver = pending
		out.Warnings = append(out.Warnings, gateBlocker{
			Code:    "criteria-authority-machine",
			Message: fmt.Sprintf("%s advanced without human approval of its criteria: %s", slug, pending.reason),
		})
	}
	return out, nil
}

// gateLedgerState is the ledger half of every advancing boundary: frozen-document
// integrity, the unapplied tail, and the ledger's own validation.
//
// Shared rather than inlined in gateBuild, because it was only in gateBuild.
// gateCode and gateDone aggregated none of it, so entering the pipeline with
// --from code walked past the only boundary that asks whether a recorded
// decision has actually been applied — and generated code from a specification
// its author had already superseded, reporting success. That is the failure
// intent supersession's applied-tail rule exists to prevent, so leaving it in
// one boundary would have made the rule true only on the path nobody was
// worried about.
//
// One helper, one code, one source. The unapplied-tail finding keeps its
// journal-aware downgrade: a refinement that has spliced but not yet
// re-baselined is mid-apply, not stale, and blocking it would stop the very
// workflow that resolves the finding.
func gateLedgerState(cfg *config.Context, slug, featurePath string) (blockers, warnings []gateBlocker) {
	if drift, err := detectDrift(cfg, slug, featurePath); err == nil && drift != nil {
		for _, f := range drift.LedgerIntegrity {
			blockers = append(blockers, gateBlocker{
				Code:    "ledger_integrity",
				Message: f,
				Fix:     "restore the frozen text and record the change as an amendment; for a pre-v0.4 edit run parlay migrate-ledger",
			})
		}
		if len(drift.UnappliedAmendments) > 0 {
			finding := gateBlocker{
				Code: "unapplied-amendments",
				Message: fmt.Sprintf("%d amendment(s) recorded but not applied to the contract artifacts: %v",
					len(drift.UnappliedAmendments), drift.UnappliedAmendments),
				Fix: "run /parlay-refine to apply the ledger tail",
			}
			// Name a pending supersession specifically. An unapplied amendment
			// that merely edits a contract entry leaves the spec incomplete; one
			// that retires a promise leaves the feature still making a promise
			// its author has withdrawn, and the reader should not have to open
			// the ledger to tell those apart.
			if res, resErr := resolveActiveIntents(cfg, slug); resErr == nil && res.HasPending() {
				finding.Message += fmt.Sprintf(" — including a pending intent supersession (%s), so this feature still promises what that decision withdraws", res.PendingSummary())
			}
			if refineSanctionsUnappliedTail(cfg, slug, featurePath) {
				finding.Message += " — sanctioned by an in-flight refinement (splice applied, re-baseline pending)"
				warnings = append(warnings, finding)
			} else {
				blockers = append(blockers, finding)
			}
		}
	}

	// check-amendments, only when a ledger exists.
	if amendments, err := parser.LoadFeatureAmendments(featurePath); err == nil && len(amendments) > 0 {
		ca := computeCheckAmendments(cfg, slug)
		for _, iss := range ca.Issues {
			switch iss.Severity {
			case "error":
				blockers = append(blockers, gateBlocker{Code: iss.Code, Message: iss.Message})
			case "warning":
				warnings = append(warnings, gateBlocker{Code: iss.Code, Message: iss.Message})
			}
		}
	}

	return blockers, warnings
}

// gateTestcasesReadiness is the mechanical half of the bargain that replaces
// the blanket human gate: a person approves the standard, and whether the tests
// actually discharge it is checked here rather than by anyone clicking through
// suite names.
//
// It exists because the walkers were graduated to error and nothing ran them in
// build mode — validate --type testcases hardcodes authoring, and no boundary
// called them at all — so the middle was advisory on every path that mattered.
func gateTestcasesReadiness(cfg *config.Context, slug string) (blockers, warnings []gateBlocker) {
	r := CheckTestcasesReadiness(cfg, slug)
	for _, b := range r.Blockers {
		blockers = append(blockers, gateBlocker{
			Code: "testcases-not-ready", Message: b,
			Fix: "rebuild the testcases, or record an exception for a criterion that genuinely needs no test",
		})
	}
	for _, w := range r.Warnings {
		warnings = append(warnings, gateBlocker{Code: "testcases-readiness-warning", Message: w})
	}
	return blockers, warnings
}

// gateCoverageExceptions surfaces a stale or broken exception ledger.
//
// Reached from the same boundaries as the other ledger checks, because the
// evaluation existed and nothing carried its findings anywhere: validate copied
// the excused set and dropped every blocker, so the freshness rule was written,
// tested at the leaf, and unreachable in production.
func gateCoverageExceptions(cfg *config.Context, slug string) (blockers, warnings []gateBlocker) {
	v := CheckCoverageExceptions(cfg, slug)
	for _, b := range v.Blockers {
		blockers = append(blockers, gateBlocker{
			Code: "coverage-exception-invalid", Message: b,
			Fix: "re-review the exception, or remove it",
		})
	}
	for _, w := range v.Warnings {
		warnings = append(warnings, gateBlocker{Code: "coverage-exception-broad", Message: w})
	}
	return blockers, warnings
}

// gateCriteriaAuthority refuses codegen against a standard nobody approved.
//
// Shared and called from every independently invocable boundary that can reach
// codegen, for the same reason gateLedgerState is: computeGate evaluates ONE
// stage, so --from code never crosses the build boundary and a check living
// only there protects the path nobody was worried about.
//
// The machine flag is deliberately absent from this signature. A gate reports
// what is true of the feature; exercising a waiver is something an invocation
// does, and the CLI passes it where the run is actually authorized. Reading it
// here would let a gate silently answer a governance question on behalf of a
// caller that never asked.
// pendingMachineRun is a waiver this boundary would exercise IF it advances.
type pendingMachineRun struct {
	criteria []AuthorizedCriterion
	reason   string
}

func gateCriteriaAuthority(cfg *config.Context, slug string, machine bool) (blockers, warnings []gateBlocker, pending *pendingMachineRun) {
	verdict, current, err := CheckCriteriaAuthority(cfg, slug, machine)
	if err != nil {
		return []gateBlocker{{
			Code:    "criteria-authority-unreadable",
			Message: fmt.Sprintf("cannot establish which standard %s is graded against: %v", slug, err),
			Fix:     "repair the contract artifacts, then re-run",
		}}, nil, nil
	}
	if verdict.Proceed {
		if verdict.Machine {
			// PENDING, not written. Evaluation stays pure: this subcheck
			// permitting the waiver says nothing about whether the boundary
			// advances, and a stale buildfile or a broken testcase later in the
			// aggregation can still block it. Writing here produced an audit
			// trail asserting that a run which was refused had "advanced" — a
			// record that lies about the one thing it exists to record.
			pending = &pendingMachineRun{criteria: current, reason: verdict.Reason}
		}
		return nil, warnings, pending
	}

	msg := verdict.Reason
	if len(verdict.Added) > 0 || len(verdict.Removed) > 0 {
		// Show WHAT changed. A refusal that only says the standard moved
		// leaves the reader to diff it themselves, which is the work the gate
		// just did.
		for _, c := range verdict.Removed {
			msg += fmt.Sprintf("\n    - no longer: %s — %q", c.Ref, c.Text)
		}
		for _, c := range verdict.Added {
			msg += fmt.Sprintf("\n    + now: %s — %q", c.Ref, c.Text)
		}
	}
	return []gateBlocker{{
		Code:    "criteria-authority-missing",
		Message: fmt.Sprintf("%s (%d criteria): %s", slug, len(current), msg),
		Fix:     "approve the criteria interactively, or authorize this run explicitly if the project permits it",
	}}, nil, nil
}

// refineSanctionsUnappliedTail implements 2b: an unapplied ledger tail is
// sanctioned in-flight work — and so downgrades from blocker to warning — only
// when a refine journal is present, has reached splice-applied or later, and
// records the amendment that the dirty tail is waiting on. A journal at an
// earlier step (the amendment is written but the splice is not yet in the
// contract), or a stale journal naming a different amendment than the dirty
// tail, does NOT downgrade — the tail is genuinely unapplied in both cases.
func refineSanctionsUnappliedTail(cfg *config.Context, slug, featurePath string) bool {
	journal, err := loadRefineJournal(cfg, slug)
	if err != nil || journal == nil {
		return false
	}
	if !refineJournalReached(journal, "splice-applied") {
		return false
	}
	if journal.Amendment <= 0 {
		return false
	}
	// The journal's amendment must be part of the unapplied tail it claims to
	// be applying. Recompute the tail's sequence set from the ledger and the
	// baseline rather than trusting the drift filenames.
	lastApplied := 0
	if blData, readErr := os.ReadFile(baselinePath(cfg, slug)); readErr == nil {
		var baseline Baseline
		if yaml.Unmarshal(blData, &baseline) == nil {
			lastApplied = baseline.LastAppliedAmendment
		}
	}
	amendments, err := parser.LoadFeatureAmendments(featurePath)
	if err != nil {
		return false
	}
	for _, a := range amendments {
		if a.Seq > lastApplied && a.Seq == journal.Amendment {
			return true
		}
	}
	return false
}

// refineJournalReached reports whether a journal has completed the named step.
func refineJournalReached(j *refineJournal, step string) bool {
	for _, s := range j.Completed {
		if s == step {
			return true
		}
	}
	return false
}

// buildfileSignatures is the shape needed to read a buildfile's recorded
// source-signatures block without decoding the whole document.
type buildfileSignatures struct {
	SourceSignatures map[string]string `yaml:"source-signatures"`
}

// checkBuildfileFreshness compares the buildfile's recorded source-signatures
// against freshly-computed content hashes of the same source artifacts. A
// mismatch, or an absent block, is a stale-buildfile blocker — the same verdict
// generate-code step 11.6 reaches by prose. When freshness cannot be computed
// (no adapter to hash, unreadable source), it degrades to a warning rather than
// a blocker: an undeterminable freshness must not silently pass, but neither
// should it hard-block on a condition the gate could not actually evaluate.
func checkBuildfileFreshness(cfg *config.Context, slug, featurePath string, warnings *[]gateBlocker) []gateBlocker {
	bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	data, err := os.ReadFile(bfPath)
	if err != nil {
		// A missing buildfile is check-buildfile's finding, not freshness'.
		return nil
	}
	var recorded buildfileSignatures
	if err := yaml.Unmarshal(data, &recorded); err != nil {
		*warnings = append(*warnings, gateBlocker{
			Code:    "buildfile-unreadable",
			Message: fmt.Sprintf("cannot parse %s to check freshness: %v", bfPath, err),
		})
		return nil
	}
	if len(recorded.SourceSignatures) == 0 {
		return []gateBlocker{{
			Code:    "stale-buildfile",
			Message: "buildfile has no source-signatures block — its freshness against the current sources cannot be established",
			Fix:     fmt.Sprintf("run parlay internal scaffold-signatures @%s, then re-run the build", slug),
		}}
	}

	adapterPath := presentationAdapterFile(cfg)
	current, err := computeSourceSignatures(featurePath, cfg.RepoRoot(), adapterPath, authoredUnitHashes(cfg))
	if err != nil {
		*warnings = append(*warnings, gateBlocker{
			Code:    "freshness-uncomputable",
			Message: fmt.Sprintf("could not recompute source signatures for %s: %v", slug, err),
		})
		return nil
	}

	// Any recorded field whose current hash differs, or any current field the
	// buildfile did not record, is drift. Both directions matter: a changed
	// artifact and a newly-added artifact each make the buildfile stale.
	var stale []string
	for field, rec := range recorded.SourceSignatures {
		if cur, ok := current[field]; !ok || cur != rec {
			stale = append(stale, field)
		}
	}
	for field := range current {
		if _, ok := recorded.SourceSignatures[field]; !ok {
			stale = append(stale, field)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	sort.Strings(stale)
	return []gateBlocker{{
		Code:    "stale-buildfile",
		Message: fmt.Sprintf("buildfile reflects stale sources for: %v", uniqueStrings(stale)),
		Fix:     fmt.Sprintf("run /parlay-build-feature @%s to refresh the buildfile, then re-run codegen", slug),
	}}
}

// dedupeGateFindings collapses identical (code, message) findings that two
// aggregated checkers may both report, and sorts by code then message so two
// runs over the same disk state produce byte-identical output.
func dedupeGateFindings(in []gateBlocker) []gateBlocker {
	seen := map[string]bool{}
	out := make([]gateBlocker, 0, len(in))
	for _, f := range in {
		key := f.Code + "\x00" + f.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// machineAuthorized reports whether THIS invocation asked to advance without
// human approval of the criteria.
//
// Read from the advancing command rather than passed down from a reporting one,
// because the question is about a run, not about a feature.
func machineAuthorized() bool {
	return gateAuthorizeCriteria == machineAuthorizationMode
}

// validateAuthorizeCriteriaFlag refuses a value that is neither empty nor the
// one mode.
//
// A typo silently meaning "no waiver" is the worst of both: the run stops for a
// reason the author believes they addressed, and nothing says the flag was
// ignored.
func validateAuthorizeCriteriaFlag(v string) error {
	if v == "" || v == machineAuthorizationMode {
		return nil
	}
	return fmt.Errorf("--authorize-criteria=%q is not a mode; the only value is %q", v, machineAuthorizationMode)
}

// commitPendingWaiver persists a machine authorization AFTER the boundary
// passed, and fails the command if it cannot.
//
// A normal loop crosses code and then done, and logging both would record one
// pipeline as two runs that proceeded without human approval. Code is where
// unapproved criteria authorize generation, so that is the crossing worth
// holding, and done inherits it.
//
// But "done inherits it" was written as "done records nothing", which is only
// the same thing when code actually ran. A directly invoked
// `gate done --authorize-criteria=machine` consumed the waiver, passed, and
// left no audit at all — the one shape where the inheritance assumption is
// false is exactly the shape where the record matters most.
//
// Done now inherits only from a machine run belonging to THIS execution. The
// first attempt at this matched on criteria hash alone, which is wrong in a way
// worth recording: a hash proves what was waived, not which run waived it. An
// unchanged standard machine-run on Monday would have made a direct done
// crossing on Friday inherit from it and write no Friday audit — precisely the
// "an audit event is not standing authority" invariant that
// APastMachineRunDoesNotAuthorizeALaterOne exists to hold.
//
// Execution identity comes from runIdentity(). In CI it is the job or run id,
// which is genuinely shared across the separate `gate code` and `gate done`
// invocations of one pipeline, so done inherits and the pipeline logs once.
// Locally it is host and pid, which differ per invocation — so each boundary
// records its own. That is the honest outcome: with no carrier proving the two
// crossings belong to one pipeline, claiming they do would be a guess. A loop
// that wants the single-event shape locally sets PARLAY_RUN_ID for the
// duration of the pipeline, which runIdentity() already reads.
//
// An unrecordable waiver fails the command rather than passing quietly: a
// waiver nobody can find later is indistinguishable from one that never
// happened, which defeats the only reason to permit it.
func commitPendingWaiver(cfg *config.Context, slug, stage string, out gateOutput) error {
	if out.PendingWaiver == nil {
		return nil
	}
	if stage != gateStageCode && stage != gateStageDone {
		return nil
	}
	if stage == gateStageDone {
		already, err := machineRunRecordedFor(cfg, slug, out.PendingWaiver.criteria, runIdentity())
		if err != nil {
			return fmt.Errorf("this run advanced without human approval of its criteria and the existing audit trail could not be read to see whether that was already recorded: %w", err)
		}
		if already {
			return nil
		}
	}
	if err := RecordMachineRun(cfg, slug, out.PendingWaiver.criteria,
		time.Now().UTC().Format(time.RFC3339),
		"parlay.criterion-authority.allow-machine", runIdentity(),
		fmt.Sprintf("--authorize-criteria=machine at the %s boundary", stage),
	); err != nil {
		return fmt.Errorf("this run advanced without human approval of its criteria but the waiver could not be recorded: %w", err)
	}
	return nil
}
