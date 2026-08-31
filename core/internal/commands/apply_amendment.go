package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
// SCOPE, deliberately narrow. Today it recognises exactly one shape: an
// amendment carrying BOTH affects: and a governance withdrawal, which is the
// shape the accounting rule produces for any feature whose retiring promise
// still has live contract entries, and which no other command can apply. Every
// other shape fails closed and names the command that owns it. Future
// transition modes (extend, revise, narrow) are a separate design and are NOT
// inferred from today's frontmatter — a record is never classified into a mode
// it does not declare.
//
// The command boundary is named for what it does rather than for this one
// mode, because the mode set will grow and the public surface should not have
// to be renamed when it does.

const transitionModeWithdrawAndSplice = "withdraw-and-splice"

var (
	applyAmendmentConfirm string
	applyAmendmentJSON    bool
)

var applyAmendmentCmd = &cobra.Command{
	Use:   "apply-amendment @<feature>",
	Short: "Apply one amendment under the proof bundle its transition requires",
	Long: `Apply a feature's single pending amendment, given proof of what it did.

Currently supports one transition: a COMBINED record carrying both affects: and
a governance withdrawal. That record has a splice somebody performed and
promises somebody must approve, and neither half may be applied without the
other — which is why no other command will touch it.

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
	// Exact-tail. This command advances the marker, and the marker passes EVERY
	// record below it, so an unaccounted one would be recorded applied without
	// anybody applying it.
	if len(pending) > 1 {
		return fmt.Errorf("%s has %d unapplied amendments (%s) — this operation applies exactly "+
			"one, and advancing past the others would record them applied without anybody having "+
			"applied them. Resolve the earlier records first",
			slug, len(pending), joinNames(identities(pending)))
	}
	record := pending[0]

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
		return fmt.Errorf("%s carries an amends_intents: transition, and no applier exists for "+
			"the evolution vocabulary yet. It is readable and inert; nothing will apply it until "+
			"its ceremony is built", amendmentIdentity(record))
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
	if reasons := proveTailJournal(cfg, slug, pending); len(reasons) > 0 {
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
		},
		SpliceProof: spliceProof,
	}, nil
}

// transitionDigest is the full SHA-256 of the canonical payload. Not truncated:
// there is no usability gain worth making an authority token weaker, or worth
// having to explain the exception.
func transitionDigest(p transitionPayload) (string, error) {
	// encoding/json sorts map keys, so this is canonical for a fixed struct.
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("canonicalise the approval payload: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
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
