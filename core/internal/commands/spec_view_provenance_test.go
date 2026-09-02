// parlay-feature: parlay-tool/ledger-and-contract
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
)

// THE DEFECT THIS CLOSES.
//
// The banner used to read "what follows is what the code currently
// answers to" — a claim about generated code, from a command that reads
// intents.md and the applied ledger and nothing else. It held in the
// clean steady state, because save-build-state blesses code at the same
// moment the amendment applies, and it stopped holding mid-refinement
// and after any hand-edit.
//
// Nothing in the wording can be right, because the promises half IS the
// promise set — not the code — whatever the code happens to be doing.
func TestSpecView_BannerDoesNotClaimToDescribeCode(t *testing.T) {
	// Not one literal. "what follows is what currently runs" would revive
	// the defect and pass a substring check for "the code", so this
	// rejects the whole family of claims about execution.
	banner := strings.ToLower(specPendingBanner())
	for _, forbidden := range []string{"the code", "what runs", "currently runs", "the binary", "executes", "in production"} {
		if strings.Contains(banner, forbidden) {
			t.Errorf("the pending banner claims to describe execution (%q): %q", forbidden, specPendingBanner())
		}
	}
	// And it must positively say what it IS showing.
	if !strings.Contains(banner, "promise set") {
		t.Errorf("the banner should name what it actually shows: %q", specPendingBanner())
	}
}

// The four answers are kept apart because they are different facts. A
// reader who cannot tell "verified in sync" from "never recorded" draws
// opposite conclusions from the same silence.
func TestDescribeCodeProvenance_UnrecordedIsNotMatching(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")

	got := describeCodeProvenance(cfg, "widget")
	if got.State != codeProvenanceUnrecorded {
		t.Fatalf("a feature with no hashes must report %q, got %q", codeProvenanceUnrecorded, got.State)
	}
	if got.State == codeProvenanceMatching {
		t.Fatal("absence of provenance is not evidence of agreement")
	}
	if !strings.Contains(got.Detail, "cannot be checked") {
		t.Errorf("the detail must say the check was not possible: %q", got.Detail)
	}
}

// writeCodeHashes lays down a real snapshot so describeCodeProvenance is
// exercised rather than simulated. The previous version of these tests
// built the result struct AND its prose by hand, so it passed whether or
// not the function existed.
func writeCodeHashes(t *testing.T, cfg *config.Context, slug string, files map[string]CodeHashEntry) {
	t.Helper()
	h := CodeHashes{SchemaVersion: CodeHashesSchemaVersion, GeneratedAt: "2026-09-01T00:00:00Z", Files: files}
	data, err := yaml.Marshal(&h)
	if err != nil {
		t.Fatal(err)
	}
	path := codeHashesPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A real snapshot with a modified file and a missing one, driven through
// the real function.
func TestDescribeCodeProvenance_MovedCountsRealFiles(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")

	// One file whose bytes DIFFER from what was recorded. Deliberately not
	// called a hand-edit: verify-generated is explicit that Modified is
	// ambiguous between a hand-edit and an ordinary regeneration, and this
	// summariser cannot tell which either.
	differing := filepath.Join(featurePath, "differing.go")
	if err := os.WriteFile(differing, []byte("package x // not what was recorded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// One ADOPTED: bytes match, but a save recorded it as written by
	// something other than codegen.
	adopted := filepath.Join(featurePath, "adopted.go")
	if err := os.WriteFile(adopted, []byte("package y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adoptedHash, err := hashFileContent(adopted)
	if err != nil {
		t.Fatal(err)
	}
	writeCodeHashes(t, cfg, "widget", map[string]CodeHashEntry{
		differing:                             {Component: "c", Hash: "sha256:0000", Provenance: ProvenanceGenerated},
		adopted:                               {Component: "c", Hash: adoptedHash, Provenance: ProvenanceAdopted},
		filepath.Join(featurePath, "gone.go"): {Component: "c", Hash: "sha256:1111", Provenance: ProvenanceGenerated},
	})

	got := describeCodeProvenance(cfg, "widget")
	if got.State != codeProvenanceMoved {
		t.Fatalf("want %q, got %q (%s)", codeProvenanceMoved, got.State, got.Detail)
	}
	// Modified AND Adopted both fold into moved here; pinning only one
	// would let the fold silently drop the other.
	if got.Moved != 2 {
		t.Errorf("differing + adopted must both count as moved: got %d", got.Moved)
	}
	if got.Missing != 1 {
		t.Errorf("a recorded file that is gone must count as missing: got %d", got.Missing)
	}
	if !strings.Contains(got.Detail, "2 changed") || !strings.Contains(got.Detail, "1 missing") {
		t.Errorf("the detail must carry the real counts: %q", got.Detail)
	}
}

// The Unknown bucket: a v0 snapshot predates provenance, so every file it
// records is unaccounted for.
func TestDescribeCodeProvenance_UnknownProvenanceCounts(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	onDisk := filepath.Join(featurePath, "legacy.go")
	if err := os.WriteFile(onDisk, []byte("package z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := hashFileContent(onDisk)
	if err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(&CodeHashes{
		GeneratedAt: "2026-09-01T00:00:00Z",
		Files:       map[string]CodeHashEntry{onDisk: {Component: "c", Hash: h}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := codeHashesPath(cfg, "widget")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got := describeCodeProvenance(cfg, "widget")
	if got.State != codeProvenanceMoved {
		t.Fatalf("unaccounted files are not a clean bill: want %q, got %q", codeProvenanceMoved, got.State)
	}
	if got.Unknown != 1 {
		t.Errorf("want 1 unknown, got %d (%s)", got.Unknown, got.Detail)
	}
}

// THE FOURTH STATE, reached for real rather than injected.
//
// Without this, `unreadable` existed only in a struct the renderer test
// built by hand, so the error branch could later collapse into
// `unrecorded` with nothing to notice. Those are different facts: a
// missing baseline versus a broken one, and they send a reader to
// different places.
func TestDescribeCodeProvenance_UnreadableSnapshotIsNotUnrecorded(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	path := codeHashesPath(cfg, "widget")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("files: [this is not\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := describeCodeProvenance(cfg, "widget")
	if got.State != codeProvenanceUnreadable {
		t.Fatalf("a corrupt snapshot must be %q, not %q", codeProvenanceUnreadable, got.State)
	}
	if !strings.Contains(got.Detail, "could not be read") {
		t.Errorf("the detail must say the read failed: %q", got.Detail)
	}
}

// An untouched file matching its recorded hash is the silent case.
func TestDescribeCodeProvenance_MatchingIsReachable(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	onDisk := filepath.Join(featurePath, "generated.go")
	content := []byte("package x\n")
	if err := os.WriteFile(onDisk, content, 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := hashFileContent(onDisk)
	if err != nil {
		t.Fatal(err)
	}
	writeCodeHashes(t, cfg, "widget", map[string]CodeHashEntry{
		onDisk: {Component: "c", Hash: h, Provenance: ProvenanceGenerated},
	})

	got := describeCodeProvenance(cfg, "widget")
	if got.State != codeProvenanceMatching {
		t.Fatalf("want %q, got %q (%s)", codeProvenanceMatching, got.State, got.Detail)
	}
}

// RENDERING, through the real writer. The previous test reimplemented
// `state == matching` and never called it, so it passed whether the
// renderer printed every state or none.
func TestSpecView_RendersEveryNonMatchingStateExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		state  string
		detail string
		want   int
	}{
		{codeProvenanceMatching, "the generated code still matches what was blessed", 0},
		{codeProvenanceMoved, "the generated code has moved since it was blessed: 2 changed", 1},
		{codeProvenanceUnrecorded, "no generated-code provenance is recorded for this feature", 1},
		{codeProvenanceUnreadable, "the generated-code provenance could not be read", 1},
	} {
		t.Run(tc.state, func(t *testing.T) {
			cp := specCodeProvenance{State: tc.state, Detail: tc.detail}
			out := specViewOutput{
				Feature: "widget", Promises: []specPromise{}, Retired: []specPromise{},
				ContractStatus: "empty", CurrentCodeProvenance: &cp,
			}
			var buf bytes.Buffer
			writeSpecView(&buf, out)
			if got := strings.Count(buf.String(), tc.detail); got != tc.want {
				t.Errorf("state %q: want the detail %d time(s), got %d\n%s", tc.state, tc.want, got, buf.String())
			}
		})
	}
}

// END TO END, through runSpecView, so the field cannot be wired wrongly.
func TestSpecView_UnrecordedProvenanceReachesBothOutputs(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")

	human := runSpecFor(t, cfg, false)
	if !strings.Contains(human, "no generated-code provenance is recorded") {
		t.Errorf("the human view must report unrecorded provenance:\n%s", human)
	}
	jsonOut := runSpecFor(t, cfg, true)
	var env specViewOutput
	if err := json.Unmarshal([]byte(jsonOut), &env); err != nil {
		t.Fatalf("json did not parse: %v\n%s", err, jsonOut)
	}
	if env.CurrentCodeProvenance == nil {
		t.Fatal("the current view must carry the provenance field")
	}
	if env.CurrentCodeProvenance.State != codeProvenanceUnrecorded {
		t.Errorf("want %q, got %q", codeProvenanceUnrecorded, env.CurrentCodeProvenance.State)
	}
	if env.Derivation.CodeProvenance == "" {
		t.Error("the derivation must say what the provenance answer is derived from")
	}
}

func runSpecFor(t *testing.T, cfg *config.Context, asJSON bool) string {
	t.Helper()
	prev := specViewJSON
	specViewJSON = asJSON
	t.Cleanup(func() { specViewJSON = prev })
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runSpecView(cmd, []string{"@widget"}); err != nil {
		t.Fatalf("spec view failed: %v", err)
	}
	return buf.String()
}

// A HISTORICAL view omits the field entirely.
//
// describeCodeProvenance compares today's files against today's blessed
// snapshot, which is independent of --at. Beside a projection of the
// promises at amendment N, a bare `matching` reads as "the code matches
// THIS projection" — a claim it never establishes. The text happened to
// avoid the false inference because matching is silent; JSON did not.
func TestSpecView_HistoricalViewOmitsCodeProvenance(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	writeAmendment(t, featurePath, "001-first.md", amend001)

	// The applied marker must be AHEAD of the point being asked for, or
	// there is no historical projection to test — `--at 0` against a
	// feature that has applied nothing is just the current view, and the
	// field belongs there. A first attempt at this test asserted omission
	// in exactly that case and was asserting the wrong thing.
	bpath := baselinePath(cfg, "widget")
	if err := os.MkdirAll(filepath.Dir(bpath), 0o755); err != nil {
		t.Fatal(err)
	}
	// The marker alone is refused: the strict reader wants the EVIDENCE
	// that 001 was applied, because resolving without it would answer with
	// the text that preceded the amendment while claiming to answer with
	// the amendment applied. So record the hash the applier would have
	// recorded.
	amPath := filepath.Join(featurePath, "amendments", "001-first.md")
	hash, ok := hashWholeFile(amPath)
	if !ok {
		t.Fatalf("could not hash %s", amPath)
	}
	baseline := Baseline{
		LastAppliedAmendment: 1,
		Sources:              &HashedSources{Amendments: map[string]string{"001-first.md": hash}},
		AppliedAt:            map[string]string{"001-first.md": "2026-09-01T00:00:00Z"},
	}
	data, err := yaml.Marshal(&baseline)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bpath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	prevAt, prevJSON := specViewAt, specViewJSON
	specViewAt, specViewJSON = "0", true
	t.Cleanup(func() { specViewAt, specViewJSON = prevAt, prevJSON })

	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	// Never Skip here. A skipped test covers nothing while looking like
	// it covers something, which is the failure this whole file was just
	// rewritten to remove.
	if err := runSpecView(cmd, []string{"@widget"}); err != nil {
		t.Fatalf("the historical projection must work for this to prove anything: %v", err)
	}

	var env specViewOutput
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("json did not parse: %v\n%s", err, buf.String())
	}
	if env.CurrentCodeProvenance != nil {
		t.Errorf("a historical view must omit the current-code answer, got %+v", env.CurrentCodeProvenance)
	}
	// And the raw JSON must not carry the key at all — omitempty on a
	// pointer, so a consumer sees absence rather than a null to interpret.
	if strings.Contains(buf.String(), "current_code_provenance") {
		t.Errorf("the key should be absent on a historical view:\n%s", buf.String())
	}
}
