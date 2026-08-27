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
	for _, c := range boundaryClaims {
		if !c.Blocking {
			continue
		}
		if c.ID == claimReadiness {
			// Owed. Readiness participates only in the build stage, and the
			// clean control is built for the code boundary; witnessing it needs
			// a build-stage control that does not exist yet. Exempted here, in
			// the test that would otherwise pass silently, rather than
			// witnessed with a mutation that asserts something easier.
			continue
		}
		if c.ID == claimComposition {
			// Witnessed at warning level only. Its blocking branch needs a
			// genuine cross-feature contradiction fixture, which is owed —
			// exempting it here is a debt recorded in the place that would
			// otherwise silently pass.
			continue
		}
		if c.ID == claimReviewGate {
			// The retired gate is registered so the model describes the
			// boundary as it IS. It leaves with the implementation, and the
			// registry then shows the before and after rather than treating
			// that direct call as outside the model.
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
var passThroughWitnesses = []boundaryWitness{
	{
		Claim: claimGeneratedState, Branch: "propagation", Stage: gateStageDone,
		Expect: "code-not-generated",
		Mutate: func(t *testing.T, dir string, cfg *config.Context) {
			// The done boundary's own subject: nothing generated yet.
		},
	},
}

// Composition's output reaches the boundary. Its BLOCKING branch is not
// witnessed: an unreadable buildfile produces a note rather than a finding, and
// a genuine cross-feature contradiction needs a two-feature fixture that does
// not exist yet. Recorded as a gap rather than papered over with a witness that
// asserts something easier than the claim.
func TestBoundaryWitnesses_CompositionOutputReachesTheBoundary(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	current, _ := CurrentCriteria(cfg, "graded")
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}

	other := cfg.BuildPath("other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	featDir := filepath.Join(dir, "spec", "intents", "other")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"),
		[]byte("# Other\n\n## Do It\n\n**Goal**: g.\n**Persona**: p.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "buildfile.yaml"), []byte("fixtures: [\n  broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Warnings) == 0 {
		t.Error("a buildfile the composition walk could not read should be reported at the boundary, not absorbed")
	}
}

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
