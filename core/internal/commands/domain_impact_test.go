// parlay-feature: domain-model-editor/feature-contributions
// parlay-component: domain-impact
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// setupContributionProject builds a temp project with a root domain model
// and, per feature, an optional contribution / capabilities.yaml / buildfile.
type featureFiles struct {
	contribution string
	capabilities string
	buildfile    string
}

func setupContributionProject(t *testing.T, rootModel string, features map[string]featureFiles) *config.Context {
	t.Helper()
	dir := setupTestDir(t)

	if rootModel != "" {
		if err := os.WriteFile(filepath.Join(dir, config.DomainModelFile), []byte(rootModel), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for feature, files := range features {
		featDir := filepath.Join(dir, config.SpecDir, config.IntentsDir, filepath.FromSlash(feature))
		if err := os.MkdirAll(featDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte("# "+feature+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if files.contribution != "" {
			if err := os.WriteFile(filepath.Join(featDir, ContributionFile), []byte(files.contribution), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if files.capabilities != "" {
			if err := os.WriteFile(filepath.Join(featDir, "capabilities.yaml"), []byte(files.capabilities), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if files.buildfile != "" {
			buildDir := filepath.Join(dir, config.ParlayDir, config.BuildDir, filepath.FromSlash(feature))
			if err := os.MkdirAll(buildDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte(files.buildfile), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return testContext(t)
}

func runDomainImpactForTest(t *testing.T, cfg *config.Context, feature string, apply bool) (domainImpactOutput, error) {
	t.Helper()
	prev := domainImpactApply
	domainImpactApply = apply
	t.Cleanup(func() { domainImpactApply = prev })

	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	err := runDomainImpact(cmd, []string{feature})

	var out domainImpactOutput
	if buf.Len() > 0 {
		if decodeErr := json.Unmarshal(buf.Bytes(), &out); decodeErr != nil {
			t.Fatalf("decode output: %v (raw: %s)", decodeErr, buf.String())
		}
	}
	return out, err
}

const rootWithReport = `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: uuid
`

// A project that authors only the root model behaves exactly as it did
// before contributions existed — the command says so rather than failing.
func TestDomainImpact_NoContributionIsNotAFailure(t *testing.T) {
	cfg := setupContributionProject(t, rootWithReport, map[string]featureFiles{
		"submit-expense": {},
	})
	out, err := runDomainImpactForTest(t, cfg, "submit-expense", false)
	if err != nil {
		t.Fatalf("a feature with no contribution must not fail: %v", err)
	}
	if out.Contributed {
		t.Errorf("contributed should be false: %#v", out)
	}
	if !out.Applicable {
		t.Errorf("nothing proposed is trivially applicable: %#v", out)
	}
}

// The whole feature has to be invisible to a project that does not use it.
// Reading a contribution that is not there must not touch the root model, and
// the capabilities cross-reference must grade exactly as it did before.
func TestNoContributionsLeavesTheProjectUntouched(t *testing.T) {
	cfg := setupContributionProject(t, rootWithReport, map[string]featureFiles{
		"submit-expense": {capabilities: `schema_version: 1
feature: submit-expense
operations:
  - id: list
    kind: query
    subject:
      entity: ExpenseReport
    steps:
      - type: read-many
`},
	})

	before, err := os.ReadFile(cfg.DomainModelPath())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runDomainImpactForTest(t, cfg, "submit-expense", false); err != nil {
		t.Fatalf("domain-impact on a project with no contributions: %v", err)
	}
	if got := loadContributions(cfg); got != nil {
		t.Errorf("a project with no contribution files has no contributions: %#v", got)
	}

	after, err := os.ReadFile(cfg.DomainModelPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("the root model changed:\n%s", after)
	}
}

// The report names the other features and the fixtures a new field reaches.
func TestDomainImpact_ReportsWhoANewFieldAffects(t *testing.T) {
	cfg := setupContributionProject(t, rootWithReport, map[string]featureFiles{
		"submit-expense": {contribution: `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: settledAt
        type: datetime
`},
		"dashboard": {
			capabilities: `schema_version: 1
feature: dashboard
operations:
  - id: list
    kind: query
    subject:
      entity: ExpenseReport
    steps:
      - type: read-many
`,
			buildfile: `feature: dashboard
fixtures:
  seed:
    composes: true
    data:
      ExpenseReport:
        - id: rep-1
          title: Berlin
`,
		},
	})

	out, err := runDomainImpactForTest(t, cfg, "submit-expense", false)
	if err != nil {
		t.Fatalf("domain-impact: %v", err)
	}
	if !out.Contributed || !out.Applicable {
		t.Fatalf("want an applicable contribution: %#v", out)
	}
	if len(out.Additions) != 1 || out.Additions[0].Path != "entities.ExpenseReport.fields.settledAt" {
		t.Fatalf("additions = %#v", out.Additions)
	}
	if len(out.Affects) != 1 {
		t.Fatalf("affects = %#v", out.Affects)
	}
	a := out.Affects[0]
	if len(a.Features) != 1 || a.Features[0] != "dashboard" {
		t.Errorf("the capabilities reference must put dashboard in the audience: %#v", a)
	}
	if len(a.Fixtures) != 1 || a.Fixtures[0].Feature != "dashboard" || a.Fixtures[0].Fixture != "seed" {
		t.Errorf("the fixture holding ExpenseReport records must be named: %#v", a.Fixtures)
	}
	if len(a.Fixtures[0].Fields) != 1 || a.Fixtures[0].Fields[0] != "settledAt" {
		t.Errorf("the fixture entry must name the field it would need: %#v", a.Fixtures[0])
	}
}

// Applying merges the additions into the root model, through the editor's
// save path.
func TestDomainImpact_ApplyMergesIntoTheRootModel(t *testing.T) {
	cfg := setupContributionProject(t, rootWithReport, map[string]featureFiles{
		"approvals": {contribution: `schema_version: 1
entities:
  - name: Approval
    fields:
      - name: id
        type: uuid
`},
	})

	out, err := runDomainImpactForTest(t, cfg, "approvals", true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !out.Applied {
		t.Errorf("applied should be true: %#v", out)
	}

	after, err := os.ReadFile(cfg.DomainModelPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "Approval") {
		t.Errorf("the contribution did not land:\n%s", after)
	}
	if !strings.Contains(string(after), "ExpenseReport") {
		t.Errorf("the merge dropped what was already there:\n%s", after)
	}
}

// A conflicting contribution refuses, writes nothing, and still reports why.
func TestDomainImpact_ApplyOnAConflictRefusesAndWritesNothing(t *testing.T) {
	cfg := setupContributionProject(t, rootWithReport, map[string]featureFiles{
		"submit-expense": {contribution: `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: string
`},
	})

	before, err := os.ReadFile(cfg.DomainModelPath())
	if err != nil {
		t.Fatal(err)
	}

	out, err := runDomainImpactForTest(t, cfg, "submit-expense", true)
	if err == nil {
		t.Fatal("a conflicting contribution must not apply")
	}
	if out.Applied {
		t.Error("applied must stay false on a refusal")
	}
	if len(out.Conflicts) != 1 {
		t.Errorf("the refusal must still report the conflict: %#v", out.Conflicts)
	}

	after, err := os.ReadFile(cfg.DomainModelPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("the root model changed despite the refusal:\n%s", after)
	}
}

// Reporting a conflicting contribution without --apply still exits non-zero:
// the loop reads the exit code to decide whether the boundary can be crossed.
func TestDomainImpact_AConflictExitsNonZeroWithoutApply(t *testing.T) {
	cfg := setupContributionProject(t, rootWithReport, map[string]featureFiles{
		"submit-expense": {contribution: `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: string
`},
	})
	out, err := runDomainImpactForTest(t, cfg, "submit-expense", false)
	if err == nil {
		t.Error("a conflicting contribution should exit non-zero")
	}
	if out.Applicable {
		t.Error("applicable must be false")
	}
}

// The whole point of the pending softening: a feature referencing an entity
// another feature proposes gets a warning naming the proposer, not an error
// it has to work around with a placeholder.
func TestValidateCapabilities_ReferenceToAProposedEntityIsPending(t *testing.T) {
	cfg := setupContributionProject(t, rootWithReport, map[string]featureFiles{
		"approvals": {contribution: `schema_version: 1
entities:
  - name: Approval
    fields:
      - name: id
        type: uuid
`},
		"review-queue": {capabilities: `schema_version: 1
feature: review-queue
operations:
  - id: decide
    kind: command
    subject:
      entity: Approval
    steps:
      - type: update-one
`},
	})

	capPath := filepath.Join(cfg.FeaturePath("review-queue"), "capabilities.yaml")
	prev := validateType
	validateType = "capabilities"
	t.Cleanup(func() { validateType = prev })

	cmd := testCommandWithContext(t, cfg)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	if err := runValidate(cmd, []string{capPath}); err != nil {
		t.Fatalf("a pending entity must not fail validation: %v (stderr: %s)", err, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "capabilities-entity-pending") {
		t.Errorf("want capabilities-entity-pending on stderr, got: %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "approvals") {
		t.Errorf("the warning must name the proposing feature: %q", errBuf.String())
	}
}
