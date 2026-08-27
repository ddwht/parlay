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
	// Actions are keyed by the same stable outcome IDs the decision block
	// offers, so the skill maps a choice to a command by lookup, never by
	// reasoning about which flags this occurrence needs.
	Actions map[string]ReviewAction `json:"actions,omitempty"`
	// ExcludeToken is what to pass to --exclude after handling this
	// occurrence, ready-made. The bare fingerprint is wrong when copies exist:
	// excluding copy 1 by fingerprint alone would fail to exclude it and the
	// sitting would re-offer the same question.
	ExcludeToken string `json:"exclude_token,omitempty"`
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

// ReviewAction is a fully-formed command for one outcome.
//
// The skill selects actions[choice], appends the authority values, and runs it.
// It does not assemble argv from packet data — an earlier version told it to,
// and the instruction was not followable: the fields it named had been
// deliberately removed from the JSON, entry-wide occurrences need a flag
// OMITTED rather than filled, and the exclusion token is not the bare
// fingerprint when copies exist. Every one of those is a branch, and a branch
// in prose is a branch nobody tests.
type ReviewAction struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	// Requires names the flags the skill must append from the decision channel.
	// They are the only values not derivable here, because they do not exist
	// until a person answers.
	Requires []string `json:"requires"`
}

type reviewTokens struct {
	Fingerprint    string `json:"fingerprint"`
	Duplicate      int    `json:"duplicate"`
	LegacyFileHash string `json:"legacy_file_hash"`
}

// reviewActions builds the command for each outcome.
//
// Two things here are decisions, not formatting. An entry-wide occurrence gets
// NO --criterion, because passing one would narrow a decision the reviewer was
// explicitly asked to make over every requirement — the packet said "ALL N" and
// recording one bullet would contradict what they answered. And --by is left to
// the caller rather than defaulted, because a literal baked in here would put
// the same generic attribution on every judgment in the ledger, which is not
// attribution at all.
func reviewActions(slug string, occ MigrationOccurrence, packet *ReviewPacket, fileHash string) map[string]ReviewAction {
	feature := "@" + slug
	tokens := []string{
		"--legacy-file-hash", fileHash,
	}
	dup := fmt.Sprintf("%d", occ.Duplicate)

	reconfirm := []string{feature, "--from-legacy",
		"--legacy-fingerprint", occ.Fingerprint,
		"--legacy-duplicate", dup,
		"--legacy-file-hash", fileHash,
		"--ref", occ.Ref, "--kind", "waived",
	}
	// Bullet-specific only. An entry-wide exemption maps to an entry-wide
	// exception, and record-exception requires --criterion omitted for that.
	if !packet.subject.EntryWide {
		reconfirm = append(reconfirm, "--criterion", occ.Text)
	}

	withTokens := func(extra ...string) []string {
		args := append([]string{feature,
			"--fingerprint", occ.Fingerprint,
			"--duplicate", dup,
		}, tokens...)
		return append(args, extra...)
	}

	needs := []string{"--reason", "--by"}
	return map[string]ReviewAction{
		OutcomeReconfirm: {Command: "record-exception", Args: reconfirm, Requires: needs},
		OutcomeDrop:      {Command: "drop-legacy-exemption", Args: withTokens(), Requires: needs},
		OutcomeDefer:     {Command: "defer-legacy-exemption", Args: withTokens(), Requires: needs},
	}
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
	out.Actions = reviewActions(slug, *next, packet, fileHash)
	out.ExcludeToken = next.Fingerprint
	if next.Duplicate > 0 {
		out.ExcludeToken = fmt.Sprintf("%s#%d", next.Fingerprint, next.Duplicate)
	}
	out.Note = "Present packet.display verbatim. Build the chooser from packet.decision. Run actions[<chosen id>] with its args, appending the flags it lists in `requires`. Then pass exclude_token to --exclude for the rest of the sitting."
	return enc.Encode(out)
}
