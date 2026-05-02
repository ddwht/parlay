package parser

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/internal/config"
)

// FormatCandidateList renders an ordered list of root candidates as a
// lettered prompt, e.g.:
//
//	A: web (apps/web)
//	B: api (apps/api)
//
// Used by the interactive resolver (when walk-up returns multiple
// candidate roots) AND by status/list output. Returns an empty string
// when candidates is empty.
func FormatCandidateList(candidates []config.Candidate) string {
	if len(candidates) == 0 {
		return ""
	}
	var b strings.Builder
	for i, c := range candidates {
		letter := LetterFor(i)
		desc := c.Name
		if c.RelativePath != "" && c.RelativePath != c.Name {
			desc = fmt.Sprintf("%s (%s)", c.Name, c.RelativePath)
		}
		fmt.Fprintf(&b, "  %s: %s\n", letter, desc)
	}
	return b.String()
}

// LetterFor returns A, B, C, ... for index 0, 1, 2, ... Returns ""
// for negative indices and empty string for indices >= 26.
func LetterFor(i int) string {
	if i < 0 || i >= 26 {
		return ""
	}
	return string(rune('A' + i))
}

// IndexForLetter returns the zero-based index for "A", "B", ... or -1
// for anything else. Lowercase letters are accepted.
func IndexForLetter(s string) int {
	if len(s) != 1 {
		return -1
	}
	c := s[0]
	if c >= 'A' && c <= 'Z' {
		return int(c - 'A')
	}
	if c >= 'a' && c <= 'z' {
		return int(c - 'a')
	}
	return -1
}
