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

var gateStage string

func init() {
	gateCmd.Flags().StringVar(&gateStage, "stage", "",
		"Boundary to gate, aligned with the FeaturePhase ladder: build (designer->build), code (build->code), done (code->complete)")
	gateCmd.MarkFlagRequired("stage")
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
	slug := parser.FeatureSlug(args[0])
	out, err := computeGate(cfg, slug, gateStage)
	if err != nil {
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

	var blockers, warnings []gateBlocker
	switch stage {
	case gateStageBuild:
		blockers, warnings = gateBuild(cfg, slug, featurePath)
	case gateStageCode:
		blockers, warnings = gateCode(cfg, slug, featurePath)
	case gateStageDone:
		blockers, warnings = gateDone(cfg, slug)
	default:
		return out, fmt.Errorf("unknown gate stage %q — supported: %s, %s, %s",
			stage, gateStageBuild, gateStageCode, gateStageDone)
	}

	out.Blockers = dedupeGateFindings(blockers)
	out.Warnings = dedupeGateFindings(warnings)
	out.Passed = len(out.Blockers) == 0
	return out, nil
}

// gateBuild aggregates the designer->build boundary: readiness, the ledger's
// integrity and unapplied tail, and the ledger's own validation.
func gateBuild(cfg *config.Context, slug, featurePath string) (blockers, warnings []gateBlocker) {
	// check-readiness build-feature. This already carries 1c's
	// unapplied-amendments error, but the gate recomputes that verdict itself
	// with journal precision (2b) below — so strip it here to avoid both
	// double-counting it and inheriting readiness's coarse "any journal
	// suppresses" rule.
	for _, iss := range checkBuildFeatureReadiness(cfg, featurePath, slug) {
		if iss.Code == "unapplied-amendments" {
			continue
		}
		switch iss.Severity {
		case "error":
			blockers = append(blockers, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		case "warning":
			warnings = append(warnings, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		}
	}

	// check-drift's ledger findings. Integrity violations always block — a
	// mutated or deleted amendment, or an edited frozen founding doc, is never
	// sanctioned by an in-flight refine.
	lb, lw := gateLedgerState(cfg, slug, featurePath)
	blockers = append(blockers, lb...)
	warnings = append(warnings, lw...)

	return blockers, warnings
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

// gateCode aggregates the build->code boundary: buildfile validity, cross-
// feature composition coherence, and buildfile freshness against live sources.
func gateCode(cfg *config.Context, slug, featurePath string) (blockers, warnings []gateBlocker) {
	cb := computeCheckBuildfile(cfg, slug)
	for _, iss := range cb.Issues {
		switch iss.Severity {
		case "error":
			blockers = append(blockers, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		case "warning":
			warnings = append(warnings, gateBlocker{Code: iss.Code, Message: iss.Message, Fix: iss.Fix})
		}
	}

	// Composition is a whole-project coherence check; a failure here is a
	// cross-feature contradiction the boundary must not advance past.
	if comp, err := computeComposition(cfg); err == nil {
		for _, f := range comp.Findings {
			blockers = append(blockers, gateBlocker{Code: f.Code, Message: f.Message})
		}
		for _, n := range comp.Notes {
			warnings = append(warnings, gateBlocker{Code: n.Code, Message: n.Message})
		}
	}

	// Buildfile freshness — the source-signatures comparison generate-code step
	// 11.6 performs by prose, done mechanically here so the boundary catches a
	// stale buildfile before codegen does.
	blockers = append(blockers, checkBuildfileFreshness(cfg, slug, featurePath, &warnings)...)

	// Ledger state. A caller entering with --from code never crosses the build
	// boundary, so without this the only check for a recorded-but-unapplied
	// decision was on a path they skipped: codegen ran against a specification
	// its author had already superseded and reported success.
	lb, lw := gateLedgerState(cfg, slug, featurePath)
	blockers = append(blockers, lb...)
	warnings = append(warnings, lw...)

	return blockers, warnings
}

// gateDone aggregates the code->complete boundary: generated code matches its
// recorded hashes, and the coverage-review gate is satisfied.
func gateDone(cfg *config.Context, slug string) (blockers, warnings []gateBlocker) {
	verify, err := computeProjectVerifyOutput(cfg)
	if err == nil && verify != nil {
		if !verify.HasHashes {
			blockers = append(blockers, gateBlocker{
				Code:    "code-not-generated",
				Message: "no generated-code hashes recorded — the code phase has not produced a blessed prototype",
				Fix:     "run /parlay-generate-code",
			})
		}
		for _, f := range verify.Modified {
			blockers = append(blockers, gateBlocker{
				Code:    "generated-file-modified",
				Message: fmt.Sprintf("%s differs from the last recorded emission (possible hand-edit)", f.Path),
				Fix:     "re-run /parlay-generate-code, or reconcile the edit",
			})
		}
		for _, f := range verify.Adopted {
			blockers = append(blockers, gateBlocker{
				Code:    "generated-file-adopted",
				Message: fmt.Sprintf("%s was written outside codegen", f.Path),
			})
		}
		for _, f := range verify.Unknown {
			blockers = append(blockers, gateBlocker{
				Code:    "generated-file-unknown-provenance",
				Message: fmt.Sprintf("%s has undeclared provenance", f.Path),
			})
		}
		for _, f := range verify.Missing {
			blockers = append(blockers, gateBlocker{
				Code:    "generated-file-missing",
				Message: fmt.Sprintf("%s was recorded as generated but is gone from disk", f.Path),
				Fix:     "re-run /parlay-generate-code, or drop the component",
			})
		}
	}

	rg := computeReviewGate(cfg, slug)
	for _, iss := range rg.Issues {
		if iss.Severity == "error" {
			blockers = append(blockers, gateBlocker{Code: iss.Code, Message: iss.Message})
		}
	}

	// Ledger state here too. Marking a feature complete while a recorded
	// decision is unapplied asserts the strongest thing the ladder can say
	// about work that does not yet reflect a change its author already made.
	lb, lw := gateLedgerState(cfg, slug, cfg.FeaturePath(slug))
	blockers = append(blockers, lb...)
	warnings = append(warnings, lw...)

	return blockers, warnings
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
