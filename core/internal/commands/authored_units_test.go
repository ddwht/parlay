package commands

// parlay-feature: parlay-tool/hand-authored-units
// parlay-component: authored-unit-ingestion
// parlay-artifact: test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// declareUnit writes a unit at spec/intents/<id>/ owning the given globs.
func declareUnit(t *testing.T, dir, id string, sources ...string) {
	t.Helper()
	unitDir := filepath.Join(dir, "spec", "intents", id)
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "intents.md"), []byte("# "+id+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	body := "schema_version: 1\nunit: " + id + "\nsummary: hand-written engine\nsources:\n"
	for _, s := range sources {
		body += "  - \"" + s + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(unitDir, config.AuthoredFile), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// writePlainFile writes a file carrying NO parlay marker — the state every
// hand-authored source is in, and the reason ScanGenerated can never see one.
func writePlainFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// The central claim of Part A: code parlay did not write becomes visible.
// Before this, an unmarked file was not merely untracked — no ingestion path
// existed that could ever return it.
func TestAuthoredFilesAreTrackedThoughNoMarkerScanReturnsThem(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)

	engine := filepath.Join(dir, "App", "Sources", "Core", "mesh.swift")
	writePlainFile(t, engine, "struct Mesh {}\n")
	declareUnit(t, dir, "geometry-engine", "App/Sources/Core/**")

	authored, projection, err := resolveAuthoredUnits(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Units) != 1 || projection.Units[0].Unit != "geometry-engine" {
		t.Fatalf("projection = %+v, want one geometry-engine entry", projection.Units)
	}

	// The marker scan must return nothing here — if it ever does, this test
	// stops proving what it claims to prove.
	sourceRoot := filepath.Join(dir, "App")
	markerOnly, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(markerOnly.Files) != 0 {
		t.Fatalf("premise broken: the marker scan returned %d file(s) for unmarked sources", len(markerOnly.Files))
	}

	hashes, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, nil, nil, authored)
	if err != nil {
		t.Fatal(err)
	}
	rel := "App/Sources/Core/mesh.swift"
	entry, tracked := hashes.Files[rel]
	if !tracked {
		t.Fatalf("hand-authored file is not tracked; files = %v", keysOf(hashes.Files))
	}
	if entry.Provenance != ProvenanceHandAuthored {
		t.Errorf("provenance = %q, want %q", entry.Provenance, ProvenanceHandAuthored)
	}
	if entry.Component != "geometry-engine" {
		t.Errorf("component = %q, want the owning unit", entry.Component)
	}
	if entry.EmittedAt != "" {
		t.Errorf("emitted-at = %q — nothing emitted this file", entry.EmittedAt)
	}
	if hashes.SchemaVersion != 2 {
		t.Errorf("schema version = %d, want 2 — a snapshot that can contain hand-authored entries is not a v1 snapshot", hashes.SchemaVersion)
	}
}

// Both statements are authoritative and they contradict each other, so the
// tool must refuse rather than pick a winner.
func TestCodegenWritingIntoAUnitIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)

	engine := filepath.Join(dir, "App", "Sources", "Core", "mesh.swift")
	writePlainFile(t, engine, "struct Mesh {}\n")
	declareUnit(t, dir, "geometry-engine", "App/Sources/Core/**")

	authored, _, err := resolveAuthoredUnits(cfg)
	if err != nil {
		t.Fatal(err)
	}

	emitted := declare(t, cfg, "App/Sources/Core/mesh.swift")
	_, _, err = buildCodeHashesWithProvenance(cfg, "", filepath.Join(dir, "App"), emitted, nil, authored)
	if err == nil {
		t.Fatal("expected a refusal when codegen declares writing a unit's file")
	}
	if !strings.Contains(err.Error(), "authored-glob-overlaps-generated") {
		t.Errorf("error = %v, want authored-glob-overlaps-generated", err)
	}
}

// A glob that matches nothing reads as "this unit owns these files" while
// tracking none of them — declared and undeclared at once.
func TestEmptyGlobIsRefusedRatherThanSilentlyTrackingNothing(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	declareUnit(t, dir, "geometry-engine", "App/Sources/DoesNotExist/**")

	_, _, err := resolveAuthoredUnits(cfg)
	if err == nil {
		t.Fatal("expected a refusal for a glob matching no file")
	}
	if !strings.Contains(err.Error(), "authored-glob-empty") {
		t.Errorf("error = %v, want authored-glob-empty", err)
	}
}

func TestTwoUnitsCannotClaimTheSameFile(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	writePlainFile(t, filepath.Join(dir, "App", "Sources", "Core", "mesh.swift"), "struct Mesh {}\n")
	declareUnit(t, dir, "engine-a", "App/Sources/Core/**")
	declareUnit(t, dir, "engine-b", "App/Sources/**")

	_, _, err := resolveAuthoredUnits(cfg)
	if err == nil || !strings.Contains(err.Error(), "authored-glob-overlaps-generated") {
		t.Errorf("error = %v, want a two-owner refusal", err)
	}
}

func TestMatchAuthoredGlob(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"App/Sources/Core/**", "App/Sources/Core/mesh.swift", true},
		{"App/Sources/Core/**", "App/Sources/Core/deep/nested/mesh.swift", true},
		// `**` matches zero segments in trailing position too, so the
		// directory itself is inside the unit. Ingestion never asks this
		// (it tests files only); a containment caller would want yes.
		{"App/Sources/Core/**", "App/Sources/Core", true},
		{"App/Sources/Core/**", "App/Sources/Other/mesh.swift", false},
		{"App/**/*.swift", "App/Sources/Core/mesh.swift", true},
		{"App/**/*.swift", "App/Sources/Core/mesh.metal", false},
		// `**` matches zero segments too.
		{"App/**/mesh.swift", "App/mesh.swift", true},
		{"App/*.swift", "App/Sources/mesh.swift", false},
	}
	for _, tc := range cases {
		if got := matchAuthoredGlob(tc.glob, tc.path); got != tc.want {
			t.Errorf("match(%q, %q) = %v, want %v", tc.glob, tc.path, got, tc.want)
		}
	}
}

func keysOf(m map[string]CodeHashEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The advisory half of the same signal. check-drift and diff block nothing
// by explicit design, but they must still SAY the engine moved — that is
// what marks dependent features for rebuild.
func TestUnitChangeIsReportedAsSharedDrift(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)

	engine := filepath.Join(dir, "App", "Sources", "Core", "mesh.swift")
	writePlainFile(t, engine, "struct Mesh { let scale = 1.0 }\n")
	declareUnit(t, dir, "geometry-engine", "App/Sources/Core/**")

	before := authoredUnitHashes(cfg)
	if before["geometry-engine"] == "" {
		t.Fatal("no hash recorded for the declared unit")
	}

	// One line of the engine changes — the block-printing case exactly.
	writePlainFile(t, engine, "struct Mesh { let scale = 2.0 }\n")
	after := authoredUnitHashes(cfg)
	if after["geometry-engine"] == before["geometry-engine"] {
		t.Error("editing a unit's source must move its aggregate hash")
	}
}

// Renaming a file without editing it changes the unit. Hashing content
// alone would call that stable, and a consumer referring to the old path
// would break against a signature that never moved.
func TestUnitRenameMovesTheHash(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)

	body := "struct Mesh {}\n"
	writePlainFile(t, filepath.Join(dir, "App", "Sources", "Core", "mesh.swift"), body)
	declareUnit(t, dir, "geometry-engine", "App/Sources/Core/**")
	before := authoredUnitHashes(cfg)

	if err := os.Remove(filepath.Join(dir, "App", "Sources", "Core", "mesh.swift")); err != nil {
		t.Fatal(err)
	}
	writePlainFile(t, filepath.Join(dir, "App", "Sources", "Core", "geometry.swift"), body)

	if after := authoredUnitHashes(cfg); after["geometry-engine"] == before["geometry-engine"] {
		t.Error("a rename with identical bytes must still move the unit hash")
	}
}

// The commands whose whole operation is a pipeline step a unit has no
// place in must refuse. create-dialogs is the one that matters most: it
// WRITES, and without a guard it authors a dialogs.md inside the unit's
// own directory, leaving the unit looking like a half-built feature.
func TestPipelineCommandsRefuseAUnit(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	writePlainFile(t, filepath.Join(dir, "App", "Sources", "Core", "mesh.swift"), "struct Mesh {}\n")
	declareUnit(t, dir, "geometry-engine", "App/Sources/Core/**")

	err := refuseOnUnit(cfg, "geometry-engine", "because reasons")
	if err == nil {
		t.Fatal("expected a refusal for a unit")
	}
	if !strings.Contains(err.Error(), CodeUnitNotAFeature) {
		t.Errorf("error = %v, want the %s code", err, CodeUnitNotAFeature)
	}

	// An ordinary feature is untouched by the guard.
	featureDir := filepath.Join(dir, "spec", "intents", "checkout")
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte("# c\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := refuseOnUnit(cfg, "checkout", "because reasons"); err != nil {
		t.Errorf("a feature must pass the guard, got %v", err)
	}
}
