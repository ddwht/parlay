package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// FeatureRefKind distinguishes bare (no prefix) from prefixed feature references.
type FeatureRefKind string

const (
	RefKindBare     FeatureRefKind = "bare"
	RefKindPrefixed FeatureRefKind = "prefixed"
)

// FeatureReference is a parsed feature reference like "web:@parlay-tool/multi-root".
type FeatureReference struct {
	Raw         string
	RootPrefix  string
	FeatureSlug string
	Kind        FeatureRefKind
}

// ValidationError captures a forbidden cross-root reference in authored
// content (intent / dialog files). File and Line locate the offending
// reference; Ref echoes the literal token.
type ValidationError struct {
	File string
	Line int
	Ref  string
	Msg  string
}

func (e ValidationError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("%s: %s", e.Msg, e.Ref)
	}
	return fmt.Sprintf("%s: %s at %s:%d", e.Msg, e.Ref, e.File, e.Line)
}

var rootSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ParseFeatureRef parses "web:@parlay-tool/multi-root", "@feat", "feat",
// "@parlay-tool/multi-root" into a FeatureReference. An empty input or a
// malformed prefix returns an error.
func ParseFeatureRef(s string) (*FeatureReference, error) {
	if s == "" {
		return nil, fmt.Errorf("empty feature reference")
	}

	raw := s
	prefix := ""
	rest := s

	if i := strings.Index(s, ":"); i > 0 {
		candidate := s[:i]
		if rootSlugRe.MatchString(candidate) {
			prefix = candidate
			rest = s[i+1:]
		}
	}

	rest = strings.TrimPrefix(rest, "@")
	if rest == "" {
		return nil, fmt.Errorf("feature slug missing in %q", raw)
	}

	kind := RefKindBare
	if prefix != "" {
		kind = RefKindPrefixed
	}

	return &FeatureReference{
		Raw:         raw,
		RootPrefix:  prefix,
		FeatureSlug: rest,
		Kind:        kind,
	}, nil
}

// FeatureSlug strips a leading "@" from a CLI argument and returns the
// remaining slug. Used by every command that takes an `@feature` argument.
// Equivalent to strings.TrimPrefix(arg, "@").
func FeatureSlug(arg string) string {
	return strings.TrimPrefix(arg, "@")
}

var crossRootInBodyRe = regexp.MustCompile(`(?:^|[\s\(\[\{,;])([a-z][a-z0-9-]*):@([A-Za-z0-9_/-]+)`)

// ValidateNoCrossRootRefsInContent returns a ValidationError for every
// cross-root reference (e.g. "web:@some-feature") it finds in body. The
// detection is line-based: the regex matches a word-start <slug>:@ pattern
// bounded by non-identifier characters so URLs are not caught.
func ValidateNoCrossRootRefsInContent(filePath string, body []byte) []ValidationError {
	if len(body) == 0 {
		return nil
	}
	var out []ValidationError
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		matches := crossRootInBodyRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) < 3 {
				continue
			}
			ref := m[1] + ":@" + m[2]
			out = append(out, ValidationError{
				File: filePath,
				Line: i + 1,
				Ref:  ref,
				Msg:  "cross-root references in intent content are not supported in v1",
			})
		}
	}
	return out
}
