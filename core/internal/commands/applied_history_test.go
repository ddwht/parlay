package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// WP3 — applied history stops having to resolve against a contract that has
// legitimately moved on, but ONLY when "applied" is a checked fact.

const historyAmendment = `---
amendment: tighten-create
date: 2026-08-13
trigger: "duplicate names slipped through"
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
Creation rejects duplicate names case-insensitively.

## Acceptance
- Duplicates are rejected.
`

// seedAppliedHistory writes 001 against the verify fixture, applies it in the
// baseline with its real hash, then removes the operation it names — the
// retirement-shaped state where history refers to a contract entry that has
// been legitimately disposed of.
func seedAppliedHistory(t *testing.T, dir string, mutate func(*Baseline)) string {
	t.Helper()
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-tighten-create.md", historyAmendment)

	cfg := testContext(t)
	if err := os.MkdirAll(cfg.BuildPath("verify-fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash, ok := hashWholeFile(filepath.Join(featDir, "amendments", "001-tighten-create.md"))
	if !ok {
		t.Fatal("fixture: amendment must hash")
	}
	bl := Baseline{
		SchemaVersion:        BaselineSchemaVersion,
		LastAppliedAmendment: 1,
		Sources:              &HashedSources{Amendments: map[string]string{"001-tighten-create.md": hash}},
	}
	if mutate != nil {
		mutate(&bl)
	}
	data, err := marshalBaseline(&bl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath(cfg, "verify-fixture"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return featDir
}

// dropCapabilities removes the artifact the historical ref resolves against,
// which is what retirement is required to do.
func dropCapabilities(t *testing.T, featDir string) {
	t.Helper()
	if err := os.Remove(filepath.Join(featDir, "capabilities.yaml")); err != nil {
		t.Fatal(err)
	}
}

func unresolvedIssues(out checkAmendmentsOutput) []string {
	var got []string
	for _, iss := range out.Issues {
		if iss.Code == "amendment-affects-unresolved" {
			got = append(got, iss.Message)
		}
	}
	return got
}

// The headline. An applied record whose stored hash still matches its retained
// bytes is history; its ref may stop resolving without that being an error, so
// retirement can dispose of the feature-local contract at all.
func TestWP3_TrustedAppliedRefToleratesDisposedContract(t *testing.T) {
	dir := setupTestDir(t)
	featDir := seedAppliedHistory(t, dir, nil)
	dropCapabilities(t, featDir)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if got := unresolvedIssues(out); len(got) != 0 {
		t.Errorf("a trusted applied record's ref is history and must not be fatal; got %v", got)
	}
	// It stays in the audit footprint — dropping it would make all_affects
	// silently lose exactly the retired history this tolerance preserves.
	if len(out.AllAffects) != 1 {
		t.Errorf("all_affects = %v, want the historical ref retained for audit", out.AllAffects)
	}
	// And it is emphatically NOT dirty: applied history scopes no rebuild.
	if len(out.DirtySet) != 0 {
		t.Errorf("dirty_set = %v, want empty — a trusted historical ref scopes no rebuild",
			out.DirtySet)
	}
}

// Compaction moves applied records to amendments/archive/. Trust must follow
// them there, or compacting a ledger would silently un-trust its history.
//
// This is asserted against the PREDICATE directly, not through
// check-amendments. LoadFeatureAmendments skips subdirectories, so once a
// record is archived the ledger walk never sees it and never evaluates its
// ref — "zero unresolved issues" would then prove nothing at all. Whether
// archived records should rejoin the semantic walk is a much larger question
// that conflicts with what compaction is for; it is deliberately not what this
// pins.
func TestWP3_TrustFollowsCompactionIntoArchive(t *testing.T) {
	dir := setupTestDir(t)
	featDir := seedAppliedHistory(t, dir, nil)
	cfg := testContext(t)

	// Capture the parsed record BEFORE archiving it — afterwards the loader
	// cannot see it.
	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(amendments) != 1 {
		t.Fatalf("fixture: want 1 amendment, got %d", len(amendments))
	}
	record := amendments[0]

	capsule, err := observeAppliedAuthority(cfg, "verify-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !amendmentTrustedApplied(capsule, featDir, record) {
		t.Fatal("fixture: the record must be trusted while still in the active ledger")
	}

	archive := filepath.Join(featDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(record.Path, filepath.Join(archive, filepath.Base(record.Path))); err != nil {
		t.Fatal(err)
	}

	if !amendmentTrustedApplied(capsule, featDir, record) {
		t.Error("archived history is retained history: a compacted record whose bytes still " +
			"match its stored hash must stay trusted, or compacting a ledger would silently " +
			"un-trust everything in it")
	}

	// And an edit to the archived copy destroys that trust, exactly as an edit
	// to the active copy would.
	if err := os.WriteFile(filepath.Join(archive, filepath.Base(record.Path)),
		[]byte("---\namendment: tighten-create\ndate: 2026-08-13\n---\n## Change\nrewritten\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if amendmentTrustedApplied(capsule, featDir, record) {
		t.Error("a mutated archived record must lose trust — history that was rewritten is not " +
			"evidence of what was applied")
	}
}

// The active copy wins when both exist. A stale archive copy must never stand
// in for a mutated active one, or an edit to a live record could hide behind
// history that still matches.
func TestWP3_ActiveCopyTakesPrecedenceOverArchive(t *testing.T) {
	dir := setupTestDir(t)
	featDir := seedAppliedHistory(t, dir, nil)
	cfg := testContext(t)

	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		t.Fatal(err)
	}
	record := amendments[0]
	name := filepath.Base(record.Path)

	// A pristine copy in archive/, and a MUTATED one still active.
	archive := filepath.Join(featDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	pristine, err := os.ReadFile(record.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, name), pristine, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record.Path, append(pristine, []byte("\nedited\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	capsule, err := observeAppliedAuthority(cfg, "verify-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if amendmentTrustedApplied(capsule, featDir, record) {
		t.Error("the mutated ACTIVE record must decide trust; a matching archive copy must not " +
			"stand in for it, or an edit to a live record hides behind stale history")
	}
}

// The marker alone is not authority. A record the marker covers but for which
// no hash was ever recorded has nothing tying it to specific bytes — which is
// exactly what a hand-moved marker looks like.
func TestWP3_MarkerWithoutStoredHashIsNotTrusted(t *testing.T) {
	dir := setupTestDir(t)
	featDir := seedAppliedHistory(t, dir, func(b *Baseline) {
		b.Sources.Amendments = map[string]string{}
	})
	dropCapabilities(t, featDir)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if got := unresolvedIssues(out); len(got) != 1 {
		t.Errorf("a marker with no recorded evidence must not confer trust; unresolved = %v", got)
	}
}

// Evidence that does not match the bytes is not evidence.
func TestWP3_HashMismatchIsNotTrusted(t *testing.T) {
	dir := setupTestDir(t)
	featDir := seedAppliedHistory(t, dir, func(b *Baseline) {
		b.Sources.Amendments["001-tighten-create.md"] = "0000badc0ffee000"
	})
	dropCapabilities(t, featDir)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if got := unresolvedIssues(out); len(got) != 1 {
		t.Errorf("a stored hash that no longer matches the record must not confer trust; "+
			"unresolved = %v", got)
	}
}

// A record ABOVE the marker is pending however good its hash looks. Recording
// a hash is not applying a record.
func TestWP3_MatchingHashAboveMarkerIsStillPending(t *testing.T) {
	dir := setupTestDir(t)
	featDir := seedAppliedHistory(t, dir, func(b *Baseline) {
		b.LastAppliedAmendment = 0
	})
	dropCapabilities(t, featDir)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if got := unresolvedIssues(out); len(got) != 1 {
		t.Errorf("a record above the marker is pending and its refs stay fatal; unresolved = %v", got)
	}
}

// An unreadable baseline is its own finding. Degrading to "nothing applied"
// would turn every historical ref back into a fatal one, which reads as drift
// rather than as a broken baseline.
func TestWP3_UnreadableAuthorityIsAnExplicitIssue(t *testing.T) {
	dir := setupTestDir(t)
	featDir := seedAppliedHistory(t, dir, nil)
	dropCapabilities(t, featDir)
	if err := os.WriteFile(baselinePath(testContext(t), "verify-fixture"),
		[]byte("{{ not: [valid yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	var found bool
	for _, iss := range out.Issues {
		if iss.Code == "amendment-authority-unreadable" {
			found = true
			if iss.Severity != "error" {
				t.Errorf("severity = %q, want error", iss.Severity)
			}
		}
	}
	if !found {
		t.Errorf("an unreadable authority record must be reported as itself, not silently "+
			"degraded; issues = %+v", out.Issues)
	}
	// The other half of fail-closed: with no readable authority, nothing is
	// trusted, so the historical ref stays fatal rather than being tolerated.
	if got := unresolvedIssues(out); len(got) != 1 {
		t.Errorf("an unreadable capsule must trust nothing, leaving the historical ref fatal; "+
			"unresolved = %v", got)
	}
}

// Scope. Only the three feature-local kinds deadlock. A domain ref is
// root-scoped and outlives its own feature's retirement; it must not gain a
// broad exemption on the strength of a trusted marker.
func TestWP3_DomainRefKeepsExistingResolution(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-domain-ref.md", `---
amendment: domain-ref
date: 2026-08-13
trigger: "entity reshaped"
affects:
  - "@verify-fixture/domain:NoSuchEntity"
---

## Change
The entity changed.

## Acceptance
- It changed.
`)
	cfg := testContext(t)
	if err := os.MkdirAll(cfg.BuildPath("verify-fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash, _ := hashWholeFile(filepath.Join(featDir, "amendments", "001-domain-ref.md"))
	bl := Baseline{
		SchemaVersion:        BaselineSchemaVersion,
		LastAppliedAmendment: 1,
		Sources:              &HashedSources{Amendments: map[string]string{"001-domain-ref.md": hash}},
	}
	data, err := marshalBaseline(&bl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath(cfg, "verify-fixture"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	got := unresolvedIssues(out)
	if len(got) != 1 {
		t.Fatalf("a domain ref is root-scoped and keeps its existing resolution behaviour even "+
			"under a trusted marker; unresolved = %v", got)
	}
	if !strings.Contains(got[0], "NoSuchEntity") {
		t.Errorf("the unresolved domain entity must be named; got %q", got[0])
	}
}

// Scope, the other half. A cross-feature ref resolves against ANOTHER
// feature's contract, whose disposal is that feature's own drift
// responsibility. A trusted marker on this feature confers nothing over there.
func TestWP3_CrossFeatureRefKeepsExistingResolution(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-cross-feature.md", `---
amendment: cross-feature
date: 2026-08-13
trigger: "another feature's contract moved"
affects:
  - "@other-feature/operation:gone.away"
---

## Change
The other feature's operation changed.

## Acceptance
- It changed.
`)
	cfg := testContext(t)
	if err := os.MkdirAll(cfg.BuildPath("verify-fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash, _ := hashWholeFile(filepath.Join(featDir, "amendments", "001-cross-feature.md"))
	bl := Baseline{
		SchemaVersion:        BaselineSchemaVersion,
		LastAppliedAmendment: 1,
		Sources:              &HashedSources{Amendments: map[string]string{"001-cross-feature.md": hash}},
	}
	data, err := marshalBaseline(&bl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath(cfg, "verify-fixture"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if got := unresolvedIssues(out); len(got) != 1 {
		t.Errorf("a cross-feature ref is the other feature's responsibility and stays fatal "+
			"under a trusted marker here; unresolved = %v", got)
	}
}

// All three permitted kinds are tolerated, not just the operation case the
// headline test exercises.
func TestWP3_AllThreeFeatureLocalKindsAreTolerated(t *testing.T) {
	for _, tc := range []struct{ kind, name string }{
		{"operation", "thing.create"},
		{"surface", "thing-list"},
		{"infrastructure", "some-boundary"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			dir := setupTestDir(t)
			featDir := writeVerifyFixture(t, dir)
			writeAmendment(t, featDir, "001-kind.md", "---\namendment: kind\ndate: 2026-08-13\naffects:\n  - \"@verify-fixture/"+tc.kind+":"+tc.name+"\"\n---\n\n## Change\nchanged\n\n## Acceptance\n- done\n")

			cfg := testContext(t)
			if err := os.MkdirAll(cfg.BuildPath("verify-fixture"), 0o755); err != nil {
				t.Fatal(err)
			}
			hash, _ := hashWholeFile(filepath.Join(featDir, "amendments", "001-kind.md"))
			bl := Baseline{
				SchemaVersion:        BaselineSchemaVersion,
				LastAppliedAmendment: 1,
				Sources:              &HashedSources{Amendments: map[string]string{"001-kind.md": hash}},
			}
			data, err := marshalBaseline(&bl)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(baselinePath(cfg, "verify-fixture"), data, 0o644); err != nil {
				t.Fatal(err)
			}
			// Dispose of every feature-local contract artifact, as retirement does.
			for _, f := range []string{"capabilities.yaml", "surface.yaml", "infrastructure.md"} {
				_ = os.Remove(filepath.Join(featDir, f))
			}

			out, _ := runCheckAmendments_(t, "@verify-fixture")
			if got := unresolvedIssues(out); len(got) != 0 {
				t.Errorf("a trusted applied %s ref must be tolerated once its target is "+
					"disposed; unresolved = %v", tc.kind, got)
			}
			if len(out.AllAffects) != 1 {
				t.Errorf("the tolerated %s ref must stay in the audit footprint; all_affects = %v",
					tc.kind, out.AllAffects)
			}
		})
	}
}
