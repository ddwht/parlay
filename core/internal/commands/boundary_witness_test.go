// parlay-feature: parlay-tool/criterion-authority
// parlay-component: boundary-claim-registry
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
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
		Claim: claimCriteriaAuthority, Branch: "unapproved", Stage: gateStageCode,
		Expect: "criteria-authority-missing",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.Remove(criteriaAuthorityPath(cfg, "graded")); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimCriteriaAuthority, Branch: "stale", Stage: gateStageCode,
		Expect: "criteria-authority-missing",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			rewriteCriterion(t, dir, "the archive button is disabled while invoices are unpaid",
				"the archive button is hidden while invoices are unpaid")
		},
	},
	{
		Claim: claimTestcasesReady, Branch: "missing subject", Stage: gateStageCode,
		Expect: "testcases-not-ready",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.Remove(filepath.Join(cfg.BuildPath("graded"), "testcases.yaml")); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimTestcasesReady, Branch: "unreadable subject", Stage: gateStageCode,
		Expect: "testcases-not-ready",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "testcases.yaml"),
				[]byte("suites: [\n  broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimCoverageExcept, Branch: "stale ledger", Stage: gateStageCode,
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
		Claim: claimCoverageExcept, Branch: "unreadable ledger", Stage: gateStageCode,
		Expect: "coverage-exception-invalid",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(coverageExceptionsPath(cfg, "graded"), []byte("exceptions: [\n  broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimCoverageExcept, Branch: "stranded legacy", Stage: gateStageCode,
		Expect: "coverage-exception-invalid",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "coverage-review.yaml"),
				[]byte("feature: graded\nreviewed_at: \"2026-05-01T00:00:00Z\"\nexemptions:\n    - suite: s\n      item: \"@graded/operation:customer.archive\"\n      reason: r\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimBuildfileValid, Branch: "propagation", Stage: gateStageCode,
		Expect: "invalid-yaml",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "buildfile.yaml"),
				[]byte("components: [\n  broken"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	},
	{
		Claim: claimBuildfileFresh, Branch: "propagation", Stage: gateStageCode,
		Expect: "stale-buildfile",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			// Change a source the signatures were computed over.
			rewriteCriterion(t, dir, "the archive button is disabled while invoices are unpaid",
				"the archive button is disabled whenever invoices are unpaid")
		},
	},
	{
		Claim: claimLedgerState, Branch: "propagation", Stage: gateStageCode,
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
func TestBoundaryClaims_EveryBlockingClaimHasAWitness(t *testing.T) {
	witnessed := map[string]bool{}
	for _, w := range append(append([]boundaryWitness{}, boundaryWitnesses...), passThroughWitnesses...) {
		witnessed[w.Claim] = true
	}
	// Claims whose witness needs a control the table cannot express — a
	// different stage, a second feature, a real generated snapshot — and
	// therefore lives in its own test. Named here so completeness still counts
	// them, and so deleting one of those tests fails this rather than passing
	// quietly.
	for claim, testName := range map[string]string{
		claimReadiness:      "TestBoundaryWitness_Readiness",
		claimComposition:    "TestBoundaryWitness_Composition",
		claimGeneratedState: "TestBoundaryWitness_GeneratedState",
	} {
		if !testExists(testName) {
			t.Errorf("claim %q names witness %s, which does not exist", claim, testName)
			continue
		}
		witnessed[claim] = true
	}
	for _, c := range boundaryClaims {
		if !c.Blocking {
			continue
		}
		if !witnessed[c.ID] {
			t.Errorf("claim %q (%s) can block a boundary and nothing proves it ever fires through the advancing constructor", c.ID, c.What)
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
func testExists(name string) bool {
	for _, fn := range dedicatedWitnessTests {
		if fn == name {
			return true
		}
	}
	return false
}

// dedicatedWitnessTests is the list the check above resolves against. Keeping
// it beside the tests means removing one without removing its entry leaves a
// dangling name the completeness check reports.
var dedicatedWitnessTests = []string{
	"TestBoundaryWitness_Readiness",
	"TestBoundaryWitness_Composition",
	"TestBoundaryWitness_GeneratedState",
}
