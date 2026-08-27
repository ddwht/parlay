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

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
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

func TestSourceNamesIntent_MatchesEveryRealRefShape(t *testing.T) {
	// All three shapes occur in a real tree; only the last segment is ever
	// the intent slug.
	for _, src := range []string{
		"browse-the-things",
		"@verify-fixture/browse-the-things",
		"@parlay-tool/verify-fixture/browse-the-things",
		"@verify-fixture/create-the-thing, @verify-fixture/browse-the-things",
	} {
		if !sourceNamesIntent(src, "verify-fixture", "browse-the-things") {
			t.Errorf("source %q should name the intent", src)
		}
	}
	// And the feature may be addressed by full slug or bare name.
	if !sourceNamesIntent("@verify-fixture/browse-the-things", "parlay-tool/verify-fixture", "browse-the-things") {
		t.Error("a bare feature name should still resolve against an initiative-qualified feature")
	}
}

func TestSourceNamesIntent_DoesNotMatchAnotherFeaturesIntent(t *testing.T) {
	// A contract entry may legitimately source another feature's intent —
	// cross-feature pressure looks exactly like this on disk. Matching the last
	// segment alone would let an identically-slugged intent elsewhere satisfy
	// the lookup, and the author would be handed a blocking demand to account
	// for an entry that does not derive from the retired promise at all.
	if sourceNamesIntent("@other-feature/browse-the-things", "verify-fixture", "browse-the-things") {
		t.Error("another feature's identically-slugged intent must not match")
	}
	if sourceNamesIntent("@parlay-tool/other-feature/browse-the-things", "parlay-tool/verify-fixture", "browse-the-things") {
		t.Error("initiative-qualified refs must still be checked against the feature")
	}
	if sourceNamesIntent("@verify-fixture/create-the-thing", "verify-fixture", "browse-the-things") {
		t.Error("a different intent in the same feature must not match")
	}
}

// Fork detection has to follow the supersession CHAIN, not just find some
// earlier claimant. 001 claims X; 002 claims X and supersedes 001; 003 claims X
// but supersedes 001 rather than the live 002. 002 and 003 are competing live
// heads, and ordering after a decision 002 already replaced settles nothing.
func TestCheckAmendments_ForkAgainstALiveHeadNotAnyPredecessor(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	mk := func(slug string, supersedes string) string {
		sup := ""
		if supersedes != "" {
			sup = "supersedes:\n  - " + supersedes + "\n"
		}
		return "---\namendment: " + slug + "\ndate: 2026-08-26\n" + sup +
			"affects:\n  - \"@verify-fixture/surface:thing-list\"\nsupersedes_intents:\n  - browse-the-things\n---\n\n## Change\nc\n\n## Why\nw\n\n## Acceptance\n- a\n"
	}
	writeAmendment(t, featDir, "001-first.md", mk("first", ""))
	writeAmendment(t, featDir, "002-second.md", mk("second", "first"))
	writeAmendment(t, featDir, "003-third.md", mk("third", "first"))

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "amendment-supersedes-intent-forked") {
		t.Fatalf("003 orders after a decision 002 already replaced, so 002 and 003 are competing live heads; issues=%+v", out.Issues)
	}
	var msg string
	for _, iss := range out.Issues {
		if iss.Code == "amendment-supersedes-intent-forked" {
			msg = iss.Message
		}
	}
	if !strings.Contains(msg, "second") {
		t.Errorf("the message must name the live head that was not superseded, not whichever claimant came before by position; got %q", msg)
	}
}

// A legitimate transitive chain must NOT fork: 003 supersedes 002 which
// supersedes 001, so there is exactly one live head throughout.
func TestCheckAmendments_TransitiveChainIsNotAFork(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	mk := func(slug, supersedes string) string {
		return "---\namendment: " + slug + "\ndate: 2026-08-26\nsupersedes:\n  - " + supersedes +
			"\naffects:\n  - \"@verify-fixture/surface:thing-list\"\nsupersedes_intents:\n  - browse-the-things\n---\n\n## Change\nc\n\n## Why\nw\n\n## Acceptance\n- a\n"
	}
	writeAmendment(t, featDir, "001-first.md",
		"---\namendment: first\ndate: 2026-08-26\naffects:\n  - \"@verify-fixture/surface:thing-list\"\nsupersedes_intents:\n  - browse-the-things\n---\n\n## Change\nc\n\n## Why\nw\n\n## Acceptance\n- a\n")
	writeAmendment(t, featDir, "002-second.md", mk("second", "first"))
	writeAmendment(t, featDir, "003-third.md", mk("third", "second"))

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if amendmentIssueSeen(out, "amendment-supersedes-intent-forked") {
		t.Errorf("a single chain of replacements has one live head and is not a fork; issues=%+v", out.Issues)
	}
}

func TestSourceNamesIntent_DistinguishesInitiatives(t *testing.T) {
	// Basename comparison cannot tell two initiatives apart, so it must not be
	// reached when both sides carry one.
	if sourceNamesIntent("@initiative-b/catalog/same-intent", "initiative-a/catalog", "same-intent") {
		t.Error("two initiatives with an identically-named feature must not match")
	}
	if !sourceNamesIntent("@initiative-a/catalog/same-intent", "initiative-a/catalog", "same-intent") {
		t.Error("the same initiative and feature must still match")
	}
	// One side unqualified is the legacy shape the fallback exists for.
	if !sourceNamesIntent("@catalog/same-intent", "initiative-a/catalog", "same-intent") {
		t.Error("a legacy unqualified ref should still resolve")
	}
}

// A replacement must restate the disposition. The amendment it replaces is
// history, not specification, so its affects: cannot go on covering scope the
// standing decision says nothing about.
func TestCheckAmendments_ReplacementMustRestateDisposition(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	// 001 retires create-the-thing and accounts for both entries it sourced.
	writeAmendment(t, featDir, "001-drop-creation.md", `---
amendment: drop-creation
date: 2026-08-26
affects:
  - "@verify-fixture/operation:thing.create"
  - "@verify-fixture/surface:create-form"
supersedes_intents:
  - create-the-thing
---

## Change
Creation moves to import.

## Why
Bulk load replaced manual entry.

## Acceptance
- Import is the only way a thing enters.
`)
	// 002 replaces 001 and claims the same intent, accounting for nothing.
	writeAmendment(t, featDir, "002-drop-creation-differently.md", `---
amendment: drop-creation-differently
date: 2026-08-26
supersedes:
  - drop-creation
supersedes_intents:
  - create-the-thing
---

## Change
Creation moves to the API instead.

## Why
Import was the wrong home.

## Acceptance
- The API is the only way a thing enters.
`)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "intent-supersession-unaccounted-affect") {
		t.Fatalf("the standing decision accounts for nothing; a superseded amendment's affects: must not cover for it. issues=%+v", out.Issues)
	}
	for _, iss := range out.Issues {
		if iss.Code == "intent-supersession-unaccounted-affect" && !strings.Contains(iss.Message, "drop-creation-differently") {
			t.Errorf("the finding should name the standing decision that owes the disposition; got %q", iss.Message)
		}
	}
}

// An ordinary amendment must not be able to replace a governance decision.
//
// 001 retires an intent; 002 supersedes 001 but claims no intent itself. The
// ledger says a superseded amendment is history, so 001 no longer speaks — yet
// the retirement it performed is still in force, resting on a decision the
// ledger has replaced. Reading it the other way is worse: the promise would
// quietly come back, undone by an ordinary amendment that never faced the
// no-safe-default gate retiring it required.
//
// So the replacement has to restate the retirement, and take on its Why,
// Acceptance and scope obligations with it.
func TestCheckAmendments_OrdinaryAmendmentCannotReplaceAGovernanceDecision(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-browsing-moves-to-search.md", supersedeBrowse)
	writeAmendment(t, featDir, "002-unrelated.md", `---
amendment: unrelated
date: 2026-08-26
supersedes:
  - browsing-moves-to-search
affects:
  - "@verify-fixture/surface:thing-list"
---

## Change
Something else about the list.

## Why
Unrelated to the retirement.

## Acceptance
- The list renders.
`)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "amendment-supersedes-governance-incomplete") {
		t.Fatalf("an ordinary amendment must not replace a governance decision without restating it; issues=%+v", out.Issues)
	}

	// Restating it settles the objection — the replacement now owns the
	// retirement and everything that comes with it.
	writeAmendment(t, featDir, "002-unrelated.md", `---
amendment: unrelated
date: 2026-08-26
supersedes:
  - browsing-moves-to-search
affects:
  - "@verify-fixture/surface:thing-list"
supersedes_intents:
  - browse-the-things
---

## Change
Browsing is replaced by a feed instead.

## Why
Search was the wrong answer.

## Acceptance
- The feed shows newest first.
`)
	out2, _ := runCheckAmendments_(t, "@verify-fixture")
	if amendmentIssueSeen(out2, "amendment-supersedes-governance-incomplete") {
		t.Errorf("restating the retirement should settle it; issues=%+v", out2.Issues)
	}
	if got := out2.SupersededIntents["browse-the-things"]; got != "unrelated" {
		t.Errorf("the standing decision should now be the replacement; got %q", got)
	}
}

// --- feature retirement --------------------------------------------------

func retirementAmendment(outcome, replacement, intents string) string {
	repl := ""
	if replacement != "" {
		repl = "replacement_feature: \"" + replacement + "\"\n"
	}
	return `---
amendment: close-the-feature
date: 2026-08-26
retires_feature: true
outcome: ` + outcome + `
` + repl + `supersedes_intents:
` + intents + `---

## Change
The feature is closed.

## Why
Its job moved out of this feature entirely.

## Acceptance
- Nothing in this feature is reachable.
`
}

const bothIntents = "  - create-the-thing\n  - browse-the-things\n"

// writeBareFeature creates the protocol-only shape: founding documents and
// nothing else. This is the shape retirement is actually for — 18 of 27
// features in this repo carry no contract artifact — and the only shape the
// narrow cut accepts, since retirement removes nothing and a feature with
// artifacts would keep them after being declared gone.
func writeBareFeature(t *testing.T, dir, name string) string {
	t.Helper()
	featDir := filepath.Join(dir, "spec", "intents", name)
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for f, body := range map[string]string{
		"intents.md": "# Bare\n\n## Do The Thing\n\n**Goal**: g.\n**Persona**: p.\n\n## Do The Other Thing\n\n**Goal**: g2.\n**Persona**: p.\n",
		"dialogs.md": "# Bare — Dialogs\n\n---\n",
	} {
		if err := os.WriteFile(filepath.Join(featDir, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return featDir
}

const bareIntents = "  - do-the-thing\n  - do-the-other-thing\n"

func TestFeatureRetirement_RetiresWhenNothingPointsAtIt(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	writeBaselineApplied(t, "bare", 1)

	out, err := runCheckAmendments_(t, "@bare")
	if err != nil {
		t.Fatalf("a feature nothing points at should retire: %v (issues %+v)", err, out.Issues)
	}
	if out.RetiredBy != "close-the-feature" {
		t.Errorf("the terminal amendment should be reported; got %q", out.RetiredBy)
	}
	// The last-intent refusal must NOT fire: that is the rule this operation
	// exists to satisfy, not one to work around.
	if amendmentIssueSeen(out, "amendment-supersedes-last-intent") {
		t.Errorf("retirement is what the last-intent refusal points at; it must not also block it: %+v", out.Issues)
	}
}

func TestFeatureRetirement_RefusesWhileSomethingPointsAtIt(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bothIntents))

	// Another feature's SPEC references it — and is never built, which is the
	// case a rebuild-scoping probe cannot see.
	otherDir := filepath.Join(dir, "spec", "intents", "consumer")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"intents.md": "# Consumer\n\n## Use The Thing\n\n**Goal**: g.\n**Persona**: p.\n",
		"surface.yaml": `feature: consumer
fragments:
    - name: Thing Echo
      page: things
      region: main
      shows: data-list
      order: 1
      source: '@consumer/use-the-thing'
      supersedes: '@verify-fixture/thing-list'
`,
	} {
		if err := os.WriteFile(filepath.Join(otherDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	var msg string
	for _, iss := range out.Issues {
		if iss.Code == "feature-retirement-still-referenced" {
			msg = iss.Message
		}
	}
	if msg == "" {
		t.Fatalf("retiring a feature something stands on must be refused; issues=%+v", out.Issues)
	}
	if !strings.Contains(msg, "consumer") || !strings.Contains(msg, "surface.yaml") {
		t.Errorf("the refusal must name the owning artifact and where; got %q", msg)
	}
}

// Provenance is not a dependency. A rule that blocks on any occurrence of the
// name is one people learn to route around.
func TestFeatureRetirement_ProvenanceAndProseDoNotBlock(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bothIntents))

	otherDir := filepath.Join(dir, "spec", "intents", "consumer")
	if err := os.MkdirAll(filepath.Join(otherDir, "amendments"), 0o755); err != nil {
		t.Fatal(err)
	}
	// intents.md naming it is a story about why this feature exists; a
	// trigger: naming it records what prompted a change.
	if err := os.WriteFile(filepath.Join(otherDir, "intents.md"),
		[]byte("# Consumer\n\n## Use It\n\n**Goal**: replaces @verify-fixture eventually.\n**Persona**: p.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, otherDir, "001-because-of-them.md", `---
amendment: because-of-them
date: 2026-08-26
trigger: "@verify-fixture needed this"
affects:
  - "@consumer/surface:thing-echo"
---

## Change
c

## Acceptance
- a
`)

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if amendmentIssueSeen(out, "feature-retirement-still-referenced") {
		for _, iss := range out.Issues {
			if iss.Code == "feature-retirement-still-referenced" {
				t.Errorf("prose and trigger provenance are not dependencies: %s", iss.Message)
			}
		}
	}
}

func TestFeatureRetirement_OutcomeAndReplacementRules(t *testing.T) {
	cases := []struct {
		name, outcome, replacement, want string
	}{
		{"replaced needs a destination", "replaced", "", "amendment-retirement-replacement-missing"},
		{"obsolete forbids one", "obsolete", "@other", "amendment-retirement-replacement-unexpected"},
		{"outcome is closed", "archived", "", "amendment-retirement-outcome-unknown"},
		{"cannot replace itself", "replaced", "@verify-fixture", "amendment-retirement-replaces-itself"},
		{"replacement must exist", "replaced", "@nowhere", "amendment-retirement-replacement-unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			featDir := writeVerifyFixture(t, dir)
			writeAmendment(t, featDir, "001-close-the-feature.md",
				retirementAmendment(tc.outcome, tc.replacement, bothIntents))
			out, _ := runCheckAmendments_(t, "@verify-fixture")
			if !amendmentIssueSeen(out, tc.want) {
				t.Errorf("expected %s; issues=%+v", tc.want, out.Issues)
			}
		})
	}
}

func TestFeatureRetirement_MustNameEveryLivePromise(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-close-the-feature.md",
		retirementAmendment("obsolete", "", "  - create-the-thing\n"))

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "amendment-retirement-incomplete") {
		t.Fatalf("a promise left unnamed stands after the feature is gone; issues=%+v", out.Issues)
	}
	for _, iss := range out.Issues {
		if iss.Code == "amendment-retirement-incomplete" && !strings.Contains(iss.Message, "browse-the-things") {
			t.Errorf("the refusal must name what was missed; got %q", iss.Message)
		}
	}
}

// The marker is what carries the obligations, so the fields mean nothing
// without it — and quietly ignoring them would let an author believe they had
// retired a feature.
func TestFeatureRetirement_FieldsWithoutTheMarkerAreRefused(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-not-really.md", `---
amendment: not-really
date: 2026-08-26
outcome: obsolete
affects:
  - "@verify-fixture/operation:thing.create"
---

## Change
c

## Why
w

## Acceptance
- a
`)
	out, _ := runCheckAmendments_(t, "@verify-fixture")
	if !amendmentIssueSeen(out, "amendment-retirement-fields-without-marker") {
		t.Errorf("outcome: without retires_feature: must not be silently ignored; issues=%+v", out.Issues)
	}
}

// The precondition that makes the narrow cut sound rather than merely narrow.
// Retirement removes nothing, so a feature with artifacts would keep them on
// disk and readable after being declared gone.
func TestFeatureRetirement_RefusesAFeatureThatHasOutput(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir) // carries surface.yaml + capabilities.yaml
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bothIntents))

	out, _ := runCheckAmendments_(t, "@verify-fixture")
	var msg string
	for _, iss := range out.Issues {
		if iss.Code == "feature-retirement-has-output" {
			msg = iss.Message
		}
	}
	if msg == "" {
		t.Fatalf("a feature with artifacts must be refused, not partially retired; issues=%+v", out.Issues)
	}
	for _, want := range []string{"surface.yaml", "capabilities.yaml"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal must name what is still there; %q missing from %q", want, msg)
		}
	}
}

// Declared is not effective. Saying a feature has ended while it still makes
// every promise it ever did is the same error as treating an unapplied
// supersession as current.
func TestFeatureRetirement_UnappliedIsPendingNotEnded(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	// No baseline: recorded, not applied.

	out, _ := runCheckAmendments_(t, "@bare")
	if out.RetiredBy != "" {
		t.Errorf("an unapplied retirement has not ended the feature; retired_by=%q", out.RetiredBy)
	}
	if out.PendingRetirement != "close-the-feature" {
		t.Errorf("it should be reported as pending; got %q", out.PendingRetirement)
	}
}

func TestFeatureRetirement_MustBeTheLastWordInTheLedger(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	// A record after the end of the feature changes something that has
	// stopped existing.
	writeAmendment(t, featDir, "002-after-the-end.md", `---
amendment: after-the-end
date: 2026-08-27
affects:
  - "@bare/surface:something"
---

## Change
c

## Acceptance
- a
`)
	out, _ := runCheckAmendments_(t, "@bare")
	if !amendmentIssueSeen(out, "amendment-retirement-not-terminal") {
		t.Errorf("a feature cannot carry on after it ended; issues=%+v", out.Issues)
	}
}

// An amendment in ANOTHER feature's ledger can name this feature's contract in
// affects:. That position was documented as counted and was structurally
// unreachable, because the scan was keyed by filename and a ledger is a
// directory.
func TestFeatureRetirement_AmendmentAffectsIsADependency(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	writeBaselineApplied(t, "bare", 1)

	consumer := writeBareFeature(t, dir, "consumer")
	writeAmendment(t, consumer, "001-reaches-in.md", `---
amendment: reaches-in
date: 2026-08-27
affects:
  - "@bare/surface:some-fragment"
---

## Change
c

## Acceptance
- a
`)

	out, _ := runCheckAmendments_(t, "@bare")
	var msg string
	for _, iss := range out.Issues {
		if iss.Code == "feature-retirement-still-referenced" {
			msg = iss.Message
		}
	}
	if msg == "" {
		t.Fatalf("a cross-feature affects: is a dependency on this feature's contract; issues=%+v", out.Issues)
	}
	if !strings.Contains(msg, "consumer") || !strings.Contains(msg, "amendment") {
		t.Errorf("the refusal should name the amendment and its owner; got %q", msg)
	}
}

// Generated output is recorded in the code-hashes sidecar, and the buildfile is
// not a proxy for it. A partial or stale state — no buildfile, non-empty Files
// map — would have retired cleanly while the generated code kept shipping,
// which is the exact failure the precondition exists to prevent.
func TestFeatureRetirement_RefusesWhenGeneratedCodeIsStillRecorded(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	writeBaselineApplied(t, "bare", 1)

	cfg := testContext(t)

	// Written in the shape the CLI actually produces: the PROJECT snapshot.
	// The per-feature sidecar is only ever written by a helper documented as
	// being for tests, so a fixture using it proved that a test-only artifact
	// blocks retirement while real generated output passed straight through.
	src := filepath.Join(dir, "src", "features", "bare")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(src, "Bare.tsx")
	if err := os.WriteFile(generated, []byte("// parlay-feature: bare\n// parlay-component: bare-view\nexport const Bare = () => null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProjectCodeHashes(t, cfg, map[string]CodeHashEntry{
		"src/features/bare/Bare.tsx": {Component: "bare-view", Hash: "abc", Provenance: ProvenanceGenerated},
	})

	out, _ := runCheckAmendments_(t, "@bare")
	if !amendmentIssueSeen(out, "feature-retirement-has-output") {
		t.Fatalf("a feature whose generated code is still recorded must not retire; issues=%+v", out.Issues)
	}
}

// A file that merely EXTENDS one of the retiring feature's components is partly
// its output and would outlive it.
func TestFeatureRetirement_RefusesWhenAFileExtendsIt(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	writeBaselineApplied(t, "bare", 1)

	cfg := testContext(t)
	src := filepath.Join(dir, "src", "shared")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Shared.tsx"),
		[]byte("// parlay-feature: other\n// parlay-extends: bare/bare-view\nexport const S = () => null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProjectCodeHashes(t, cfg, map[string]CodeHashEntry{
		"src/shared/Shared.tsx": {Component: "shared", Hash: "def", Provenance: ProvenanceGenerated},
	})

	out, _ := runCheckAmendments_(t, "@bare")
	if !amendmentIssueSeen(out, "feature-retirement-has-output") {
		t.Errorf("a file extending the feature's component is partly its output; issues=%+v", out.Issues)
	}
}

// A feature that owns none of the tracked files still retires: the snapshot is
// project-wide, so "anything is tracked" is not the question.
func TestFeatureRetirement_OtherFeaturesOutputDoesNotBlock(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	writeBaselineApplied(t, "bare", 1)

	cfg := testContext(t)
	src := filepath.Join(dir, "src", "elsewhere")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Other.tsx"),
		[]byte("// parlay-feature: unrelated\n// parlay-component: other\nexport const O = () => null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProjectCodeHashes(t, cfg, map[string]CodeHashEntry{
		"src/elsewhere/Other.tsx": {Component: "other", Hash: "ghi", Provenance: ProvenanceGenerated},
	})

	out, err := runCheckAmendments_(t, "@bare")
	if err != nil {
		t.Fatalf("another feature's output is not this feature's: %v (issues %+v)", err, out.Issues)
	}
}

func writeProjectCodeHashes(t *testing.T, cfg *config.Context, files map[string]CodeHashEntry) {
	t.Helper()
	path := projectCodeHashesPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(&CodeHashes{SchemaVersion: 2, Files: files})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A feature-local domain model is authored contract the earlier fixed list did
// not name, and it exists in this tree.
func TestFeatureRetirement_RefusesOnAnyAuthoredArtifact(t *testing.T) {
	for _, artifact := range []string{"domain-model.md", "domain-model.yaml", "surface.md", "things.layout.yaml"} {
		t.Run(artifact, func(t *testing.T) {
			dir := setupTestDir(t)
			featDir := writeBareFeature(t, dir, "bare")
			if err := os.WriteFile(filepath.Join(featDir, artifact), []byte("# x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
			writeBaselineApplied(t, "bare", 1)

			out, _ := runCheckAmendments_(t, "@bare")
			if !amendmentIssueSeen(out, "feature-retirement-has-output") {
				t.Errorf("%s is authored contract retirement would leave behind; issues=%+v", artifact, out.Issues)
			}
		})
	}
}
