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
		if p.Entries == nil || len(*p.Entries) != 1 ||
			(*p.Entries)[0].Ref != "@my-feature/operation:x" {
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
	// In a CURRENT view. Null is reserved for "no contract to consult", which is
	// a historical view's answer and a different fact from an empty one.
	if strings.Contains(raw, "null") {
		t.Errorf("a current view must not serialise a collection as null — null means the "+
			"contract is unavailable, not that it is empty; got:\n%s", raw)
	}
	if !strings.Contains(raw, `"contract_status": "available"`) {
		t.Errorf("a current view must say its contract is available; got:\n%s", raw)
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

// --- Stage 4: --at <amendment> ---

// setupTwoRevisions applies two revisions so there is an earlier state worth
// asking about.
func setupTwoRevisions(t *testing.T) (*config.Context, string) {
	t.Helper()
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	writeAmendment(t, featureDir, "002-again.md", `---
amendment: again
date: 2026-09-03
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: Check Readiness
      goal: A promise that reads differently AGAIN.
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
	writeRefineJournal(t, cfg, "my-feature", 2)
	armApplyAmendment(t, "", false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("apply the second revision: %v", err)
	}
	return cfg, featureDir
}

func armSpecViewAt(t *testing.T, at string) {
	t.Helper()
	specViewAt = at
	t.Cleanup(func() { specViewAt = "" })
}

// Versions are snapshots rather than patches, which is the only reason an
// earlier state is answerable at all.
func TestStage4_AtShowsThePromiseAsItStoodThen(t *testing.T) {
	cfg, _ := setupTwoRevisions(t)

	now := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(now, "A promise that reads differently AGAIN.") {
		t.Fatalf("the current view must show the latest text; got:\n%s", now)
	}

	armSpecViewAt(t, "1")
	then := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(then, "A promise that now reads differently.") {
		t.Errorf("--at 1 must show the text in force at that point; got:\n%s", then)
	}
	if strings.Contains(then, "A promise that reads differently AGAIN.") {
		t.Error("--at 1 showed a decision made after the point being asked about")
	}
	// Provenance has to move with the text. Naming a later record beside earlier
	// text is worse than either error alone: each half looks right, and the
	// reader has no way to see they disagree.
	if !strings.Contains(then, "current text from 001-reworded") {
		t.Errorf("--at 1 must attribute the text to the decision in force then; got:\n%s", then)
	}
	if strings.Contains(then, "002-again (revise)") {
		t.Error("--at 1 attributed its text to a decision made afterwards")
	}
	// It must announce itself as history in the first line, because a reader who
	// skims the body will act on it either way.
	if !strings.Contains(then, "AS IT STOOD") || !strings.Contains(then, "This is history") {
		t.Errorf("a historical view must say so up front; got:\n%s", then)
	}
	// And say what has happened since, or a reader cannot tell whether it holds.
	if !strings.Contains(then, "Decided since") || !strings.Contains(then, "002-again") {
		t.Errorf("the view must name what was decided after the point shown; got:\n%s", then)
	}
}

// --at 0 is the founding state, which is a real and useful question: what did
// we promise before any of this.
func TestStage4_AtZeroIsTheFoundingState(t *testing.T) {
	cfg, _ := setupTwoRevisions(t)
	armSpecViewAt(t, "0")
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "See if the cluster is ready.") {
		t.Errorf("--at 0 must show the founding text; got:\n%s", out)
	}
	if !strings.Contains(out, "current text from the founding document") {
		t.Errorf("--at 0 must attribute the text to the founding document; got:\n%s", out)
	}
}

// The contract half has no earlier version, and saying so is the whole point.
// Rendering today's entries under yesterday's promises would attribute present
// facts to a past state — the more tempting mistake, because the output would
// look complete.
func TestStage4_HistoricalViewOmitsTheContractAndSaysWhy(t *testing.T) {
	cfg, _ := setupTwoRevisions(t)
	armSpecViewAt(t, "1")
	out := runSpecView_(t, cfg, "@my-feature")
	if strings.Contains(out, "@my-feature/operation:x") {
		t.Error("a historical view rendered today's contract entries, dating something that " +
			"has no date")
	}
	if !strings.Contains(out, "not omitted for brevity") {
		t.Errorf("the omission must be explained, or it reads as an empty contract; got:\n%s", out)
	}
}

// A sequence beyond the marker is not an earlier state, it is a proposal — the
// apply ceremony's question, and it comes with an approval attached. A
// read-only view answering it in the same breath as "what was true" makes the
// two indistinguishable in the reader's head.
func TestStage4_AtBeyondTheMarkerIsRefused(t *testing.T) {
	cfg, featureDir := setupTwoRevisions(t)
	writeAmendment(t, featureDir, "003-later.md", `---
amendment: later
date: 2026-09-04
affects:
  - "@my-feature/operation:x"
---

## Change
Not applied.

## Acceptance
- Later.
`)
	armSpecViewAt(t, "3")
	cmd := testCommandWithContext(t, cfg)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := runSpecView(cmd, []string{"@my-feature"})
	if err == nil {
		t.Fatal("--at beyond the applied marker must refuse")
	}
	if !strings.Contains(err.Error(), "it is a proposal") {
		t.Errorf("the refusal must say why it is not history; got: %v", err)
	}
}

func TestStage4_AtAcceptsAnIdentityAndRejectsNonsense(t *testing.T) {
	cfg, _ := setupTwoRevisions(t)

	armSpecViewAt(t, "001-reworded")
	if out := runSpecView_(t, cfg, "@my-feature"); !strings.Contains(out,
		"A promise that now reads differently.") {
		t.Errorf("--at must accept an amendment identity; got:\n%s", out)
	}

	specViewAt = "no-such-record"
	cmd := testCommandWithContext(t, cfg)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := runSpecView(cmd, []string{"@my-feature"}); err == nil {
		t.Fatal("--at naming no amendment must refuse")
	} else if !strings.Contains(err.Error(), "names no amendment") {
		t.Errorf("the refusal must say what is wrong; got: %v", err)
	}
}

// The historical view is still one coherent state, and the machine form says
// which point it is and what came after.
func TestStage4_JSONCarriesThePointAndWhatCameAfter(t *testing.T) {
	cfg, _ := setupTwoRevisions(t)
	armSpecViewAt(t, "1")
	armSpecView(t, true)
	var out specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &out); err != nil {
		t.Fatal(err)
	}
	if out.At != 1 || out.AppliedThrough != 2 {
		t.Errorf("at=%d applied_through=%d, want 1 and 2", out.At, out.AppliedThrough)
	}
	if len(out.Since) == 0 {
		t.Error("the machine form must say what was decided after the point shown")
	}
	if out.Unattributed != nil {
		t.Error("a historical view must leave the contract UNAVAILABLE, not assert it is empty " +
			"— a machine consumer reads the structure, not the prose beside it")
	}
	if out.ContractStatus != contractUnavailableHistorical {
		t.Errorf("a historical view must not claim its contract is available; got %q",
			out.ContractStatus)
	}
	for _, p := range out.Promises {
		if p.Entries != nil {
			t.Errorf("%s asserted a contract population in a view that has none", p.Slug)
		}
	}
	if !strings.Contains(out.Derivation.Contract, "omitted") {
		t.Errorf("the JSON must say the contract half is omitted and why; got %q",
			out.Derivation.Contract)
	}
}

// "Known to provide nothing" and "there is no contract to consult" are
// different facts, and they must be distinguishable in the structure rather
// than only in prose beside it. A machine consumer reads the structure.
func TestStage4_KnownEmptyAndUnavailableAreDistinguishable(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	// A feature with a genuinely empty contract: the artifact is gone, so
	// nothing is attributed and that is KNOWN.
	if err := os.Remove(filepath.Join(featureDir, "capabilities.yaml")); err != nil {
		t.Fatal(err)
	}

	armSpecView(t, true)
	var current specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &current); err != nil {
		t.Fatal(err)
	}
	if current.ContractStatus != contractAvailable {
		t.Fatalf("a current view over an empty contract still HAS a contract to report; got %q",
			current.ContractStatus)
	}
	if current.Unattributed == nil {
		t.Error("a known-empty contract must encode as an empty array, not as unavailable")
	}
	for _, p := range current.Promises {
		if p.Entries == nil {
			t.Errorf("%s must report a known-empty population, not an absent one", p.Slug)
		} else if len(*p.Entries) != 0 {
			t.Errorf("%s should justify nothing here; got %+v", p.Slug, *p.Entries)
		}
	}

	// The same feature asked about historically: unavailable, not empty.
	armSpecViewAt(t, "0")
	var past specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &past); err != nil {
		t.Fatal(err)
	}
	if past.ContractStatus != contractUnavailableHistorical || past.Unattributed != nil {
		t.Errorf("a historical view must encode the contract as unavailable for a stated "+
			"reason; got %q", past.ContractStatus)
	}
}

// And in prose: a historical view must not print the sentence that says a
// promise is known to justify nothing, since it goes on to say it cannot know.
// The reader meets the false claim first.
func TestStage4_HistoricalProseMakesNoEmptyContractClaim(t *testing.T) {
	cfg, _ := setupTwoRevisions(t)
	armSpecViewAt(t, "1")
	out := runSpecView_(t, cfg, "@my-feature")
	if strings.Contains(out, "nothing in the contract names this promise") {
		t.Errorf("a historical view claimed a promise justifies nothing, then said it could not "+
			"know; got:\n%s", out)
	}
	if strings.Contains(out, "provides:") {
		t.Errorf("a historical view rendered a provides block for a contract it has not read; "+
			"got:\n%s", out)
	}
	if !strings.Contains(out, "not omitted for brevity") {
		t.Errorf("the single omission section must still be there; got:\n%s", out)
	}
}

// A sequence must IDENTIFY a record. With records 1 and 3, `--at 2` marks no
// boundary any amendment created, and the command's own wording — "when this
// amendment was the last applied one" — names an amendment that does not exist.
func TestStage4_AtMustNameARealRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	// Applied through 1; give the ledger a gap by never creating 002.
	writeAmendment(t, featureDir, "003-later.md", `---
amendment: later
date: 2026-09-04
affects:
  - "@my-feature/operation:x"
---

## Change
Unapplied.

## Acceptance
- Later.
`)
	for _, arg := range []string{"2", "9"} {
		specViewAt = arg
		cmd := testCommandWithContext(t, cfg)
		var buf strings.Builder
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		err := runSpecView(cmd, []string{"@my-feature"})
		if err == nil {
			t.Fatalf("--at %s identifies no amendment and must refuse", arg)
		}
		if !strings.Contains(err.Error(), "names no amendment") {
			t.Errorf("--at %s: the refusal must say the record does not exist; got: %v", arg, err)
		}
	}
	specViewAt = ""
}

// Ambiguity is refused rather than resolved by sort order: "the first one" is
// not an answer anybody asked for.
func TestStage4_AmbiguousAtIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	// The same slug at a different sequence, so the bare form matches twice.
	writeAmendment(t, featureDir, "002-reworded.md", `---
amendment: reworded
date: 2026-09-03
affects:
  - "@my-feature/operation:x"
---

## Change
Same slug, later.

## Acceptance
- Done.
`)
	specViewAt = "reworded"
	t.Cleanup(func() { specViewAt = "" })
	cmd := testCommandWithContext(t, cfg)
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := runSpecView(cmd, []string{"@my-feature"})
	if err == nil {
		t.Fatal("a bare slug matching two records must refuse")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("the refusal must say it is ambiguous; got: %v", err)
	}
	// The exact identity still works. This fixture is applied through 001, so
	// asking for that point asks for the present — and the present is rendered
	// as the present. Labelling it history because a flag was passed would put
	// a date on the current state.
	specViewAt = "001-reworded"
	out := runSpecView_(t, cfg, "@my-feature")
	if !strings.Contains(out, "current specification") {
		t.Errorf("--at naming the applied point IS the current view; got:\n%s", out)
	}
	if !strings.Contains(out, "A promise that now reads differently.") {
		t.Errorf("the exact identity must still resolve; got:\n%s", out)
	}
}

// The authenticated snapshot is immutable. A view over it says which point is
// being asked about; it does not rewrite what was authenticated.
func TestStage4_ViewDoesNotRewriteTheAuthenticatedSnapshot(t *testing.T) {
	cfg, _ := setupTwoRevisions(t)
	snap, err := acquireAppliedLedger(cfg, "my-feature", cfg.FeaturePath("my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Through != snap.Capsule.Through {
		t.Fatalf("the snapshot's own markers already disagree: %d and %d",
			snap.Through, snap.Capsule.Through)
	}
	v := snap.viewAt(1)
	if v.Through != 1 {
		t.Errorf("the view must ask about the requested point; got %d", v.Through)
	}
	if v.Snapshot.Through != snap.Through || v.Snapshot.Capsule.Through != snap.Capsule.Through {
		t.Error("the view rewrote the authenticated snapshot — an object that looks like " +
			"authority while carrying a marker for a state that never existed")
	}
	if snap.Through != snap.Capsule.Through {
		t.Error("taking a view mutated the snapshot it was taken from")
	}
}

// An artifact that EXISTS and cannot be parsed is neither empty nor historical.
// The earlier code recorded the failure in the banner and then encoded
// known-empty arrays anyway — the same prose-cannot-undo-structure defect one
// branch over from the historical one, and this time the two claims were in the
// same view.
func TestStage4_UnreadableContractIsNotEncodedAsEmpty(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupRevisedFeature(t, dir)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	armSpecView(t, true)
	var out specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &out); err != nil {
		t.Fatal(err)
	}
	if out.ContractStatus != contractUnreadable {
		t.Errorf("an unparseable artifact must read as unreadable, not %q", out.ContractStatus)
	}
	if out.Unattributed != nil {
		t.Error("an unreadable contract must not encode as a known-empty one")
	}
	for _, p := range out.Promises {
		if p.Entries != nil {
			t.Errorf("%s asserted a population from a contract nobody could read", p.Slug)
		}
	}
	if len(out.Blocking) == 0 {
		t.Error("the reason must still be reported")
	}

	// And the prose must not claim it either.
	specViewJSON = false
	prose := runSpecView_(t, cfg, "@my-feature")
	if strings.Contains(prose, "nothing in the contract names this promise") {
		t.Errorf("the view claimed a promise justifies nothing from a contract it could not "+
			"read; got:\n%s", prose)
	}
	if !strings.Contains(prose, "cannot be read") {
		t.Errorf("the reason must be the only claim made about the missing population; got:\n%s",
			prose)
	}
	// The historical omission section belongs to history, not to this.
	if strings.Contains(prose, "not omitted for brevity") {
		t.Error("an unreadable current contract was explained as a historical omission")
	}
}

// nil is the sentinel for "no contract to consult". A retired promise must not
// carry it while the same view says globally that the contract IS available.
func TestStage4_RetiredPromisesUseTheSameSentinelAsEverythingElse(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
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

	armSpecView(t, true)
	var out specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &out); err != nil {
		t.Fatal(err)
	}
	if out.ContractStatus != contractAvailable {
		t.Fatalf("the fixture must have a readable contract; got %q", out.ContractStatus)
	}
	if len(out.Retired) == 0 {
		t.Fatal("the fixture must carry a retired promise")
	}
	for _, p := range out.Retired {
		if p.Entries == nil {
			t.Errorf("%s used the unavailable sentinel while the view says the contract IS "+
				"available — a retired promise justifies nothing, which is an empty array, not "+
				"an absent one", p.Slug)
		} else if len(*p.Entries) != 0 {
			t.Errorf("%s is retired and must justify nothing; got %+v", p.Slug, *p.Entries)
		}
	}

	// And under a historical view the sentinel IS correct for them. A second
	// applied record is needed for the point to be genuinely in the past while
	// the retirement has already happened — at 0 nothing is retired yet, so the
	// retired list would be empty and the assertion would check nothing.
	writeAmendment(t, featureDir, "002-later.md", `---
amendment: later
date: 2026-09-03
affects:
  - "@my-feature/operation:x"
---

## Change
After the retirement.

## Acceptance
- Later.
`)
	writeBaselineApplied(t, "my-feature", 2)

	armSpecViewAt(t, "1")
	var past specViewOutput
	if err := json.Unmarshal([]byte(runSpecView_(t, cfg, "@my-feature")), &past); err != nil {
		t.Fatal(err)
	}
	if len(past.Retired) == 0 {
		t.Fatal("the historical point must still show the promise as retired, or this asserts " +
			"nothing")
	}
	for _, p := range past.Retired {
		if p.Entries != nil {
			t.Errorf("%s asserted a population in a view with no contract", p.Slug)
		}
	}
}
