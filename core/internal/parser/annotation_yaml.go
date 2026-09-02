package parser

import (
	"fmt"
	"strings"
)

// YAML annotations, §3.1 and §4.2. The `#` column is the selector: walk up to
// the first line whose content starts at a column less than or equal to the
// annotation's, and that line opens the anchored node.
//
// This is exactly how a YAML author already reads comments, which is why it
// needs no new convention — and why `section` is refused here: the column
// already says how wide the annotation is.

// YAML anchor unit kinds.
const (
	annUnitSeqItem  = "seq-item"
	annUnitMapping  = "mapping"
	annUnitPair     = "pair"
	annUnitDocument = "document"
)

// yamlRegion is a run of lines read by the YAML rules: a whole .yaml file, or
// the frontmatter of a .page.md or an amendment record.
type yamlRegion struct {
	lines []string
	// start is the 0-based index of lines[0] in the whole file, so a finding
	// can name the line a reviewer sees.
	start int
}

func (r yamlRegion) lineNo(i int) int { return r.start + i + 1 }

func (r yamlRegion) indexOf(lineNo int) int { return lineNo - r.start - 1 }

// lexYAMLAnnotations finds `# @...` comments, tracking the two places a `#` is
// not a comment: inside a block scalar, where it is the scalar's content, and
// inside a quoted string on a content line.
func lexYAMLAnnotations(path string, region yamlRegion) ([]rawEntry, []AnnotationFinding) {
	var entries []rawEntry
	var findings []AnnotationFinding

	blockIndent := -1
	flowDepth := 0

	for i := 0; i < len(region.lines); i++ {
		line := region.lines[i]
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		if blockIndent >= 0 {
			if trimmed == "" || indent > blockIndent {
				// Scalar content. `capabilities.yaml` carries `notes: |` on
				// nearly every operation, so a sigil written here would be
				// ingested as the operation's notes by the parser and
				// invisible to the scanner at the same time. Refuse it
				// loudly instead.
				if strings.HasPrefix(trimmed, "#") && looksLikeAnnotation(strings.TrimSpace(trimmed[1:])) {
					findings = append(findings, AnnotationFinding{
						Code:    AnnotationInBlockScalar,
						File:    path,
						Line:    region.lineNo(i),
						Message: "this sigil is inside a block scalar, where it is the scalar's content and not a comment",
						Fix:     "place it at the key's column after the scalar ends",
					})
				}
				continue
			}
			blockIndent = -1
		}

		if flowDepth > 0 {
			flowDepth += flowDelta(line)
			continue
		}

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			body := strings.TrimSpace(trimmed[1:])
			if !looksLikeAnnotation(body) {
				continue
			}
			if strings.ContainsRune(line[:indent], '\t') {
				findings = append(findings, AnnotationFinding{
					Code:    AnnotationMalformed,
					File:    path,
					Line:    region.lineNo(i),
					Message: "this annotation is indented with a tab, which is not indentation in YAML",
					Fix:     "indent with spaces; the column is what selects the anchored node",
				})
				continue
			}
			// Consecutive `#` lines at the same column continue the
			// annotation; one that itself opens with `@` starts a new one.
			var text strings.Builder
			text.WriteString(body)
			end := i
			for j := i + 1; j < len(region.lines); j++ {
				next := region.lines[j]
				nextTrimmed := strings.TrimSpace(next)
				nextIndent := len(next) - len(strings.TrimLeft(next, " \t"))
				if !strings.HasPrefix(nextTrimmed, "#") || nextIndent != indent {
					break
				}
				cont := strings.TrimSpace(nextTrimmed[1:])
				if looksLikeAnnotation(cont) {
					break
				}
				text.WriteString("\n")
				text.WriteString(cont)
				end = j
			}
			entries = append(entries, rawEntry{
				body:    text.String(),
				line:    region.lineNo(i),
				endLine: region.lineNo(end),
				column:  indent,
			})
			i = end
			continue
		}

		// A content line. A trailing `# @…` on it is an annotation sharing
		// its line with content.
		if at := trailingCommentAt(line); at >= 0 {
			if body := strings.TrimSpace(line[at+1:]); looksLikeAnnotation(body) {
				entries = append(entries, rawEntry{
					body:    body,
					line:    region.lineNo(i),
					endLine: region.lineNo(i),
					column:  at,
					inline:  true,
				})
			}
		}

		value := yamlValuePart(line)
		if blockScalarOpens(value) {
			blockIndent = indent
			continue
		}
		flowDepth += flowDelta(line)
		if flowDepth < 0 {
			flowDepth = 0
		}
	}
	return entries, findings
}

// yamlOutsideQuotes marks the bytes of a line that are NOT inside a quoted
// scalar. Everything that reads structure off a raw line — the comment marker,
// the key's colon, the flow brackets — asks this first.
//
// Naive quote toggling gets three things wrong that appear in real artifacts:
// a backslash escape inside a double-quoted scalar (`"he said \"x\" # y"`), a
// doubled single quote inside a single-quoted one (`'it”s fine # here'`), and
// an apostrophe in an ordinary plain scalar (`shows: don't lose the draft`),
// which is not a quote at all. The last is why a quote only opens a scalar
// when it BEGINS a token.
func yamlOutsideQuotes(line string) []bool {
	out := make([]bool, len(line))
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch quote {
		case '"':
			if c == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if c == '"' {
				quote = 0
			}
		case '\'':
			if c == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				quote = 0
			}
		default:
			if (c == '"' || c == '\'') && yamlTokenStart(line, i) {
				quote = c
				continue
			}
			out[i] = true
		}
	}
	return out
}

// yamlTokenStart reports whether position i begins a token, which is where a
// quoted scalar may open.
func yamlTokenStart(line string, i int) bool {
	if i == 0 {
		return true
	}
	switch line[i-1] {
	case ' ', '\t', ',', '[', '{', ':', '-':
		return true
	}
	return false
}

// trailingCommentAt returns the index of a `#` that opens a comment on a
// content line — preceded by whitespace and outside any quoted scalar — or -1.
func trailingCommentAt(line string) int {
	outside := yamlOutsideQuotes(line)
	for i := 1; i < len(line); i++ {
		if outside[i] && line[i] == '#' && (line[i-1] == ' ' || line[i-1] == '\t') {
			return i
		}
	}
	return -1
}

// yamlValuePart returns a line with any trailing comment removed.
func yamlValuePart(line string) string {
	if at := trailingCommentAt(line); at >= 0 {
		return strings.TrimRight(line[:at], " \t")
	}
	return strings.TrimRight(line, " \t")
}

// blockScalarOpens reports whether a comment-free line ends with a block
// scalar indicator (`|`, `>`, with an optional chomping or indentation
// modifier).
func blockScalarOpens(value string) bool {
	value = strings.TrimRight(value, " \t")
	if value == "" {
		return false
	}
	end := len(value)
	for end > 0 && (value[end-1] == '-' || value[end-1] == '+' || (value[end-1] >= '0' && value[end-1] <= '9')) {
		end--
	}
	return end > 0 && (value[end-1] == '|' || value[end-1] == '>')
}

// flowDelta reports the net bracket depth a comment-free line adds.
func flowDelta(line string) int {
	value := yamlValuePart(line)
	outside := yamlOutsideQuotes(value)
	depth := 0
	for i := 0; i < len(value); i++ {
		if !outside[i] {
			continue
		}
		switch value[i] {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		}
	}
	return depth
}

// yamlStart is one node opening on a line: the column its content starts at,
// the path that reaches it, and what kind of node it is.
type yamlStart struct {
	column int
	path   string
	unit   string
}

type yamlFrame struct {
	indent int
	seg    string
	isSeq  bool
	index  int
}

// yamlStructure is the per-line node openings a whole region contributes.
type yamlStructure struct {
	region  yamlRegion
	starts  [][]yamlStart
	content []bool
	indent  []int
}

// analyseYAML walks the region once and records, for every line, the nodes it
// opens. A `- key: value` line opens two: the item at the dash's column and
// the first pair at the key's — which is exactly the ambiguity §4.2 resolves
// by column, and why commenting on a whole item means outdenting to the dash.
func analyseYAML(region yamlRegion) *yamlStructure {
	s := &yamlStructure{
		region:  region,
		starts:  make([][]yamlStart, len(region.lines)),
		content: make([]bool, len(region.lines)),
		indent:  make([]int, len(region.lines)),
	}
	var stack []yamlFrame
	blockIndent := -1
	flowDepth := 0

	path := func() string {
		var b strings.Builder
		for _, f := range stack {
			if f.isSeq {
				b.WriteString(f.seg)
				continue
			}
			if b.Len() > 0 {
				b.WriteString(".")
			}
			b.WriteString(f.seg)
		}
		return b.String()
	}

	for i, line := range region.lines {
		s.indent[i] = -1
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		if blockIndent >= 0 {
			if trimmed == "" || indent > blockIndent {
				continue
			}
			blockIndent = -1
		}
		if flowDepth > 0 {
			flowDepth += flowDelta(line)
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		s.content[i] = true
		s.indent[i] = indent
		value := strings.TrimSpace(yamlValuePart(line))

		if rest, isItem := strings.CutPrefix(value, "-"); isItem && (rest == "" || strings.HasPrefix(rest, " ")) {
			for len(stack) > 0 && stack[len(stack)-1].indent > indent {
				stack = stack[:len(stack)-1]
			}
			index := 0
			if len(stack) > 0 && stack[len(stack)-1].isSeq && stack[len(stack)-1].indent == indent {
				index = stack[len(stack)-1].index + 1
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, yamlFrame{indent: indent, seg: fmt.Sprintf("[%d]", index), isSeq: true, index: index})
			s.starts[i] = append(s.starts[i], yamlStart{column: indent, path: path(), unit: annUnitSeqItem})

			lead := len(rest) - len(strings.TrimLeft(rest, " "))
			inner := strings.TrimLeft(rest, " ")
			innerCol := indent + 1 + lead
			if key, after, ok := splitYAMLKey(inner); ok {
				stack = append(stack, yamlFrame{indent: innerCol, seg: key})
				s.starts[i] = append(s.starts[i], yamlStart{column: innerCol, path: path(), unit: pairUnit(after)})
			}
		} else if key, after, ok := splitYAMLKey(value); ok {
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, yamlFrame{indent: indent, seg: key})
			s.starts[i] = append(s.starts[i], yamlStart{column: indent, path: path(), unit: pairUnit(after)})
		}

		if blockScalarOpens(yamlValuePart(line)) {
			blockIndent = indent
			continue
		}
		flowDepth += flowDelta(line)
		if flowDepth < 0 {
			flowDepth = 0
		}
	}
	return s
}

func pairUnit(after string) string {
	if strings.TrimSpace(after) == "" {
		return annUnitMapping
	}
	return annUnitPair
}

// splitYAMLKey finds `key:` at the start of a value, returning the key and
// whatever follows the colon.
func splitYAMLKey(s string) (key, after string, ok bool) {
	if s == "" {
		return "", "", false
	}
	outside := yamlOutsideQuotes(s)
	for i := 0; i < len(s); i++ {
		if !outside[i] || s[i] != ':' {
			continue
		}
		if i+1 < len(s) && s[i+1] != ' ' && s[i+1] != '\t' {
			return "", "", false
		}
		key = strings.Trim(strings.TrimSpace(s[:i]), `"'`)
		if key == "" {
			return "", "", false
		}
		return key, s[i+1:], true
	}
	return "", "", false
}

// anchorYAMLThreads resolves every thread's anchor by column and appends the
// results to scan.
func anchorYAMLThreads(path string, region yamlRegion, threads []AnnotationThread, scan *AnnotationScan) {
	if len(threads) == 0 {
		return
	}
	s := analyseYAML(region)
	for _, thread := range threads {
		anchor, finding := s.anchorFor(path, thread)
		if finding != nil {
			scan.Findings = append(scan.Findings, *finding)
			continue
		}
		thread.Anchor = *anchor
		scan.Threads = append(scan.Threads, thread)
	}
}

func (s *yamlStructure) anchorFor(path string, thread AnnotationThread) (*AnnotationAnchor, *AnnotationFinding) {
	first := thread.Entries[0]
	at := s.region.indexOf(first.Line)

	found := -1
	var chosen yamlStart
	for i := at - 1; i >= 0 && found < 0; i-- {
		if !s.content[i] {
			continue
		}
		for _, start := range s.starts[i] {
			if start.column <= first.Column && (found < 0 || start.column > chosen.column) {
				chosen = start
				found = i
			}
		}
		if found < 0 && s.indent[i] <= first.Column {
			// A content line at or left of the column that opens no node —
			// a bare scalar in a sequence, say. Stop here rather than walking
			// past it into a node it is not part of.
			break
		}
	}

	if found < 0 {
		lastAbove := -1
		for i := at - 1; i >= 0; i-- {
			if s.content[i] {
				lastAbove = i
				break
			}
		}
		if lastAbove < 0 {
			// Nothing above at all. §3.5's rule governs: the anchor is always
			// ABOVE the annotation, so a comment at the top of a file is about
			// nothing yet.
			return nil, &AnnotationFinding{
				Code:    AnnotationUnanchored,
				File:    path,
				Line:    first.Line,
				Message: "nothing above this annotation but the top of the file, blank lines, or other annotations",
				Fix:     "an annotation goes after the text it is about; a comment about the whole document goes at the bottom, at column 0",
			}
		}
		// Content above, but nothing opening a node at or left of the column —
		// §4.2's document row. The span stops above the request: an anchor is
		// never about text it precedes.
		return s.finish(path, thread, annUnitDocument, "",
			[2]int{s.region.lineNo(0), s.region.lineNo(lastAbove)}, s.visibleText(0, lastAbove))
	}

	end := found
	_ = end
	if chosen.unit != annUnitPair {
		end = s.subtreeEnd(found, chosen.column)
	} else {
		end = s.continuationEnd(found, chosen.column)
	}
	// Clamp to the request. A subtree can run past a comment written in the
	// middle of it, and a unit that swallowed text BELOW the annotation would
	// claim the reviewer commented on something they had not reached.
	if limit := at - 1; end > limit {
		end = limit
	}
	return s.finish(path, thread, chosen.unit, chosen.path,
		[2]int{s.region.lineNo(found), s.region.lineNo(end)}, s.visibleText(found, end))
}

// visibleText renders a span as a YAML reader sees it: comment lines dropped,
// blank lines and content kept. The anchor's text must never contain the
// thread, or a quoted phrase could narrow against the reviewer's own words.
func (s *yamlStructure) visibleText(from, to int) string {
	var out []string
	for i := from; i <= to && i < len(s.region.lines); i++ {
		if strings.TrimSpace(s.region.lines[i]) == "" {
			out = append(out, "")
			continue
		}
		if !s.content[i] {
			continue
		}
		// yamlValuePart, not the raw line: a whole comment line is dropped by
		// the content test above, but a TRAILING one — `- criterion A # note`
		// — would otherwise carry its comment into the anchor.
		out = append(out, yamlValuePart(s.region.lines[i]))
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n ")
}

// subtreeEnd is the last line of the node opened at `column` on line `from`:
// everything below it that is indented further.
func (s *yamlStructure) subtreeEnd(from, column int) int {
	end := from
	for j := from + 1; j < len(s.region.lines); j++ {
		if !s.content[j] {
			if strings.TrimSpace(s.region.lines[j]) == "" {
				continue
			}
			continue
		}
		if s.indent[j] <= column {
			break
		}
		end = j
	}
	return end
}

// continuationEnd covers a `key: scalar` pair whose scalar runs on — a block
// scalar or a multi-line flow collection.
func (s *yamlStructure) continuationEnd(from, column int) int {
	end := from
	for j := from + 1; j < len(s.region.lines); j++ {
		if s.content[j] {
			break
		}
		if strings.TrimSpace(s.region.lines[j]) == "" || strings.HasPrefix(strings.TrimSpace(s.region.lines[j]), "#") {
			break
		}
		end = j
	}
	return end
}

func (s *yamlStructure) finish(path string, thread AnnotationThread, unit, yamlPath string, span [2]int, text string) (*AnnotationAnchor, *AnnotationFinding) {
	anchor := &AnnotationAnchor{
		Unit:     unit,
		Span:     span,
		YAMLPath: yamlPath,
		Text:     strings.TrimRight(text, "\n "),
	}
	if phrase := thread.Entries[0].Phrase; phrase != "" {
		if !strings.Contains(anchor.Text, phrase) {
			return nil, &AnnotationFinding{
				Code:    AnnotationPhraseNotFound,
				File:    path,
				Line:    thread.Entries[0].Line,
				Message: "the quoted phrase does not occur in the anchored text",
				Fix:     "quote the phrase verbatim, or drop the quote to comment on the whole node",
				Unit:    anchor.Text,
			}
		}
		anchor.Phrase = &phrase
	}
	return anchor, nil
}
