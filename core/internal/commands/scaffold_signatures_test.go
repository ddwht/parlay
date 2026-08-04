package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func sigFixture(t *testing.T, files map[string]string) (featureDir, root, adapter string) {
	t.Helper()
	root = t.TempDir()
	featureDir = filepath.Join(root, "spec", "intents", "f")
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(featureDir, name)
		if name == "domain-model.yaml" {
			p = filepath.Join(root, name)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	adapter = filepath.Join(root, "a.adapter.yaml")
	if err := os.WriteFile(adapter, []byte("name: a\nversion: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return featureDir, root, adapter
}

// Absent artifacts must be omitted, not hashed as empty. The schema's
// "when <artifact> exists" column is load-bearing: a signature recorded for
// a file the feature does not have can never match, so the freshness gate
// would refuse to generate that feature forever.
func TestSignaturesOmitAbsentArtifacts(t *testing.T) {
	fd, root, adapter := sigFixture(t, map[string]string{
		"intents.md": "# i",
		"dialogs.md": "# d",
	})
	sigs, err := computeSourceSignatures(fd, root, adapter, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"intents", "dialogs", "adapter-version"} {
		if sigs[want] == "" {
			t.Errorf("missing signature for %s", want)
		}
	}
	for _, absent := range []string{"surface", "capabilities", "infrastructure", "domain", "layout"} {
		if _, ok := sigs[absent]; ok {
			t.Errorf("recorded a signature for %s, which does not exist", absent)
		}
	}
}

// Content, not mtime — the schema is explicit that re-saving an unedited
// file must leave the signature identical, or every fresh checkout reads as
// stale.
func TestSignaturesAreContentBased(t *testing.T) {
	fd, root, adapter := sigFixture(t, map[string]string{"intents.md": "# i"})
	first, err := computeSourceSignatures(fd, root, adapter, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Rewrite identical bytes; mtime changes, content does not.
	if err := os.WriteFile(filepath.Join(fd, "intents.md"), []byte("# i"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := computeSourceSignatures(fd, root, adapter, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first["intents"] != second["intents"] {
		t.Fatalf("signature changed on an identical rewrite: %s vs %s", first["intents"], second["intents"])
	}

	if err := os.WriteFile(filepath.Join(fd, "intents.md"), []byte("# i changed"), 0644); err != nil {
		t.Fatal(err)
	}
	third, _ := computeSourceSignatures(fd, root, adapter, nil)
	if first["intents"] == third["intents"] {
		t.Fatal("signature did not change when the content did")
	}
}

// surface.yaml wins over surface.md — the migration ships both shapes, and
// hashing the legacy file on a migrated feature would make the gate track
// an artifact nothing reads any more.
func TestSurfaceYAMLPreferredOverMarkdown(t *testing.T) {
	fd, root, adapter := sigFixture(t, map[string]string{
		"intents.md":   "# i",
		"surface.md":   "legacy",
		"surface.yaml": "pages: []",
	})
	sigs, _ := computeSourceSignatures(fd, root, adapter, nil)
	yamlOnly, _, _ := sigFixture(t, map[string]string{"intents.md": "# i", "surface.yaml": "pages: []"})
	ySigs, _ := computeSourceSignatures(yamlOnly, root, adapter, nil)
	if sigs["surface"] != ySigs["surface"] {
		t.Fatal("surface signature came from surface.md while surface.yaml exists")
	}
}

// Writing the block must not disturb the rest of the buildfile: it is
// mostly agent-authored judgment, and a round-trip through a struct would
// silently drop any field the struct does not model.
func TestWriteSignaturesPreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	bf := filepath.Join(dir, "buildfile.yaml")
	original := `feature: f
adapter: angular-clarity
components:
  list:
    kind: page
    unknown-future-field: keep-me
cross-cutting:
  - id: x
    transform: t
source-signatures:
  intents: oldhash
`
	if err := os.WriteFile(bf, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceSignatures(bf, map[string]string{"intents": "newhash", "adapter-version": "av"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(bf)
	body := string(got)

	for _, must := range []string{"unknown-future-field: keep-me", "cross-cutting:", "id: x", "newhash", `adapter-version: "av"`} {
		if !strings.Contains(body, must) {
			t.Errorf("lost %q after writing signatures:\n%s", must, body)
		}
	}
	if strings.Contains(body, "oldhash") {
		t.Error("stale signature survived the rewrite")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
	if _, ok := parsed["components"]; !ok {
		t.Error("components section disappeared")
	}
}

// A buildfile is a long human-reviewed document. Writing signatures must
// leave every other byte alone — the first implementation re-encoded the
// YAML, which preserved all values while reflowing folded descriptions and
// dropping the blank lines that group components, turning the next review
// diff into noise.
func TestWriteSignaturesLeavesEveryOtherByteUntouched(t *testing.T) {
	dir := t.TempDir()
	bf := filepath.Join(dir, "buildfile.yaml")
	original := `feature: f

components:
  list:
    kind: page
    description: >
      A folded description that a YAML round-trip
      would rewrap onto one very long line.

    # An explanatory comment the author wrote.
    elements: [a, b]

cross-cutting:
  - id: x
source-signatures:
  intents: "sha256:old"

`
	if err := os.WriteFile(bf, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceSignatures(bf, map[string]string{"intents": "sha256:new"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(bf)

	wantPrefix := original[:strings.Index(original, "source-signatures:")]
	if !strings.HasPrefix(string(got), wantPrefix) {
		t.Fatalf("content before the block changed.\n--- want prefix ---\n%s\n--- got ---\n%s", wantPrefix, got)
	}
	if !strings.Contains(string(got), "sha256:new") || strings.Contains(string(got), "sha256:old") {
		t.Error("signature not replaced")
	}
}

// Appending the block to a buildfile that has none must not disturb the
// existing content either.
func TestWriteSignaturesAppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	bf := filepath.Join(dir, "buildfile.yaml")
	original := "feature: f\ncomponents:\n  list:\n    kind: page\n"
	os.WriteFile(bf, []byte(original), 0644)
	if err := writeSourceSignatures(bf, map[string]string{"intents": "sha256:x"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(bf)
	if !strings.HasPrefix(string(got), original) {
		t.Fatalf("existing content disturbed:\n%s", got)
	}
	if !strings.Contains(string(got), "source-signatures:") {
		t.Error("block not appended")
	}
}

// Emission order follows the schema, so regenerating a hand-written block
// diffs as changed hashes rather than as a reshuffle.
func TestSignatureFieldOrderMatchesSchema(t *testing.T) {
	dir := t.TempDir()
	bf := filepath.Join(dir, "buildfile.yaml")
	os.WriteFile(bf, []byte("feature: f\n"), 0644)
	if err := writeSourceSignatures(bf, map[string]string{
		"adapter-version": "av", "intents": "i", "domain": "d", "surface": "s",
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(bf)
	idx := func(s string) int { return strings.Index(string(body), s) }
	if !(idx("intents:") < idx("surface:") && idx("surface:") < idx("domain:") && idx("domain:") < idx("adapter-version:")) {
		t.Fatalf("fields not in schema order:\n%s", body)
	}
}

// A project with no adapter must fail loudly. adapter-version is the one
// required field, and an adapter upgrade changes every emitted file — a
// signature block silently missing it would pass the gate after an upgrade
// that invalidated everything.
func TestMissingAdapterIsAnError(t *testing.T) {
	fd, root, _ := sigFixture(t, map[string]string{"intents.md": "# i"})
	if _, err := computeSourceSignatures(fd, root, "", nil); err == nil {
		t.Fatal("expected an error when no adapter could be found")
	}
}

// The block-printing failure, reduced: two commits to a hand-written
// geometry engine invalidated fixture numbers in two dependent buildfiles,
// and nothing reported it. The engine's files carried no generation marker,
// so no ingestion path could return them and no hash could move.
//
// source-signatures is the mechanism that actually blocks — check-drift and
// diff are advisory by explicit design — so this asserts at the gate.
func TestEngineChangeMovesTheFeatureSignature(t *testing.T) {
	fd, root, adapter := sigFixture(t, map[string]string{"intents.md": "# i"})

	units := map[string]string{"geometry-engine": "sha256:aaa"}
	before, err := computeSourceSignatures(fd, root, adapter, units)
	if err != nil {
		t.Fatal(err)
	}
	if before["authored"] == "" {
		t.Fatal("a project with a unit must carry an authored signature")
	}

	// The engine changes. Nothing about the feature's own spec moved.
	units["geometry-engine"] = "sha256:bbb"
	after, err := computeSourceSignatures(fd, root, adapter, units)
	if err != nil {
		t.Fatal(err)
	}
	if after["authored"] == before["authored"] {
		t.Error("an engine change must move the consuming feature's signature, or the build stays green over stale fixtures")
	}
	for _, unchanged := range []string{"intents", "adapter-version"} {
		if after[unchanged] != before[unchanged] {
			t.Errorf("%s moved, but only the unit changed", unchanged)
		}
	}
}

// A project with no units records no authored signature at all. Recording
// one would make every existing buildfile stale on upgrade, which is the
// failure mode the "when <artifact> exists" rule exists to prevent.
func TestNoUnitsMeansNoAuthoredSignature(t *testing.T) {
	fd, root, adapter := sigFixture(t, map[string]string{"intents.md": "# i"})
	sigs, err := computeSourceSignatures(fd, root, adapter, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := sigs["authored"]; present {
		t.Error("a project with no units must not carry an authored signature")
	}
}

// Map iteration order must not reach the signature: an unstable value here
// would rewrite the block on every run and fire the gate at random.
func TestAuthoredSignatureIsOrderIndependent(t *testing.T) {
	a := combineUnitHashes(map[string]string{"engine": "sha256:1", "codec": "sha256:2"})
	b := combineUnitHashes(map[string]string{"codec": "sha256:2", "engine": "sha256:1"})
	if a != b {
		t.Errorf("signature depends on map order: %s vs %s", a, b)
	}
	// And two different unit sets must not collide once concatenated.
	c := combineUnitHashes(map[string]string{"engine": "sha256:1sha256:2", "codec": ""})
	if a == c {
		t.Error("unit ids and hashes must be delimited, not concatenated")
	}
}
