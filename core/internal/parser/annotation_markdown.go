package parser

import (
	"strings"
)

// Markdown annotations, §3.1 and §4.1: an HTML comment on its own line, and
// the unit above it that it is about.
//
// The lexer here shares its comment recognition with comments.go — the same
// findCommentOpen, the same fence rules. That sharing is load-bearing rather
// than tidy: if the scanner and the parsers disagreed about where a comment
// begins, the same bytes would be content to the structural parser and an
// actionable request to the resolver at once. Inside code, the sigil is a
// picture of a sigil, for both readers.

// Markdown anchor unit kinds.
const (
	annUnitHeading     = "heading"
	annUnitListItem    = "list-item"
	annUnitField       = "field"
	annUnitTurn        = "turn"
	annUnitParagraph   = "paragraph"
	annUnitFrontmatter = "frontmatter"
	annUnitSection     = "section"
)

// findCommentOpen returns the index of the next `<!--` on the line that is not
// inside an inline code span, or -1.
func findCommentOpen(line string) int {
	pos := 0
	for {
		idx, skip := nextCommentOpen(line[pos:])
		if skip > 0 {
			pos += skip
			continue
		}
		if idx < 0 {
			return -1
		}
		return pos + idx
	}
}

// lexMarkdownAnnotations finds every HTML comment whose content opens with the
// sigil, and reports where it sat.
//
// It reads its structure off the SAME pass the parsers use (analyseMarkdown,
// which drives mdComments): which lines are fenced code, and what text on a
// line survives comment stripping. Nothing here re-derives either rule, so
// the scanner and the parsers cannot come to disagree about where a comment
// begins or what counts as content beside one.
func lexMarkdownAnnotations(path string, b *mdBody) ([]rawEntry, []AnnotationFinding) {
	var entries []rawEntry
	var findings []AnnotationFinding

	for i := b.bodyAt; i < len(b.lines); i++ {
		if b.info[i].code {
			continue
		}
		col := 0
		for col <= len(b.lines[i]) {
			rel := findCommentOpen(b.lines[i][col:])
			if rel < 0 {
				break
			}
			open := col + rel
			body, endLine, closeEnd, closed := b.readComment(i, open)
			if !closed {
				if looksLikeAnnotation(body) {
					findings = append(findings, AnnotationFinding{
						Code:    AnnotationMalformed,
						File:    path,
						Line:    i + 1,
						Message: "the comment holding this annotation is never closed",
						Fix:     "close it with -->",
					})
				}
				return entries, findings
			}
			if looksLikeAnnotation(body) {
				entries = append(entries, rawEntry{
					body:    body,
					line:    i + 1,
					endLine: endLine + 1,
					column:  open,
					inline:  b.hasContent(i) || b.hasContent(endLine),
				})
			}
			// Resume where the comment ended, on its closing line. A second
			// comment after the first on that line is still to be read.
			i = endLine
			col = closeEnd
		}
	}
	return entries, findings
}

// readComment collects one HTML comment starting at `open` on line `i`,
// returning its content, the line it closes on, the index just past `-->`, and
// whether it closed at all.
func (b *mdBody) readComment(i, open int) (body string, endLine, closeEnd int, closed bool) {
	var content strings.Builder
	segment := b.lines[i][open+len("<!--"):]
	base := open + len("<!--")
	for j := i; j < len(b.lines); j++ {
		if j > i {
			segment = b.lines[j]
			base = 0
		}
		if k := strings.Index(segment, "-->"); k >= 0 {
			content.WriteString(segment[:k])
			return strings.TrimSpace(content.String()), j, base + k + len("-->"), true
		}
		content.WriteString(segment)
		content.WriteString("\n")
		endLine = j
	}
	return strings.TrimSpace(content.String()), endLine, 0, false
}

// hasContent reports whether a line carries text the parsers can see. It is
// the inline test: an annotation may share its line with another comment, and
// with nothing else.
func (b *mdBody) hasContent(i int) bool {
	return i >= 0 && i < len(b.info) && strings.TrimSpace(b.info[i].visible) != ""
}

// mdLine is one body line's structure, computed once so anchoring is a lookup
// rather than a re-scan per thread.
type mdLine struct {
	// present is false for a line the parsers cannot see: blank, or wholly a
	// comment. Those lines are transparent to the walk up (§3.5).
	present bool
	blank   bool
	indent  int
	// visible is what the parsers read on this line, and it is what decides
	// whether an annotation here shares its line with CONTENT. Another comment
	// beside an annotation is not content: `<!-- a --><!-- @dwht: x -->` is two
	// comments on one line, and neither is inline.
	visible string
	// code marks a line inside a fenced block, fences included. The scanner
	// skips these for the same reason the parsers pass them through: inside
	// code, the sigil is a picture of a sigil.
	code bool

	headingLevel  int
	headingPath   []string
	headingLevels []int

	// item is the index into mdBody.items of the list item this line opens or
	// belongs to, or -1.
	item int
}

type mdItem struct {
	line          int
	markerIndent  int
	contentIndent int
	parent        int
	end           int
}

type mdBody struct {
	lines  []string
	info   []mdLine
	items  []mdItem
	fmEnd  int
	bodyAt int
}

// analyseMarkdown computes the structure §4.1 selects on: which lines the
// parsers can see, the heading path at each, and the list-item tree.
func analyseMarkdown(lines []string, bodyStart, fmEnd int) *mdBody {
	b := &mdBody{lines: lines, info: make([]mdLine, len(lines)), fmEnd: fmEnd, bodyAt: bodyStart}

	var comments mdComments
	var headings []string
	var levels []int
	var open []int // stack of open list items, innermost last

	for i := range lines {
		b.info[i] = mdLine{item: -1, indent: -1}
		if i < bodyStart {
			continue
		}
		line := lines[i]
		wasFenced := comments.fence != ""
		visible, ok := comments.visible(line)
		blank := strings.TrimSpace(line) == ""
		b.info[i].blank = blank
		b.info[i].code = wasFenced || comments.fence != ""
		if ok {
			// visibleLine left-trims a line it touched, and indentation is
			// meaning here — it is the list level, and it is what a reader
			// sees. Put the original leading whitespace back.
			if visible != line && visible != "" {
				visible = line[:len(line)-len(strings.TrimLeft(line, " \t"))] + visible
			}
			b.info[i].visible = visible
		}
		if !ok {
			continue
		}
		b.info[i].present = true
		b.info[i].indent = len(line) - len(strings.TrimLeft(line, " \t"))

		// A heading inside a fenced block is not a heading. Without this the
		// heading stack — and so every anchor's heading path — could be
		// rewritten by an example in a code block.
		if level, title, isHeading := markdownHeading(visible); isHeading && !b.info[i].code {
			for len(levels) > 0 && levels[len(levels)-1] >= level {
				levels = levels[:len(levels)-1]
				headings = headings[:len(headings)-1]
			}
			levels = append(levels, level)
			headings = append(headings, title)
			b.info[i].headingLevel = level
			b.info[i].headingPath = append([]string(nil), headings...)
			b.info[i].headingLevels = append([]int(nil), levels...)
			open = open[:0]
			continue
		}
		b.info[i].headingPath = append([]string(nil), headings...)
		b.info[i].headingLevels = append([]int(nil), levels...)

		if blank {
			continue
		}
		ind := b.info[i].indent

		if width, isItem := listMarkerWidth(strings.TrimLeft(line, " \t")); isItem {
			for len(open) > 0 && b.items[open[len(open)-1]].contentIndent > ind {
				open = open[:len(open)-1]
			}
			parent := -1
			if len(open) > 0 {
				top := b.items[open[len(open)-1]]
				if top.markerIndent < ind {
					parent = open[len(open)-1]
				} else {
					open = open[:len(open)-1]
					parent = top.parent
				}
			}
			b.items = append(b.items, mdItem{
				line:          i,
				markerIndent:  ind,
				contentIndent: ind + width,
				parent:        parent,
				end:           i,
			})
			idx := len(b.items) - 1
			open = append(open, idx)
			b.info[i].item = idx
			continue
		}

		for len(open) > 0 && b.items[open[len(open)-1]].contentIndent > ind {
			open = open[:len(open)-1]
		}
		if len(open) > 0 {
			b.info[i].item = open[len(open)-1]
		}
	}

	// An item runs to the last line that belongs to it or to any descendant.
	for idx := range b.items {
		end := b.items[idx].line
		for j := b.items[idx].line + 1; j < len(lines); j++ {
			if !b.info[j].present {
				continue
			}
			if b.info[j].blank {
				continue
			}
			if b.info[j].indent < b.items[idx].contentIndent {
				break
			}
			end = j
		}
		b.items[idx].end = end
	}
	return b
}

// markdownHeading reports an ATX heading's level and title.
//
// At most three spaces of indentation, as CommonMark requires. Trimming any
// amount made `    # @dwht: wrong` — an annotation indented inside a fenced
// YAML block — read as a level-1 heading, which ended the enclosing page's
// `## Layout` section three lines early and made its fence look unterminated.
func markdownHeading(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(trimmed[level:]), true
}

// listMarkerWidth reports the width of a list marker at the start of an
// already-left-trimmed line, and whether there is one.
func listMarkerWidth(s string) (int, bool) {
	if len(s) >= 2 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && (s[1] == ' ' || s[1] == '\t') {
		return 2 + countLeadingSpaces(s[2:]), true
	}
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits+1 < len(s) && (s[digits] == '.' || s[digits] == ')') && s[digits+1] == ' ' {
		return digits + 2 + countLeadingSpaces(s[digits+2:]), true
	}
	return 0, false
}

func countLeadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

// isFieldLine reports a `**Field**:` line — the shape intents, dialogs and
// infrastructure all use for their named fields.
func isFieldLine(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "**") {
		return "", false
	}
	end := strings.Index(trimmed[2:], "**")
	if end < 0 {
		return "", false
	}
	name := trimmed[2 : 2+end]
	after := trimmed[2+end+2:]
	if !strings.HasPrefix(after, ":") || name == "" || strings.Contains(name, "*") {
		return "", false
	}
	return name, true
}

// anchorMarkdownThreads resolves every thread's anchor and appends the results
// to scan.
func anchorMarkdownThreads(path string, b *mdBody, threads []AnnotationThread, scan *AnnotationScan) {
	for _, thread := range threads {
		anchor, finding := b.anchorFor(path, thread)
		if finding != nil {
			scan.Findings = append(scan.Findings, *finding)
			continue
		}
		thread.Anchor = *anchor
		scan.Threads = append(scan.Threads, thread)
	}
}

func (b *mdBody) anchorFor(path string, thread AnnotationThread) (*AnnotationAnchor, *AnnotationFinding) {
	first := thread.Entries[0]
	at := first.Line - 1

	content := -1
	for i := at - 1; i >= b.bodyAt; i-- {
		if b.info[i].present && !b.info[i].blank {
			content = i
			break
		}
	}
	if content < 0 {
		if b.fmEnd > 0 && at > b.fmEnd {
			return b.frontmatterAnchor(thread), nil
		}
		return nil, &AnnotationFinding{
			Code:    AnnotationUnanchored,
			File:    path,
			Line:    first.Line,
			Message: "nothing above this annotation but the top of the file, blank lines, or other annotations",
			Fix:     "an annotation goes after the text it is about; move it below that text",
		}
	}

	if first.Section {
		return b.sectionAnchor(path, thread, content)
	}
	if content == b.fmEnd && b.fmEnd > 0 {
		return b.frontmatterAnchor(thread), nil
	}

	unit, span := b.unitAt(content, first.Column)
	if limit := at - 1; span[1] > limit {
		// A field takes the bullets under it and an item takes its nested
		// items, and either can run past a comment written in the middle. An
		// anchor is never about text it precedes, so the span stops at the
		// request. `section` is the one exception, and it is handled above:
		// the design defines it forward from its heading on purpose.
		span[1] = limit
	}
	if unit == "" {
		return nil, &AnnotationFinding{
			Code:    AnnotationUnanchored,
			File:    path,
			Line:    first.Line,
			Message: "the line above this annotation is a `---` separator, which is not text",
			Fix:     "put the annotation above the separator, under the text it is about",
		}
	}
	return b.finish(path, thread, unit, span)
}

func (b *mdBody) frontmatterAnchor(thread AnnotationThread) *AnnotationAnchor {
	// The frontmatter is YAML, and analyseMarkdown starts below it, so its
	// lines carry no Markdown metadata to filter on. The rule is the same
	// one: drop the comment lines, keep everything else.
	var out []string
	for i := 0; i <= b.fmEnd && i < len(b.lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(b.lines[i]), "#") {
			continue
		}
		out = append(out, b.lines[i])
	}
	return &AnnotationAnchor{
		Unit: annUnitFrontmatter,
		Span: [2]int{1, b.fmEnd + 1},
		Text: strings.TrimRight(strings.Join(out, "\n"), "\n "),
	}
}

// unitAt classifies the content line above an annotation and returns the span
// it selects. An empty unit means the line is not text at all.
func (b *mdBody) unitAt(at, column int) (string, [2]int) {
	line := b.lines[at]
	trimmed := strings.TrimSpace(line)

	if trimmed == "---" {
		return "", [2]int{}
	}
	if _, _, isHeading := markdownHeading(line); isHeading {
		// The heading line itself — the title. Never the section under it:
		// that text comes AFTER the comment, and a comment is never about
		// text it precedes.
		return annUnitHeading, [2]int{at, at}
	}
	if b.info[at].item >= 0 {
		return annUnitListItem, b.itemSpan(at, column)
	}
	if _, ok := isFieldLine(line); ok {
		return annUnitField, [2]int{at, b.fieldEnd(at)}
	}
	if IsRecognisedTurn(line) {
		return annUnitTurn, [2]int{at, b.turnEnd(at)}
	}
	if turn := b.enclosingTurn(at); turn >= 0 {
		// An indented `A:`/`B:` option line is part of the turn that offered
		// it, so a comment under the last option is about the turn — which is
		// where a reviewer naturally puts "there is no cancel".
		return annUnitTurn, [2]int{turn, b.turnEnd(turn)}
	}
	return annUnitParagraph, [2]int{b.paragraphStart(at), at}
}

// itemSpan picks the list level the annotation's own indentation selects, and
// returns that item's span.
//
// The rule is the innermost enclosing item whose MARKER column is at or left
// of the annotation's — align with the dash you mean, or indent past it. An
// earlier draft used the item's CONTENT column, which is what a continuation
// line obeys and what §3.5 first said; it made a comment aligned with a nested
// dash select that item's PARENT, the exact opposite of §4.2's YAML rule for
// the same visual alignment. Two opposite conventions for one gesture is a
// defect in the syntax, not a detail of the implementation.
func (b *mdBody) itemSpan(at, column int) [2]int {
	chosen := b.info[at].item
	for i := chosen; i >= 0; i = b.items[i].parent {
		if b.items[i].markerIndent <= column {
			chosen = i
			break
		}
		chosen = i
	}
	return [2]int{b.items[chosen].line, b.items[chosen].end}
}

// fieldEnd extends a `**Field**:` line over the bullets that are its value.
func (b *mdBody) fieldEnd(at int) int {
	end := at
	for j := at + 1; j < len(b.lines); j++ {
		if !b.info[j].present {
			continue
		}
		if b.info[j].blank {
			continue
		}
		if b.info[j].item < 0 {
			break
		}
		end = j
	}
	return end
}

// enclosingTurn returns the turn an indented line belongs to, or -1. The walk
// stops at the first line flush with the margin: that line is either the turn
// or something else entirely, and the option lines never reach past it.
func (b *mdBody) enclosingTurn(at int) int {
	if b.info[at].indent <= 0 {
		return -1
	}
	for j := at - 1; j >= b.bodyAt; j-- {
		if !b.info[j].present {
			continue
		}
		if b.info[j].blank {
			return -1
		}
		if b.info[j].indent == 0 {
			if IsRecognisedTurn(b.lines[j]) {
				return j
			}
			return -1
		}
	}
	return -1
}

// turnEnd extends a dialog turn over its indented option lines.
func (b *mdBody) turnEnd(at int) int {
	end := at
	for j := at + 1; j < len(b.lines); j++ {
		if !b.info[j].present {
			continue
		}
		if b.info[j].blank || b.info[j].indent == 0 {
			break
		}
		end = j
	}
	return end
}

// paragraphStart walks back over the contiguous run of non-blank lines ending
// at `at`.
func (b *mdBody) paragraphStart(at int) int {
	start := at
	for j := at - 1; j >= b.bodyAt; j-- {
		if !b.info[j].present {
			// A comment inside a paragraph does not break it; a blank line
			// does, and a blank line is reported as blank whether the parsers
			// can see it or not.
			if b.info[j].blank {
				break
			}
			continue
		}
		if b.info[j].blank {
			break
		}
		if _, _, isHeading := markdownHeading(b.lines[j]); isHeading {
			break
		}
		start = j
	}
	return start
}

// sectionAnchor widens to the enclosing section: from the nearest heading
// above to the next heading of equal or higher level, or the next `---`.
func (b *mdBody) sectionAnchor(path string, thread AnnotationThread, content int) (*AnnotationAnchor, *AnnotationFinding) {
	head := -1
	for i := content; i >= b.bodyAt; i-- {
		if b.info[i].present && b.info[i].headingLevel > 0 {
			head = i
			break
		}
	}
	start, level := b.bodyAt, 0
	if head >= 0 {
		start, level = head, b.info[head].headingLevel
	}
	end := len(b.lines) - 1
	for j := start + 1; j < len(b.lines); j++ {
		if !b.info[j].present {
			continue
		}
		if strings.TrimSpace(b.lines[j]) == "---" {
			end = j - 1
			break
		}
		if lv := b.info[j].headingLevel; lv > 0 && (level == 0 || lv <= level) {
			end = j - 1
			break
		}
	}
	return b.finish(path, thread, annUnitSection, [2]int{start, end})
}

// visibleText renders a span the way a parser reads it: comment lines dropped,
// everything else verbatim.
//
// The anchor's TEXT must not contain the thread. `section` makes that fatal
// rather than untidy — its span runs forward from the heading and so covers
// the annotation itself, and a phrase then narrows against words the reviewer
// wrote in their own request. The span stays raw, because a span names lines
// in the file a person will open.
func (b *mdBody) visibleText(span [2]int) string {
	var out []string
	for i := span[0]; i <= span[1] && i < len(b.lines); i++ {
		if b.info[i].blank {
			out = append(out, "")
			continue
		}
		if !b.info[i].present {
			continue
		}
		// info.visible, not the raw line: a whole comment line is dropped by
		// the `present` test above, but an INLINE one — `- text <!-- note -->`
		// — leaves a line the parsers read as `- text` and would otherwise
		// carry its comment into the anchor.
		out = append(out, b.info[i].visible)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n ")
}

// finish assembles the anchor, applying phrase narrowing (§4.3).
func (b *mdBody) finish(path string, thread AnnotationThread, unit string, span [2]int) (*AnnotationAnchor, *AnnotationFinding) {
	text := b.visibleText(span)
	anchor := &AnnotationAnchor{
		Unit:          unit,
		Span:          [2]int{span[0] + 1, span[1] + 1},
		HeadingPath:   b.info[span[0]].headingPath,
		HeadingLevels: b.info[span[0]].headingLevels,
		Text:          text,
	}
	if phrase := thread.Entries[0].Phrase; phrase != "" {
		if !strings.Contains(text, phrase) {
			return nil, &AnnotationFinding{
				Code:    AnnotationPhraseNotFound,
				File:    path,
				Line:    thread.Entries[0].Line,
				Message: "the quoted phrase does not occur in the anchored text",
				Fix:     "quote the phrase verbatim, or drop the quote to comment on the whole unit",
				Unit:    text,
			}
		}
		anchor.Phrase = &phrase
	}
	return anchor, nil
}
