// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: backlog-item

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/ledger"
	"github.com/ddwht/parlay/core/internal/parser"
)

// `parlay promote` turns an observation into work the pipeline can carry.
//
// This is the only exit from the backlog that produces something rather
// than closing something, and it is what keeps the inbox from being a
// place notes go to die. The two shapes are genuinely different acts:
// a new feature is a promise the project has not made yet, and an
// amendment is a change to one it already has.
//
// The item is RETAINED as provenance, never moved and never duplicated
// into active requirements. Months later the question "where did this
// feature come from" has an answer, and the answer is a record nobody
// had to remember to keep.
var promoteCmd = &cobra.Command{
	Use:   "promote <item>",
	Short: "Turn a backlog item into a feature or an amendment",
	Long: `Promote one observation into work the pipeline carries.

  parlay promote <item> --as-feature <name> [--initiative X]
  parlay promote <item> --as-amendment @feature

--as-feature writes the standard zero-intent scaffold and does NOT seed a
Goal. An implementation observation is usually not a user-world outcome and
has no Persona, so seeding one manufactures exactly the malformed intent the
scaffold warns against — and the feature would then parse as having an intent
it does not really have, which is the state the ` + "`planned`" + ` phase exists to
distinguish. The scaffold carries a non-parsing backlog-origin link instead,
and the intents phase translates the evidence into a real promise.

--as-amendment does not write the amendment. It emits the pre-filled
` + "`trigger: backlog:<id>`" + ` for /parlay-refine, because an amendment is
authored with a person in the loop and a command that wrote one alone would
be recording a decision nobody made.

A PRIORITY IS PASSED THROUGH AS A PROPOSAL, never as a decided rank. A debt
item ranked for its blast radius in the codebase is exactly the case the
intents phase must re-judge against the user outcome rather than copy.

The item is retained as provenance. Nothing is moved and nothing is
duplicated into active requirements.`,
	Args: cobra.ExactArgs(1),
	RunE: runPromote,
}

var (
	promoteAsFeature   string
	promoteAsAmendment string
	promoteInitiative  string
	promoteBy          string
)

func init() {
	f := promoteCmd.Flags()
	f.StringVar(&promoteAsFeature, "as-feature", "", "create a new feature with this name")
	f.StringVar(&promoteAsAmendment, "as-amendment", "", "target an existing @feature with an amendment")
	f.StringVar(&promoteInitiative, "initiative", "", "place the new feature under this initiative")
	f.StringVar(&promoteBy, "by", "", "who promoted it (required)")
}

func runPromote(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	feature := strings.TrimSpace(promoteAsFeature)
	amendment := strings.TrimSpace(promoteAsAmendment)

	switch {
	case feature == "" && amendment == "":
		return fmt.Errorf("pass --as-feature <name> or --as-amendment @feature: promotion has to say what the work becomes, and `becomes:` is required on the event it records")
	case feature != "" && amendment != "":
		return fmt.Errorf("--as-feature and --as-amendment are different acts: one makes a promise the project has not made, the other changes one it has. Pick one")
	}
	if promoteInitiative != "" && feature == "" {
		return fmt.Errorf("--initiative places a new feature, so it only applies with --as-feature")
	}
	if strings.TrimSpace(promoteBy) == "" {
		return fmt.Errorf("--by is required: promotion is a decision, and a decision nobody can attribute is one nobody can review")
	}

	item, path, err := resolveBacklogItem(cfg, args[0])
	if err != nil {
		return err
	}
	// A closed item cannot be promoted. The schema allows at most one
	// terminal event and it must be last, so appending would produce a
	// record the validator refuses — better to say why here than to let
	// the write fail with a shape error the caller cannot act on.
	if item.State() != parser.StateOpen {
		return fmt.Errorf("%s is already %s: a closed item cannot be promoted. If the decision has changed, that is a new observation", shortID(item.ID), item.State())
	}

	if amendment != "" {
		// No guard: this writes nothing and closes nothing.
		return promoteAsAmendmentTo(cmd, cfg, item, path, amendment)
	}
	return promoteFeature(cmd, cfg, args[0], feature, promoteInitiative, strings.TrimSpace(promoteBy))
}

// promoteFeature is the whole --as-feature decision, taking its inputs
// as ARGUMENTS rather than reading the package flag globals.
//
// The flags stay where cobra needs them; this exists so the decision can
// be exercised the way it actually behaves — concurrently — without two
// callers racing on shared variables that a real single-process
// invocation never contends for. A concurrency test written against the
// flag-reading entry point measures the test's own data race instead of
// the guard it is trying to check.
func promoteFeature(cmd *cobra.Command, cfg *config.Context, itemRef, name, initiative, by string) error {
	// RESOLVE FIRST, then lock by the CANONICAL id.
	//
	// The guard used to be keyed on the caller's raw ref, while every
	// other terminal verb resolves first and locks by full id. An item
	// addressed once by full id and once by its accepted short prefix
	// therefore took two different locks and did not contend at all —
	// the guard was there and the race was still open.
	resolved, _, err := resolveBacklogItem(cfg, itemRef)
	if err != nil {
		return err
	}
	// Held across deciding, scaffolding AND recording — see
	// withBacklogDecisionLock. Checking open state outside a lock and
	// then doing work on the strength of it is how two concurrent
	// promotions scaffold two features for one observation.
	return withBacklogDecisionLock(cfg, resolved.ID, func() error {
		// Re-read INSIDE the guard: the state that authorises the work
		// must be the state at the moment the work starts.
		current, curPath, rerr := resolveBacklogItem(cfg, resolved.ID)
		if rerr != nil {
			return rerr
		}
		if current.State() != parser.StateOpen {
			return fmt.Errorf("%s is already %s: a closed item cannot be promoted. If the decision has changed, that is a new observation", shortID(current.ID), current.State())
		}
		return promoteAsNewFeature(cmd, cfg, current, curPath, name, initiative, by)
	})
}

// promoteAsNewFeature scaffolds first and records second.
//
// ORDER MATTERS and this is the whole reason it is written out. The
// scaffold is a sequence of directory and file writes, not an atomic
// multi-file transaction, so it can fail halfway. Recording `promoted`
// first would leave an item closed against a feature that does not
// exist — an observation lost with a receipt saying it was handled.
// Recording second leaves, in the worst case, a scaffolded feature and
// an item still open, which is visible, repairable, and reported.
//
// CALLED UNDER withBacklogDecisionLock, and the "still open" claim in
// the errors below is only true because of that. Without the guard, a
// concurrent decline could close the item while this scaffolds, and the
// message would be telling the caller something false at the exact
// moment they most need it to be true.
func promoteAsNewFeature(cmd *cobra.Command, cfg *config.Context, item *parser.BacklogItem, path, name, initiative, by string) error {
	slug := parser.Slugify(name)
	ref := "@" + slug
	if initiative != "" {
		ref = "@" + parser.Slugify(initiative) + "/" + slug
	}

	// Reuse add-feature rather than reimplement the scaffold. A second
	// copy of the three-tree layout is a second thing to keep in step
	// with the tree it is supposed to mirror.
	// A SECOND guard, on the TARGET.
	//
	// The decision guard is per ITEM, so it does not coordinate two
	// different items promoting to the same feature name. Both could
	// pass the existence check before either created the directory, and
	// appendBacklogOrigin is a read-then-replace, so one origin could be
	// silently lost — atomic replacement prevents truncation, not lost
	// updates.
	//
	// THE GUARANTEE: target creation is EXCLUSIVE, and interrupted
	// promotions are RESUMABLE. One promotion wins a contested target;
	// the loser's item stays open and is told the feature exists, because
	// whether two observations are the same work is a judgment for a
	// person. But a target that exists because THIS item's own promotion
	// crashed halfway is not a contested target at all, and refusing it
	// would strand the item permanently — the scaffold is a sequence of
	// writes, not a transaction, so that window is ordinary rather than
	// exotic.
	//
	// LOCK ORDER: decision-guard (per item) then target-guard (per
	// feature). Never the reverse.
	featurePath := featurePathFor(cfg, ref)
	if err := withPromotionTargetLock(cfg, ref, func() error {
		// The RESERVATION is written before anything is created, and it
		// is what makes recovery possible. Existence of a directory
		// cannot distinguish "our interrupted scaffold" from "somebody
		// else's feature", and a half-written scaffold may carry no
		// origin comment to go on — so the claim on the target is
		// recorded durably before the claim is acted on.
		holder, err := readPromotionReservation(cfg, ref)
		if err != nil {
			return err
		}
		if holder != "" && holder != item.ID {
			// Stale if the holder is no longer open: that promotion
			// either finished or was abandoned, and either way the
			// reservation is not protecting anything. An error here is
			// NOT stale — see reservationIsStale.
			stale, serr := reservationIsStale(cfg, holder)
			if serr != nil {
				return serr
			}
			if stale {
				holder = ""
			}
		}
		if holder != "" && holder != item.ID {
			return fmt.Errorf("%s is being promoted to %s by %s. %s stays open",
				shortID(holder), ref, shortID(holder), shortID(item.ID))
		}

		if holder == "" {
			// No reservation. If the target already exists it belongs to
			// somebody — unless its own intents.md names this item,
			// which is the pre-reservation crash window.
			if _, statErr := os.Stat(featurePath); statErr == nil {
				owner, oerr := scaffoldOriginID(featurePath)
				if oerr != nil {
					return oerr
				}
				if owner != item.ID {
					who := "another promotion"
					if owner != "" {
						who = shortID(owner) + "'s promotion"
					}
					return fmt.Errorf("%s already exists — %s created it. %s stays open: whether these are the same work is a judgment for a person, so decide it and either fold this item or promote it to its own feature",
						ref, who, shortID(item.ID))
				}
			}
		}

		if err := writePromotionReservation(cfg, ref, item.ID); err != nil {
			return err
		}

		// IDEMPOTENT from here, and the scaffold runs EVERY time.
		//
		// It used to run only when intents.md was absent, using that one
		// file as a completion sentinel — but the scaffold creates three
		// tree directories and two files, so a crash after intents.md
		// and before dialogs.md, or before the handoff and build trees
		// existed, left a tree the retry then skipped entirely and
		// recorded `promoted` against. No single file marks the scaffold
		// complete; the conditional scaffold is what makes it so, since
		// every step inside it is already skip-if-present.
		if aerr := scaffoldFeature(cmd, cfg, name, initiative, false, true); aerr != nil {
			return fmt.Errorf("scaffolding %s: %w", ref, aerr)
		}
		owner, oerr := scaffoldOriginID(featurePath)
		if oerr != nil {
			return oerr
		}
		if owner == "" {
			// The backlog-origin link is NON-PARSING on purpose. It has
			// to survive in the file a person reads while being
			// invisible to the intents parser, because a promoted
			// feature that parsed as having an intent would report
			// itself further along than it is — and `planned` exists
			// precisely to tell those apart.
			return appendBacklogOrigin(featurePath, item)
		}
		if owner != item.ID {
			return fmt.Errorf("%s carries %s's origin, not %s's — refusing to write a second one", ref, shortID(owner), shortID(item.ID))
		}
		return nil
	}); err != nil {
		return fmt.Errorf("%w (the item is untouched and still open; re-run to resume)", err)
	}

	// FAILURE INJECTION SEAM. Nil in production; set only by tests, to
	// interrupt a real promotion between the filesystem work and the
	// terminal event. The window it stands in is the ordinary one — the
	// scaffold is a sequence of writes, not a transaction — and without
	// a way to enter it deliberately, a test can only simulate the
	// aftermath, which proves nothing about how the aftermath arose.
	if promotionFailAfterScaffold != nil {
		if err := promotionFailAfterScaffold(); err != nil {
			return err
		}
	}

	event := parser.BacklogEvent{
		Event:   parser.EventPromoted,
		Becomes: ref,
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		By:      by,
	}
	// Through mutateBacklogItem, which re-reads and re-validates under
	// the lock, so the open-state check above is re-made against the
	// bytes actually on disk rather than against what we read earlier.
	if err := mutateBacklogItem(cfg, path, func(cur *parser.BacklogItem) error {
		if cur.State() != parser.StateOpen {
			return fmt.Errorf("%s became %s while the feature was being scaffolded", shortID(cur.ID), cur.State())
		}
		cur.History = append(cur.History, event)
		return nil
	}); err != nil {
		return fmt.Errorf("scaffolded %s but could not record the promotion on %s: %w — the feature exists and the item is still open; re-run to resume", ref, shortID(item.ID), err)
	}

	// The reservation is released only after the terminal event lands.
	// A failure to clear it is NOT a failure of the promotion: the work
	// is done and recorded, and a stale reservation is detected as stale
	// by the next promotion that wants this target.
	clearPromotionReservation(cfg, ref)

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nPromoted %s -> %s\n", shortID(item.ID), ref)
	fmt.Fprintf(out, "  The item is retained as provenance; it is not duplicated into the feature.\n")
	if item.Priority != "" {
		fmt.Fprintf(out, "  PROPOSED priority %s — the intents phase confirms or re-judges it against the user outcome. It is not a decided rank.\n", item.Priority)
	}
	fmt.Fprintf(out, "  No Goal was seeded. Write the promise in intents.md: parlay create-dialogs %s\n", ref)
	return nil
}

// promoteAsAmendmentTo emits the trigger and records nothing yet.
//
// It deliberately does not write the amendment. An amendment is authored
// with a person in the loop — /parlay-refine gates the text before it
// lands, and an amendment is written once and never edited — so a
// command that wrote one alone would be recording a decision nobody
// made. What it CAN do is close the causal gap the amendment schema
// already describes: `trigger: backlog:<id>` makes "which amendments came
// from things we noticed while building" a question the project can
// actually answer.
func promoteAsAmendmentTo(cmd *cobra.Command, cfg *config.Context, item *parser.BacklogItem, path, target string) error {
	// BARE FEATURE FORM ONLY. ValidateAboutRef also accepts a contract
	// ref like @widget/operation:x, and BareAboutFeature then resolves
	// that to "widget" — so a syntactically wrong target was silently
	// accepted and echoed back as though it were the feature. The flag
	// says "an existing @feature", so it has to mean that.
	if err := parser.ValidateAboutRef(target); err != nil {
		return fmt.Errorf("--as-amendment %q: %w", target, err)
	}
	if strings.Contains(target, ":") {
		return fmt.Errorf("--as-amendment takes a FEATURE, not a contract entry: %q names an entry inside @%s. An amendment is authored against the feature and declares the entries it affects itself",
			target, parser.BareAboutFeature(target))
	}
	slug := parser.BareAboutFeature(target)
	featurePath := cfg.FeaturePath(slug)
	if _, err := os.Stat(featurePath); err != nil {
		// Named rather than created. An amendment amends a promise that
		// exists; if the feature does not, the act is --as-feature.
		return fmt.Errorf("no feature %s at %s — an amendment changes a promise the project already made. If this is a new promise, use --as-feature", target, featurePath)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Ready to amend %s from %s.\n\n", target, shortID(item.ID))
	fmt.Fprintf(out, "Nothing has been written. Run:\n\n")
	fmt.Fprintf(out, "  /parlay-refine \"%s\" %s\n\n", item.Title, target)
	fmt.Fprintf(out, "and put this in the amendment's frontmatter:\n\n")
	fmt.Fprintf(out, "  trigger: backlog:%s\n\n", item.ID)
	if item.Body != "" {
		fmt.Fprintf(out, "The observation, in full:\n  %s\n\n", strings.ReplaceAll(item.Body, "\n", "\n  "))
	}
	if len(item.Evidence) > 0 {
		fmt.Fprintf(out, "Evidence:\n")
		for _, e := range item.Evidence {
			if e.Line > 0 {
				fmt.Fprintf(out, "  - %s:%d\n", e.Path, e.Line)
				continue
			}
			fmt.Fprintf(out, "  - %s\n", e.Path)
		}
		fmt.Fprintln(out)
	}
	if item.Priority != "" {
		fmt.Fprintf(out, "PROPOSED priority %s — for the refinement to confirm, not to inherit.\n\n", item.Priority)
	}
	// Recorded by /parlay-refine once the amendment exists, so the
	// `becomes:` names a real amendment rather than an intention.
	// `amend`, not `fold`. fold --into resolves another BACKLOG ITEM id
	// and rejects every amendment ref, so the command this used to
	// print could not be run at all. The two events are also
	// deliberately distinct: folded means the work merged into another
	// observation, amended means it landed as a change to a promise.
	fmt.Fprintf(out, "The item stays OPEN until the amendment lands. Then close it with:\n")
	fmt.Fprintf(out, "  parlay backlog amend %s --into %s/NNN-<slug> --by %s\n",
		item.ID, target, strings.TrimSpace(promoteBy))
	return nil
}

// appendBacklogOrigin adds the non-parsing provenance link.
//
// An HTML comment, so the intents parser cannot see it. The alternative
// — a real heading — would make the scaffolded feature parse as having
// an intent it does not have, and every phase downstream would read it
// as further along than it is.
func appendBacklogOrigin(featurePath string, item *parser.BacklogItem) error {
	intents := filepath.Join(featurePath, "intents.md")
	current, err := os.ReadFile(intents)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.Write(current)
	b.WriteString("\n<!--\nbacklog-origin: ")
	b.WriteString(item.ID)
	b.WriteString("\n\n")
	b.WriteString(item.Title)
	if item.Body != "" {
		b.WriteString("\n\n")
		b.WriteString(item.Body)
	}
	for _, e := range item.Evidence {
		if e.Line > 0 {
			fmt.Fprintf(&b, "\n\nEvidence: %s:%d", e.Path, e.Line)
			continue
		}
		fmt.Fprintf(&b, "\n\nEvidence: %s", e.Path)
	}
	if item.Priority != "" {
		fmt.Fprintf(&b, "\n\nProposed priority: %s — confirm or re-judge it against the user outcome; it is not inherited.", item.Priority)
	}
	b.WriteString("\n\nThis is the observation, not the promise. Write the promise above.\n-->\n")
	// Atomic. This is a read-modify-write of a founding document, and a
	// raw WriteFile can truncate it on a crash — leaving the feature
	// with a half-written intents.md that then freezes as its founding
	// state at first build.
	return atomicfile.WriteAtomic(intents, []byte(b.String()))
}

// featurePathFor is the directory a promotion ref names.
func featurePathFor(cfg *config.Context, ref string) string {
	return cfg.FeaturePath(strings.TrimPrefix(ref, "@"))
}

// withPromotionTargetLock serialises creation of ONE feature name.
//
// Per target rather than per item, because the thing being protected is
// the feature directory and the origin file inside it, and two different
// items can name the same target. Taken INSIDE the decision guard;
// see the lock-order note there.
func withPromotionTargetLock(cfg *config.Context, ref string, fn func() error) error {
	name := ".promote-" + strings.NewReplacer("/", "_", "@", "", string(os.PathSeparator), "_").Replace(ref)
	guard := ledger.New(cfg.Root.Path, filepath.Join(parser.BacklogRoot(cfg.Root.Path), name))
	var inner error
	if err := guard.Update(func([]byte, bool) ([]byte, bool, error) {
		inner = fn()
		// Never write: the guard file is a lock name, not a record.
		return nil, false, nil
	}); err != nil {
		return err
	}
	return inner
}

// promotionFailAfterScaffold is the injection point above. Package-level
// and nil by default; tests set and restore it.
var promotionFailAfterScaffold func() error

// ---------------------------------------------------------------------
// Promotion reservations — the durable half of resumability.
//
// In .parlay/, which is tool-internal and never user-facing, because a
// reservation is bookkeeping about an in-flight operation rather than
// anything a designer reads or decides on.
// ---------------------------------------------------------------------

func promotionReservationPath(cfg *config.Context, ref string) string {
	name := strings.NewReplacer("/", "__", "@", "", string(os.PathSeparator), "__").Replace(ref)
	return filepath.Join(cfg.Root.Path, config.ParlayDir, "promotions", name+".reservation")
}

// readPromotionReservation returns the item id holding this target, or
// empty if nobody holds it.
// Through the LEDGER STORE, not atomicfile.
//
// atomicfile fsyncs the temp file and renames, but does not fsync the
// PARENT DIRECTORY — so after a power loss the rename itself can be
// missing and the reservation with it, which is the one thing this
// record must survive. The ledger store was built precisely to hold a
// rename through a directory fsync and to name an ambiguous
// post-publication failure rather than swallow it. Calling the
// reservation durable while writing it the weaker way was a claim the
// code did not support.
func promotionReservationStore(cfg *config.Context, ref string) *ledger.Store {
	return ledger.New(cfg.Root.Path, promotionReservationPath(cfg, ref))
}

func readPromotionReservation(cfg *config.Context, ref string) (string, error) {
	data, exists, err := promotionReservationStore(cfg, ref).Read()
	if err != nil {
		return "", fmt.Errorf("read promotion reservation for %s: %w", ref, err)
	}
	if !exists {
		return "", nil
	}
	return strings.TrimSpace(string(data)), nil
}

func writePromotionReservation(cfg *config.Context, ref, itemID string) error {
	return promotionReservationStore(cfg, ref).Update(func([]byte, bool) ([]byte, bool, error) {
		return []byte(itemID + "\n"), true, nil
	})
}

// clearPromotionReservation is best-effort by design, and it is NOT
// crash-durable — recovery is, cleanup is not, and the asymmetry is
// deliberate. A stale reservation is recoverable (the next promotion
// wanting that target detects the holder as closed and proceeds); an
// unrecorded promotion is not. So this must never be able to fail the
// operation that already succeeded, and a crash between the terminal
// event and this call leaves a reservation behind on purpose rather
// than by oversight.
func clearPromotionReservation(cfg *config.Context, ref string) {
	_ = os.Remove(promotionReservationPath(cfg, ref))
}

// reservationIsStale reports whether the holder has stopped being a
// reason to refuse — it is closed, or it no longer exists at all.
func reservationIsStale(cfg *config.Context, holder string) (bool, error) {
	// UNREADABLE FIRST. loadBacklog carries an unparseable item in
	// `broken` rather than in `items`, so a corrupt record and a deleted
	// one both surface as not-found from resolveBacklogItem — and
	// treating that as "definitely gone" would hand the target away at
	// exactly the moment ownership cannot be verified. The two have to
	// be told apart here, before the resolve.
	_, broken, err := loadBacklog(cfg)
	if err != nil {
		return false, fmt.Errorf("cannot verify who holds this target (%s): %w", shortID(holder), err)
	}
	// PRECONDITION, and it is narrow: `broken` entries are
	// filename-prefixed strings, so matching by prefix is sound only
	// because a reservation contains a canonical FULL id written by
	// writePromotionReservation. This helper is not safe for arbitrary
	// reservation contents — a short prefix or a hand-edited file could
	// match the wrong record. Nothing else writes reservations today;
	// if anything ever does, this needs a real filename comparison.
	for _, b := range broken {
		if strings.HasPrefix(b, holder+".yaml:") {
			return false, fmt.Errorf("cannot verify who holds this target (%s): its record will not parse — %s", shortID(holder), b)
		}
	}

	item, _, err := resolveBacklogItem(cfg, holder)
	if err != nil {
		if errors.Is(err, errBacklogNotFound) {
			// Definitely gone, and readable enough to know it: an id
			// that resolves to nothing cannot be protecting a claim.
			return true, nil
		}
		// ANY other failure — an ambiguous reference, an I/O error —
		// means ownership cannot be verified, and this used to return
		// "stale" for all of them. Promotion becomes unavailable
		// instead, and the reservation stands.
		return false, fmt.Errorf("cannot verify who holds this target (%s): %w", shortID(holder), err)
	}
	return item.State() != parser.StateOpen, nil
}

// scaffoldOriginID reads the backlog id a scaffolded feature was
// promoted from, or empty if it carries none.
//
// Empty is not an error and must not be treated as one: a feature
// created by `parlay add-feature` legitimately has no origin, and so
// does a scaffold interrupted before the comment was written.
func scaffoldOriginID(featurePath string) (string, error) {
	content, err := os.ReadFile(filepath.Join(featurePath, "intents.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", filepath.Join(featurePath, "intents.md"), err)
	}
	const marker = "backlog-origin: "
	i := strings.Index(string(content), marker)
	if i < 0 {
		return "", nil
	}
	rest := string(content)[i+len(marker):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	return strings.TrimSpace(rest), nil
}
