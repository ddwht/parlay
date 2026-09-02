package parser

import (
	"fmt"
	"strings"
)

// The grammar is one sentence: an annotation is `@you: text` inside a comment.
//
//	@<handle> [<kind>] [section] [ "<phrase>" ]: <text>
//
// The words between the handle and the colon are parsed as a SET, not by
// position: each is a kind or the scope word, each may appear once, in either
// order. `@dwht ask section:` and `@dwht section ask:` are the same
// annotation.

// annotationKinds is the closed set. A reviewer writes do, ask or close; a
// resolver writes done, answer or declined. `do` is the default when no kind
// is written at all, and is also writable — the kind has a name and a reviewer
// who writes it down means the same thing as one who leaves it out.
var annotationKinds = map[string]bool{
	AnnotationKindDo:       true,
	AnnotationKindAsk:      true,
	AnnotationKindClose:    true,
	AnnotationKindDone:     true,
	AnnotationKindAnswer:   true,
	AnnotationKindDeclined: true,
}

const annotationScopeWord = "section"

// looksLikeAnnotation reports whether a comment's content is a candidate
// annotation at all, as opposed to prose that happens to open with an `@`.
//
// The discriminator is what follows the handle. `@<feature>/<name>` is the
// ref vocabulary the whole tree already writes, and a wrapped comment line
// really does open with one: a buildfile in this repo carries
// `# @design-loop/design-loop respectively and stay byte-equivalent`, which
// under a looser rule became a malformed annotation nobody had written. A
// handle cannot contain a slash, so a slash after it says "this is a ref".
//
// Everything else that opens with the sigil IS a candidate, including a
// broken one: `@dwht` with no colon is reported, not skipped, because a
// reviewer who typed the sigil meant something.
func looksLikeAnnotation(body string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(body), "@")
	if !ok {
		return false
	}
	handle := leadingHandle(rest)
	if handle == "" {
		// `@` followed by nothing usable. Still a candidate: the reviewer
		// typed the sigil, and annotation-malformed says why it did not work.
		return true
	}
	return !strings.HasPrefix(rest[len(handle):], "/")
}

// parseAnnotationEntry reads one lexed annotation's grammar. It returns either
// an entry or the findings that say why there is none — never both, and never
// a guess: a reviewer who typed the sigil meant something, so a shape the
// grammar does not accept is reported rather than skipped or interpreted.
func parseAnnotationEntry(path, host string, raw rawEntry) (*AnnotationEntry, []AnnotationFinding) {
	fail := func(code, msg, fix string) (*AnnotationEntry, []AnnotationFinding) {
		return nil, []AnnotationFinding{{
			Code: code, File: path, Line: raw.line, Message: msg, Fix: fix,
		}}
	}

	if raw.inline {
		return fail(AnnotationInline,
			"an annotation shares its line with content",
			"put the annotation on its own line, directly below the text it is about")
	}

	body := strings.TrimSpace(raw.body)
	rest, ok := strings.CutPrefix(body, "@")
	if !ok {
		return fail(AnnotationMalformed, "an annotation opens with @", "write `@<handle>: <text>`")
	}

	handle := leadingHandle(rest)
	if handle == "" {
		return fail(AnnotationMalformed,
			"no handle after @ — a handle is letters, digits, and _ . -",
			"write `@<handle>: <text>`, as `parlay note --by` names a person")
	}
	rest = rest[len(handle):]

	entry := AnnotationEntry{
		By:      handle,
		Line:    raw.line,
		EndLine: raw.endLine,
		Column:  raw.column,
	}
	sawColon := false
	sawPhrase := false

	for {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		switch {
		case rest[0] == ':':
			sawColon = true
			entry.Text = normaliseAnnotationText(rest[1:])
			rest = ""
		case rest[0] == '"':
			if sawPhrase {
				return fail(AnnotationMalformed,
					"two quoted phrases in one annotation",
					"quote one phrase; it narrows the anchor to a place inside the unit, not to a range")
			}
			closing := strings.Index(rest[1:], `"`)
			if closing < 0 {
				return fail(AnnotationMalformed,
					"unterminated quote",
					`close the phrase with a second " — the phrase must occur verbatim in the anchored text`)
			}
			entry.Phrase = rest[1 : 1+closing]
			sawPhrase = true
			rest = rest[closing+2:]
		default:
			word := leadingAnnotationWord(rest)
			rest = rest[len(word):]
			switch {
			case word == annotationScopeWord:
				if host == AnnotationHostYAML {
					return fail(AnnotationMalformed,
						"`section` is a Markdown-only scope word",
						"in YAML the comment's column already says how wide the annotation is — outdent to the node you mean")
				}
				if entry.Section {
					return fail(AnnotationWordUnknown,
						"`section` appears twice",
						"write the scope word once")
				}
				entry.Section = true
			case annotationKinds[word]:
				if entry.Kind != "" {
					return fail(AnnotationWordUnknown,
						fmt.Sprintf("two kinds in one annotation: %q and %q", entry.Kind, word),
						"an annotation carries one kind")
				}
				entry.Kind = word
			default:
				return fail(AnnotationWordUnknown,
					fmt.Sprintf("%q is neither a kind (ask, do, done, answer, declined, close) nor the scope word `section`", word),
					"remove the word, or move it after the colon where it is part of the comment's text")
			}
		}
		if rest == "" {
			break
		}
	}

	if entry.Kind == "" {
		// Absent means `do`: a change is requested. The common case is the
		// short one.
		entry.Kind = AnnotationKindDo
	}

	if !sawColon {
		if entry.Kind == AnnotationKindClose {
			return &entry, nil
		}
		return fail(AnnotationMalformed,
			"no colon — a colon is required whenever there is text",
			"write `@"+handle+": <text>`; the one complete colon-less form is `@"+handle+" close`")
	}
	if entry.Text == "" && entry.Kind != AnnotationKindClose {
		return fail(AnnotationMalformed,
			"nothing after the colon",
			"say what is wrong; text is optional only after `close`")
	}
	return &entry, nil
}

// leadingHandle returns the run of handle characters at the start of s.
func leadingHandle(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '_', c == '.', c == '-':
		default:
			return s[:i]
		}
	}
	return s
}

// leadingAnnotationWord returns the word at the start of s: everything up to a
// space, a colon or a quote.
func leadingAnnotationWord(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', ':', '"':
			return s[:i]
		}
	}
	return s
}

// normaliseAnnotationText collapses a multi-line annotation into one string.
// The continuation lines have already had their comment markers removed, so
// what is left is the reviewer's own wrapping, which carries no meaning.
func normaliseAnnotationText(text string) string {
	var parts []string
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, " ")
}
