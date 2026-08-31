package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The evolve ceremony: applying a transition that keeps a lineage alive.
//
// Deliberately narrow. Only the PRESERVATION form is supported — an extend, or
// a revise whose scope declaration lists no exception — because a general
// revise can narrow support while the lineage stays alive, and until the scope
// accounting exists there is nothing to collect the consequences of a declared
// loss. Approving a loss whose consequences nobody accounted for would be the
// bypass this whole line of work exists to close, wearing a new verb.
//
// Not pretending preservation is machine-provable. It records a human assertion
// against an exact inventory, and says so.

func runEvolveTransition(cmd *cobra.Command, cfg *config.Context, slug, featDir string, record parser.Amendment, pending []parser.Amendment, prior appliedAuthority) error {
	// All-or-nothing. One unsupported transition refuses the whole record;
	// a partial application would leave a lineage evolved and its sibling not,
	// which no reader could classify.
	var lineages []string
	for _, tr := range record.IntentTransitions() {
		switch tr.Mode {
		case parser.IntentExtend, parser.IntentRevise:
			lineages = append(lineages, tr.Intent)
		default:
			return fmt.Errorf("%s changes lineage %q with mode %q, which has no applier yet: a "+
				"narrowing or a retirement can take support away from contract entries, and the "+
				"accounting that collects those consequences is not built. The whole record is "+
				"refused rather than partly applied",
				amendmentIdentity(record), tr.Intent, tr.Mode)
		}
	}
	if len(lineages) == 0 {
		return fmt.Errorf("%s declares no applicable transition", amendmentIdentity(record))
	}

	// The scope declaration is the author's, and it must exist: inferring it
	// would mean this ceremony manufacturing a preservation claim the
	// amendment never made.
	si := record.ScopeImpact
	if si == nil {
		return fmt.Errorf("%s changes a founding promise but declares no scope_impact — the "+
			"approval is a claim about the contract entries attributed to that promise, and "+
			"this record makes no such claim. Add scope_impact with version: 1 and "+
			"preserves_unlisted: true", amendmentIdentity(record))
	}
	if problems := record.ValidateScopeImpact(); len(problems) > 0 {
		return fmt.Errorf("%s: the scope declaration is not usable:\n  - %s",
			amendmentIdentity(record), joinLines(problems))
	}
	if len(si.Exceptions) > 0 {
		return fmt.Errorf("%s declares %d scope exception(s), which means it takes support away "+
			"from an entry. There is no applier for that yet: the accounting that collects the "+
			"consequences of a declared loss is not built, and approving one without it would "+
			"record a decision whose downstream effects nobody gathered",
			amendmentIdentity(record), len(si.Exceptions))
	}

	// The ledger's own rules first and in full, before anything is shown.
	ca := computeCheckAmendments(cfg, slug)
	var blocking []string
	for _, iss := range ca.Issues {
		if iss.Severity == "error" {
			blocking = append(blocking, fmt.Sprintf("[%s] %s", iss.Code, iss.Message))
		}
	}
	if len(blocking) > 0 {
		sort.Strings(blocking)
		return fmt.Errorf("%s: the ledger has %d unresolved error(s), so this transition is not "+
			"sound to apply and no promise delta will be shown:\n  - %s",
			slug, len(blocking), joinLines(blocking))
	}

	// Proof 1 — the splice happened and was tested. Required even when the
	// record declares no contract change: the journal is what ties this
	// approval to a completed run.
	if reasons := proveTailJournal(cfg, slug, pending); len(reasons) > 0 {
		return fmt.Errorf("the work behind %s is not proven:\n  - %s",
			amendmentIdentity(record), joinLines(reasons))
	}
	spliceProof := fmt.Sprintf("refine-journal:%s:%d:tested", slug, record.Seq)

	subjects, err := buildEvolutionSubjects(cfg, slug, featDir, record, lineages, si)
	if err != nil {
		return err
	}

	affects := append([]string(nil), record.Affects...)
	sort.Strings(affects)
	payload, err := buildTransitionPayload(slug, record, nil, affects, prior, spliceProof)
	if err != nil {
		return err
	}
	payload.Mode = transitionModeEvolve
	payload.Evolution = subjects

	digest, err := transitionDigest(payload)
	if err != nil {
		return err
	}

	out := applyAmendmentPreflight{
		Feature: slug, Amendment: amendmentIdentity(record),
		Mode: transitionModeEvolve, Affects: affects, Digest: digest,
	}
	if applyAmendmentConfirm == "" {
		return emitEvolvePreflight(cmd, out, subjects)
	}
	if applyAmendmentConfirm != digest {
		return fmt.Errorf("the confirmation does not match what is on disk now. It was given for "+
			"%s and this transition is %s — the amendment, the promise text, the entries "+
			"attributed to it, the scope declaration or the applied authority has changed since "+
			"it was shown. Re-run without --confirm and approve the current state",
			applyAmendmentConfirm, digest)
	}

	if err := commitEvolveTransition(cfg, slug, featDir, record, lineages, si, prior, payload, digest); err != nil {
		return err
	}
	out.Confirmed = true
	return emitEvolvePreflight(cmd, out, subjects)
}

// buildEvolutionSubjects derives before, after, delta, attestation and the
// attributed population for every lineage this record changes.
func buildEvolutionSubjects(cfg *config.Context, slug, featDir string, record parser.Amendment, lineages []string, si *parser.ScopeImpact) ([]*evolutionSubject, error) {
	// BEFORE is what the applied ledger currently leaves in force — not the
	// founding text, which a previous revision may already have replaced.
	before, err := resolveActiveIntents(cfg, slug)
	if err != nil {
		return nil, fmt.Errorf("resolve the promises currently in force: %w", err)
	}
	beforeBySlug := map[string]parser.Intent{}
	for _, in := range before.Active {
		beforeBySlug[in.Slug] = in
	}

	// AFTER is prospective: exactly this record applied on top.
	after, err := resolveIntents(cfg, slug, agent.ProspectiveAuthority)
	if err != nil {
		return nil, fmt.Errorf("resolve the promises this record would put in force: %w", err)
	}
	afterBySlug := map[string]parser.Intent{}
	for _, in := range after.Active {
		afterBySlug[in.Slug] = in
	}

	scopes, err := deriveLineageScope(featDir, slug, lineages, record.Affects)
	if err != nil {
		return nil, err
	}
	scopeBySlug := map[string]lineageScope{}
	for _, sc := range scopes {
		scopeBySlug[sc.Lineage] = sc
	}

	sorted := append([]string(nil), lineages...)
	sort.Strings(sorted)
	var out []*evolutionSubject
	for _, lineage := range sorted {
		b, okB := beforeBySlug[lineage]
		a, okA := afterBySlug[lineage]
		if !okB || !okA {
			return nil, fmt.Errorf("%s changes lineage %q, which is not in force — a transition "+
				"applies to a promise the feature currently makes",
				amendmentIdentity(record), lineage)
		}
		mode := parser.IntentMode("")
		for _, tr := range record.IntentTransitions() {
			if tr.Intent == lineage {
				mode = tr.Mode
			}
		}
		out = append(out, &evolutionSubject{
			Lineage: lineage, Mode: string(mode),
			Before: b, After: a, Delta: diffVersions(b, a),
			Attestation:       parser.AttestationFor(mode),
			Scope:             scopeBySlug[lineage],
			PreservesUnlisted: si.PreservesUnlisted,
		})
	}
	return out, nil
}

// commitEvolveTransition re-derives the mutable half of the subject under the
// lock and advances.
func commitEvolveTransition(cfg *config.Context, slug, featDir string, record parser.Amendment, lineages []string, si *parser.ScopeImpact, prior appliedAuthority, payload transitionPayload, digest string) error {
	return withVerifiedAuthority(cfg, slug, prior, func(current appliedAuthority) error {
		// The capsule comparison the boundary performs says nothing about
		// attribution, which comes from mutable contract artifacts. Re-derive
		// the inventory immediately before committing, or a contract edit
		// between preflight and confirmation changes the subject the human's
		// claim was about while leaving the token valid.
		// The amendment itself can also change in the preflight-to-lock window.
		// Rehash it strictly: without this the capsule would be written with
		// the CURRENT bytes while the receipt kept the approved payload's hash,
		// and the mismatch would only surface later as an unreadable capsule.
		nowHash, ok := hashWholeFile(record.Path)
		if !ok {
			return fmt.Errorf("%s could not be rehashed under the lock, so the approval cannot "+
				"be tied to what is about to be written", amendmentIdentity(record))
		}
		if nowHash != payload.AmendmentHash {
			return fmt.Errorf("%s changed between approval and write. Nothing was written — "+
				"re-run without --confirm and approve the current record", amendmentIdentity(record))
		}
		nowScopes, err := deriveLineageScope(featDir, slug, lineages, record.Affects)
		if err != nil {
			return err
		}
		if scopeInventoryDigest(nowScopes) != scopeInventoryDigest(scopesOf(payload.Evolution)) {
			return fmt.Errorf("the contract entries attributed to this promise changed between " +
				"approval and write, so the approval describes a different population. Nothing " +
				"was written — re-run without --confirm and approve the current scope")
		}

		path := baselinePath(cfg, slug)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read baseline for %s under lock: %w", slug, err)
		}
		var baseline Baseline
		if err := yaml.Unmarshal(data, &baseline); err != nil {
			return fmt.Errorf("parse baseline for %s: %w", slug, err)
		}
		op := advanceAuthority(current, record.Seq, []parser.Amendment{record})
		if err := applyAuthorityCapsule(&baseline, slug, op); err != nil {
			return err
		}
		if baseline.TransitionReceipts == nil {
			baseline.TransitionReceipts = map[string]TransitionReceipt{}
		}
		baseline.TransitionReceipts[filepath.Base(record.Path)] = TransitionReceipt{
			Payload: payload, Digest: digest,
		}
		out, err := yaml.Marshal(&baseline)
		if err != nil {
			return err
		}
		return atomicfile.WriteAtomic(path, out)
	})
}

func scopesOf(subjects []*evolutionSubject) []lineageScope {
	out := make([]lineageScope, 0, len(subjects))
	for _, s := range subjects {
		out = append(out, s.Scope)
	}
	return out
}

func emitEvolvePreflight(cmd *cobra.Command, out applyAmendmentPreflight, subjects []*evolutionSubject) error {
	if applyAmendmentJSON {
		return emitPreflight(cmd, out)
	}
	w := cmd.OutOrStdout()
	if out.Confirmed {
		fmt.Fprintf(w, "Applied %s to %s (%s).\nReceipt %s\n",
			out.Amendment, out.Feature, out.Mode, out.Digest)
		for _, s := range subjects {
			fmt.Fprintf(w, "  %s — %s\n", s.Lineage, s.Mode)
		}
		return nil
	}

	fmt.Fprintf(w, "%s changes %d founding promise(s) of %s.\n",
		out.Amendment, len(subjects), out.Feature)
	for _, s := range subjects {
		fmt.Fprintf(w, "\n── %s (%s) ──\n", s.Lineage, s.Mode)
		if len(s.Delta) == 0 {
			fmt.Fprintln(w, "  (no field changes)")
		}
		for _, d := range s.Delta {
			fmt.Fprintf(w, "  %s\n    before: %s\n    after:  %s\n", d.Field, d.Before, d.After)
		}
		fmt.Fprintf(w, "\n  Contract entries attributed to this promise:\n")
		if len(s.Scope.Named) == 0 && len(s.Scope.Unlisted) == 0 {
			fmt.Fprintln(w, "    (none)")
		}
		for _, r := range refsOf(s.Scope.Named) {
			fmt.Fprintf(w, "    %s   [this record declares it changed]\n", r)
		}
		for _, r := range refsOf(s.Scope.Unlisted) {
			fmt.Fprintf(w, "    %s   [not declared changed]\n", r)
		}
		fmt.Fprintf(w, "\n  You are asserting: %s\n", s.Attestation)
		fmt.Fprintln(w, "  And: every entry above marked [not declared changed] remains")
		fmt.Fprintln(w, "  supported by the new promise.")
	}
	fmt.Fprintln(w, "\nNothing checks those two claims. The tool can see that a lineage still")
	fmt.Fprintln(w, "resolves; it cannot see whether a revised promise still entails an entry")
	fmt.Fprintln(w, "attributed to it. That judgement is yours.")
	fmt.Fprintf(w, "\nTo apply:\n\n  parlay internal apply-amendment @%s --confirm %s\n",
		out.Feature, out.Digest)
	return nil
}

var _ = strings.TrimSpace
