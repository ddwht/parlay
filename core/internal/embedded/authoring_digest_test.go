package embedded

import (
	"io/fs"
	"strings"
	"testing"
)

// TestAuthoringDigest_ExtractsMarkedBlocksVerbatim pins the core property: a
// digest is DERIVED, never authored. Every marked block appears in it
// byte-for-byte, so a digest cannot say something its schema does not.
func TestAuthoringDigest_ExtractsMarkedBlocksVerbatim(t *testing.T) {
	schema := `# Demo Schema

Rationale prose an author does not need.

## Structure
<!-- parlay:normative -->
| Field | Required |
|---|---|
| ` + "`id`" + ` | Yes |
<!-- /parlay:normative -->

## History

Long explanation of why the field exists.
`
	d := BuildAuthoringDigest("demo.schema.md", schema)
	if d.Title != "Demo Schema" {
		t.Errorf("title = %q", d.Title)
	}
	if len(d.Blocks) != 1 {
		t.Fatalf("expected 1 marked block, got %d", len(d.Blocks))
	}
	if !strings.Contains(d.Blocks[0], "| `id` | Yes |") {
		t.Errorf("block lost its table: %q", d.Blocks[0])
	}
	rendered := RenderAuthoringDigest(d)
	if strings.Contains(rendered, "Long explanation") {
		t.Error("unmarked prose must not reach the digest")
	}
	if !strings.Contains(rendered, "| `id` | Yes |") {
		t.Error("the marked table must reach the digest verbatim")
	}
}

// TestAuthoringDigest_StripsRationaleWithinNormative pins the nested
// exclusion: a normative section may contain history, and the history is what
// gets dropped — not the rules around it.
func TestAuthoringDigest_StripsRationaleWithinNormative(t *testing.T) {
	schema := "# Demo\n\n## Structure\n" +
		normativeOpen + "\n" +
		"Rule one: every entry declares an id.\n\n" +
		rationaleOpen + "\n" +
		"This field used to be called ident, which broke three projects.\n" +
		rationaleClose + "\n\n" +
		"Rule two: ids are unique.\n" +
		normativeClose + "\n"

	d := BuildAuthoringDigest("demo.schema.md", schema)
	out := RenderAuthoringDigest(d)
	for _, want := range []string{"Rule one", "Rule two"} {
		if !strings.Contains(out, want) {
			t.Errorf("rationale excision dropped %q — rules stated in prose must survive", want)
		}
	}
	if strings.Contains(out, "used to be called ident") {
		t.Error("rationale-marked history must not reach the digest")
	}
}

func TestAuthoringDigest_UnmarkedSchemaSaysSo(t *testing.T) {
	d := BuildAuthoringDigest("bare.schema.md", "# Bare\n\nAll prose, no markers.\n")
	out := RenderAuthoringDigest(d)
	if !strings.Contains(out, "not yet marked up") {
		t.Error("an unmarked schema's digest must say so and point at the full schema")
	}
	if !strings.Contains(out, "bare.schema.md") {
		t.Error("the fallback must name the schema to read instead")
	}
}

// TestSchemaMarkersAreBalanced is the ratchet against a half-edited schema:
// an unbalanced or misplaced fence silently truncates a digest, which reads
// as complete.
func TestSchemaMarkersAreBalanced(t *testing.T) {
	entries, err := fs.ReadDir(schemasFS, "schemas")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".schema.md") {
			continue
		}
		data, err := schemasFS.ReadFile("schemas/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)

		if got, want := strings.Count(body, normativeOpen), strings.Count(body, normativeClose); got != want {
			t.Errorf("%s: %d normative open markers vs %d close", e.Name(), got, want)
		}
		if got, want := strings.Count(body, rationaleOpen), strings.Count(body, rationaleClose); got != want {
			t.Errorf("%s: %d rationale open markers vs %d close", e.Name(), got, want)
		}

		// A marker inside a fenced code block would be rendered as content
		// rather than obeyed, and the extractor would cut in the wrong place.
		inFence := false
		for i, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if inFence && strings.Contains(line, "parlay:normative") {
				t.Errorf("%s:%d: normative marker inside a code fence", e.Name(), i+1)
			}
			if inFence && strings.Contains(line, "parlay:rationale") {
				t.Errorf("%s:%d: rationale marker inside a code fence", e.Name(), i+1)
			}
		}
	}
}

// TestAuthoringDigestsAreSmallerThanTheirSchemas is the size ratchet. A
// digest that approaches its schema's size means rationale has migrated
// inside the fences — the slow failure this mechanism is most exposed to,
// because each individual paragraph looks defensible.
//
// The threshold is deliberately loose (a digest may legitimately be most of a
// schema that is almost entirely field tables) and exists to catch drift
// toward "the digest IS the schema", not to police individual edits.
func TestAuthoringDigestsAreSmallerThanTheirSchemas(t *testing.T) {
	digests, err := AllAuthoringDigests()
	if err != nil {
		t.Fatal(err)
	}
	if len(digests) == 0 {
		t.Fatal("no digests built — the extractor has drifted from the schema set")
	}
	marked := 0
	for _, d := range digests {
		if len(d.Blocks) == 0 {
			continue // unmarked schemas render a short pointer; nothing to ratchet
		}
		marked++
		if d.DigestBytes >= d.SourceBytes {
			t.Errorf("%s: digest (%d B) is not smaller than its schema (%d B) — rationale has migrated inside the fences",
				d.Schema, d.DigestBytes, d.SourceBytes)
		}
	}
	if marked == 0 {
		t.Fatal("no schema carries normative markers — the ratchet would assert nothing")
	}
}

// TestPhaseLoadedSchemasAreMarked pins the read-lists the modules actually
// use: if build-feature or generate-code names a schema at step 1, that
// schema must have a real digest to read instead. An unmarked one silently
// sends the phase back to the full file.
func TestPhaseLoadedSchemasAreMarked(t *testing.T) {
	phaseLoaded := []string{
		"buildfile.schema.md", "adapter.schema.md", "testcases.schema.md",
		"surface.schema.md", "intent.schema.md", "dialog.schema.md",
		"blueprint.schema.md",
	}
	digests, err := AllAuthoringDigests()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]AuthoringDigest{}
	for _, d := range digests {
		byName[d.Schema] = d
	}
	for _, name := range phaseLoaded {
		d, ok := byName[name]
		if !ok {
			t.Errorf("%s is named in a phase read-list but has no digest", name)
			continue
		}
		if len(d.Blocks) == 0 {
			t.Errorf("%s is loaded by a phase but carries no parlay:normative markers — "+
				"its digest is a pointer back to the full file, so the phase saves nothing", name)
		}
	}
}
