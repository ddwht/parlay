package parser

import "strings"

// HTML comments are invisible to every Markdown parser in this package.
//
// This rule started in the intents parser, for a bug that was never specific
// to intents: a scanner with no Markdown state reads `## X` inside
// `<!-- ... -->` as a heading, and everything under it as that heading's
// content. A commented-out block — a scaffold template, a passage an author
// parked while rewriting — parsed as a real one. A comment is the one
// construct whose whole purpose is "the tools should not read this", so
// reading it was the bug wherever it happened, and it happened in four
// parsers.
//
// It is also the property annotations rest on. An annotation is a comment in
// the host format, so a parser that cannot see comments cannot see an
// annotation, and the founding-document hashes — computed over PARSED content
// (hashIntent, hashDialogContent) — cannot move when a reviewer writes one
// into a frozen file.
//
// Code is the exception, and it is not a nicety. `<!-- parlay:begin -->`
// inside backticks is what the marker looks like when a spec is TALKING about
// the marker, and the claude-md-section-preservation feature does exactly that
// in its intents, dialogs and infrastructure. Stripping there deletes a
// quotation from the middle of a promise — a founding hash moves, and the
// ledger accuses a reviewer of editing text nobody touched. Every Markdown
// reader agrees a comment opener inside a code span or a fenced block is
// literal text; so does this one.

// mdComments carries the cross-line Markdown state the strippers need: an
// open HTML comment, and an open fenced code block.
//
// A struct rather than the bare *bool it replaced, because fences are the
// second piece of state and threading two pointers through five parsers
// invites one of them to forget the new one.
type mdComments struct {
	inComment bool
	// fence is the opening fence's marker run (``` or ~~~, three or more),
	// empty when no fence is open. CommonMark closes a fence only on a run of
	// the same character at least as long, so the opener has to be kept.
	fence string
}

// visible is what a line-oriented Markdown scanner should call before any
// content rule. It returns the line with comment regions removed and whether
// anything is left to read; ok=false means skip the line entirely.
//
// A line the stripper did not touch is returned byte-for-byte, so nothing
// about existing files changes. A line it did touch is re-trimmed on the left,
// because the prefix rules in these parsers match exact starts and
// `<!-- n --> ## X` would otherwise leave a leading space that hides a real
// heading.
func (c *mdComments) visible(line string) (string, bool) {
	rest, hidden := c.strip(line)
	if hidden {
		return "", false
	}
	if rest != line {
		// Only a line the stripper actually touched is re-trimmed, on both
		// ends: the prefix rules match exact starts, so `<!-- n --> ## X`
		// would otherwise leave a leading space that hides a real heading,
		// and a trailing space left where an inline comment used to be would
		// end up inside a bullet's text and move a founding hash.
		rest = strings.Trim(rest, " \t")
		if rest == "" {
			return "", false
		}
	}
	return rest, true
}

// strip removes HTML-comment regions from one line, carrying open comment and
// fence state across lines.
//
// Returns the visible remainder and whether the line is entirely hidden. A
// line that is only a comment yields hidden=true; a line with text outside the
// comment yields that text, so `<!-- note --> ## Real` still parses.
func (c *mdComments) strip(line string) (string, bool) {
	// A fenced block is content, verbatim, including a line that looks like a
	// comment. Only the closing fence is read, and only when no comment is
	// open — a fence opened inside `<!-- ... -->` was never opened at all.
	if !c.inComment && c.fence != "" {
		if closesFence(line, c.fence) {
			c.fence = ""
		}
		return line, false
	}
	if !c.inComment {
		if marker, ok := opensFence(line); ok {
			c.fence = marker
			return line, false
		}
	}

	var out strings.Builder
	for {
		if c.inComment {
			end := strings.Index(line, "-->")
			if end < 0 {
				// Whole remainder is inside the comment.
				return out.String(), out.Len() == 0
			}
			c.inComment = false
			line = line[end+len("-->"):]
			continue
		}
		start, span := nextCommentOpen(line)
		if span > 0 {
			// A code span sits before any comment opener on this line. Copy
			// it through untouched and keep looking after it.
			out.WriteString(line[:span])
			line = line[span:]
			continue
		}
		if start < 0 {
			out.WriteString(line)
			break
		}
		out.WriteString(line[:start])
		c.inComment = true
		line = line[start+len("<!--"):]
	}
	return out.String(), strings.TrimSpace(out.String()) == ""
}

// nextCommentOpen finds the next `<!--` that is not inside an inline code
// span. It returns either the opener's index (with skip == 0) or the length of
// a leading run the caller must copy through verbatim (with idx == -1) — the
// code span that came first, plus anything before it.
//
// Reported as a length rather than consumed here so that strip keeps one place
// where output is written and one place where comment state changes.
func nextCommentOpen(line string) (idx, skip int) {
	for i := 0; i < len(line); {
		switch line[i] {
		case '`':
			run := backtickRun(line, i)
			end := closingBacktickRun(line, i+run, run)
			if end < 0 {
				// No closing run: the backticks are literal, and whatever
				// follows is ordinary text. Nothing to protect.
				i += run
				continue
			}
			return -1, end + run
		case '<':
			if strings.HasPrefix(line[i:], "<!--") {
				return i, 0
			}
			i++
		default:
			i++
		}
	}
	return -1, 0
}

// backtickRun returns the length of the run of backticks starting at i.
func backtickRun(line string, i int) int {
	return backtickRunOf(line, i, '`')
}

// closingBacktickRun returns the index of the run of exactly n backticks that
// closes a code span opened with n, searching from `from`. Returns -1 when the
// span is never closed on this line.
func closingBacktickRun(line string, from, n int) int {
	for i := from; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := backtickRun(line, i)
		if run == n {
			return i
		}
		i += run
	}
	return -1
}

// opensFence reports whether a line opens a fenced code block, and with what
// marker. Only a run of three or more backticks or tildes at the start of the
// line (after up to three spaces of indentation, as CommonMark allows) counts.
func opensFence(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || trimmed == "" {
		return "", false
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return "", false
	}
	run := backtickRunOf(trimmed, 0, ch)
	if run < 3 {
		return "", false
	}
	return strings.Repeat(string(ch), run), true
}

// closesFence reports whether a line closes a fence opened with marker. The
// closer must be the same character and at least as long, and carry nothing
// but whitespace after it.
func closesFence(line, marker string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < len(marker) {
		return false
	}
	ch := marker[0]
	return backtickRunOf(trimmed, 0, ch) == len(trimmed)
}

// backtickRunOf returns the length of the run of ch starting at i.
func backtickRunOf(s string, i int, ch byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == ch {
		n++
	}
	return n
}
