package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The evolve ceremony: applying a transition in the amends_intents: vocabulary.
//
// All four modes. Three of them — extend, revise and narrow — leave a promise
// behind and are presented as a DELTA against the version currently in force.
// The fourth, retire, ends the promise and is presented as an ENDING: the whole
// proposition shown in full, because a before/after with every field blanking
// reads like a rewrite and asks the human a different question than the one
// they are being asked.
//
// A transition that can take support away from contract entries — narrow,
// retire, or a revise declaring exceptions — must account for every entry that
// loses it, against a pre-splice inventory the artifacts can no longer supply.
// Approving a loss whose consequences nobody accounted for would be the bypass
// this whole line of work exists to close, wearing a new verb.
//
// Nothing here pretends the human's claim is machine-provable. It records an
// assertion against an exact inventory, names what was not checked, and says so
// per promise rather than in one sentence that fits neither mode.

func runEvolveTransition(cmd *cobra.Command, cfg *config.Context, slug, featDir string, record parser.Amendment, pending []parser.Amendment, prior appliedAuthority) error {
	// All-or-nothing. One unsupported transition refuses the whole record;
	// a partial application would leave a lineage evolved and its sibling not,
	// which no reader could classify.
	var lineages []string
	narrowing := map[string]bool{}
	retiring := map[string]bool{}
	for _, tr := range record.IntentTransitions() {
		switch tr.Mode {
		case parser.IntentExtend, parser.IntentRevise:
			lineages = append(lineages, tr.Intent)
		case parser.IntentNarrow:
			lineages = append(lineages, tr.Intent)
			narrowing[tr.Intent] = true
		case parser.IntentRetire:
			// retire is the end of a promise, not a change to one, and the
			// ceremony below presents it as such: no delta, the full promise
			// being ended, and an accounting for EVERY entry it justified
			// rather than only those outside a closure. It shares this path
			// because the machinery it needs — the pre-splice capture, the
			// consequence checks, the authority transaction — is the same
			// machinery, and a second copy of it would be a second place for
			// the guarantees to drift.
			lineages = append(lineages, tr.Intent)
			narrowing[tr.Intent] = true
			retiring[tr.Intent] = true
		default:
			// The legacy spelling has UNKNOWN semantics by construction, so it
			// cannot be executed as any particular mode. It keeps the two-proof
			// withdrawal path, which shows the promise list and asks the human
			// what the record actually meant.
			return fmt.Errorf("%s ends lineage %q with mode %q, whose meaning was never "+
				"recorded — it goes through the withdrawal ceremony, which asks what the record "+
				"meant rather than assuming. The whole record is refused rather than partly "+
				"applied", amendmentIdentity(record), tr.Intent, tr.Mode)
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
	if reasons := proveTailJournalFor(cfg, slug, pending, selectedAmendment(record)); len(reasons) > 0 {
		return fmt.Errorf("the work behind %s is not proven:\n  - %s",
			amendmentIdentity(record), joinLines(reasons))
	}
	spliceProof := fmt.Sprintf("refine-journal:%s:%d:tested", slug, record.Seq)

	subjects, err := buildEvolutionSubjects(cfg, slug, featDir, record, lineages, si)
	if err != nil {
		return err
	}

	// Consequence accounting. Every declared exception is checked against the
	// derived population and the splice: a disposition that contradicts what is
	// on disk is a claim about a state nobody is in.
	scopes := make([]lineageScope, 0, len(subjects))
	for _, sub := range subjects {
		scopes = append(scopes, sub.Scope)
	}
	// The prior subject population, captured before the splice mutated
	// anything. Without it there is no evidence of what the promise justified:
	// a removed entry is invisible after the fact, so any plausible absent ref
	// could be declared a consequence.
	journal, jerr := loadRefineJournal(cfg, slug)
	if jerr != nil || journal == nil {
		return fmt.Errorf("%s: the refine journal is unavailable, so the entries this promise "+
			"justified BEFORE the change cannot be established", amendmentIdentity(record))
	}
	if err := scopeCaptureMatches(journal, record, lineages); err != nil {
		return fmt.Errorf("%s: %w — an inventory is evidence about the record it was captured "+
			"for and no other", amendmentIdentity(record), err)
	}

	consequences := checkScopeConsequences(cfg, slug, record, journal.ScopeBefore, scopes, narrowing, retiring)
	if len(consequences.Problems) > 0 {
		return fmt.Errorf("%s: the declared consequences do not match the contract:\n  - %s",
			amendmentIdentity(record), joinLines(consequences.Problems))
	}
	for i := range subjects {
		subjects[i].Consequences = consequences.ByLineage[subjects[i].Lineage]
		subjects[i].ScopeBefore = scopeBeforeFor(journal.ScopeBefore, subjects[i].Lineage)
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

	if err := commitEvolveTransition(cfg, slug, featDir, record, lineages, si, prior, payload, digest, journal.ScopeBefore, narrowing, retiring); err != nil {
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

	// AFTER is prospective for THIS RECORD: the applied ledger plus
	// exactly the selected amendment, and nothing after it.
	//
	// It used to ask ProspectiveAuthority over the whole unapplied tail,
	// which resolves the newest claim — so with 002 superseding 001,
	// selecting 001 derived, displayed and receipted 002's promise text.
	// The comment said "exactly this record" while the code did not.
	after, err := resolveIntentsThrough(cfg, slug, record.Seq)
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
		mode := parser.IntentMode("")
		for _, tr := range record.IntentTransitions() {
			if tr.Intent == lineage {
				mode = tr.Mode
			}
		}
		if !okB {
			return nil, fmt.Errorf("%s changes lineage %q, which is not in force — a transition "+
				"applies to a promise the feature currently makes",
				amendmentIdentity(record), lineage)
		}
		// The after state is the inverse for a retirement, and asserting it is
		// how the ceremony knows the record it is about to approve actually
		// ends the promise rather than merely claiming to.
		//
		// Defence in depth, not a pinned guard: this compares two independently
		// computed things — what the record says it does, and what the resolver
		// says the feature will promise once it is applied — and I could not
		// construct a document where they disagree, because the resolver treats
		// any lineage-ending claim as winning. It stays because the two are
		// separate subsystems that can drift, and a silent disagreement between
		// them is exactly the kind of thing an approval must not be built on.
		if mode == parser.IntentRetire {
			if okA {
				return nil, fmt.Errorf("%s retires lineage %q, but that promise is still in "+
					"force once this record is applied — the resolver and the record disagree "+
					"about what this change does", amendmentIdentity(record), lineage)
			}
			a = parser.Intent{}
		} else if !okA {
			return nil, fmt.Errorf("%s changes lineage %q, but that promise is no longer in "+
				"force once this record is applied — a change to a promise leaves one behind",
				amendmentIdentity(record), lineage)
		}
		delta := diffVersions(b, a)
		if mode == parser.IntentRetire {
			// A delta against an empty promise would render as every field
			// being blanked, which reads like a rewrite rather than an ending.
			// The presentation below shows the promise being ended in full.
			delta = nil
		}
		out = append(out, &evolutionSubject{
			Lineage: lineage, Mode: string(mode),
			Before: b, After: a, Delta: delta,
			Attestation: parser.AttestationFor(mode),
			Scope:       scopeBySlug[lineage],
			// PER LINEAGE, not the record's global flag. A record may retire
			// one promise and revise another; the closure claim belongs to the
			// living one and is false for the ending one, so recording the
			// record-level bool against a retirement would store an assertion
			// the author is not making.
			PreservesUnlisted: si.PreservesUnlisted && mode != parser.IntentRetire,
		})
	}
	return out, nil
}

// commitEvolveTransition re-derives the mutable half of the subject under the
// lock and advances.
func commitEvolveTransition(cfg *config.Context, slug, featDir string, record parser.Amendment, lineages []string, si *parser.ScopeImpact, prior appliedAuthority, payload transitionPayload, digest string, journalScopeBefore []lineageScope, narrowing, retiring map[string]bool) error {
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
		// The scope re-derivation only watches entries still attributed to the
		// changed lineages, so on its own it cannot see a replacement or a
		// revised entry that moved elsewhere. Re-derive the consequence
		// subjects too, or an edit to a replacement between approval and write
		// would change what the human agreed to while leaving the token valid.
		nowConsequences := checkScopeConsequences(cfg, slug, record, journalScopeBefore, mustScopes(featDir, slug, lineages, record.Affects), narrowing, retiring)
		if len(nowConsequences.Problems) > 0 {
			return fmt.Errorf("the declared consequences no longer hold:\n  - %s",
				joinLines(nowConsequences.Problems))
		}
		nowConsequenceDigest, err := consequenceDigest(nowConsequences.ByLineage)
		if err != nil {
			return err
		}
		approvedConsequenceDigest, err := consequenceDigest(consequencesOf(payload.Evolution))
		if err != nil {
			return err
		}
		if nowConsequenceDigest != approvedConsequenceDigest {
			return fmt.Errorf("what becomes of the entries this record declares changed between " +
				"approval and write — a replacement or a surviving entry is not what it was when " +
				"it was approved. Nothing was written")
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
		if s.Mode == string(parser.IntentRetire) {
			// An ending is shown in full, never as a delta. A retirement
			// rendered as every field blanking reads like a rewrite, and the
			// human is being asked for a different approval than that: not
			// "is this the right new text" but "is this promise over".
			fmt.Fprintln(w, "  This promise ENDS. Nothing takes it over.")
			fmt.Fprintf(w, "\n  The promise being ended, in full:\n")
			writeWholeIntent(w, "    ", s.Before)
		} else {
			if len(s.Delta) == 0 {
				fmt.Fprintln(w, "  (no field changes)")
			}
			for _, d := range s.Delta {
				fmt.Fprintf(w, "  %s\n    before: %s\n    after:  %s\n", d.Field, d.Before, d.After)
			}
		}
		if s.Mode == string(parser.IntentRetire) {
			fmt.Fprintf(w, "\n  Contract entries this promise justified, all of which lose it:\n")
			for _, c := range s.Consequences {
				fmt.Fprintf(w, "    %s\n", c.Ref)
			}
			if len(s.Consequences) == 0 {
				fmt.Fprintln(w, "    (none — this promise supported no contract entry)")
			}
		} else {
			fmt.Fprintf(w, "\n  Contract entries attributed to this promise:\n")
			if len(s.Scope.Named) == 0 && len(s.Scope.Unlisted) == 0 {
				fmt.Fprintln(w, "    (none)")
			}
		}
		if s.Mode != string(parser.IntentRetire) {
			for _, r := range refsOf(s.Scope.Named) {
				fmt.Fprintf(w, "    %s   [this record declares it changed]\n", r)
			}
			for _, r := range refsOf(s.Scope.Unlisted) {
				fmt.Fprintf(w, "    %s   [not declared changed]\n", r)
			}
		}
		if len(s.Consequences) > 0 {
			fmt.Fprintf(w, "\n  What becomes of the entries this record does declare:\n")
			for _, c := range s.Consequences {
				fmt.Fprintf(w, "    %s — %s\n", c.Ref, c.Claim)
				if c.AfterRef != "" && c.AfterRef != c.Ref {
					fmt.Fprintf(w, "      now carried by: %s\n", c.AfterRef)
				}
			}
		}
		fmt.Fprintf(w, "\n  You are asserting: %s\n", s.Attestation)
		// The closure claim is about entries that stay supported by the new
		// promise. A retirement leaves no new promise, so there is no such
		// claim to make and printing one would ask for an assertion that
		// cannot be true.
		if s.Mode != string(parser.IntentRetire) {
			fmt.Fprintln(w, "  And: every entry above marked [not declared changed] remains")
			fmt.Fprintln(w, "  supported by the new promise.")
		}
	}
	// The epistemic footer is per subject, not one global sentence. A record can
	// carry a revision and a retirement at once, and the unchecked claims are
	// different in kind: for a living promise the tool cannot see whether the
	// new text still entails an entry attributed to it; for a retirement there
	// is no closure claim at all, and the lineage specifically must NOT resolve
	// afterwards. Printing the living sentence over a retirement describes a
	// check the tool did not make about a promise that no longer exists.
	fmt.Fprintln(w, "\nWhat the tool cannot check, per promise:")
	for _, s := range subjects {
		if s.Mode == string(parser.IntentRetire) {
			fmt.Fprintf(w, "  %s — that this promise SHOULD end, and that each consequence above\n",
				s.Lineage)
			fmt.Fprintln(w, "    is the right fate for the work it justified. The tool checked that")
			fmt.Fprintln(w, "    every entry is accounted for and that none still names this promise;")
			fmt.Fprintln(w, "    it cannot judge whether ending it is correct.")
			continue
		}
		fmt.Fprintf(w, "  %s — that the new text still entails every entry attributed to it.\n",
			s.Lineage)
		fmt.Fprintln(w, "    The tool can see the lineage still resolves; it cannot read the promise")
		fmt.Fprintln(w, "    and the entry and tell you one follows from the other.")
	}
	fmt.Fprintln(w, "\nThat judgement is yours.")
	fmt.Fprintf(w, "\nTo apply:\n\n  parlay internal apply-amendment @%s --confirm %s\n",
		out.Feature, out.Digest)
	return nil
}

// writeWholeIntent prints every field of a promise, absent ones included.
//
// A withdrawal approval is over the COMPLETE proposition being ended, so an
// omitted field is not a tidier rendering — it is a part of the promise the
// human was never shown before agreeing it is over. Absence prints as (none)
// rather than vanishing, so what is missing is visible as missing.
func writeWholeIntent(w io.Writer, indent string, in parser.Intent) {
	line := func(label, v string) {
		if strings.TrimSpace(v) == "" {
			v = "(none)"
		}
		fmt.Fprintf(w, "%s%-12s %s\n", indent, label+":", v)
	}
	list := func(label string, vs []string) {
		if len(vs) == 0 {
			fmt.Fprintf(w, "%s%-12s (none)\n", indent, label+":")
			return
		}
		fmt.Fprintf(w, "%s%s:\n", indent, label)
		for _, v := range vs {
			fmt.Fprintf(w, "%s  - %s\n", indent, v)
		}
	}
	line("title", in.Title)
	line("goal", in.Goal)
	line("persona", in.Persona)
	line("priority", in.Priority)
	line("context", in.Context)
	line("action", in.Action)
	list("objects", in.Objects)
	list("constraints", in.Constraints)
	list("verify", in.Verify)
	list("questions", in.Questions)
}

// mustScopes re-derives the post-splice population, treating a derivation
// failure as an empty population so the comparison below refuses rather than
// panicking. The scope derivation itself already refused unreadable artifacts
// during the preflight; this is the belt on that brace.
func mustScopes(featDir, slug string, lineages, affects []string) []lineageScope {
	scopes, err := deriveLineageScope(featDir, slug, lineages, affects)
	if err != nil {
		return nil
	}
	return scopes
}

func consequencesOf(subjects []*evolutionSubject) map[string][]ConsequenceReceipt {
	out := map[string][]ConsequenceReceipt{}
	for _, s := range subjects {
		out[s.Lineage] = s.Consequences
	}
	return out
}

// consequenceDigest hashes the COMPLETE structured result, so what the commit
// re-check compares is what the approval showed and what the receipt stores.
//
// The earlier version listed fields by hand and had already drifted: it omitted
// Claim — which is part of what the human reads and what gets stored — and took
// Lineage from the outer map rather than the record. A hand-maintained
// projection means a field added later is silently outside the comparison until
// somebody remembers this function. Serialising the whole value through the
// storage encoding removes that trap: a new field is covered the day it exists,
// and the digest is taken over the shape the receipt actually round-trips to.
func consequenceDigest(byLineage map[string][]ConsequenceReceipt) (string, error) {
	// Only lineages that actually carry a consequence, so "no key" and "an
	// empty list" canonicalise the same way. Otherwise the two sides of the
	// comparison differ over nothing.
	subject := map[string][]ConsequenceReceipt{}
	for k, v := range byLineage {
		if len(v) > 0 {
			subject[k] = v
		}
	}
	// Round-trip through the storage encoding first, for the same reason the
	// transition receipt does: a value hashed in memory can differ from the one
	// read back (nil versus empty, omitted versus zero), and the digest has to
	// describe what will be on disk.
	encoded, err := yaml.Marshal(subject)
	if err != nil {
		return "", fmt.Errorf("canonicalise the consequences: %w", err)
	}
	var out map[string][]ConsequenceReceipt
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return "", fmt.Errorf("canonicalise the consequences: %w", err)
	}
	// encoding/json sorts map keys, so this is canonical for a fixed struct.
	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("canonicalise the consequences: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
