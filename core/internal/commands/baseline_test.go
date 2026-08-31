package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

func TestSaveAndDetectDrift_NoDrift(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)
	os.MkdirAll(testContext(t).BuildPath("my-feature"), 0755)

	intents := `## Check Readiness

**Goal**: See if the cluster is ready.
**Persona**: Admin
**Objects**: cluster, upgrade

**Constraints**:
- Must show status

**Verify**:
- Readiness status is displayed
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	// Save baseline
	parsed, _ := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	baseline := Baseline{
		GeneratedAt: "2026-04-06T00:00:00Z",
		Intents:     make(map[string]IntentHash),
	}
	for _, intent := range parsed {
		baseline.Intents[intent.Slug] = hashIntent(intent)
	}
	data, _ := yaml.Marshal(baseline)
	os.WriteFile(baselinePath(testContext(t), "my-feature"), data, 0644)

	// Check drift — should be none
	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if output.HasDrift {
		t.Error("expected no drift, got drift")
	}
}

func TestDetectDrift_GoalChanged(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)
	os.MkdirAll(testContext(t).BuildPath("my-feature"), 0755)

	// Original intent
	original := `## Check Readiness

**Goal**: See if the cluster is ready.
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(original), 0644)

	// Save baseline from original
	parsed, _ := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	baseline := Baseline{
		GeneratedAt: "2026-04-06T00:00:00Z",
		Intents:     make(map[string]IntentHash),
	}
	for _, intent := range parsed {
		baseline.Intents[intent.Slug] = hashIntent(intent)
	}
	data, _ := yaml.Marshal(baseline)
	os.WriteFile(baselinePath(testContext(t), "my-feature"), data, 0644)

	// Modify the goal
	modified := `## Check Readiness

**Goal**: Verify all prerequisites are met before upgrading.
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(modified), 0644)

	// Check drift: founding docs are frozen at first build, so an edited
	// goal is an integrity finding, not rebuild-drift.
	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if !output.HasDrift {
		t.Fatal("expected drift")
	}
	if len(output.Drifted) != 0 {
		t.Fatalf("an intent edit must not be rebuild-drift; got %+v", output.Drifted)
	}
	if len(output.LedgerIntegrity) != 1 || !strings.Contains(output.LedgerIntegrity[0], "changed after freeze") {
		t.Errorf("expected one changed-after-freeze integrity finding; got %v", output.LedgerIntegrity)
	}
}

func TestDetectDrift_NewAndRemovedIntents(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)
	os.MkdirAll(testContext(t).BuildPath("my-feature"), 0755)

	// Baseline had two intents
	baseline := Baseline{
		GeneratedAt: "2026-04-06T00:00:00Z",
		Intents: map[string]IntentHash{
			"intent-a": hashIntent(parser.Intent{Title: "Intent A", Slug: "intent-a", Goal: "Do A", Persona: "Admin"}),
			"intent-b": hashIntent(parser.Intent{Title: "Intent B", Slug: "intent-b", Goal: "Do B", Persona: "Admin"}),
		},
	}
	data, _ := yaml.Marshal(baseline)
	os.WriteFile(baselinePath(testContext(t), "my-feature"), data, 0644)

	// Current intents: A (unchanged) + C (new), B removed
	current := `## Intent A

**Goal**: Do A
**Persona**: Admin

---

## Intent C

**Goal**: Do C
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(current), 0644)

	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if !output.HasDrift {
		t.Fatal("expected drift")
	}
	// Added and removed intents both classify as integrity findings —
	// the founding doc is frozen, so its intent set cannot change.
	var added, removed bool
	for _, f := range output.LedgerIntegrity {
		if strings.Contains(f, "\"intent-c\" added after freeze") {
			added = true
		}
		if strings.Contains(f, "\"intent-b\" removed after freeze") {
			removed = true
		}
	}
	if !added || !removed {
		t.Errorf("expected added + removed integrity findings; got %v", output.LedgerIntegrity)
	}
	if len(output.NewIntents) != 0 || len(output.Removed) != 0 {
		t.Errorf("founding-doc changes must not report as rebuild-drift; got new=%v removed=%v", output.NewIntents, output.Removed)
	}
	// Intent A should not be flagged at all
	if len(output.Drifted) != 0 {
		t.Errorf("Drifted = %d, want 0 (Intent A unchanged)", len(output.Drifted))
	}
}

func TestDetectDrift_NoBaseline(t *testing.T) {
	dir := setupTestDir(t)

	featureDir := filepath.Join(dir, "spec", "intents", "my-feature")
	os.MkdirAll(featureDir, 0755)

	intents := `## Some Intent

**Goal**: Do something
**Persona**: Admin
`
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)

	// No baseline file — should return no drift
	output, err := detectDrift(testContext(t), "my-feature", featureDir)
	if err != nil {
		t.Fatal(err)
	}
	if output.HasDrift {
		t.Error("expected no drift when no baseline exists")
	}
}

// TestBaseline_TracksDomainModel is the regression guard for the defect
// where a domain-model.yaml edit was structurally invisible to drift
// detection: HashedSources had no Domain field, so a designer could change
// the shared model — the entire purpose of the Studio editor — and every
// dependent feature still reported has_drift:false, marking nothing for
// rebuild.
func TestBaseline_TracksDomainModel(t *testing.T) {
	setupTestDir(t)
	cfg := testContext(t)
	slug := "drift-domain"
	writeFeatureFiles(t, slug, "# Feature\n\n## An intent\n\n**Goal**: g\n**Persona**: p\n", "", "")

	domainPath := cfg.DomainModelPath()
	if err := os.MkdirAll(filepath.Dir(domainPath), 0o755); err != nil {
		t.Fatalf("mkdir domain dir: %v", err)
	}
	if err := os.WriteFile(domainPath, []byte("schema_version: 1\nentities:\n  - name: Widget\n"), 0o644); err != nil {
		t.Fatalf("write domain model: %v", err)
	}

	baseline, err := buildBaseline(cfg, slug)
	if err != nil {
		t.Fatalf("buildBaseline: %v", err)
	}
	if baseline.SchemaVersion != BaselineSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", baseline.SchemaVersion, BaselineSchemaVersion)
	}
	if baseline.Sources.Domain == "" {
		t.Fatal("Sources.Domain is empty — domain-model.yaml was not hashed, so drift detection is blind to it")
	}
	recorded := baseline.Sources.Domain

	// Edit the shared domain model, exactly as the Studio editor would.
	if err := os.WriteFile(domainPath, []byte("schema_version: 1\nentities:\n  - name: Widget\n  - name: Gadget\n"), 0o644); err != nil {
		t.Fatalf("rewrite domain model: %v", err)
	}

	got := computeAdvisorySourceDiff(cfg.FeaturePath(slug), baseline.Sources, baseline.SchemaVersion, domainPath, "", nil)
	if got["domain"] != "changed" {
		t.Errorf("advisory domain = %q, want \"changed\" (recorded %s)", got["domain"], recorded)
	}
}

// TestBaseline_PreV1BaselineDoesNotReportFalseDrift covers the upgrade
// hazard: baselines written before the Domain field existed carry no hash,
// and naively comparing "" against a real hash would report every
// pre-existing project as drifted the moment the binary is upgraded.
func TestBaseline_PreV1BaselineDoesNotReportFalseDrift(t *testing.T) {
	setupTestDir(t)
	cfg := testContext(t)
	slug := "drift-legacy"
	writeFeatureFiles(t, slug, "# Feature\n\n## An intent\n\n**Goal**: g\n**Persona**: p\n", "", "")

	domainPath := cfg.DomainModelPath()
	if err := os.MkdirAll(filepath.Dir(domainPath), 0o755); err != nil {
		t.Fatalf("mkdir domain dir: %v", err)
	}
	if err := os.WriteFile(domainPath, []byte("schema_version: 1\nentities: []\n"), 0o644); err != nil {
		t.Fatalf("write domain model: %v", err)
	}

	// A pre-v1 baseline: no SchemaVersion, no Domain hash.
	legacy := &HashedSources{Intents: map[string]string{}}

	got := computeAdvisorySourceDiff(cfg.FeaturePath(slug), legacy, 0, domainPath, "", nil)
	if got["domain"] == "changed" || got["domain"] == "new" {
		t.Errorf("advisory domain = %q on a pre-v1 baseline; want \"unknown\" so upgrading the binary does not mass-report false drift", got["domain"])
	}
	if got["domain"] != "unknown" {
		t.Errorf("advisory domain = %q, want \"unknown\"", got["domain"])
	}
}

// TestCheckDrift_DomainModelEditIsDrift is the end-to-end guard for the
// command agents actually gate on. detectDrift was intents-only, so a
// Studio save to the shared domain model left has_drift:false and no
// feature was ever marked for rebuild.
func TestCheckDrift_DomainModelEditIsDrift(t *testing.T) {
	setupTestDir(t)
	cfg := testContext(t)
	slug := "drift-e2e"
	featurePath := writeFeatureFiles(t, slug, "# F\n\n## An intent\n\n**Goal**: g\n**Persona**: p\n", "", "")

	domainPath := cfg.DomainModelPath()
	if err := os.MkdirAll(filepath.Dir(domainPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(domainPath, []byte("schema_version: 1\nentities:\n  - name: Widget\n"), 0o644); err != nil {
		t.Fatalf("write domain: %v", err)
	}

	baseline, err := buildBaseline(cfg, slug)
	if err != nil {
		t.Fatalf("buildBaseline: %v", err)
	}
	writeBaseline(t, slug, *baseline)

	// Baseline is fresh — nothing should be drifted yet.
	out, err := detectDrift(cfg, slug, featurePath)
	if err != nil {
		t.Fatalf("detectDrift (pre-edit): %v", err)
	}
	if out.HasDrift {
		t.Fatalf("has_drift = true immediately after baseline; want false (shared=%v)", out.SharedSourcesChanged)
	}

	// Edit the shared model, exactly as the Studio editor's PUT would.
	if err := os.WriteFile(domainPath, []byte("schema_version: 1\nentities:\n  - name: Widget\n  - name: Gadget\n"), 0o644); err != nil {
		t.Fatalf("rewrite domain: %v", err)
	}

	out, err = detectDrift(cfg, slug, featurePath)
	if err != nil {
		t.Fatalf("detectDrift (post-edit): %v", err)
	}
	if !out.HasDrift {
		t.Error("has_drift = false after a domain-model edit; the drift checker is blind to the shared model")
	}
	if len(out.SharedSourcesChanged) != 1 || out.SharedSourcesChanged[0] != "domain-model" {
		t.Errorf("shared_sources_changed = %v, want [domain-model]", out.SharedSourcesChanged)
	}
	if len(out.Drifted) != 0 {
		t.Errorf("Drifted = %v, want empty — no intent changed, only the shared model", out.Drifted)
	}
}

// TestHashBuildfileSections_V2RoutesResolved is the WP2.1 hasher regression.
// In a v2 (multi-target) buildfile the routes: block relocates under
// targets.presentation.routes:. The hasher read a raw top-level routes: only,
// so every multi-target project produced NO routes section hash — the
// cross-cutting regeneration signal never saw a route change. The hasher now
// falls back to the presentation target, so the section is present and hashes
// the relocated node.
func TestHashBuildfileSections_V2RoutesResolved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "buildfile.yaml")
	buildfile := `feature: notes
adapter-set: notes-stack
models:
  Note: {}
fixtures:
  default:
    data: {}
targets:
  presentation:
    adapter: react-antd
    routes:
      - path: /notes
        page: NotesPage
`
	if err := os.WriteFile(path, []byte(buildfile), 0o644); err != nil {
		t.Fatalf("write buildfile: %v", err)
	}

	sections, err := hashBuildfileSections(path)
	if err != nil {
		t.Fatalf("hashBuildfileSections: %v", err)
	}
	if _, ok := sections["routes"]; !ok {
		t.Fatal("no routes section hash for a v2 buildfile; the hasher is still blind to targets.presentation.routes")
	}

	// The routes hash must equal hashing the relocated node itself, proving
	// the fallback hashed the right content.
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(buildfile), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	presRoutes := raw["targets"].(map[string]interface{})["presentation"].(map[string]interface{})["routes"]
	rb, _ := yaml.Marshal(presRoutes)
	if want := sha256Hex(string(rb)); sections["routes"] != want {
		t.Errorf("routes hash = %q, want %q (hash of the presentation routes node)", sections["routes"], want)
	}
	// models and fixtures stay top-level in both shapes and must still hash.
	if _, ok := sections["models"]; !ok {
		t.Error("models section missing — top-level models: must still hash in v2")
	}
	if _, ok := sections["fixtures"]; !ok {
		t.Error("fixtures section missing — top-level fixtures: must still hash in v2")
	}
}

// TestHashBuildfileSections_V1Unchanged pins that the v2-aware fallback does
// not touch the v1 path: a top-level routes: hashes exactly as it always did.
func TestHashBuildfileSections_V1Unchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "buildfile.yaml")
	buildfile := `feature: notes
adapter: go-cli
routes:
  - path: /notes
    page: NotesPage
`
	if err := os.WriteFile(path, []byte(buildfile), 0o644); err != nil {
		t.Fatalf("write buildfile: %v", err)
	}
	sections, err := hashBuildfileSections(path)
	if err != nil {
		t.Fatalf("hashBuildfileSections: %v", err)
	}
	var raw map[string]interface{}
	yaml.Unmarshal([]byte(buildfile), &raw)
	rb, _ := yaml.Marshal(raw["routes"])
	if want := sha256Hex(string(rb)); sections["routes"] != want {
		t.Errorf("v1 routes hash = %q, want %q — the fallback must not change v1 hashing", sections["routes"], want)
	}
}

// The design-spec surface was retired (amendment design-spec-surface-retired,
// 2026-08-31): the Baseline struct no longer carries design-spec-fragments /
// design-spec-shared fields. Baselines written BEFORE the retirement may still
// contain them on disk; yaml decoding must ignore the removed keys and drift
// detection must not report spurious dirtiness because of them. This pins the
// amendment's decode-safety promise, since every former design-spec test was
// deleted with the feature.
func TestBaseline_LegacyDesignSpecFieldsDecodeSafely(t *testing.T) {
	setupTestDir(t)
	cfg := testContext(t)
	slug := "legacy-design-spec"
	writeFeatureFiles(t, slug, "# Feature\n\n## An intent\n\n**Goal**: g\n**Persona**: p\n", "", "")

	// Write a real baseline first, then splice the retired keys into it so
	// everything else (hashes, schema version) is genuinely current.
	baseline, err := buildBaseline(cfg, slug)
	if err != nil {
		t.Fatalf("buildBaseline: %v", err)
	}
	writeBaseline(t, slug, *baseline)
	blPath := baselinePath(cfg, slug)
	data, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	legacy := string(data) + "design-spec-fragments:\n  hero: abc123\ndesign-spec-shared: def456\n"
	if err := os.WriteFile(blPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy baseline: %v", err)
	}

	out, err := detectDrift(cfg, slug, cfg.FeaturePath(slug))
	if err != nil {
		t.Fatalf("detectDrift must decode a baseline carrying retired design-spec keys: %v", err)
	}
	if out.HasDrift {
		t.Errorf("no source changed; retired design-spec keys must not surface as drift: %+v", out)
	}
}
