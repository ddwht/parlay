// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: amendment-ledger-check
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAmendment writes one ledger file into a feature dir.
func writeAmendment(t *testing.T, featDir, name, content string) {
	t.Helper()
	amDir := filepath.Join(featDir, "amendments")
	if err := os.MkdirAll(amDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(amDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCheckAmendments_(t *testing.T, featureRef string) (checkAmendmentsOutput, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	runErr := runCheckAmendments(cmd, []string{featureRef})
	var out checkAmendmentsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	return out, runErr
}

func TestCheckAmendments_HealthyLedgerEmitsDirtySet(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir) // verify-fixture: op thing.create, fragments Create Form / Thing List

	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-13
trigger: "duplicate names slipped through"
affects:
  - "@verify-fixture/operation:thing.create"
  - "@verify-fixture/surface:thing-list"
---

## Change
Creation rejects duplicate names case-insensitively.

## Why
Two Things differing only by case are the same thing.

## Acceptance
- Creating "Alpha" when "alpha" exists is rejected with conflict.
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("healthy ledger should exit zero: %v (issues: %+v)", err, out.Issues)
	}
	if !out.Ready {
		t.Errorf("expected ready=true; issues: %+v", out.Issues)
	}
	if len(out.DirtySet) != 2 {
		t.Errorf("expected 2 refs in dirty set, got %v", out.DirtySet)
	}
	if len(out.Amendments) != 1 || out.Amendments[0].Slug != "tighten-create" {
		t.Errorf("amendment listing wrong: %+v", out.Amendments)
	}
}

func TestCheckAmendments_UnresolvedAffectsIsError(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	writeAmendment(t, featDir, "001-ghost.md", `---
amendment: ghost
date: 2026-08-13
affects:
  - "@verify-fixture/operation:does.not.exist"
---

## Change
X.

## Acceptance
- Y.
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err == nil {
		t.Fatal("unresolved affects ref should exit non-zero")
	}
	if !hasIssueCode(out, "amendment-affects-unresolved") {
		t.Errorf("expected amendment-affects-unresolved; got %+v", out.Issues)
	}
	if len(out.DirtySet) != 0 {
		t.Errorf("unresolved ref must not enter the dirty set; got %v", out.DirtySet)
	}
}

func TestCheckAmendments_LedgerIntegrityFindings(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	// 001: frontmatter slug disagrees with filename.
	writeAmendment(t, featDir, "001-real-name.md", `---
amendment: other-name
date: 2026-08-13
affects: ["@verify-fixture/operation:thing.create"]
---

## Change
X.

## Acceptance
- Y.
`)
	// 003: gap after 001 (002 missing), supersedes something unknown.
	writeAmendment(t, featDir, "003-later.md", `---
amendment: later
date: 2026-08-14
affects: ["@verify-fixture/operation:thing.create"]
supersedes: [never-existed]
---

## Change
X.

## Acceptance
- Y.
`)
	// A stray file invisible to the ledger.
	writeAmendment(t, featDir, "07-badly-numbered.md", "whatever")

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err == nil {
		t.Fatal("integrity problems should exit non-zero")
	}
	for _, code := range []string{"amendment-slug-mismatch", "amendment-supersedes-unknown", "amendment-out-of-sequence"} {
		if !hasIssueCode(out, code) {
			t.Errorf("expected %s; got %+v", code, out.Issues)
		}
	}
	if !hasIssueWith(out, "amendment-sequence-gap", "001 -> 003") {
		t.Errorf("expected a sequence-gap warning naming 001 -> 003; got %+v", out.Issues)
	}
}

func TestCheckAmendments_EmptyLedgerIsHealthy(t *testing.T) {
	dir := setupTestDir(t)
	writeVerifyFixture(t, dir)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("empty ledger should be healthy: %v", err)
	}
	if !out.Ready || len(out.Amendments) != 0 || len(out.DirtySet) != 0 {
		t.Errorf("empty ledger should be ready with nothing to report: %+v", out)
	}
}

// TestCheckAmendments_DirtySetScopesToUnappliedTail is the L7 regression.
// dirty_set was the cumulative union of every amendment's affects, so a ref
// touched by a long-applied amendment stayed "dirty" forever and never agreed
// with what `parlay internal diff` infers from hashes. It must now name only
// the unapplied tail — amendments beyond the baseline's last-applied-amendment
// — while the full union lives under all_affects.
func TestCheckAmendments_DirtySetScopesToUnappliedTail(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	// 001 (applied) touches the operation; 002 (unapplied tail) touches the
	// surface fragment. Two distinct resolvable refs so the sets are legible.
	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-13
trigger: "duplicate names slipped through"
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
Creation rejects duplicate names.

## Acceptance
- Creating "Alpha" when "alpha" exists is rejected with conflict.
`)
	writeAmendment(t, featDir, "002-relabel-list.md", `---
amendment: relabel-list
date: 2026-08-14
trigger: "the list heading was ambiguous"
affects:
  - "@verify-fixture/surface:thing-list"
---

## Change
Relabel the list heading.

## Acceptance
- The heading reads "Your Things".
`)

	// Baseline records 001 as applied, 002 as not.
	blPath := baselinePath(testContext(t), "verify-fixture")
	if err := os.MkdirAll(filepath.Dir(blPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blPath, []byte("schema-version: 3\ngenerated-at: 2026-08-13T00:00:00Z\nlast-applied-amendment: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("healthy ledger should exit zero: %v (issues: %+v)", err, out.Issues)
	}

	// dirty_set: only the unapplied tail's ref.
	if len(out.DirtySet) != 1 || out.DirtySet[0] != "@verify-fixture/surface:thing-list" {
		t.Errorf("dirty_set must be the unapplied tail only ([surface:thing-list]); got %v", out.DirtySet)
	}
	// all_affects: the whole ledger footprint.
	if len(out.AllAffects) != 2 {
		t.Errorf("all_affects must be the full union of both amendments; got %v", out.AllAffects)
	}
}

// TestCheckAmendments_NoBaselineTreatsAllAsUnapplied pins the conservative
// fallback: with no baseline (never built / pre-v3), last-applied is 0 so every
// amendment is unapplied and dirty_set equals all_affects.
func TestCheckAmendments_NoBaselineTreatsAllAsUnapplied(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-13
trigger: "x"
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
c

## Acceptance
- a
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("healthy ledger should exit zero: %v", err)
	}
	if len(out.DirtySet) != 1 || len(out.AllAffects) != 1 {
		t.Errorf("with no baseline, dirty_set==all_affects==[operation:thing.create]; got dirty=%v all=%v", out.DirtySet, out.AllAffects)
	}
}

// TestCheckAmendments_ScopeOverlapWarnsWhenNotSuperseded is the L15/F18
// regression: a later amendment editing a contract entry an earlier one also
// edits, without naming the earlier in supersedes:, is two unordered writers on
// the same entry and must warn.
func TestCheckAmendments_ScopeOverlapWarnsWhenNotSuperseded(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-13
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
Creation rejects duplicates.

## Acceptance
- dup rejected.
`)
	writeAmendment(t, featDir, "002-loosen-create.md", `---
amendment: loosen-create
date: 2026-08-14
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
Creation permits duplicates after all.

## Acceptance
- dup allowed.
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("scope overlap is a warning, not an error: %v (issues %+v)", err, out.Issues)
	}
	if !hasIssueCode(out, "amendment-scope-overlap") {
		t.Errorf("expected amendment-scope-overlap; got %+v", out.Issues)
	}
	if !out.Ready {
		t.Errorf("a warning must not flip ready to false: %+v", out.Issues)
	}
}

// TestCheckAmendments_ScopeOverlapSilentWhenSuperseded confirms naming the
// earlier amendment in supersedes: — the declaration that this change replaces
// it — silences the overlap warning, and that the forward link appears in
// superseded_by.
func TestCheckAmendments_ScopeOverlapSilentWhenSuperseded(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-13
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
Creation rejects duplicates.

## Acceptance
- dup rejected.
`)
	writeAmendment(t, featDir, "002-loosen-create.md", `---
amendment: loosen-create
date: 2026-08-14
affects:
  - "@verify-fixture/operation:thing.create"
supersedes: [tighten-create]
---

## Change
Creation permits duplicates after all.

## Acceptance
- dup allowed.
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("healthy superseding ledger should exit zero: %v (issues %+v)", err, out.Issues)
	}
	if hasIssueCode(out, "amendment-scope-overlap") {
		t.Errorf("an overlap the later amendment supersedes must be silent; got %+v", out.Issues)
	}
	if got := out.SupersededBy["tighten-create"]; len(got) != 1 || got[0] != "loosen-create" {
		t.Errorf("superseded_by should record the forward link tighten-create -> loosen-create; got %v", out.SupersededBy)
	}
}

// TestCheckAmendments_DisjointScopesDoNotOverlap confirms two amendments
// touching different contract entries do not warn.
func TestCheckAmendments_DisjointScopesDoNotOverlap(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-13
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
c

## Acceptance
- a
`)
	writeAmendment(t, featDir, "002-relabel-list.md", `---
amendment: relabel-list
date: 2026-08-14
affects:
  - "@verify-fixture/surface:thing-list"
---

## Change
c

## Acceptance
- a
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("disjoint ledger should exit zero: %v", err)
	}
	if hasIssueCode(out, "amendment-scope-overlap") {
		t.Errorf("disjoint affects must not overlap; got %+v", out.Issues)
	}
}

func hasIssueCode(out checkAmendmentsOutput, code string) bool {
	for _, i := range out.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func hasIssueWith(out checkAmendmentsOutput, code, substr string) bool {
	for _, i := range out.Issues {
		if i.Code == code && strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

// --- intent supersession -------------------------------------------------
//
// These run through runCheckAmendments — the only production caller of the
// amendment validators — rather than the leaf functions. A rule proven only
// against a hand-built input cannot tell a working check from an unreachable
// one, which is the failure this ledger has already shipped twice.

// supersedeBrowse retires the intent behind exactly one fragment, and accounts
// for it. browse-the-things sources only surface:thing-list in the fixture.
const supersedeBrowse = `---
amendment: browsing-moves-to-search
date: 2026-08-26
trigger: "the list view is replaced by search"
affects:
  - "@verify-fixture/surface:thing-list"
supersedes_intents:
  - browse-the-things
---

## Change
Browsing is replaced by search over the same collection.

## Why
The list could not scale past a few hundred things and nobody browsed it.

## Acceptance
- Searching for a thing by name returns it.
`

func TestCheckAmendments_SupersessionAccountingForItsScopePasses(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-browsing-moves-to-search.md", supersedeBrowse)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("accounted supersession should exit zero: %v (issues %+v)", err, out.Issues)
	}
	if got := out.SupersededIntents["browse-the-things"]; got != "browsing-moves-to-search" {
		t.Errorf("expected the retiring amendment to be reported; got %q (%+v)", got, out.SupersededIntents)
	}
}

func TestCheckAmendments_SupersessionOrphaningScopeIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	// create-the-thing sources BOTH operation:thing.create and
	// surface:create-form; this names neither.
	writeAmendment(t, featDir, "001-drop-creation.md", `---
amendment: drop-creation
date: 2026-08-26
supersedes_intents:
  - create-the-thing
---

## Change
Things are no longer created through this feature.

## Why
Creation moved to the import pipeline.

## Acceptance
- The import pipeline is the only way a thing enters the system.
`)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	var msg string
	for _, iss := range out.Issues {
		if iss.Code == "intent-supersession-unaccounted-affect" {
			msg = iss.Message
		}
	}
	if msg == "" {
		t.Fatalf("retiring a promise must not silently orphan what it produced; issues %+v", out.Issues)
	}
	for _, want := range []string{"operation:thing.create", "surface:create-form"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected %s to be named as unaccounted; got %q", want, msg)
		}
	}
}

func TestCheckAmendments_SupersedingUnknownIntentIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-ghost.md", strings.Replace(supersedeBrowse,
		"  - browse-the-things", "  - a-promise-never-made", 1))

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "amendment-supersedes-intent-unknown") {
		t.Errorf("an amendment may only retire a promise its own feature made; issues %+v", out.Issues)
	}
}

func TestCheckAmendments_TwoClaimsOnOneIntentFork(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-browsing-moves-to-search.md", supersedeBrowse)
	second := strings.Replace(supersedeBrowse, "amendment: browsing-moves-to-search", "amendment: browsing-moves-to-feed", 1)
	writeAmendment(t, featDir, "002-browsing-moves-to-feed.md", second)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "amendment-supersedes-intent-forked") {
		t.Fatalf("two live decisions retiring one promise have no ordering; issues %+v", out.Issues)
	}

	// Declaring which decision stands is the existing ledger relation, not a
	// second ordering model invented for supersession.
	ordered := strings.Replace(second, "date: 2026-08-26", "date: 2026-08-26\nsupersedes:\n  - browsing-moves-to-search", 1)
	writeAmendment(t, featDir, "002-browsing-moves-to-feed.md", ordered)
	out2, _ := runCheckAmendments_(t, "@verify-fixture")
	if amendmentIssueSeen(out2, "amendment-supersedes-intent-forked") {
		t.Errorf("naming the earlier amendment should settle the fork; issues %+v", out2.Issues)
	}
}

func TestCheckAmendments_CannotRetireEveryIntent(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-retire-everything.md", `---
amendment: retire-everything
date: 2026-08-26
affects:
  - "@verify-fixture/operation:thing.create"
  - "@verify-fixture/surface:create-form"
  - "@verify-fixture/surface:thing-list"
supersedes_intents:
  - create-the-thing
  - browse-the-things
---

## Change
The feature is withdrawn.

## Why
Its job moved elsewhere.

## Acceptance
- Nothing in this feature is reachable.
`)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "amendment-supersedes-last-intent") {
		t.Errorf("a feature promising nothing is a lifecycle question, not a ledger entry; issues %+v", out.Issues)
	}
}

// The negative control: an ordinary ledger, with no supersession anywhere,
// must not acquire any of these findings. A check that fires without its
// subject present is matching something other than what it claims to.
func TestCheckAmendments_OrdinaryLedgerRaisesNoSupersessionFindings(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-26
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
Creation rejects duplicate names.

## Why
Two things with one name are one thing.

## Acceptance
- Creating a duplicate is rejected.
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("ordinary ledger should stay healthy: %v (issues %+v)", err, out.Issues)
	}
	for _, iss := range out.Issues {
		if strings.Contains(iss.Code, "supersed") || strings.Contains(iss.Code, "intent-supersession") {
			t.Errorf("supersession finding %q on a ledger with no supersession: %s", iss.Code, iss.Message)
		}
	}
	if len(out.SupersededIntents) != 0 {
		t.Errorf("expected no retired intents, got %+v", out.SupersededIntents)
	}
}

func amendmentIssueSeen(out checkAmendmentsOutput, code string) bool {
	for _, iss := range out.Issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}
