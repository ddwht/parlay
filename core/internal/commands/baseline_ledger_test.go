// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: baseline-freeze-semantics
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

const ledgerTestIntents = `## Check Readiness

**Goal**: See if the cluster is ready.
**Persona**: Admin
**Objects**: cluster, upgrade

**Constraints**:
- Must show status

**Verify**:
- Readiness status is displayed
`

// setupLedgerFeature lays down a feature with a saved baseline and returns
// the feature dir. The config carries a leftover `ledger: true` key on
// purpose — old projects still have one, and it must be inert.
func setupLedgerFeature(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".parlay"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".parlay", "config.yaml"), []byte("ledger: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(testContext(t).BuildPath("my-feature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(ledgerTestIntents), 0o644); err != nil {
		t.Fatal(err)
	}
	return featureDir
}

func saveLedgerBaseline(t *testing.T, featureDir string, mutate func(*Baseline)) {
	t.Helper()
	parsed, err := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		t.Fatal(err)
	}
	baseline := Baseline{
		SchemaVersion: BaselineSchemaVersion,
		GeneratedAt:   "2026-08-13T00:00:00Z",
		Intents:       make(map[string]IntentHash),
		Sources:       &HashedSources{},
	}
	for _, intent := range parsed {
		baseline.Intents[intent.Slug] = hashIntent(intent)
	}
	if mutate != nil {
		mutate(&baseline)
	}
	data, err := yaml.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath(testContext(t), "my-feature"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectDrift_LedgerFreezesIntents(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, nil)

	// Edit the frozen founding doc.
	edited := strings.Replace(ledgerTestIntents, "See if the cluster is ready.", "See if EVERYTHING is ready.", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Drifted) != 0 {
		t.Errorf("under the ledger flag an intent edit must not be rebuild-drift; got %+v", output.Drifted)
	}
	if len(output.LedgerIntegrity) != 1 || !strings.Contains(output.LedgerIntegrity[0], "changed after freeze") {
		t.Errorf("expected one changed-after-freeze integrity finding; got %v", output.LedgerIntegrity)
	}
	if !output.HasDrift {
		t.Error("an integrity violation must still set has_drift so gates see it")
	}
}

// TestLedgerFlagIsRemoved pins the v0.4 removal: `ledger: false` in an old
// config is inert — freeze semantics apply regardless (same idiom as
// TestNoStudioFlagIsRemoved).
func TestLedgerFlagIsRemoved(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".parlay", "config.yaml"), []byte("ledger: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveLedgerBaseline(t, featureDir, nil)

	edited := strings.Replace(ledgerTestIntents, "See if the cluster is ready.", "See if EVERYTHING is ready.", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Drifted) != 0 || len(output.LedgerIntegrity) != 1 {
		t.Errorf("ledger: false must be inert — an intent edit is an integrity finding; got drifted=%+v integrity=%v", output.Drifted, output.LedgerIntegrity)
	}
}

func TestDetectDrift_LedgerUnappliedAmendmentTail(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	// Baseline recorded with amendment 001 applied.
	writeAmendment(t, featureDir, "001-first.md", "---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\na\n## Acceptance\n- b\n")
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})

	// A new amendment lands after the save.
	writeAmendment(t, featureDir, "002-second.md", "---\namendment: second\ndate: 2026-08-14\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\nc\n## Acceptance\n- d\n")

	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.UnappliedAmendments) != 1 || output.UnappliedAmendments[0] != "002-second.md" {
		t.Errorf("expected 002-second.md as the unapplied tail; got %v", output.UnappliedAmendments)
	}
	if len(output.LedgerIntegrity) != 0 {
		t.Errorf("a new amendment is not an integrity violation; got %v", output.LedgerIntegrity)
	}
	if !output.HasDrift {
		t.Error("an unapplied amendment must set has_drift")
	}
}

func TestDetectDrift_LedgerAmendmentMutatedOrRemoved(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", "---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\na\n## Acceptance\n- b\n")
	writeAmendment(t, featureDir, "002-second.md", "---\namendment: second\ndate: 2026-08-14\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\nc\n## Acceptance\n- d\n")
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 2
		b.Sources.Amendments = map[string]string{}
		for _, name := range []string{"001-first.md", "002-second.md"} {
			if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", name)); ok {
				b.Sources.Amendments[name] = hash
			}
		}
	})

	// Mutate one recorded amendment, delete the other.
	writeAmendment(t, featureDir, "001-first.md", "---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\nREWRITTEN\n## Acceptance\n- b\n")
	if err := os.Remove(filepath.Join(featureDir, "amendments", "002-second.md")); err != nil {
		t.Fatal(err)
	}

	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	var mutated, removed bool
	for _, f := range output.LedgerIntegrity {
		if strings.Contains(f, "001-first.md mutated") {
			mutated = true
		}
		if strings.Contains(f, "002-second.md removed") {
			removed = true
		}
	}
	if !mutated || !removed {
		t.Errorf("expected mutated + removed findings; got %v", output.LedgerIntegrity)
	}
}

func TestBuildBaseline_RecordsLedgerState(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", "---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\na\n## Acceptance\n- b\n")
	writeAmendment(t, featureDir, "002-second.md", "---\namendment: second\ndate: 2026-08-14\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\nc\n## Acceptance\n- d\n")

	baseline, err := buildBaseline(testContext(t), "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.LastAppliedAmendment != 2 {
		t.Errorf("expected last-applied-amendment 2, got %d", baseline.LastAppliedAmendment)
	}
	if len(baseline.Sources.Amendments) != 2 {
		t.Errorf("expected 2 recorded amendment hashes, got %v", baseline.Sources.Amendments)
	}
	if baseline.SchemaVersion != 3 {
		t.Errorf("expected baseline schema-version 3, got %d", baseline.SchemaVersion)
	}
}

// A baseline that counted a commented-out intent must FAIL VISIBLY.
//
// ParseIntentsFile used to read `## ` headings inside `<!-- ... -->` as real
// intents. Any feature built under that behaviour has a baseline entry for a
// commented block, and under the corrected parser that slug is simply gone —
// so the promise set silently shrinks unless drift catches it.
//
// It does: the slug is reported as removed-after-freeze. That is the right
// outcome. A parser fix at the base of the pipeline changes what a founding
// document promises, and the one thing it must not do is change it quietly.
//
// Deliberately NOT auto-repaired. The remedy is a decision only the author can
// make: if the block was meant to stay a founding promise, uncommenting the
// identical text restores the same slug and hash under the new parser and
// nothing semantic changed; if it was meant to be inactive, retiring it is a
// lifecycle act that goes through supersession, not something that should
// happen as a side effect of upgrading.
func TestDetectDrift_BaselineCountingACommentedIntentFailsVisibly(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		// The entry a legacy build would have written for a commented block.
		b.Intents["retired-probe"] = IntentHash{ContentHash: "sha256:legacy", Goal: "sha256:legacy"}
	})

	// The founding doc carries that block, commented out — visible to the old
	// parser, correctly invisible to the new one.
	commented := ledgerTestIntents + "\n<!--\n## Retired Probe\n\n**Goal**: An old idea.\n**Persona**: Admin\n-->\n"
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(commented), 0o644); err != nil {
		t.Fatal(err)
	}

	// Control: the commented block must not parse, or the assertion below
	// would be about something else entirely.
	parsed, err := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range parsed {
		if p.Slug == "retired-probe" {
			t.Fatal("the commented block still parses; this test is not exercising the new semantics")
		}
	}

	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range output.LedgerIntegrity {
		if strings.Contains(v, "retired-probe") && strings.Contains(v, "removed after freeze") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a baselined intent that no longer parses must be reported, not silently dropped; ledger_integrity=%v", output.LedgerIntegrity)
	}
}

// Compaction moves applied amendments to amendments/archive/ (the schema's
// documented post-compaction shape). Integrity must keep hash-checking them
// there: a compacted file is history retained, an edited one is still a
// write-once violation, and only a file in NEITHER place was erased. The
// active tail must see only active amendments throughout.
func TestDetectDrift_CompactedAmendmentsRemainHashChecked(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeAmendment(t, featureDir, "001-first.md", "---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\na\n## Acceptance\n- b\n")
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{}
		if hash, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "001-first.md")); ok {
			b.Sources.Amendments["001-first.md"] = hash
		}
	})

	// Compact: move 001 into archive/.
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(featureDir, "amendments", "001-first.md"),
		filepath.Join(archive, "001-first.md")); err != nil {
		t.Fatal(err)
	}

	out, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.LedgerIntegrity) != 0 {
		t.Errorf("a compacted amendment byte-identical in archive/ is retained history, not an integrity finding; got %v", out.LedgerIntegrity)
	}
	if len(out.UnappliedAmendments) != 0 {
		t.Errorf("archived amendments must not join the active tail; got %v", out.UnappliedAmendments)
	}

	// An EDIT to the archived file is still caught.
	if err := os.WriteFile(filepath.Join(archive, "001-first.md"),
		[]byte("---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\nrewritten history\n## Acceptance\n- b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.LedgerIntegrity) != 1 || !strings.Contains(out.LedgerIntegrity[0], "mutated") {
		t.Errorf("an edited archived amendment is a write-once violation; got %v", out.LedgerIntegrity)
	}

	// Gone from BOTH places = erased.
	if err := os.Remove(filepath.Join(archive, "001-first.md")); err != nil {
		t.Fatal(err)
	}
	out, err = detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.LedgerIntegrity) != 1 || !strings.Contains(out.LedgerIntegrity[0], "removed from the ledger") {
		t.Errorf("an amendment in neither the ledger nor archive/ was erased; got %v", out.LedgerIntegrity)
	}
}
