package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/parser"
)

// capsFixture lays down a feature with two operations and a saved baseline.
// yamlUnmarshalBaseline reads a baseline back the way the tool does.
func yamlUnmarshalBaseline(data []byte, out *Baseline) error {
	return yaml.Unmarshal(data, out)
}

func capsFixture(t *testing.T, ops string) (string, string) {
	t.Helper()
	dir := setupTestDir(t)
	featDir := setupLedgerFeature(t, dir)
	if err := os.WriteFile(filepath.Join(featDir, "capabilities.yaml"), []byte(ops), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, featDir
}

const twoOps = `schema_version: 1
feature: my-feature
operations:
  - id: x
    kind: command
    summary: does x
    source: "@my-feature/check-readiness"
  - id: y
    kind: query
    summary: does y
    source: "@my-feature/check-readiness"
`

func advisoryFor(t *testing.T, featDir string, stored *HashedSources) map[string]string {
	t.Helper()
	return computeAdvisorySourceDiff(featDir, "my-feature", stored, BaselineSchemaVersion, "", "", nil)
}

func savedSources(t *testing.T, cfg interface{ FeaturePath(string) string }, featDir string) *HashedSources {
	t.Helper()
	bl, err := buildBaseline(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if bl.Sources == nil {
		t.Fatal("the baseline recorded no sources")
	}
	return bl.Sources
}

// The localisation the whole-file hash cannot give: WHICH operation changed,
// at the granularity an amendment's affects: already speaks in.
func TestEntryDiff_LocalisesTheChangedOperation(t *testing.T) {
	_, featDir := capsFixture(t, twoOps)
	stored := savedSources(t, testContext(t), featDir)
	if len(stored.CapabilityEntries) != 2 {
		t.Fatalf("the baseline must record both operations; got %v", stored.CapabilityEntries)
	}

	// Change exactly one operation, in a field the PARSER DOES NOT MODEL.
	// `summary` is the standing example — a fingerprint over the parsed struct
	// would miss this entirely, which is how it was once an approval bypass.
	changed := strings.Replace(twoOps, "summary: does x", "summary: does x, differently", 1)
	if err := os.WriteFile(filepath.Join(featDir, "capabilities.yaml"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}

	got := advisoryFor(t, featDir, stored)
	if got["capability-entry:@my-feature/operation:x"] != "changed" {
		t.Errorf("the edited operation must read changed; got %q",
			got["capability-entry:@my-feature/operation:x"])
	}
	if got["capability-entry:@my-feature/operation:y"] != "stable" {
		t.Errorf("the untouched operation must read stable — localising is the whole point; got %q",
			got["capability-entry:@my-feature/operation:y"])
	}
	// The whole-file verdict still fires, and is now the coarse half rather
	// than the only half.
	if got["capabilities"] != "changed" {
		t.Errorf("the file-level verdict must still be reported; got %q", got["capabilities"])
	}
}

func TestEntryDiff_ReportsAdditionsAndRemovals(t *testing.T) {
	_, featDir := capsFixture(t, twoOps)
	stored := savedSources(t, testContext(t), featDir)

	// y goes, z arrives.
	next := strings.Replace(twoOps, `  - id: y
    kind: query
    summary: does y
    source: "@my-feature/check-readiness"
`, `  - id: z
    kind: query
    summary: does z
    source: "@my-feature/check-readiness"
`, 1)
	if next == twoOps {
		t.Fatal("fixture: the replacement changed nothing")
	}
	if err := os.WriteFile(filepath.Join(featDir, "capabilities.yaml"), []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}

	got := advisoryFor(t, featDir, stored)
	if got["capability-entry:@my-feature/operation:z"] != "new" {
		t.Errorf("an added operation must read new; got %q",
			got["capability-entry:@my-feature/operation:z"])
	}
	if got["capability-entry:@my-feature/operation:y"] != "removed" {
		t.Errorf("a removed operation must be reported, not merely absent; got %q",
			got["capability-entry:@my-feature/operation:y"])
	}
}

// "This feature declares no operations" and "nobody measured them" are
// different facts, and a consumer that cannot tell them apart draws the wrong
// conclusion from silence. A baseline written before this existed says so.
func TestEntryDiff_LegacyBaselineSaysUnrecordedRatherThanEmpty(t *testing.T) {
	_, featDir := capsFixture(t, twoOps)
	stored := savedSources(t, testContext(t), featDir)
	// The pre-upgrade shape: whole-file hash present, no measurement taken.
	legacy := *stored
	legacy.CapabilityEntries = nil
	legacy.CapabilityEntriesRecorded = false

	got := advisoryFor(t, featDir, &legacy)
	if got["capability-entries"] != "unrecorded" {
		t.Errorf("a baseline with no recorded entries must say so; got %q",
			got["capability-entries"])
	}
	for k := range got {
		if strings.HasPrefix(k, "capability-entry:") {
			t.Errorf("no per-entry verdict may be reported against a baseline that measured "+
				"none — %q claims a comparison nobody made", k)
		}
	}
	// And the next save backfills it.
	after := savedSources(t, testContext(t), featDir)
	if len(after.CapabilityEntries) != 2 {
		t.Errorf("the next save must backfill the entries; got %v", after.CapabilityEntries)
	}
}

// An artifact that exists and will not parse is neither unrecorded nor changed.
// Both of those are measurements; this is the absence of one.
func TestEntryDiff_UnreadableArtifactIsItsOwnAnswer(t *testing.T) {
	_, featDir := capsFixture(t, twoOps)
	stored := savedSources(t, testContext(t), featDir)
	if err := os.WriteFile(filepath.Join(featDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := advisoryFor(t, featDir, stored)
	if got["capability-entries"] != "unreadable" {
		t.Errorf("an unparseable artifact must say so; got %q", got["capability-entries"])
	}
	for k, v := range got {
		if strings.HasPrefix(k, "capability-entry:") {
			t.Errorf("%q = %q claims a comparison against a file nobody could read", k, v)
		}
	}
}

// A feature with no capabilities.yaml has nothing to localise, and must not be
// told its entries are unrecorded — there are none to record.
func TestEntryDiff_AbsentArtifactReportsNothing(t *testing.T) {
	dir := setupTestDir(t)
	featDir := setupLedgerFeature(t, dir)
	_ = dir
	got := advisoryFor(t, featDir, &HashedSources{})
	if _, said := got["capability-entries"]; said {
		t.Errorf("a feature with no capabilities artifact must say nothing about its entries; "+
			"got %q", got["capability-entries"])
	}
}

// An empty measurement is a measurement. A map with omitempty serialises an
// empty one as nothing and reloads it as nil, so a feature declaring
// `operations: []` would read "unrecorded" forever — indistinguishable from a
// baseline written before any of this existed. That is the same known-empty
// versus unavailable defect the spec view had, and it has to survive the round
// trip through disk to be worth anything.
func TestEntryDiff_MeasuredEmptySurvivesTheStorageRoundTrip(t *testing.T) {
	_, featDir := capsFixture(t, "schema_version: 1\nfeature: my-feature\noperations: []\n")
	cfg := testContext(t)

	bl, err := buildBaseline(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	// Through disk, not in memory: in memory the empty map is non-nil and the
	// bug is invisible.
	data, err := marshalBaseline(bl)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded Baseline
	if err := yamlUnmarshalBaseline(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reloaded.Sources.CapabilityEntriesRecorded {
		t.Fatal("a measurement of an empty artifact must survive as a measurement")
	}

	got := advisoryFor(t, featDir, reloaded.Sources)
	if _, said := got["capability-entries"]; said {
		t.Errorf("a measured empty set must not be reported as unrecorded or unreadable; got %q",
			got["capability-entries"])
	}
	for k := range got {
		if strings.HasPrefix(k, "capability-entry:") {
			t.Errorf("there are no operations to report; got %q", k)
		}
	}
}

// A splice that deletes capabilities.yaml deletes every operation in it, and
// those refs are exactly what the walkthrough's ref-by-ref comparison needs.
// The coarse verdict says the file went; without this the refs go unreported.
func TestEntryDiff_ArtifactRemovalReportsEveryStoredRef(t *testing.T) {
	_, featDir := capsFixture(t, twoOps)
	stored := savedSources(t, testContext(t), featDir)
	if err := os.Remove(filepath.Join(featDir, "capabilities.yaml")); err != nil {
		t.Fatal(err)
	}

	got := advisoryFor(t, featDir, stored)
	for _, ref := range []string{
		"capability-entry:@my-feature/operation:x",
		"capability-entry:@my-feature/operation:y",
	} {
		if got[ref] != "removed" {
			t.Errorf("%s must be reported removed when the artifact holding it is deleted; got %q",
				ref, got[ref])
		}
	}
	if got["capabilities"] != "removed" {
		t.Errorf("the coarse verdict must still fire; got %q", got["capabilities"])
	}
}

// Never measured and now absent: no entry verdict may be invented, because
// there is no prior measurement to have lost anything from.
func TestEntryDiff_UnmeasuredAndAbsentInventsNothing(t *testing.T) {
	dir := setupTestDir(t)
	featDir := setupLedgerFeature(t, dir)
	_ = dir
	got := advisoryFor(t, featDir, &HashedSources{Capabilities: "deadbeefdeadbeef"})
	for k := range got {
		if strings.HasPrefix(k, "capability-entry") {
			t.Errorf("nothing was ever measured, so %q reports a comparison nobody made", k)
		}
	}
}

// The coarse verdict and the per-entry verdicts must describe the SAME bytes:
// the walkthrough compares them ref by ref, so assembling them from two moments
// makes the comparison meaningless in exactly the case it exists for.
//
// A green ordinary test proves the two agree today, not that they came from one
// read. The hook counts.
func TestEntryDiff_ReadsTheArtifactExactlyOnce(t *testing.T) {
	_, featDir := capsFixture(t, twoOps)
	stored := savedSources(t, testContext(t), featDir)

	capsPath := filepath.Join(featDir, "capabilities.yaml")
	reads := 0
	diffArtifactReadHook = func(p string) {
		if p == capsPath {
			reads++
		}
	}
	t.Cleanup(func() { diffArtifactReadHook = nil })

	got := advisoryFor(t, featDir, stored)
	if reads != 1 {
		t.Errorf("one diff read capabilities.yaml %d times; the file verdict and the entry "+
			"verdicts must come from one byte state", reads)
	}
	// And it still produced both halves from that one read.
	if got["capabilities"] != "stable" {
		t.Errorf("the file verdict is missing; got %q", got["capabilities"])
	}
	if got["capability-entry:@my-feature/operation:x"] != "stable" {
		t.Errorf("the entry verdicts are missing; got %q",
			got["capability-entry:@my-feature/operation:x"])
	}
}

// A read failure is not a deletion. Treating one as the other announces that
// every operation was removed while the file is still sitting there.
func TestEntryDiff_UnreadableIsNotRemoval(t *testing.T) {
	cases := []struct{ name, kind string }{
		{"a directory where the artifact should be", "dir"},
		{"a dangling symlink", "symlink"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, featDir := capsFixture(t, twoOps)
			stored := savedSources(t, testContext(t), featDir)
			capsPath := filepath.Join(featDir, "capabilities.yaml")
			if err := os.Remove(capsPath); err != nil {
				t.Fatal(err)
			}
			switch tc.kind {
			case "dir":
				if err := os.Mkdir(capsPath, 0o755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(filepath.Join(featDir, "nothing-here.yaml"), capsPath); err != nil {
					t.Fatal(err)
				}
			}

			got := advisoryFor(t, featDir, stored)
			if got["capability-entries"] != "unreadable" {
				t.Errorf("an artifact that exists and cannot be read must say so; got %q",
					got["capability-entries"])
			}
			// The COARSE verdict too. Reporting "removed" beside an entry-level
			// "unreadable" makes one output state two contradictory things
			// about the same file, and the coarse half is what a reader sees
			// first.
			if got["capabilities"] == "removed" {
				t.Error("the file-level verdict announced a deletion for an artifact that is " +
					"still on disk, contradicting the entry-level verdict beside it")
			}
			if got["capabilities"] != "unreadable" {
				t.Errorf("the file-level verdict must say what actually happened; got %q",
					got["capabilities"])
			}
			for k, v := range got {
				if strings.HasPrefix(k, "capability-entry:") && v == "removed" {
					t.Errorf("%s was announced removed while the artifact is still there", k)
				}
			}
		})
	}
}

// A correct narrowing removes an operation, and a removed operation cannot
// appear in dirty_set: dirty_set is the RESOLVABLE affects: of the unapplied
// tail, and the schema deliberately does not require removed or replaced-by
// entries in affects: at all — their fate is declared in scope_impact.
//
// So the diff reporting a removed ref that dirty_set does not contain is the
// CORRECT outcome, not a defect. A workflow rule demanding they match would
// reject every legitimate removal, which is why the guidance says the removed
// half is reconciled by hand.
func TestEntryDiff_RemovedRefIsAbsentFromDirtySetByDesign(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featDir := setupRetirement(t, dir, retireDispositions)
	_ = dir

	// The retirement fixture's splice removes y outright.
	stored := &HashedSources{
		Capabilities:              "irrelevant",
		CapabilityEntriesRecorded: true,
		CapabilityEntries: map[string]string{
			"@my-feature/operation:x": strings.Repeat("ab", 32),
			"@my-feature/operation:y": strings.Repeat("cd", 32),
		},
	}
	got := computeAdvisorySourceDiff(featDir, "my-feature", stored, BaselineSchemaVersion, "", "", nil)
	if got["capability-entry:@my-feature/operation:y"] != "removed" {
		t.Fatalf("the removed operation must be localised; got %q",
			got["capability-entry:@my-feature/operation:y"])
	}

	ca := computeCheckAmendments(cfg, "my-feature")
	for _, ref := range ca.DirtySet {
		if ref == "@my-feature/operation:y" {
			t.Fatal("the fixture no longer demonstrates the asymmetry — a removed ref appeared " +
				"in dirty_set, which it cannot do once it stops resolving")
		}
	}
	// And the record DOES account for it, in the place the schema puts it.
	records, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		t.Fatal(err)
	}
	var accounted bool
	for _, a := range records {
		if a.ScopeImpact == nil {
			continue
		}
		for _, ex := range a.ScopeImpact.Exceptions {
			if ex.Ref == "@my-feature/operation:y" {
				accounted = true
			}
		}
	}
	if !accounted {
		t.Error("the removal is unaccounted even in scope_impact — the fixture is not a " +
			"correct narrowing and proves nothing about the workflow rule")
	}
}
