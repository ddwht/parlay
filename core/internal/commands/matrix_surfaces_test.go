// parlay-artifact: test
//
// THE SURFACE LIST. Every entry point in parlay that emits a verdict about
// a project, written down in one place for the first time.
//
// That it did not exist is most of why this file does. parlay grew roughly
// seventeen surfaces that each answer "is this project healthy?", each with
// its own finding shape, its own severity vocabulary and its own exit-code
// logic. Several compute the same property twice, independently, and
// disagree — and when they disagree, the more reassuring one is usually the
// one a user is pointed at. Nobody could see that, because no artifact in
// the tree listed the surfaces side by side.
//
// The companion file matrix_test.go runs every surface here against every
// fixture and records (output, exit code) as a golden file. A disagreement
// is then visible as two adjacent lines in one file — `check-composition:
// coherent:true` beside `scaffold-seed: derivable:false` — rather than as
// something a person has to go looking for.
//
// ADDING A SURFACE. If you add a command that answers a question about
// project health, add it here. The test that keeps this list honest is
// TestMatrix_SurfaceListCoversTheCheckCommands, which fails when a
// `check-*` command exists that this list does not name.

package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// verdict is the three-valued answer the plan this work implements puts at
// its centre. Today's surfaces answer a two-valued question — is this fine?
// — so when one cannot tell, it has to pick, and it picks fine. That
// collapse is the shape of nearly every finding this work closes.
//
// It lives in the test rather than in production code deliberately. Giving
// parlay a real Verdict type is a larger change than the evidence currently
// supports (the source review found the finding *shape* already largely
// unified — what diverges is the envelope and the grading). Deriving the
// verdict here, from each surface's own output, needs no production change
// and still makes every disagreement assertable. It also documents, in one
// readable place, what each surface's output actually grades to — which is
// the specification by-product this matrix exists to produce.
type verdict string

const (
	// satisfied — checked, and it holds.
	satisfied verdict = "satisfied"
	// violated — checked, and it does not hold.
	violated verdict = "violated"
	// indeterminate — COULD NOT be checked. Never aggregates into
	// satisfied. A surface reporting this has established nothing, and a
	// caller must not read it as a clean bill of health.
	indeterminate verdict = "indeterminate"
)

// worst returns the more severe of two verdicts, on the ordering
// satisfied < indeterminate < violated. This is the aggregation rule: a
// surface's verdict is the worst of its findings, so one "I couldn't check"
// is enough to stop the whole thing reading as fine.
func worst(a, b verdict) verdict {
	rank := map[verdict]int{satisfied: 0, indeterminate: 1, violated: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// surface is one verdict-emitting entry point, as a user types it.
type surface struct {
	// name is the command as typed, including flags: "verify-generated --strict".
	name string
	// args are the positional arguments, usually none or one "@feature".
	args []string
	// flags, when set, is called before the run to put package-level flag
	// vars into the state this surface represents. matrix_test.go restores
	// every flag afterwards via resetFlagsAfterTest, so a surface that sets
	// one cannot leak into the next.
	flags func()
	// flagSet is the real command's FlagSet, snapshotted for restoration.
	// Nil when the surface sets no flags.
	flagSet *pflag.FlagSet
	// run is the handler. Every one of them has this signature already,
	// which is what makes a table like this possible at all.
	run func(*cobra.Command, []string) error
	// verdictOf grades this surface's own output. Reading the exit code
	// alone would not do: half the findings this work closes ARE exit-code
	// bugs, so grading by exit code would make the matrix agree with the
	// bug. The verdict comes from what the surface SAID; the exit code is
	// recorded separately and checked against it.
	verdictOf func(stdout string, exitCode int) verdict
	// noInput distinguishes "there was nothing recorded to check against"
	// from "there was something and I could not interpret it". Both grade
	// as indeterminate — neither establishes anything — but only the second
	// is a reason for --strict to fail.
	//
	// The distinction is load-bearing and easy to get backwards. A project
	// that has never generated code has no snapshot, and generate-code
	// documents exactly that as the first-generation signal; failing
	// --strict on it would break every greenfield pipeline on its first
	// run, and would train people to stop passing --strict, which costs
	// more than it buys. A project that HAS a snapshot it cannot read is
	// the opposite case and must fail.
	//
	// Nil means the surface always had input to check.
	noInput func(stdout string) bool
	// onlyFixtures restricts this surface to the named fixtures. Empty
	// means every fixture.
	//
	// Used only where running the surface everywhere would produce a cell
	// that says nothing: `validate --type domain-model` against a project
	// with no domain model reports the file is unreadable, which is true
	// and useless — a project without a domain model is a normal state,
	// not a finding, and a column of them would bury the rows that matter.
	onlyFixtures []string
}

// appliesTo reports whether this surface runs against the named fixture.
func (s surface) appliesTo(fixture string) bool {
	if len(s.onlyFixtures) == 0 {
		return true
	}
	for _, f := range s.onlyFixtures {
		if f == fixture {
			return true
		}
	}
	return false
}

// matrixSurfaces is the list. Ordered as a user would meet them: the
// project-wide checks first, then the per-feature ones, then the surfaces
// that report rather than gate.
//
// Surfaces deliberately absent, with reasons — this list is a claim about
// coverage, so the gaps belong in it:
//
//   - build-feature, generate-code, create-artifacts and the other pipeline
//     commands: they produce artifacts rather than verdicts. They fail, but
//     on their own work, not on a judgement about the project.
//   - the editor's POST /validate and its save gate: they live in
//     internal/editor and cannot be reached from this package. They are the
//     largest known gap in this matrix. The save gate's dropped `code` field
//     (R4-23) is invisible here for exactly that reason.
//   - init, upgrade, sync, add-feature, move-feature: mutations, not checks.
//   - repair --dry-run and migrate-ledger --dry-run: they report a plan
//     rather than a verdict, and running them needs a build tree in a
//     specific broken state per fixture rather than one shared fixture.
//     Worth adding when a fixture motivates it. (migrate-ledger's verdict
//     states are pinned directly in migrate_ledger_test.go instead.)
func matrixSurfaces() []surface {
	return []surface{
		{
			name:      "check-composition",
			run:       runCheckComposition,
			verdictOf: verdictFromCoherent,
		},
		{
			name:      "scaffold-seed",
			run:       runScaffoldSeed,
			verdictOf: verdictFromDerivable,
		},
		{
			// --json because the matrix grades machine-readable output.
			// Rendering must never participate in grading, so the prose
			// form is expected to reach the same verdict; where a surface
			// offers both, the matrix takes the one a script would read.
			name:      "status --json",
			run:       runStatus,
			flags:     func() { statusJSON = true },
			flagSet:   statusCmd.Flags(),
			verdictOf: verdictFromStatus,
		},
		{
			name:      "verify-generated",
			run:       runVerifyGenerated,
			verdictOf: verdictFromVerify,
			noInput:   noRecordedSnapshot,
		},
		{
			name:      "verify-generated --strict",
			run:       runVerifyGenerated,
			flags:     func() { verifyGeneratedStrict = true },
			flagSet:   verifyGeneratedCmd.Flags(),
			verdictOf: verdictFromVerify,
			noInput:   noRecordedSnapshot,
		},
		{
			name: "validate --type domain-model --json",
			args: []string{"domain-model.yaml"},
			run:  runValidate,
			flags: func() {
				validateType = "domain-model"
				validateJSON = true
			},
			flagSet:      validateCmd.Flags(),
			verdictOf:    verdictFromFindingArray,
			onlyFixtures: []string{"domain-model-invalid", "domain-model-deprecated"},
		},
		{
			name:      "check-coverage",
			run:       runCheckCoverage,
			verdictOf: verdictFromExitAndBody,
		},
		{
			name:      "check-drift",
			run:       runCheckDrift,
			verdictOf: verdictFromDrift,
		},
	}
}

// runSurface executes one surface against one prepared fixture and returns
// what it printed and what it exited with.
//
// Exit code is captured, not inferred. Half the findings in this family are
// exit-code bugs and exit codes are absent from most existing tests, so a
// matrix that recorded output alone would be blind to the half of the
// problem that hurts CI.
func runSurface(t *testing.T, s surface, cfg *config.Context) (stdout string, exitCode int) {
	t.Helper()

	if s.flagSet != nil {
		resetFlagsAfterTest(t, s.flagSet)
	}
	if s.flags != nil {
		s.flags()
	}
	// Undo the flag mutation as soon as this surface is done, so the next
	// surface in the same fixture's row does not inherit it. The
	// resetFlagsAfterTest cleanup above is the backstop for a t.Fatal.
	defer func() {
		if s.flagSet != nil {
			s.flagSet.VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) })
		}
	}()

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := s.run(cmd, s.args)
	return out.String(), exitCodeOf(err)
}

// ---------------------------------------------------------------------
// The graders.
//
// One per surface, each reading that surface's own output and saying what
// it amounts to. Collectively they are the first written-down statement of
// what parlay's checks actually grade to — which is the by-product Phase 0
// of the plan promised, and the thing that makes "these two surfaces
// disagree" a claim a test can make.
//
// Each grader answers only from the output. Where a surface cannot express
// a distinction — check-drift printing the same has_drift:false whether it
// compared against a baseline or found none — the grader cannot invent it,
// and the gap shows up in TestMatrix_NoSurfaceAnswersSatisfiedWhenItCouldNotCheck
// as a tracked entry rather than as a silent pass.
// ---------------------------------------------------------------------

// verdictFromCoherent grades check-composition.
//
// Records == 0 is indeterminate, not satisfied. A coherence verdict over a
// project that contributed no fixture records established nothing: there
// was nothing to compare. Reporting that as coherent is the "I had nothing
// to check it against" reading of green that this whole effort exists to
// separate from "I checked this and it is good."
func verdictFromCoherent(stdout string, _ int) verdict {
	var out compositionOutput
	if json.Unmarshal([]byte(stdout), &out) != nil {
		return indeterminate
	}
	if out.Records == 0 {
		return indeterminate
	}
	if !out.Coherent {
		return violated
	}
	// An unbuilt feature means coherence was established over a subset —
	// the command says so itself ("Coherence was checked over the remaining
	// features only"). That is a real limit on what the verdict covers, so
	// it cannot be a clean satisfied.
	//
	// The finding arrives under `notes`, not `findings`: notes deliberately
	// do not flip Coherent, because an unbuilt feature contributes nothing
	// and so cannot make the built ones disagree. That reasoning is sound
	// for the coherence question and is exactly why the third verdict is
	// needed — "the features I could see agree" is not "the project is
	// coherent", and today only one word is available for both.
	for _, n := range out.Notes {
		if n.Code == "composition-feature-unbuilt" {
			return indeterminate
		}
	}
	return satisfied
}

// verdictFromDerivable grades scaffold-seed. The derivation refusing is the
// strongest statement in the tree about whether the composed runtime is
// coherent — it is the surface check-composition's contradiction verdict is
// now equivalent to, by construction.
func verdictFromDerivable(stdout string, _ int) verdict {
	var out struct {
		Derivable bool `json:"derivable"`
		Records   []struct {
			ID string `json:"id"`
		} `json:"records"`
		Contributors map[string]string `json:"contributors"`
	}
	if json.Unmarshal([]byte(stdout), &out) != nil {
		return indeterminate
	}
	if !out.Derivable {
		return violated
	}
	if len(out.Contributors) == 0 {
		return indeterminate
	}
	return satisfied
}

// verdictFromStatus grades status. An orphaned build directory is a tree
// inconsistency; a project with no features at all has nothing to report on.
func verdictFromStatus(stdout string, _ int) verdict {
	var out struct {
		Root struct {
			Features []struct {
				Phase string `json:"phase"`
			} `json:"features"`
			OrphanedBuildDirs []string `json:"orphaned_build_dirs"`
		} `json:"root"`
	}
	if json.Unmarshal([]byte(stdout), &out) != nil {
		return indeterminate
	}
	if len(out.Root.OrphanedBuildDirs) > 0 {
		return violated
	}
	if len(out.Root.Features) == 0 {
		return indeterminate
	}
	return satisfied
}

// verdictFromVerify grades verify-generated.
//
// has_hashes:false is indeterminate: there is no recorded state to compare
// against, so nothing was verified. `unknown` is indeterminate by its own
// definition — it is the bucket that exists so an uncertified snapshot
// cannot read as a clean bill of health. `modified` joins adopted and
// missing on the violated side under the decision recorded in
// emitVerifyJSON: a file parlay cannot account for is not confirmed safe to
// overwrite.
func verdictFromVerify(stdout string, _ int) verdict {
	var out verifyOutput
	if json.Unmarshal([]byte(stdout), &out) != nil {
		return indeterminate
	}
	if !out.HasHashes {
		return indeterminate
	}
	v := satisfied
	if len(out.Unknown) > 0 {
		v = worst(v, indeterminate)
	}
	if len(out.Adopted) > 0 || len(out.Modified) > 0 || len(out.Missing) > 0 {
		v = worst(v, violated)
	}
	return v
}

// noRecordedSnapshot reports that verify-generated found no code-hashes
// sidecar at all — the documented first-generation state, not a check that
// failed.
func noRecordedSnapshot(stdout string) bool {
	var out verifyOutput
	if json.Unmarshal([]byte(stdout), &out) != nil {
		return false
	}
	return !out.HasHashes
}

// verdictFromDrift grades check-drift.
//
// NOTE THE BLIND SPOT, which is the point of recording it here: this output
// carries no way to distinguish "compared against a baseline, nothing
// drifted" from "found no baseline, so compared against nothing". Both print
// has_drift:false. The grader therefore cannot return indeterminate for the
// second case however much it should, and the no-baseline fixture's entry in
// knownIndeterminacyGaps is what keeps that visible.
func verdictFromDrift(stdout string, _ int) verdict {
	var out driftOutput
	if json.Unmarshal([]byte(stdout), &out) != nil {
		return indeterminate
	}
	if out.HasDrift {
		return violated
	}
	return satisfied
}

// verdictFromExitAndBody grades check-coverage: an intent no dialog covers
// is a gap in the chain, and a feature with no intents at all was not
// checked.
func verdictFromExitAndBody(stdout string, _ int) verdict {
	var out struct {
		Covered   []struct{} `json:"covered"`
		Uncovered []string   `json:"uncovered"`
	}
	if json.Unmarshal([]byte(stdout), &out) != nil {
		return indeterminate
	}
	if len(out.Uncovered) > 0 {
		return violated
	}
	if len(out.Covered) == 0 {
		return indeterminate
	}
	return satisfied
}

// verdictFromFindingArray grades the bare finding array that
// `validate --type domain-model --json` emits.
//
// Severity is what decides, not count: a warning is a real finding that is
// not a reason to fail, which is why a deprecation notice must not stop
// someone committing a model. This mirrors blockingCount in validate.go
// rather than reimplementing the judgement, so the matrix cannot come to a
// different view of the same findings than the command does.
func verdictFromFindingArray(stdout string, _ int) verdict {
	var findings []struct {
		Severity string `json:"severity"`
	}
	if json.Unmarshal([]byte(stdout), &findings) != nil {
		return indeterminate
	}
	v := satisfied
	for _, f := range findings {
		if f.Severity == "warning" {
			continue
		}
		v = worst(v, violated)
	}
	return v
}

// normalizeOutput makes a surface's output stable enough to commit. Temp
// directory paths change every run, so the fixture root is replaced with a
// fixed token; everything else is recorded verbatim, which is the whole
// value of a golden file.
//
// Both spellings of the root are replaced. On macOS the temp directory is
// reached through a /private symlink, and whether a given path is recorded
// resolved or unresolved depends on whether it passed through EvalSymlinks
// on its way — testContext resolves, os.Stat-derived paths do not. Handling
// only one spelling left absolute machine paths in the golden files, which
// makes them unreviewable and unportable.
func normalizeOutput(s, root string) string {
	out := strings.ReplaceAll(s, root, "<project>")
	if unresolved := strings.TrimPrefix(root, "/private"); unresolved != root {
		out = strings.ReplaceAll(out, unresolved, "<project>")
	}
	return strings.TrimRight(out, "\n")
}

// exitCodeOf maps a handler's error to the process exit code it would
// produce. A plain error is cobra's own failure path, which exits 1.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
