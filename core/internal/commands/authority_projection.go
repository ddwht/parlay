package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The authority projection (WP5).
//
// Compaction moves applied records into amendments/archive/. That is a
// LOCATION change, and it must not be an AUTHORITY change — but the loader
// ignores subdirectories, so archiving is not inert: an archived record's
// supersedes_intents claim disappears from the ledger's view, which can
// reactivate a founding intent, and an active record naming an archived slug
// in supersedes: becomes a dangling edge reported as
// amendment-supersedes-unknown.
//
// So compaction is a semantic projection operation, not a file move with a
// verification bolted on. This computes the projection that must be identical
// either side of it.
//
// What is NOT promised identical: the historical and audit views. Archived
// records leave the semantic walk entirely, so the amendment listing and
// all_affects shrink — that IS what compaction means. Only the fields below
// are guaranteed unchanged.

// authorityProjection is the canonical, order-normalised statement of what a
// feature currently promises and how far its ledger is applied.
type authorityProjection struct {
	// ActiveIntents are the founding intents still in force.
	ActiveIntents []string
	// ActiveVersions is what each of those promises currently SAYS, and which
	// decision put that text there, as `<amendment>|<mode>|<text fingerprint>`.
	//
	// Slugs alone were not enough. Compaction moves applied records to
	// amendments/archive/, and a projection comparing only slugs passes
	// unchanged while the promise text silently reverts to its founding
	// version — the equivalence the operation advertises could not detect the
	// one class of loss the operation can cause. The resolver now reads the
	// archive, so this would no longer regress; it is recorded anyway, because
	// an equivalence check that cannot see a field is not evidence about it.
	//
	// Provenance is in the value, not only the text. `parlay spec` reports
	// which decision put a promise's text there, so compaction must not be able
	// to change that answer and still pass. Two amendments producing identical
	// snapshots are a real possibility — a revert, a reapplied wording — and a
	// bare text hash cannot tell them apart.
	ActiveVersions map[string]string
	// SupersededIntents maps a retired founding intent to the amendment
	// identity that retired it — the winning head. Archiving that amendment
	// without this changing is the whole risk.
	SupersededIntents map[string]string
	// RetiredBy is the terminal record's identity, empty when not retired.
	RetiredBy string
	// PendingTail names the unapplied records by identity.
	PendingTail []string
	// SupersededBy is the forward-link graph: superseded slug to the slugs
	// that replace it. A dangling edge shows up here as a lost entry.
	SupersededBy map[string][]string
	// Errors are the error-severity ledger findings. A compaction may neither
	// begin over them nor create one.
	Errors []string
	// AppliedThrough and Evidence are the authority capsule itself.
	AppliedThrough int
	Evidence       map[string]string
	Outputless     map[string]bool
}

// computeAuthorityProjection derives the projection from disk.
func computeAuthorityProjection(cfg *config.Context, slug string) (authorityProjection, error) {
	return computeAuthorityProjectionTx(cfg, slug, false)
}

// computeAuthorityProjectionTx optionally ignores the in-flight-compaction
// finding.
//
// Only compaction's own capture and recovery pass true, and only after they
// have validated that the journal is theirs and about this feature. The
// finding is otherwise a real error that must reach every consumer — it is
// suppressed for exactly the operation that is resolving it, never globally,
// because a transaction may not hide the condition it created from everyone
// else while it decides what to do about it.
func computeAuthorityProjectionTx(cfg *config.Context, slug string, allowInFlightCompaction bool) (authorityProjection, error) {
	out := authorityProjection{
		ActiveVersions:    map[string]string{},
		SupersededIntents: map[string]string{},
		SupersededBy:      map[string][]string{},
		Evidence:          map[string]string{},
		Outputless:        map[string]bool{},
	}

	ca := computeCheckAmendments(cfg, slug)
	// Supersession VALIDITY is part of the projection, not merely its visible
	// consequences. Without this an already-dangling ledger could be compacted
	// (its errors are equally present either side, so the comparison passes),
	// and a newly created error would only register if it happened to change
	// one of the fields below. Warnings are not authority.
	for _, iss := range ca.Issues {
		if iss.Severity != "error" {
			continue
		}
		if allowInFlightCompaction && iss.Code == "amendment-compaction-incomplete" {
			continue
		}
		out.Errors = append(out.Errors, iss.Code+": "+iss.Message)
	}
	sort.Strings(out.Errors)
	for k, v := range ca.SupersededIntents {
		out.SupersededIntents[k] = v
	}
	for k, v := range ca.SupersededBy {
		cp := append([]string(nil), v...)
		sort.Strings(cp)
		out.SupersededBy[k] = cp
	}
	out.RetiredBy = ca.RetiredBy

	// Active founding intents: declared minus superseded.
	//
	// ONE ACQUISITION for the intents, their provenance, the marker and the
	// evidence. Composing those from separate observations let a concurrent
	// authority writer produce a projection whose parts never coexisted — and
	// the projection's whole job is to say two states are the same, which it
	// cannot do honestly if either state is a mixture.
	featDir := cfg.FeaturePath(slug)
	snap, serr := acquireAppliedLedger(cfg, slug, featDir)
	if serr != nil {
		return out, fmt.Errorf("read %s's applied history: %w", slug, serr)
	}
	intents, ierr := parser.ParseIntentsFile(filepath.Join(featDir, "intents.md"))
	if ierr != nil {
		return out, fmt.Errorf("read %s's founding promises: %w", slug, ierr)
	}
	res := resolveIntentsFrom(snap, intents, agent.AppliedAuthority)
	version, mode := intentProvenanceFrom(snap)
	for _, in := range res.Active {
		out.ActiveIntents = append(out.ActiveIntents, in.Slug)
		fp, ferr := intentVersionFingerprint(in)
		if ferr != nil {
			return out, ferr
		}
		by := version[in.Slug]
		if by == "" {
			by = "founding"
		}
		out.ActiveVersions[in.Slug] = fmt.Sprintf("%s|%s|%s", by, mode[in.Slug], fp)
	}
	sort.Strings(out.ActiveIntents)

	capsule := snap.Capsule
	out.AppliedThrough = snap.Through
	for _, a := range ca.Amendments {
		if a.Seq > capsule.Through {
			out.PendingTail = append(out.PendingTail, fmt.Sprintf("%03d-%s", a.Seq, a.Slug))
		}
	}
	sort.Strings(out.PendingTail)
	for k, v := range capsule.Hashes {
		out.Evidence[k] = v
	}
	for k, v := range capsule.Outputless {
		out.Outputless[k] = v
	}
	return out, nil
}

// canonical renders the projection as stable text so two of them compare by
// bytes. Ordering is normalised everywhere, because a map iteration order
// difference is not an authority difference and must not read as one.
func (p authorityProjection) canonical() string {
	var b strings.Builder
	fmt.Fprintf(&b, "applied-through: %d\n", p.AppliedThrough)
	fmt.Fprintf(&b, "retired-by: %s\n", p.RetiredBy)

	b.WriteString("active-intents:\n")
	for _, s := range p.ActiveIntents {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	b.WriteString("active-versions:\n")
	for _, k := range sortedKeys(p.ActiveVersions) {
		fmt.Fprintf(&b, "  %s: %s\n", k, p.ActiveVersions[k])
	}
	b.WriteString("pending-tail:\n")
	for _, s := range p.PendingTail {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	b.WriteString("superseded-intents:\n")
	for _, k := range sortedKeys(p.SupersededIntents) {
		fmt.Fprintf(&b, "  %s: %s\n", k, p.SupersededIntents[k])
	}
	b.WriteString("superseded-by:\n")
	for _, k := range sortedStringSliceKeys(p.SupersededBy) {
		fmt.Fprintf(&b, "  %s: %s\n", k, strings.Join(p.SupersededBy[k], ","))
	}
	b.WriteString("evidence:\n")
	for _, k := range sortedKeys(p.Evidence) {
		fmt.Fprintf(&b, "  %s: %s\n", k, p.Evidence[k])
	}
	b.WriteString("errors:\n")
	for _, e := range p.Errors {
		fmt.Fprintf(&b, "  - %s\n", e)
	}
	b.WriteString("outputless:\n")
	for _, k := range sortedBoolKeys(p.Outputless) {
		fmt.Fprintf(&b, "  %s: %t\n", k, p.Outputless[k])
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedStringSliceKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// intentVersionFingerprint hashes what a promise currently says.
//
// The whole snapshot, not a chosen subset: any field of a version can change
// what the promise means, and a projection that watches three of them is
// evidence about three of them. Through the storage encoding first, for the
// same reason every other digest here is — a value compared in two different
// shapes is not one value.
func intentVersionFingerprint(in parser.Intent) (string, error) {
	encoded, err := yaml.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("fingerprint the promise %q: %w", in.Slug, err)
	}
	var round parser.Intent
	if err := yaml.Unmarshal(encoded, &round); err != nil {
		return "", fmt.Errorf("fingerprint the promise %q: %w", in.Slug, err)
	}
	data, err := json.Marshal(round)
	if err != nil {
		return "", fmt.Errorf("fingerprint the promise %q: %w", in.Slug, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
