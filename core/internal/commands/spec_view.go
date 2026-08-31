package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
)

// parlay spec @feature — what the feature promises and provides RIGHT NOW.
//
// The requirement this answers is "the new state should also equal a new spec".
// Before this, a reader wanting the current specification had to open the frozen
// founding documents, read the whole amendment ledger, and apply it in their
// head — which is not a specification, it is the raw material for one. Worse, it
// is a job people do wrong: the founding text READS like current truth, so the
// natural failure is to trust it and never reach the amendments.
//
// TWO HALVES, AND THEY ARE NOT EQUALLY DERIVED. This is the honest scope, and
// the rendering says so rather than leaving a reader to assume uniformity:
//
//   - The PROMISES are projected. Each is the latest version its lineage
//     reached under the applied ledger, computed here and stored nowhere.
//   - The CONTRACT is a stored snapshot. The splice mutates capabilities.yaml
//     and its siblings in place, so what is shown is a file somebody wrote,
//     attributed to promises by its own source: fields.
//
// Full materialisation of the second half is a later stage and may never be
// worth it. Presenting a half-derived composite as though both halves were
// derived would be the more comfortable lie and the more expensive one.
//
// It also refuses to look authoritative when it is not. An unapplied ledger tail
// means the projection is already out of date with respect to decisions that
// have been made, and a ledger with errors means the projection rests on records
// the tool cannot read. Both are said at the top, where somebody reading for an
// answer will see them, not in a footnote.

var (
	specViewJSON bool
	specViewAt   string
)

var specCmd = &cobra.Command{
	Use:   "spec @<feature>",
	Short: "Show what a feature promises and provides right now",
	Long: `Render a feature's CURRENT specification.

The founding documents say what was decided at the beginning; the amendment
ledger says what has been decided since. Neither is the current specification on
its own, and reconstructing it by hand is both tedious and easy to get wrong —
the founding text reads like present truth, so the natural mistake is to stop
there.

The promises shown are projected: each is the latest version its lineage reached
under the amendments that have been applied. The contract shown is the stored
artifacts as they are on disk, attributed to the promises their source: fields
name. That difference is stated in the output, because the two halves carry
different guarantees.

Nothing is written. If the ledger has unapplied records or unresolved errors,
that is reported first — the projection is only as current as the ledger it
could read.

With --at, the promises are projected to an earlier point: what this feature
promised when that amendment was the last applied one. The contract half is
omitted there, and deliberately — the artifacts are a stored snapshot that the
splice edits in place, so there is no earlier version of them to show. Rendering
today's entries under yesterday's promises would put a date on something that
does not have one.

There is no --at <date>. What is recorded is which decisions are applied, never
when they became applied, so a date query could only answer "what had been
decided by then", which is a different question from the one the flag would
appear to answer.`,
	Args: cobra.ExactArgs(1),
	RunE: runSpecView,
}

func init() {
	specCmd.Flags().BoolVar(&specViewJSON, "json", false, "Emit the composite as JSON")
	specCmd.Flags().StringVar(&specViewAt, "at", "",
		"Show the promises as they stood when this amendment was the last applied one "+
			"(0 for the founding state, a sequence like 3, or an identity like 003-widen-timeout)")
}

// specPromise is one projected promise with its provenance.
type specPromise struct {
	Slug string `json:"slug"`
	// Version says which decision put this text in force: the founding
	// document, or the amendment that last changed it. Without it the reader
	// cannot tell a promise nobody has touched from one revised three times.
	Version string `json:"version,omitempty"`
	Mode    string `json:"mode,omitempty"`
	// Intent is the COMPLETE promise as it currently reads.
	//
	// Whole, not a title-and-goal summary. Stage 1 established that a version
	// is a snapshot: every field is current state, and omission there means the
	// field is ABSENT rather than inherited. A JSON form carrying two of ten
	// fields would answer a different question from the text form standing
	// beside it, and a consumer reading it would believe a promise had no
	// constraints when it has five.
	Intent specIntent `json:"promise"`
	// Entries is NULL when no contract is available, and an array — possibly
	// empty — when one is.
	//
	// A pointer precisely so those two are distinguishable. An empty array said
	// "this promise is known to justify nothing", which is a different fact from
	// "there is no contract to consult", and a historical view was asserting the
	// first while a prose note elsewhere said the second. A machine consumer
	// reads the structure, not the note.
	Entries     *[]specEntry `json:"entries"`
	Superseded  bool         `json:"superseded,omitempty"`
	SupersededB string       `json:"superseded_by,omitempty"`

	// intent is the parsed form the text renderer uses.
	intent parser.Intent
}

// specIntent is a promise's complete current text.
//
// Every field explicit and every list always present, so a cleared list — which
// under snapshot semantics means the promise no longer carries one — reads as
// an empty array rather than as an absent field a consumer might treat as
// unknown.
type specIntent struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Goal        string   `json:"goal"`
	Persona     string   `json:"persona"`
	Priority    string   `json:"priority"`
	Context     string   `json:"context"`
	Action      string   `json:"action"`
	Objects     []string `json:"objects"`
	Constraints []string `json:"constraints"`
	Verify      []string `json:"verify"`
	Questions   []string `json:"questions"`
}

func asSpecIntent(in parser.Intent) specIntent {
	list := func(v []string) []string {
		if v == nil {
			return []string{}
		}
		return v
	}
	return specIntent{
		Slug: in.Slug, Title: in.Title, Goal: in.Goal, Persona: in.Persona,
		Priority: in.Priority, Context: in.Context, Action: in.Action,
		Objects: list(in.Objects), Constraints: list(in.Constraints),
		Verify: list(in.Verify), Questions: list(in.Questions),
	}
}

// specEntry is one contract entry attributed to a promise.
type specEntry struct {
	Ref     string `json:"ref"`
	Kind    string `json:"kind"`
	Summary string `json:"summary,omitempty"`
	// Shared says this entry names more than one promise as its source, so a
	// reader who removes the promise above it does not thereby remove the
	// entry.
	Shared bool `json:"shared,omitempty"`
}

type specViewOutput struct {
	Feature string `json:"feature"`
	// AppliedThrough is how far the ledger has been folded into what runs.
	AppliedThrough int `json:"applied_through"`
	// At is the point this view is projected to. Equal to AppliedThrough for
	// the ordinary current view.
	At int `json:"at"`
	// Since names the applied decisions between At and AppliedThrough — what has
	// happened to this feature since the point being shown. Empty for the
	// current view.
	Since    []string      `json:"since,omitempty"`
	Promises []specPromise `json:"promises"`
	Retired  []specPromise `json:"retired"`
	// ContractStatus says whether there is a contract to report, and when there
	// is not, WHY.
	//
	// Three states, not two, because the two negative ones are different facts
	// and a consumer that conflates them draws opposite conclusions. A boolean
	// made "we read it and it is empty" indistinguishable from "we could not
	// read it", which is how the unreadable path came to encode known-empty
	// arrays under a banner saying the opposite.
	ContractStatus string `json:"contract_status"`
	// Unattributed are contract entries no live promise justifies. Reported
	// rather than hidden: an entry with no promise behind it is either an
	// orphan or a missing source:, and both are things a reader of "what is
	// true now" needs to see.
	// Null when no contract is available, for the same reason Entries is.
	Unattributed *[]specEntry `json:"unattributed"`
	// Derivation says how each half of this composite was produced. In the
	// JSON as well as the prose: a machine consumer must not assume uniform
	// derivation any more than a person should, and the prose paragraph that
	// says so is invisible to it.
	Derivation specDerivation `json:"derivation"`
	// Pending and Blocking are why the view might not be what the reader
	// thinks it is.
	Pending  []string `json:"pending,omitempty"`
	Blocking []string `json:"blocking,omitempty"`
}

// resolveSpecViewAt turns a --at argument into a sequence, checked against the
// ledger it will be resolved from.
//
// Only backwards. A sequence above the applied marker is not history, it is a
// proposal — what the feature WOULD promise if a pending record were applied,
// which is the apply ceremony's question and carries an approval with it. A
// read-only view must not answer it in the same breath as "what was true", or
// the two become indistinguishable in the reader's head.
func resolveSpecViewAt(snap appliedLedgerSnapshot, arg string) (int, error) {
	arg = strings.TrimSpace(arg)

	// 0 is the founding point, named explicitly: what this feature promised
	// before any amendment. It is the one value that does not identify a record
	// and still means something.
	if arg == "0" {
		return 0, nil
	}

	if n, err := strconv.Atoi(arg); err == nil {
		// A sequence must IDENTIFY a record. Accepting any integer up to the
		// marker meant that with records 1 and 3, `--at 2` silently invented a
		// boundary no amendment marks — the command says "when this amendment
		// was the last applied one", and there was no such amendment.
		var matches []parser.Amendment
		for _, a := range snap.Records {
			if a.Seq == n {
				matches = append(matches, a)
			}
		}
		return uniqueSpecViewAt(snap, arg, matches)
	}

	// Textual: the exact NNN-slug identity, or a bare slug where it is
	// unambiguous. Ambiguity is refused rather than resolved by sort order,
	// because "the first one" is not an answer anybody asked for.
	var matches []parser.Amendment
	for _, a := range snap.Records {
		if fmt.Sprintf("%03d-%s", a.Seq, a.FileSlug) == arg || a.FileSlug == arg {
			matches = append(matches, a)
		}
	}
	return uniqueSpecViewAt(snap, arg, matches)
}

// uniqueSpecViewAt turns exactly one matching record into a point.
func uniqueSpecViewAt(snap appliedLedgerSnapshot, arg string, matches []parser.Amendment) (int, error) {
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("--at %q names no amendment in this feature's ledger. Give 0 for "+
			"the founding state, a sequence like 3, or an identity like 003-widen-timeout", arg)
	case 1:
	default:
		var names []string
		for _, a := range matches {
			names = append(names, fmt.Sprintf("%03d-%s", a.Seq, a.FileSlug))
		}
		sort.Strings(names)
		return 0, fmt.Errorf("--at %q matches %d records (%s), so which point is meant is "+
			"ambiguous. Name one exactly", arg, len(matches), strings.Join(names, ", "))
	}
	seq := matches[0].Seq
	if seq > snap.Through {
		return 0, fmt.Errorf("--at %s is beyond what has been applied (%03d). That is not an "+
			"earlier state, it is a proposal — what the feature WOULD promise once that record "+
			"is applied, which is the apply ceremony's question and comes with an approval",
			arg, snap.Through)
	}
	return seq, nil
}

// retiredEntries gives a retired promise the same representation the rest of
// the view uses.
//
// nil is the sentinel for "no contract to consult", so a retired promise must
// not carry it while the view says globally that the contract IS available.
// A retired promise justifies nothing by definition — entries still naming it
// as their source appear under the unattributed heading — so the honest value
// is an empty array, which says exactly that.
func retiredEntries(readable bool) *[]specEntry {
	if !readable {
		return nil
	}
	e := []specEntry{}
	return &e
}

// The three states a contract half can be in.
const (
	// contractAvailable: enumerated successfully. Arrays are populated, and an
	// empty one is real knowledge — this promise justifies nothing.
	contractAvailable = "available"
	// contractHistorical: a past point was asked about. The artifacts are a
	// stored snapshot the splice edits in place, so there is no earlier version
	// of them to read.
	contractUnavailableHistorical = "unavailable_historical"
	// contractUnreadable: a current view whose artifacts exist and could not be
	// established. Distinct from historical because it is a defect somebody can
	// fix, and distinct from empty because nothing was learned.
	contractUnreadable = "unreadable"
)

// specDerivation records the guarantee behind each half of the composite.
type specDerivation struct {
	Promises string `json:"promises"`
	Contract string `json:"contract"`
}

func runSpecView(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featDir := cfg.FeaturePath(slug)

	// Empty slices, never nil: a consumer that iterates the JSON must not have
	// to distinguish "no promises" from "the field was absent".
	// ONE ACQUISITION for the whole view. The marker shown, the promises, their
	// provenance and the pending tail all come from the same authenticated
	// snapshot, so what is rendered is a state that existed rather than parts
	// of two.
	//
	// STRICT observation, not the fail-soft reader. lastAppliedAmendment folds
	// every error to 0, which is the conservative reading where the question is
	// "may this promise be retired" — but here the number is emitted as an
	// observed fact, and a corrupt baseline silently encoded as 0 would tell a
	// machine consumer that nothing has been applied. A banner elsewhere does
	// not repair a field that reads as data.
	snap, serr := acquireAppliedLedger(cfg, slug, featDir)
	if serr != nil {
		return fmt.Errorf("read %s's applied history: %w — how far the ledger has been applied "+
			"could not be established, and a specification that cannot say that is not a "+
			"specification of anything in particular", slug, serr)
	}

	// The historical point, if one was asked for. Resolved against the snapshot
	// so it is checked against the same authenticated ledger everything else
	// below is derived from.
	at := snap.Through
	if specViewAt != "" {
		var aerr error
		at, aerr = resolveSpecViewAt(snap, specViewAt)
		if aerr != nil {
			return aerr
		}
	}

	out := specViewOutput{
		Feature: slug, AppliedThrough: snap.Through, At: at,
		Promises: []specPromise{}, Retired: []specPromise{},
		Derivation: specDerivation{
			Promises: "projected: the latest version each lineage reached under the applied " +
				"ledger, computed on read and stored nowhere",
			Contract: "stored snapshot: the artifacts as they are on disk, attributed by their " +
				"own source: fields, which the splice edits in place",
		},
	}

	// The two reasons the projection may not be what it looks like, gathered
	// before anything is rendered.
	ca := computeCheckAmendments(cfg, slug)
	for _, iss := range ca.Issues {
		if iss.Severity == "error" {
			out.Blocking = append(out.Blocking, fmt.Sprintf("[%s] %s", iss.Code, iss.Message))
		}
	}
	sort.Strings(out.Blocking)

	intents, ierr := parser.ParseIntentsFile(filepath.Join(featDir, "intents.md"))
	if ierr != nil {
		return fmt.Errorf("read %s's founding promises: %w", slug, ierr)
	}
	res := resolveIntentsFrom(snap.viewAt(at), intents, agent.AppliedAuthority)

	// Where each promise's current text came from, and what has been decided
	// but not folded in yet.
	version, mode := intentProvenanceFrom(snap.viewAt(at))
	for _, a := range snap.Records {
		// The UNAPPLIED TAIL, taken from the ledger rather than from the
		// resolver's pending list. The resolver reports a pending transition
		// only where no applied version exists for that lineage, which is the
		// right question for what is in force and the wrong one here: a second
		// revision over an already-revised promise, and any record that touches
		// no promise at all, are both invisible to it, and both mean this view
		// is behind the decisions that have been made.
		if a.Seq > out.AppliedThrough {
			out.Pending = append(out.Pending, describeUnapplied(a))
			continue
		}
		// Between the historical point and now: applied, and deliberately not
		// reflected above. A reader looking at an earlier state needs to know
		// what has happened since, or they cannot tell whether it still holds.
		if a.Seq > at {
			out.Since = append(out.Since, describeApplied(a))
		}
	}
	sort.Strings(out.Pending)

	// The contract half. A derivation failure is reported, never silently
	// rendered as an empty contract: "this feature provides nothing" and "the
	// artifacts could not be read" are answers a reader must not confuse.
	// THE CONTRACT HALF HAS NO HISTORY. The artifacts are a stored snapshot the
	// splice edits in place, so there is no earlier version of them to show.
	// Rendering today's entries under an earlier promise set would attribute
	// present facts to a past state — the one thing a historical query must not
	// do, and the more tempting mistake because the output would look complete.
	historical := at != snap.Through
	if historical {
		out.Derivation.Contract = "omitted: the contract artifacts are a stored snapshot the " +
			"splice edits in place, so there is no earlier version of them to show"
	}

	var entries []contractEntry
	readable := false
	switch {
	case historical:
		out.ContractStatus = contractUnavailableHistorical
	default:
		var eerr error
		entries, eerr = enumerateContractEntries(featDir, slug)
		if eerr != nil {
			// UNREADABLE, not empty. Continuing with a nil entry list and then
			// normalising it to [] said the contract had been read and found to
			// hold nothing, while the banner above said it could not be read at
			// all — the same prose-cannot-undo-structure defect, one branch
			// over from the historical one.
			out.Blocking = append(out.Blocking, eerr.Error())
			out.ContractStatus = contractUnreadable
			break
		}
		readable = true
		out.ContractStatus = contractAvailable
	}
	summaries := entrySummaries(featDir, slug)
	byPromise := map[string][]specEntry{}
	var unattributed []specEntry
	for _, e := range entries {
		se := specEntry{Ref: e.Ref, Kind: refKind(e.Ref), Summary: summaries[e.Ref]}
		// Attribution uses sourceNamesIntent, which already owns this grammar
		// and owns it carefully: bare, feature-qualified and
		// initiative-qualified forms, with the lossy basename fallback confined
		// to the case where exactly one side is unqualified. An earlier version
		// here kept only the last path segment, which made
		// `@other-feature/check-readiness` display under the LOCAL promise of
		// the same name — the precise cross-feature confusion the shared helper
		// exists to avoid.
		var owners []string
		for _, in := range res.Active {
			if sourceNamesIntent(e.Sources, slug, in.Slug) {
				owners = append(owners, in.Slug)
			}
		}
		// Shared is about the SOURCE FIELD, not about how many local promises
		// matched: a reader deciding what removing this promise would cost
		// needs to know the entry is justified by something else as well, and
		// that something else may be in another feature.
		se.Shared = len(sourceRefCount(e.Sources)) > 1
		if len(owners) == 0 {
			unattributed = append(unattributed, se)
			continue
		}
		for _, owner := range owners {
			byPromise[owner] = append(byPromise[owner], se)
		}
	}
	sort.Slice(unattributed, func(i, j int) bool { return unattributed[i].Ref < unattributed[j].Ref })
	if readable {
		if unattributed == nil {
			unattributed = []specEntry{}
		}
		out.Unattributed = &unattributed
	}

	for _, in := range res.Active {
		p := specPromise{
			Slug: in.Slug, Intent: asSpecIntent(in), intent: in,
			Version: "founding document",
		}
		if readable {
			e := byPromise[in.Slug]
			if e == nil {
				e = []specEntry{}
			}
			sort.Slice(e, func(i, j int) bool { return e[i].Ref < e[j].Ref })
			p.Entries = &e
		}
		if v, ok := version[in.Slug]; ok {
			p.Version, p.Mode = v, mode[in.Slug]
		}
		out.Promises = append(out.Promises, p)
	}
	for _, s := range res.Superseded {
		out.Retired = append(out.Retired, specPromise{
			Slug: s.Intent.Slug, Intent: asSpecIntent(s.Intent), intent: s.Intent,
			Superseded: true, SupersededB: fmt.Sprintf("%03d-%s", s.Seq, s.ByAmendment),
			Mode: string(s.Mode), Entries: retiredEntries(readable),
		})
	}

	if specViewJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	writeSpecView(cmd.OutOrStdout(), out)
	return nil
}

// sourceRefCount returns the distinct refs a source: field names.
//
// Distinct rather than raw count: the same entry written twice is one
// justification, and reporting it as shared would tell a reader something else
// supports it when nothing does.
func sourceRefCount(src string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(src, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func refKind(ref string) string {
	if i := strings.Index(ref, "/"); i >= 0 {
		rest := ref[i+1:]
		if j := strings.Index(rest, ":"); j >= 0 {
			return rest[:j]
		}
	}
	return ""
}

// describeApplied says what a decision made SINCE the point being shown did.
func describeApplied(a parser.Amendment) string {
	return strings.TrimSuffix(describeUnapplied(a), ", and has not been applied")
}

// describeUnapplied says what a recorded-but-unapplied decision would do.
//
// By what it CHANGES, not by its slug: a reader deciding whether the view below
// is good enough for their purpose needs to know whether the thing waiting is a
// reworded promise or a retired one.
func describeUnapplied(a parser.Amendment) string {
	// Grouped by verb. A record ending three promises said "ends X and ends Y
	// and ends Z", which buries the one fact a reader wants — that three
	// promises ended — under a repetition of the verb.
	var verbs []string
	byVerb := map[string][]string{}
	for _, tr := range a.IntentTransitions() {
		v := pendingVerbFor(tr.Mode)
		if _, seen := byVerb[v]; !seen {
			verbs = append(verbs, v)
		}
		byVerb[v] = append(byVerb[v], fmt.Sprintf("%q", tr.Intent))
	}
	var parts []string
	for _, v := range verbs {
		parts = append(parts, fmt.Sprintf("%s %s", v, strings.Join(byVerb[v], ", ")))
	}
	if len(a.Affects) > 0 {
		parts = append(parts, fmt.Sprintf("changes %d contract entr%s",
			len(a.Affects), plural(len(a.Affects), "y", "ies")))
	}
	what := "is recorded"
	if len(parts) > 0 {
		what = strings.Join(parts, " and ")
	}
	return fmt.Sprintf("%03d-%s %s, and has not been applied", a.Seq, a.FileSlug, what)
}

// pendingVerbFor phrases an unapplied transition for a reader.
func pendingVerbFor(m parser.IntentMode) string {
	switch m {
	case parser.IntentExtend:
		return "extends"
	case parser.IntentRevise:
		return "revises"
	case parser.IntentNarrow:
		return "narrows"
	case parser.IntentRetire:
		return "retires"
	case parser.IntentLegacySupersession:
		return "ends (legacy record, meaning never stated)"
	}
	return "changes"
}

func writeSpecView(w io.Writer, out specViewOutput) {
	if out.At != out.AppliedThrough {
		// Named as history in the first line, because everything below it is
		// answering a different question from the one this command usually
		// answers, and a reader who skims the body will act on it either way.
		fmt.Fprintf(w, "%s — specification AS IT STOOD at amendment %03d\n", out.Feature, out.At)
		fmt.Fprintf(w, "This is history. The feature is applied through %03d today.\n",
			out.AppliedThrough)
	} else {
		fmt.Fprintf(w, "%s — current specification\n", out.Feature)
		fmt.Fprintf(w, "Applied through amendment %03d.\n", out.AppliedThrough)
	}

	// First, and not in a footnote. A reader here wants an answer, and both of
	// these mean the answer on screen is not the whole one.
	if len(out.Blocking) > 0 {
		fmt.Fprintf(w, "\n!! The ledger has %d unresolved error(s), so this projection rests on\n",
			len(out.Blocking))
		fmt.Fprintln(w, "   records the tool could not fully read:")
		for _, b := range out.Blocking {
			fmt.Fprintf(w, "     - %s\n", b)
		}
	}
	if len(out.Pending) > 0 {
		fmt.Fprintln(w, "\n!! Decisions have been recorded but not applied. They are NOT reflected")
		fmt.Fprintln(w, "   below — what follows is what the code currently answers to:")
		for _, p := range out.Pending {
			fmt.Fprintf(w, "     - %s\n", p)
		}
	}

	if len(out.Since) > 0 {
		fmt.Fprintln(w, "\nDecided since, and NOT reflected below:")
		for _, sn := range out.Since {
			fmt.Fprintf(w, "  - %s\n", sn)
		}
	}

	fmt.Fprintf(w, "\n═══ Promises (projected from the ledger) ═══\n")
	if len(out.Promises) == 0 {
		fmt.Fprintln(w, "\n  (this feature makes no promise that is still in force)")
	}
	for _, p := range out.Promises {
		fmt.Fprintf(w, "\n── %s ──\n", p.Slug)
		if p.Mode != "" {
			fmt.Fprintf(w, "  current text from %s (%s)\n\n", p.Version, p.Mode)
		} else {
			fmt.Fprintf(w, "  current text from the %s\n\n", p.Version)
		}
		writeWholeIntent(w, "    ", p.intent)
		// No provides: block at all when there is no contract to consult.
		// Printing "(nothing in the contract names this promise)" there stated
		// a fact — known to provide nothing — that the view goes on to say it
		// cannot know, and the reader meets the false one first.
		if p.Entries == nil {
			continue
		}
		fmt.Fprintf(w, "\n    provides:\n")
		if len(*p.Entries) == 0 {
			fmt.Fprintln(w, "      (nothing in the contract names this promise)")
		}
		for _, e := range *p.Entries {
			shared := ""
			if e.Shared {
				shared = "   [also justified by another promise]"
			}
			fmt.Fprintf(w, "      %s%s\n", e.Ref, shared)
			if e.Summary != "" {
				fmt.Fprintf(w, "        %s\n", e.Summary)
			}
		}
	}

	if out.ContractStatus == contractUnavailableHistorical {
		fmt.Fprintf(w, "\n═══ What this feature provided then ═══\n\n")
		fmt.Fprintln(w, "  Not shown, and not omitted for brevity. The contract artifacts are a")
		fmt.Fprintln(w, "  stored snapshot that the splice edits in place, so there is no earlier")
		fmt.Fprintln(w, "  version of them to read. Showing today's entries under these promises")
		fmt.Fprintln(w, "  would put a date on something that does not have one.")
	}

	if out.Unattributed != nil && len(*out.Unattributed) > 0 {
		fmt.Fprintf(w, "\n═══ Contract entries no live promise justifies ═══\n\n")
		for _, e := range *out.Unattributed {
			fmt.Fprintf(w, "  %s\n", e.Ref)
			if e.Summary != "" {
				fmt.Fprintf(w, "    %s\n", e.Summary)
			}
		}
	}

	if len(out.Retired) > 0 {
		fmt.Fprintf(w, "\n═══ Promises this feature no longer makes ═══\n")
		for _, p := range out.Retired {
			// The MODE matters to a reader, and the legacy spelling most of
			// all: a `legacy_supersession` records that a promise ended without
			// recording what the author meant by ending it, and presenting that
			// as an ordinary retirement would claim knowledge nobody has.
			how := ""
			switch {
			case p.Mode == string(parser.IntentLegacySupersession):
				how = " (legacy record — what this ending meant was never stated)"
			case p.Mode != "":
				how = fmt.Sprintf(" (%s)", p.Mode)
			}
			fmt.Fprintf(w, "\n  %s — ended by %s%s\n", p.Slug, p.SupersededB, how)
			goal := p.Intent.Goal
			if strings.TrimSpace(goal) == "" {
				goal = "(no goal was recorded for this promise)"
			}
			fmt.Fprintf(w, "    was: %s\n", goal)
		}
	}

	// The scope of the claim, stated. Both halves are current; they are not
	// current in the same way, and a reader who assumes they are will trust a
	// hand-edited file as though the tool had derived it.
	fmt.Fprintln(w, "\n───")
	if out.At != out.AppliedThrough {
		fmt.Fprintln(w, "The promises above are PROJECTED to an earlier point: each is the latest")
		fmt.Fprintln(w, "version its lineage had reached by then. Versions are snapshots rather")
		fmt.Fprintln(w, "than patches, which is the only reason this is answerable at all.")
		return
	}
	fmt.Fprintln(w, "The promises above are PROJECTED: each is the latest version its lineage")
	fmt.Fprintln(w, "reached under the applied ledger, computed on read and stored nowhere.")
	fmt.Fprintln(w, "The contract entries are a STORED SNAPSHOT: the artifacts as they are on")
	fmt.Fprintln(w, "disk, attributed by their own source: fields. The splice edits those files")
	fmt.Fprintln(w, "in place, so they carry a weaker guarantee than the promises do.")
}

// entrySummaries reads the human-readable line for each contract entry.
//
// From the RAW nodes, not the parsed structs: parser.CapabilityOperation has no
// Summary field, so an operation's one-line description — the part a reader of a
// specification actually wants — is invisible to the parser. That gap is already
// documented where it matters most (a fingerprint over the parsed struct missed
// `summary` entirely, which was a live approval bypass); here it just means the
// summary has to come from the document.
//
// Best effort by design. A missing or unreadable artifact costs a description,
// never a row: the entry list itself comes from the fail-closed enumeration, and
// this only decorates it.
func entrySummaries(featDir, slug string) map[string]string {
	out := map[string]string{}
	readList := func(file, listKey, idKey, kind string, textKeys ...string) {
		data, err := os.ReadFile(filepath.Join(featDir, file))
		if err != nil {
			return
		}
		var doc yaml.Node
		if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 {
			return
		}
		root := doc.Content[0]
		if root.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value != listKey || root.Content[i+1].Kind != yaml.SequenceNode {
				continue
			}
			for _, item := range root.Content[i+1].Content {
				if item.Kind != yaml.MappingNode {
					continue
				}
				var id, text string
				for j := 0; j+1 < len(item.Content); j += 2 {
					key, val := item.Content[j].Value, item.Content[j+1].Value
					if key == idKey {
						id = val
					}
					for _, tk := range textKeys {
						if key == tk && text == "" {
							text = val
						}
					}
				}
				if id == "" {
					continue
				}
				name := id
				if kind == "surface" {
					name = parser.Slugify(id)
				}
				out[fmt.Sprintf("@%s/%s:%s", slug, kind, name)] = text
			}
		}
	}
	readList("capabilities.yaml", "operations", "id", "operation", "summary", "description")
	readList("surface.yaml", "fragments", "name", "surface", "summary", "description")

	// Infrastructure is prose, not a keyed list, so it needs its own reader —
	// and it needs one. enumerateContractEntries includes infrastructure
	// fragments, so leaving them out here is not a missing artifact costing a
	// description; it is a whole KIND rendering as a bare address in a document
	// whose purpose is to be read. Its heading is the description: that is what
	// the fragment is named by and what a reader recognises it as.
	if _, _, frags, err := readFragments(filepath.Join(featDir, "infrastructure.md")); err == nil {
		for _, f := range frags {
			ref := fmt.Sprintf("@%s/infrastructure:%s", slug, fragmentSlug(f.heading))
			out[ref] = strings.TrimSpace(strings.TrimLeft(f.heading, "# "))
		}
	}
	return out
}
