// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: ledger-migration
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestScanLedgerMigration_Verdicts(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, nil)

	// Clean feature.
	verdicts, err := scanLedgerMigration(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 || verdicts[0].State != ledgerClean {
		t.Fatalf("expected one clean verdict, got %+v", verdicts)
	}

	// Drift the founding doc → needs-freeze with per-file detail.
	edited := strings.Replace(ledgerTestIntents, "See if the cluster is ready.", "See if EVERYTHING is ready.", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	verdicts, err = scanLedgerMigration(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 || verdicts[0].State != ledgerNeedsFreeze {
		t.Fatalf("expected needs-freeze, got %+v", verdicts)
	}
	if len(verdicts[0].Detail) != 1 || !strings.Contains(verdicts[0].Detail[0], "intents.md: \"check-readiness\" changed") {
		t.Errorf("expected per-file drift detail, got %v", verdicts[0].Detail)
	}

	// Amendments alongside the drift → refuse.
	writeAmendment(t, featureDir, "001-first.md", "---\namendment: first\ndate: 2026-08-13\naffects: [\"@my-feature/operation:x\"]\n---\n## Change\na\n## Acceptance\n- b\n")
	verdicts, _ = scanLedgerMigration(dir)
	if verdicts[0].State != ledgerRefuseAmendments {
		t.Fatalf("drifted docs + existing amendments must refuse, got %+v", verdicts[0])
	}
	if err := os.RemoveAll(filepath.Join(featureDir, "amendments")); err != nil {
		t.Fatal(err)
	}

	// Leftover surface.md → refuse regardless of drift.
	if err := os.WriteFile(filepath.Join(featureDir, "surface.md"), []byte("## Fragment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verdicts, _ = scanLedgerMigration(dir)
	if verdicts[0].State != ledgerRefuseSurfaceMD {
		t.Fatalf("surface.md presence must refuse, got %+v", verdicts[0])
	}
	if err := os.Remove(filepath.Join(featureDir, "surface.md")); err != nil {
		t.Fatal(err)
	}

	// No baseline → nothing to do; it freezes at first green build.
	if err := os.Remove(baselinePath(testContext(t), "my-feature")); err != nil {
		t.Fatal(err)
	}
	verdicts, _ = scanLedgerMigration(dir)
	if verdicts[0].State != ledgerNoBaseline {
		t.Fatalf("expected no-baseline, got %+v", verdicts[0])
	}
}

// TestRestampFoundingHashes_SpecSideOnly is the regression pin against the
// WP6 false-stable failure mode: the migrator must rewrite ONLY the
// spec-side founding hashes and leave every build-side hash untouched, so a
// drifted-but-unrebuilt feature keeps reporting spec→build staleness.
func TestRestampFoundingHashes_SpecSideOnly(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)

	buildSide := func(b *Baseline) {
		b.GeneratedAt = "2026-08-13T00:00:00Z"
		b.LastAppliedAmendment = 0
		b.BuildfileSections = map[string]string{"components": "aaaa"}
		b.Sources.SurfaceFragments = map[string]string{"main-view": "bbbb"}
		b.Sources.Capabilities = "cccc"
		b.Sources.Infrastructure = "dddd"
		b.Sources.SurfaceYAML = "eeee"
		b.Sources.Domain = "ffff"
		b.Sources.AdapterVersion = "gggg"
		b.Sources.Authored = map[string]string{"engine": "hhhh"}
	}
	saveLedgerBaseline(t, featureDir, buildSide)

	edited := strings.Replace(ledgerTestIntents, "See if the cluster is ready.", "See if EVERYTHING is ready.", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := restampFoundingHashes(dir, "my-feature"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(baselinePath(testContext(t), "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	var after Baseline
	if err := yaml.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}

	// Spec-side hashes now match the edited text: check-drift is clean.
	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.LedgerIntegrity) != 0 {
		t.Errorf("after re-stamp the edited text IS the founding state; got %v", output.LedgerIntegrity)
	}

	// Build-side hashes and stamps pass through byte-identical.
	if after.GeneratedAt != "2026-08-13T00:00:00Z" {
		t.Errorf("GeneratedAt must not be re-stamped; got %q", after.GeneratedAt)
	}
	if after.BuildfileSections["components"] != "aaaa" {
		t.Errorf("BuildfileSections touched: %v", after.BuildfileSections)
	}
	s := after.Sources
	for name, got := range map[string]string{
		"SurfaceFragments[main-view]": s.SurfaceFragments["main-view"],
		"Capabilities":                s.Capabilities,
		"Infrastructure":              s.Infrastructure,
		"SurfaceYAML":                 s.SurfaceYAML,
		"Domain":                      s.Domain,
		"AdapterVersion":              s.AdapterVersion,
		"Authored[engine]":            s.Authored["engine"],
	} {
		if got == "" {
			t.Errorf("build-side hash %s was dropped by the re-stamp", name)
		}
	}
	if s.Capabilities != "cccc" || s.Domain != "ffff" || s.AdapterVersion != "gggg" {
		t.Errorf("build-side hashes mutated: capabilities=%q domain=%q adapter=%q", s.Capabilities, s.Domain, s.AdapterVersion)
	}
}

func TestMigrateLedger_Idempotent(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	saveLedgerBaseline(t, featureDir, nil)

	edited := strings.Replace(ledgerTestIntents, "See if the cluster is ready.", "See if EVERYTHING is ready.", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := restampFoundingHashes(dir, "my-feature"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(baselinePath(testContext(t), "my-feature"))
	if err != nil {
		t.Fatal(err)
	}

	// A second scan finds everything clean — nothing left to write.
	verdicts, err := scanLedgerMigration(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 || verdicts[0].State != ledgerClean {
		t.Fatalf("second run must find everything clean, got %+v", verdicts)
	}
	after, err := os.ReadFile(baselinePath(testContext(t), "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("baseline changed without a re-stamp")
	}
}

func TestMigrateLedger_DialogsFreezeMirrorsCheckDrift(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	dialogs := "### Greet\n\n**Trigger**: user opens app\n\nUser: hi\nSystem: hello\n"
	if err := os.WriteFile(filepath.Join(featureDir, "dialogs.md"), []byte(dialogs), 0o644); err != nil {
		t.Fatal(err)
	}
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		// Freeze the dialog as buildBaseline would.
		full, err := buildBaseline(testContext(t), "my-feature")
		if err != nil {
			t.Fatal(err)
		}
		b.Sources.Dialogs = full.Sources.Dialogs
	})

	// Edit the frozen dialog.
	if err := os.WriteFile(filepath.Join(featureDir, "dialogs.md"), []byte(strings.Replace(dialogs, "hello", "howdy", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	verdicts, err := scanLedgerMigration(dir)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].State != ledgerNeedsFreeze {
		t.Fatalf("edited frozen dialog must need freezing, got %+v", verdicts[0])
	}

	if err := restampFoundingHashes(dir, "my-feature"); err != nil {
		t.Fatal(err)
	}
	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.LedgerIntegrity) != 0 {
		t.Errorf("re-stamped dialogs must be clean under check-drift; got %v", output.LedgerIntegrity)
	}
}
