package embedded

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ddwht/parlay/core/internal/atomicfile"
)

// Authoring digests.
//
// DIGEST.md solved half of this problem: an agent that has just been handed a
// validation error should read a 21 KB routing table, not the 337 KB corpus,
// to learn which schema to open. What it did not solve is the read that
// happens BEFORE any error exists — build-feature step 1 loads seven schemas
// whole (~186 KB) and generate-code step 1 loads three (~132 KB), every run,
// to author artifacts against them.
//
// Most of that weight is not authoring instruction. The schemas carry their
// own history: why a field exists, which earlier reading was wrong, what a
// previous comment claimed and why it was mistaken. That prose is genuinely
// valuable — it is what stops a future editor re-introducing a removed
// mechanism — and it is exactly what an agent filling in a buildfile does not
// need in context.
//
// So: mark the parts that ARE authoring instruction, and extract those.
//
// The marker convention is deliberate rather than heuristic, and the reason is
// recorded in digest.go: an attempt to infer closed vocabularies from prose
// ("the backticked tokens on a line that says closed set") produced entries
// mixing schema filenames with field names, and "a reference with junk in it
// is worse than a shorter one, because the junk teaches people to distrust the
// parts that are correct". The schemas have no shared section structure to
// anchor on either — adapter.schema.md has 18 `##` headings, capabilities has
// 8, and they name different things. What digest.go called for was for "the
// schemas to mark their closed sets in a parseable way, which is a schema
// change rather than a digest change". These fences are that schema change.
//
// A digest is never hand-edited and never committed: it is derived at deploy
// time from the marked blocks, byte-for-byte, so it cannot drift from its
// schema. If a digest is wrong, the schema's markers are wrong.

// normativeOpen and normativeClose delimit a block of authoring-normative
// content: field tables, closed vocabularies, required shapes, invariants.
const (
	normativeOpen  = "<!-- parlay:normative -->"
	normativeClose = "<!-- /parlay:normative -->"
)

// rationaleOpen and rationaleClose mark a region INSIDE a normative block
// that is history rather than instruction: why a field exists, which earlier
// reading was wrong, what a previous revision claimed.
//
// The two-marker model exists because the alternative failed in practice.
// Marking whole sections cut the buildfile schema by only 20%: its field
// tables and YAML shapes sit interleaved with long "*Which reading won and
// why*" paragraphs, and a section-level fence has to take both or neither.
// Excluding all prose instead was not an option either — plenty of real
// rules are stated in sentences ("Required for new buildfiles", the plan
// integrity list), and dropping those would produce a digest that authorizes
// invalid artifacts.
//
// So the normative fence says "an author needs this section", and the
// rationale fence says "except this part, which explains rather than
// instructs". Both are explicit and both are reviewable in the schema diff.
const (
	rationaleOpen  = "<!-- parlay:rationale -->"
	rationaleClose = "<!-- /parlay:rationale -->"
)

// stripRationale removes rationale-marked regions from a normative block.
// An unclosed rationale marker drops the remainder of the block — the same
// fail-toward-less choice the unclosed normative fence makes, and the
// ratchet reports the imbalance either way.
func stripRationale(block string) string {
	var b strings.Builder
	rest := block
	for {
		start := strings.Index(rest, rationaleOpen)
		if start < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:start])
		afterOpen := rest[start+len(rationaleOpen):]
		end := strings.Index(afterOpen, rationaleClose)
		if end < 0 {
			break
		}
		rest = afterOpen[end+len(rationaleClose):]
	}
	// Collapse the blank-line runs the excision leaves behind.
	out := regexp.MustCompile(`\n{3,}`).ReplaceAllString(b.String(), "\n\n")
	return strings.TrimSpace(out)
}

// authoringDigestSuffix is the deployed file suffix. Digests land in a
// digests/ subdirectory of the schemas directory so a glob over
// *.schema.md — which several call sites use — cannot pick them up.
const authoringDigestSuffix = ".digest.md"

// AuthoringDigestsDir is the deployed location, relative to the schemas dir.
const AuthoringDigestsDir = "digests"

// schemaTitle matches the first H1 of a schema.
var schemaTitle = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// AuthoringDigest is one schema reduced to what an author needs.
type AuthoringDigest struct {
	// Schema is the source file name, e.g. "buildfile.schema.md".
	Schema string
	// Title is the schema's H1.
	Title string
	// Blocks are the marked normative regions, verbatim and in source order.
	Blocks []string
	// SourceBytes and DigestBytes support the size ratchet.
	SourceBytes int
	DigestBytes int
}

// BuildAuthoringDigest extracts one schema's normative blocks. A schema with
// no markers yields a digest with no blocks — reported rather than treated as
// an error, so schemas can be marked incrementally and the ratchet can name
// the ones still unmarked.
func BuildAuthoringDigest(name string, body string) AuthoringDigest {
	d := AuthoringDigest{Schema: name, SourceBytes: len(body)}
	if m := schemaTitle.FindStringSubmatch(body); m != nil {
		d.Title = strings.TrimSpace(m[1])
	}

	rest := body
	for {
		start := strings.Index(rest, normativeOpen)
		if start < 0 {
			break
		}
		afterOpen := rest[start+len(normativeOpen):]
		end := strings.Index(afterOpen, normativeClose)
		if end < 0 {
			// An unclosed fence takes the rest of the file rather than
			// silently dropping the block: a truncated digest that looks
			// complete is the failure mode this whole file is written
			// against. The ratchet reports the imbalance.
			if block := stripRationale(afterOpen); block != "" {
				d.Blocks = append(d.Blocks, block)
			}
			break
		}
		if block := stripRationale(afterOpen[:end]); block != "" {
			d.Blocks = append(d.Blocks, block)
		}
		rest = afterOpen[end+len(normativeClose):]
	}

	d.DigestBytes = len(RenderAuthoringDigest(d))
	return d
}

// RenderAuthoringDigest produces the deployed markdown.
func RenderAuthoringDigest(d AuthoringDigest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — authoring digest\n\n", d.Title)
	fmt.Fprintf(&b, "Derived from `%s` at deploy time. Never hand-edit: edit the schema's\n", d.Schema)
	b.WriteString("`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.\n\n")
	b.WriteString("This is what you need to AUTHOR the artifact — field tables, closed\n")
	b.WriteString("vocabularies, required shapes, invariants. It deliberately omits the\n")
	b.WriteString("schema's rationale and history. Open the full schema when a validator\n")
	b.WriteString("finding routes you there, when you are changing the schema itself, or\n")
	b.WriteString("when you need to know WHY a rule exists rather than what it is.\n")

	if len(d.Blocks) == 0 {
		b.WriteString("\n**This schema is not yet marked up.** Read `")
		b.WriteString(d.Schema)
		b.WriteString("` in full.\n")
		return b.String()
	}
	for _, block := range d.Blocks {
		b.WriteString("\n---\n\n")
		b.WriteString(block)
		b.WriteString("\n")
	}
	return b.String()
}

// AllAuthoringDigests builds a digest for every embedded schema.
func AllAuthoringDigests() ([]AuthoringDigest, error) {
	entries, err := fs.ReadDir(schemasFS, "schemas")
	if err != nil {
		return nil, err
	}
	var out []AuthoringDigest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".schema.md") {
			continue
		}
		data, err := schemasFS.ReadFile("schemas/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, BuildAuthoringDigest(e.Name(), string(data)))
	}
	return out, nil
}

// digestFileName maps a schema name to its digest name:
// "buildfile.schema.md" → "buildfile.digest.md".
func digestFileName(schemaName string) string {
	return strings.TrimSuffix(schemaName, ".schema.md") + authoringDigestSuffix
}

// WriteAuthoringDigests materializes one digest per schema under
// <schemasDir>/digests/ and returns how many it actually wrote. Same
// write-if-changed discipline as WriteSchemas: a no-op upgrade reports 0.
//
// Digests are written for unmarked schemas too. The alternative — deploying
// only the marked ones — makes a missing digest ambiguous between "this
// schema needs no digest" and "nobody has marked it yet", and a module that
// reads digests would then silently skip a schema it needs.
func WriteAuthoringDigests(schemasDir string) (int, error) {
	digests, err := AllAuthoringDigests()
	if err != nil {
		return 0, err
	}
	dir := filepath.Join(schemasDir, AuthoringDigestsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	written := 0
	for _, d := range digests {
		wrote, err := atomicfile.WriteIfChanged(
			filepath.Join(dir, digestFileName(d.Schema)),
			[]byte(RenderAuthoringDigest(d)),
		)
		if err != nil {
			return written, err
		}
		if wrote {
			written++
		}
	}
	return written, nil
}

// PruneStaleAuthoringDigests removes digests whose schema is gone — the same
// retirement path PruneStaleSchemas gives schemas, for the same reason: a
// digest describing a deleted schema is read as authoritative.
func PruneStaleAuthoringDigests(schemasDir string) (int, error) {
	names, err := SchemaNames()
	if err != nil {
		return 0, err
	}
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[digestFileName(n)] = true
	}
	dir := filepath.Join(schemasDir, AuthoringDigestsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Missing directory on a project deployed before digests existed —
		// nothing to prune; the next WriteAuthoringDigests creates it.
		return 0, nil
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || wanted[name] || !strings.HasSuffix(name, authoringDigestSuffix) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
