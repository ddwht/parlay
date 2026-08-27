package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/parser"
)

// next-legacy-review hands a reviewer one stranded judgment to answer.
//
// It is one read-only composition over the same semantic object the listing
// reports: load the status once, select the next pending occurrence, build its
// packet. Neither command computes counts or state independently, so the
// overlap is two projections rather than two sources of truth — and a parity
// test holds them equal.
//
// It stays separate from migrate-coverage-exceptions rather than becoming a
// mode flag on it. The listing is inventory: what is here, what is left. This
// is a workflow step a person is walked through, with its own reachability
// contract, and collapsing the two would blur which of them a skill must call.
var nextLegacyReviewCmd = &cobra.Command{
	Use:   "next-legacy-review <@feature>",
	Short: "Emit the next stranded judgment for a person to decide",
	Long: `Select one unanswered legacy exemption and emit everything needed to decide it.

The packet's display is the evidence, ordered: what the contract requires now
and what observes it, then the prior reasoning labelled as context. The decision
block is the three-way choice, with no default. Present the display as given and
build the chooser from the decision block; do not assemble either yourself.

Never-reviewed entries come before deferred ones, so the question somebody
already found hard does not block the ones nobody has looked at.

Pass --exclude for each occurrence already handled in this sitting. Deferring
does not answer an entry, so without exclusions this would eventually hand back
the same question it just asked. Exclusions are ephemeral: a later run starts
fresh and revisits deferred work, and no session state is written anywhere.`,
	Args: cobra.ExactArgs(1),
	RunE: runNextLegacyReview,
}

var nextReviewExclude []string

func init() {
	nextLegacyReviewCmd.Flags().StringArrayVar(&nextReviewExclude, "exclude", nil,
		"occurrence already handled this sitting, as FINGERPRINT or FINGERPRINT#COPY (repeatable)")
}

type nextReviewOutput struct {
	Feature string `json:"feature"`
	// Summary is the same projection the listing reports. It is emitted here so
	// the reviewer sees one coherent snapshot rather than having to correlate
	// two calls that may have loaded different states.
	Summary *CoverageMigrationStatus `json:"summary"`
	Packet  *ReviewPacket            `json:"packet,omitempty"`
	// Tokens travel in the envelope rather than the display: they are for the
	// skill to forward to the writer, never for a person to read or retype.
	Tokens *reviewTokens `json:"tokens,omitempty"`
	// Done is true when nothing remains for this sitting. Exhausted says
	// whether that is because everything is answered, or only because the rest
	// were already handled here.
	Done      bool   `json:"done"`
	Exhausted bool   `json:"all_answered"`
	Note      string `json:"note"`
}

type reviewTokens struct {
	Fingerprint    string `json:"fingerprint"`
	Duplicate      int    `json:"duplicate"`
	LegacyFileHash string `json:"legacy_file_hash"`
}

// parseExclusion accepts FINGERPRINT or FINGERPRINT#COPY.
func parseExclusion(v string) (string, int, error) {
	fp, copyPart, found := strings.Cut(strings.TrimSpace(v), "#")
	if fp == "" {
		return "", 0, fmt.Errorf("--exclude %q names no fingerprint", v)
	}
	if !found {
		return fp, 0, nil
	}
	n, err := strconv.Atoi(copyPart)
	if err != nil || n < 0 {
		return "", 0, fmt.Errorf("--exclude %q has a copy index that is not a number", v)
	}
	return fp, n, nil
}

func runNextLegacyReview(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])

	status, err := CollectCoverageMigrationStatus(cfg, slug)
	if err != nil {
		return err
	}
	_, fileHash, err := loadLegacyEntries(cfg, slug)
	if err != nil {
		return err
	}

	known := map[string]bool{}
	for _, o := range status.Occurrences {
		known[legacyDispositionKey(o.Fingerprint, o.Duplicate)] = true
	}

	excluded := map[string]bool{}
	for _, raw := range nextReviewExclude {
		fp, dup, pErr := parseExclusion(raw)
		if pErr != nil {
			return pErr
		}
		key := legacyDispositionKey(fp, dup)
		// A stale exclusion is refused rather than ignored. Silently accepting
		// one would let a caller carrying identities from an older version of
		// the legacy file skip occurrences that are not the ones it thinks it
		// handled — which is the same class of error the writers' token check
		// exists to prevent, arriving through the back door.
		if !known[key] {
			return fmt.Errorf("--exclude %q names no occurrence in this feature's current status. If the retired review changed since you listed it, start the sitting again from a fresh listing", raw)
		}
		excluded[key] = true
	}

	var next *MigrationOccurrence
	for _, want := range []string{"unreviewed", "deferred"} {
		for i := range status.Occurrences {
			o := &status.Occurrences[i]
			if o.State != want || excluded[legacyDispositionKey(o.Fingerprint, o.Duplicate)] {
				continue
			}
			next = o
			break
		}
		if next != nil {
			break
		}
	}

	out := nextReviewOutput{Feature: slug, Summary: status}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")

	if next == nil {
		out.Done = true
		out.Exhausted = status.PendingTotal == 0
		if out.Exhausted {
			out.Note = "Every stranded judgment has been answered. The boundary will stop reporting them."
		} else {
			out.Note = fmt.Sprintf("Nothing further for this sitting: %d still unanswered, all handled here already. Deferred entries stay unanswered and will be offered again next time.", status.PendingTotal)
		}
		return enc.Encode(out)
	}

	packet, err := BuildReviewPacket(cfg, slug, *next)
	if err != nil {
		return err
	}
	out.Packet = packet
	out.Tokens = &reviewTokens{
		Fingerprint: next.Fingerprint, Duplicate: next.Duplicate,
		LegacyFileHash: fileHash,
	}
	out.Note = "Present packet.display verbatim. Build the chooser from packet.decision. Forward tokens to whichever writer the reviewer's answer selects, then add this fingerprint to --exclude for the rest of the sitting."
	return enc.Encode(out)
}
