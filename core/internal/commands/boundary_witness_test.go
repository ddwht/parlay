// parlay-feature: parlay-tool/criterion-authority
// parlay-component: boundary-claim-registry
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// boundaryWitness proves one claim can actually hold a boundary shut.
//
// The pattern is deliberately clean-then-mutate. A mutation test with no clean
// control proves nothing: if the unmutated fixture already blocks, a blocker in
// the mutated one is not evidence the mutation caused it — and this release
// shipped exactly that mistake, a test that passed because its mutation
// silently did nothing.
type boundaryWitness struct {
	// Claim this witnesses, by registry ID.
	Claim string
	// Branch distinguishes several witnesses of one claim. Wrapper claims
	// render many codes over independent subject constructors, and one row per
	// claim would let six of seven guarantees stay unreachable while the claim
	// looked covered.
	Branch string
	Stage  string
	// Mutate introduces the defect into an otherwise-clean feature.
	Mutate func(t *testing.T, dir string, cfg *config.Context)
	// Expect is the code the mutation must produce.
	Expect string
}

var boundaryWitnesses = []boundaryWitness{
	{
		Claim: claimCriteriaAuthority, Branch: branchUnapproved, Stage: gateStageCode,
		Expect: "criteria-authority-missing",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.Remove(criteriaAuthorityPath(cfg, "graded")); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimCriteriaAuthority, Branch: branchStaleState, Stage: gateStageCode,
		Expect: "criteria-authority-missing",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			rewriteCriterion(t, dir, "the archive button is disabled while invoices are unpaid",
				"the archive button is hidden while invoices are unpaid")
		},
	},
	{
		Claim: claimTestcasesReady, Branch: branchSubjectMissing, Stage: gateStageCode,
		Expect: "testcases-not-ready",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.Remove(filepath.Join(cfg.BuildPath("graded"), "testcases.yaml")); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimTestcasesReady, Branch: branchSubjectUnreadable, Stage: gateStageCode,
		Expect: "testcases-not-ready",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "testcases.yaml"),
				[]byte("suites: [\n  broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimCoverageExcept, Branch: branchStaleState, Stage: gateStageCode,
		Expect: "coverage-exception-invalid",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := saveCoverageExceptions(cfg, "graded", &CoverageExceptions{
				Feature: "graded", GrantedAt: "2026-08-27T00:00:00Z",
				Exceptions: []CoverageException{{
					Ref: "@graded/operation:customer.archive", Text: "a claim this contract never made",
					Kind: ExceptionWaived, Reason: "r",
				}},
			}); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimCoverageExcept, Branch: branchSubjectUnreadable, Stage: gateStageCode,
		Expect: "coverage-exception-invalid",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(coverageDecisionsPath(cfg, "graded"), []byte("exceptions: [\n  broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimCoverageExcept, Branch: branchStrandedLegacy, Stage: gateStageCode,
		Expect: "coverage-exception-invalid",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "coverage-review.yaml"),
				[]byte("feature: graded\nreviewed_at: \"2026-05-01T00:00:00Z\"\nexemptions:\n    - suite: s\n      item: \"@graded/operation:customer.archive\"\n      reason: r\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimBuildfileValid, Branch: branchPropagation, Stage: gateStageCode,
		Expect: "invalid-yaml",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "buildfile.yaml"),
				[]byte("components: [\n  broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimBuildfileFresh, Branch: branchPropagation, Stage: gateStageCode,
		Expect: "stale-buildfile",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			// Change a source the signatures were computed over.
			rewriteCriterion(t, dir, "the archive button is disabled while invoices are unpaid",
				"the archive button is disabled whenever invoices are unpaid")
		},
	},
	{
		Claim: claimLedgerState, Branch: branchPropagation, Stage: gateStageCode,
		Expect: "unapplied-amendments",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			featDir := cfg.FeaturePath("graded")
			writeAmendment(t, featDir, "001-later.md", `---
amendment: later
date: 2026-08-27
affects:
  - "@graded/operation:customer.archive"
---

## Change
c

## Acceptance
- a
`)
		},
	},
}

func rewriteCriterion(t *testing.T, dir, from, to string) {
	t.Helper()
	path := filepath.Join(dir, "spec", "intents", "graded", "surface.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := replaceOnce(string(body), from, to)
	if out == string(body) {
		t.Fatalf("mutation found nothing to change; the witness would prove nothing")
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceOnce(s, from, to string) string {
	i := indexOf(s, from)
	if i < 0 {
		return s
	}
	return s[:i] + to + s[i+len(from):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Every witness: the clean fixture passes, then the mutation produces the
// claimed code. Both halves matter — the first is what makes the second mean
// anything.
func TestBoundaryWitnesses(t *testing.T) {
	for _, w := range boundaryWitnesses {
		name := w.Claim + "/" + w.Branch
		t.Run(name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg := writeCleanCodeBoundary(t, dir)
			current, err := CurrentCriteria(cfg, "graded")
			if err != nil {
				t.Fatal(err)
			}
			if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
				t.Fatal(err)
			}

			clean, err := computeGate(cfg, "graded", w.Stage)
			if err != nil {
				t.Fatal(err)
			}
			if !clean.Passed {
				t.Fatalf("the control must pass, or the mutation below proves nothing: %+v", clean.Blockers)
			}

			w.Mutate(t, dir, cfg)

			out, err := computeGate(cfg, "graded", w.Stage)
			if err != nil {
				t.Fatal(err)
			}
			if !gateHasCode(out.Blockers, w.Expect) {
				t.Errorf("mutation did not produce %s — the claim %q cannot be shown to hold a boundary shut: %+v",
					w.Expect, w.Claim, out.Blockers)
			}
		})
	}
}

// Completeness, and the reason it is not circular: stages are ASSEMBLED from
// the registry, so a checker with no claim does not run. An unwitnessed path
// therefore cannot hide by going unregistered.
func TestBoundaryClaims_EveryBlockingBranchHasAWitness(t *testing.T) {
	type key struct{ claim, branch string }

	// Claims whose witness needs a control the table cannot express — a
	// different stage, a second feature, a real generated snapshot — and
	// therefore lives in its own test. Named here so completeness still counts
	// them, and so deleting one of those tests fails this rather than passing
	// quietly. Each names WHICH branch it covers, so a standalone test can no
	// longer stand in for a whole family.
	standalone := map[key]string{
		{claimReadiness, branchPropagation}:              "TestBoundaryWitness_Readiness",
		{claimComposition, branchPropagation}:            "TestBoundaryWitness_Composition",
		{claimGeneratedState, branchSubjectUnreadable}:   "TestBoundaryWitness_GeneratedState",
		{claimGeneratedState, branchModified}:            "TestBoundaryWitness_GeneratedState",
		{claimGeneratedState, branchSubjectRemoved}:      "TestBoundaryWitness_GeneratedState",
		{claimGeneratedState, branchNotGenerated}:        "TestBoundaryWitness_GeneratedStateUnrecorded",
		{claimGeneratedState, branchAdopted}:             "TestBoundaryWitness_GeneratedStateUnrecorded",
		{claimLedgerState, branchUnappliedTail}:          "TestBoundaryWitness_LedgerUnappliedTail",
		{claimTestcasesReady, branchDowngradeUnapproved}: "TestBoundaryWitness_TestcasesDowngradeUnapproved",

		// Both need a SECOND feature to do the superseding — the table's
		// single-feature fixture cannot express a cross-feature retirement,
		// which is the only shape these branches fire on.
		{claimRetiredOutput, branchRetiredEmitted}:     "TestBoundaryWitness_RetiredContributionStillEmitted",
		{claimRetiredOutput, branchRetiredShared}:      "TestBoundaryWitness_RetiredContributionSharedPath",
		{claimRetiredOutput, branchRetiredUnaccounted}: "TestBoundaryWitness_RetiredContributionUnaccounted",
		{claimRetiredOutput, branchSubjectUnreadable}:  "TestBoundaryWitness_RetiredContributionUnreadable",
		{claimRetiredOutput, branchSubjectMissing}:     "TestBoundaryWitness_RetiredContributionBuildfileMissing",
	}

	witnessed := map[key]bool{}
	declared := map[key]bool{}
	for _, c := range boundaryClaims {
		for _, b := range c.Branches {
			declared[key{c.ID, b}] = true
		}
	}

	// Every witness must name a DECLARED branch. Without this a witness could
	// invent a branch name, satisfy completeness for something the registry
	// never claimed, and leave a real branch uncovered.
	for _, w := range append(append([]boundaryWitness{}, boundaryWitnesses...), passThroughWitnesses...) {
		k := key{w.Claim, w.Branch}
		if !declared[k] {
			t.Errorf("witness %s/%s names a branch the claim does not declare — either add it to Branches, or the witness is covering something the registry does not claim",
				w.Claim, w.Branch)
			continue
		}
		if witnessed[k] {
			t.Errorf("branch %s/%s has more than one witness; each should prove one path", w.Claim, w.Branch)
		}
		witnessed[k] = true
	}
	for k, testName := range standalone {
		if !declared[k] {
			t.Errorf("standalone witness %s covers %s/%s, which the claim does not declare", testName, k.claim, k.branch)
			continue
		}
		if !testExists(testName) {
			t.Errorf("branch %s/%s names witness %s, which does not exist", k.claim, k.branch, testName)
			continue
		}
		witnessed[k] = true
	}

	// The completeness assertion, now per BRANCH. Adding a ninth refusal path
	// to an eight-path wrapper used to require no registry change and no new
	// witness — the hiding place this registry exists to close, one level down.
	for _, c := range boundaryClaims {
		if !c.Blocking {
			continue
		}
		if len(c.Branches) == 0 {
			t.Errorf("claim %q (%s) can block a boundary and declares no branches — say which refusal paths it owns, even if that is just %q",
				c.ID, c.What, branchPropagation)
			continue
		}
		for _, b := range c.Branches {
			if !witnessed[key{c.ID, b}] {
				t.Errorf("claim %q branch %q can block a boundary and nothing proves it fires through the advancing constructor (%s)",
					c.ID, b, c.What)
			}
		}
	}
}

func TestBoundaryClaims_EveryStageEntryIsRegistered(t *testing.T) {
	for _, stage := range []string{gateStageBuild, gateStageCode, gateStageDone} {
		if len(claimsForStage(stage)) == 0 {
			t.Errorf("stage %q is assembled from no registered claims — either it checks nothing, or it checks something outside the registry", stage)
		}
	}
}

// Pass-through families get one propagation witness each: enough to prove
// errors reach the boundary, without duplicating the leaf validators' own
// conformance suites by enumerating every code they can render.
var passThroughWitnesses = []boundaryWitness{}

func TestBoundaryWitnesses_PassThroughFamilies(t *testing.T) {
	for _, w := range passThroughWitnesses {
		t.Run(w.Claim+"/"+w.Branch, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg := writeCleanCodeBoundary(t, dir)
			current, _ := CurrentCriteria(cfg, "graded")
			if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
				t.Fatal(err)
			}
			w.Mutate(t, dir, cfg)
			out, err := computeGate(cfg, "graded", w.Stage)
			if err != nil {
				t.Fatal(err)
			}
			if !gateHasCode(out.Blockers, w.Expect) {
				t.Errorf("mutation did not produce %s — %q cannot be shown to reach the boundary: %+v", w.Expect, w.Claim, out.Blockers)
			}
		})
	}
}

// --- clean controls for the other two stages -------------------------------

// Composition needs TWO coherent features before a contradiction between them
// means anything.
func writeComposingPair(t *testing.T, dir string, cfg *config.Context, role string) {
	t.Helper()
	other := filepath.Join(dir, "spec", "intents", "peer")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "intents.md"),
		[]byte("# Peer\n\n## Show It\n\n**Goal**: g.\n**Persona**: p.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bd := cfg.BuildPath("peer")
	if err := os.MkdirAll(bd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bd, "buildfile.yaml"), []byte(`schema_version: 1
feature: peer
adapter: react-antd
fixtures:
  seed:
    composes: true
    data:
      Employee:
        - id: emp-1
          role: `+role+`
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// The graded feature contributes the same entity.
	gbd := cfg.BuildPath("graded")
	body, err := os.ReadFile(filepath.Join(gbd, "buildfile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gbd, "buildfile.yaml"), append(body, []byte(`fixtures:
  seed:
    composes: true
    data:
      Employee:
        - id: emp-1
          role: manager
`)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Two features agreeing compose cleanly; two disagreeing about the same entity
// field is the contradiction. The clean half is the point — without it a
// blocker in the mutated case proves nothing about the mutation.
func TestBoundaryWitness_Composition(t *testing.T) {
	t.Run("agreeing pair passes", func(t *testing.T) {
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)
		approveClean(t, cfg)
		writeComposingPair(t, dir, cfg, "manager")

		out, err := computeGate(cfg, "graded", gateStageCode)
		if err != nil {
			t.Fatal(err)
		}
		if gateHasCode(out.Blockers, "composition-fixture-contradiction") {
			t.Fatalf("both features say manager: %+v", out.Blockers)
		}
	})

	t.Run("disagreeing pair blocks", func(t *testing.T) {
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)
		approveClean(t, cfg)
		writeComposingPair(t, dir, cfg, "employee")

		out, err := computeGate(cfg, "graded", gateStageCode)
		if err != nil {
			t.Fatal(err)
		}
		if !gateHasCode(out.Blockers, "composition-fixture-contradiction") {
			t.Errorf("one persona cannot be a manager on one page and an employee on another: %+v", out.Blockers)
		}
	})
}

func approveClean(t *testing.T, cfg *config.Context) {
	t.Helper()
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}
}

// Generated state, with the clean control the first attempt lacked entirely.
//
// That attempt had an empty mutator and no control, so it observed
// code-not-generated — a blocker present before any defect was introduced. It
// asserted the fixture's pre-existing state and called it a witness, which is
// precisely the false-positive pattern this file's header forbids.
func TestBoundaryWitness_GeneratedState(t *testing.T) {
	setup := func(t *testing.T) (string, *config.Context, string) {
		t.Helper()
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)
		approveClean(t, cfg)

		// A real tracked generated file, recorded through the project snapshot
		// the CLI actually writes.
		rel := "src/features/graded/CustomerDetail.tsx"
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "// parlay-feature: graded\n// parlay-component: customer-detail\nexport const CustomerDetail = () => null\n"
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		hash, err := hashFileContent(full)
		if err != nil {
			t.Fatal(err)
		}
		writeProjectCodeHashes(t, cfg, map[string]CodeHashEntry{
			rel: {Component: "customer-detail", Hash: hash, Provenance: ProvenanceGenerated},
		})
		return dir, cfg, full
	}

	t.Run("recorded and unchanged passes", func(t *testing.T) {
		_, cfg, _ := setup(t)
		out, err := computeGate(cfg, "graded", gateStageDone)
		if err != nil {
			t.Fatal(err)
		}
		for _, code := range []string{"code-not-generated", "generated-file-modified", "generated-file-missing"} {
			if gateHasCode(out.Blockers, code) {
				t.Fatalf("the control must pass or nothing below proves anything: %s in %+v", code, out.Blockers)
			}
		}
	})

	t.Run("edited under the record blocks", func(t *testing.T) {
		_, cfg, full := setup(t)
		if err := os.WriteFile(full, []byte("// hand-edited\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _ := computeGate(cfg, "graded", gateStageDone)
		if !gateHasCode(out.Blockers, "generated-file-modified") {
			t.Errorf("a file that changed since it was generated must be reported: %+v", out.Blockers)
		}
	})

	t.Run("removed under the record blocks", func(t *testing.T) {
		_, cfg, full := setup(t)
		if err := os.Remove(full); err != nil {
			t.Fatal(err)
		}
		out, _ := computeGate(cfg, "graded", gateStageDone)
		if !gateHasCode(out.Blockers, "generated-file-missing") {
			t.Errorf("a file recorded as generated and now gone must be reported: %+v", out.Blockers)
		}
	})

	// The branch the registry actually changed: an unreadable snapshot used to
	// contribute nothing, so it was indistinguishable from code that matches —
	// at the boundary that certifies completion.
	t.Run("unreadable snapshot blocks", func(t *testing.T) {
		_, cfg, _ := setup(t)
		if err := os.WriteFile(projectCodeHashesPath(cfg), []byte("files: [\n  broken"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, _ := computeGate(cfg, "graded", gateStageDone)
		if !gateHasCode(out.Blockers, "generated-state-unreadable") {
			t.Errorf("completion cannot be certified over a snapshot nothing could read: %+v", out.Blockers)
		}
	})
}

// Readiness participates only in the build stage, so it gets its own control
// there rather than the code fixture forced backward.
func TestBoundaryWitness_Readiness(t *testing.T) {
	writeConfig := func(t *testing.T, dir string) {
		t.Helper()
		cfgDir := filepath.Join(dir, ".parlay")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("ai-agent: Claude Code\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		adapters := filepath.Join(cfgDir, "adapters")
		if err := os.MkdirAll(adapters, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(adapters, "react-antd.adapter.yaml"),
			[]byte("name: react-antd\nkind: presentation\nfile-conventions:\n  project-root: \".\"\n  source-root: src/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, "adapter-set.yaml"),
			[]byte("targets:\n  presentation:\n    adapter: react-antd\n    root: .\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a ready feature passes the build boundary", func(t *testing.T) {
		dir := setupTestDir(t)
		writeCriteriaFixture(t, dir)
		writeConfig(t, dir)
		cfg := testContext(t)

		out, err := computeGate(cfg, "graded", gateStageBuild)
		if err != nil {
			t.Fatal(err)
		}
		if !out.Passed {
			t.Fatalf("the control must pass or the mutation below proves nothing: %+v", out.Blockers)
		}
	})

	t.Run("a vacated contract blocks it", func(t *testing.T) {
		dir := setupTestDir(t)
		writeCriteriaFixture(t, dir)
		writeConfig(t, dir)
		featDir := filepath.Join(dir, "spec", "intents", "graded")
		for _, name := range []string{"surface.yaml", "capabilities.yaml"} {
			if err := os.Remove(filepath.Join(featDir, name)); err != nil {
				t.Fatal(err)
			}
		}

		out, _ := computeGate(testContext(t), "graded", gateStageBuild)
		if !gateHasCode(out.Blockers, "no-surface-no-infrastructure") {
			t.Errorf("a feature with no contract artifact is not ready to build: %+v", out.Blockers)
		}
	})
}

// Registry integrity: the cheap invariants that keep the model from decaying
// into a list nobody maintains.
func TestBoundaryClaims_RegistryIsWellFormed(t *testing.T) {
	known := map[string]bool{gateStageBuild: true, gateStageCode: true, gateStageDone: true}
	seen := map[string]bool{}
	for _, c := range boundaryClaims {
		if c.ID == "" {
			t.Error("a claim with no ID cannot be witnessed against")
		}
		if seen[c.ID] {
			t.Errorf("claim ID %q is registered twice; one shadows the other", c.ID)
		}
		seen[c.ID] = true
		if c.Check == nil {
			t.Errorf("claim %q has no check — it would contribute nothing while appearing covered", c.ID)
		}
		if c.What == "" {
			t.Errorf("claim %q says nothing about what it checks; a failure message needs it", c.ID)
		}
		if len(c.Stages) == 0 {
			t.Errorf("claim %q participates in no stage, so it never runs", c.ID)
		}
		stages := map[string]bool{}
		for _, s := range c.Stages {
			if !known[s] {
				t.Errorf("claim %q names unknown stage %q", c.ID, s)
			}
			if stages[s] {
				t.Errorf("claim %q names stage %q twice; it would run and report twice", c.ID, s)
			}
			stages[s] = true
		}
	}
}

func TestBoundaryClaims_EveryWitnessNamesARegisteredClaim(t *testing.T) {
	registered := map[string]map[string]bool{}
	for _, c := range boundaryClaims {
		registered[c.ID] = map[string]bool{}
		for _, s := range c.Stages {
			registered[c.ID][s] = true
		}
	}
	for _, w := range append(append([]boundaryWitness{}, boundaryWitnesses...), passThroughWitnesses...) {
		stages, ok := registered[w.Claim]
		if !ok {
			t.Errorf("witness %s/%s names no registered claim", w.Claim, w.Branch)
			continue
		}
		if !stages[w.Stage] {
			t.Errorf("witness %s/%s exercises stage %q, which that claim does not participate in", w.Claim, w.Branch, w.Stage)
		}
	}
}

// testExists reports whether a named test function is compiled into this
// package, so a claim cannot point at a witness somebody deleted.
// testExists reports whether a dedicated witness test is actually present.
//
// It reads this file rather than a hand-maintained list. The list version was
// the same hiding place one more level out: renaming a test without updating
// the list broke the link silently in one direction, and deleting a test while
// leaving its entry passed in the other. Scanning source cannot drift from it.
func testExists(name string) bool {
	for _, fn := range dedicatedWitnessTests() {
		if fn == name {
			return true
		}
	}
	return false
}

var dedicatedWitnessTests = sync.OnceValue(func() []string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range witnessTestRe.FindAllStringSubmatch(string(body), -1) {
		out = append(out, m[1])
	}
	return out
})

var witnessTestRe = regexp.MustCompile(`(?m)^func (TestBoundaryWitness_\w+)\(`)

// generated-state owns two refusal paths that no other witness reached: a
// feature whose code was never generated, and a file adopted into the record
// rather than written by codegen. Both were added inside the wrapper without
// requiring a registry change, which is exactly the gap per-branch completeness
// closes.
func TestBoundaryWitness_GeneratedStateUnrecorded(t *testing.T) {
	t.Run("never generated blocks", func(t *testing.T) {
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)
		approveClean(t, cfg)
		// No snapshot at all: codegen never ran. An EMPTY snapshot is not the
		// same state — the file existing is what makes HasHashes true — so
		// writing one would have tested nothing.
		snapshot := filepath.Join(cfg.BuildPath("_project"), CodeHashesFile)
		if err := os.Remove(snapshot); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if _, err := os.Stat(snapshot); !os.IsNotExist(err) {
			t.Fatalf("precondition: no snapshot may exist for this branch to fire (%v)", err)
		}

		out, err := computeGate(cfg, "graded", gateStageDone)
		if err != nil {
			t.Fatal(err)
		}
		if !gateHasCode(out.Blockers, "code-not-generated") {
			t.Errorf("done must refuse a feature whose code was never generated: %+v", out.Blockers)
		}
	})

	t.Run("adopted file blocks", func(t *testing.T) {
		dir := setupTestDir(t)
		cfg := writeCleanCodeBoundary(t, dir)
		approveClean(t, cfg)

		rel := "src/features/graded/CustomerDetail.tsx"
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "// parlay-feature: graded\n// parlay-component: customer-detail\nexport const CustomerDetail = () => null\n"
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		hash, err := hashFileContent(full)
		if err != nil {
			t.Fatal(err)
		}
		// Recorded as adopted: written outside codegen and taken into the
		// record, which the done boundary must not treat as generated.
		writeProjectCodeHashes(t, cfg, map[string]CodeHashEntry{
			rel: {Component: "customer-detail", Hash: hash, Provenance: ProvenanceAdopted},
		})

		out, err := computeGate(cfg, "graded", gateStageDone)
		if err != nil {
			t.Fatal(err)
		}
		if !gateHasCode(out.Blockers, "generated-file-adopted") {
			t.Errorf("done must refuse a file adopted rather than generated: %+v", out.Blockers)
		}
	})
}

// gateBlockerMentions finds an inner diagnostic inside a wrapper's rendered
// message. Wrapper claims collapse many independent paths into one code, so the
// code alone cannot witness which path fired.
func gateBlockerMentions(blockers []gateBlocker, marker string) bool {
	for _, b := range blockers {
		if strings.Contains(b.Message, marker) {
			return true
		}
	}
	return false
}

// ledger-state owns a second path beside propagation: a feature whose amendment
// ledger has entries the baseline has not applied. The wrapper recomputes that
// verdict itself with journal precision — readiness's own copy is stripped in
// claimReadinessCheck — so nothing else covers it.
func TestBoundaryWitness_LedgerUnappliedTail(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveClean(t, cfg)

	clean, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !clean.Passed {
		t.Fatalf("the control must pass, or the mutation proves nothing: %+v", clean.Blockers)
	}
	if gateHasCode(clean.Blockers, "unapplied-amendments") {
		t.Fatal("precondition: the clean boundary must have no unapplied tail")
	}

	// An amendment nobody has applied.
	amendDir := filepath.Join(cfg.FeaturePath("graded"), "amendments")
	if err := os.MkdirAll(amendDir, 0o755); err != nil {
		t.Fatal(err)
	}
	amendment := strings.Join([]string{
		"---",
		"amendment: widen-the-archive-rule",
		"date: 2026-08-27",
		"trigger: \"the rule turned out to be narrower than intended\"",
		"affects:",
		"  - \"@graded/operation:customer.archive\"",
		"---",
		"",
		"## Change",
		"",
		"The archive rule widens to cover pending invoices.",
		"",
		"## Acceptance",
		"",
		"- archiving a customer with pending invoices is rejected",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(amendDir, "001-widen-the-archive-rule.md"), []byte(amendment), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "unapplied-amendments") {
		t.Errorf("an unapplied amendment must hold the code boundary shut — the feature still makes the old promise: %+v", out.Blockers)
	}
}

// testcases-readiness owns a third path: a case discharging its criterion by
// observing STATE rather than what the criterion says, with nobody having
// accepted that weakening. The case passes and cites its criterion correctly,
// so no other check can see it.
func TestBoundaryWitness_TestcasesDowngradeUnapproved(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveClean(t, cfg)

	clean, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !clean.Passed {
		t.Fatalf("the control must pass, or the mutation proves nothing: %+v", clean.Blockers)
	}

	current, err := CurrentCriteria(cfg, "graded")
	if err != nil || len(current) == 0 {
		t.Fatalf("precondition: the fixture must declare a criterion to weaken (%v)", err)
	}
	target := current[0]

	// The case cites its criterion correctly and passes; it simply observes
	// state instead of what the criterion states.
	cases := "schema_version: 3\nsuites:\n  - name: Archive\n    cases:\n" +
		"      - name: rejects unpaid\n        coverage: state-only\n" +
		"        criterion: {ref: \"" + target.Ref + "\", text: \"" + target.Text + "\"}\n" +
		"        steps: [\"check the store\"]\n"
	if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "testcases.yaml"), []byte(cases), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	// The wrapper renders all eight of its paths under one user-facing code, so
	// gateHasCode cannot tell them apart — which is precisely why completeness
	// keys on internal branch IDs rather than on rendered codes.
	if !gateBlockerMentions(out.Blockers, "criterion-observed-weakly") {
		t.Errorf("a weakened observation nobody accepted must hold the boundary shut: %+v", out.Blockers)
	}
}

// supersedeGradedDetail adds a SECOND feature whose fragment retires the clean
// fixture's "Customer Detail". Nothing inside graded/ is touched — which is the
// whole point: every feature-local signature stays byte-identical, so this is
// the exact shape that used to leave graded's component emitting into a slot it
// no longer owns.
func supersedeGradedDetail(t *testing.T, dir string) {
	t.Helper()
	featDir := filepath.Join(dir, "spec", "intents", "replacement")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `feature: replacement
fragments:
    - name: Customer Detail V2
      page: customers
      region: main
      shows: detail
      order: 1
      actions: invoke
      source: '@replacement/rework-the-detail'
      supersedes: '@graded/customer-detail'
      verify:
        - the archive button is disabled while invoices are unpaid
`
	if err := os.WriteFile(filepath.Join(featDir, "surface.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Witness: claimRetiredOutput / branchRetiredEmitted.
//
// The clean control passes, then a sibling retires graded's fragment while
// graded's plan still writes CustomerDetail.tsx. The boundary must shut: the
// superseded output would otherwise be emitted and routed beside its
// replacement — the racing pair supersedes: exists to prevent.
func TestBoundaryWitness_RetiredContributionStillEmitted(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveGradedCriteria(t, cfg)

	clean, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !clean.Passed {
		t.Fatalf("the control must pass, or the mutation proves nothing: %+v", clean.Blockers)
	}

	supersedeGradedDetail(t, dir)

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "retired-contribution-still-emitted") {
		t.Fatalf("a retired fragment whose output the plan still writes must hold the boundary shut; blockers=%+v", out.Blockers)
	}
}

// Witness: claimRetiredOutput / branchRetiredShared.
//
// Same retirement, but the path graded writes is ALSO produced for a
// still-active component. Here the file must survive — deleting it would take
// the active contributor's output with it — so the finding has to be the
// mount/route one, not the emission one. This is the distinction that makes
// removal keyed by page contribution rather than by fragment name.
func TestBoundaryWitness_RetiredContributionSharedPath(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveGradedCriteria(t, cfg)

	// A second component in graded, sourced from a fragment that is NOT
	// retired, writing the same path as the one that will be.
	bf := filepath.Join(cfg.BuildPath("graded"), "buildfile.yaml")
	data, err := os.ReadFile(bf)
	if err != nil {
		t.Fatal(err)
	}
	patched := replaceOnce(string(data),
		`    source: "@graded/fragment:Customer Detail"
plan:
  creates:
    - path: src/features/graded/CustomerDetail.tsx
      sources: ["component/customer-detail"]`,
		`    source: "@graded/fragment:Customer Detail"
  customer-summary:
    kind: component
    source: "@graded/fragment:Customer Summary"
plan:
  creates:
    - path: src/features/graded/CustomerDetail.tsx
      sources: ["component/customer-detail", "component/customer-summary"]`)
	if patched == string(data) {
		t.Fatal("fixture patch found nothing to change; the witness would prove nothing")
	}
	if err := os.WriteFile(bf, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}

	supersedeGradedDetail(t, dir)

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "retired-contribution-still-mounted") {
		t.Fatalf("a shared path must report the mount/route removal, not deletion; blockers=%+v", out.Blockers)
	}
	if gateHasCode(out.Blockers, "retired-contribution-still-emitted") {
		t.Errorf("a shared path must NOT be reported as deletable output — that would remove an active contributor's file; blockers=%+v", out.Blockers)
	}
}

func approveGradedCriteria(t *testing.T, cfg *config.Context) {
	t.Helper()
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}
}

// Witness: claimRetiredOutput / branchSubjectUnreadable.
//
// A feature with retired fragments whose buildfile cannot be parsed must not
// read as "retires nothing". Before this branch existed every unreadable input
// returned no findings, so a corrupt buildfile and a clean one produced the
// identical verdict at the boundary that claims the superseded output is gone.
func TestBoundaryWitness_RetiredContributionUnreadable(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveGradedCriteria(t, cfg)

	clean, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !clean.Passed {
		t.Fatalf("the control must pass, or the mutation proves nothing: %+v", clean.Blockers)
	}

	// Retire graded's fragment from another feature, then corrupt the
	// buildfile that would say whether its output is still emitted.
	supersedeGradedDetail(t, dir)
	if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "buildfile.yaml"),
		[]byte("components: [\n  broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "retired-contribution-unresolvable") {
		t.Fatalf("an unreadable buildfile must not read as 'retires nothing'; blockers=%+v", out.Blockers)
	}
}

// Witness: claimRetiredOutput / branchRetiredUnaccounted.
//
// The third refusal path, registered because promoting it from warning to
// error made it an independent way to hold the boundary shut — and an
// unwitnessed inner branch is exactly what the per-branch completeness
// mechanism exists to stop.
//
// Shape: the retired component's path is neither written by the plan, nor
// shared with an active contributor, nor listed in plan.deletes. Its
// previously generated output is left on disk with nothing accounting for it.
func TestBoundaryWitness_RetiredContributionUnaccounted(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveGradedCriteria(t, cfg)

	clean, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !clean.Passed {
		t.Fatalf("the control must pass, or the mutation proves nothing: %+v", clean.Blockers)
	}

	// Move the component's row out of plan.creates into plan.modifies of a
	// DIFFERENT path, so the component still resolves and still cites its
	// source, but its own output path is no longer written, shared or deleted.
	bf := filepath.Join(cfg.BuildPath("graded"), "buildfile.yaml")
	data, err := os.ReadFile(bf)
	if err != nil {
		t.Fatal(err)
	}
	patched := replaceOnce(string(data),
		`plan:
  creates:
    - path: src/features/graded/CustomerDetail.tsx
      sources: ["component/customer-detail"]`,
		`plan:
  deletes: []
  creates:
    - path: src/features/graded/Unrelated.tsx
      sources: ["component/customer-detail"]`)
	if patched == string(data) {
		t.Fatal("fixture patch found nothing to change; the witness would prove nothing")
	}
	if err := os.WriteFile(bf, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}

	supersedeGradedDetail(t, dir)

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "retired-contribution-still-emitted") &&
		!gateHasCode(out.Blockers, "retired-contribution-unaccounted") {
		t.Fatalf("a retired component whose output nothing accounts for must hold the boundary shut; blockers=%+v", out.Blockers)
	}
}

// Witness: claimRetiredOutput / branchSubjectMissing.
//
// A missing buildfile is not a clean retirement check. The buildfile can be
// deleted after code was generated, leaving the superseded output on disk with
// the one artifact that could have named it gone — so absence of the evidence
// must not read as evidence of absence.
func TestBoundaryWitness_RetiredContributionBuildfileMissing(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveGradedCriteria(t, cfg)

	clean, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !clean.Passed {
		t.Fatalf("the control must pass, or the mutation proves nothing: %+v", clean.Blockers)
	}

	supersedeGradedDetail(t, dir)
	if err := os.Remove(filepath.Join(cfg.BuildPath("graded"), "buildfile.yaml")); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "retired-contribution-subject-missing") {
		t.Fatalf("a lost buildfile with retired fragments must not read as a clean retirement check; blockers=%+v", out.Blockers)
	}
}
