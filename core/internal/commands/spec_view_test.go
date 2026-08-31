package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func runSpecView_(t *testing.T, cfg *config.Context, ref string) string {
	t.Helper()
	cmd := testCommandWithContext(t, cfg)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runSpecView(cmd, []string{ref}); err != nil {
		t.Fatalf("spec %s: %v\n%s", ref, err, buf.String())
	}
	return buf.String()
}

func armSpecView(t *testing.T, asJSON bool) {
	t.Helper()
	specViewJSON = asJSON
	t.Cleanup(func() { specViewJSON = false })
}

// The requirement the whole design exists for: after a revision is applied, the
// current specification says what the promise NOW says. Reading the founding
// document at that point gives the old text, which is exactly the failure this
// command removes — the founding text reads like present truth, so the natural
// mistake is to stop there.
func TestStage3_ProjectedPromiseShowsTheAmendedText(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := setupRevisedFeature(t, dir)

	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "A promise that now reads differently.") {
		t.Errorf("the projection must show the amended text; got:\n%s", out)
	}
	if strings.Contains(out, "See if the cluster is ready.") {
		t.Error("the projection showed the founding text, which is the state this command " +
			"exists to stop a reader trusting")
	}
	// Provenance: a reader cannot tell an untouched promise from a revised one
	// without being told which decision put the text there.
	if !strings.Contains(out, "001-reworded") || !strings.Contains(out, "revise") {
		t.Errorf("the projection must name the decision its text came from; got:\n%s", out)
	}
}

// An unapplied decision is NOT reflected, and saying so is the difference
// between a current specification and a stale one presented as current.
func TestStage3_UnappliedDecisionsAreReportedNotApplied(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	// A second decision, recorded and not applied.
	writeAmendment(t, featureDir, "002-again.md", `---
amendment: again
date: 2026-09-03
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: T
      goal: A promise nobody has applied yet.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
---

## Change
Again.

## Why
Because.

## Acceptance
- Done.
`)
	out := runSpecView_(t, cfg, "@my-feature")
	if strings.Contains(out, "A promise nobody has applied yet.") {
		t.Error("an unapplied decision was rendered as current — the code does not answer to it")
	}
	if !strings.Contains(out, "not applied") || !strings.Contains(out, "002-again") {
		t.Errorf("the unapplied tail must be reported, and named; got:\n%s", out)
	}
	// And prominently: a reader scanning for the answer must meet this before
	// the promises, not after them.
	if strings.Index(out, "002-again") > strings.Index(out, "═══ Promises") {
		t.Error("the unapplied tail was reported below the promises, where a reader who got " +
			"their answer has already stopped")
	}
}

// The two halves carry different guarantees, and the output says which is
// which. Presenting a half-derived composite as uniformly derived would be the
// more comfortable claim and the false one.
func TestStage3_TheTwoHalvesAreLabelledDifferently(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := setupRevisedFeature(t, dir)
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "PROJECTED") {
		t.Error("the promise half must be identified as projected")
	}
	if !strings.Contains(out, "STORED SNAPSHOT") {
		t.Error("the contract half must be identified as a stored snapshot — the splice edits " +
			"those files in place, and a reader who assumes both halves are derived will trust " +
			"a hand-edited file as though the tool produced it")
	}
}

// Attribution is the relation that makes the composite a specification rather
// than two lists: a promise and the contract entries that exist because of it.
func TestStage3_ContractEntriesAreShownUnderThePromiseThatJustifiesThem(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := setupRevisedFeature(t, dir)
	out := runSpecView_(t, cfg, "@my-feature")
	promiseAt := strings.Index(out, "── check-readiness ──")
	entryAt := strings.Index(out, "@my-feature/operation:x")
	if promiseAt < 0 || entryAt < 0 || entryAt < promiseAt {
		t.Errorf("the entry must appear under the promise that justifies it; got:\n%s", out)
	}
	if !strings.Contains(out, "does x") {
		t.Errorf("an entry with no description is an address, not a specification; got:\n%s", out)
	}
}

// An entry no live promise justifies is reported rather than dropped. Silently
// omitting it would make the view claim the contract is smaller than it is,
// which is the reading a person makes decisions from.
func TestStage3_UnattributedEntriesAreReported(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: orphan\n    kind: query\n    summary: nobody asked for this\n    source: \"@my-feature/long-gone\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "no live promise justifies") ||
		!strings.Contains(out, "@my-feature/operation:orphan") {
		t.Errorf("an entry with no live promise behind it must be reported; got:\n%s", out)
	}
}

// A ledger the tool cannot fully read means the projection rests on records it
// could not parse. That is said, and said first.
func TestStage3_UnsoundLedgerIsReportedBeforeTheProjection(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	writeAmendment(t, featureDir, "002-broken.md", `---
amendment: broken
date: 2026-09-03
affects:
  - "@my-feature/operation:does-not-exist"
---

## Change
Nothing resolvable.

## Acceptance
- Nothing.
`)
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "unresolved error") {
		t.Errorf("an unsound ledger must be reported; got:\n%s", out)
	}
	if strings.Index(out, "unresolved error") > strings.Index(out, "═══ Promises") {
		t.Error("the ledger findings were reported below the projection they undermine")
	}
}

// A retirement recorded in the legacy spelling says a promise ended without
// saying what its author meant by ending it. Presenting that as an ordinary
// retirement would claim knowledge nobody has.
func TestStage3_LegacySupersessionSaysItsMeaningWasNeverRecorded(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	// Built from scratch rather than by rewriting an applied record: swapping
	// an amendment's contents after it was applied leaves bytes that do not
	// match the recorded evidence, which current-state resolution now refuses —
	// correctly, and for reasons that have nothing to do with what this test is
	// about.
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-legacy.md", `---
amendment: legacy
date: 2026-09-02
supersedes_intents:
  - check-readiness
---

## Change
It is over.

## Why
Because.

## Acceptance
- Over.
`)
	writeBaselineApplied(t, "my-feature", 1)
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "no longer makes") {
		t.Errorf("a retired promise must be shown as history; got:\n%s", out)
	}
	if !strings.Contains(out, "never stated") {
		t.Errorf("a legacy record's unknown meaning must be stated, not presented as an "+
			"ordinary retirement; got:\n%s", out)
	}
}

func TestStage3_JSONCarriesTheSameComposite(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := setupRevisedFeature(t, dir)
	armSpecView(t, true)
	var out specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &out); err != nil {
		t.Fatalf("the JSON form must parse: %v", err)
	}
	if out.AppliedThrough != 1 {
		t.Errorf("applied_through = %d, want 1", out.AppliedThrough)
	}
	var found bool
	for _, p := range out.Promises {
		if p.Slug != "check-readiness" {
			continue
		}
		found = true
		if p.Intent.Goal != "A promise that now reads differently." {
			t.Errorf("the JSON form must carry the projected text; got %q", p.Intent.Goal)
		}
		if p.Version != "001-reworded" || p.Mode != "revise" {
			t.Errorf("the JSON form must carry provenance; got %q/%q", p.Version, p.Mode)
		}
		if len(p.Entries) != 1 || p.Entries[0].Ref != "@my-feature/operation:x" {
			t.Errorf("the JSON form must carry attribution; got %+v", p.Entries)
		}
	}
	if !found {
		t.Error("the projected promise is missing from the JSON form")
	}
}

// setupRevisedFeature builds a feature whose promise has been revised and
// applied, which is the state the whole command exists to render.
func setupRevisedFeature(t *testing.T, dir string) (*config.Context, string) {
	t.Helper()
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-reworded.md", `---
amendment: reworded
date: 2026-09-02
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: Check Readiness
      goal: A promise that now reads differently.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
---

## Change
Reworded.

## Why
Clarity.

## Acceptance
- Clearer.
`)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRefineJournal(t, cfg, "my-feature", 1)
	armApplyAmendment(t, "", false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("apply the revision: %v", err)
	}
	return cfg, featureDir
}

// A consumer that iterates the JSON must not have to tell "no promises" from
// "the field was absent". Cheap to get wrong and awkward to discover.
func TestStage3_JSONCollectionsAreNeverNil(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := setupLedgerFeature(t, dir)
	_ = featureDir
	armSpecView(t, true)
	raw := runSpecView_(t, testContext(t), "@my-feature")
	for _, field := range []string{`"promises": []`, `"retired": []`, `"unattributed": []`} {
		if !strings.Contains(raw, field) && !strings.Contains(raw, strings.TrimSuffix(field, " []")+" [") {
			t.Errorf("%s must be an empty array, never null; got:\n%s", field, raw)
		}
	}
	if strings.Contains(raw, "null") {
		t.Errorf("no collection may serialise as null; got:\n%s", raw)
	}
}

// Compaction moves an applied record to amendments/archive/, and
// LoadFeatureAmendments skips subdirectories. Without reading the archive, an
// applied revision that has since been compacted vanishes from the resolution
// and the promise silently reverts to its founding text — a sanctioned
// operation quietly undoing an applied decision, with every consumer of "what
// does this feature promise" showing stale truth and no sign anything is
// missing.
func TestStage3_CompactedRevisionStaysInForce(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)

	before := runSpecView_(t, cfg, "@my-feature")
	beforeRes, err := resolveActiveIntents(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}

	// Compact by hand — the move is what matters, not the command around it.
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(featureDir, "amendments", "001-reworded.md"),
		filepath.Join(archive, "001-reworded.md")); err != nil {
		t.Fatal(err)
	}

	after := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(after, "A promise that now reads differently.") {
		t.Errorf("an applied revision stopped counting once compacted — the promise reverted to "+
			"its founding text:\n%s", after)
	}
	if before != after {
		t.Errorf("compaction changed the projected specification.\nbefore:\n%s\nafter:\n%s",
			before, after)
	}
	afterRes, err := resolveActiveIntents(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeRes.Active) != len(afterRes.Active) {
		t.Fatalf("promise count changed across compaction: %d then %d",
			len(beforeRes.Active), len(afterRes.Active))
	}
	for i := range beforeRes.Active {
		if beforeRes.Active[i].Goal != afterRes.Active[i].Goal {
			t.Errorf("%s reverted across compaction: %q became %q",
				beforeRes.Active[i].Slug, beforeRes.Active[i].Goal, afterRes.Active[i].Goal)
		}
	}
}

// The equivalence compaction advertises must be able to SEE this class of loss.
// A projection comparing only slugs passes unchanged while the promise text
// reverts, which makes it evidence about slugs rather than about promises.
//
// Asserted on the canonical form directly. An earlier version of this test
// renamed an amendment out of the ledger and compared two real projections —
// and passed with the version field removed entirely, because renaming a record
// also moves other fields. It was evidence that SOMETHING changed, which is not
// the claim.
func TestStage3_ProjectionCanonicalFormSeesAPromiseTextChange(t *testing.T) {
	base := func() authorityProjection {
		return authorityProjection{
			ActiveIntents:     []string{"check-readiness"},
			ActiveVersions:    map[string]string{"check-readiness": strings.Repeat("ab", 32)},
			SupersededIntents: map[string]string{},
			SupersededBy:      map[string][]string{},
			Evidence:          map[string]string{},
			Outputless:        map[string]bool{},
		}
	}
	before := base()
	after := base()
	// Same slug, different text. Nothing else moves.
	after.ActiveVersions["check-readiness"] = strings.Repeat("cd", 32)
	if before.canonical() == after.canonical() {
		t.Error("the projection cannot see a promise's text change — its advertised equivalence " +
			"would pass while an applied decision was silently reverted")
	}
}

// And the field is actually populated from the resolution, so the canonical
// form has something real to compare.
func TestStage3_ProjectionRecordsWhatEachPromiseSays(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := setupRevisedFeature(t, dir)
	p, err := computeAuthorityProjection(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range p.ActiveIntents {
		v := p.ActiveVersions[slug]
		parts := strings.Split(v, "|")
		if len(parts) != 3 || parts[0] == "" || !validSHA256(parts[2]) {
			t.Errorf("%q must record deciding record, mode and text fingerprint; got %q", slug, v)
		}
	}
	// And the deciding record is the real one, not a placeholder.
	if got := p.ActiveVersions["check-readiness"]; !strings.HasPrefix(got, "001-reworded|revise|") {
		t.Errorf("the revised promise must record the decision behind it; got %q", got)
	}
	if got := p.ActiveVersions["survives"]; !strings.HasPrefix(got, "founding||") {
		t.Errorf("an untouched promise must record that its text is founding; got %q", got)
	}
	if len(p.ActiveIntents) == 0 {
		t.Fatal("the fixture must carry active promises")
	}
}

// An unreadable archive is not the same failure as an unreadable active ledger,
// and must not get the same treatment. Ignoring the active one keeps promises
// in force, which is conservative; ignoring the archive REVERTS applied
// decisions, which is a silent rollback.
func TestStage3_UnreadableArchiveFailsClosed(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "002-broken.md"),
		[]byte("not an amendment at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveActiveIntents(cfg, "my-feature"); err == nil {
		t.Fatal("an unreadable archive must refuse — ignoring it silently reverts applied " +
			"decisions rather than preserving them")
	}
}

// An entry sourced to ANOTHER feature's promise of the same name belongs to
// that feature. Displaying it under the local promise of the same slug is the
// exact confusion the shared source grammar exists to prevent.
func TestStage3_CrossFeatureAttributionIsNotLocal(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: elsewhere\n    kind: query\n    summary: belongs to another feature\n    source: \"@other-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := runSpecView_(t, cfg, "@my-feature")
	promiseAt := strings.Index(out, "── check-readiness ──")
	unattributedAt := strings.Index(out, "no live promise justifies")
	elsewhereAt := strings.Index(out, "@my-feature/operation:elsewhere")
	if elsewhereAt < 0 {
		t.Fatalf("the entry must appear somewhere; got:\n%s", out)
	}
	if unattributedAt < 0 || elsewhereAt < unattributedAt {
		t.Errorf("an entry sourced to another feature's promise was nested under the local "+
			"promise of the same name; got:\n%s", out)
	}
	// And the local one is still attributed, so the fix did not simply reject
	// everything.
	xAt := strings.Index(out, "@my-feature/operation:x")
	if xAt < promiseAt || (unattributedAt > 0 && xAt > unattributedAt) {
		t.Errorf("the genuinely local entry stopped being attributed; got:\n%s", out)
	}
}

// Every field of a version is current state, and omission under snapshot
// semantics means ABSENT rather than inherited. A JSON form carrying two of ten
// fields answers a different question from the text beside it.
func TestStage3_JSONCarriesTheWholePromise(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	// Give the surviving promise every field, and leave the revised one's lists
	// cleared — which is a real state under snapshot semantics, not an absence.
	intents, err := os.ReadFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		t.Fatal(err)
	}
	enriched := strings.Replace(string(intents), "**Goal**: Something that stays.",
		"**Goal**: Something that stays.\n**Priority**: P2\n**Context**: Always.\n"+
			"**Action**: Keep going.\n\n**Constraints**:\n- Must keep going\n\n"+
			"**Verify**:\n- It kept going\n\n**Questions**:\n- For how long?", 1)
	if enriched == string(intents) {
		t.Fatal("the fixture promise was not enriched")
	}
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(enriched), 0o644); err != nil {
		t.Fatal(err)
	}

	armSpecView(t, true)
	var out specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &out); err != nil {
		t.Fatal(err)
	}
	byslug := map[string]specPromise{}
	for _, p := range out.Promises {
		byslug[p.Slug] = p
	}
	full := byslug["survives"].Intent
	for label, got := range map[string]string{
		"priority": full.Priority, "context": full.Context, "action": full.Action,
	} {
		if got == "" {
			t.Errorf("the JSON form dropped %s, which the text form shows", label)
		}
	}
	for label, got := range map[string][]string{
		"constraints": full.Constraints, "verify": full.Verify, "questions": full.Questions,
	} {
		if len(got) == 0 {
			t.Errorf("the JSON form dropped %s, which the text form shows", label)
		}
	}
	// A cleared list is a real state: the revision supplied no constraints, so
	// the promise has none. It must read as an empty array, not as an absent
	// field a consumer would treat as unknown.
	revised := byslug["check-readiness"].Intent
	if revised.Constraints == nil || revised.Verify == nil {
		t.Error("a cleared list must serialise as [], not null — under snapshot semantics an " +
			"omitted field means the promise no longer carries one, which is knowledge, not " +
			"an absence of it")
	}
	if out.Derivation.Promises == "" || out.Derivation.Contract == "" {
		t.Error("a machine consumer must not have to assume the two halves are equally derived")
	}
}

// Infrastructure fragments are part of the enumerated contract, so rendering
// them as bare addresses in a document meant to be read is a whole kind going
// undescribed — not a missing artifact costing decoration.
func TestStage3_InfrastructureEntriesAreDescribed(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	if err := os.WriteFile(filepath.Join(featureDir, "infrastructure.md"),
		[]byte("# Infrastructure\n\n## Readiness probe boundary\n\n"+
			"source: @my-feature/check-readiness\n\nThe probe runs out of process.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "infrastructure:readiness-probe-boundary") {
		t.Fatalf("the infrastructure entry must appear; got:\n%s", out)
	}
	if !strings.Contains(out, "Readiness probe boundary") {
		t.Errorf("an infrastructure entry rendered as a bare address; got:\n%s", out)
	}
}

// A baseline the tool cannot read means applied position is unknown, and this
// command emits that position as machine-readable data. Encoding 0 would tell a
// consumer that nothing has been applied.
func TestStage3_CorruptBaselineDoesNotReadAsNothingApplied(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := setupRevisedFeature(t, dir)
	if err := os.WriteFile(baselinePath(cfg, "my-feature"),
		[]byte("{{{ not yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := testCommandWithContext(t, cfg)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := runSpecView(cmd, []string{"@my-feature"})
	if err == nil {
		t.Fatal("a corrupt baseline must refuse rather than emit applied_through: 0 as fact")
	}
	if !strings.Contains(err.Error(), "applied authority") {
		t.Errorf("the refusal must name what could not be established; got: %v", err)
	}
}

// Location is not trust. Feeding every well-formed file in amendments/archive/
// to the resolver let a hand-written record become current truth by choosing a
// sequence at or below the marker — no capsule hash, no ceremony, no evidence
// of any kind, just a plausible filename in a directory the resolver had
// started reading.
func TestStage3_ArchivedRecordWithNoEvidenceIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	// Precondition: the genuinely applied record resolves.
	if _, err := resolveActiveIntents(cfg, "my-feature"); err != nil {
		t.Fatalf("the honest fixture must resolve: %v", err)
	}

	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	// A well-formed evolution record at a sequence the marker covers, whose
	// filename the capsule has never heard of.
	if err := os.WriteFile(filepath.Join(archive, "001-forged.md"), []byte(`---
amendment: forged
date: 2020-01-01
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: Forged
      goal: Whatever the forger wanted it to say.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
---

## Change
Forged.

## Why
Because nobody checked.

## Acceptance
- Forged.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := resolveActiveIntents(cfg, "my-feature")
	if err == nil {
		for _, in := range res.Active {
			if in.Goal == "Whatever the forger wanted it to say." {
				t.Fatal("a forged archived record became current truth")
			}
		}
		t.Fatal("an applied-range record with no recorded evidence must refuse — ignoring it " +
			"answers with the text that preceded it, which is a silent rollback")
	}
	if !strings.Contains(err.Error(), "no evidence") {
		t.Errorf("the refusal must say what is missing; got: %v", err)
	}
}

// The positive case, so the rule is not simply "refuse archived records": a
// compacted record whose hash the capsule holds is trusted history and counts.
func TestStage3_TrustedCompactedRecordIsAccepted(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(featureDir, "amendments", "001-reworded.md"),
		filepath.Join(archive, "001-reworded.md")); err != nil {
		t.Fatal(err)
	}
	res, err := resolveActiveIntents(cfg, "my-feature")
	if err != nil {
		t.Fatalf("a trusted compacted record must still count: %v", err)
	}
	var found bool
	for _, in := range res.Active {
		if in.Slug == "check-readiness" && in.Goal == "A promise that now reads differently." {
			found = true
		}
	}
	if !found {
		t.Error("the compacted revision stopped being in force")
	}
}

// Active bytes win, so altering the active copy destroys trust rather than
// letting a pristine archived copy vouch for it.
func TestStage3_AlteredActiveCopyIsNotVouchedForByTheArchive(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	active := filepath.Join(featureDir, "amendments", "001-reworded.md")
	body, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(featureDir, "amendments", "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	// A pristine archived copy beside an altered active one.
	if err := os.WriteFile(filepath.Join(archive, "001-reworded.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(active, append(body, []byte("\nQuietly appended.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveActiveIntents(cfg, "my-feature"); err == nil {
		t.Fatal("an altered active record must destroy trust — a pristine archived copy of the " +
			"same filename must not vouch for it")
	} else if !strings.Contains(err.Error(), "not the ones that were applied") {
		t.Errorf("the refusal must name the mismatch; got: %v", err)
	}
}

// An applied record whose bytes are gone from both locations is erased history.
// It must not read as a decision that never happened.
func TestStage3_ErasedAppliedRecordIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-reworded.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveActiveIntents(cfg, "my-feature"); err == nil {
		t.Fatal("an erased applied record must refuse, not silently revert the promise")
	} else if !strings.Contains(err.Error(), "no such record exists") {
		t.Errorf("the refusal must say the record is gone; got: %v", err)
	}
}

// The pending tail owes no applied evidence — it has not been applied. A rule
// that demanded it would make every ordinary unapplied refinement unresolvable.
func TestStage3_PendingTailNeedsNoAppliedEvidence(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	writeAmendment(t, featureDir, "002-later.md", `---
amendment: later
date: 2026-09-03
affects:
  - "@my-feature/operation:x"
---

## Change
Not applied yet.

## Acceptance
- Later.
`)
	if _, err := resolveActiveIntents(cfg, "my-feature"); err != nil {
		t.Fatalf("an ordinary pending record must not be treated as untrusted applied "+
			"history: %v", err)
	}
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "002-later") {
		t.Errorf("the pending record must still be reportable; got:\n%s", out)
	}
}

// Compaction must not make provenance lie. `parlay spec` names the decision
// behind each promise, so an equivalence check that protects the text but not
// the deciding record leaves that claim unguarded.
func TestStage3_ProjectionSeesADifferentDecidingRecord(t *testing.T) {
	base := func() authorityProjection {
		return authorityProjection{
			ActiveIntents: []string{"check-readiness"},
			ActiveVersions: map[string]string{
				"check-readiness": "001-first|revise|" + strings.Repeat("ab", 32)},
			SupersededIntents: map[string]string{},
			SupersededBy:      map[string][]string{},
			Evidence:          map[string]string{},
			Outputless:        map[string]bool{},
		}
	}
	before := base()
	after := base()
	// Same text, different decision behind it.
	after.ActiveVersions["check-readiness"] = "002-second|revise|" + strings.Repeat("ab", 32)
	if before.canonical() == after.canonical() {
		t.Error("the projection cannot see which decision put the text there, so compaction " +
			"could change the provenance parlay spec reports and pass its own equivalence check")
	}
	third := base()
	third.ActiveVersions["check-readiness"] = "001-first|narrow|" + strings.Repeat("ab", 32)
	if before.canonical() == third.canonical() {
		t.Error("the projection cannot see the mode a promise was changed under")
	}
}

// --- The bytes that are authenticated are the bytes that are interpreted ---

// Parsing a path and later hashing that path reads the file TWICE, and the gap
// between the two reads is where a swap goes: forged bytes get parsed, genuine
// bytes get hashed, and the forgery comes out authenticated. The hook below
// fires at exactly that instant.
//
// Deterministic, not a timing test: the hook runs synchronously between the one
// read and everything that follows it.
func TestStage3_SwapAfterTheReadCannotLaunderAForgery(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	path := filepath.Join(featureDir, "amendments", "001-reworded.md")
	genuine, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	forged := []byte(strings.Replace(string(genuine),
		"A promise that now reads differently.", "Whatever the forger wanted.", 1))
	if string(forged) == string(genuine) {
		t.Fatal("fixture: the forgery changed nothing")
	}

	// The unsafe direction. Forged bytes are on disk when the record is read;
	// the genuine bytes are restored immediately afterwards, which under the
	// old two-read shape would have supplied a matching hash for content
	// nobody applied.
	if err := os.WriteFile(path, forged, 0o644); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	appliedLedgerReadHook = func(name string) {
		if name != "001-reworded.md" {
			return
		}
		once.Do(func() { _ = os.WriteFile(path, genuine, 0o644) })
	}
	t.Cleanup(func() { appliedLedgerReadHook = nil })

	res, err := resolveActiveIntents(cfg, "my-feature")
	if err == nil {
		for _, in := range res.Active {
			if in.Goal == "Whatever the forger wanted." {
				t.Fatal("forged content was parsed and genuine content was hashed — the swap " +
					"laundered a record nobody applied")
			}
		}
		t.Fatal("the forged bytes were the ones read, so authentication must refuse them")
	}
	if !strings.Contains(err.Error(), "not the ones that were applied") {
		t.Errorf("the refusal must name the mismatch; got: %v", err)
	}
}

// The other direction of the same property: a mutation AFTER the read cannot
// change what this snapshot says, because the snapshot is the bytes it read.
func TestStage3_MutationAfterTheReadDoesNotAffectTheSnapshot(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	path := filepath.Join(featureDir, "amendments", "001-reworded.md")
	genuine, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	appliedLedgerReadHook = func(name string) {
		if name != "001-reworded.md" {
			return
		}
		once.Do(func() {
			_ = os.WriteFile(path, []byte(strings.Replace(string(genuine),
				"A promise that now reads differently.", "Written after the read.", 1)), 0o644)
		})
	}
	t.Cleanup(func() { appliedLedgerReadHook = nil })

	res, err := resolveActiveIntents(cfg, "my-feature")
	if err != nil {
		t.Fatalf("the bytes that were read are genuine, so this must resolve: %v", err)
	}
	for _, in := range res.Active {
		if in.Goal == "Written after the read." {
			t.Error("a write after the read reached the snapshot — the snapshot is not the " +
				"bytes it authenticated")
		}
	}
}

// Authority moving mid-acquisition would produce a view that is part of one
// state and part of another. One coherent old view, one coherent new view, or
// a refusal — never a mixture.
func TestStage3_AuthorityMovingMidAcquisitionRefuses(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	writeAmendment(t, featureDir, "002-later.md", `---
amendment: later
date: 2026-09-03
affects:
  - "@my-feature/operation:x"
---

## Change
Applied by somebody else, mid-read.

## Acceptance
- Later.
`)
	// Precondition: with authority still, this resolves.
	if _, err := resolveActiveIntents(cfg, "my-feature"); err != nil {
		t.Fatalf("the fixture must resolve while authority is still: %v", err)
	}

	var once sync.Once
	appliedLedgerCapsuleHook = func(slug string) {
		once.Do(func() {
			bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
			h, ok := hashWholeFile(filepath.Join(featureDir, "amendments", "002-later.md"))
			if !ok {
				t.Error("hash the newly applied record")
				return
			}
			bl.LastAppliedAmendment = 2
			bl.Sources.Amendments["002-later.md"] = h
			data, merr := marshalBaseline(&bl)
			if merr != nil {
				t.Error(merr)
				return
			}
			_ = os.WriteFile(baselinePath(cfg, "my-feature"), data, 0o644)
		})
	}
	t.Cleanup(func() { appliedLedgerCapsuleHook = nil })

	_, err := resolveActiveIntents(cfg, "my-feature")
	if err == nil {
		t.Fatal("authority advancing mid-acquisition must refuse rather than return a view " +
			"assembled from two states")
	}
	if !strings.Contains(err.Error(), "changed while its ledger was being read") {
		t.Errorf("the refusal must say why; got: %v", err)
	}

	// And the new state, read cleanly, is coherent.
	appliedLedgerCapsuleHook = nil
	if _, err := resolveActiveIntents(cfg, "my-feature"); err != nil {
		t.Errorf("the advanced state must resolve on its own: %v", err)
	}
}

// The whole view comes from one acquisition, so the marker it displays and the
// promises it renders cannot disagree.
func TestStage3_SpecViewRendersOneCoherentState(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	writeAmendment(t, featureDir, "002-later.md", `---
amendment: later
date: 2026-09-03
affects:
  - "@my-feature/operation:x"
---

## Change
Applied mid-render.

## Acceptance
- Later.
`)
	var once sync.Once
	appliedLedgerCapsuleHook = func(string) {
		once.Do(func() {
			bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
			h, _ := hashWholeFile(filepath.Join(featureDir, "amendments", "002-later.md"))
			bl.LastAppliedAmendment = 2
			bl.Sources.Amendments["002-later.md"] = h
			data, _ := marshalBaseline(&bl)
			_ = os.WriteFile(baselinePath(cfg, "my-feature"), data, 0o644)
		})
	}
	t.Cleanup(func() { appliedLedgerCapsuleHook = nil })

	cmd := testCommandWithContext(t, cfg)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runSpecView(cmd, []string{"@my-feature"}); err == nil {
		t.Fatal("a specification assembled from two authority states must refuse")
	}
	if strings.Contains(buf.String(), "Applied through") {
		t.Error("the view printed a marker for a state it could not confirm")
	}
}
