// parlay-feature: annotations
// parlay-component: annotation-scanner
//
// The scanner behind anchored review comments. An annotation is
// `@<handle> [<kind>] [section] ["<phrase>"]: <text>` written inside whatever
// a comment already is in the host file — an HTML comment in Markdown, a `#`
// comment in YAML — on its own line, directly BELOW the text it is about.
//
// Everything here is pure over lines: give it a path (for the host) and the
// file's bytes, and it returns threads with resolved anchors and findings for
// the malformed ones. Nothing reads the filesystem, nothing knows about
// baselines or features; those are the CLI's job.

package parser

import (
	"path/filepath"
	"strings"
)

// Annotation error codes. Every one of these names a shape a reviewer can
// write and the scanner refuses to guess at: a broken sigil is reported, never
// skipped, because a reviewer who typed `@` meant something.
const (
	AnnotationMalformed       = "annotation-malformed"
	AnnotationWordUnknown     = "annotation-word-unknown"
	AnnotationInBlockScalar   = "annotation-in-block-scalar"
	AnnotationInline          = "annotation-inline"
	AnnotationUnanchored      = "annotation-unanchored"
	AnnotationPhraseNotFound  = "annotation-phrase-not-found"
	AnnotationReplyOrphaned   = "annotation-reply-orphaned"
	AnnotationReplyColumn     = "annotation-reply-column"
	AnnotationAfterClose      = "annotation-after-close"
	AnnotationInAppliedRecord = "annotation-in-applied-record"
)

// Annotation kinds. `do` and `ask` are requests a reviewer writes; `done`,
// `answer` and `declined` are replies a resolver writes; `close` is terminal
// and only a reviewer writes it.
const (
	AnnotationKindDo       = "do"
	AnnotationKindAsk      = "ask"
	AnnotationKindClose    = "close"
	AnnotationKindDone     = "done"
	AnnotationKindAnswer   = "answer"
	AnnotationKindDeclined = "declined"
)

// Thread states, derived from the last entry and never stored.
const (
	AnnotationOpen     = "open"
	AnnotationAnswered = "answered"
	AnnotationClosed   = "closed"
)

// Host forms. The wrapper is the file's own comment marker, not parlay's, so
// the extension decides it.
const (
	AnnotationHostMarkdown = "markdown"
	AnnotationHostYAML     = "yaml"
)

// AnnotationEntry is one line (or one run of continuation lines) of a thread.
type AnnotationEntry struct {
	By      string `json:"by"`
	Kind    string `json:"kind"`
	Section bool   `json:"section,omitempty"`
	Phrase  string `json:"phrase,omitempty"`
	Text    string `json:"text"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line"`

	// Column is the annotation's own column: the `#` in YAML, the `<!--` in
	// Markdown. It selects the anchor in YAML and the list level in Markdown,
	// and it is what a reply has to match.
	Column int `json:"-"`
}

// IsRequest reports whether this entry is one a resolver owes a response to.
func (e AnnotationEntry) IsRequest() bool {
	return e.Kind == AnnotationKindDo || e.Kind == AnnotationKindAsk
}

// IsReply reports whether this entry is one a resolver wrote.
func (e AnnotationEntry) IsReply() bool {
	switch e.Kind {
	case AnnotationKindDone, AnnotationKindAnswer, AnnotationKindDeclined:
		return true
	}
	return false
}

// AnnotationAnchor is the text a thread is about, in two identities: a generic
// one any Markdown or YAML file can supply, and — filled in downstream by ref
// resolution — the parlay ref for a file parlay understands.
type AnnotationAnchor struct {
	Unit        string   `json:"unit"`
	Span        [2]int   `json:"span"`
	HeadingPath []string `json:"heading_path,omitempty"`
	// HeadingLevels parallels HeadingPath. Ref resolution needs to know which
	// entry is the `##` and which the `###` — an intent is a level-2 heading
	// and a dialog a level-3 one, and the file's title sits above both.
	HeadingLevels []int   `json:"-"`
	YAMLPath      string  `json:"yaml_path,omitempty"`
	Text          string  `json:"text"`
	Phrase        *string `json:"phrase"`

	// Ref, Field and Index are populated by ref resolution for the file types
	// parlay knows (WP2). An unknown file keeps the generic identity above and
	// leaves these empty.
	Ref   string `json:"ref,omitempty"`
	Field string `json:"field,omitempty"`
	Index *int   `json:"index,omitempty"`
}

// AnnotationThread is a request and the entries that followed it.
type AnnotationThread struct {
	File    string            `json:"file"`
	Line    int               `json:"line"`
	State   string            `json:"state"`
	Frozen  bool              `json:"frozen"`
	Anchor  AnnotationAnchor  `json:"anchor"`
	Entries []AnnotationEntry `json:"entries"`
}

// AnnotationFinding is one malformed annotation, named by its code.
type AnnotationFinding struct {
	Code    string `json:"code"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`

	// Unit carries the anchored unit's text for annotation-phrase-not-found,
	// where a reviewer cannot fix the quote without seeing what was quoted.
	Unit string `json:"unit,omitempty"`
}

// AnnotationScan is one file's threads and findings. A file with findings
// still reports its well-formed threads: one broken sigil must not hide the
// review around it.
type AnnotationScan struct {
	File     string              `json:"file"`
	Host     string              `json:"host"`
	Threads  []AnnotationThread  `json:"threads"`
	Findings []AnnotationFinding `json:"findings"`
}

// AnnotationHostFor returns the comment form a path uses, or "" for a file the
// scanner does not read.
func AnnotationHostFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md":
		return AnnotationHostMarkdown
	case ".yaml", ".yml":
		return AnnotationHostYAML
	}
	return ""
}

// rawEntry is one lexed annotation before its grammar is read: the text from
// the `@` onward with the host's comment markers removed, and where it sat.
type rawEntry struct {
	body    string
	line    int
	endLine int
	column  int
	// inline records that the annotation shared its line with content. The
	// finding is raised here rather than at lex time so that a line carrying
	// an ordinary comment — one that is not an annotation at all — passes
	// without comment.
	inline bool
}

// ScanAnnotations reads one file's annotations. path decides the host form;
// content is the file's bytes.
//
// A Markdown file with YAML frontmatter is two regions: the frontmatter reads
// by the YAML rules (§4.2) and the body below it by the Markdown ones (§4.1),
// which is what `*.page.md` and amendment records need.
func ScanAnnotations(path string, content []byte) AnnotationScan {
	host := AnnotationHostFor(path)
	scan := AnnotationScan{File: path, Host: host}
	if host == "" {
		return scan
	}

	lines := splitAnnotationLines(string(content))

	switch host {
	case AnnotationHostYAML:
		region := yamlRegion{lines: lines, start: 0}
		entries, findings := lexYAMLAnnotations(path, region)
		scan.Findings = append(scan.Findings, findings...)
		threads, tf := assembleThreads(path, AnnotationHostYAML, lines, entries)
		scan.Findings = append(scan.Findings, tf...)
		anchorYAMLThreads(path, region, threads, &scan)
	case AnnotationHostMarkdown:
		fmEnd := frontmatterEnd(lines)
		if fmEnd > 0 {
			region := yamlRegion{lines: lines[1:fmEnd], start: 1}
			entries, findings := lexYAMLAnnotations(path, region)
			scan.Findings = append(scan.Findings, findings...)
			threads, tf := assembleThreads(path, AnnotationHostYAML, lines, entries)
			scan.Findings = append(scan.Findings, tf...)
			anchorYAMLThreads(path, region, threads, &scan)
		}
		body := analyseMarkdown(lines, bodyStartAfter(fmEnd), fmEnd)
		entries, findings := lexMarkdownAnnotations(path, body)
		scan.Findings = append(scan.Findings, findings...)
		threads, tf := assembleThreads(path, AnnotationHostMarkdown, lines, entries)
		scan.Findings = append(scan.Findings, tf...)
		anchorMarkdownThreads(path, body, threads, &scan)
	}

	sortAnnotationsByLine(&scan)
	return scan
}

// splitAnnotationLines splits content into lines with line endings normalised.
// A trailing newline does not produce a final empty line, so line numbers
// match what an editor shows.
func splitAnnotationLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

// frontmatterEnd returns the index of the closing `---` of a leading YAML
// frontmatter block, or 0 when the file has none.
func frontmatterEnd(lines []string) int {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i
		}
	}
	return 0
}

// assembleThreads groups lexed entries into threads and raises the findings
// that are about a thread's shape rather than one entry's grammar.
//
// A thread is a run of CONSECUTIVE entries at one column — nothing between
// them at all, not even a blank line. An earlier draft let blank lines through,
// on the reading that §3.5 makes them transparent to an anchor. It broke the
// documented way to reopen a conversation: "start a new thread on the same
// text" under a closed one became `annotation-after-close`, and the reviewer's
// new request vanished from the listing instead of being read. A blank line is
// how a person separates two conversations, so it has to separate them here.
//
// The reply that follows a request also has to sit at the request's column; in
// YAML a reply one column over would silently re-anchor itself to other text.
func assembleThreads(path, host string, lines []string, entries []rawEntry) ([]AnnotationThread, []AnnotationFinding) {
	var threads []AnnotationThread
	var findings []AnnotationFinding

	var current *AnnotationThread
	prevEnd := 0   // 1-based end line of the previous entry; 0 means none
	closedAt := -1 // column of a close that ended the current thread

	flush := func() {
		if current != nil {
			threads = append(threads, *current)
			current = nil
		}
	}

	for _, raw := range entries {
		entry, fs := parseAnnotationEntry(path, host, raw)
		findings = append(findings, fs...)
		if entry == nil {
			// Malformed: it is reported, and it does not join a thread. It
			// still ends the run, because guessing which side of a broken
			// entry a later reply belongs to is exactly the guess the scanner
			// must not make.
			flush()
			prevEnd = 0
			closedAt = -1
			continue
		}

		if prevEnd == 0 || raw.line != prevEnd+1 {
			flush()
			closedAt = -1
		}

		if current != nil && closedAt >= 0 && closedAt == entry.Column {
			findings = append(findings, AnnotationFinding{
				Code:    AnnotationAfterClose,
				File:    path,
				Line:    entry.Line,
				Message: "an entry follows a close at the same column; the closed thread is about to be removed and this would go with it",
				Fix:     "leave a blank line and start a new thread below the closed one",
			})
			prevEnd = raw.endLine
			continue
		}

		if current != nil && entry.Column != current.Entries[0].Column {
			if entry.IsReply() || entry.Kind == AnnotationKindClose {
				findings = append(findings, AnnotationFinding{
					Code:    AnnotationReplyColumn,
					File:    path,
					Line:    entry.Line,
					Message: "this reply sits at a different column than the request it follows, which would anchor it to different text",
					Fix:     "place the reply at the request's column",
				})
				prevEnd = raw.endLine
				continue
			}
			flush()
			closedAt = -1
		}

		if current == nil {
			if entry.IsReply() || entry.Kind == AnnotationKindClose {
				// A close ends a conversation, so it needs one. On its own it
				// would be a thread born closed — swept on the next clear,
				// with nothing ever having been asked or answered.
				what, fix := "a reply", "write the reply directly under the request it answers"
				if entry.Kind == AnnotationKindClose {
					what, fix = "a close", "write the close directly under the request or reply it ends"
				}
				findings = append(findings, AnnotationFinding{
					Code:    AnnotationReplyOrphaned,
					File:    path,
					Line:    entry.Line,
					Message: what + " with no request directly above it",
					Fix:     fix,
				})
				prevEnd = raw.endLine
				continue
			}
			current = &AnnotationThread{File: path, Line: entry.Line}
		}
		current.Entries = append(current.Entries, *entry)
		if entry.Kind == AnnotationKindClose {
			closedAt = entry.Column
		}
		prevEnd = raw.endLine
	}
	flush()

	for i := range threads {
		threads[i].State = threadState(threads[i].Entries)
	}
	return threads, findings
}

// threadState derives a thread's state from its last entry. Never stored: a
// state field in the file would be a second thing to keep true.
func threadState(entries []AnnotationEntry) string {
	if len(entries) == 0 {
		return AnnotationOpen
	}
	last := entries[len(entries)-1]
	switch {
	case last.Kind == AnnotationKindClose:
		return AnnotationClosed
	case last.IsReply():
		return AnnotationAnswered
	default:
		return AnnotationOpen
	}
}

func sortAnnotationsByLine(scan *AnnotationScan) {
	sortByLine(scan.Threads, func(t AnnotationThread) int { return t.Line })
	sortByLine(scan.Findings, func(f AnnotationFinding) int { return f.Line })
}

// sortByLine is an insertion sort over the small slices a single file yields,
// stable so that two findings on one line keep the order they were raised in.
func sortByLine[T any](items []T, line func(T) int) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && line(items[j]) < line(items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// StructuralLines returns, for each line of a Markdown or YAML file, the
// structural text a parser recognises there — empty for a line that is wholly
// a comment, and in YAML for block-scalar content, which is a scalar's value
// rather than structure. Blank lines come back blank, which they already were.
//
// A projection, not a literal transcript: a Markdown line whose comment was
// stripped has its leading whitespace restored, because indentation is the
// list level and losing it would change the structure this exists to report.
//
// Exported for ref resolution, which counts bullets, fields and turns to say
// "the second constraint of this intent". Counting raw lines there would count
// a commented-out bullet, and the ref would name a different bullet than the
// one the reviewer read — the scanner and the ref resolver have to agree about
// what a comment is, exactly as the scanner and the parsers do.
func StructuralLines(path string, content []byte) []string {
	lines := splitAnnotationLines(string(content))
	out := make([]string, len(lines))

	switch AnnotationHostFor(path) {
	case AnnotationHostMarkdown:
		fmEnd := frontmatterEnd(lines)
		if fmEnd > 0 {
			region := yamlRegion{lines: lines[1:fmEnd], start: 1}
			s := analyseYAML(region)
			for i := range region.lines {
				if s.content[i] {
					out[1+i] = yamlValuePart(region.lines[i])
				}
			}
		}
		body := analyseMarkdown(lines, bodyStartAfter(fmEnd), fmEnd)
		for i := range lines {
			if body.info[i].present {
				out[i] = body.info[i].visible
			}
		}
	case AnnotationHostYAML:
		region := yamlRegion{lines: lines}
		s := analyseYAML(region)
		for i := range lines {
			if s.content[i] {
				// The value part, not the raw line: `id: task.create # note`
				// must not resolve an entry's name to "task.create # note".
				out[i] = yamlValuePart(lines[i])
			}
		}
	default:
		copy(out, lines)
	}
	return out
}

// bodyStartAfter is where the Markdown body begins given a frontmatter end.
func bodyStartAfter(fmEnd int) int {
	if fmEnd > 0 {
		return fmEnd + 1
	}
	return 0
}
