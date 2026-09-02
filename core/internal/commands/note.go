// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: backlog-item

package commands

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/ledger"
	"github.com/ddwht/parlay/core/internal/parser"
)

// `parlay note` records something observed and not done.
//
// The thing it replaces is a conversation. An agent mid-build finds a
// defect, a gap, a shortcut worth revisiting — says so, and the session
// ends. Parlay records the code, the decisions enforced in the code, the
// drift and the coverage judgments, and records nothing about work it
// noticed and walked past.
//
// So capture has to be CHEAP. Cheaper to record the thing than to argue
// about it: no prompt, no triage, no priority unless somebody actually
// gave one. A capture that costs a round trip to the user will be skipped
// in exactly the situation it exists for — mid-implementation, inside a
// subagent that cannot ask anyone anything.
//
// Cheap for the CALLER, not sloppy in the WRITER. The two are different
// obligations and conflating them is a mistake: this is durable,
// user-facing, schema-versioned state, so malformed input is refused with
// a non-zero exit rather than written as a corrupt record. What the
// calling phase owes is to treat that failure as non-blocking — a note
// that fails to write must never fail the phase that tried.
var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Record something observed but not done, without interrupting the work",
	Long: `Create one backlog item.

For a concrete, evidenced piece of undone work, or an explicit later/defer
statement. Not for speculation, generic suggestions, or work already recorded —
every phase boundary reports the ids captured during it, so noise is visible
rather than silent.

Priority is optional and is NEVER guessed. Absent means untriaged, which is a
fact about the record rather than a default, and it is what lets a listing
surface the pile that needs a person. Pass --priority only when somebody
actually ranked it.`,
	Args: cobra.NoArgs,
	RunE: runNote,
}

var (
	noteKind     string
	noteTitle    string
	noteBody     string
	notePriority string
	noteFeature  string
	notePhase    string
	noteAbout    []string
	noteEvidence []string
	noteBy       string
)

func init() {
	f := noteCmd.Flags()
	f.StringVar(&noteKind, "kind", "", "defect | gap | debt | idea (required)")
	f.StringVar(&noteTitle, "title", "", "one line a reader can recognise in a listing (required)")
	f.StringVar(&noteBody, "body", "", "the observation in full")
	f.StringVar(&notePriority, "priority", "", "P0 | P1 | P2 — only when somebody ranked it")
	f.StringVar(&noteFeature, "feature", "", "the feature being worked on when this was found")
	f.StringVar(&notePhase, "phase", "", "the pipeline phase it was found in")
	f.StringArrayVar(&noteAbout, "about", nil, "a parlay ref this concerns (repeatable)")
	f.StringArrayVar(&noteEvidence, "evidence", nil, "path[:line] (repeatable)")
	f.StringVar(&noteBy, "by", "", "who or what observed it (required)")
}

func runNote(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	// Every refusal carries its published code and fix, including these
	// — a missing --by is as much a malformed capture as a bad --kind,
	// and the caller has no file to validate afterwards either way.
	for _, req := range []struct {
		flag, value string
		kind        parser.BacklogProblemKind
		why         string
	}{
		{"--kind", noteKind, parser.BacklogProblemMissingKind, "an item with no kind cannot be filtered or triaged"},
		{"--title", noteTitle, parser.BacklogProblemMissingTitle, "an item with no title is one nobody can recognise in a listing"},
		{"--by", noteBy, parser.BacklogProblemCaptureMissing, "an observation nobody can attribute is one nobody can follow up"},
	} {
		if strings.TrimSpace(req.value) == "" {
			return refusal(req.kind, fmt.Sprintf("%s is required: %s", req.flag, req.why))
		}
	}

	kind := parser.BacklogKind(strings.TrimSpace(noteKind))
	if !parser.KnownBacklogKind(kind) {
		return refusal(parser.BacklogProblemMissingKind,
			fmt.Sprintf("--kind %q is not one of defect, gap, debt, idea", noteKind))
	}
	// Never inferred. A guessed priority is worse than an absent one: it
	// looks like a judgment and is not.
	if p := strings.TrimSpace(notePriority); p != "" && !parser.KnownBacklogPriority(p) {
		return refusal(parser.BacklogProblemPriority,
			fmt.Sprintf("--priority %q is not one of P0, P1, P2", notePriority))
	}
	evidence, err := parseEvidenceFlags(noteEvidence)
	if err != nil {
		return err
	}

	// ONE instant for both the id and captured.at. They were two
	// time.Now() calls, so an item's recorded capture time and the
	// timestamp inside its own id could disagree — a small thing that
	// makes an id useless as a cross-check against the field it mirrors.
	now := time.Now().UTC()

	item := &parser.BacklogItem{
		SchemaVersion: parser.BacklogSchemaVersion,
		ID:            newBacklogIDAt(now, strings.TrimSpace(noteTitle)),
		Kind:          kind,
		Priority:      strings.TrimSpace(notePriority),
		Title:         strings.TrimSpace(noteTitle),
		Body:          strings.TrimSpace(noteBody),
		About:         noteAbout,
		Evidence:      evidence,
		Captured: parser.BacklogCapture{
			At: now.Format(time.RFC3339Nano),
			By: strings.TrimSpace(noteBy),
			// Free provenance: the loop already mints and exports
			// PARLAY_RUN_ID for every pipeline run, so an item ties back
			// to the exact run that produced it at no cost to the caller.
			Run:        os.Getenv("PARLAY_RUN_ID"),
			Feature:    strings.TrimSpace(noteFeature),
			Phase:      strings.TrimSpace(notePhase),
			OriginRoot: activeRootLabel(cfg),
		},
	}

	// Validate before writing, not after. The record is durable.
	if problems := parser.ValidateBacklogShape(item); len(problems) > 0 {
		return refusal(problems[0].Kind, problems[0].Message)
	}

	path := backlogItemPath(cfg, item.ID)
	out, err := yaml.Marshal(item)
	if err != nil {
		return fmt.Errorf("serialise item: %w", err)
	}
	// Create, never replace. Ids are collision-safe by construction, and
	// that is a statement about how often collisions happen rather than a
	// guarantee that one cannot silently overwrite somebody's record.
	if err := ledger.New(cfg.Root.Path, path).Create(out); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", item.ID)
	return nil
}

func backlogItemPath(cfg *config.Context, id string) string {
	return parser.BacklogRoot(cfg.Root.Path) + string(os.PathSeparator) + id + ".yaml"
}

// newBacklogIDAt builds an item id from a caller-supplied instant.
//
// WHAT IT GUARANTEES, precisely — because two earlier versions of this
// comment claimed more than the code delivers, and each time the fix was
// to widen precision rather than to narrow the claim.
//
// The id is LEXICALLY TIME-SORTABLE and therefore APPROXIMATELY
// chronological: sorting a directory listing puts earlier captures first
// whenever their timestamps differ. It is NOT a total capture order.
// Timestamps can tie, wall clocks can repeat or step backwards, and two
// concurrent processes have no shared ordering to observe in the first
// place — for equal timestamps the random suffix decides, which is
// arbitrary rather than chronological. Microseconds make ties rarer; they
// do not make them impossible, and rarer is not never.
//
// Not a sequential NNN like amendments, and that is the point of the
// randomness rather than a compromise: phase agents record mid-run and in
// parallel, so a sequential allocator is a race in which two agents pick
// the same number and one record is lost. Approximate order with no lost
// records beats exact order that drops one.
//
// The slug makes the filename legible without opening it.
func newBacklogIDAt(now time.Time, title string) string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Degrade rather than refuse: a note that cannot be written
		// because the entropy source hiccuped is a lost observation, and
		// Create's O_EXCL still refuses an actual collision.
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (8 * i))
		}
	}
	suffix := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
	return now.UTC().Format("20060102T150405.000000Z") + "-" + suffix + "-" + slugifyTitle(title)
}

// slugifyTitle keeps a filename readable and bounded.
func slugifyTitle(title string) string {
	var out []rune
	lastDash := true
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			lastDash = false
		case !lastDash && len(out) > 0:
			out = append(out, '-')
			lastDash = true
		}
		if len(out) >= 48 {
			break
		}
	}
	s := strings.Trim(string(out), "-")
	if s == "" {
		return "item"
	}
	return s
}

// parseEvidenceFlags turns `path[:line]` into structured evidence.
//
// Structured rather than free text, because `about` already holds the
// semantic refs and a path is the one thing a later reader can actually
// go and open.
//
// THE GRAMMAR, exactly: a trailing `:` followed by a positive integer is
// ALWAYS read as a line number. Every other colon stays part of the path,
// so a Windows drive letter survives. A file genuinely named `foo:118`
// therefore cannot be expressed — this parses it as `foo` line 118. That
// is a real limitation rather than a case the code handles, and an
// earlier comment here claimed otherwise.
func parseEvidenceFlags(raw []string) ([]parser.BacklogEvidence, error) {
	var out []parser.BacklogEvidence
	for _, e := range raw {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		path, line := e, 0
		if i := strings.LastIndex(e, ":"); i > 0 && i < len(e)-1 {
			if n, err := strconv.Atoi(e[i+1:]); err == nil && n > 0 {
				path, line = e[:i], n
			}
		}
		out = append(out, parser.BacklogEvidence{Path: path, Line: line})
	}
	return out, nil
}

// refusal reports a rejected capture with its PUBLISHED code and fix.
//
// Nothing is written when a capture is refused, so there is no file for
// `parlay validate` to diagnose afterwards — the refusal is the only
// diagnosis the caller will ever get. Returning a bare sentence made the
// founding intent's promise ("refused with a published code and a fix")
// false at the one moment it matters: an agent mid-phase, told its
// capture failed and not told what would make it succeed.
//
// Same mapping the validator uses, so a caller sees one vocabulary
// whether the item was refused at the boundary or diagnosed on disk.
func refusal(kind parser.BacklogProblemKind, message string) error {
	code, fix := agent.BacklogDiagnostic(kind)
	return fmt.Errorf("%s: %s\n  fix: %s", code, message, fix)
}

// validateBacklogContent runs the published validator over raw bytes, so
// the commands package has one route to the same answer the CLI's
// `--type backlog` gives rather than a second opinion.
func validateBacklogContent(path string, content []byte) []agent.ValidationOutcome {
	return agent.ValidateBacklog(agent.ModeAuthoring, path, content)
}
