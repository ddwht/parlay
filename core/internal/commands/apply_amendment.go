package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// parlay internal apply-amendment — apply ONE amendment under a proof bundle.
//
// This is the authority boundary. save-build-state is storage plumbing: it
// records build evidence after a caller has established authority, and the
// remediation that precedes this command spent five work packages removing its
// ability to manufacture authority from ledger shape, workflow convention and
// maximum sequence. Teaching it to interpret a transition mode, present a
// promise delta and consume a human confirmation would put policy back in the
// primitive that caused the bypass — and a flag becomes habitual.
//
// So the ceremony lives here and the storage stays there. Internally this
// reuses the same capsule writer, hashing and atomic write.
//
// SCOPE. Two shapes, each with its own ceremony:
//
//   - a COMBINED record carrying both affects: and a legacy governance
//     withdrawal, which no other command can apply; and
//   - an INTENT EVOLUTION record in the amends_intents: vocabulary, in any of
//     its four modes — extend, revise, narrow and retire.
//
// Every other shape fails closed and names the command that owns it. A mode is
// never inferred from frontmatter: a record is only ever classified into the
// transition it declares, which is why the legacy spelling keeps the withdrawal
// ceremony rather than being executed as a retirement it never claimed to be.
//
// The command boundary is named for what it does rather than for this one
// mode, because the mode set will grow and the public surface should not have
// to be renamed when it does.

const (
	transitionModeWithdrawAndSplice = "withdraw-and-splice"
	// transitionModeEvolve applies an intent transition in the amends_intents:
	// vocabulary. All four modes: extend, revise, narrow and retire.
	//
	// The three that can take support away from contract entries — narrow,
	// retire, and a revise declaring exceptions — must account for what becomes
	// of each entry that loses it, checked against the artifacts and against a
	// pre-splice inventory. Approving a loss whose consequences nobody
	// collected would have been the original bypass in a new spelling.
	//
	// retire shares this path because the machinery it needs is this machinery;
	// it differs in PRESENTATION (an ending shown in full rather than a delta)
	// and in having no closure to assert.
	transitionModeEvolve = "evolve"
)

var (
	applyAmendmentSelect  string
	applyAmendmentConfirm string
	applyAmendmentJSON    bool
)

var applyAmendmentCmd = &cobra.Command{
	Use:   "apply-amendment @<feature>",
	Short: "Apply one amendment under the proof bundle its transition requires",
	Long: `Apply a feature's single pending amendment, given proof of what it did.

Supports two transitions. A COMBINED record carries both affects: and a legacy
governance withdrawal: it has a splice somebody performed and promises somebody
must approve, and neither half may be applied without the other, which is why
no other command will touch it. An INTENT EVOLUTION record uses the
amends_intents: vocabulary in any of its four modes — extend, revise, narrow
and retire — and is approved against the promise delta, or for a retirement the
whole promise being ended, plus what becomes of every contract entry affected.

Two proofs are required and both are bound to the exact record:

  1. Splice evidence — the refine journal names this amendment and reached the
     test step, exactly as an ordinary refinement must.
  2. Promise approval — the exact promises that stop being in force are printed,
     and the confirmation is bound by digest to the amendment bytes, the promise
     set, the affected contract entries and the prior authority capsule.

Run once without --confirm to see the promises and obtain the digest. Any edit
to the amendment, change to the promise set, or change to the applied authority
invalidates it, and the run must be repeated.`,
	Args: cobra.ExactArgs(1),
	RunE: runApplyAmendment,
}

func init() {
	applyAmendmentCmd.Flags().StringVar(&applyAmendmentSelect, "amendment", "",
		"apply this record specifically — accepted ONLY for the earliest pending one, so a tail cannot be skipped")
	applyAmendmentCmd.Flags().StringVar(&applyAmendmentConfirm, "confirm", "",
		"The digest printed by an unconfirmed run. Binds the approval to exactly what was shown.")
	applyAmendmentCmd.Flags().BoolVar(&applyAmendmentJSON, "json", false,
		"Emit the preflight as JSON rather than prose")
}

// withdrawnPromise is one founding promise this transition ends.
type withdrawnPromise struct {
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Goal   string   `json:"goal"`
	Verify []string `json:"verify,omitempty"`
}

type applyAmendmentPreflight struct {
	Feature   string             `json:"feature"`
	Amendment string             `json:"amendment"`
	Mode      string             `json:"mode"`
	Withdraws []withdrawnPromise `json:"withdraws"`
	Affects   []string           `json:"affects"`
	Digest    string             `json:"digest"`
	Confirmed bool               `json:"confirmed"`
}

func runApplyAmendment(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featDir := cfg.FeaturePath(slug)

	// A half-moved ledger is not a ledger to apply a decision against.
	inFlight, inFlightErr := compactionInFlight(cfg, slug)
	if inFlightErr != nil {
		return fmt.Errorf("%s: %w", slug, inFlightErr)
	}
	if inFlight {
		return fmt.Errorf("%s has an interrupted compaction still in flight. Run "+
			"`parlay internal compact @%s` to recover before applying anything", slug, slug)
	}

	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		return fmt.Errorf("read ledger: %w", err)
	}
	prior, err := observeAppliedAuthority(cfg, slug)
	if err != nil {
		return fmt.Errorf("read applied authority for %s: %w — a transition may not be recorded "+
			"over authority state that could not be inspected", slug, err)
	}

	var pending []parser.Amendment
	for _, a := range amendments {
		if a.Seq > prior.Through {
			pending = append(pending, a)
		}
	}
	if len(pending) == 0 {
		return fmt.Errorf("%s has no unapplied amendments", slug)
	}
	// Exact-tail. This command advances the marker, and the marker passes
	// EVERY record below it, so an unaccounted one would be recorded applied
	// without anybody applying it.
	//
	// With more than one pending record the default still refuses — but it
	// used to refuse with instructions nobody could follow. "Resolve the
	// earlier records first" was the only way out, and there was no way to
	// name an earlier record: no selector existed, so a feature that
	// correctly recorded a second amendment before applying the first had
	// no path forward at all. Recording 002 as a superseding correction
	// rather than editing 001 — which the append-only rule requires — was
	// enough to reach it.
	//
	// --amendment resolves exactly that, and resolves nothing else. It may
	// name ONLY the earliest pending record. Naming a later one is refused
	// precisely because the marker passes everything below it, which is the
	// invariant the exact-tail rule exists to protect; the selector lets a
	// caller work THROUGH a queue in order, never around it.
	earliest := pending[0]
	record := earliest
	if sel := strings.TrimSpace(applyAmendmentSelect); sel != "" {
		match, ok := findPendingAmendment(pending, sel)
		if !ok {
			return fmt.Errorf("%s has no unapplied amendment matching %q — pending: %s",
				slug, sel, joinNames(identities(pending)))
		}
		if match.Seq != earliest.Seq {
			return fmt.Errorf("%s is not the earliest unapplied record for %s — %s is, and applying "+
				"out of order would advance the marker past it, recording it applied without anybody "+
				"having applied it. Apply %s first",
				amendmentIdentity(match), slug, amendmentIdentity(earliest), amendmentIdentity(earliest))
		}
		// Selected the earliest, which is the only selection permitted, so
		// the tail below it is empty by construction and the exact-tail
		// invariant holds.
		record = match
	} else if len(pending) > 1 {
		return fmt.Errorf("%s has %d unapplied amendments (%s) — this operation applies exactly "+
			"one, and advancing past the others would record them applied without anybody having "+
			"applied them. Apply the earliest first with --amendment %s",
			slug, len(pending), joinNames(identities(pending)), amendmentIdentity(earliest))
	}

	// Shape routing. Fail closed, and never classify a record into a mode it
	// does not declare.
	switch classifyAmendment(record) {
	case classCombined:
		// The one supported transition.
	case classGovernance:
		return fmt.Errorf("%s is a governance record with no splice to apply — it goes through "+
			"`parlay internal apply-governance @%s --confirm`, which is the ceremony that owns it",
			amendmentIdentity(record), slug)
	case classIntentEvolution:
		return runEvolveTransition(cmd, cfg, slug, featDir, record, pending, prior)
	case classSplice:
		return fmt.Errorf("%s changes contract entries and withdraws no promise, so it is an "+
			"ordinary refinement — it is applied by /parlay-refine's re-baseline, not here",
			amendmentIdentity(record))
	default:
		return fmt.Errorf("%s declares neither contract entries nor a governance withdrawal, so "+
			"there is no transition to apply", amendmentIdentity(record))
	}

	// The ledger's own rules, first and in full. A journal proves somebody
	// performed and tested a splice; it proves nothing about whether the
	// amendment satisfies the structural governance rules — that its affects:
	// resolve, that its supersedes_intents: name real promises, that its scope
	// accounting is complete. Refusing here rather than reimplementing a
	// partial validator locally: this is the authoritative check.
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
			"sound to apply and no promise list will be shown:\n  - %s",
			slug, len(blocking), joinLines(blocking))
	}

	// Proof 1 — the splice happened, and was tested. Same evidence an ordinary
	// refinement must produce; this transition relaxes nothing about the work.
	if reasons := proveTailJournalFor(cfg, slug, pending, selectedAmendment(record)); len(reasons) > 0 {
		return fmt.Errorf("the splice half of %s is not proven:\n  - %s",
			amendmentIdentity(record), joinLines(reasons))
	}
	spliceProof := fmt.Sprintf("refine-journal:%s:%d:tested", slug, record.Seq)

	// Proof 2's subject — the exact promises that stop being in force.
	withdraws, err := withdrawnPromises(featDir, record)
	if err != nil {
		return err
	}
	if len(withdraws) == 0 {
		return fmt.Errorf("%s names supersedes_intents that resolve to no founding promise in "+
			"this feature — refusing to confirm a withdrawal of nothing",
			amendmentIdentity(record))
	}

	affects := append([]string(nil), record.Affects...)
	sort.Strings(affects)

	payload, err := buildTransitionPayload(slug, record, withdraws, affects, prior, spliceProof)
	if err != nil {
		return err
	}
	digest, err := transitionDigest(payload)
	if err != nil {
		return err
	}

	out := applyAmendmentPreflight{
		Feature:   slug,
		Amendment: amendmentIdentity(record),
		Mode:      transitionModeWithdrawAndSplice,
		Withdraws: withdraws,
		Affects:   affects,
		Digest:    digest,
	}

	if applyAmendmentConfirm == "" {
		out.Confirmed = false
		return emitPreflight(cmd, out)
	}
	if applyAmendmentConfirm != digest {
		return fmt.Errorf("the confirmation does not match what is on disk now. It was given for "+
			"%s and this transition is %s — the amendment, the promises it withdraws, the entries "+
			"it affects, or the applied authority has changed since it was shown. Re-run without "+
			"--confirm, read the promises again, and confirm the new digest",
			applyAmendmentConfirm, digest)
	}

	// Both proofs hold. One protected transaction, through the shared writer.
	if err := advanceThroughTransition(cfg, slug, record, prior, payload, digest); err != nil {
		return err
	}
	out.Confirmed = true
	return emitPreflight(cmd, out)
}

// withdrawnPromises resolves the record's supersedes_intents to the founding
// promises themselves, so the human approves prose rather than slugs.
func withdrawnPromises(featDir string, record parser.Amendment) ([]withdrawnPromise, error) {
	intents, err := parser.ParseIntentsFile(filepath.Join(featDir, "intents.md"))
	if err != nil {
		return nil, fmt.Errorf("read founding promises: %w — the withdrawal list cannot be shown, "+
			"and an approval of a list nobody saw is not an approval", err)
	}
	byslug := map[string]parser.Intent{}
	for _, in := range intents {
		byslug[in.Slug] = in
	}
	var out []withdrawnPromise
	var unknown []string
	for _, s := range record.SupersedesIntents {
		in, ok := byslug[s]
		if !ok {
			// Never skip. Skipping would present and bind approval for the
			// promises that DID resolve, then advance the whole amendment —
			// so a record naming one real promise and one typo would be
			// applied on an approval that never mentioned the typo.
			unknown = append(unknown, s)
			continue
		}
		out = append(out, withdrawnPromise{
			Slug: in.Slug, Title: in.Title, Goal: in.Goal, Verify: in.Verify,
		})
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%s names %d founding promise(s) this feature does not declare "+
			"(%s) — refusing rather than approving the withdrawal of the ones that do resolve "+
			"while silently advancing past the ones that do not",
			amendmentIdentity(record), len(unknown), joinNames(unknown))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// transitionApprovalScheme names the canonicalisation a digest was computed
// under. It is part of the hashed payload, so a future scheme cannot produce a
// token that validates against this one.
const transitionApprovalScheme = "parlay.transition-approval.v1"

// priorCapsuleSnapshot is the COMPLETE applied-authority state a transition
// advanced from — including both method maps, which an earlier version of this
// digest omitted while its comment claimed to cover "the prior authority
// capsule".
type priorCapsuleSnapshot struct {
	Through    int               `yaml:"through" json:"through"`
	Amendments map[string]string `yaml:"amendments,omitempty" json:"amendments,omitempty"`
	Outputless map[string]bool   `yaml:"outputless,omitempty" json:"outputless,omitempty"`
	// Receipts covers prior receipt CONTENT by digest, not by presence. An
	// earlier version recorded presence only, so a changed prior receipt
	// produced the same next token — the field name said digest while the
	// implementation said "true".
	Receipts map[string]string `yaml:"receipts,omitempty" json:"receipts,omitempty"`
	// AppliedAt is covered for the reason the struct comment gives: this claims
	// to be the COMPLETE prior state, and a field left out of a digest that
	// says "complete" is how the method maps came to be missing from an earlier
	// version of it.
	//
	// omitempty, and that is load-bearing rather than cosmetic. A receipt
	// written before this field existed stores no applied-at, unmarshals to
	// nil, and re-marshals to nothing — so its digest is unchanged and it still
	// validates against itself. Without omitempty every stored receipt in every
	// project would fail its own check on upgrade.
	AppliedAt map[string]string `yaml:"applied-at,omitempty" json:"applied_at,omitempty"`
}

// transitionPayload is exactly what an approval approves.
//
// It is marshalled as canonical JSON rather than hand-delimited, because the
// promise text it carries is arbitrary prose: a hand-built NUL/newline
// serialisation invites two different payloads to render identically, and
// inventing delimiters for attacker-influenced content is a needless ambiguity
// surface.
//
// The domain fields matter as much as the content. A digest is a BEARER token,
// and its security domain is every invocation that will accept it — not the one
// that produced it. Two features can hold byte-identical amendments, promises,
// refs and capsules, and without the scheme, the feature slug and the exact
// sequence and filename, a token minted for one would authorise the other.
type transitionPayload struct {
	Scheme        string               `yaml:"scheme" json:"scheme"`
	Feature       string               `yaml:"feature" json:"feature"`
	Mode          string               `yaml:"mode" json:"mode"`
	AmendmentSeq  int                  `yaml:"amendment-seq" json:"amendment_seq"`
	AmendmentFile string               `yaml:"amendment-file" json:"amendment_file"`
	AmendmentHash string               `yaml:"amendment-hash" json:"amendment_hash"`
	Withdraws     []withdrawnPromise   `yaml:"withdraws" json:"withdraws"`
	Affects       []string             `yaml:"affects" json:"affects"`
	Prior         priorCapsuleSnapshot `yaml:"prior" json:"prior"`
	// Evolution is present only for transitionModeEvolve.
	Evolution []*evolutionSubject `yaml:"evolution,omitempty" json:"evolution,omitempty"`
	// SpliceProof is the NORMALISED splice-proof proposition, not a hash of
	// the journal file. The human approves a promise withdrawal, not the
	// whitespace or resume metadata of an ephemeral workflow record — and the
	// confirmed run re-proves the proposition before committing, so an edit
	// that preserves the proof need not invalidate an approval while one that
	// breaks it still refuses.
	SpliceProof string `yaml:"splice-proof" json:"splice_proof"`
}

func buildTransitionPayload(slug string, record parser.Amendment, withdraws []withdrawnPromise, affects []string, prior appliedAuthority, spliceProof string) (transitionPayload, error) {
	amendHash, ok := hashWholeFile(record.Path)
	if !ok {
		return transitionPayload{}, fmt.Errorf("%s could not be hashed, so an approval cannot be "+
			"bound to it", amendmentIdentity(record))
	}
	return transitionPayload{
		Scheme:        transitionApprovalScheme,
		Feature:       slug,
		Mode:          transitionModeWithdrawAndSplice,
		AmendmentSeq:  record.Seq,
		AmendmentFile: filepath.Base(record.Path),
		AmendmentHash: amendHash,
		Withdraws:     withdraws,
		Affects:       affects,
		Prior: priorCapsuleSnapshot{
			Through:    prior.Through,
			Amendments: prior.Hashes,
			Outputless: prior.Outputless,
			Receipts:   receiptDigests(prior.Receipts),
			AppliedAt:  prior.AppliedAt,
		},
		SpliceProof: spliceProof,
	}, nil
}

// evolutionSubject is everything a human is approving when a promise changes
// without dying.
//
// Three propositions, not one: that this promise now reads differently, that
// the declared mode is semantically honest, and that the transition does not
// silently invalidate downstream entries outside its declared scope. Text alone
// answers only the first.
type evolutionSubject struct {
	Lineage string `yaml:"lineage" json:"lineage"`
	Mode    string `yaml:"mode" json:"mode"`
	// Before is the promise as the APPLIED ledger currently leaves it, not the
	// founding text — a lineage may already have been revised, and approving a
	// delta from a version nobody is running would describe a change that is
	// not the one being made.
	Before parser.Intent `yaml:"before" json:"before"`
	After  parser.Intent `yaml:"after" json:"after"`
	Delta  []fieldDelta  `yaml:"delta" json:"delta"`
	// Attestation is the claim the human is making, recorded verbatim. The
	// tool cannot verify it from prose and the receipt must say what was
	// asserted rather than merely that something was.
	Attestation string `yaml:"attestation" json:"attestation"`
	// Scope is the exact attributed population after the change, partitioned.
	Scope lineageScope `yaml:"scope" json:"scope"`
	// ScopeBefore is the population the promise justified BEFORE it, captured
	// pre-splice. Stored so the receipt records what was approved rather than
	// leaving a later reader to reconstruct a population that no longer exists.
	ScopeBefore lineageScope `yaml:"scope-before" json:"scope_before"`
	// PreservesUnlisted is the author's closure declaration AS IT APPLIES TO
	// THIS LINEAGE. Always false for a retirement: the claim is that unlisted
	// entries stay supported by the changed promise, and a retirement leaves no
	// promise to support them. A record that retires one lineage and revises
	// another therefore stores true on one subject and false on the other,
	// which is what the author actually asserted.
	PreservesUnlisted bool `yaml:"preserves-unlisted" json:"preserves_unlisted"`
	// Consequences are the checked, structured results for each declared
	// exception — which entry, as it was, what became of it, and where the
	// disposition names one, the replacement that carries its work now.
	Consequences []ConsequenceReceipt `yaml:"consequences,omitempty" json:"consequences,omitempty"`
}

// transitionDigest is the full SHA-256 of the canonical payload. Not truncated:
// there is no usability gain worth making an authority token weaker, or worth
// having to explain the exception.
//
// The payload is round-tripped through the STORAGE encoding before hashing, so
// a digest minted in memory and one recomputed from a stored receipt are taken
// over the same shape by construction. Without that, YAML's nil-becomes-empty
// on decode made a freshly written receipt fail its own validation — and doing
// it structurally rather than normalising each slice by hand keeps that true
// when a field is added later.
func transitionDigest(p transitionPayload) (string, error) {
	canonical, err := canonicalTransitionPayload(p)
	if err != nil {
		return "", err
	}
	// encoding/json sorts map keys, so this is canonical for a fixed struct.
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonicalise the approval payload: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalTransitionPayload returns the payload as it will read back from the
// receipt it is stored in.
func canonicalTransitionPayload(p transitionPayload) (transitionPayload, error) {
	encoded, err := yaml.Marshal(&p)
	if err != nil {
		return transitionPayload{}, fmt.Errorf("canonicalise the approval payload: %w", err)
	}
	var out transitionPayload
	if err := yaml.Unmarshal(encoded, &out); err != nil {
		return transitionPayload{}, fmt.Errorf("canonicalise the approval payload: %w", err)
	}
	return out, nil
}

// advanceThroughTransition commits the capsule advance and the receipt in one
// protected transaction.
//
// The lock is not decoration. Between observing `prior` and writing, another
// writer can advance this feature's authority; an atomic rename prevents a torn
// file, not a lost update, and applyAuthorityCapsule rebuilds the capsule from
// the STALE prior — so a concurrent writer's evidence would be silently erased
// and the command's own promise that "any change to applied authority
// invalidates the approval" would be false for a change during the confirmed
// run. So: exclude, re-observe under exclusion, compare against what was
// approved, and only then write.
func advanceThroughTransition(cfg *config.Context, slug string, record parser.Amendment, prior appliedAuthority, payload transitionPayload, digest string) error {
	return withVerifiedAuthority(cfg, slug, prior, func(current appliedAuthority) error {
		path := baselinePath(cfg, slug)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read baseline for %s: %w", slug, err)
		}
		var baseline Baseline
		if err := yaml.Unmarshal(data, &baseline); err != nil {
			return fmt.Errorf("parse baseline for %s: %w", slug, err)
		}

		op := advanceAuthority(current, record.Seq, []parser.Amendment{record})
		if err := applyAuthorityCapsule(&baseline, slug, op); err != nil {
			return err
		}

		// The receipt travels with the capsule, in the same write. A boolean
		// would record only that this code path once ran; it would not record
		// WHAT was approved, under which canonicalisation, or from which
		// authority — which is weaker than the invariant this design states.
		if baseline.TransitionReceipts == nil {
			baseline.TransitionReceipts = map[string]TransitionReceipt{}
		}
		baseline.TransitionReceipts[filepath.Base(record.Path)] = TransitionReceipt{
			Payload: payload,
			Digest:  digest,
		}

		out, err := yaml.Marshal(&baseline)
		if err != nil {
			return err
		}
		return atomicfile.WriteAtomic(path, out)
	})
}

func receiptDigests(m map[string]TransitionReceipt) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, r := range m {
		out[k] = r.Digest
	}
	return out
}

func emitPreflight(cmd *cobra.Command, out applyAmendmentPreflight) error {
	if applyAmendmentJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	w := cmd.OutOrStdout()
	if out.Confirmed {
		fmt.Fprintf(w, "Applied %s to %s (%s).\n", out.Amendment, out.Feature, out.Mode)
		fmt.Fprintf(w, "Receipt %s\n", out.Digest)
		fmt.Fprintln(w, "No longer in force:")
		for _, p := range out.Withdraws {
			fmt.Fprintf(w, "  %s (%s)\n", p.Title, p.Slug)
		}
		return nil
	}
	fmt.Fprintf(w, "%s would end %d founding promise(s) of %s.\n\n",
		out.Amendment, len(out.Withdraws), out.Feature)
	fmt.Fprintln(w, "These stop being in force:")
	for _, p := range out.Withdraws {
		fmt.Fprintf(w, "\n  %s\n", p.Title)
		if p.Goal != "" {
			fmt.Fprintf(w, "    Goal: %s\n", p.Goal)
		}
		for _, v := range p.Verify {
			fmt.Fprintf(w, "    Verify: %s\n", v)
		}
	}
	if len(out.Affects) > 0 {
		fmt.Fprintf(w, "\nContract entries this record changes:\n")
		for _, a := range out.Affects {
			fmt.Fprintf(w, "  %s\n", a)
		}
	}
	fmt.Fprintf(w, "\nRead that list — it, not the amendment's filename, is what you are\n"+
		"approving. To apply:\n\n  parlay internal apply-amendment @%s --confirm %s\n\n",
		out.Feature, out.Digest)
	fmt.Fprintln(w, "The digest binds the approval to exactly what is printed above. Any edit")
	fmt.Fprintln(w, "to the record, the promises or the applied authority invalidates it.")
	return nil
}

// findPendingAmendment matches a selector against the pending records.
//
// Accepts the slug, the NNN prefix, or the filename, because a caller
// reading the refusal message sees one form and a caller reading a
// directory listing sees another, and making them guess which the flag
// wants is a refusal they will hit twice.
func findPendingAmendment(pending []parser.Amendment, sel string) (parser.Amendment, bool) {
	sel = strings.TrimSuffix(strings.TrimSpace(sel), ".md")
	for _, a := range pending {
		id := amendmentIdentity(a)
		switch sel {
		case id, a.Slug, fmt.Sprintf("%03d", a.Seq), fmt.Sprintf("%03d-%s", a.Seq, a.Slug):
			return a, true
		}
	}
	return parser.Amendment{}, false
}

// selectedAmendment names the record an apply is applying, for the tail
// proof. Always non-nil on this path: apply-amendment applies exactly one
// record, and which one it is is the fact the journal must match.
func selectedAmendment(record parser.Amendment) *parser.Amendment {
	return &record
}
