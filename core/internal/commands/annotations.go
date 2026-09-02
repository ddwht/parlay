// parlay-feature: annotations
// parlay-component: annotations-cli
//
// `parlay annotations` — the people-facing half: list what is under review,
// reply to a thread, and remove the threads a reviewer has closed.
//
// The division of labour is the design's, not an accident of layering. The
// CLI finds, reports and writes well-formed entries; it never decides that a
// conversation is finished. Closure is one entry a reviewer wrote, which is
// what makes `clear` safe to run unattended before a build.

package commands

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ddwht/parlay/core/internal/atomicfile"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var (
	annotationsListJSON  bool
	annotationsListState string
	annotationsReplyKind string
	annotationsReplyBy   string
	annotationsReplyText string
	annotationsClearFile string
)

var annotationsCmd = &cobra.Command{
	Use:   "annotations",
	Short: "Read and answer anchored review comments",
	Long: `Anchored review comments — the fourth route for saying something is wrong.

A reviewer writes ` + "`@you: text`" + ` inside a comment in the file, directly below
the text it is about; the file they are reading becomes the inbox. This command
reads those threads, writes well-formed replies into them, and removes the ones
the reviewer has explicitly closed.

It never closes a thread. A reply the reviewer has not read is
indistinguishable from one they accept, so closure is an entry they write.`,
}

var annotationsListCmd = &cobra.Command{
	Use:   "list [@feature]",
	Short: "List review threads with their resolved anchors",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAnnotationsList,
}

var annotationsReplyCmd = &cobra.Command{
	Use:   "reply <file>:<line>",
	Short: "Write a reply beneath the request at that line",
	Args:  cobra.ExactArgs(1),
	RunE:  runAnnotationsReply,
}

var annotationsClearCmd = &cobra.Command{
	Use:   "clear [@feature]",
	Short: "Delete the threads a reviewer has closed, and nothing else",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAnnotationsClear,
}

func init() {
	annotationsCmd.AddCommand(annotationsListCmd, annotationsReplyCmd, annotationsClearCmd)

	lf := annotationsListCmd.Flags()
	lf.BoolVar(&annotationsListJSON, "json", false, "machine-readable output")
	lf.StringVar(&annotationsListState, "state", "", "only open, answered or closed threads")

	rf := annotationsReplyCmd.Flags()
	rf.StringVar(&annotationsReplyKind, "kind", "", "done, answer, declined or close (required)")
	rf.StringVar(&annotationsReplyBy, "by", "", "who is replying (required)")
	rf.StringVar(&annotationsReplyText, "text", "", "the reply; optional only for close")

	annotationsClearCmd.Flags().StringVar(&annotationsClearFile, "file", "", "clear one file rather than a feature")
}

func runAnnotationsList(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	scans, err := annotationScansFor(cfg, args)
	if err != nil {
		return err
	}
	refuseAnnotationsInAppliedRecords(scans)

	if annotationsListJSON {
		out := annotationsOutput{Threads: []parser.AnnotationThread{}, Findings: []parser.AnnotationFinding{}}
		for _, scan := range scans {
			for _, thread := range scan.Threads {
				if annotationsListState != "" && thread.State != annotationsListState {
					continue
				}
				thread.Frozen = scan.Frozen
				out.Threads = append(out.Threads, thread)
			}
			out.Findings = append(out.Findings, scan.Findings...)
		}
		out.Counts = countAnnotations(scans)
		return printJSON(cmd, out)
	}

	w := cmd.OutOrStdout()
	shown := 0
	for _, scan := range scans {
		for _, thread := range scan.Threads {
			if annotationsListState != "" && thread.State != annotationsListState {
				continue
			}
			shown++
			printThread(w, scan, thread)
		}
	}
	for _, scan := range scans {
		for _, f := range scan.Findings {
			fmt.Fprintf(w, "%s:%d  %s\n    %s\n    fix: %s\n\n", f.File, f.Line, f.Code, f.Message, f.Fix)
		}
	}
	if shown == 0 {
		fmt.Fprintln(w, "no review threads")
		return nil
	}
	counts := countAnnotations(scans)
	fmt.Fprintf(w, "%d open, %d answered, %d closed\n", counts.Open, counts.Answered, counts.Closed)
	return nil
}

// printThread puts the RESOLVED ANCHOR before the request's text, deliberately.
// A YAML column or a Markdown indent will occasionally select something other
// than what the reviewer meant, and this listing is where they find that out —
// before anything acts on it, not afterwards in a diff.
func printThread(w interface{ Write([]byte) (int, error) }, scan annotationFileScan, thread parser.AnnotationThread) {
	anchor := thread.Anchor
	identity := anchor.Ref
	if identity == "" {
		identity = anchor.YAMLPath
	}
	if identity == "" {
		identity = strings.Join(anchor.HeadingPath, " › ")
	}
	if anchor.Field != "" {
		identity += "  " + anchor.Field
		if anchor.Index != nil {
			identity += "[" + strconv.Itoa(*anchor.Index) + "]"
		}
	}

	frozen := ""
	if scan.Frozen {
		frozen = "  (frozen)"
	}
	fmt.Fprintf(w, "%s:%d  %s%s\n", thread.File, thread.Line, thread.State, frozen)
	fmt.Fprintf(w, "  %s  [lines %d-%d]\n", identity, anchor.Span[0], anchor.Span[1])
	fmt.Fprintf(w, "  | %s\n", firstAnchorLine(anchor.Text))
	if anchor.Phrase != nil {
		fmt.Fprintf(w, "  | phrase: %q\n", *anchor.Phrase)
	}
	for _, e := range thread.Entries {
		text := e.Text
		if text == "" {
			text = "—"
		}
		fmt.Fprintf(w, "  @%s %s: %s\n", e.By, e.Kind, text)
	}
	fmt.Fprintln(w)
}

func firstAnchorLine(text string) string {
	if i := strings.Index(text, "\n"); i >= 0 {
		return strings.TrimSpace(text[:i]) + " …"
	}
	return strings.TrimSpace(text)
}

func annotationScansFor(cfg *config.Context, args []string) ([]annotationFileScan, error) {
	if len(args) == 1 {
		return collectFeatureAnnotations(cfg, parser.FeatureSlug(args[0]))
	}
	features, err := cfg.AllFeatures()
	if err != nil {
		return nil, fmt.Errorf("cannot enumerate features: %w", err)
	}
	var out []annotationFileScan
	for _, slug := range features {
		scans, err := collectFeatureAnnotations(cfg, slug)
		if err != nil {
			// A feature nobody can read must not report as a feature with
			// nothing to review. list and clear both act on this answer.
			return nil, err
		}
		out = append(out, scans...)
	}

	// Project sources, ONCE. They belong to no feature, so without this a
	// closed thread in the root domain model could only be swept by naming its
	// path with --file — a trap, since nothing would have told the reviewer it
	// was there.
	project, err := collectProjectAnnotations(cfg)
	if err != nil {
		return nil, err
	}
	return append(out, project...), nil
}

func runAnnotationsReply(cmd *cobra.Command, args []string) error {
	path, line, err := splitFileLine(args[0])
	if err != nil {
		return err
	}
	if annotationsReplyBy == "" {
		return fmt.Errorf("--by is required: a reply is attributed, as `parlay note --by` is")
	}
	switch annotationsReplyKind {
	case parser.AnnotationKindDone, parser.AnnotationKindAnswer, parser.AnnotationKindDeclined:
		if annotationsReplyText == "" {
			return fmt.Errorf("--text is required for %s: text is optional only after close", annotationsReplyKind)
		}
	case parser.AnnotationKindClose:
	default:
		return fmt.Errorf("--kind must be done, answer, declined or close (got %q)", annotationsReplyKind)
	}

	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	// Governance first, before the file is even read. `reply` takes a path
	// rather than a feature, so it never passed through collection and never
	// saw the routing collection performs.
	if err := refuseWriteToAppliedRecord(cfg, path); err != nil {
		return err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := insertAnnotationReply(path, content, line, annotationsReplyKind, annotationsReplyBy, annotationsReplyText)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteAtomic(path, updated); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote @%s %s under %s:%d\n", annotationsReplyBy, annotationsReplyKind, path, line)
	return nil
}

// insertAnnotationReply writes a reply beneath the entry at `line`, in the
// host's comment form and at the column the grammar requires.
//
// The skill calls this rather than editing comment text itself, so a reply is
// always well-formed and always where §3.4 says. A reply one column over would
// silently re-anchor itself to different text in YAML; a reply the tool wrote
// cannot land there.
//
// It edits by BYTE OFFSET, inserting one line and touching nothing else. The
// obvious implementation — split, rejoin with "\n" — rewrites every line
// ending in the file, so a CRLF spec would come back LF and the sweep's whole
// promise ("an ask answered, closed and swept restores the bytes exactly")
// would be false for every reviewer on Windows.
func insertAnnotationReply(path string, content []byte, line int, kind, by, text string) ([]byte, error) {
	scan := parser.ScanAnnotations(path, content)
	if scan.Host == "" {
		return nil, fmt.Errorf("%s is not a file annotations are read in (.md or .yaml)", path)
	}
	rendered, err := renderReplyBody(scan.Host, kind, by, text)
	if err != nil {
		return nil, err
	}

	var target *parser.AnnotationThread
	var entry *parser.AnnotationEntry
	for i := range scan.Threads {
		for j := range scan.Threads[i].Entries {
			if scan.Threads[i].Entries[j].Line == line {
				target = &scan.Threads[i]
				entry = &scan.Threads[i].Entries[j]
			}
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("%s:%d: %s — no request or reply on that line", path, line, parser.AnnotationReplyOrphaned)
	}
	if target.State == parser.AnnotationClosed {
		return nil, fmt.Errorf("%s:%d: %s — this thread is closed; start a new one on the same text", path, line, parser.AnnotationAfterClose)
	}

	targetIndex := -1
	for i := range scan.Threads {
		if &scan.Threads[i] == target {
			targetIndex = i
		}
	}

	column := strings.Repeat(" ", target.Entries[0].Column)
	spans := lineSpans(content)
	at := target.Entries[len(target.Entries)-1].EndLine - 1
	if at < 0 || at >= len(spans) {
		return nil, fmt.Errorf("%s:%d: the thread ends past the end of the file", path, line)
	}

	term := lineTerminator(content, spans[at])
	out := make([]byte, 0, len(content)+len(rendered)+len(column)+2*len(term))
	out = append(out, content[:spans[at][1]]...)
	if bytesEndWithNewline(content[:spans[at][1]]) {
		out = append(out, column...)
		out = append(out, rendered...)
		out = append(out, term...)
		out = append(out, content[spans[at][1]:]...)
	} else {
		// The thread ends on an unterminated last line. Give it a terminator
		// and leave the file without a trailing one, as it was.
		out = append(out, term...)
		out = append(out, column...)
		out = append(out, rendered...)
	}
	// Verified on BOTH branches. The unterminated-last-line case used to
	// return early, so the one shape most likely to render oddly was the one
	// shape nothing checked.
	return out, verifyReplyLanded(path, content, out, targetIndex, kind, by, text)
}

// annotationEntryID is an entry's content, without its position. Comparing on
// this is what makes the check survive its own insertion: writing a reply
// shifts every entry below it, so any identity that includes a line number
// reports entries as "new" that merely moved.
type annotationEntryID struct {
	by      string
	kind    string
	text    string
	phrase  string
	section bool
}

// annotationFindingSeq is the file's findings without their line numbers, in
// order — the identity that survives an insertion above them.
func annotationFindingSeq(scan parser.AnnotationScan) []string {
	var out []string
	for _, f := range scan.Findings {
		out = append(out, f.Code+"|"+f.Message+"|"+f.Fix)
	}
	return out
}

func annotationEntrySeq(scan parser.AnnotationScan) []annotationEntryID {
	var out []annotationEntryID
	for _, t := range scan.Threads {
		for _, e := range t.Entries {
			out = append(out, annotationEntryID{e.By, e.Kind, e.Text, e.Phrase, e.Section})
		}
	}
	return out
}

// verifyReplyLanded re-scans the bytes about to be written and checks they say
// what was asked for: the file's entries are exactly what they were with ONE
// inserted, at the end of the thread being answered, carrying the requested
// handle, kind and text — and no new finding.
//
// A writer that renders a comment and trusts its own rendering is a writer
// that can put a malformed or duplicated entry into a spec file. Reading the
// result back with the scanner the reviewer's own tooling uses is the only
// check that covers the rendering itself.
func verifyReplyLanded(path string, before, after []byte, targetIndex int, kind, by, text string) error {
	oldScan := parser.ScanAnnotations(path, before)
	newScan := parser.ScanAnnotations(path, after)

	// Findings are compared by IDENTITY, not by count. A count alone would
	// pass "one old finding silently replaced by a new one" — which is exactly
	// what a badly rendered reply produces when it turns a request the scanner
	// already disliked into a different complaint. Line numbers are excluded
	// because writing the reply shifts every finding below it.
	oldFindings, newFindings := annotationFindingSeq(oldScan), annotationFindingSeq(newScan)
	if len(oldFindings) != len(newFindings) {
		return fmt.Errorf("the reply would not scan back cleanly: %+v", newScan.Findings)
	}
	for i := range oldFindings {
		if oldFindings[i] != newFindings[i] {
			return fmt.Errorf("writing the reply changed a finding: %v became %v", oldFindings[i], newFindings[i])
		}
	}

	oldSeq, newSeq := annotationEntrySeq(oldScan), annotationEntrySeq(newScan)
	if len(newSeq) != len(oldSeq)+1 {
		return fmt.Errorf("writing the reply changed the entry count by %d, not 1", len(newSeq)-len(oldSeq))
	}

	// Where the insertion should be: at the end of the target thread, counted
	// across the file's entries in order.
	wantAt := 0
	for i := 0; i <= targetIndex && i < len(oldScan.Threads); i++ {
		wantAt += len(oldScan.Threads[i].Entries)
	}

	want := annotationEntryID{by: by, kind: kind, text: normaliseReplyText(text)}
	if newSeq[wantAt] != want {
		return fmt.Errorf("the reply scanned back as %+v, not %+v", newSeq[wantAt], want)
	}
	for i := 0; i < wantAt; i++ {
		if newSeq[i] != oldSeq[i] {
			return fmt.Errorf("writing the reply changed an existing entry above it: %+v became %+v", oldSeq[i], newSeq[i])
		}
	}
	for i := wantAt; i < len(oldSeq); i++ {
		if newSeq[i+1] != oldSeq[i] {
			return fmt.Errorf("writing the reply changed an existing entry below it: %+v became %+v", oldSeq[i], newSeq[i+1])
		}
	}
	return nil
}

// renderReplyBody builds the comment a reply is written as, and refuses the
// inputs that would make it something else.
//
// The handle and the text are interpolated into a comment, so they are an
// injection surface: `--text "done -->  <!-- @dwht close"` would close the
// comment early and write a SECOND entry the reviewer never authored, closing
// their own thread. A handle with a slash would not be an annotation at all
// (WP1's ref discriminator), and one with whitespace or a newline would not be
// a handle. Everything is checked before any byte is written.
func renderReplyBody(host, kind, by, text string) (string, error) {
	if by == "" || parser.ValidAnnotationHandle(by) != nil {
		return "", fmt.Errorf("--by %q is not a handle: letters, digits, and _ . - only", by)
	}
	text = normaliseReplyText(text)
	if host == parser.AnnotationHostMarkdown && strings.Contains(text, "-->") {
		return "", fmt.Errorf("--text contains `-->`, which would close the comment early and write an entry nobody authored")
	}
	if strings.ContainsAny(text, "\r\n") {
		return "", fmt.Errorf("--text still contains a line break after normalisation")
	}

	body := "@" + by + " " + kind
	if text != "" {
		body += ": " + text
	}
	if host == parser.AnnotationHostMarkdown {
		return "<!-- " + body + " -->", nil
	}
	return "# " + body, nil
}

// normaliseReplyText flattens a reply to one line. The host forms both allow
// continuation, but a tool-written reply that wraps would have to guess a width
// and would rewrap differently next time.
func normaliseReplyText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// lineSpans returns each line's [start, end) byte range, the end INCLUDING the
// line's terminator. The last line has no terminator when the file does not
// end with a newline.
func lineSpans(content []byte) [][2]int {
	var spans [][2]int
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			spans = append(spans, [2]int{start, i + 1})
			start = i + 1
		}
	}
	if start < len(content) {
		spans = append(spans, [2]int{start, len(content)})
	}
	return spans
}

// lineTerminator reports the ending a line actually uses, so an inserted line
// matches its neighbour rather than the platform.
func lineTerminator(content []byte, span [2]int) string {
	if span[1] > span[0] && content[span[1]-1] == '\n' {
		if span[1]-1 > span[0] && content[span[1]-2] == '\r' {
			return "\r\n"
		}
		return "\n"
	}
	// An unterminated last line: follow whatever the rest of the file does.
	if bytes.Contains(content, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

func bytesEndWithNewline(b []byte) bool {
	return len(b) > 0 && b[len(b)-1] == '\n'
}

func runAnnotationsClear(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	var paths []string
	if annotationsClearFile != "" {
		paths = []string{annotationsClearFile}
	} else {
		scans, err := annotationScansFor(cfg, args)
		if err != nil {
			return err
		}
		// An applied record's threads were already refused at collection —
		// they are findings, not threads — but clear re-reads and re-scans
		// each file, so the refusal has to be by PATH or the sweep would find
		// them again and delete bytes the ledger has hashed.
		for _, scan := range scans {
			if scan.Applied {
				continue
			}
			paths = append(paths, scan.Path)
		}
	}

	w := cmd.OutOrStdout()
	total := 0
	for _, path := range paths {
		if err := refuseWriteToAppliedRecord(cfg, path); err != nil {
			return err
		}
		removed, err := clearClosedThreads(path)
		if err != nil {
			return err
		}
		if removed > 0 {
			rel := relativeToRoot(cfg, path)
			fmt.Fprintf(w, "%s: removed %d closed thread(s)\n", rel, removed)
			total += removed
		}
	}
	if total == 0 {
		fmt.Fprintln(w, "no closed threads to remove")
	}
	return nil
}

// clearClosedThreads deletes every closed thread in one file and nothing else.
//
// Safe to run unattended precisely because closure is a fact in the file
// rather than an inference from the command being run: an answered thread the
// reviewer has not read stays, an open one stays, and only what a person wrote
// `close` under goes.
//
// Byte-offset deletion, for the same reason the writer inserts by offset: a
// split-and-rejoin would rewrite every line ending, and "the sweep restores
// the bytes exactly" is the property axis C was chosen to get.
func clearClosedThreads(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	scan := parser.ScanAnnotations(path, content)
	drop := map[int]bool{}
	removed := 0
	for _, thread := range scan.Threads {
		if thread.State != parser.AnnotationClosed {
			continue
		}
		removed++
		for _, e := range thread.Entries {
			for l := e.Line; l <= e.EndLine; l++ {
				drop[l] = true
			}
		}
	}
	if removed == 0 {
		return 0, nil
	}

	spans := lineSpans(content)
	out := make([]byte, 0, len(content))
	for i, span := range spans {
		if drop[i+1] {
			continue
		}
		out = append(out, content[span[0]:span[1]]...)
	}
	return removed, atomicfile.WriteAtomic(path, out)
}

func splitFileLine(arg string) (string, int, error) {
	idx := strings.LastIndex(arg, ":")
	if idx <= 0 {
		return "", 0, fmt.Errorf("expected <file>:<line>, got %q", arg)
	}
	line, err := strconv.Atoi(arg[idx+1:])
	if err != nil || line < 1 {
		return "", 0, fmt.Errorf("expected <file>:<line>, got %q", arg)
	}
	return arg[:idx], line, nil
}
