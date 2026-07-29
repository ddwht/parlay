package embedded

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// The schema digest.
//
// The 24 schemas are ~269 KB — roughly 67k tokens if a phase loaded them all.
// The project convention is to load them on demand and not carry them across
// commands, which is right for authoring but leaves every phase paying to
// re-read whichever files it needs, once per phase-group, on every run.
//
// The regression run showed what the re-reading was actually buying, and it
// was not much: build subagents reverse-engineered the validator at runtime
// instead — authoring a v2 buildfile, getting it rejected, retrying v1;
// omitting `models:`, getting missing-model-reference, re-adding it. Each
// build phase paid that independently. What they needed was not the prose but
// one specific thing from it: which diagnostics exist and when each fires.
//
// So the digest extracts exactly that — 48 codes across 24 schemas, in 9 KB
// instead of 263. It is mechanically derived, never
// hand-maintained — a hand-written cheat sheet would be one more artifact to
// drift from the schemas it summarizes, which is the failure this whole
// consolidation is about.
//
// It is a routing table, not a replacement. An agent authoring a buildfile
// still reads buildfile.schema.md. The digest is what it reads *first*, to
// know which schema to open and which mistakes are pre-checkable.

// digestCodeTableHeader matches the canonical error-code table header.
var digestCodeTableHeader = regexp.MustCompile(`^\|\s*Code\s*\|`)

// digestCodeRow matches a table row whose first cell is a backticked code.
var digestCodeRow = regexp.MustCompile("^\\|\\s*`([a-z][a-z0-9-]+)`\\s*\\|(.*)")

// Closed vocabularies are deliberately NOT extracted.
//
// They are the other half of what an agent guesses wrong — an open-looking
// field with a fixed set of legal values produces a plausible file that fails
// validation — so an extractor for them would be valuable. But "the backticked
// tokens on a line that says closed set" is not that extractor: run over the
// real corpus it produced entries like "schema_version: | schema-versioning.schema.md"
// and "steps: | type:", mixing schema filenames and field names in as if they
// were enum members.
//
// A reference with junk in it is worse than a shorter one, because the junk
// teaches people to distrust the parts that are correct. The phrasings vary
// too much across schemas for a line-level heuristic; extracting them properly
// needs the schemas to mark their closed sets in a parseable way, which is a
// schema change rather than a digest change.

// SchemaDigestEntry is one schema's summary.
type SchemaDigestEntry struct {
	File    string            `json:"file"`
	Title   string            `json:"title"`
	Purpose string            `json:"purpose,omitempty"`
	Codes   map[string]string `json:"codes,omitempty"`
}

// SchemaDigest is the whole corpus summarized.
type SchemaDigest struct {
	Schemas    []SchemaDigestEntry `json:"schemas"`
	TotalBytes int                 `json:"total_schema_bytes"`
	CodeCount  int                 `json:"total_error_codes"`
}

// BuildSchemaDigest derives the digest from the embedded schemas.
func BuildSchemaDigest() (*SchemaDigest, error) {
	entries, err := fs.ReadDir(schemasFS, "schemas")
	if err != nil {
		return nil, err
	}
	d := &SchemaDigest{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".schema.md") {
			continue
		}
		data, err := schemasFS.ReadFile("schemas/" + e.Name())
		if err != nil {
			return nil, err
		}
		d.TotalBytes += len(data)
		entry := digestOne(e.Name(), string(data))
		d.CodeCount += len(entry.Codes)
		d.Schemas = append(d.Schemas, entry)
	}
	sort.Slice(d.Schemas, func(i, j int) bool { return d.Schemas[i].File < d.Schemas[j].File })
	return d, nil
}

func digestOne(name, body string) SchemaDigestEntry {
	entry := SchemaDigestEntry{File: name, Codes: map[string]string{}}
	lines := strings.Split(body, "\n")

	inCodeTable := false
	for i, line := range lines {
		// Title: the first H1.
		if entry.Title == "" && strings.HasPrefix(line, "# ") {
			entry.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			// Purpose: the first non-blank prose line after the title that is
			// not a fence, a heading, or a file-path note.
			for _, next := range lines[i+1:] {
				t := strings.TrimSpace(next)
				if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "```") ||
					strings.HasPrefix(t, "File:") || strings.HasPrefix(t, "<!--") {
					continue
				}
				entry.Purpose = firstSentence(t)
				break
			}
			continue
		}

		if digestCodeTableHeader.MatchString(line) {
			inCodeTable = true
			continue
		}
		if inCodeTable {
			if m := digestCodeRow.FindStringSubmatch(line); m != nil {
				entry.Codes[m[1]] = firstSentence(strings.TrimSpace(strings.Trim(m[2], "| ")))
				continue
			}
			if strings.TrimSpace(line) == "" || !strings.HasPrefix(strings.TrimSpace(line), "|") {
				inCodeTable = false
			}
			continue
		}

	}
	return entry
}

// firstSentence trims a cell or paragraph to its first sentence, so the digest
// stays scannable. A digest nobody can scan is 24 schemas again.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	const max = 160
	if len(s) > max {
		cut := strings.LastIndex(s[:max], " ")
		if cut < 0 {
			cut = max
		}
		return s[:cut] + "…"
	}
	return s
}

// RenderSchemaDigestMarkdown formats the digest as the one-page reference a
// phase reads before opening any schema.
func RenderSchemaDigestMarkdown(d *SchemaDigest) string {
	var b strings.Builder
	b.WriteString("# Schema digest\n\n")
	b.WriteString("_Generated by `parlay internal schema-digest`. Do not edit — it is derived from the schemas and regenerated on every upgrade._\n\n")
	fmt.Fprintf(&b, "%d schemas, %d error codes, %s of source.\n\n",
		len(d.Schemas), d.CodeCount, humanBytes(d.TotalBytes))
	b.WriteString("Read this first to find the schema you need and the diagnostics it can produce. " +
		"Open the full schema when you are authoring that artifact — this is a routing table, not a substitute.\n\n")

	b.WriteString("## Diagnostics by schema\n\n")
	for _, s := range d.Schemas {
		if len(s.Codes) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", s.File)
		if s.Purpose != "" {
			fmt.Fprintf(&b, "%s\n\n", s.Purpose)
		}
		if len(s.Codes) > 0 {
			codes := make([]string, 0, len(s.Codes))
			for c := range s.Codes {
				codes = append(codes, c)
			}
			sort.Strings(codes)
			b.WriteString("| Code | Fires when |\n|---|---|\n")
			for _, c := range codes {
				fmt.Fprintf(&b, "| `%s` | %s |\n", c, s.Codes[c])
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Schemas with no diagnostics of their own\n\n")
	for _, s := range d.Schemas {
		if len(s.Codes) == 0 {
			fmt.Fprintf(&b, "- `%s` — %s\n", s.File, s.Purpose)
		}
	}
	return b.String()
}

func humanBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.0f KB", float64(n)/1024)
}
