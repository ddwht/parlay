// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: amendment-artifact
//
// Parser for spec/intents/<feature>/amendments/NNN-<slug>.md — the
// append-only change ledger. Each amendment is one file, written once and
// never edited; a new change is a new file with the next sequence number.
// The frontmatter carries the machine-readable routing (affects:, the
// declared dirty set) and the body carries the human record (Change, Why,
// Acceptance).

package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Amendment is one parsed ledger entry.
type Amendment struct {
	Path string // absolute path of the file
	Seq  int    // NNN from the filename
	// FileSlug is the <slug> part of the filename; the frontmatter's
	// amendment: field must match it, so a file can never disagree with
	// its own name about which amendment it is.
	FileSlug string

	Slug       string   `yaml:"amendment"`
	Date       string   `yaml:"date"`
	Trigger    string   `yaml:"trigger"`
	Affects    []string `yaml:"affects"`
	Supersedes []string `yaml:"supersedes"`

	// SupersedesIntents names founding intents in THIS feature that this
	// amendment replaces. Deliberately a separate field from Affects:
	// Affects resolves contract entries and drives the dirty set, splice
	// targeting, rebuild scoping and overlap detection, and an intent
	// retirement participates in none of those. Sharing one field would
	// make dirty_set ambiguous and push an "except intent refs" branch
	// into every consumer.
	//
	// Named supersedes_intents rather than retires because the amendment is
	// the replacing decision, parallel to Supersedes above: a commitment
	// must not be able to disappear without a successor taking its place.
	// The superseded intents.md is never opened for writing — supersession
	// records a later decision beside the frozen file and grants it no
	// exemption from the byte-integrity check.
	SupersedesIntents []string `yaml:"supersedes_intents"`

	// Body sections. Change and Why are verbatim prose; Acceptance is the
	// bullet list the apply step lands as verify: entries on the affected
	// contract artifact entries.
	Change     string
	Why        string
	Acceptance []string
}

// amendmentFilePattern is the only accepted filename shape. Three digits keep
// lexical order equal to sequence order in every directory listing.
var amendmentFilePattern = regexp.MustCompile(`^(\d{3})-([a-z0-9][a-z0-9-]*)\.md$`)

// AmendmentsDir returns the ledger directory for a feature directory.
func AmendmentsDir(featureDir string) string {
	return filepath.Join(featureDir, "amendments")
}

// AmendmentFileNameValid reports whether a filename is one the ledger
// loader will read. Exported so check-amendments can name files that are
// silently invisible to the ledger.
func AmendmentFileNameValid(name string) bool {
	return amendmentFilePattern.MatchString(name)
}

// LoadFeatureAmendments reads every amendment in a feature's ledger, sorted
// by sequence number. A missing amendments/ directory is an empty ledger,
// not an error. Files not matching the NNN-<slug>.md pattern are ignored
// here — check-amendments reports them; the loader's job is only to read
// what is well-named.
func LoadFeatureAmendments(featureDir string) ([]Amendment, error) {
	dir := AmendmentsDir(featureDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read amendments dir %s: %w", dir, err)
	}
	var out []Amendment
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := amendmentFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read amendment %s: %w", path, err)
		}
		a, err := ParseAmendmentBytes(path, content)
		if err != nil {
			return nil, err
		}
		a.Seq, _ = strconv.Atoi(m[1])
		a.FileSlug = m[2]
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// ParseAmendmentBytes parses one amendment file: YAML frontmatter between
// `---` fences, then markdown sections `## Change`, `## Why`, `## Acceptance`.
func ParseAmendmentBytes(path string, content []byte) (*Amendment, error) {
	text := string(content)
	frontmatter, body, err := splitAmendmentFrontmatter(text)
	if err != nil {
		return nil, fmt.Errorf("parse amendment %s: %w", path, err)
	}
	var a Amendment
	if err := yaml.Unmarshal([]byte(frontmatter), &a); err != nil {
		return nil, fmt.Errorf("parse amendment %s frontmatter: %w", path, err)
	}
	a.Path = path

	section := ""
	var change, why []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
			continue
		}
		switch section {
		case "change":
			change = append(change, line)
		case "why":
			why = append(why, line)
		case "acceptance":
			if strings.HasPrefix(trimmed, "- ") {
				a.Acceptance = append(a.Acceptance, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			}
		}
	}
	a.Change = strings.TrimSpace(strings.Join(change, "\n"))
	a.Why = strings.TrimSpace(strings.Join(why, "\n"))
	return &a, nil
}

// splitFrontmatter separates the YAML frontmatter from the markdown body.
func splitAmendmentFrontmatter(text string) (frontmatter, body string, err error) {
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return "", "", fmt.Errorf("missing frontmatter: file must start with ---")
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(text, "---\r\n"), "---\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("unterminated frontmatter: no closing ---")
	}
	frontmatter = rest[:idx]
	body = rest[idx+len("\n---"):]
	body = strings.TrimPrefix(strings.TrimPrefix(body, "\r\n"), "\n")
	return frontmatter, body, nil
}

// AmendmentRef is one parsed affects: reference:
// @<feature>/<kind>:<name> where kind is operation|surface|infrastructure|domain.
type AmendmentRef struct {
	Raw     string
	Feature string // may contain slashes (initiative/feature)
	Kind    string
	Name    string
}

// ParseAmendmentRef parses an affects: entry. The kind segment is what
// separates an amendment ref from an intent-style source ref: affects
// points at contract artifact entries, never at intents, so a ref without
// a <kind>: segment is malformed by construction.
func ParseAmendmentRef(raw string) (AmendmentRef, error) {
	ref := AmendmentRef{Raw: raw}
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "@") {
		return ref, fmt.Errorf("ref %q: must start with @", raw)
	}
	s = s[1:]
	slash := strings.LastIndex(s, "/")
	if slash <= 0 {
		return ref, fmt.Errorf("ref %q: expected @<feature>/<kind>:<name>", raw)
	}
	ref.Feature = s[:slash]
	kindName := s[slash+1:]
	colon := strings.Index(kindName, ":")
	if colon <= 0 || colon == len(kindName)-1 {
		return ref, fmt.Errorf("ref %q: expected <kind>:<name> after the feature, with kind one of operation|surface|infrastructure|domain", raw)
	}
	ref.Kind = kindName[:colon]
	ref.Name = kindName[colon+1:]
	switch ref.Kind {
	case "operation", "surface", "infrastructure", "domain":
		return ref, nil
	default:
		return ref, fmt.Errorf("ref %q: unknown kind %q — expected operation|surface|infrastructure|domain", raw, ref.Kind)
	}
}
