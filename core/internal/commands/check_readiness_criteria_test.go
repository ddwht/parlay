// parlay-feature: parlay-tool/multi-adapter
// parlay-component: criteria-presence
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCriteriaFeature(t *testing.T, featureDir, surface, capabilities string) {
	t.Helper()
	if surface != "" {
		if err := os.WriteFile(filepath.Join(featureDir, "surface.yaml"), []byte(surface), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if capabilities != "" {
		if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"), []byte(capabilities), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const vacantSurface = `feature: my-feature
fragments:
  - name: Customers list
    shows: list-of-items
    source: "@my-feature/browse-customers"
    page: customers
    region: main
`

const populatedCapabilities = `feature: my-feature
operations:
  - id: customer.list
    kind: query
    source: "@my-feature/browse-customers"
    verify:
      - "returns customers in name order"
`

// The designer->build boundary reports a fragment with no criteria, and gives a
// fix. This is the last point at which the omission is cheap: every coverage
// walker downstream asks whether STATED criteria are discharged, and a fragment
// stating none passes them all by vacancy.
func TestReadiness_ReportsCriteriaVacancy(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeCriteriaFeature(t, featureDir, vacantSurface, populatedCapabilities)

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "my-feature")

	frag := readinessHasCode(issues, "surface-fragment-no-criteria")
	if frag == nil {
		t.Fatalf("no surface-fragment-no-criteria issue; got %+v", issues)
	}
	if frag.Severity != "warning" {
		t.Errorf("severity = %q, want warning — an error here becomes a gate blocker", frag.Severity)
	}
	if frag.Fix == "" {
		t.Error("the issue carries no fix, so the driver cannot tell the user what to do")
	}
	if readinessHasCode(issues, "feature-surface-no-criteria") == nil {
		t.Errorf("the aggregate did not fire on a wholly vacant surface; got %+v", issues)
	}
	if readinessHasCode(issues, "capability-operation-no-criteria") != nil {
		t.Error("an operation carrying criteria was reported as having none")
	}
}

// A populated contract passes the boundary silently.
func TestReadiness_PopulatedCriteriaAreSilent(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	populatedSurface := `feature: my-feature
fragments:
  - name: Customers list
    shows: list-of-items
    source: "@my-feature/browse-customers"
    page: customers
    region: main
    verify:
      - "shows each customer's name and email"
`
	writeCriteriaFeature(t, featureDir, populatedSurface, populatedCapabilities)

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "my-feature")
	for _, code := range []string{"surface-fragment-no-criteria", "feature-surface-no-criteria", "capability-operation-no-criteria"} {
		if readinessHasCode(issues, code) != nil {
			t.Errorf("%s fired on a fully-populated contract", code)
		}
	}
}

// A pure infrastructure feature has no surface and no capabilities. It must
// stay quiet: "visible" claims live on operations when there are no fragments,
// and a feature with neither artifact has nothing to be vacant about.
func TestReadiness_InfrastructureFeatureStaysQuiet(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	infra := "## Boundary\n\n**Affects**: the edge\n**Behavior**: it holds\n**Invariants**: it stays held\n"
	if err := os.WriteFile(filepath.Join(featureDir, "infrastructure.md"), []byte(infra), 0o644); err != nil {
		t.Fatal(err)
	}

	issues := checkBuildFeatureReadiness(testContext(t), featureDir, "my-feature")
	for _, code := range []string{"surface-fragment-no-criteria", "feature-surface-no-criteria", "capability-operation-no-criteria"} {
		if readinessHasCode(issues, code) != nil {
			t.Errorf("%s fired on a feature with no surface and no capabilities", code)
		}
	}
}

// The `validate` wiring, which the readiness tests do not cover: presence is a
// cross-artifact condition, so it cannot ride inside ValidateSurface (whose
// signature sees one file's bytes) and is wrapped around it instead.
func TestValidate_CriteriaPresenceInputsResolveBothArtifacts(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	writeCriteriaFeature(t, featureDir, vacantSurface, populatedCapabilities)

	cmd := testCommandWithContext(t, testContext(t))

	// Reached via either artifact: wiring only one would make the same
	// condition appear or vanish depending on which file was validated.
	for _, artifact := range []string{"surface.yaml", "capabilities.yaml"} {
		in, ok := criteriaPresenceInputs(cmd, filepath.Join(featureDir, artifact))
		if !ok {
			t.Fatalf("%s: no feature resolved", artifact)
		}
		if !in.HasSurface || len(in.Fragments) != 1 {
			t.Errorf("%s: surface not loaded (HasSurface=%v, fragments=%d)", artifact, in.HasSurface, len(in.Fragments))
		}
		if len(in.Operations) != 1 {
			t.Errorf("%s: capabilities not loaded (operations=%d)", artifact, len(in.Operations))
		}
	}
}

// A path outside any feature resolves to nothing, and the walker stays silent
// rather than reporting a vacancy it cannot actually see.
func TestValidate_CriteriaPresenceSkipsUnresolvablePaths(t *testing.T) {
	dir := setupTestDir(t)
	setupLedgerFeature(t, dir)
	cmd := testCommandWithContext(t, testContext(t))

	if _, ok := criteriaPresenceInputs(cmd, filepath.Join(dir, "surface.yaml")); ok {
		t.Error("a path outside the intents tree resolved to a feature")
	}
}

// The same symlink hazard in the sibling gatherer. A caller-supplied path is
// the only one at risk — the other Rel-against-root sites compare paths derived
// from walking the root itself, so both sides come from one source.
func TestValidate_TestcasesCoverageInputsResolveThroughSymlinks(t *testing.T) {
	dir := setupTestDir(t)
	setupLedgerFeature(t, dir)
	cmd := testCommandWithContext(t, testContext(t))

	buildDir := testContext(t).BuildPath("my-feature")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tcPath := filepath.Join(buildDir, "testcases.yaml")
	if err := os.WriteFile(tcPath, []byte("schema_version: 2\nfeature: my-feature\nsuites: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCriteriaFeature(t, filepath.Join(dir, "spec", "intents", "my-feature"), "", populatedCapabilities)

	in := testcasesCoverageInputs(cmd, tcPath)
	if len(in.CanonicalOperations) != 1 {
		t.Errorf("operations not derived (%d) — the feature failed to resolve from the path", len(in.CanonicalOperations))
	}
	if len(in.Criteria) != 1 {
		t.Errorf("criteria not derived (%d)", len(in.Criteria))
	}
}
