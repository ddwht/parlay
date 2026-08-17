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
